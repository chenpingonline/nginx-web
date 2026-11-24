<script setup lang="ts">
import { ref, useId, watch } from "vue";
import AppSelect from "./AppSelect.vue";
import { comparisonOperators, fileOperators, type splitCondition } from "../sourceCondition";

const props = defineProps<{ condition: NonNullable<ReturnType<typeof splitCondition>>; disabled: boolean }>();
const value = defineModel<string>({ required: true });
const uid = useId();
const left = ref(props.condition.left);
const operator = ref(props.condition.operator);
const right = ref(props.condition.right);
watch([left, operator, right], () => {
  value.value = props.condition.format(left.value, operator.value, right.value);
});
</script>

<template>
  <template v-if="condition.kind !== 'file'">
    <label :for="uid + '-var'">条件变量</label>
    <input :id="uid + '-var'" v-model="left" class="input" required spellcheck="false" :disabled="disabled" />
  </template>
  <template v-if="condition.kind !== 'variable'">
    <label :for="uid + '-op'">{{ condition.kind === "file" ? "文件判断" : "比较运算符" }}</label>
    <AppSelect :id="uid + '-op'" v-model="operator" :disabled="disabled">
      <option v-for="op in (condition.kind === 'file' ? fileOperators : comparisonOperators)" :key="op" :value="op">{{ op }}</option>
    </AppSelect>
    <label :for="uid + '-rhs'">{{ condition.kind === "file" ? "检查路径" : "比较值" }}</label>
    <input :id="uid + '-rhs'" v-model="right" class="input" :class="{ 'condition-wide': condition.kind === 'comparison' }" required spellcheck="false" :disabled="disabled" />
  </template>
</template>

<style scoped>
label { justify-self: end; text-align: right; font-weight: 600; color: var(--text); }
.input { min-width: 0; width: 100%; }
@media (min-width: 701px) { .condition-wide { grid-column: 2 / -1; } }
</style>
