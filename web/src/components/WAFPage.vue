<script setup lang="ts">
import { computed, nextTick, onMounted, onBeforeUnmount, ref } from "vue";
import { request, errorMessage, jsonBody } from "../api";
import type { State, WAFInstance } from "../types";
import WAFRuleTable from "./WAFRuleTable.vue";
import AppSelect from "./AppSelect.vue";
const props = defineProps<{ state: State; busy: boolean }>();
const emit = defineEmits<{
  importConfig: [];
  exportConfig: [id: string, name: string];
 refresh: []; apply: [] }>();
interface Hit {
  body_limit?: boolean;
  partial_body?: boolean;
  time: string;
  site: string;
  host: string;
  client: string;
  rules: number[];
  path: string;
  blocked: boolean;
}
const hits = ref<Hit[]>([]),
  site = ref(""),
  client = ref(""),
  minutes = ref(60),
  error = ref(""),
  logError = ref(""),
  saving = ref(false);
const visible = computed(() =>
  hits.value.filter(
    (h) =>
      (!site.value || h.site === site.value) &&
      h.client.includes(client.value.trim()) &&
      (minutes.value === 0 ||
        Date.parse(h.time) >= Date.now() - minutes.value * 60000),
  ),
);
const instances = computed(() => props.state.waf_instances ?? []);
const dialog = ref<HTMLDialogElement>(),
  draft = ref<WAFInstance & Required<Pick<WAFInstance, "body_limit_mib" | "body_limit_action" | "upload_sites">>>(),
  deleting = ref(false);
const tab = ref<"basic"|"rules">("basic");
function siteName(id: string) {
  return props.state.rules.find((r) => r.id === id)?.domains.join(", ") ?? id;
}
async function edit(p?: WAFInstance, remove = false) {
  tab.value = "basic";
  draft.value = {
    id: "", name: "", mode: "detect", ...p,
    sites: [...(p?.sites ?? [])], exclusions: { ...p?.exclusions },
    body_limit_mib: p?.body_limit_mib || 8,
    body_limit_action: p?.body_limit_action || "reject",
    upload_sites: [...(p?.upload_sites ?? [])],
  };
  deleting.value = remove;
  error.value = "";
  await nextTick();
  dialog.value?.showModal();
}
const available = computed(() =>
  props.state.rules.filter(
    (r) =>
      !instances.value.some(
        (p) => p.id !== draft.value?.id && p.sites?.includes(r.id),
      ),
  ),
);
async function save() {
  const p = draft.value;
  if (!p || saving.value) return;
  if (!deleting.value && !p.name.trim()) {
    tab.value = "basic";
    error.value = "请输入实例名称";
    return;
  }
  const exclusions = Object.fromEntries(Object.entries(p.exclusions ?? {})
    .filter(([site, ids]) => p.sites.includes(site) && ids.length));
  if (!deleting.value && (!Number.isInteger(p.body_limit_mib) || !p.body_limit_mib || p.body_limit_mib < 1 || p.body_limit_mib > 128)) {
    tab.value = "basic";
    error.value = "请求体检查上限须为 1–128 MiB 的整数";
    return;
  }
  const upload_sites = p.upload_sites?.filter(site => p.sites.includes(site));
  saving.value = true;
  error.value = "";
  try {
    if (p.id && !deleting.value && !instances.value.some(item => item.id === p.id)) {
      await request("/config-form/create", {method:"POST",body:jsonBody({kind:"waf",id:p.id,value:{...p,exclusions,upload_sites}})});
    } else await request("/waf/instances" + (p.id ? "/" + p.id : ""), {
      method: deleting.value ? "DELETE" : p.id ? "PUT" : "POST",
      body: deleting.value ? undefined : jsonBody({ ...p, exclusions, upload_sites }),
    });
    draft.value = undefined;
    emit("refresh");
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
let timer: ReturnType<typeof setTimeout>,
  gone = false;
async function poll() {
  try {
    hits.value = await request<Hit[]>("/waf/events");
    logError.value = "";
  } catch (e) {
    logError.value = errorMessage(e);
  } finally {
    if (!gone) timer = setTimeout(poll, 3000);
  }
}
onMounted(poll);
onBeforeUnmount(() => {
  gone = true;
  clearTimeout(timer);
});
defineExpose({ edit });
</script>
<template>
  <section class="card waf">
    <header>
      <h3>防护实例{{ state.dirty ? " · 草稿" : "" }}</h3>
      <div class="waf-actions">
        <button
          v-if="state.dirty"
          class="button"
          :disabled="busy"
          @click="emit('apply')"
        >
          应用配置
        </button>
        <button class="button ghost" @click="$emit('importConfig')">从 Nginx 配置导入</button>
    <button class="button primary" @click="edit()">创建实例</button>
      </div>
    </header>
    <div class="scroll">
      <table>
        <thead>
          <tr>
            <th>实例名称</th>
            <th>规则集</th>
            <th>工作模式</th>
            <th>关联站点</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in instances" :key="p.id">
            <td>{{ p.name }}</td>
            <td>OWASP CRS v4</td>
            <td>
              <span class="mode" :class="{ enforcing: p.mode === 'block' }">{{
                p.mode === "block" ? "拦截" : "仅检测"
              }}</span>
            </td>
            <td>{{ p.sites?.map(siteName).join("、") || "尚未绑定" }}</td>
            <td>
              <div class="instance-actions">
                <button class="button ghost small" @click="$emit('exportConfig', p.id, p.name)">复制配置</button>
                <button class="button" @click="edit(p)">编辑</button>
                <button class="button danger-ghost" @click="edit(p, true)">删除</button>
              </div>
            </td>
          </tr>
          <tr v-if="!instances.length">
            <td colspan="5">暂无防护实例</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
  <section class="card waf">
    <header>
      <h3>命中日志</h3>
      <div class="filters">
        <AppSelect class="select" aria-label="筛选站点" v-model="site"
          ><option value="">全部站点</option>
          <option v-for="s in state.rules" :key="s.id" :value="s.id">
            {{ siteName(s.id) }}
          </option></AppSelect
        ><input
          class="input"
          aria-label="过滤来源 IP"
          v-model="client"
          placeholder="来源 IP"
        />
        <AppSelect class="select" aria-label="日志时间范围" v-model="minutes"
          ><option :value="15">近 15 分钟</option>
          <option :value="60">近 1 小时</option>
          <option :value="0">全部时段</option></AppSelect
        >
      </div>
    </header>
    <p v-if="logError" role="alert">{{ logError }}</p>
    <div class="scroll">
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>来源 IP</th>
            <th>站点</th>
            <th>请求路径</th>
            <th>规则 ID</th>
            <th>处理结果</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(h, i) in visible" :key="h.time + i">
            <td>{{ new Date(h.time).toLocaleTimeString() }}</td>
            <td>{{ h.client }}</td>
            <td>{{ h.host }}</td>
            <td>{{ h.path }}</td>
            <td>{{ h.rules?.join(", ") || "—" }}</td>
            <td :class="{ blocked: h.blocked }">
              <template v-if="h.body_limit">请求体超限 · {{ h.blocked ? "已拦截" : "已放行" }}<span v-if="h.partial_body">（已检查前段）</span></template>
              <template v-else>{{ h.blocked ? "已拦截" : "仅记录" }}</template>
            </td>
          </tr>
          <tr v-if="!visible.length">
            <td colspan="6">暂无匹配的防护事件</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
  <dialog v-if="draft" ref="dialog" class="waf-dialog" :class="{ wide: !deleting }" aria-labelledby="waf-dialog-title"
    @cancel.prevent="!saving && (draft = undefined)">
    <form @submit.prevent="save">
      <header class="editor-title"><h3 id="waf-dialog-title">
        {{
          deleting ? "删除防护实例" : draft.id ? "编辑防护实例" : "创建防护实例"
        }}
        {{ !deleting && draft.id ? " · " + draft.name : "" }}
      </h3><small v-if="!deleting">OWASP CRS 4.25.0</small><button type="button" class="icon-button" aria-label="关闭" :disabled="saving" @click="draft = undefined">×</button></header>
      <nav v-if="!deleting" class="editor-tabs">
        <button type="button" :aria-pressed="tab === 'basic'" @click="tab='basic'">基本设置</button>
        <button type="button" :aria-pressed="tab === 'rules'" @click="tab='rules'">规则管理</button>
      </nav>
      <p v-if="deleting">
        删除「{{ draft.name }}」并应用后，关联站点将停止使用此实例。
      </p>
      <WAFRuleTable v-else-if="tab === 'rules'" :draft="draft" :site-name="siteName"
        @change="(site, ids) => { if (draft) draft.exclusions = { ...draft.exclusions, [site]: ids }; }" />
      <template v-else
        ><label class="field-row"
          ><span>实例名称</span
          ><input
            class="input"
            required
            maxlength="80"
            v-model.trim="draft.name"
        /></label>
        <div class="field-row">
          <strong>工作模式</strong
          ><AppSelect class="select" aria-label="工作模式" v-model="draft.mode"
            ><option value="detect">仅检测</option>
            <option value="block">拦截</option></AppSelect
          >
        </div>

        <fieldset>
          <legend>关联站点</legend>
          <div v-for="r in available" :key="r.id" class="check">
            <input
              type="checkbox"
              :id="'waf-site-' + r.id"
              v-model="draft.sites"
              :value="r.id"
            /><label :for="'waf-site-' + r.id">{{ siteName(r.id) }}</label>
          </div>
          <span v-if="!available.length">暂无可绑定的 HTTP 代理站点</span>
        </fieldset>
        <details class="body-policy">
          <summary>请求体与上传</summary>
          <label class="field-row"><span>检查上限</span><span class="limit-input"><input class="input" type="number" min="1" max="128" step="1" required v-model.number="draft.body_limit_mib" aria-label="请求体检查上限（MiB）" />MiB</span></label>
          <div class="field-row"><strong>超限处理</strong><AppSelect v-model="draft.body_limit_action" class="select" aria-label="超限处理">
            <option value="reject">拒绝请求（413）</option><option value="partial">仅检查前段</option>
          </AppSelect></div>
          <p class="policy-help">{{ draft.body_limit_action === 'partial' ? '超过上限的正文不再检查。' : draft.mode === 'detect' ? '仅检测模式下，超限请求会记录并放行。' : '超过上限的上传会被拒绝。' }}</p>
          <fieldset><legend>WebDAV / 上传兼容</legend>
            <p class="policy-help">为需要文件管理或上传的站点勾选，仍检查请求内容。</p>
            <div v-for="id in draft.sites" :key="id" class="check">
              <input :id="'upload-site-' + id" type="checkbox" :value="id" v-model="draft.upload_sites" />
              <label :for="'upload-site-' + id">{{ siteName(id) }}</label>
            </div>
            <span v-if="!draft.sites.length">请先关联站点</span>
          </fieldset>
        </details></template
      >
      <p v-if="error" role="alert">{{ error }}</p>
      <footer>
        <button type="button" class="button" :disabled="saving" @click="draft = undefined">
          取消</button
        ><button class="button" :class="deleting ? 'danger' : 'primary'" :disabled="saving">
          {{ deleting ? "删除并保存草稿" : "保存草稿" }}
        </button>
      </footer>
    </form>
  </dialog>
</template>
<style scoped>
.waf + .waf {
  margin-top: 8px;
}
.filters {
  justify-content: flex-end;
  max-width: 100%;
}
.filters .input {
  width: min(160px, 100%);
}
.filters .select {
  width: auto;
  max-width: 100%;
}
@media (max-width: 600px) {
  .filters {
    justify-content: flex-start;
  }
}

.waf {
  padding: 10px 14px;
}
.mode {
  padding: 2px 8px;
  border-radius: 5px;
  color: var(--text);
  background: var(--surface-soft);
  white-space: nowrap;
}
.mode.enforcing {
  color: var(--accent-dark);
  background: var(--accent-soft);
}

.waf,
.scroll {
  min-width: 0;
}
.waf header,
.filters,
footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.waf h3 {
  margin: 0;
}
.waf-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.instance-actions {
  display: inline-flex;
  gap: 8px;
}
.scroll {
  overflow: auto;
  margin-top: 6px;
}
.waf table {
  border-collapse: collapse;
  width: 100%;
  font-size: 14px;
}
.waf th,
.waf td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--line);
  overflow-wrap: anywhere;
}
.waf th {
  color: var(--text-muted);
  white-space: nowrap;
}
.waf td:last-child {
  white-space: nowrap;
}
.blocked {
  color: var(--danger);
}
.waf-dialog {
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 18px;
  width: min(440px, 90vw);
}
.waf-dialog.wide { width: min(680px, calc(100vw - 32px)); }
.editor-title {display:flex;align-items:baseline;justify-content:space-between;gap:12px}
.editor-title small {color:var(--text-muted);margin-left:auto}
.editor-tabs {display:flex;gap:14px;border-bottom:1px solid var(--line);margin-bottom:10px}
.editor-tabs button {padding:6px 2px;background:none;border:0;border-bottom:2px solid transparent;color:var(--text-muted);cursor:pointer}
.editor-tabs button[aria-pressed="true"] {border-color:var(--accent);color:var(--text);font-weight:600}
.waf-dialog::backdrop {
  background: #10182066;
}
.waf-dialog h3 {
  margin: 0 0 14px;
}
.body-policy { margin-top: 12px; }
.body-policy summary { cursor: pointer; font-weight: 600; }
.limit-input { display: flex; align-items: center; gap: 8px; }
.limit-input input { width: 7em; }
.policy-help { margin: 6px 0; color: var(--text-muted); font-size: 13px; }
.field-row {
  display: grid;
  grid-template-columns: 75px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  margin: 10px 0;
  font-weight: 600;
}
.waf-dialog fieldset {
  border: 1px solid var(--line);
  border-radius: 8px;
  max-height: 35vh;
  overflow: auto;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px;
}
.check input {
  width: 16px;
  height: 16px;
  min-height: 0;
}
.waf-dialog footer {
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
