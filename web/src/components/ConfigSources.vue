<script setup lang="ts">
import type { SourceEntry } from "../nginxConfig";
defineProps<{ entries: SourceEntry[] }>();
defineEmits<{ edit: [entry: SourceEntry] }>();
</script>
<template>
  <article v-if="entries.length" class="card">
    <h3>自定义配置</h3>
    <div
      v-for="item in entries"
      :key="item.file.name + ':' + item.start"
      class="source-row"
    >
      <div>
        <strong>{{ item.title }}</strong>
        <div>{{ item.detail }}</div>
        <small>{{ item.file.name }} · 第 {{ item.line }} 行</small>
      </div>
      <button class="button ghost small" @click="$emit('edit', item)">
        编辑源文件
      </button>
    </div>
  </article>
</template>
<style scoped>
.source-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--line);
  overflow-wrap: anywhere;
}
.source-row button {
  flex: none;
}
</style>
