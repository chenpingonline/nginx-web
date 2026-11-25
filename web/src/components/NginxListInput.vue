<script setup lang="ts">
import { ref } from "vue";
import { formatDirectiveArguments, parseDirectiveArguments } from "../nginxConfig";
const props = defineProps<{ modelValue: string[]; directive: string; repeated?: boolean; legacyRaw?: boolean; multiline?: boolean }>();
const emit = defineEmits<{ "update:modelValue": [string[]] }>();
const draft = ref<string>();
function update(event: Event) {
  const field = event.target as HTMLInputElement | HTMLTextAreaElement;
  draft.value = field.value;
  try {
    const args = parseDirectiveArguments(field.value, props.directive, props.repeated);
    field.setCustomValidity("");
    emit("update:modelValue", args);
  } catch (error) {
    field.setCustomValidity((error as Error).message);
  }
}
function finish(event: FocusEvent) {
  if ((event.target as HTMLInputElement).validity.valid) draft.value = undefined;
}
</script>
<template>
  <component :is="repeated || multiline ? 'textarea' : 'input'" :value="draft ?? (legacyRaw ? modelValue.join(' ') : formatDirectiveArguments(modelValue, repeated ? '\n' : ' '))" @input="update" @blur="finish" />
</template>
