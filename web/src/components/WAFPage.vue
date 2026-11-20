<script setup lang="ts">
import { computed, nextTick, onMounted, onBeforeUnmount, ref } from "vue";
import { request, errorMessage, jsonBody } from "../api";
import type { State, WAFInstance } from "../types";
import AppSelect from "./AppSelect.vue";
const props = defineProps<{ state: State; busy: boolean }>();
const emit = defineEmits<{ refresh: []; apply: [] }>();
interface Hit {
 body_limit?: boolean;
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
  draft = ref<WAFInstance>(),
  deleting = ref(false);
function siteName(id: string) {
  return props.state.rules.find((r) => r.id === id)?.domains.join(", ") ?? id;
}
async function edit(p?: WAFInstance, remove = false) {
  draft.value = p
    ? { ...p, sites: [...(p.sites ?? [])] }
    : { id: "", name: "", mode: "detect", sites: [] };
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
  if (!p) return;
  saving.value = true;
  error.value = "";
  try {
    await request("/waf/instances" + (p.id ? "/" + p.id : ""), {
      method: deleting.value ? "DELETE" : p.id ? "PUT" : "POST",
      body: deleting.value ? undefined : jsonBody(p),
    });
    dialog.value?.close();
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
</script>
<template>
  <section class="card waf">
    <header>
      <h2>WAF 防火墙</h2>
      <button
        v-if="state.dirty"
        class="button"
        :disabled="busy"
        @click="emit('apply')"
      >
        应用配置
      </button>
    </header>
    <header>
      <h3>防护实例{{ state.dirty ? " · 草稿" : "" }}</h3>
      <button class="button primary" @click="edit()">创建实例</button>
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
              <button class="button" @click="edit(p)">编辑</button>
              <button class="button" @click="edit(p, true)">删除</button>
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
            <td>{{ [...h.rules,...(h.body_limit?["请求体限制"]:[])].join(", ") }}</td>
            <td :class="{ blocked: h.blocked }">
              {{ h.blocked ? "已拦截" : "仅记录" }}
            </td>
          </tr>
          <tr v-if="!visible.length">
            <td colspan="6">暂无匹配的防护事件</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
  <dialog ref="dialog" class="waf-dialog" @close="draft = undefined">
    <form v-if="draft" @submit.prevent="save">
      <h3>
        {{
          deleting ? "删除防护实例" : draft.id ? "编辑防护实例" : "创建防护实例"
        }}
      </h3>
      <p v-if="deleting">
        删除「{{ draft.name }}」并应用后，关联站点将停止使用此实例。
      </p>
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
        <p v-if="draft.mode === 'block'">拦截模式会拒绝达到 8 MiB 的请求体。</p>
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
        </fieldset></template
      >
      <p v-if="error" role="alert">{{ error }}</p>
      <footer>
        <button type="button" class="button" @click="dialog?.close()">
          取消</button
        ><button class="button primary" :disabled="saving">
          {{ deleting ? "删除并保存草稿" : "保存草稿" }}
        </button>
      </footer>
    </form>
  </dialog>
</template>
<style scoped>
.waf + .waf {
  margin-top: 14px;
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
  padding: clamp(12px, 2vw, 24px);
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
.waf h2,
.waf h3 {
  margin: 0;
}
.waf header + header {
  margin-top: 22px;
}
.scroll {
  overflow: auto;
  margin-top: 12px;
}
.waf table {
  border-collapse: collapse;
  width: 100%;
  font-size: 14px;
}
.waf th,
.waf td {
  text-align: left;
  padding: 12px 8px;
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
.waf-dialog::backdrop {
  background: #10182066;
}
.waf-dialog h3 {
  margin: 0 0 14px;
}
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
