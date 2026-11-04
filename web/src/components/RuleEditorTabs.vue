<script setup lang="ts">
const props = defineProps<{
  modelValue: string;
  prefix: string;
  tabs: { id: string; label: string; count?: number }[];
}>();
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();
function navigate(event: KeyboardEvent, index: number) {
  let target = index;
  if (event.key === 'ArrowRight') target = (index + 1) % props.tabs.length;
  else if (event.key === 'ArrowLeft') target = (index + props.tabs.length - 1) % props.tabs.length;
  else if (event.key === 'Home') target = 0;
  else if (event.key === 'End') target = props.tabs.length - 1;
  else return;
  event.preventDefault();
  const tab = props.tabs[target];
  if (!tab) return;
  emit('update:modelValue', tab.id);
  document.getElementById(`${props.prefix}-tab-${tab.id}`)?.focus();
}
</script>

<template>
  <nav class="rule-editor-tabs" role="tablist" aria-label="规则设置分类">
    <button v-for="(tab, index) in tabs" :id="`${prefix}-tab-${tab.id}`" :key="tab.id"
      type="button" role="tab" :aria-selected="modelValue === tab.id"
      :aria-controls="`${prefix}-panel-${tab.id}`" :tabindex="modelValue === tab.id ? 0 : -1"
      @click="emit('update:modelValue', tab.id)" @keydown="navigate($event, index)">
      {{ tab.label }}<span v-if="tab.count" class="rule-tab-count">{{ tab.count }}</span>
    </button>
  </nav>
</template>
