<script setup lang="ts">
import { ref } from "vue";
import { formatQuantity, parseQuantity, type QuantityUnit } from "../nginxQuantity";
const props = defineProps<{ modelValue?: number; unit: QuantityUnit; min?: string | number; max?: string | number; optional?: boolean }>();
const emit = defineEmits<{ "update:modelValue": [number]; clear: [] }>();
const draft = ref<string>();
function update(event: Event) {
  const el = event.target as HTMLInputElement;
  draft.value = el.value;
  el.setCustomValidity("");
  if (props.optional && !el.value.trim()) {
    emit("clear");
    return;
  }
  try {
    const value = parseQuantity(el.value, props.unit);
    if (props.min != null && value < Number(props.min)) throw Error("最小值为 " + formatQuantity(Number(props.min), props.unit));
    if (props.max != null && value > Number(props.max)) throw Error("最大值为 " + formatQuantity(Number(props.max), props.unit));
    emit("update:modelValue", value);
  } catch (error) {
    el.setCustomValidity((error as Error).message);
  }
}
function finish(event: FocusEvent) {
  if ((event.target as HTMLInputElement).validity.valid) draft.value = undefined;
}
</script>
<template>
  <input type="text" :value="draft ?? (modelValue == null ? '' : formatQuantity(modelValue, unit))" @input="update" @blur="finish" />
</template>
