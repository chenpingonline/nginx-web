<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted, onBeforeUnmount } from "vue";
import { PhArrowClockwise, PhPlusCircle, PhArrowSquareOut } from "@phosphor-icons/vue";
import { request, jsonBody, errorMessage } from "../api";
import type { GatewayRoute } from "../types";
const props = defineProps<{ routes: GatewayRoute[]; busy: boolean }>();
const emit = defineEmits<{ refresh: [] }>();
const open = ref(false), saving = ref(false), error = ref(""), editing = ref("");
const actionError = ref("");
type DiscoveredApp = { app: string; entry: string; name: string; path: string; target: string; gateway_url?: string; note: string; existing: boolean };
const discovering = ref(false), discoveryError = ref(""), search = ref("");
const discovery = ref<{ entries: DiscoveredApp[]; unreadable: number }>({ entries: [], unreadable: 0 });
const matches = (...values: string[]) => values.join(" ").toLowerCase().includes(search.value.trim().toLowerCase());
const visibleRoutes = computed(() => props.routes.filter(r => matches(r.name, r.path, r.target)));
const discoveredApps = computed(() => discovery.value.entries.filter(item =>
  !item.existing && matches(item.name, item.app, item.path, item.target, item.gateway_url || "")));
let discoveryController: AbortController | undefined;
async function discover() {
  discoveryController?.abort();
  const controller = new AbortController();
  discoveryController = controller;
  discovering.value = true;
  discoveryError.value = "";
  try {
    const result = await request<typeof discovery.value>("/gateway-apps", {signal: controller.signal});
    if (!controller.signal.aborted) discovery.value = result;
  } catch (e) { if (!controller.signal.aborted) discoveryError.value = errorMessage(e); }
  finally { if (!controller.signal.aborted) discovering.value = false; }
}
onMounted(discover);
onBeforeUnmount(() => discoveryController?.abort());
function refresh() { emit("refresh"); void discover(); }
function useDiscovered(item: DiscoveredApp) {
  show({ ...blank(), name: item.name, path: item.path, target: item.target });
}
function close() { if (!saving.value) { cancelProbe(); open.value = false; error.value = ""; } }
const blank = (): GatewayRoute => ({ id: "", name: "", path: "", target: "http://127.0.0.1:8080", enabled: true, tls_fingerprint: "", tls_trust_target: "", compatibility: "subpath" });
const form = reactive(blank());
const compatibilityMode = computed<"" | "subpath">({
  get: () => form.compatibility === "ikuai-v4" || form.compatibility === "subpath" ? "subpath" : "",
  set: value => { form.compatibility = value; },
});
type ProbeResult = { code: string; message: string; target: string; http_status?: number; can_trust: boolean; certificate?: { subject: string; issuer: string; fingerprint: string; not_before: string; not_after: string; names: string[] } };
const probing = ref(false), probeResult = ref<ProbeResult | null>(null);
let probeGeneration = 0, probeController: AbortController | undefined;
function cancelProbe() { probeGeneration++; probeController?.abort(); probing.value = false; }
watch(() => form.target, target => {
  cancelProbe(); probeResult.value = null;
  if (form.tls_trust_target !== target.trim().replace(/\/+$/, "")) { form.tls_fingerprint = ""; form.tls_trust_target = ""; }
});
onBeforeUnmount(cancelProbe);
async function probe() {
  cancelProbe(); const generation = probeGeneration;
  probeController = new AbortController(); probing.value = true;
  try {
    const result = await request<ProbeResult>("/gateway-probe", { method: "POST", body: jsonBody(form), signal: probeController.signal });
    if (generation === probeGeneration) probeResult.value = result;
  } catch (e) {
    if (generation === probeGeneration) probeResult.value = { code: "request_failed", message: errorMessage(e), target: form.target, can_trust: false };
  } finally { if (generation === probeGeneration) probing.value = false; }
}
function trustCertificate() {
  const result = probeResult.value;
  if (!result?.can_trust || !result.certificate || result.target !== form.target.trim().replace(/\/+$/, "")) return;
  if (!confirm(`信任 ${result.target} 当前返回的证书？\nSHA-256：${result.certificate.fingerprint}\n仅对本条网关生效，以指纹校验替代证书颁发机构和名称校验；证书更换后将拒绝连接。保存后生效。`)) return;
  form.tls_fingerprint = result.certificate.fingerprint;
  form.tls_trust_target = result.target;
  void probe();
}
function clearTrust() { cancelProbe(); form.tls_fingerprint = ""; form.tls_trust_target = ""; probeResult.value = null; }
function certificateDate(value: string) { return new Date(value).toLocaleString("zh-CN"); }
const compatibilityLabel = (route: GatewayRoute) => route.compatibility ? "通用子路径" : "原生子路径";
const entry = (path: string) => `${location.origin}/app/nginx-web/proxy/${path}/`;
function show(route?: GatewayRoute) {
  cancelProbe();
  Object.assign(form, blank(), route ? { ...route, compatibility: route.compatibility || "" } : {});
  probeResult.value = null;
  editing.value = route?.id ?? "";
  actionError.value = "";
  error.value = "";
  open.value = true;
}
async function save() {
  if (probing.value) return;
  cancelProbe();
  saving.value = true; error.value = "";
  try {
    await request(`/gateway-routes${editing.value ? `/${editing.value}` : ""}`, { method: editing.value ? "PUT" : "POST", body: jsonBody(form) });
    open.value = false; refresh();
  } catch (e) { error.value = errorMessage(e); }
  finally { saving.value = false; }
}
async function remove(route: GatewayRoute) {
  if (!confirm(`将「${route.name}」从网关配置中删除？删除后立即停止转发。`)) return;
  saving.value = true; actionError.value = "";
  try { await request(`/gateway-routes/${route.id}`, {method: "DELETE"}); refresh(); }
  catch (e) { actionError.value = errorMessage(e); }
  finally { saving.value = false; }
}
</script>

<template>
  <div class="toolbar compact-rule-toolbar">
    <input v-model="search" class="input gateway-search" type="search" aria-label="搜索网关服务" placeholder="搜索服务名称、路径或端口" />
    <span class="spacer"></span>
    <button class="button ghost" :disabled="busy || saving || discovering" @click="refresh"><PhArrowClockwise :size="16" aria-hidden="true" />{{ discovering ? '识别中…' : '刷新' }}</button>
    <button class="button primary" :disabled="busy || saving" @click="show()"><PhPlusCircle :size="17" aria-hidden="true" />添加网关服务</button>
  </div>
  <p class="notice">将 NAS 和局域网设备的 HTTP(S) 管理页面接入飞牛网关，通过 fnConnect 统一访问，无需公网 IP 或额外开放端口。仅管理员可访问；速度受 fnConnect 套餐限制。</p>
  <p v-if="actionError || discoveryError" class="notice danger" role="alert">{{ actionError || discoveryError }}</p>
  <p v-if="discovery.unreadable" class="notice">{{ discovery.unreadable }} 个应用的入口声明无法读取，可手动添加服务。</p>
  <article class="card">
    <div v-if="visibleRoutes.length || discoveredApps.length" class="table-wrap">
      <table class="table"><thead><tr><th>服务名称</th><th>网关路径</th><th>转发地址</th><th>配置</th><th>操作</th></tr></thead>
        <tbody><tr v-for="route in visibleRoutes" :key="route.id">
          <td class="rule-name">{{ route.name }}</td>
          <td><a :href="entry(route.path)" target="_blank" rel="noopener noreferrer">/proxy/{{ route.path }}/ <PhArrowSquareOut :size="14" aria-hidden="true" /></a></td>
          <td>{{ route.target }}</td><td><span class="badge" :class="route.enabled ? 'success' : ''">{{ route.enabled ? '启用' : '停用' }}</span><div class="help-text">{{ compatibilityLabel(route) }}</div></td>
          <td><div class="table-actions"><button class="button ghost small" :disabled="busy || saving" @click="show(route)">编辑</button><button class="button danger-ghost small" :disabled="busy || saving" @click="remove(route)">删除</button></div></td>
        </tr>
        <tr v-for="item in discoveredApps" :key="item.app + ':' + item.entry">
          <td class="rule-name">{{ item.name }}</td>
          <td><a v-if="item.gateway_url" :href="item.gateway_url" target="_blank" rel="noopener noreferrer">{{ item.gateway_url }} <PhArrowSquareOut :size="14" aria-hidden="true" /></a><span v-else>/proxy/{{ item.path }}/</span></td>
          <td>{{ item.target || (item.gateway_url ? '—' : '端口待填写') }}</td>
          <td><span class="badge">{{ item.gateway_url ? '原生入口' : '待接入' }}</span></td>
          <td><div v-if="!item.gateway_url" class="table-actions"><button class="button ghost small" :disabled="busy || saving" @click="useDiscovered(item)">{{ item.target ? '接入' : '填写端口' }}</button></div></td>
        </tr></tbody>
      </table>
    </div>
    <div v-else class="empty-state"><h3>{{ search ? "没有匹配的服务" : discovering ? "正在识别已安装应用…" : "还没有网关服务" }}</h3><p v-if="!search && !discovering">为可信的 HTTP 服务添加路径，通过飞牛域名统一访问。</p></div>
  </article>
  <p class="notice">通用子路径模式适配资源、接口、跳转和本服务的浏览器会话。若页面能打开但登录或菜单异常，请先检测连接，再检查设备的访问基础路径设置。网关与飞牛共用域名，只接入可信设备。</p>
  <Teleport to="body">
    <div v-if="open" class="modal-backdrop" @mousedown.self="close" @keydown.esc.stop="close">
    <section class="modal policy-modal" role="dialog" aria-modal="true" aria-labelledby="gateway-title">
      <header class="modal-header"><div><h2 id="gateway-title">{{ editing ? '编辑网关服务' : '添加网关服务' }}</h2></div>
        <div class="rule-header-actions"><button class="button ghost" :disabled="saving" @click="close">取消</button><button class="button primary" type="submit" form="gateway-form" :disabled="saving || probing">{{ saving ? '保存中…' : '保存' }}</button></div>
        <button class="icon-button modal-close" aria-label="关闭" :disabled="saving" @click="close">×</button>
      </header>
      <div class="modal-body"><div v-if="error" class="notice danger" role="alert">{{ error }}</div>
        <form id="gateway-form" class="form-grid modal-form-grid" @submit.prevent="save">
          <div class="field full"><label for="gateway-name">服务名称</label><input id="gateway-name" v-model.trim="form.name" class="input" required maxlength="80" placeholder="例如：音乐服务" /></div>
          <div class="field full"><label for="gateway-path">服务路径</label><input id="gateway-path" v-model.trim="form.path" class="input" required pattern="[a-z0-9][a-z0-9-]{0,47}" maxlength="48" placeholder="例如：music" /><small class="field-help">使用小写字母、数字或连字符，首位不能为连字符。</small><small class="field-help" style="overflow-wrap:anywhere">{{ entry(form.path || 'music') }}</small></div>
          <div class="field full"><label for="gateway-target">转发地址</label><input id="gateway-target" v-model.trim="form.target" class="input" type="url" required placeholder="http://127.0.0.1:4533" /><small class="field-help">填写 NAS 本机或局域网内 HTTP(S) 地址，可带基础路径；HTTPS 默认校验证书；自签名证书可先检测，再单独信任当前证书。</small></div>
          <div class="field full"><label for="gateway-compatibility">页面适配</label><select id="gateway-compatibility" v-model="compatibilityMode" class="select"><option value="subpath">通用子路径兼容</option><option value="">原生子路径（服务已配置基础路径）</option></select><small class="field-help">不按设备品牌区分。通用模式处理根路径资源、同目标绝对地址、接口请求、页面跳转及 Cookie / 浏览器存储；不修改目标设备设置。</small></div>
          <p v-if="form.compatibility" class="notice full">Hash 路由保留原样。使用 History 路由、Service Worker、严格 CSP 或特殊登录机制的页面，仍可能需要在设备端设置基础路径；连接检测成功不代表所有页面功能已兼容。</p>
          <div class="full gateway-diagnostics">
            <div class="gateway-probe-actions"><button type="button" class="button secondary" :disabled="probing || saving" :aria-busy="probing" @click="probe">{{ probing ? '检测中…' : '检测连接' }}</button><span v-if="form.tls_fingerprint" class="badge success">已指定信任证书</span><button v-if="form.tls_fingerprint" type="button" class="button ghost small" :disabled="saving || probing" @click="clearTrust">清除证书信任</button></div>
            <p class="gateway-probe-status" role="status">{{ probing ? '正在从 NAS 检测目标连接…' : probeResult?.message || '检测不会保存配置，也不会发送 NAS 登录信息。' }}</p>
            <div v-if="form.tls_fingerprint" class="gateway-trust-summary"><span>信任指纹（SHA-256）</span><code>{{ form.tls_fingerprint }}</code><small>仅限此服务和当前目标地址，证书过期或更换后拒绝连接。</small></div>
            <section v-if="probeResult?.certificate" class="gateway-certificate" aria-label="目标证书">
              <dl><div><dt>证书名称</dt><dd>{{ probeResult.certificate.subject || '未提供' }}</dd></div><div><dt>颁发者</dt><dd>{{ probeResult.certificate.issuer || '未提供' }}</dd></div><div><dt>有效期</dt><dd>{{ certificateDate(probeResult.certificate.not_before) }} 至 {{ certificateDate(probeResult.certificate.not_after) }}</dd></div><div><dt>适用名称</dt><dd>{{ probeResult.certificate.names.join('、') || '未声明' }}</dd></div><div><dt>SHA-256 指纹</dt><dd><code>{{ probeResult.certificate.fingerprint }}</code></dd></div></dl>
              <button v-if="probeResult.can_trust && form.tls_fingerprint !== probeResult.certificate.fingerprint" type="button" class="button secondary" :disabled="saving || probing" @click="trustCertificate">信任此证书</button>
              <small v-if="probeResult.can_trust">信任前请核对该证书属于目标设备。保存后生效。</small>
            </section>
          </div>
          <div class="field full"><label><input v-model="form.enabled" type="checkbox" /> 启用此服务</label></div>
        </form>
      </div>
    </section>
  </div></Teleport>
</template>

<style scoped>
.gateway-search { width: min(100%, 300px); }
.gateway-probe-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.gateway-probe-status { min-height: 2.8em; margin: 8px 0; color: var(--text-muted); font-size: 13px; line-height: 1.4; }
.gateway-trust-summary, .gateway-certificate { border: 1px solid var(--line); border-radius: 9px; padding: 12px; margin-top: 8px; overflow-wrap: anywhere; }
.gateway-trust-summary { display: grid; gap: 6px; }
.gateway-certificate dl { margin: 0 0 10px; }
.gateway-certificate dl > div { display: grid; grid-template-columns: 110px minmax(0, 1fr); gap: 10px; padding: 5px 0; }
.gateway-certificate dt { color: var(--text-muted); }
.gateway-certificate dd { margin: 0; }
.gateway-certificate small { display: block; margin-top: 8px; }
code { overflow-wrap: anywhere; font-size: 12px; }
@media (max-width: 600px) { .gateway-certificate dl > div { grid-template-columns: 1fr; gap: 4px; } }
</style>
