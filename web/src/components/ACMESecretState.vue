<script setup lang="ts">
defineProps<{ path: string; configured: boolean; clearing: boolean; busy: boolean }>();
const emit = defineEmits<{ clear: [path: string] }>();
</script>
<template>
  <div v-if="configured" class="field-help secret-state">
    <span>{{ clearing ? '保存后将清除此凭据' : '已配置。留空保留，填写新值替换。' }}</span>
    <label><input type="checkbox" :checked="clearing" :disabled="busy" @change="emit('clear', path)" />清除此凭据</label>
  </div>
</template>
<style scoped>
.secret-state { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 14px; }
.secret-state label { display: inline-flex; align-items: center; gap: 5px; color: var(--danger); }
</style>
