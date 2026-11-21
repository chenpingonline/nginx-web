<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { editParameters, parameterGroups } from "../nginxConfig";
import { errorMessage, jsonBody, request } from "../api";

import SourceParameterField from "./SourceParameterField.vue";

import type { CustomConfig } from "../types";

const props = defineProps<{ entry: { file: CustomConfig; title: string; start?: number } }>();
const emit = defineEmits<{ close: []; saved: [] }>();
const error = ref("");
const groups = ref<ReturnType<typeof parameterGroups>>([]);
try {
  groups.value = parameterGroups(props.entry);
} catch (e) {
  error.value = errorMessage(e);
}
const fields = computed(() => groups.value.flatMap(group => group.fields));
const changed = computed(() => fields.value.some(f => f.value !== f.original));
const dialog = ref<HTMLDialogElement>();
const saving = ref(false);
onMounted(() => dialog.value?.showModal());

async function save() {
  if (saving.value || !changed.value) return;
  saving.value = true;
  error.value = "";
  try {
    const file = props.entry.file;
    const content = editParameters(file.content, fields.value);
    await request("/custom-configs/" + encodeURIComponent(file.name), {
      method: "PUT", body: jsonBody({ name: file.name, content, expected_content: file.content }),
    });
    emit("saved");
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <dialog ref="dialog" aria-labelledby="source-param-title" @cancel.prevent="!saving && emit('close')">
    <form @submit.prevent="save">
      <header>
        <h2 id="source-param-title">编辑配置参数 · {{ entry.title }}</h2>
        <div class="actions">
          <button type="button" class="button ghost" :disabled="saving" @click="emit('close')">取消</button>
          <button class="button primary" :disabled="saving || !changed">保存为草稿</button>
        </div>
      </header>
      <section>
        <p v-if="!error && !groups.length">此配置块没有可在表单中编辑的参数，请使用“编辑源文件”。</p>
        <fieldset v-for="group in groups" :key="group.fields[0].start">
          <legend>{{ group.title }} · 第 {{ group.line }} 行</legend>
          <SourceParameterField v-for="field in group.fields" :key="field.start"
            :field="field" v-model="field.value" :stream="entry.file.name.endsWith('.stream')" :disabled="saving || field.readonly" />
        </fieldset>
        <p v-if="error" role="alert">{{ error }}</p>
      </section>
    </form>
  </dialog>
</template>

<style scoped>
dialog { padding: 0; width: min(920px, 94vw); max-width: 94vw; border: 1px solid var(--line); border-radius: 14px; background: var(--surface, Canvas); color: inherit; }
dialog::backdrop { background: rgb(0 0 0 / 45%); }
form { display: flex; flex-direction: column; max-height: 85dvh; }
header, .actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
header { padding: 14px 18px; justify-content: space-between; border-bottom: 1px solid var(--line); }
h2 { margin: 0; font-size: 17px; overflow-wrap: anywhere; }
section { padding: 16px 18px; overflow: auto; }
fieldset { min-width: 0; margin: 0 0 16px; padding: 0; border: 0; }
fieldset:last-child { margin-bottom: 0; }
legend { font-weight: 600; margin-bottom: 10px; overflow-wrap: anywhere; }
[role=alert] { color: var(--danger); }
</style>
