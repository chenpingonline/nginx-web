<script setup lang="ts">
import { PhCopySimple, PhSignIn } from "@phosphor-icons/vue";
import { minListenPort } from "../permissionMode";
import AppSelect from "./AppSelect.vue";
import { computed, reactive, ref, toRaw, watch } from "vue";
import type { Page, Settings } from "../types";
const appVersion = __APP_VERSION__;
const clientCAPlaceholder = __CONTAINER__ ? "/mnt/auth/client-ca.pem" : __STANDALONE__ ? "/srv/auth/client-ca.pem" : "/vol1/.../client-ca.pem";
const props = defineProps<{
  settings: Settings;
  menus: { id: Page; label: string }[];
  busy: boolean;
  dirty: boolean;
}>();
const emit = defineEmits<{
  importConfig: [];
  exportConfig: [id: string, name: string];

  save: [value: Settings];
  clearCache: [];
  test: [value: Settings];
  apply: [value: Settings];
}>();
const form = reactive<Settings>(structuredClone(toRaw(props.settings)));
const trusted = ref(""),
  gzipTypes = ref("");
function restore(value: Settings) {
  delete form.hidden_menus;
  Object.assign(form, structuredClone(toRaw(value)));
  trusted.value = value.real_ip.trusted_proxies.join("\n");
  gzipTypes.value = value.gzip.types.join("\n");
}
watch(() => props.settings, restore, { immediate: true, deep: true });
defineExpose({ restore });
const tabs = ["常用设置", "客户端真实 IP", "压缩", "动态路由变量", "Worker 与文件资源", "TLS 与日志", "菜单设置"] as const;
const activeTab = ref<typeof tabs[number]>("常用设置");
function moveTab(event: KeyboardEvent, index: number) {
  const target = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1
    : event.key === "ArrowRight" ? (index + 1) % tabs.length
    : event.key === "ArrowLeft" ? (index + tabs.length - 1) % tabs.length : -1;
  if (target < 0) return;
  event.preventDefault();
  activeTab.value = tabs[target]!;
  document.getElementById("settings-tab-" + target)?.focus();
}
function toggleMenu(id: Page) {
  const hidden = form.hidden_menus ?? [];
  const next = hidden.includes(id) ? hidden.filter(item => item !== id) : [...hidden, id];
  if (next.length) form.hidden_menus = next;
  else delete form.hidden_menus;
}
function formValue(): Settings {
  return {
    ...JSON.parse(JSON.stringify(form)),
    real_ip: {
      ...form.real_ip,
      trusted_proxies: trusted.value.split(/[\s,]+/).filter(Boolean),
    },
    gzip: {
      ...form.gzip,
      types: gzipTypes.value.split(/[\s,]+/).filter(Boolean),
    },
  };
}
function comparableSettings({ hidden_menus, ...settings }: Settings) {
  return JSON.stringify({ ...settings, hidden_menus: [...(hidden_menus ?? [])].sort() });
}
const hasChanges = computed(() => comparableSettings(formValue()) !== comparableSettings(props.settings));
function submit(event: Event) {
  const action = (event as SubmitEvent).submitter?.getAttribute("data-action");
  if (action === "test") emit("test", formValue());
  else if (action === "apply") emit("apply", formValue());
  else emit("save", formValue());
}
function addMap() {
  form.routing.maps.push({ name: "Host 路由", source: "$host", variable: "$backend", hostnames: true, default: "default", entries: [] });
}
function addGeo() {
  form.routing.geos.push({ name: "IP 分组", source: "$remote_addr", variable: "$region", default: "default", entries: [] });
}
function addSplit() {
  form.routing.splits.push({ name: "灰度分流", source: "$request_id", variable: "$variant", entries: [{ key: "10%", value: "canary" }, { key: "*", value: "stable" }] });
}
</script>
<template>
  <form class="settings-stack" @submit.prevent="submit">
    <article class="card settings-card">
      <div class="rule-editor-tabs settings-tabs" role="tablist" aria-label="全局设置">
        <button v-for="(tab, index) in tabs" :id="'settings-tab-' + index" :key="tab" type="button" role="tab"
          :aria-selected="activeTab === tab" aria-controls="settings-panel" :tabindex="activeTab === tab ? 0 : -1"
          @click="activeTab = tab" @keydown="moveTab($event, index)">{{ tab }}</button>
      </div>
      <div id="settings-panel" role="tabpanel" :aria-labelledby="'settings-tab-' + tabs.indexOf(activeTab)">
    <section v-if="activeTab === '常用设置'">
      <p class="settings-note">没有 HTTP 规则时，默认 HTTP 端口用于默认入口。</p>
      <div class="card-body form-grid">
        <div class="field">
          <label>新建代理默认 HTTP 端口</label
          ><input
            v-model.number="form.default_http_port"
            class="input"
            type="number"
            :min="minListenPort"
            max="65535"
            required
          />
        </div>
        <div class="field">
          <label>新建代理默认 HTTPS 端口</label
          ><input
            v-model.number="form.default_https_port"
            class="input"
            type="number"
            :min="minListenPort"
            max="65535"
            required
          />
        </div>
      </div>
    </section>
    <section v-if="activeTab === '客户端真实 IP'">
      <div class="card-body form-grid">
        <div class="field">
          <label>状态</label
          ><label class="checkbox-row"
            ><input v-model="form.real_ip.enabled" type="checkbox" /><span>启用 Real IP</span></label
          ><label class="checkbox-row"
            ><input
              v-model="form.real_ip.recursive"
              type="checkbox"
            /><span>递归解析代理链</span></label
          >
        </div>
        <div class="field">
          <label>来源 Header</label
          ><AppSelect v-model="form.real_ip.header" class="select">
            <option value="X-Forwarded-For">X-Forwarded-For</option>
            <option value="X-Real-IP">X-Real-IP</option>
            <option value="proxy_protocol">PROXY Protocol</option>
          </AppSelect>
        </div>
        <div class="field full">
          <label>可信代理 IP / CIDR</label
          ><textarea
            v-model="trusted"
            class="textarea"
            placeholder="192.168.1.0/24&#10;10.0.0.1"
          ></textarea>
        </div>
      </div>
    </section>
    <section v-if="activeTab === '压缩'">
      <div class="card-body form-grid">
        <div class="field">
          <label>开关</label
          ><label class="checkbox-row"
            ><input v-model="form.gzip.enabled" type="checkbox" /><span>启用 Gzip</span></label
          ><label class="checkbox-row"
            ><input v-model="form.gzip.static" type="checkbox" /><span>优先使用 .gz 文件</span></label
          ><label class="checkbox-row"
            ><input v-model="form.gzip.gunzip" type="checkbox" /><span>启用 Gunzip</span></label
          >
        </div>
        <div class="field">
          <label>压缩级别</label
          ><input
            v-model.number="form.gzip.level"
            class="input"
            type="number"
            min="1"
            max="9"
          /><label>最小响应大小（字节）</label
          ><input
            v-model.number="form.gzip.min_length"
            class="input"
            type="number"
            min="0"
            max="10485760"
          />
        </div>
        <div class="field full">
          <label>MIME 类型</label
          ><textarea v-model="gzipTypes" class="textarea"></textarea>
        </div>
      </div>
    </section>
    <section v-if="activeTab === '动态路由变量'">
      <div class="card-body settings-stack">
        <details class="advanced-box"><summary>Map（{{ form.routing.maps.length }}）</summary><div class="form-grid compact-grid">
          <div v-for="(item, index) in form.routing.maps" :key="`map-${index}`" class="routing-editor full">
            <input v-model.trim="item.name" class="input" placeholder="名称" /><input v-model.trim="item.source" class="input" placeholder="$host" /><input v-model.trim="item.variable" class="input" placeholder="$backend" /><input v-model="item.default" class="input" placeholder="默认值" />
            <label class="checkbox-row"><input v-model="item.hostnames" type="checkbox" /><span>域名匹配</span></label><button type="button" class="button danger compact" @click="form.routing.maps.splice(index, 1)">删除</button>
            <div v-for="(entry, entryIndex) in item.entries" :key="entryIndex" class="inline-editor routing-entry"><input v-model="entry.key" class="input" placeholder="匹配值" /><input v-model="entry.value" class="input" placeholder="输出值" /><button type="button" class="button danger compact" @click="item.entries.splice(entryIndex, 1)">−</button></div>
            <button type="button" class="button ghost compact fit" @click="item.entries.push({ key: '', value: '' })">添加条目</button>
          </div><button type="button" class="button ghost compact fit" @click="addMap">添加 Map</button>
        </div></details>
        <details class="advanced-box"><summary>Geo（{{ form.routing.geos.length }}）</summary><div class="form-grid compact-grid">
          <div v-for="(item, index) in form.routing.geos" :key="`geo-${index}`" class="routing-editor full"><input v-model.trim="item.name" class="input" placeholder="名称" /><input v-model.trim="item.source" class="input" placeholder="$remote_addr" /><input v-model.trim="item.variable" class="input" placeholder="$region" /><input v-model="item.default" class="input" placeholder="默认值" /><button type="button" class="button danger compact" @click="form.routing.geos.splice(index, 1)">删除</button><div v-for="(entry, entryIndex) in item.entries" :key="entryIndex" class="inline-editor routing-entry"><input v-model="entry.key" class="input" placeholder="IP / CIDR" /><input v-model="entry.value" class="input" placeholder="输出值" /><button type="button" class="button danger compact" @click="item.entries.splice(entryIndex, 1)">−</button></div><button type="button" class="button ghost compact fit" @click="item.entries.push({ key: '', value: '' })">添加条目</button></div>
          <button type="button" class="button ghost compact fit" @click="addGeo">添加 Geo</button>
        </div></details>
        <details class="advanced-box"><summary>Split Clients（{{ form.routing.splits.length }}）</summary><div class="form-grid compact-grid">
          <div v-for="(item, index) in form.routing.splits" :key="`split-${index}`" class="routing-editor full"><input v-model.trim="item.name" class="input" placeholder="名称" /><input v-model.trim="item.source" class="input" placeholder="$request_id" /><input v-model.trim="item.variable" class="input" placeholder="$variant" /><button type="button" class="button danger compact" @click="form.routing.splits.splice(index, 1)">删除</button><div v-for="(entry, entryIndex) in item.entries" :key="entryIndex" class="inline-editor routing-entry"><input v-model="entry.key" class="input" placeholder="10% 或 *" /><input v-model="entry.value" class="input" placeholder="输出值" /><button type="button" class="button danger compact" @click="item.entries.splice(entryIndex, 1)">−</button></div><button type="button" class="button ghost compact fit" @click="item.entries.splice(Math.max(0, item.entries.length - 1), 0, { key: '10%', value: '' })">添加比例</button></div>
          <button type="button" class="button ghost compact fit" @click="addSplit">添加 Split</button>
        </div></details>
      </div>
    </section>
    <section v-if="activeTab === 'Worker 与文件资源'">
      <div class="card-body form-grid">
        <div class="field">
          <label>Worker 数量</label
          ><input
            v-model.number="form.worker_processes"
            class="input"
            type="number"
            min="0"
            max="128"
          /><span class="field-help">0 表示根据 CPU 自动选择。</span>
        </div>
        <div class="field">
          <label>Worker Connections</label
          ><input
            v-model.number="form.worker_connections"
            class="input"
            type="number"
            min="128"
            max="1048576"
          />
        </div>
        <div class="field">
          <label>Worker 文件句柄上限</label
          ><input
            v-model.number="form.worker_rlimit_nofile"
            class="input"
            type="number"
            min="0"
            max="1048576"
          /><span class="field-help">0 表示自动限制到当前系统软上限。</span>
        </div>
        <div class="field">
          <label>事件处理</label
          ><label class="checkbox-row"
            ><input
              v-model="form.multi_accept"
              type="checkbox"
            /><span>一次接受多个连接</span></label
          ><label class="checkbox-row"
            ><input v-model="form.file_aio" type="checkbox" /><span>启用文件 AIO</span></label
          >
        </div>
        <div class="field"><label>线程池线程数</label><input v-model.number="form.thread_pool_threads" class="input" type="number" min="0" max="1024" /><span class="field-help">0 表示关闭；启用后文件 I/O 使用线程池。</span></div>
        <div class="field"><label>线程池队列上限</label><input v-model.number="form.thread_pool_queue" class="input" type="number" min="1" max="1048576" /></div>
      </div>
    </section>
    <section v-if="activeTab === 'TLS 与日志'">
      <div class="card-body form-grid">
        <div class="field">
          <label>TLS 协议</label
          ><label class="checkbox-row"
            ><input
              v-model="form.tls.protocols"
              type="checkbox"
              value="TLSv1.2"
            /><span>TLS 1.2</span></label
          ><label class="checkbox-row"
            ><input
              v-model="form.tls.protocols"
              type="checkbox"
              value="TLSv1.3"
            /><span>TLS 1.3</span></label
          >
        </div>
        <div class="field">
          <label>加密套件（可选）</label
          ><input
            v-model.trim="form.tls.ciphers"
            class="input"
            placeholder="留空使用 OpenSSL 默认值"
          /><label>会话缓存（MB）</label
          ><input
            v-model.number="form.tls.session_cache_mb"
            class="input"
            type="number"
            min="1"
            max="1024"
          /><label>会话超时（分钟）</label
          ><input
            v-model.number="form.tls.session_timeout_minutes"
            class="input"
            type="number"
            min="1"
            max="1440"
          />
          <label class="checkbox-row"><input v-model="form.tls.ocsp_stapling" type="checkbox" /><span>启用 OCSP Stapling</span></label>
        </div>
        <div class="field">
          <label>客户端证书校验</label><AppSelect v-model="form.tls.client_verify" class="select"><option value="off">关闭</option><option value="on">强制</option><option value="optional">可选并校验 CA</option><option value="optional_no_ca">可选且不校验 CA</option></AppSelect>
          <template v-if="form.tls.client_verify !== 'off'"><label>客户端 CA 文件</label><input v-model.trim="form.tls.client_ca_file" class="input" :placeholder="clientCAPlaceholder" required /><label>校验深度</label><input v-model.number="form.tls.client_verify_depth" class="input" type="number" min="1" max="10" /></template>
        </div>
        <div class="field">
          <label>访问日志</label
          ><label class="checkbox-row"
            ><input
              v-model="form.logging.access_enabled"
              type="checkbox"
            /><span>启用访问日志</span></label
          ><label>缓冲（KB）</label
          ><input
            v-model.number="form.logging.access_buffer_kb"
            class="input"
            type="number"
            min="0"
            max="1024"
          /><label>刷新间隔（秒）</label
          ><input
            v-model.number="form.logging.access_flush_seconds"
            class="input"
            type="number"
            min="1"
            max="3600"
          />
        </div>
        <div class="field">
          <label>错误日志级别</label
          ><AppSelect v-model="form.logging.error_level" class="select">
            <option
              v-for="level in [
                'debug',
                'info',
                'notice',
                'warn',
                'error',
                'crit',
                'alert',
                'emerg',
              ]"
              :key="level"
            >
              {{ level }}
            </option>
          </AppSelect>
        </div>
        <div class="field full"><label>自定义访问日志格式（可选）</label><input v-model="form.logging.custom_format" class="input" placeholder='$remote_addr [$time_local] "$request" $status' /><span class="field-help">留空使用内置 fnproxy 格式。</span></div>
        <div class="field"><label>日志轮转大小（MB）</label><input v-model.number="form.logging.rotate_size_mb" class="input" type="number" min="1" max="10240" /></div>
        <div class="field"><label>轮转文件保留数</label><input v-model.number="form.logging.rotate_keep" class="input" type="number" min="1" max="100" /></div>
      </div>
    </section>
      <section v-if="activeTab === '菜单设置'">
        <p class="settings-note">应用后，未勾选的菜单将隐藏，对应的代理转发、采集或自动续期也会停用。已有配置会保留。</p>
        <div class="card-body menu-options">
          <div v-for="menu in menus" :key="menu.id" class="menu-option">
            <input :id="'menu-' + menu.id" type="checkbox" :checked="!form.hidden_menus?.includes(menu.id)"
              :disabled="menu.id === 'settings'" @change="toggleMenu(menu.id)" />
            <label :for="'menu-' + menu.id">{{ menu.label }}</label>
          </div>
        </div>
      </section>
      </div>
    </article>
    <div class="settings-actions-shell"><div class="sticky-actions">
      <div class="settings-app-info">
        <a
          class="app-update-github"
          href="https://github.com/chenpingonline/nginx-web-fnos"
          target="_blank"
          rel="noopener noreferrer"
          aria-label="打开 nginx-web GitHub 仓库"
          title="GitHub 仓库"
        >
          <!-- GitHub circular mark from Primer Octicons; see THIRD_PARTY_LICENSES.md. -->
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="20"
            height="20"
            viewBox="0 0 16 16"
            fill="currentColor"
            aria-hidden="true"
            focusable="false"
          >
            <path d="M6.766 11.328c-2.063-.25-3.516-1.734-3.516-3.656 0-.781.281-1.625.75-2.188-.203-.515-.172-1.609.063-2.062.625-.078 1.468.25 1.968.703.594-.187 1.219-.281 1.985-.281.765 0 1.39.094 1.953.265.484-.437 1.344-.765 1.969-.687.218.422.25 1.515.046 2.047.5.593.766 1.39.766 2.203 0 1.922-1.453 3.375-3.547 3.64.531.344.89 1.094.89 1.954v1.625c0 .468.391.734.86.547C13.781 14.359 16 11.53 16 8.03 16 3.61 12.406 0 7.984 0 3.563 0 0 3.61 0 8.031a7.88 7.88 0 0 0 5.172 7.422c.422.156.828-.125.828-.547v-1.25c-.219.094-.5.156-.75.156-1.031 0-1.64-.562-2.078-1.609-.172-.422-.36-.672-.719-.719-.187-.015-.25-.093-.25-.187 0-.188.313-.328.625-.328.453 0 .844.281 1.25.86.313.452.64.655 1.031.655s.641-.14 1-.5c.266-.265.47-.5.657-.656" />
          </svg>
        </a>
        <span class="settings-app-version">v{{ appVersion }}</span>
      </div>
      <div class="config-tools" role="group" aria-label="配置操作">
        <button type="button" title="从 Nginx 配置导入" aria-label="从 Nginx 配置导入" @click="emit('importConfig')">
          <PhSignIn :size="17" aria-hidden="true" /><span>导入配置</span>
        </button>
        <button type="button" title="复制配置" aria-label="复制配置" @click="emit('exportConfig', '', '全局设置')">
          <PhCopySimple :size="17" aria-hidden="true" /><span>复制配置</span>
        </button>
      </div>
      <span class="spacer"></span>
      <button class="button danger-ghost" type="button" :disabled="busy" @click="emit('clearCache')">清理代理缓存</button>
      <button class="button secondary" type="submit" data-action="test" :disabled="busy" title="校验当前填写的设置，不保存、不应用">校验配置</button>
      <button class="button secondary" type="submit" :disabled="busy || !hasChanges">
        {{ busy ? "处理中…" : "保存为草稿" }}
      </button>
      <button
        v-if="hasChanges || dirty"
        class="button primary"
        type="submit"
        data-action="apply"
        :disabled="busy"
        title="保存当前设置并应用全部草稿配置"
      >
        保存并应用
      </button>
    </div></div>
  </form>
</template>

<style scoped>
.settings-card { min-width: 0; }
.settings-tabs { justify-content: flex-start; padding-inline: 16px; border-radius: var(--radius) var(--radius) 0 0; }
.config-tools { display: flex; flex: none; gap: 2px; padding-inline: 8px; border-inline-start: 1px solid var(--line); }
.config-tools button { display: inline-flex; align-items: center; justify-content: center; gap: 5px; min-width: 32px; height: 32px; padding: 0 7px; border: 0; border-radius: 6px; background: transparent; color: var(--text-muted); font: inherit; font-size: 13px; white-space: nowrap; cursor: pointer; }
.config-tools button:hover, .config-tools button:focus-visible { color: var(--accent); background: var(--accent-soft); }
.config-tools button:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
.settings-card .field > label:not(.checkbox-row) { justify-content: flex-end; text-align: right; color: var(--text); font-weight: 600; }
.settings-card .checkbox-row { pointer-events: none; }
.settings-card .checkbox-row > * { pointer-events: auto; cursor: pointer; }
@media (max-width: 760px) {
  .settings-actions-shell { position: static; padding-bottom: 0; }
  .settings-card .field { display: grid; grid-template-columns: min(40%, 145px) minmax(0, 1fr); align-items: center; gap: 6px 8px; }
  .settings-card .field > label:not(.checkbox-row) { grid-column: 1; }
  .settings-card .field > :not(label), .settings-card .field > .checkbox-row { grid-column: 2; min-width: 0; }
}
.settings-note { margin: 16px 17px 0; color: var(--text-muted); }
.menu-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px 36px; }
.menu-option { display: flex; align-items: center; gap: 8px; }
.menu-option input { width: 16px; height: 16px; margin: 0; accent-color: var(--accent); }
.menu-option label { font-weight: 600; cursor: pointer; }
@media (max-width: 600px) { .menu-options { grid-template-columns: 1fr; } }
/* Override the legacy global link colors only for the settings repository mark. */
.settings-stack .app-update-github {
  color: var(--text);
}
.settings-app-info { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
.settings-app-version { color: var(--text-muted); font-size: 13px; white-space: nowrap; }
</style>
