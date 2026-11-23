<script setup lang="ts">
import { ref } from "vue";
import { PhCaretDown, PhCaretRight, PhCopy } from "@phosphor-icons/vue";
import type { SourceEntry } from "../nginxConfig";

import RuleStatusSwitch from "./RuleStatusSwitch.vue";
import ProxyAddress from "./ProxyAddress.vue";
import SourceParameterDialog from "./SourceParameterDialog.vue";

const editing = ref<SourceEntry>();
defineProps<{ entries: SourceEntry[]; ruleEditor?: boolean; streamEditor?: boolean; busy?: boolean; pending?: SourceEntry }>();
defineEmits<{ disable: [entry: SourceEntry]; remove: [entry: SourceEntry]; preview: [entry: SourceEntry]; rule: [entry: SourceEntry]; edit: [entry: SourceEntry]; saved: []; copy: [value: string, label: string] }>();

const collapsed = ref(false);
function canOpenEntrance(value: string) {
  // Nginx also accepts wildcard, regex, variable and catch-all server names.
  if (!/^https?:\/\/(?:[a-z0-9-]+(?:\.[a-z0-9-]+)*\.?|\[[0-9a-f:]+\]):\d+$/i.test(value)) return false;
  try { return Boolean(new URL(value).hostname); } catch { return false; }
}
const expanded = ref(new Set<string>());
const keyOf = (entry: SourceEntry) => entry.file.name + ":" + entry.start;
function toggle(entry: SourceEntry) {
  const key = keyOf(entry);
  if (expanded.value.has(key)) expanded.value.delete(key);
  else expanded.value.add(key);
}
</script>

<template>
  <article v-if="entries.length" class="card proxy-rule-card grouped-rule-card">
    <header class="proxy-group-heading">
      <div class="proxy-group-summary">
        <button type="button" class="button ghost small group-toggle" :aria-expanded="!collapsed" aria-label="切换自定义配置分组" @click="collapsed = !collapsed">
          <component :is="collapsed ? PhCaretRight : PhCaretDown" :size="15" aria-hidden="true" />{{ collapsed ? '展开' : '收起' }}
        </button>
        <h3>自定义配置</h3><span class="proxy-group-count">{{ entries.length }} 条规则</span>
      </div>
    </header>
    <div v-show="!collapsed" class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>状态</th><th>名称</th><th>入口</th><th>转发服务</th><th aria-label="操作"></th></tr>
        </thead>
        <tbody>
          <template v-for="item in entries" :key="keyOf(item)">
            <tr>
              <td><RuleStatusSwitch v-if="ruleEditor && !item.error" :enabled="true" :pending-enabled="pending === item ? false : undefined" :disabled="!!busy" :label="'停用规则 ' + item.title" @change="$emit('disable', item)" /><span v-else class="badge" :class="item.error ? 'warning' : 'success'">{{ item.error ? '解析异常' : '已配置' }}</span></td>
              <td class="rule-name">{{ item.title }}</td>
              <td>
                <ProxyAddress v-for="value in item.entrances" :key="value" :value="value" entry :openable="canOpenEntrance(value)" @copy="(address, label) => $emit('copy', address, label)" />
                <span v-if="!item.entrances.length">—</span>
              </td>
              <td>
                <ProxyAddress v-for="target in item.targets" :key="target" :value="target" @copy="(address, label) => $emit('copy', address, label)" />
                <span v-if="!item.targets.length">—</span>
              </td>
              <td>
                <div class="table-actions">
                  <button v-if="ruleEditor && !item.error" type="button" class="icon-button rule-config-copy" aria-label="复制 Nginx 配置" @click="$emit('preview', item)"><PhCopy :size="16" aria-hidden="true" /></button>
                  <button v-if="!ruleEditor && !item.error" type="button" class="button ghost small"
                    :aria-expanded="expanded.has(keyOf(item))" @click="toggle(item)">
                    <PhCaretDown v-if="expanded.has(keyOf(item))" aria-hidden="true" />
                    <PhCaretRight v-else aria-hidden="true" />
                    {{ expanded.has(keyOf(item)) ? '收起详情' : '展开详情' }}
                  </button>
                  <button v-if="(ruleEditor || streamEditor) && !item.error" type="button" class="button ghost small" @click="$emit('rule', item)">编辑</button>
                  <button v-if="!ruleEditor && !item.error" type="button" class="button small" @click="editing = item">编辑参数</button>
                  <button v-if="ruleEditor && !item.error" type="button" class="button danger-ghost small" :disabled="busy" @click="$emit('remove', item)">删除</button>
                </div>
              </td>
            </tr>
            <tr v-if="item.error || expanded.has(keyOf(item))">
              <td colspan="5">
                <span v-if="item.error" class="badge warning">{{ item.error }}</span>
                <dl class="source-details">
                  <dt>来源</dt><dd>{{ item.file.name }} · 第 {{ item.line }} 行 <button type="button" class="button ghost small" @click="$emit('edit', item)">编辑源文件</button></dd>
                  <template v-if="item.listeners.length"><dt>监听</dt><dd><div v-for="(value, i) in item.listeners" :key="i">{{ value }}</div></dd></template>
                  <template v-if="item.routes.length"><dt>路径</dt><dd><div v-for="(value, i) in item.routes" :key="i">{{ value }}</div></dd></template>
                </dl>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </article>
  <SourceParameterDialog v-if="editing" :entry="editing" @close="editing = undefined" @saved="editing = undefined; $emit('saved')" />
</template>

<style scoped>
.source-details { display: grid; grid-template-columns: max-content 1fr; gap: 6px 16px; margin: 0; }
.source-details dt { font-weight: 600; }
.source-details dd { margin: 0; overflow-wrap: anywhere; }
.table-actions .button { white-space: nowrap; }
</style>
