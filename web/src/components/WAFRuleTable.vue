<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { request, errorMessage } from "../api";
import AppSelect from "./AppSelect.vue";
import type { WAFInstance } from "../types";

interface Rule { id: number; message: string; category: string; level: number }
const props = defineProps<{ draft: WAFInstance; siteName: (id: string) => string }>();
const emit = defineEmits<{ change: [site: string, ids: number[]] }>();
const rules = ref<Rule[]>([]);
const site = ref("");
const query = ref("");
const filter = ref("all");
const loading = ref(false);
const error = ref("");
watch(() => props.draft.sites.slice(), sites => {
  if (!sites.includes(site.value)) site.value = sites[0] ?? "";
}, { immediate: true });
const excluded = computed(() => new Set(props.draft.exclusions?.[site.value]));
const descriptions: Record<number,string> = {
  911100: "请求方法限制", 941100: "跨站脚本检测", 942100: "SQL 注入检测",
};
const categories: Record<string,string> = {
  "METHOD-ENFORCEMENT":"请求方法", "SCANNER-DETECTION":"扫描器",
  "PROTOCOL-ENFORCEMENT":"协议规范", "PROTOCOL-ATTACK":"协议攻击", "MULTIPART-ATTACK":"分段请求",
  "APPLICATION-ATTACK-LFI":"本地文件包含", "APPLICATION-ATTACK-RFI":"远程文件包含",
  "APPLICATION-ATTACK-RCE":"命令执行", "APPLICATION-ATTACK-XSS":"跨站脚本",
  "APPLICATION-ATTACK-GENERIC":"通用攻击", "APPLICATION-ATTACK-JAVA":"Java 应用", "APPLICATION-ATTACK-PHP":"PHP 应用",
  "APPLICATION-ATTACK-SQLI":"SQL 注入", "APPLICATION-ATTACK-SESSION-FIXATION":"会话固定",
};
function status(rule: Rule) {
  if (excluded.value.has(rule.id)) return "本站已排除";
  return rule.level === 1 ? "默认生效" : "当前级别未启用";
}
const filtered = computed(() => {
  const term = query.value.trim().toLowerCase();
  return rules.value.filter(rule =>
    (filter.value === "all" || status(rule) === filter.value) &&
    [rule.id, rule.message, rule.category, categories[rule.category], descriptions[rule.id]]
      .join(" ").toLowerCase().includes(term));
});
function toggle(id: number) {
  if (!site.value) return;
  const ids = new Set(excluded.value);
  if (ids.has(id)) ids.delete(id);
  else ids.add(id);
  emit("change",site.value,[...ids].sort((a,b) => a-b));
}
async function load() {
  loading.value = true;
  error.value = "";
  try { rules.value = await request<Rule[]>("/waf/rules"); }
  catch (e) { error.value = errorMessage(e); }
  finally { loading.value = false; }
}
onMounted(load);
</script>
<template>
  <div class="tools">
    <AppSelect v-model="site" class="select" aria-label="规则所属站点" :disabled="!draft.sites.length">
      <option v-if="!draft.sites.length" value="">未绑定站点</option>
      <option v-for="id in draft.sites" :key="id" :value="id">{{ siteName(id) }}</option>
    </AppSelect>
    <input v-model="query" class="input" type="search" aria-label="搜索规则" placeholder="搜索规则 ID、说明或分类">
    <AppSelect v-model="filter" class="select" aria-label="规则状态">
      <option value="all">全部状态</option>
      <option v-for="label in ['默认生效','本站已排除','当前级别未启用']" :key="label" :value="label">{{ label }}</option>
    </AppSelect>
  </div>
  <div class="rules-scroll">
    <table class="table">
      <thead><tr><th>规则 ID</th><th>说明</th><th>级别</th><th>本站状态</th><th>操作</th></tr></thead>
      <tbody>
        <tr v-for="rule in filtered" :key="rule.id">
          <td>{{ rule.id }}</td>
          <td :title="rule.message">{{ descriptions[rule.id] || rule.message }}</td>
          <td>PL{{ rule.level }}</td>
          <td :class="{ exception: excluded.has(rule.id) }">{{ status(rule) }}</td>
          <td><button type="button" class="button ghost small" :disabled="!site" @click="toggle(rule.id)">{{ excluded.has(rule.id) ? '恢复默认' : '排除' }}</button></td>
        </tr>
        <tr v-if="!filtered.length"><td colspan="5">{{ loading ? '正在读取规则…' : error || '没有匹配的规则' }}
          <button v-if="error" type="button" class="button ghost small" @click="load">重试</button>
        </td></tr>
      </tbody>
    </table>
  </div>
</template>
<style scoped>
.tools { display:grid; grid-template-columns:minmax(110px,1fr) minmax(150px,2fr) minmax(120px,1fr); gap:8px; padding:3px; margin:8px -3px; }
.tools > * {min-width:0}
.rules-scroll {overflow:auto; height:min(270px,38vh); border-block:1px solid var(--line)}
.table {font-size:13px}
.table th {position:sticky;top:0;z-index:1;background:var(--surface-solid)}
.table td {padding:5px 8px}
.table td:not(:nth-child(2)) {white-space:nowrap}
.table td:nth-child(2) {overflow-wrap:anywhere}
.exception {color:var(--warning)}
@media(max-width:600px){.tools{grid-template-columns:1fr 1fr}.tools input{grid-column:1/-1;grid-row:2}.table{min-width:480px}}
</style>
