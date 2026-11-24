<script setup lang="ts">
import { reactive, ref, useId, watchEffect } from "vue";
import AppSelect from "./AppSelect.vue";
import { formatSourceProxyTarget, type SourceProxyTarget } from "../sourceProxyTarget";

const props = defineProps<{ target: SourceProxyTarget; disabled: boolean }>();
const value = defineModel<string>({required: true});
const target = reactive({...props.target});
const field = ref<HTMLInputElement>();
const id = useId();

watchEffect(() => {
  try {
    value.value = formatSourceProxyTarget(target);
    field.value?.setCustomValidity("");
  } catch (error) {
    field.value?.setCustomValidity((error as Error).message);
  }
});
</script>

<template>
  <div class="source-proxy">
    <template v-if="target.scheme">
      <label :for="id + '-scheme'">后端协议</label>
      <AppSelect :id="id + '-scheme'" :model-value="target.scheme.toLowerCase()" :disabled="disabled"
        @update:model-value="scheme => target.scheme = scheme">
        <option value="http">HTTP</option><option value="https">HTTPS</option>
      </AppSelect>
    </template>
    <template v-if="target.kind === 'unix'">
      <label :for="id">Unix socket 路径</label>
      <input :id="id" ref="field" v-model="target.path" class="input" required spellcheck="false" :disabled="disabled" />
      <template v-if="!target.stream">
        <label :for="id + '-uri'">上游 URI</label>
        <input :id="id + '-uri'" v-model="target.uri" class="input" spellcheck="false" :disabled="disabled" />
      </template>
    </template>
    <template v-else>
      <label :for="id">上游表达式</label>
      <input :id="id" ref="field" v-model="target.expression" class="input" required spellcheck="false" :disabled="disabled" />
    </template>
  </div>
</template>

<style scoped>
.source-proxy {
  display: grid;
  grid-template-columns: 8em minmax(0, 1fr) max-content minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
label { justify-self: end; text-align: right; font-weight: 600; color: var(--text); }
label:nth-child(3) { margin-left: 28px; }
.input { min-width: 0; width: 100%; }
.input:last-child:nth-child(2), .input:nth-child(6) { grid-column: 2 / -1; }
@media (max-width: 700px) {
  .source-proxy { grid-template-columns: 7em minmax(0, 1fr); }
  label:nth-child(3) { margin-left: 0; }
}
</style>
