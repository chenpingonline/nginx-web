<script setup lang="ts">
import { PhSpinnerGap } from "@phosphor-icons/vue";
const props = defineProps<{
  enabled: boolean;
  pendingEnabled?: boolean;
  disabled: boolean;
  label: string;
}>();
const emit = defineEmits<{ change: [enabled: boolean] }>();
function change(event: Event) {
  const input = event.target as HTMLInputElement;
  if (props.disabled || props.pendingEnabled !== undefined) {
    input.checked = props.pendingEnabled ?? props.enabled;
    return;
  }
  emit('change', input.checked);
}
</script>

<template>
  <div class="rule-status-control" :aria-busy="pendingEnabled !== undefined">
    <label class="switch" :class="{ 'is-pending': pendingEnabled !== undefined }">
      <input type="checkbox" :checked="pendingEnabled ?? enabled"
        :disabled="disabled || pendingEnabled !== undefined" :aria-label="label" @change="change" />
      <span></span>
    </label>
    <span v-if="pendingEnabled !== undefined" class="rule-toggle-progress" role="status">
      {{ pendingEnabled ? '正在启用…' : '正在停用…' }}
      <PhSpinnerGap class="rule-toggle-spinner" :size="14" aria-hidden="true" />
    </span>
  </div>
</template>

<style scoped>
.rule-status-control { display: flex; align-items: center; gap: 8px; }
.switch { flex-shrink: 0; }
.rule-toggle-progress { display: inline-flex; align-items: center; gap: 5px; color: var(--text-muted); font-size: 12px; white-space: nowrap; }
.rule-toggle-spinner { flex-shrink: 0; animation: rule-toggle-spin .8s linear infinite; }
.switch input:focus-visible + span { outline: 2px solid var(--accent); outline-offset: 3px; }
@keyframes rule-toggle-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .rule-toggle-spinner { animation: none; } }
</style>
