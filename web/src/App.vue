<script setup lang="ts">
import { minListenPort } from "./permissionMode";
import type { ACMEInput } from "./types";
import ACMEJobs from "./components/ACMEJobs.vue";
import AppSelect from "./components/AppSelect.vue";
import RevisionRetentionForm from "./components/RevisionRetentionForm.vue";
import RevisionPreview from "./components/RevisionPreview.vue";
import DraftChangesDialog from "./components/DraftChangesDialog.vue";
import type { DraftChanges } from "./types";
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  watch,
} from "vue";
import {
  PhArrowClockwise,
  PhArrowUp,
  PhArrowDown,
  PhCopy,
  PhFileCode,
  PhCaretDown,
  PhCaretRight,
  PhArrowsLeftRight,
  PhCertificate,
  PhCheckCircle,
  PhList,
  PhPlusCircle,
  PhShareNetwork,
  PhX,
  PhXCircle,
} from "@phosphor-icons/vue";
import { errorMessage, jsonBody, request } from "./api";
import { highlightLog, searchLogLines } from "./logHighlight";
const fnosMode = !__STANDALONE__;
const appVersion = __APP_VERSION__;
defineProps<{ adminUsername?: string; loggingOut?: boolean }>();
const emit = defineEmits<{ logout: [] }>();
import RuleGroups from "./components/RuleGroups.vue";
import type { GatewayRoute, RuleGroup } from "./types";
import RuleForm from "./components/RuleForm.vue";
import RuleStatusSwitch from "./components/RuleStatusSwitch.vue";
import WAFPage from "./components/WAFPage.vue";
import NetworkTopology from "./components/NetworkTopology.vue";
import DashboardPage from "./components/DashboardPage.vue";
import ErrorDetailsPage from "./components/AnalysisPage.vue";
import CertificateForm from "./components/CertificateForm.vue";
import UpstreamPoolsPage from "./components/UpstreamPoolsPage.vue";
import RateLimitPoliciesPage from "./components/RateLimitPoliciesPage.vue";
import AuthProfilesPage from "./components/AuthProfilesPage.vue";
import RateLimitPolicyManager from "./components/RateLimitPolicyManager.vue";
import BackupPage, { type BackupFile } from "./components/BackupPage.vue";
import RuntimeSettingsForm from "./components/RuntimeSettingsForm.vue";
import SidebarIcon from "./components/SidebarIcon.vue";
import GatewayRoutesPage from "./components/GatewayRoutesPage.vue";
import ConfigDialog from "./components/ConfigDialog.vue";
import NameConflictNotice from "./components/NameConflictNotice.vue";
import SourceParameterDialog from "./components/SourceParameterDialog.vue";
import ProxyAddress from "./components/ProxyAddress.vue";
import HttpRuleTableHead from "./components/HttpRuleTableHead.vue";
import ConfigSources from "./components/ConfigSources.vue";
import ConfigTextEditor from "./components/ConfigTextEditor.vue";
import { UnsupportedFormError, replaceSourceRange, formSourceBlock, updateSourceBlock, sourceBlock, sourceRuleCandidate, ruleDirectives, sourceEntries,matchSource,type SourceEntry } from "./nginxConfig";
import StreamRulesPage from "./components/StreamRulesPage.vue";
import type {
  ApplyResult,
  AuthProfile,
  CertificateInput,
  CustomConfig,
  GeneratedConfig,
  LogResponse,
  LogType,
  NginxAction,
  Overview,
  Page,
  ProxyRule,
  ProxyRuleInput,
  RateLimitPolicy,
  RateLimitPolicyInput,
  Revision,
  Settings,
  State,
  StreamRule,
  StreamRuleInput,
  Toast,
  UpstreamPool,
  UpstreamPoolInput,
} from "./types";
import { matchSavedConfig, type ConfigForm } from "./configForm";
import { formatDate, formatHost, shortFingerprint } from "./utils";

type SidebarIconName =
  | "overview"
  | "http"
  | "stream"
  | "upstreams"
  | "auth"
  | "waf"
  | "rate-limit"
  | "certificate"
  | "logs"
  | "requests"
  | "backup"
  | "history"
  | "config"
  | "settings";
const pages: { id: Page; icon: SidebarIconName; label: string; subtitle: string; groupLabel?: string }[] = [
  {
    id: "dashboard",
    icon: "overview",
    label: "总览",
    subtitle: "查看代理服务与配置状态",
  },
  {
    id: "rules",
    icon: "http",
    label: "HTTP(S) 代理",
    subtitle: "管理域名、监听端口与转发服务",
    groupLabel: "代理配置",
  },
  {
    id: "streams",
    icon: "stream",
    label: "TCP/UDP 代理",
    subtitle: "管理四层转发、TLS 终止与 SNI 透传",
  },
  {
    id: "upstreams",
    icon: "upstreams",
    label: "转发服务组",
    subtitle: "管理负载均衡、节点健康参数与连接复用",
  },
  { id: "auth-profiles", icon: "auth", label: "认证策略", subtitle: "管理代理规则使用的认证用户", groupLabel: "安全与限流" },
  {
    id: "rate-limits",
    icon: "rate-limit",
    label: "限流策略",
    subtitle: "复用限流参数并保持规则额度独立",
  },
  {
    id: "certificates",
    icon: "certificate",
    label: "SSL/TLS 证书",
    subtitle: "管理证书导入、ACME 申请与自动续期",
  },
  { id: "waf", icon: "waf", label: "WAF 防火墙", subtitle: "管理站点防护实例与命中日志" },
  {
    id: "gateway",
    icon: "http",
    label: "飞牛网关",
    subtitle: "通过飞牛访问域名按路径转发 HTTP 服务",
    groupLabel: "网关",
  },
  {id:"topology",icon:"stream",label:"网络拓扑",subtitle:"查看代理流量与活动访客", groupLabel: "监控分析" },
  { id: "errors", icon: "requests", label: "请求分析", subtitle: "分析 HTTP / HTTPS 请求、性能与异常" },
  {
    id: "logs",
    icon: "logs",
    label: "运行日志",
    subtitle: "查看 Nginx 与管理服务日志",
  },
  { id: "backup", icon: "backup", label: "备份与恢复", groupLabel: "配置管理", subtitle: "导出配置备份并恢复为草稿" },
  {
    id: "revisions",
    icon: "history",
    label: "配置历史",
    subtitle: "恢复或清理已应用的配置版本",
  },
  {
    id: "config",
    icon: "config",
    label: "Nginx 配置",
    subtitle: "查看 nginx-web 实际生成的配置文件",
  },
  {
    id: "settings",
    icon: "settings",
    label: "全局设置",
    subtitle: "调整默认端口与历史保留策略",
  },
];
const platformPages = computed(() => pages.filter(item => item.id !== "gateway" || fnosMode));
const page = ref<Page>("dashboard"),
  overview = ref<Overview | null>(null),
  state = ref<State | null>(null),
  revisions = ref<Revision[]>([]),
  config = ref<GeneratedConfig | null>(null);
const menuEnabled = (id: Page) => !overview.value?.hidden_menus?.includes(id);
const visiblePages = computed(() => {
  let group = "", lastGroup = "";
  return platformPages.value.flatMap(item => {
    if (item.groupLabel) group = item.groupLabel;
    if (!menuEnabled(item.id)) return [];
    const groupLabel = group !== lastGroup ? group : undefined;
    lastGroup = group;
    return [{ ...item, groupLabel }];
  });
});
watch(visiblePages, items => {
  if (!items.some(item => item.id === page.value)) setPage(items[0]?.id ?? "settings");
}, { flush: "sync" });
const incompatiblePorts = computed(() => {
  const value = state.value;
  if (!value || minListenPort === 1) return [];
  const items: string[] = [];
  if (value.settings.default_http_port < minListenPort) items.push(`默认 HTTP 端口 ${value.settings.default_http_port}`);
  if (value.settings.default_https_port < minListenPort) items.push(`默认 HTTPS 端口 ${value.settings.default_https_port}`);
  for (const rule of value.rules ?? []) if (rule.listen_port < minListenPort) items.push(`HTTP 规则「${rule.name}」：${rule.listen_port}`);
  for (const rule of value.stream_rules ?? []) if (rule.listen_port < minListenPort) items.push(`${rule.protocol.toUpperCase()} 规则「${rule.name}」：${rule.listen_port}`);
  for (const group of value.rule_groups ?? []) if (group.listen_port < minListenPort) items.push(`分组「${group.name}」：${group.listen_port}`);
  return items;
});
const appliedConfigurationKnown = computed(() => overview.value?.applied_known ?? false);
const restoredRevision = computed(() => state.value?.dirty && state.value.draft_revision_id
  ? revisions.value.find(item => item.id === state.value!.draft_revision_id) : undefined);
const showDraftPreview = ref(false);
const draftChanges = ref<DraftChanges | null>(null);
const draftChangesLoading = ref(false);
const draftChangesTrigger = ref<HTMLButtonElement | null>(null);
async function openDraftChanges() {
  draftChangesLoading.value = true;
  try {
    draftChanges.value = await request<DraftChanges>("/draft-changes");
    showDraftPreview.value = true;
  } catch (error) { toast(errorMessage(error), "error"); }
  finally { draftChangesLoading.value = false; }
}
const errorScope = ref({ minutes: 60, rule: "" });
const ruleProtocol = ref<"all" | "http" | "https">("all");
const ruleEnabled = ref<"all" | "enabled" | "disabled">("all");
const loading = ref(true),
  busy = ref(false),
  connectionError = ref(""),
  menuOpen = ref(false),
  ruleSearch = ref(""),
  logType = ref<LogType>("access"),
  logLines = ref<string[]>([]),
  logsLoading = ref(false),
  configTab = ref("master");
const previewRevisionID = ref<string | null>(null);
const activePreview = computed(() => revisions.value.find(item => item.id === previewRevisionID.value) ?? null);
function closePreview() {
  showDraftPreview.value = false;
  previewRevisionID.value = null;
}
const logLineLimit = ref(500);
const logSearch = ref("");
const logWrap = ref(false);
const activeLogMatch = ref(0);
const highlightedLogLines = computed(() => logLines.value.map(highlightLog));
const searchedLogs = computed(() => searchLogLines(highlightedLogLines.value, logSearch.value));
async function revealLogMatch() {
  await nextTick();
  const view = logView.value;
  const target = view?.querySelector<HTMLElement>(`[data-log-match="${activeLogMatch.value}"]`);
  if (view && target) {
    view.scrollTop += target.getBoundingClientRect().top - view.getBoundingClientRect().top - view.clientHeight / 2;
  }
}
function jumpLogMatch(direction: number) {
  if (!searchedLogs.value.count) return;
  activeLogMatch.value = (activeLogMatch.value + direction + searchedLogs.value.count) % searchedLogs.value.count;
  void revealLogMatch();
}
watch(logSearch, (query) => {
  activeLogMatch.value = 0;
  if (query) {
    stopLogs();
    void revealLogMatch();
  } else if (page.value === "logs") void loadLogs();
});
const acmeJobsPanel = ref<InstanceType<typeof ACMEJobs> | null>(null);
const modal = ref<"rule" | "certificate" | null>(null),
  editingRule = ref<ProxyRule | null>(null),
  rateLimitManagerOpen = ref(false),
  editingRateLimitPolicy = ref<RateLimitPolicy>(),
  toasts = ref<Toast[]>([]),
  logView = ref<HTMLElement | null>(null),
  toastID = ref(0);
const confirmBox = reactive({
  open: false,
  title: "",
  message: "",
  resolve: null as ((value: boolean) => void) | null,
});
const currentPage = computed(
  () => pages.find((item) => item.id === page.value) ?? pages[0]!,
);
const selectedGroup = ref("all");
const groupPanel = ref<InstanceType<typeof RuleGroups>>();
const ruleGroups = computed(() => state.value?.rule_groups ?? []);
function updateAuthProfiles(profiles: AuthProfile[]) {
  if (state.value) {
    state.value.auth_profiles = profiles;
    state.value.dirty = true;
  }
}
async function removeAuthProfile(profile: AuthProfile) {
  if (!(await ask("删除认证策略", `确定删除“${profile.name}”吗？`))) return;
  const deleted = await mutate(() => request(`/auth-profiles/${profile.id}`, { method: "DELETE" }), "认证策略已删除");
  if (deleted) await loadCore(true);
}
async function saveRuleGroup(value: RuleGroup): Promise<{ group: RuleGroup; error?: string }> {
  if (busy.value) throw new Error("请等待当前操作完成");
  busy.value = true;
  let saved = false;
  try {
    const group = await saveObject<RuleGroup>("/rule-groups", value.id, value);
    saved = true;
    try {
      await applyDraft(`保存分组：${group.name}`, "分组已保存并应用");
      return { group };
    } catch (e) { return { group, error: `分组已保存，但应用失败：${errorMessage(e)}。修正后可再次保存并应用。` }; }
  } finally {
    if (saved) await loadCore(true);
    busy.value = false;
  }
}
async function deleteRuleGroup(id: string): Promise<boolean> {
  if (busy.value) return false;
  if (!(await ask("删除分组", "组内规则将移至未分组并保留当前访问配置。删除后会应用所有尚未应用的修改，是否继续？"))) return false;
  busy.value = true;
  let deleted = false;
  try {
    await request(`/rule-groups/${id}`, { method: "DELETE" });
    deleted = true;
    try {
      await applyDraft("删除代理分组", "分组已删除，规则配置已保留");
    } catch (e) { toast(`分组已删除，但应用失败：${errorMessage(e)}，请到“Nginx 配置”页面重试保存并应用。`, "error"); }
    return true;
  } finally {
    if (deleted) await loadCore(true);
    busy.value = false;
  }
}
const sources = computed(() => sourceEntries(state.value?.custom_configs ?? []));
const httpSources = computed(() => sources.value.filter(s=>s.file.name.endsWith(".conf")));
const filteredSources = computed(() => selectedGroup.value && selectedGroup.value !== "all" ? [] :
  httpSources.value.filter(s=>matchSource(s,ruleSearch.value,ruleProtocol.value,ruleEnabled.value)));
const importing = ref(false);
const streamEditor = ref<InstanceType<typeof StreamRulesPage>>();
const upstreamEditor = ref<InstanceType<typeof UpstreamPoolsPage>>();
const policyEditor = ref<InstanceType<typeof RateLimitPoliciesPage>>();
const wafEditor = ref<InstanceType<typeof WAFPage>>();
const authEditor = ref<InstanceType<typeof AuthProfilesPage>>();
const settingsEditor = ref<InstanceType<typeof RuntimeSettingsForm>>();

const importedCreate = ref<{path: string; id: string}>();
async function saveObject<T>(path: string, id: string, value: object): Promise<T> {
  if (id && importedCreate.value?.id === id && importedCreate.value.path === path) {
    const kind = path === "/rate-limit-policies" ? "rate-limits" : path.slice(1);
    const saved = await request<T>("/config-form/create", { method: "POST", body: jsonBody({kind, id, value: {...value, id}}) });
    importedCreate.value = undefined;
    return saved;
  }
  return request<T>(id ? path + "/" + id : path, {method: id ? "PUT" : "POST", body: jsonBody(value)});
}
async function openImportedForm(form: ConfigForm) {
  if (!state.value || !menuEnabled(form.kind === "rule-groups" ? "rules" : form.kind)) throw new Error("请先启用对应菜单");
  const collections = { rules: state.value.rules, streams: state.value.stream_rules, upstreams: state.value.upstream_pools,
    "rate-limits": state.value.rate_limit_policies, "rule-groups": state.value.rule_groups ?? [] };
  importedCreate.value = undefined;
  if (form.kind in collections && !collections[form.kind as keyof typeof collections].some(row => row.id === form.id))
    importedCreate.value = {path: form.kind === "rate-limits" ? "/rate-limit-policies" : "/" + form.kind, id: form.id};
  importing.value = false;
  setPage(form.kind === "rule-groups" ? "rules" : form.kind);
  await nextTick();
  switch (form.kind) {
    case "rule-groups": groupPanel.value?.show(form.value); break;
    case "rules": openRule(form.value); break;
    case "streams": streamEditor.value?.show(form.value); break;
    case "upstreams": upstreamEditor.value?.show(form.value); break;
    case "rate-limits": policyEditor.value?.show(form.value); break;
    case "waf": await wafEditor.value?.edit(form.value); break;
    case "auth-profiles": authEditor.value?.show(form.value); break;
    case "settings": settingsEditor.value?.restore(form.value); break;
  }
}
async function previewForm(kind: ConfigForm["kind"], id: string, name: string) {
  try {
    const data = await request<{content: string}>(`/config-form?kind=${kind}&id=${encodeURIComponent(id)}`);
    rulePreview.value = { ...data, name, draft: !!state.value?.dirty };
  } catch (e) { toast(errorMessage(e), "error"); }
}
const rulePreview = ref<{content:string;name:string;draft:boolean}>();
async function showConfig(id:string,name:string,stream=false) {
  await previewForm(stream ? "streams" : "rules", id, name);
}
async function sourceSaved() {
  toast("自定义配置已保存为草稿，请校验后应用", "success");
  await loadCore(true);
}
const configEditor = ref<InstanceType<typeof ConfigTextEditor>>();
const sourceEditorEntry = ref<SourceEntry>();
const sourceFormIssue = ref("");
async function finishSourceEdit(apply: boolean) {
  const entry = sourceEditorEntry.value;
  if (modal.value === "rule") modal.value = null;
  sourceEditorEntry.value = undefined;
  if (apply && entry) {
    await mutate(() => applyDraft("保存并应用自定义配置：" + entry.file.name, "自定义配置已保存并应用"));
    await loadCore(true);
  } else {
    await sourceSaved();
  }
}
async function keepSource(source: string, stream: boolean, line?: number) {
  importing.value = false;
  openCustomConfig();
  if (stream) customConfigForm.name = "custom-config.stream";
  customConfigForm.content = source;
  if (line !== undefined) {
    await nextTick();
    configEditor.value?.focusLine(line);
  }
}
async function imported() {importing.value=false;await loadCore(true);toast("导入已保存为草稿","success");}
const ruleBackends = computed(() => {
  const pools = new Map(state.value?.upstream_pools.map(pool => [pool.id, pool.name]));
  return new Map((state.value?.rules ?? []).map(rule => [rule.id, ruleBackend(rule, pools)]));
});
const filteredRules = computed(() => {
  const rules = state.value?.rules ?? [],
    term = ruleSearch.value.trim().toLowerCase();
  return rules.filter(
    (rule) =>
      (selectedGroup.value === "all" || (rule.group_id || "") === selectedGroup.value) &&
      (ruleProtocol.value === "all" || (rule.tls ? "https" : "http") === ruleProtocol.value) &&
      (ruleEnabled.value === "all" || rule.enabled === (ruleEnabled.value === "enabled")) &&
      (!term ||
      [
        rule.name,
        ...(rule.domains ?? []).map(domain => `${domain}:${rule.listen_port}`),
        String(rule.listen_port),
        ruleBackends.value.get(rule.id)?.label,
      ]
        .join(" ")
        .toLowerCase()
        .includes(term)),
  );
});
const collapsedGroups = ref(new Set<string>());
function toggleGroupExpanded(id: string) {
  if (collapsedGroups.value.has(id)) collapsedGroups.value.delete(id);
  else collapsedGroups.value.add(id);
}
const groupedRuleSections = computed(() => {
  const groups: { id: string; name: string; group?: RuleGroup; rules: ProxyRule[] }[] = ruleGroups.value.map(group => ({ id: group.id, name: group.name, group, rules: filteredRules.value.filter(rule => rule.group_id === group.id) }));
  const ungrouped = filteredRules.value.filter(rule => !rule.group_id);
  if (ungrouped.length || !groups.length || selectedGroup.value === '') groups.push({ id: '', name: '未分组', rules: ungrouped });
  const filtering = Boolean(ruleSearch.value || ruleProtocol.value !== 'all' || ruleEnabled.value !== 'all');
  return groups.filter(group => (selectedGroup.value === 'all' || selectedGroup.value === group.id) && (!filtering || group.rules.length));
});
function groupDescription(group: RuleGroup) {
  const family = group.listen_type === 'dual' ? 'IPv4 + IPv6' : group.listen_type === 'ipv6' ? 'IPv6' : 'IPv4';
  const cert = state.value?.certificates.find(cert => cert.id === group.certificate_id)?.name || '未选择证书';
  return `${group.tls ? 'HTTPS' : 'HTTP'} · ${group.listen_port} · ${family}${group.tls ? ` · ${cert} · HTTP/2 ${group.http2 ? '开启' : '关闭'}` : ''}`;
}
const newRuleGroup = ref('');
function addRuleToGroup(id: string) { openRule(); newRuleGroup.value = id; }
const hasRuleFilters = computed(() => Boolean(selectedGroup.value !== "all" || ruleSearch.value || ruleProtocol.value !== "all" || ruleEnabled.value !== "all"));
function resetRuleFilters() {
  selectedGroup.value = "all";
  ruleSearch.value = "";
  ruleProtocol.value = "all";
  ruleEnabled.value = "all";
}
function formatRuleEntry(rule: ProxyRule, domain = rule.domains[0]): string {
  if (!domain) return `未配置域名:${rule.listen_port}`;
  if (domain === "*") return `所有域名:${rule.listen_port}`;
  return `${rule.tls ? "https" : "http"}://${formatHost(domain)}:${rule.listen_port}`;
}
function canOpenRuleEntry(domain?: string): boolean {
  return Boolean(domain && !domain.includes("*"));
}
function ruleBackend(rule: ProxyRule, pools: Map<string, string>): {label: string; address?: string} {
  const loc = rule.root_location;
  if (rule.redirect_to_https) return {label: `HTTPS 跳转 · ${rule.redirect_https_port || 443}`};
  if (loc.backend_type === "static") return {label: `静态文件 · ${loc.static_path}`};
  if (loc.backend_type === "return") return {label: `返回 ${loc.return_code} ${loc.return_target || ""}`.trimEnd()};
  if (loc.backend_type === "status") return {label: "Nginx 状态"};
  if (loc.upstream_pool_id) return {label: pools.get(loc.upstream_pool_id) || "转发服务组"};
  const target = `${formatHost(loc.upstream_host)}:${loc.upstream_port}`;
  if (loc.backend_type !== "proxy") return {label: `${loc.backend_type.toUpperCase()} · ${target}`};
  const address = `${loc.upstream_scheme}://${target}${loc.upstream_path || ""}`;
  return {label: address, address};
}
async function copyText(value: string) {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const previous = document.activeElement as HTMLElement | null;
    const input = document.createElement("textarea");
    input.value = value;
    input.style.cssText = "position:fixed;left:-9999px;top:0";
    // A modal dialog makes the rest of the page inert, including textareas.
    const container = previous?.closest("dialog[open]") ?? document.querySelector("dialog:modal") ?? document.body;
    container.appendChild(input);
    try {
      input.focus({ preventScroll: true });
      input.select();
      if (!document.execCommand("copy")) throw new Error("copy failed");
    } finally { input.remove(); previous?.focus(); }
  }
}
async function copyProxyAddress(value: string, label: string) {
  try {
    await copyText(value);
    toast(`${label}已复制`, "success");
  } catch { toast(`浏览器不允许复制，请手动选择${label}`, "error"); }
}
const configKeys = computed(() => [
  "master",
  ...new Set([...Object.keys(config.value?.files ?? {}), ...(state.value?.custom_configs ?? []).map(item => item.name)]),
]);
const customConfigNames = computed(() => new Set((state.value?.custom_configs ?? []).map(item => item.name)));
const selectedCustomConfig = computed(() => state.value?.custom_configs?.find(item => item.name === configTab.value));
const configFileGroups = computed(() => {
  const categories = [
    { id: 'system', name: '系统配置', files: [] as { key: string; label: string }[] },
    { id: 'http', name: 'HTTP(S) 代理', files: [] as { key: string; label: string }[] },
    { id: 'stream', name: 'TCP/UDP 代理', files: [] as { key: string; label: string }[] },
    { id: 'custom', name: '自定义配置', files: [] as { key: string; label: string }[] },
  ];
  for (const key of configKeys.value.filter(key => key !== 'master')) {
    const match = key.match(/^\d+-(\d+)-([a-f0-9]+)\.(conf|stream)$/i);
    const kind = customConfigNames.value.has(key) || key.startsWith('custom-') ? 'custom' : match ? match[3] === 'stream' ? 'stream' : 'http' : 'system';
    let label = key;
    if (match) {
      const candidates = kind === 'stream' ? state.value?.stream_rules : state.value?.rules;
      const rule = candidates?.find(rule => rule.id === match[2]);
      label = rule ? `${rule.name} · ${Number(match[1])}` : key;
    } else if (key === '000-monitoring.conf') label = '监控配置';
    else if (key === '001-runtime-and-upstreams.conf') label = 'HTTP 公共配置与转发服务组';
    else if (key === '001-stream-runtime.stream') label = 'TCP/UDP 公共配置与转发服务组';
    else {
      const fallback = key.match(/^000-default-(\d+)(-ipv6)?\.conf$/);
      if (fallback) label = `默认站点 · ${Number(fallback[1])} · ${fallback[2] ? 'IPv6' : 'IPv4'}`;
    }
    categories.find(category => category.id === kind)!.files.push({ key, label });
  }
  return categories.filter(category => category.files.length);
});
const configText = computed(() =>
  selectedCustomConfig.value?.content ?? (configTab.value === "master"
    ? (config.value?.master ?? "")
    : (config.value?.files[configTab.value] ?? "")),
);
const configCopied = ref(false);
let configCopiedTimer: number | undefined;
const parameterEntry = ref<{file: CustomConfig; title: string; start?: number}>();
const customConfigOpen = ref(false);
const customConfigOriginal = ref("");
const customConfigForm = reactive<CustomConfig>({ name: "custom-config.conf", content: "server {\n    listen 8080;\n    server_name example.com;\n    return 200;\n}\n" });
function openCustomConfig(config?: CustomConfig) {
  customConfigOriginal.value = config?.name ?? "";
  customConfigForm.name = config?.name ?? "custom-config.conf";
  customConfigForm.content = config?.content ?? "server {\n    listen 8080;\n    server_name example.com;\n    return 200;\n}\n";
  customConfigOpen.value = true;
}
async function saveCustomConfig() {
  const original = customConfigOriginal.value;
  const saved = await mutate(() => request<CustomConfig>(original ? `/custom-configs/${encodeURIComponent(original)}` : "/custom-configs", { method: original ? "PUT" : "POST", body: jsonBody(customConfigForm) }));
  if (!saved) return;
  customConfigOpen.value = false;
  configTab.value = saved.name;
  toast("自定义配置已保存为草稿，请校验后应用", "success");
  await loadCore(true);
}
async function deleteCustomConfig(config: CustomConfig) {
  if (!(await ask("删除自定义配置", `确定删除“${config.name}”吗？删除将保存为草稿，应用后才会从 Nginx 配置中移除。`))) return;
  const deleted = await mutate(() => request(`/custom-configs/${encodeURIComponent(config.name)}`, { method: "DELETE" }));
  if (deleted === undefined) return;
  configTab.value = "master";
  toast("自定义配置已删除，等待应用", "success");
  await loadCore(true);
}


function toast(message: string, type: Toast["type"] = "") {
  const id = ++toastID.value;
  toasts.value.push({ id, message, type });
  window.setTimeout(
    () => (toasts.value = toasts.value.filter((item) => item.id !== id)),
    5200,
  );
}
function setPage(value: Page) {
  if (value === "errors") errorScope.value = { ...errorScope.value, rule: "" };
  if (!menuEnabled(value)) return;
  page.value = value;
  menuOpen.value = false;
}
function openErrorDetails(scope: { minutes: number; rule: string }) {
  setPage("errors");
  errorScope.value = scope;
  document.querySelector(".workspace")?.scrollTo({ top: 0 });
}
function updateGatewayRoute(updated: GatewayRoute) {
  if (state.value?.gateway_routes) {
    state.value.gateway_routes = state.value.gateway_routes.map(route => route.id === updated.id ? updated : route);
  }
}

async function loadCore(quiet = false) {
  if (!quiet) {
    loading.value = true;
    busy.value = true;
  }
  try {
    const [o, s] = await Promise.all([
      request<Overview>("/overview"),
      request<State>("/state"),

    ]);
    overview.value = o;
    state.value = s;
    revisions.value = menuEnabled("revisions") ? await request<Revision[]>("/revisions") ?? [] : [];
    connectionError.value = "";
  } catch (error) {
    connectionError.value = errorMessage(error);
    if (quiet) toast(connectionError.value, "error");
  } finally {
    loading.value = false;
    busy.value = false;
  }
}
let logTimer: ReturnType<typeof setTimeout> | undefined;
let logController: AbortController | undefined;
let logGeneration = 0;
function stopLogs() {
  clearTimeout(logTimer);
  logController?.abort();
  logGeneration++;
  logsLoading.value = false;
}
async function loadLogs() {
  clearTimeout(logTimer);
  logController?.abort();
  const generation = ++logGeneration;
  logController = new AbortController();
  logsLoading.value = true;
  try {
    const result = await request<LogResponse>(
      `/logs?type=${encodeURIComponent(logType.value)}&lines=${logLineLimit.value}`,
      { signal: logController.signal },
    );
    if (generation !== logGeneration) return;
    logLines.value = result.lines ?? [];
  } catch (error) {
    if (generation !== logGeneration) return;
    logLines.value = [`读取失败：${errorMessage(error)}`];
  } finally {
    if (generation === logGeneration) {
      logsLoading.value = false;
      await nextTick();
      if (logSearch.value) {
        activeLogMatch.value = Math.min(activeLogMatch.value, Math.max(0, searchedLogs.value.count - 1));
        void revealLogMatch();
      } else if (logView.value) logView.value.scrollTop = logView.value.scrollHeight;
      if (page.value === "logs" && !logSearch.value) logTimer = setTimeout(() => void loadLogs(), 3000);
    }
  }
}
function changeLogScope() {
  activeLogMatch.value = 0;
  logLines.value = [];
  void loadLogs();
}
async function openDashboardRule(id: string, sourceFile?: string) {
  await loadCore(true);
  if (connectionError.value) return;
  if (sourceFile) {
    const file = state.value?.custom_configs?.find(item => item.name === sourceFile);
    if (file) openCustomConfig(file);
    else toast("该源文件已不存在，请刷新列表", "error");
    return;
  }
  const rule = state.value?.rules.find(item => item.id === id);
  if (rule) openRule(rule);
  else toast("该规则已不存在，请刷新列表", "error");
}
async function loadConfig() {
  try {
    config.value = await request<GeneratedConfig>("/config");
    if (!configKeys.value.includes(configTab.value)) configTab.value = "master";
  } catch (error) {
    toast(errorMessage(error), "error");
  }
}
async function mutate<T>(fn: () => Promise<T>, success?: string) {
  if (busy.value) return;
  busy.value = true;
  try {
    const result = await fn();
    if (success) toast(success, "success");
    return result;
  } catch (error) {
    toast(errorMessage(error), "error");
    return undefined;
  } finally {
    busy.value = false;
  }
}
const conflictNotice = ref<ApplyResult>();
function reportActivation(result: ApplyResult, message?: string) {
  conflictNotice.value = result.warnings?.length ? result : undefined;
  if (!conflictNotice.value) toast(message || result.message, "success");
}
async function applyDraft(summary: string, message?: string) {
  const result = await request<ApplyResult>("/apply", { method: "POST", body: jsonBody({ summary }) });
  reportActivation(result, message);
  return result;
}
async function runNginxAction(action: NginxAction) {
  const labels = { start: "启动", stop: "停止", reload: "重载", test: "校验" };
  const result = await mutate(() =>
    request<ApplyResult>(`/nginx/${action}`, { method: "POST", body: "{}" }),
  );
  if (result) {
    if (action === "reload" || action === "start") reportActivation(result);
    else toast(result.message || `${labels[action]}成功`, "success");
    await loadCore(true);
  }
}
async function applyConfiguration() {
  if (await mutate(() => applyDraft("从管理页面保存并应用"))) await loadCore(true);
}
async function discardDraft() {
  if (!(await ask(
    "放弃当前草稿",
    "确定放弃当前全部未应用修改吗？草稿将恢复为 Nginx 正在使用的配置，此操作无法撤销，但不会重载 Nginx 或删除配置历史。",
  ))) return;
  const result = await mutate(() => request<State>("/draft", { method: "DELETE" }));
  if (result) {
    toast("当前草稿已放弃", "success");
    showDraftPreview.value = false;
    await loadCore(true);
  }
}
const sourceReplacement = ref<ReturnType<typeof sourceRuleCandidate>["source"]>();
const ruleSourceEntry = ref<SourceEntry>();
const annotatedHTTPSource = ref(false);
async function inspectSource(entry: SourceEntry) {
  const block = formSourceBlock(entry);
  if (block.annotated) {
    const form = await request<ConfigForm>("/config-form",{method:"POST",body:jsonBody({content:block.raw})});
    if (form.kind!=="rules") throw Error("配置表单类型与 HTTP 站点不匹配");
    return {source:block.source,rule:form.value,annotated:true};
  }
  const { source, raw, rule, generated } = sourceRuleCandidate(entry, state.value!.rules);
  const parsed = await request<{rule: ProxyRule; content: string}>("/config-import/inspect", {
    method: "POST", body: jsonBody({source, rules: [rule]}),
  });
  if (generated && ruleDirectives(parsed.content) !== ruleDirectives(raw)) {
    throw new UnsupportedFormError(entry.line, "此配置含规则表单无法完整还原的指令");
  }
  return { source, rule: parsed.rule, annotated:false };
}
const pendingSource = ref<SourceEntry>();
function previewSource(entry: SourceEntry) {
  rulePreview.value = { name: entry.title, content: formSourceBlock(entry).raw, draft: !!state.value?.dirty };
}
async function changeSource(entry: SourceEntry, remove: boolean) {
  const verb = remove ? "删除" : "停用";
  if (busy.value || pendingSource.value) return;
  if (!remove) pendingSource.value = entry;
  try {
    const action = remove ? "删除此配置块" : "将此配置块转为未分组规则并停用";
    if (!await ask(verb + "代理规则", "确定" + action + "（" + entry.title + "）吗？此操作会立即应用当前全部草稿。")) return;
    let saved = false;
    await mutate(async () => {
      if (remove) {
        await request("/config-import/source-block", { method: "DELETE", body: jsonBody(formSourceBlock(entry).source) });
      } else {
        const { source, rule } = await inspectSource(entry);
        await saveSourceRule(source, { ...rule, enabled: false });
      }
      saved = true;
      await applyDraft(verb + "规则：" + entry.title, "规则已" + verb + "并应用");
    });
    if (saved) await loadCore(true);
  } finally { pendingSource.value = undefined; }
}
async function saveSourceRule(source: ReturnType<typeof sourceBlock>["source"], rule: ProxyRuleInput & {id?: string}) {
  const payload = { source, rules: [rule] };
  const expected = await request<string[]>("/config-import/preview", { method: "POST", body: jsonBody(payload) });
  await request("/config-import/save", { method: "POST", body: jsonBody({ ...payload, expected }) });
}
async function openSourceRule(entry: SourceEntry) {
  if (entry.file.name.endsWith(".stream")) {
    const { raw, annotated } = formSourceBlock(entry);
    const form = annotated
      ? await request<ConfigForm>("/config-form", {method:"POST",body:jsonBody({content:raw})})
      : await matchSavedConfig(raw, state.value!, entry.line);
    if (form?.kind !== "streams") throw Error("此配置无法完整还原为 TCP/UDP 规则表单");
    streamEditor.value?.show(form.value, entry);
  } else {
    const {source, rule, annotated} = await inspectSource(entry);
    if (page.value !== "rules" || modal.value) return;
    openRule(rule);
    ruleSourceEntry.value = entry;
    annotatedHTTPSource.value = annotated;
    sourceReplacement.value = source;
  }
  sourceEditorEntry.value = undefined;
}
async function editHTTPSource(entry: SourceEntry) {
  if (busy.value) return;
  sourceFormIssue.value = "";
  if (entry.error) {
    sourceEditorEntry.value = entry;
    return;
  }
  busy.value = true;
  try {
    await openSourceRule(entry);
  } catch (error) {
    if (page.value === "rules" && !modal.value) {
      sourceFormIssue.value = "无法打开规则表单：" + errorMessage(error);
      sourceEditorEntry.value = entry;
    }
  } finally {
    busy.value = false;
  }
}
const ruleForm = ref<InstanceType<typeof RuleForm>>();
const ruleParametersButton = ref<HTMLButtonElement>();
async function closeSourceEditor() {
  sourceEditorEntry.value = undefined;
  if (modal.value === "rule") {
    await nextTick();
    ruleParametersButton.value?.focus();
  }
}
async function openRuleParameters() {
  const entry = ruleSourceEntry.value;
  if (!entry || busy.value) return;
  if (ruleForm.value?.hasEdits() && !await ask("切换编辑方式", "参数与源码显示已保存的配置。返回表单可继续当前修改；保存参数或源码将结束本次表单编辑。是否继续？")) return;
  sourceFormIssue.value = "";
  sourceEditorEntry.value = entry;
}
function openRule(rule: ProxyRule | null = null) {
  sourceReplacement.value = undefined;
  ruleSourceEntry.value = undefined;
  annotatedHTTPSource.value = false;
  editingRule.value = rule;
  newRuleGroup.value = selectedGroup.value === "all" ? "" : selectedGroup.value;
  modal.value = "rule";
}
function openCertificate() {
  modal.value = "certificate";
}
function closeModal() {
  if (!busy.value) {
    rateLimitManagerOpen.value = false;
    modal.value = null;
  }
}
function manageRateLimitPolicies(policy?: RateLimitPolicy) {
  if (busy.value) return;
  editingRateLimitPolicy.value = policy;
  rateLimitManagerOpen.value = true;
}
async function saveRule(value: ProxyRuleInput, applyAfter: boolean) {
  const id = editingRule.value?.id;
  let persisted = false;
  const saved = await mutate(async () => {
    if (sourceReplacement.value) {
      const source = sourceReplacement.value;
      let replacement;
      if (annotatedHTTPSource.value) {
        const preview = await request<{exported:string}>("/config-form/preview", {method:"POST",body:jsonBody({kind:"rules",id,value:{...value,id}})});
        replacement = replaceSourceRange(source,preview.exported.trimEnd());
      } else {
        const [rendered] = await request<string[]>("/config-import/preview", {method:"POST",body:jsonBody({source,rules:[{...value,id}]})});
        replacement = updateSourceBlock(source,rendered);
      }
      const {content,end} = replacement;
      const file = await request<CustomConfig>("/custom-configs/" + encodeURIComponent(source.name), {
        method: "PUT", body: jsonBody({ name: source.name, content, expected_content: source.expected }),
      });
      sourceReplacement.value = { ...source, expected: file.content, end };
      if (ruleSourceEntry.value) {
        const entry = ruleSourceEntry.value;
        const index = sourceEntries([entry.file]).findIndex(item => item.start === entry.start);
        ruleSourceEntry.value = sourceEntries([file])[index];
      }
    } else {
      editingRule.value = await saveObject<ProxyRule>("/rules", id ?? "", value);
    }
    persisted = true;
    if (applyAfter) {
      try {
        await applyDraft(`${id ? "修改" : "新增"}规则：${value.name}`, "规则已保存并应用");
      } catch (e) { throw new Error(`规则已保存，但应用失败：${errorMessage(e)}。修正后可再次保存并应用。`); }
    }
    return true;
  });
  if (saved) {
    modal.value = null;
    if (!applyAfter) toast("规则已保存为草稿", "success");
  }
  if (persisted) await loadCore(true);
}
async function importCertificate(value: CertificateInput) {
  const saved = await mutate(() =>
    request("/certificates", { method: "POST", body: jsonBody(value) }),
  );
  if (saved !== undefined) {
    modal.value = null;
    toast("证书已安全导入", "success");
    await loadCore(true);
  }
}
async function createACME(value: ACMEInput) {
  const saved = await mutate(() => request('/acme', { method: 'POST', body: jsonBody(value) }));
  if (saved !== undefined) { modal.value = null; toast('ACME 任务已创建，将在后台申请证书', 'success'); }
}
function ask(title: string, message: string) {
  confirmBox.title = title;
  confirmBox.message = message;
  confirmBox.open = true;
  return new Promise<boolean>((resolve) => (confirmBox.resolve = resolve));
}
function answerConfirm(value: boolean) {
  confirmBox.open = false;
  confirmBox.resolve?.(value);
  confirmBox.resolve = null;
}
async function deleteRule(rule: ProxyRule) {
  if (
    !(await ask(
      "删除代理规则",
      `确定删除“${rule.name}”吗？删除后会立即应用当前全部草稿，并更新运行中的 Nginx。`,
    ))
  )
    return;
  let deleted = false;
  await mutate(
    async () => {
      await request(`/rules/${rule.id}`, { method: "DELETE" });
      deleted = true;
      try {
        return await applyDraft(`删除规则：${rule.name}`, "规则已删除并应用");
      } catch (error) {
        throw new Error(`规则已删除，但应用失败：${errorMessage(error)}。请修正后重新保存并应用。`);
      }
    },
  );
  if (deleted) await loadCore(true);
}
const httpToggleTargets = reactive<Record<string, boolean | undefined>>({});
const streamToggleTargets = reactive<Record<string, boolean | undefined>>({});
async function toggleRuleEnabled(rule: ProxyRule | StreamRule, enabled: boolean, stream = false) {
  if (busy.value) return;
  const targets = stream ? streamToggleTargets : httpToggleTargets;
  targets[rule.id] = enabled;
  try {
    await mutate(async () => {
      const result = await request<ApplyResult>(`/${stream ? "streams" : "rules"}/${rule.id}/enabled`, {
        method: "POST", body: jsonBody({ enabled }),
      });
      // Commit the confirmed flag before the broader overview/state refresh.
      const rules = stream ? state.value?.stream_rules : state.value?.rules;
      const current = rules?.find(item => item.id === rule.id);
      if (current) current.enabled = enabled;
      reportActivation(result, `${stream ? "Stream " : ""}规则已${enabled ? "启用" : "停用"}并立即生效`);
      await loadCore(true);
      return true;
    });
  } finally {
    delete targets[rule.id];
  }
}
function toggleRule(rule: ProxyRule, enabled: boolean) {
  return toggleRuleEnabled(rule, enabled);
}
async function deleteCertificate(id: string, name: string) {
  if (
    !(await ask(
      "删除 SSL/TLS 证书",
      `确定删除“${name}”吗？将同时删除证书、私钥及关联的 ACME 任务和凭据，并停止自动续期。此操作无法恢复。`,
    ))
  )
    return;
  const ok = await mutate(
    () => request(`/certificates/${id}`, { method: "DELETE" }),
    "证书已删除",
  );
  if (ok !== undefined) await loadCore(true);
}
async function stopNginx() {
  if (
    await ask(
      "停止独立 Nginx",
      "停止后所有由 nginx-web 提供的代理入口都会暂时不可访问，管理页面仍可使用。",
    )
  )
    await runNginxAction("stop");
}
async function revisionAction(
  revision: Revision,
  action: "restore" | "delete",
) {
  const restoring = action === "restore";
  if (
    !(await ask(
      restoring ? "恢复配置历史" : "删除配置历史",
      restoring
        ? "历史版本的规则和设置将覆盖当前草稿（包括尚未应用的修改），不会立刻影响当前代理。"
        : "删除后不能再恢复该版本。",
    ))
  )
    return;
  const ok = await mutate(
    () =>
      request(
        `/revisions/${encodeURIComponent(revision.id)}${restoring ? "/restore" : ""}`,
        {
          method: restoring ? "POST" : "DELETE",
          body: restoring ? "{}" : undefined,
        },
      ),
    restoring ? "历史版本已恢复为草稿" : "配置历史已删除",
  );
  if (ok !== undefined) {
    await loadCore(true);
    if (restoring) {
      closePreview();
      await nextTick();
      document.querySelector(".workspace")?.scrollTo({ top: 0 });
    }
  }
}
async function saveSettings(value: Settings) {
  const ok = await mutate(
    () => request("/settings", { method: "PUT", body: jsonBody(value) }),
    "设置已保存为草稿",
  );
  if (ok !== undefined) await loadCore(true);
}
async function saveRevisionLimit(value: number) {
  if (!state.value) return;
  await saveSettings({ ...state.value.settings, revision_limit: value });
}
async function testSettings(value: Settings) {
  const result = await mutate(() => request<ApplyResult>("/settings/test", {
    method: "POST", body: jsonBody(value),
  }));
  if (result) toast(result.message, "success");
}
async function applySettings(value: Settings) {
  if (busy.value) return;
  busy.value = true;
  let saved = false;
  try {
    await request("/settings", { method: "PUT", body: jsonBody(value) });
    saved = true;
    await applyDraft("保存并应用全局设置");
  } catch (error) {
    toast((saved ? "设置已保存为草稿，但应用失败：" : "") + errorMessage(error), "error");
  } finally {
    if (saved) await loadCore(true);
    busy.value = false;
  }
}
async function downloadBackup() {
  const backup = await mutate(() => request<BackupFile>("/backup"));
  if (!backup) return;
  const url = URL.createObjectURL(new Blob([JSON.stringify(backup, null, 2)], { type: "application/json" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = `nginx-web-backup-${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
async function restoreBackup(backup: BackupFile) {
  if (!await ask("从备份恢复", "将替换已保存的全局设置和代理配置，并保留恢复前的配置历史。恢复后仅保存为草稿，不会立即应用。")) return;
  const result = await mutate(() => request<State>("/backup/restore", {
    method: "POST", body: jsonBody(backup),
  }), "备份已恢复为草稿，请确认后应用");
  if (result) await loadCore(true);
}
async function clearCache() {
  if (!(await ask("清理代理缓存", "将删除 nginx-web 生成的全部 HTTP 缓存文件，正在处理的请求可能重新回源。"))) return;
  await mutate(() => request("/cache", { method: "DELETE" }), "代理缓存已清理");
}
async function saveAndApplyResource<T>(path: string, value: object, id: string, label: string, done: (saved?: T, error?: string) => void) {
  if (busy.value) return;
  busy.value = true;
  let saved: T | undefined;
  try {
    saved = await saveObject<T>(path, id, value);
    await applyDraft(`保存并应用${label}`, `${label}已保存并应用`);
    done(saved);
  } catch (error) {
    done(saved, saved ? `${label}已保存，但应用失败：${errorMessage(error)}。修正后可再次保存并应用。` : errorMessage(error));
  } finally {
    if (saved) await loadCore(true);
    busy.value = false;
  }
}
async function saveUpstreamPool(value: UpstreamPoolInput, id: string, done: (saved?: UpstreamPool, error?: string) => void) {
  await saveAndApplyResource("/upstreams", value, id, "转发服务组", done);
}
async function removeUpstreamPool(pool: UpstreamPool) {
  if (
    !(await ask(
      "删除转发服务组",
      `确定删除“${pool.name}”吗？正在使用的转发服务组不能删除。`,
    ))
  )
    return;
  const ok = await mutate(
    () => request(`/upstreams/${pool.id}`, { method: "DELETE" }),
    "转发服务组已删除",
  );
  if (ok !== undefined) await loadCore(true);
}
async function saveRateLimitPolicy(value: RateLimitPolicyInput, id: string, done: (saved?: RateLimitPolicy, error?: string) => void) {
  await saveAndApplyResource("/rate-limit-policies", value, id, "限流策略", done);
}
async function removeRateLimitPolicy(policy: RateLimitPolicy) {
  if (
    !(await ask(
      "删除限流策略",
      `确定删除“${policy.name}”吗？正在使用的策略不能删除。`,
    ))
  )
    return;
  const ok = await mutate(
    () => request(`/rate-limit-policies/${policy.id}`, { method: "DELETE" }),
    "限流策略已删除",
  );
  if (ok !== undefined) await loadCore(true);
}
async function saveStreamRule(value: StreamRuleInput, id: string, entry?: SourceEntry) {
  let saved = false;
  await mutate(
    async () => {
      if (entry) {
        const preview = await request<{content:string;exported:string}>("/config-form/preview", {method:"POST", body:jsonBody({kind:"streams",id,value:{...value,id}})});
        const {source,annotated} = formSourceBlock(entry);
        const index = sourceEntries([entry.file]).findIndex(item=>item.start===entry.start);
        const content = (annotated ? replaceSourceRange(source,preview.exported.trimEnd()) : updateSourceBlock(source,preview.content)).content;
        const savedFile = await request<CustomConfig>("/custom-configs/" + encodeURIComponent(source.name), {method:"PUT",body:jsonBody({name:source.name,content,expected_content:source.expected})});
        streamEditor.value?.show({...value,id} as StreamRule, sourceEntries([savedFile])[index]);
      } else await saveObject("/streams", id, value);
      saved = true;
      try {
        const result = await applyDraft("保存并应用 TCP/UDP 规则", "TCP/UDP 规则已保存并应用");
        streamEditor.value?.close();
        return result;
      } catch (error) {
        throw new Error(`规则已保存，但应用失败：${errorMessage(error)}。请修正后重新保存，或到“Nginx 配置”页面重新应用。`);
      }
    },
  );
  if (saved) await loadCore(true);
}
async function removeStreamRule(rule: StreamRule) {
  if (!(await ask("删除 TCP/UDP 规则", `确定删除“${rule.name}”吗？`))) return;
  const ok = await mutate(
    () => request(`/streams/${rule.id}`, { method: "DELETE" }),
    "Stream 规则已删除",
  );
  if (ok !== undefined) await loadCore(true);
}
function toggleStreamRule(rule: StreamRule, enabled: boolean) {
  return toggleRuleEnabled(rule, enabled, true);
}
async function copyConfig() {
  try {
    await copyText(configText.value);
    configCopied.value = true;
    if (configCopiedTimer) window.clearTimeout(configCopiedTimer);
    configCopiedTimer = window.setTimeout(() => {
      configCopied.value = false;
      configCopiedTimer = undefined;
    }, 1800);
  } catch {
    const message = "浏览器不允许复制，请手动选择文本";
    if (!toasts.value.some(item => item.message === message)) toast(message, "error");
  }
}
function certificateState(notAfter: string) {
  const end = new Date(notAfter).getTime(),
    expired = end < Date.now(),
    soon = !expired && end - Date.now() < 30 * 86400000;
  return {
    label: expired ? "已过期" : soon ? "即将过期" : "有效",
    className: expired ? "danger" : soon ? "warning" : "success",
  };
}
watch(page, (value) => {
  document.querySelector(".workspace")?.scrollTo({ top: 0 });
  if (value === "logs") void loadLogs();
  else stopLogs();
  if (value === "config") void loadConfig();
});
watch(configTab, () => {
  configCopied.value = false;
  if (configCopiedTimer) window.clearTimeout(configCopiedTimer);
  configCopiedTimer = undefined;
});
watch(menuOpen, (value) => document.body.classList.toggle("menu-open", value));
function keydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    if (activePreview.value || sourceEditorEntry.value) return;
    if (confirmBox.open) answerConfirm(false);
    else if (rateLimitManagerOpen.value && !busy.value) rateLimitManagerOpen.value = false;
    else closeModal();
  }
}
onMounted(() => {
  document.addEventListener("keydown", keydown);
  void loadCore();
});
onBeforeUnmount(() => {
  stopLogs();
  if (configCopiedTimer) window.clearTimeout(configCopiedTimer);
  document.removeEventListener("keydown", keydown);
  document.body.classList.remove("menu-open");
});
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <nav class="nav" aria-label="主导航">
        <template v-for="item in visiblePages" :key="item.id">
          <h2 v-if="item.groupLabel" class="nav-group-label">{{ item.groupLabel }}</h2>
        <button
          type="button"
          class="nav-item"
          :class="{ active: page === item.id }"
          :aria-current="page === item.id ? 'page' : undefined"
          @click="setPage(item.id)"
        >
          <SidebarIcon :name="item.icon" class="nav-icon" />
          <span>{{ item.label }}</span>
        </button>
        </template>
      </nav>
      <div class="sidebar-bottom">
        <button v-if="adminUsername" type="button" class="button ghost small admin-logout" :disabled="loggingOut" @click="emit('logout')" :title="adminUsername + ' · 退出登录'">{{ loggingOut ? '正在退出…' : '退出登录' }}<span>{{ adminUsername }}</span></button>
        <div class="sidebar-footer">
          <div class="sidebar-runtime-row">
            <div class="mini-status">
              <span
                class="status-dot"
                :class="
                  connectionError
                    ? 'offline'
                    : !overview
                      ? 'neutral'
                      : overview.nginx.running
                        ? overview.dirty
                          ? 'warning'
                          : 'online'
                        : 'offline'
                "
              ></span
              ><span>{{
                connectionError
                  ? "连接管理服务失败"
                  : !overview
                    ? "正在连接…"
                    : overview.nginx.running
                      ? overview.dirty
                        ? (!overview.applied_known ? "运行中 · 配置状态待确认" : "运行中 · 有未应用变更")
                        : "运行中"
                      : "已停止"
              }}</span>
            </div>
            <span class="sidebar-version">Nginx {{ overview?.nginx_version ?? "1.30.7" }}</span>
          </div>
        </div>
      </div>
    </aside>
    <div class="workspace-frame">
    <div class="workspace" :class="{ 'logs-workspace': page === 'logs' }">
      <div class="mobile-nav-bar">
        <button
          type="button"
          class="icon-button"
          aria-label="打开菜单"
          @click="menuOpen = !menuOpen"
        >
          <PhList :size="22" aria-hidden="true" />
        </button>
        <strong>{{ currentPage.label }}</strong>
      </div>
      <main class="content" aria-live="polite">
        <div v-if="page === 'settings' && !loading && !connectionError && incompatiblePorts.length" class="notice warning" role="status">
          <strong>当前标准版不支持以下低端口配置</strong>
          <p>相关规则已停用，原始端口和转发设置已保留；低端口默认入口不会监听。请将端口改为 1024–65535 后重新启用并应用，或安装全端口版后手动启用。</p>
          <ul><li v-for="item in incompatiblePorts" :key="item">{{ item }}</li></ul>
        </div>
        <div v-if="loading" class="loading-panel">
          <div class="spinner"></div>
          <p>正在读取 nginx-web 状态…</p>
        </div>
        <article v-else-if="connectionError" class="card empty-state">
          <div class="empty-icon"><PhXCircle :size="24" aria-hidden="true" /></div>
          <h3>无法读取 nginx-web 状态</h3>
          <p>{{ connectionError }}</p>
          <button type="button" class="button primary" @click="loadCore()">
            重新连接
          </button>
        </article>
        <template v-else-if="overview && state">
          <DashboardPage v-if="page === 'dashboard'"
            :overview="overview" :busy="busy" :updated-at="state.updated_at"
            :certificates="state.certificates"
            :initial-minutes="errorScope.minutes" initial-rule=""
            @overview="overview = $event" @add="openRule()"
            @test="runNginxAction('test')" @reload="runNginxAction('reload')" @start="runNginxAction('start')"
            @stop="stopNginx"
            @copy="copyProxyAddress"
            @errors="openErrorDetails"
            @navigate="setPage"
          />
          <WAFPage ref="wafEditor" @import-config="importing=true" @export-config="(id,name)=>previewForm('waf',id,name)" v-else-if="page === 'waf'" :state="state" :busy="busy" @refresh="loadCore(true)" @apply="applyConfiguration" />
          <NetworkTopology v-else-if="page === 'topology'" />
          <ErrorDetailsPage v-else-if="page === 'errors'"
            :overview="overview" :busy="busy" :updated-at="state.updated_at"
            :initial-minutes="errorScope.minutes" :initial-rule="errorScope.rule"
            @overview="overview = $event" @edit="openDashboardRule"
          />
          <BackupPage v-else-if="page === 'backup'" :state="state" :busy="busy" @download="downloadBackup" @restore="restoreBackup" @apply="applyConfiguration" />
          <GatewayRoutesPage v-else-if="page === 'gateway'" :routes="state.gateway_routes ?? []" @updated="updateGatewayRoute" :busy="busy" @refresh="loadCore(true)" />
          <template v-else-if="page === 'streams'">
            <StreamRulesPage ref="streamEditor" @import-config="importing=true" @export-config="(id,name)=>previewForm('streams',id,name)" @copy="copyProxyAddress"
              :rules="state.stream_rules" :sources="sources.filter(s=>s.file.name.endsWith('.stream'))" @source="sourceEditorEntry = $event" @preview="(r)=>showConfig(r.id,r.name,true)"
              :dirty="state.dirty"
              :pending-toggles="streamToggleTargets"
              :pools="state.upstream_pools"
              :certificates="state.certificates"
              :busy="busy"
              @save="saveStreamRule"
              @remove="removeStreamRule"
              @toggle="toggleStreamRule"
              @refresh="loadCore()"
            />
          </template>
          <template v-else-if="page === 'upstreams'"
            ><UpstreamPoolsPage ref="upstreamEditor" @import-config="importing=true" @export-config="(id,name)=>previewForm('upstreams',id,name)"
              :pools="state.upstream_pools"
              :busy="busy"
              @save="saveUpstreamPool"
              @remove="removeUpstreamPool"
              @refresh="loadCore()"
          /></template>
          <AuthProfilesPage ref="authEditor" @import-config="importing=true" @export-config="(id,name)=>previewForm('auth-profiles',id,name)" v-else-if="page === 'auth-profiles'" :profiles="state.auth_profiles ?? []" :rules="state.rules" :busy="busy"
            @changed="updateAuthProfiles" @remove="removeAuthProfile" @refresh="loadCore()" />
          <template v-else-if="page === 'rate-limits'"
            ><RateLimitPoliciesPage ref="policyEditor" @import-config="importing=true" @export-config="(id,name)=>previewForm('rate-limits',id,name)"
              :policies="state.rate_limit_policies"
              :rules="state.rules"
              :busy="busy"
              @save="saveRateLimitPolicy"
              @remove="removeRateLimitPolicy"
              @refresh="loadCore()"
              :analysis-enabled="menuEnabled('errors')"
              @inspect="setPage('errors')"
          /></template>
          <template v-else-if="page === 'rules'"
            ><div class="toolbar compact-rule-toolbar"><span
                class="badge"
                :class="state.dirty ? 'warning' : 'success'"
                >{{ !appliedConfigurationKnown ? (state.last_applied_at ? "配置状态待确认" : "待初始化") : state.dirty ? "有未应用变更" : "配置已同步" }}</span>
              <span class="rule-port-hint">监听端口 {{ minListenPort }}–65535</span>
              <span class="spacer"></span
              ><button class="button ghost" :disabled="busy" @click="loadCore()">
                <PhArrowClockwise :size="15" aria-hidden="true" />刷新
              </button>
              <button class="button ghost" :disabled="busy" @click="groupPanel?.show()"><PhPlusCircle :size="17" aria-hidden="true" />新建分组</button>
              <button class="button secondary" @click="importing=true">从 Nginx 配置导入</button>
              <button class="button primary" @click="openRule()">
                <PhPlusCircle :size="17" aria-hidden="true" />添加代理规则
              </button>
            </div>
<div class="toolbar rule-filters compact-rule-toolbar" role="search" aria-label="HTTP/HTTPS 规则筛选">
              <input
                v-model="ruleSearch"
                class="input search-input"
                type="search"
                aria-label="搜索 HTTP/HTTPS 规则"
                placeholder="搜索名称、域名、端口或目标"
              />
              <AppSelect v-model="selectedGroup" class="select" aria-label="分组筛选">
                <option value="all">全部分组</option><option value="">未分组</option>
                <option v-for="group in ruleGroups" :key="group.id" :value="group.id">{{ group.name }}</option>
              </AppSelect>
              <AppSelect v-model="ruleProtocol" class="select" aria-label="HTTP/HTTPS 协议筛选">
                <option value="all">全部协议</option><option value="http">HTTP</option><option value="https">HTTPS</option>
              </AppSelect>
              <AppSelect v-model="ruleEnabled" class="select" aria-label="HTTP/HTTPS 启用状态筛选">
                <option value="all">全部状态</option><option value="enabled">已启用</option><option value="disabled">已停用</option>
              </AppSelect>
              <button class="button ghost" :disabled="!hasRuleFilters" @click="resetRuleFilters">重置筛选</button>
              <span class="filter-count">{{ filteredRules.length + filteredSources.length }} / {{ state.rules.length + httpSources.length }} 条</span>
            </div>
            <ConfigSources rule-editor :busy="busy || !!pendingSource" :pending="pendingSource" @preview="previewSource" @disable="changeSource($event, false)" @remove="changeSource($event, true)" @copy="copyProxyAddress" :entries="filteredSources" @edit="editHTTPSource" />
            <RuleGroups ref="groupPanel" :groups="ruleGroups" :rules="state.rules" :certificates="state.certificates"
              :selected="selectedGroup" :busy="busy" :default-port="state.settings.default_http_port"
              :save="saveRuleGroup" :remove="deleteRuleGroup" @select="selectedGroup = $event" />
            <article v-for="section in groupedRuleSections" :key="section.id" class="card proxy-rule-card grouped-rule-card">
              <header class="proxy-group-heading" :class="{ 'is-collapsed': collapsedGroups.has(section.id) }">
                <div class="proxy-group-summary">
                  <button type="button" class="button ghost small group-toggle" :aria-expanded="!collapsedGroups.has(section.id)" :aria-controls="`group-rules-${section.id || 'ungrouped'}`" :aria-label="`${collapsedGroups.has(section.id) ? '展开' : '收起'}分组 ${section.name}`" @click="toggleGroupExpanded(section.id)"><PhCaretRight v-if="collapsedGroups.has(section.id)" :size="15" aria-hidden="true" /><PhCaretDown v-else :size="15" aria-hidden="true" />{{ collapsedGroups.has(section.id) ? '展开' : '收起' }}</button>
                  <h3>{{ section.name }}</h3><span class="proxy-group-count">{{ section.rules.length }} 条规则</span><p v-if="section.group" :title="groupDescription(section.group)">{{ groupDescription(section.group) }}</p>
                </div>
                <div class="table-actions"><button v-if="section.group" class="button ghost small" @click="previewForm('rule-groups', section.group.id, section.group.name)">复制分组配置</button><button v-if="section.group" class="button secondary small" :disabled="busy" @click="groupPanel?.show(section.group)">编辑分组</button><button class="button primary small" :disabled="busy" @click="addRuleToGroup(section.id)"><PhPlusCircle :size="15" />添加规则</button></div>
              </header>
              <div v-show="!collapsedGroups.has(section.id)" :id="`group-rules-${section.id || 'ungrouped'}`">
              <div v-if="section.rules.length" class="table-wrap">
                <table class="table http-rule-table">
                  <HttpRuleTableHead />
                  <tbody>
                    <tr v-for="rule in section.rules" :key="rule.id">
                      <td>
                        <RuleStatusSwitch :enabled="rule.enabled" :pending-enabled="httpToggleTargets[rule.id]"
                          :disabled="busy" :label="`${rule.enabled ? '停用' : '启用'}规则 ${rule.name}`"
                          @change="toggleRule(rule, $event)" />
                      </td>
                      <td>
                        <div class="rule-name rule-cell-text" :title="rule.name">{{ rule.name }}</div>
                      </td>
                      <td>
                        <ProxyAddress v-for="domain in rule.domains" :key="domain" :value="formatRuleEntry(rule, domain)" entry
                          :openable="canOpenRuleEntry(domain)" :copy-disabled="domain === '*'" :unsupported="rule.listen_port < minListenPort" @copy="copyProxyAddress" />
                        <div v-if="!rule.domains.length" class="rule-cell-text" :class="{ 'unsupported-entry': rule.listen_port < minListenPort }" :title="formatRuleEntry(rule) + (rule.listen_port < minListenPort ? '（当前版本不支持此监听端口）' : '')">{{ formatRuleEntry(rule) }}</div>
                      </td>
                      <td>
                        <ProxyAddress v-if="ruleBackends.get(rule.id)?.address" :value="ruleBackends.get(rule.id)!.label" @copy="copyProxyAddress" />
                        <span v-else class="rule-cell-text" :title="ruleBackends.get(rule.id)?.label">{{ ruleBackends.get(rule.id)?.label }}</span>
                      </td>
                      <td>
                        <div class="table-actions">
                          <button type="button" class="icon-button" aria-label="查看配置" title="查看配置" aria-haspopup="dialog" @click="showConfig(rule.id, rule.name)">
                            <PhFileCode :size="18" aria-hidden="true" />
                          </button>
                          <button
                            class="button ghost small"
                            @click="openRule(rule)"
                          >
                            编辑</button
                          ><button
                            class="button danger-ghost small"
                            @click="deleteRule(rule)"
                          >
                            删除
                          </button>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <div v-else class="group-empty">此分组暂无规则，点击“添加规则”创建。</div>
              </div>
            </article>
            <article v-if="!groupedRuleSections.length" class="card empty-state"><h3>没有匹配的规则</h3><p>调整分组、关键词、协议或状态筛选后重试。</p></article>
            </template
          >
          <template v-else-if="page === 'certificates'"
            ><div class="toolbar compact-rule-toolbar">
              <div class="notice">
                只保存 PEM
                文件，不会把私钥返回到浏览器。证书目录权限为
                0700，私钥文件权限为 0600。
              </div>
              <span class="spacer"></span
              ><button class="button ghost" :disabled="busy" @click="loadCore()">
                <PhArrowClockwise :size="15" aria-hidden="true" />刷新
              </button>
              <button class="button primary" @click="openCertificate">
                <PhPlusCircle :size="17" aria-hidden="true" />导入证书
              </button>
            </div>
            <article class="card">
              <ACMEJobs ref="acmeJobsPanel" :certificates="state.certificates" @changed="loadCore(true)" />
              <div v-if="state.certificates.length" class="table-wrap">
                <table class="table">
                  <thead>
                    <tr>
                      <th>名称</th>
                      <th>域名 / 主体</th>
                      <th>有效期</th>
                      <th>指纹</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="cert in state.certificates" :key="cert.id">
                      <td>
                        <div class="rule-name">{{ cert.name }}</div>
                      </td>
                      <td>
                        <div>
                          {{ (cert.dns_names ?? []).join(", ") || cert.subject || "—" }}
                        </div>
                      </td>
                      <td>
                        <div class="certificate-validity">
                          <span class="badge" :class="certificateState(cert.not_after).className">{{ certificateState(cert.not_after).label }}</span>
                          <span class="certificate-validity-dates">{{ formatDate(cert.not_before, true) }} ～ {{ formatDate(cert.not_after, true) }}</span>
                        </div>
                      </td>
                      <td>
                        <code :title="cert.fingerprint">{{
                          shortFingerprint(cert.fingerprint)
                        }}</code>
                      </td>
                      <td>
                        <div class="certificate-row-actions">
                        <button class="button ghost small" @click="acmeJobsPanel?.showDetails(cert.id)">详情</button>
                        <button class="button ghost small" @click="acmeJobsPanel?.showEdit(cert.id)">修改</button>
                        <button
                          class="button danger-ghost small"
                          @click="deleteCertificate(cert.id, cert.name)"
                        >
                          删除
                        </button>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <div v-else class="empty-state">
                <div class="empty-icon">◇</div>
                <h3>尚未导入 SSL/TLS 证书</h3>
                <p>
                  支持 ACME 自动申请、文件上传、服务器路径和粘贴 PEM。
                </p>
              </div>
            </article></template
          >
          <template v-else-if="page === 'logs'"
            ><article class="card log-card">
              <header class="card-header log-card-header">
                <div>
                  <h2>实时日志</h2>
                  <p>显示最近 {{ logLineLimit }} 行 · {{ logSearch ? "搜索中，自动刷新已暂停" : "每 3 秒刷新 · 自动滚动至最新日志" }}</p>
                </div>
                <span class="spacer"></span>
                <div class="log-toolbar">
                  <AppSelect v-model="logType" hide-check class="select" aria-label="日志类型" @change="changeLogScope">
                    <option value="access">HTTP/HTTPS 访问日志</option>
                    <option value="stream">TCP/UDP Stream 日志</option>
                    <option value="backend">nginx-web 管理日志</option>
                    <option value="error">Nginx 错误日志</option></AppSelect
                  ><button class="button ghost small" @click="loadLogs">
                    刷新
                  </button>
                </div>
              </header>
              <div class="log-search-toolbar" role="search" aria-label="搜索日志">
                <AppSelect v-model="logLineLimit" hide-check class="select log-line-limit" aria-label="显示日志行数" @change="changeLogScope">
                  <option v-for="limit in [100, 200, 500, 1000, 2000]" :key="limit" :value="limit">最近 {{ limit }} 行</option>
                </AppSelect>
                <input v-model="logSearch" class="input" type="search" :placeholder="`搜索最近 ${logLineLimit} 行日志…`" aria-label="搜索日志内容" @keydown.enter.prevent="jumpLogMatch($event.shiftKey ? -1 : 1)" @keydown.esc.prevent="logSearch = ''" />
                <span v-if="logSearch" class="log-search-count" aria-live="polite">{{ logSearch ? searchedLogs.count ? `${activeLogMatch + 1} / ${searchedLogs.count}` : '无匹配' : '' }}</span>
                <button v-if="logSearch" class="button ghost small log-match-button" :disabled="!searchedLogs.count" aria-label="上一个匹配" title="上一个匹配（Shift+Enter）" @click="jumpLogMatch(-1)"><PhArrowUp :size="18" weight="bold" aria-hidden="true" /></button>
                <button v-if="logSearch" class="button ghost small log-match-button" :disabled="!searchedLogs.count" aria-label="下一个匹配" title="下一个匹配（Enter）" @click="jumpLogMatch(1)"><PhArrowDown :size="18" weight="bold" aria-hidden="true" /></button>
                <button v-if="logSearch" class="button ghost small" @click="logSearch = ''">清除</button>
                <label class="checkbox-row log-wrap-toggle"><input v-model="logWrap" type="checkbox" />自动换行</label>
              </div>
              <pre ref="logView" class="log-view" :class="{ 'is-wrapped': logWrap }"><template v-if="logLines.length"><template v-for="(line, lineIndex) in searchedLogs.lines" :key="lineIndex"><span v-for="(token, tokenIndex) in line" :key="tokenIndex" :data-log-match="token.matchIndex" :class="[token.tone ? `log-token-${token.tone}` : undefined, { 'log-search-hit': token.matchIndex !== undefined, 'log-search-current': token.matchIndex !== undefined && token.matchIndex === activeLogMatch }]">{{ token.text }}</span>{{ lineIndex < highlightedLogLines.length - 1 ? '\n' : '' }}</template></template><template v-else>{{ logsLoading ? "正在读取…" : "暂无日志。" }}</template></pre>
            </article></template
          >
          <template v-else-if="page === 'revisions'"
            >
            <article class="card" :aria-label="appliedConfigurationKnown ? '当前草稿状态' : '当前配置初始化状态'">
              <header class="card-header">
                <div aria-live="polite">
                  <h2>
                    {{ appliedConfigurationKnown ? '当前草稿' : '当前配置' }}
                    <span class="badge" :class="!appliedConfigurationKnown || state.dirty ? 'warning' : 'success'">
                      {{ !appliedConfigurationKnown ? (state.last_applied_at ? '待确认' : '待初始化') : state.dirty ? '待应用' : '已同步' }}
                    </span>
                  </h2>
                  <template v-if="state.dirty && state.draft_revision_id">
                    <p class="draft-restored-title">已恢复：{{ restoredRevision ? `${formatDate(restoredRevision.created_at)} · ${restoredRevision.summary || '配置快照'}` : state.draft_revision_id }}</p>
                    <p>此版本已写入当前草稿，尚未应用到 Nginx。后续编辑会保留在草稿中。</p>
                  </template>
                  <p v-else-if="!appliedConfigurationKnown">{{ state.last_applied_at ? '无法确认上次生效的配置，暂时不能判断是否有待应用修改。' : '尚无成功应用记录，首次应用后可查看配置差异。' }}</p>
                  <p v-else>{{ state.dirty ? '当前有尚未应用的修改，运行配置尚未切换。' : '当前配置已同步，没有待应用的修改。' }}</p>
                </div>
                <span class="spacer"></span>
                <button ref="draftChangesTrigger" class="button ghost small" aria-haspopup="dialog" :disabled="draftChangesLoading" @click="openDraftChanges">{{ draftChangesLoading ? '正在对比…' : '查看草稿变更' }}</button>
                <button class="button ghost small" @click="page = 'rules'">编辑代理规则</button>
                <button
                  v-if="state.dirty && appliedConfigurationKnown"
                  class="button danger-ghost small"
                  :disabled="busy"
                  title="恢复为当前正在生效的配置，不会重载 Nginx"
                  @click="discardDraft"
                >
                  放弃草稿
                </button>
                <button
                  v-if="state.dirty"
                  class="button primary small"
                  :disabled="busy"
                  :title="appliedConfigurationKnown ? '将当前草稿应用到 Nginx，并生成新的历史版本' : '首次应用当前配置，并创建第一个历史版本'"
                  @click="applyConfiguration"
                >
                  <PhCheckCircle :size="14" aria-hidden="true" />{{ appliedConfigurationKnown || state.last_applied_at ? '保存并应用' : '初始化并应用' }}
                </button>
              </header>
              <div v-if="state.last_apply_error" class="notice warning" role="alert">上次应用失败：{{ state.last_apply_error }}</div>
            </article>
            <article class="card section-gap">
              <header class="card-header">
                <div>
                  <h2>已应用版本</h2>
                  <p>
                    每次应用成功后自动保留，可预览完整历史配置，当前上限
                    {{ state.settings.revision_limit }} 个
                  </p>
                </div>
                <span class="spacer"></span>
                <button class="button ghost small" :disabled="busy" @click="loadCore()">
                  <PhArrowClockwise :size="14" aria-hidden="true" />刷新
                </button>

              </header>
              <RevisionRetentionForm :limit="state.settings.revision_limit" :busy="busy" @save="saveRevisionLimit" />
              <div v-if="revisions.length" class="table-wrap">
                <table class="table">
                  <thead>
                    <tr>
                      <th>时间</th>
                      <th>说明</th>
                      <th>规则</th>
                      <th>草稿状态</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <template v-for="revision in revisions" :key="revision.id">
                    <tr :class="{ 'revision-source-row': state.dirty && state.draft_revision_id === revision.id }">
                      <td>{{ formatDate(revision.created_at) }}</td>
                      <td>
                        <div class="rule-name">
                          {{ revision.summary || "配置快照" }}
                        </div>
                        <div class="rule-sub">{{ revision.id }}</div>
                      </td>
                      <td>
                        {{ revision.enabled_count }} /
                        {{ revision.rule_count }} 条启用
                      </td>
                      <td>
                        <span v-if="state.dirty && state.draft_revision_id === revision.id" class="badge warning">当前草稿来源 · 待应用</span>
                        <span v-else class="rule-sub">—</span>
                      </td>
                      <td>
                        <div class="table-actions">
                          <button class="button ghost small"
                            aria-haspopup="dialog"
                            @click="showDraftPreview = false; previewRevisionID = revision.id">
                            预览配置
                          </button>
                          <button
                            class="button ghost small"
                            :disabled="busy"
                            @click="revisionAction(revision, 'restore')"
                          >
                            恢复为草稿</button
                          ><button
                            class="button danger-ghost small"
                            :disabled="busy"
                            @click="revisionAction(revision, 'delete')"
                          >
                            删除
                          </button>
                        </div>
                      </td>
                    </tr>
                    </template>
                  </tbody>
                </table>
              </div>
              <div v-else class="empty-state">
                <div class="empty-icon">↶</div>
                <h3>还没有配置历史</h3>
                <p>首次保存并应用成功后，这里会出现可恢复的版本。</p>
              </div>
            </article>
            <div class="notice warning section-gap">
              预览不会修改配置。恢复为草稿会覆盖当前草稿中的规则和设置，但不会立即影响运行中的 Nginx；检查草稿后，点击“保存并应用”才会生效。顶部按钮应用的是当前全部草稿，并非正在预览的历史版本。
            </div></template
          >
          <template v-else-if="page === 'config'"
            ><article class="card">
              <header class="card-header config-toolbar">
                <button class="button ghost small" :disabled="busy" @click="loadConfig">
                  <PhArrowClockwise :size="14" aria-hidden="true" />刷新配置
                </button>
                <button class="button secondary small" :disabled="busy" @click="runNginxAction('test')">
                  <PhCheckCircle :size="14" aria-hidden="true" />校验配置
                </button>
                <button
                  v-if="state.dirty"
                  class="button primary small"
                  :disabled="busy"
                  @click="applyConfiguration"
                >
                  <PhCheckCircle :size="14" aria-hidden="true" />保存并应用
                </button>
                <button v-if="selectedCustomConfig" class="button secondary small" :disabled="busy" @click="parameterEntry = { file: selectedCustomConfig, title: selectedCustomConfig.name }">编辑参数</button>
                <button v-if="selectedCustomConfig" class="button ghost small" :disabled="busy" @click="openCustomConfig(selectedCustomConfig)">编辑源文件</button>
                <button v-if="selectedCustomConfig" class="button danger-ghost small" :disabled="busy" @click="deleteCustomConfig(selectedCustomConfig)">删除自定义配置</button>
                <button class="button secondary small" :disabled="busy" @click="openCustomConfig()"><PhPlusCircle :size="14" />新建自定义配置</button>
              </header>
              <nav class="config-file-selectors" aria-label="配置文件分类">
                <button class="button ghost" :class="{ 'config-category-active': configTab === 'master' }" :aria-pressed="configTab === 'master'" @click="configTab = 'master'">主配置</button>
                <AppSelect v-for="category in configFileGroups" :key="category.id" class="select config-category-select" :class="{ 'config-category-active': category.files.some(file => file.key === configTab) }" :aria-label="category.name" :model-value="category.files.some(file => file.key === configTab) ? configTab : ''" @update:model-value="configTab = $event">
                  <option value="" disabled>{{ category.name }}</option>
                  <option v-for="file in category.files" :key="file.key" :value="file.key">{{ file.label }}</option>
                </AppSelect>
              </nav>
              <div class="config-current-file">
                <span>{{ configTab === 'master' ? 'nginx.conf' : configTab }}</span>
                <button
                  type="button"
                  class="icon-button config-copy-button"
                  :aria-label="configCopied ? '配置已复制' : '复制当前配置文件'"
                  :title="configCopied ? '配置已复制' : '复制当前文件'"
                  :disabled="!configText"
                  @click="copyConfig"
                >
                  <PhCheckCircle v-if="configCopied" class="config-copy-success" :size="16" aria-hidden="true" />
                  <PhCopy v-else :size="16" aria-hidden="true" />
                </button>
              </div>
              <pre class="code-view">{{
                config ? configText : "正在读取…"
              }}</pre>
            </article></template
          >
          <template v-else>
            <RuntimeSettingsForm ref="settingsEditor" @import-config="importing=true" @export-config="(id,name)=>previewForm('settings',id,name)"
              :menus="platformPages"
              :settings="state.settings"
              :busy="busy"
              :dirty="state.dirty"
              @save="saveSettings"
              @clear-cache="clearCache"
              @test="testSettings"
              @apply="applySettings"
            />
          </template>
        </template>
      </main>
    </div>
    <div class="workspace-scrollbar" aria-hidden="true"></div>
    </div>
  </div>
  <DraftChangesDialog v-if="showDraftPreview && draftChanges" :changes="draftChanges" :return-focus="draftChangesTrigger" @close="closePreview" />
  <RevisionPreview v-else-if="activePreview" :revision="activePreview" @close="closePreview" />
  <ConfigDialog v-if="state && (importing || rulePreview)" :state="state" :open-form="openImportedForm" :custom-config-enabled="menuEnabled('config')" :stream-page="page === 'streams'" :rules="state?.rules ?? []" :preview="rulePreview" @close="importing=false;rulePreview=undefined" @copy="copyProxyAddress" @raw="keepSource" @saved="imported" />
  <SourceParameterDialog v-if="sourceEditorEntry" :key="sourceEditorEntry.file.name + ':' + sourceEditorEntry.start" :entry="sourceEditorEntry"
    :block="sourceEditorEntry" :form-issue="page === 'rules' ? sourceFormIssue : ''" :from-rule-form="modal === 'rule'" :open-rule-form="openSourceRule" @close="closeSourceEditor" @saved="finishSourceEdit" />
  <SourceParameterDialog v-if="parameterEntry" :entry="parameterEntry" @close="parameterEntry = undefined" @saved="parameterEntry = undefined; sourceSaved()" />
  <div v-if="customConfigOpen" class="modal-backdrop" @mousedown.self="!busy && (customConfigOpen = false)">
    <section class="modal custom-config-modal" role="dialog" aria-modal="true" aria-labelledby="custom-config-title">
      <header class="modal-header">
        <div><h2 id="custom-config-title">{{ customConfigOriginal ? '编辑自定义配置' : '新建自定义配置' }}</h2><p>HTTP 配置使用 .conf，TCP/UDP Stream 配置使用 .stream；保存后需校验并应用。</p></div>
        <button class="icon-button modal-close" aria-label="关闭" :disabled="busy" @click="customConfigOpen = false"><PhX :size="20" /></button>
      </header>
      <form class="modal-body custom-config-form" @submit.prevent="saveCustomConfig">
        <div class="field custom-config-name-field">
          <label for="custom-config-name">文件名</label>
          <input id="custom-config-name" v-model.trim="customConfigForm.name" class="input" required maxlength="80" pattern="custom-[A-Za-z0-9][A-Za-z0-9._-]{0,71}\.(conf|stream)" placeholder="custom-1panel.conf" />
          <span class="field-help">必须以 custom- 开头，并以 .conf 或 .stream 结尾。</span>
        </div>
        <label for="custom-config-content">配置内容</label>
        <ConfigTextEditor ref="configEditor" v-model="customConfigForm.content" />
        <div class="notice warning">自定义指令不会经过结构化安全限制。应用前会执行 nginx -t；校验失败时不会替换当前运行配置。</div>
        <footer class="modal-footer"><button type="button" class="button ghost" :disabled="busy" @click="customConfigOpen = false">取消</button><button type="submit" class="button primary" :disabled="busy">{{ busy ? '保存中…' : '保存为草稿' }}</button></footer>
      </form>
    </section>
  </div>
  <div
    v-if="modal"
    class="modal-backdrop"
    aria-hidden="false"
    @mousedown.self="closeModal"
  >
    <section
      class="modal"
      :class="{ 'rule-modal': modal === 'rule', 'certificate-modal': modal === 'certificate' }"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="`${modal}-title`"
    >
      <header class="modal-header">
        <PhShareNetwork
          v-if="modal === 'rule'"
          class="modal-title-icon"
          :size="34"
          weight="regular"
          aria-hidden="true"
        />
        <div :class="{ 'rule-title-row': modal === 'rule' }">
          <h2 :id="`${modal}-title`">
            {{
              modal === "rule"
                ? editingRule
                  ? "编辑代理规则"
                  : "添加代理规则"
                : "导入 SSL/TLS 证书"
            }}
          </h2>
          <NameConflictNotice v-if="modal === 'rule' && editingRule && (ruleSourceEntry || state?.rules.some(r => r.id === editingRule?.id && r.enabled))"
            :key="editingRule.id" :block="ruleSourceEntry" :rule-id="editingRule.id" />
          <p v-if="modal === 'certificate'">证书链必须与私钥匹配，支持 RSA 和 ECDSA PEM。</p>
        </div>
        <button v-if="modal === 'rule' && sourceReplacement" ref="ruleParametersButton" type="button" class="button ghost small" :disabled="busy" @click="openRuleParameters">参数与源码</button>
        <button type="button" class="icon-button modal-close" aria-label="关闭" @click="closeModal">
          <PhX :size="20" aria-hidden="true" />
        </button>
      </header>
      <div class="modal-body">
        <RuleForm
          v-if="modal === 'rule'"
          ref="ruleForm"
          :source-file="sourceReplacement?.name"
          :rule="editingRule"
          :groups="ruleGroups"
          :initial-group="newRuleGroup"
          :settings="state!.settings"
          :certificates="state!.certificates"
          :upstream-pools="state!.upstream_pools"
          :rate-limits-enabled="menuEnabled('rate-limits')"
          :rate-limit-policies="state!.rate_limit_policies"
          :manage-auth-profiles="menuEnabled('auth-profiles')"
          :auth-profiles="state!.auth_profiles ?? []"
          :busy="busy"
          @save="saveRule"
          @manage-rate-limits="manageRateLimitPolicies"
          @auth-profiles-changed="updateAuthProfiles"
        /><CertificateForm
          @acme="createACME"
          v-else
          :busy="busy"
          @save="importCertificate"
          @cancel="closeModal"
        />
      </div>
      <footer v-if="modal === 'rule'" class="modal-footer rule-editor-footer">
        <button type="button" class="button ghost" :disabled="busy" @click="closeModal">取消</button>
        <button v-if="sourceReplacement" type="submit" form="proxy-rule-form" value="draft" class="button secondary" :disabled="busy">保存为草稿</button>
        <button type="submit" form="proxy-rule-form" class="button primary" :disabled="busy">
          {{ busy ? "处理中…" : "保存并应用" }}
        </button>
      </footer>
    </section>
  </div>
  <Teleport to="body">
    <div
      v-if="rateLimitManagerOpen"
      class="rate-limit-manager-backdrop"
      @mousedown.self="!busy && (rateLimitManagerOpen = false)"
    >
      <section
        class="rate-limit-manager"
        role="dialog"
        aria-modal="true"
        aria-labelledby="rate-limit-manager-title"
      >
        <header class="rate-limit-manager-header">
          <div>
            <h2 id="rate-limit-manager-title">{{ editingRateLimitPolicy ? "编辑限流策略" : "添加限流策略" }}</h2>
            <p v-if="editingRateLimitPolicy">保存并应用后，引用此策略的规则都会使用新设置。</p>
          </div>
          <button
            type="button"
            class="icon-button"
            aria-label="关闭"
            :disabled="busy"
            @click="rateLimitManagerOpen = false"
          >
            <PhX :size="20" aria-hidden="true" />
          </button>
        </header>
        <div class="rate-limit-manager-body">
          <RateLimitPolicyManager
            :policy="editingRateLimitPolicy"
            :busy="busy"
            @save="saveRateLimitPolicy"
            @close="rateLimitManagerOpen = false"
          />
        </div>
      </section>
    </div>
  </Teleport>
  <div v-if="confirmBox.open" class="modal-backdrop" aria-hidden="false">
    <section
      class="modal confirm-modal"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
    >
      <header class="modal-header">
        <div>
          <h2 id="confirm-title">{{ confirmBox.title }}</h2>
          <p>{{ confirmBox.message }}</p>
        </div>
      </header>
      <footer class="modal-footer">
        <button class="button ghost" @click="answerConfirm(false)">取消</button
        ><button class="button danger" @click="answerConfirm(true)">
          确认
        </button>
      </footer>
    </section>
  </div>
  <div class="toast-region" aria-live="assertive">
    <section v-if="conflictNotice" class="toast conflict-notice" aria-label="域名冲突">
      <header><strong>{{ conflictNotice.message }}，有域名冲突</strong>
        <button type="button" aria-label="关闭域名冲突提示" @click="conflictNotice = undefined">×</button>
      </header>
      <ul><li v-for="(warning, index) in conflictNotice.warnings" :key="index">{{ warning }}</li></ul>
    </section>
    <div v-for="item in toasts" :key="item.id" class="toast" :class="item.type">
      <b>{{
        item.type === "success" ? "✓" : item.type === "error" ? "!" : "i"
      }}</b
      ><span>{{ item.message }}</span>
    </div>
  </div>
</template>

<style scoped>
.draft-restored-title { color: var(--text); font-weight: 600; }
.table tr.revision-source-row > td { background: var(--warning-soft); }
.table tr.revision-source-row > td:first-child { box-shadow: inset 3px 0 var(--warning); }
.custom-config-modal { width: min(960px, 100%); }
.custom-config-form { display: grid; gap: 10px; }
.custom-config-form > label { font-weight: 650; }
.custom-config-form .modal-footer { padding: 8px 0 0; }
.admin-logout { width: 100%; justify-content: space-between; margin-bottom: 8px; }
.admin-logout span { max-width: 90px; overflow: hidden; text-overflow: ellipsis; }
.rate-limit-manager-backdrop { position: fixed; inset: 0; z-index: 105; display: grid; place-items: center; padding: 20px; background: rgba(18,25,31,.58); }
.rate-limit-manager { width: min(720px, calc(100vw - 40px)); max-height: calc(100vh - 28px); overflow: hidden; display: flex; flex-direction: column; border: 1px solid var(--line); border-radius: 14px; background: var(--surface-solid); box-shadow: 0 24px 64px rgba(0,0,0,.22); }
.rate-limit-manager-header { min-height: 44px; flex: 0 0 auto; display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 6px 16px; border-bottom: 1px solid var(--line); }
.rate-limit-manager-header > div { min-width: 0; display: flex; align-items: baseline; gap: 14px; }
.rate-limit-manager-header h2 { flex: 0 0 auto; margin: 0; font-size: 16px; line-height: 24px; }
.rate-limit-manager-header p { margin: 0; color: var(--muted); font-size: 13px; }
.rate-limit-manager-header .icon-button { width: 30px; height: 30px; }
.rate-limit-manager-body { min-height: 0; overflow: hidden; }
@media (max-width: 720px) {
  .rate-limit-manager-backdrop { padding: 10px; }
  .rate-limit-manager-header > div { display: grid; gap: 3px; }
  .rate-limit-manager-body { min-height: 0; }
}
</style>
