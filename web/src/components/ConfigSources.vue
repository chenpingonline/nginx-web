<script setup lang="ts">
import { ref } from "vue";
import { PhCaretDown, PhCaretRight } from "@phosphor-icons/vue";
import type { SourceEntry } from "../nginxConfig";

import SourceParameterDialog from "./SourceParameterDialog.vue";

const editing = ref<SourceEntry>();
defineProps<{ entries: SourceEntry[] }>();
defineEmits<{ edit: [entry: SourceEntry]; saved: [] }>();

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
      <div class="proxy-group-summary"><h3>自定义配置</h3></div>
    </header>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>名称</th><th>入口</th><th>转发服务</th><th>来源</th><th aria-label="操作"></th></tr>
        </thead>
        <tbody>
          <template v-for="item in entries" :key="keyOf(item)">
            <tr>
              <td class="rule-name">{{ item.title }}</td>
              <td>
                <div v-for="value in item.entrances" :key="value">{{ value }}</div>
                <span v-if="!item.entrances.length">—</span>
              </td>
              <td>
                <div v-for="target in item.targets" :key="target">{{ target }}</div>
                <span v-if="!item.targets.length">—</span>
              </td>
              <td>{{ item.file.name }} · 第 {{ item.line }} 行</td>
              <td>
                <div class="table-actions">
                  <button v-if="item.listeners.length || item.routes.length" type="button" class="button ghost small"
                    :aria-expanded="expanded.has(keyOf(item))" @click="toggle(item)">
                    <PhCaretDown v-if="expanded.has(keyOf(item))" aria-hidden="true" />
                    <PhCaretRight v-else aria-hidden="true" />
                    {{ expanded.has(keyOf(item)) ? '收起详情' : '展开详情' }}
                  </button>
                  <button v-if="!item.error" type="button" class="button small" @click="editing = item">编辑参数</button>
                  <button type="button" class="button ghost small" @click="$emit('edit', item)">编辑源文件</button>
                </div>
              </td>
            </tr>
            <tr v-if="item.error || expanded.has(keyOf(item))">
              <td colspan="5">
                <span v-if="item.error" class="badge warning">{{ item.error }}</span>
                <dl v-else class="source-details">
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
