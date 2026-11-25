<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { PhX } from "@phosphor-icons/vue";
import type { DraftChanges } from "../types";
const props = defineProps<{ changes: DraftChanges; returnFocus?: HTMLElement | null }>();
const emit = defineEmits<{ close: [] }>();
const dialog = ref<HTMLDialogElement | null>(null);
let returnFocus: HTMLElement | null = null;
onMounted(() => {
  returnFocus = props.returnFocus ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
  dialog.value?.showModal();
});
onBeforeUnmount(() => { dialog.value?.close(); returnFocus?.focus(); });
const sections: Record<string, string> = { rules: "HTTP(S) 代理", stream_rules: "TCP/UDP 代理", gateway_routes: "飞牛网关", rule_groups: "代理分组", upstream_pools: "转发服务组", rate_limit_policies: "限流策略", auth_profiles: "访问认证", waf_instances:"WAF 防火墙", custom_configs: "自定义配置", settings: "全局设置" };
const operations = { added: "新增", deleted: "删除", modified: "修改" };
const totals = computed(() => (["added", "deleted", "modified"] as const).map(operation => ({ operation, count: props.changes.changes.filter(change => change.operation === operation).length })));
const labels: Record<string, string> = {
 compatibility: "路径适配", tls_fingerprint: "信任证书指纹", tls_trust_target: "证书信任目标",
  authentication: "访问认证", forward_authorization: "转发认证信息", client_max_body_size: "请求体大小上限", client_max_body_mb: "请求体大小上限（MB）", connect_timeout_seconds: "连接超时（秒）", read_timeout_seconds: "读取超时（秒）", send_timeout_seconds: "发送超时（秒）", proxy_timeout_seconds: "代理超时（秒）", preserve_host: "保留原始 Host", cache: "缓存", inactive_minutes: "闲置清理时间（分钟）", key: "缓存键", keys_zone_mb: "缓存索引大小（MB）", max_size_mb: "缓存大小上限（MB）", slice_kb: "分片大小（KB）", use_stale: "使用过期缓存", valid_seconds: "有效时间（秒）", download_kbps: "下载限速（KB/s）", no_delay: "突发请求不延迟", basic_auth: "基础认证", basic_auth_file: "认证文件", basic_auth_realm: "认证提示", dav: "WebDAV", create_full_put_path: "自动创建目录", methods: "允许的方法", max_connections: "连接数上限", max_fails: "失败次数上限", fail_timeout_seconds: "失败超时（秒）", verify_upstream_tls: "验证目标 TLS 证书", accept_proxy_protocol: "接收 PROXY 协议", proxy_protocol: "发送 PROXY 协议", worker_rlimit_nofile: "文件描述符上限", logging: "日志设置", access_enabled: "记录访问日志", access_buffer_kb: "日志缓冲大小（KB）", access_flush_seconds: "日志刷写间隔（秒）", custom_format: "自定义日志格式", error_level: "错误日志级别", rotate_keep: "日志保留份数", rotate_size_mb: "日志轮转大小（MB）", real_ip: "真实客户端 IP", header: "请求头", trusted_proxies: "受信任代理", recursive: "递归解析", gzip: "Gzip 压缩", gunzip: "Gzip 解压", min_length: "最小长度", types: "内容类型", http: "HTTP 设置", keepalive: "保持连接", keepalive_requests: "单连接请求上限", keepalive_time_seconds: "连接存活上限（秒）", keepalive_timeout_seconds: "空闲连接超时（秒）", multi_accept: "批量接收连接", listen_type: "监听方式", protocols: "TLS 协议版本", ciphers: "加密套件", client_ca_file: "客户端 CA 文件", client_verify: "客户端证书验证", client_verify_depth: "客户端证书验证深度", session_cache_mb: "会话缓存大小（MB）", session_timeout_minutes: "会话超时（分钟）", ocsp_stapling: "OCSP 装订", sni_routes: "SNI 分流", udp_responses: "UDP 响应数量", request_headers: "请求头设置", response_headers: "响应头设置", return_code: "返回状态码", return_target: "返回内容或地址", rewrites: "地址重写", pattern: "匹配表达式", replacement: "替换内容", flag: "重写方式", static: "静态服务", static_alias: "目录映射方式", autoindex: "目录浏览", index_files: "首页文件", try_files: "文件查找顺序", allow: "允许访问", deny: "拒绝访问", valid_referers: "允许的来源", deny_invalid_referer: "阻止无效来源", expires: "过期时间", sub_filters: "内容替换", sub_filter_once: "仅替换一次", sub_filter_types: "替换内容类型", ssi: "SSI", secure_link: "安全链接", secret: "密钥", mirror: "请求镜像", mirror_request_body: "镜像请求体", addition_before: "响应前插入", addition_after: "响应后插入", file_aio: "异步文件读取", thread_pool_queue: "线程池队列上限", thread_pool_threads: "线程池线程数", min_delete_depth: "最小删除层级", group_id: "所属分组", inherit_fields: "继承设置", profile_id: "认证策略", username: "用户名", realm: "认证提示", mode: "模式", geos: "IP 区域映射", maps: "变量映射", splits: "流量分配", variable: "变量", source: "来源变量", default: "默认值", entries: "映射条目", value: "值", match: "匹配条件", hostnames: "按主机名匹配", server_names: "服务域名", search: "查找内容", argument: "参数", hash_key: "哈希依据", action: "动作", always: "始终添加", bypass: "绕过条件",
  upstream_path: "上游路径", proxy_pass_trailing_slash: "转发地址末尾添加 /", name: "名称", enabled: "启用状态", target: "目标地址", path: "路径", order: "排列顺序", domains: "域名", listen_port: "监听端口", listen_address: "监听地址", upstream_scheme: "目标协议", upstream_host: "目标主机", upstream_port: "目标端口", upstream_pool_id: "转发服务组", certificate_id: "证书", tls: "HTTPS", http2: "HTTP/2", websocket: "WebSocket", streaming: "流式响应", settings: "设置", root_location: "根路径", locations: "路径规则", servers: "目标服务器", host: "主机", port: "端口", weight: "权重", down: "停用", backup: "备用", strategy: "调度策略", users: "认证用户", password_hash: "密码", auth_profile_id: "访问认证", content: "配置内容", protocol: "协议", default_http_port: "默认 HTTP 端口", default_https_port: "默认 HTTPS 端口", worker_connections: "工作进程连接数", worker_processes: "工作进程数", revision_limit: "历史保留数量", access_log: "访问日志", access_log_enabled: "访问日志", rate_limit: "限流", rate_limit_policy_id: "限流策略", connections: "并发连接", requests_per_second: "每秒请求数", burst: "突发请求数", backend_type: "服务类型", static_path: "静态目录", redirect_to_https: "跳转 HTTPS", redirect_https_port: "HTTPS 跳转端口", routing: "路由设置", proxy_connect_timeout: "连接超时", proxy_read_timeout: "读取超时", proxy_send_timeout: "发送超时", tls_mode: "TLS 模式",
};
function fieldLabel(path: string) {
  return path.split(".").map(part => {
    const match = /^(.*?)(\[\d+\])?$/.exec(part);
    const key = match?.[1] || part;
    return `${labels[key] || key}${match?.[2] || ""}`;
  }).join(" / ");
}
</script>

<template>
  <dialog ref="dialog" class="modal draft-changes-dialog" aria-labelledby="draft-changes-title" @cancel.prevent="emit('close')">
    <header class="modal-header">
      <div><h2 id="draft-changes-title">当前草稿变更</h2><p>与上次成功生效的配置对比，仅展示变化项</p></div>
      <button class="icon-button modal-close" aria-label="关闭草稿变更" autofocus @click="emit('close')"><PhX :size="20" /></button>
    </header>
    <div class="modal-body">
      <p v-if="changes.status === 'synced'" class="empty-state">当前配置已同步，没有待应用的修改。</p>
      <p v-else-if="changes.status === 'unknown'" class="notice warning">无法确认上次生效的配置，暂时不能计算差异。请检查生效记录；当前运行配置未被修改。</p>
      <p v-else-if="changes.status === 'uninitialized'" class="empty-state">尚无成功应用记录，暂时没有可对比的生效配置。</p>
      <template v-else>
        <div class="change-totals"><span v-for="total in totals" :key="total.operation">{{ operations[total.operation] }} <strong>{{ total.count }}</strong> 项</span></div>
        <article v-for="(change, index) in changes.changes" :key="index" class="change-card">
          <header><div><span class="section-name">{{ sections[change.section] || change.section }}</span><h3>{{ change.name }}</h3></div><span class="badge" :class="change.operation === 'deleted' ? 'danger' : change.operation === 'added' ? 'success' : 'warning'">{{ operations[change.operation] }}</span></header>
          <div class="fields-table" role="table" :aria-label="`${change.name}的变更`">
          <div class="field-head" role="row"><span role="columnheader">配置项</span><span role="columnheader">已生效</span><span role="columnheader">当前草稿</span></div>
          <div v-for="field in change.fields" :key="field.path" class="field-change" role="row">
            <strong role="cell">{{ fieldLabel(field.path) }}</strong>
            <template v-if="!field.sensitive"><div role="cell" :aria-label="`已生效：${field.before}`"><span class="mobile-label">已生效：</span>{{ field.before }}</div><div role="cell" :aria-label="`当前草稿：${field.after}`"><span class="mobile-label">当前草稿：</span>{{ field.after }}</div></template>
            <p v-else class="sensitive-change" role="cell">内容已变更，具体值已隐藏</p>
          </div>
          </div>
        </article>
      </template>
    </div>
    <footer class="modal-footer"><span>查看变更不会保存或应用配置</span><button class="button ghost" @click="emit('close')">关闭</button></footer>
  </dialog>
</template>

<style scoped>
.draft-changes-dialog { position: fixed; inset: 0; margin: auto; padding: 0; width: min(900px, calc(100vw - 32px)); max-height: calc(100dvh - 48px); color: var(--text); }
.draft-changes-dialog:not([open]) { display: none; }
.draft-changes-dialog::backdrop { background: rgba(18,25,31,.58); }
.modal-header { padding: 12px 20px; }
h2 { margin: 0; font-size: 18px; }
.modal-header p, .section-name, .modal-footer, .mobile-label { color: var(--text-muted); font-size: 13px; }
.modal-header p { margin: 5px 0 0; }
.modal-body { padding: 16px 20px; overflow-y: auto; min-height: 0; }
.change-totals { display: flex; flex-wrap: wrap; gap: 20px; padding-bottom: 12px; }
.change-card { border: 1px solid var(--line); border-radius: 9px; padding: 12px; margin-bottom: 12px; overflow-wrap: anywhere; }
.change-card > header { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
h3 { margin: 4px 0 12px; font-size: 15px; }
.field-head, .field-change { display: grid; grid-template-columns: minmax(120px, 1fr) minmax(0, 1fr) minmax(0, 1fr); gap: 12px; padding: 10px 0; }
.field-head { color: var(--text-muted); font-size: 12px; }
.field-change { border-top: 1px solid var(--line); font-size: 13px; line-height: 1.6; }
.field-change > strong { font-weight: 500; }
.sensitive-change { grid-column: 2 / -1; margin: 0; color: var(--text-muted); }
.mobile-label { display: none; }
.modal-footer { padding: 10px 20px; justify-content: space-between; gap: 12px; }
.empty-state { padding: 24px 0; color: var(--text-muted); }
@media (max-width: 600px) { .field-head { display: none; } .field-change { grid-template-columns: 1fr; gap: 4px; } .mobile-label { display: inline; } .sensitive-change { grid-column: auto; } .modal-footer { flex-wrap: wrap; } }
</style>
