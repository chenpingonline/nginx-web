<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { editParameters, parameterGroups } from "../nginxConfig";
import { errorMessage, jsonBody, request } from "../api";

import { validateSourceConditions } from "../sourceCondition";
import SourceParameterField from "./SourceParameterField.vue";

import type { CustomConfig } from "../types";

const props = defineProps<{ entry: { file: CustomConfig; title: string; start?: number }; local?: boolean; create?: boolean; baseFile?: CustomConfig }>();
const emit = defineEmits<{ close: []; saved: []; edited: [content: string] }>();
const error = ref("");
const parseError = ref("");
const name = ref(props.entry.file.name);
const extension = name.value.endsWith(".stream") ? "stream" : "conf";
const namePattern = String.raw`custom-[A-Za-z0-9][A-Za-z0-9._\-]{0,71}\.${extension}`;
const groups = ref<ReturnType<typeof parameterGroups>>([]);
try {
  groups.value = parameterGroups(props.entry);
} catch (e) {
  parseError.value = errorMessage(e);
}
const fields = computed(() => groups.value.flatMap(group => group.fields));
const canSave = computed(() => props.create || props.local || !!props.baseFile || fields.value.some(f => f.value !== f.original));
const dialog = ref<HTMLDialogElement>();
const saving = ref(false);
onMounted(() => dialog.value?.showModal());

async function save() {
  if (saving.value || parseError.value || !canSave.value) return;
  saving.value = true;
  error.value = "";
  try {
    const file = props.entry.file;
    validateSourceConditions(fields.value, !!props.create || !!props.local || !!props.baseFile);
    const content = editParameters(file.content, fields.value);
    if (props.local) { emit("edited", content); return; }
    if (props.create) {
      await request("/custom-configs", { method: "POST", body: jsonBody({name: name.value.trim(), content}) });
    } else {
      await request("/custom-configs/" + encodeURIComponent(file.name), {
        method: "PUT", body: jsonBody({ name: file.name, content, expected_content: props.baseFile?.content ?? file.content }),
      });
    }
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
        <h2 id="source-param-title">{{ create ? "导入配置参数" : "编辑配置参数 · " + entry.title }}</h2>
        <div class="actions">
          <button type="button" class="button ghost" :disabled="saving" @click="emit('close')">取消</button>
          <button class="button primary" :disabled="saving || !!parseError || !canSave">{{ local ? "使用此配置" : "保存为草稿" }}</button>
        </div>
      </header>
      <section>
        <div v-if="create" class="file-name">
          <label for="source-param-name">文件名</label>
          <input id="source-param-name" v-model.trim="name" class="input" :pattern="namePattern" :title="'文件名须以 custom- 开头、以 .' + extension + ' 结尾'" required :disabled="saving" />
        </div>
        <p v-if="!error && !parseError && !groups.length">此配置块没有可在表单中编辑的参数，请使用“编辑源文件”。</p>
        <fieldset v-for="group in groups" :key="group.fields[0].start">
          <legend>{{ group.title }} · 第 {{ group.line }} 行</legend>
          <SourceParameterField v-for="field in group.fields" :key="field.start"
            :field="field" v-model="field.value" :stream="entry.file.name.endsWith('.stream')" :disabled="saving || field.readonly" />
        </fieldset>
        <p v-if="parseError || error" role="alert">{{ parseError || error }}</p>
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
.file-name { display: grid; grid-template-columns: 8em minmax(0, 1fr); gap: 8px; align-items: center; margin-bottom: 16px; }
.file-name label { justify-self: end; font-weight: 600; color: var(--text); }
@media (max-width: 700px) { .file-name { grid-template-columns: 7em minmax(0, 1fr); } }
fieldset { min-width: 0; margin: 0 0 16px; padding: 0; border: 0; }
fieldset:last-child { margin-bottom: 0; }
legend { font-weight: 600; margin-bottom: 10px; overflow-wrap: anywhere; }
[role=alert] { color: var(--danger); }
</style>
