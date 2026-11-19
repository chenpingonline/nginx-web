<script setup lang="ts">
import { ref, watch } from "vue";

const props = defineProps<{ limit: number; busy: boolean }>();
const emit = defineEmits<{ save: [limit: number] }>();
const value = ref(props.limit);
watch(() => props.limit, limit => { value.value = limit; });
function submit() {
  if (Number.isInteger(value.value) && value.value >= 1 && value.value <= 100) {
    emit("save", value.value);
  }
}
</script>

<template>
  <form class="card-body retention-form" @submit.prevent="submit">
    <div class="field">
      <label for="revision-limit">配置历史保留数量</label>
      <div class="retention-controls">
        <input id="revision-limit" v-model.number="value" class="input" type="number" min="1" max="100" required :disabled="busy" />
        <button class="button secondary small" type="submit" :disabled="busy || value === limit">{{ busy ? '处理中…' : '保存为草稿' }}</button>
      </div>
      <span class="field-help">可保留 1–100 个版本。保存后需应用草稿；应用成功时按上限清理最早的历史版本。</span>
    </div>
  </form>
</template>

<style scoped>
.retention-form { border-bottom: 1px solid var(--line); }
.retention-controls { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.retention-controls .input { width: 110px; }
</style>
