<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { editParameters, parameterGroups, sourceBlock, replaceSourceRange, type SourceEntry } from "../nginxConfig";
import { errorMessage, jsonBody, request } from "../api";

import { validateSourceConditions } from "../sourceCondition";
import NameConflictNotice from "./NameConflictNotice.vue";
import ConfigTextEditor from "./ConfigTextEditor.vue";
import SourceParameterField from "./SourceParameterField.vue";

import type { CustomConfig } from "../types";

const props = defineProps<{ entry: { file: CustomConfig; title: string; start?: number }; local?: boolean; create?: boolean; block?: SourceEntry; formIssue?: string; fromRuleForm?: boolean; openRuleForm?: (block: SourceEntry) => Promise<void>; baseFile?: CustomConfig }>();
const emit = defineEmits<{ close: []; saved: [apply: boolean]; edited: [content: string] }>();
const error = ref("");
const parseError = ref("");
const name = ref(props.entry.file.name);
const extension = name.value.endsWith(".stream") ? "stream" : "conf";
const namePattern = String.raw`custom-[A-Za-z0-9][A-Za-z0-9._\-]{0,71}\.${extension}`;
const file = props.entry.file;
// A list row owns its server block. Parse failures can only be repaired at file scope.
const range = props.block && !props.block.error ? sourceBlock(props.block) : undefined;
const original = range?.raw ?? file.content;
const source = ref(original);
const mode = ref<"parameters" | "source">(props.block?.error ? "source" : "parameters");
const groups = ref<ReturnType<typeof parameterGroups>>([]);
function readParameters() {
  groups.value = props.block
    ? parameterGroups({file: {...file, content: source.value}})
    : parameterGroups(props.entry);
}
try {
  readParameters();
} catch (e) {
  parseError.value = errorMessage(e);
  if (props.block) mode.value = "source";
}
const fields = computed(() => groups.value.flatMap(group => group.fields));
const dirty = computed(() => source.value !== original || (mode.value === "parameters" && fields.value.some(f => f.value !== f.original)));
const canSave = computed(() => props.create || props.local || !!props.baseFile || dirty.value);
const dialog = ref<HTMLDialogElement>();
const saving = ref(false);
const confirmReturn = ref(false);
function requestClose() {
  if (saving.value) return;
  if (props.fromRuleForm && dirty.value) {
    confirmReturn.value = true;
    return;
  }
  emit("close");
}
onMounted(() => dialog.value?.showModal());

function parameterText() {
  validateSourceConditions(fields.value, !!props.create || !!props.local || !!props.baseFile);
  return editParameters(source.value, fields.value);
}
function switchMode(next: typeof mode.value) {
  if (mode.value === next) return;
  error.value = "";
  try {
    if (next === "parameters") {
      readParameters();
    } else {
      source.value = parameterText();
    }
    parseError.value = "";
    mode.value = next;
  } catch (e) {
    error.value = errorMessage(e);
  }
}
async function openForm() {
  if (!props.block || !props.openRuleForm) return;
  saving.value = true;
  try { await props.openRuleForm(props.block); }
  catch (e) { error.value = errorMessage(e); }
  finally { saving.value = false; }
}
async function save(apply = false) {
  if (confirmReturn.value || saving.value || (!apply && !canSave.value)) return;
  saving.value = true;
  error.value = "";
  try {
    const raw = mode.value === "source" ? source.value : parameterText();
    const content = range ? replaceSourceRange(range.source, raw).content : raw;
    if (props.local) { emit("edited", content); return; }
    if (props.create) {
      await request("/custom-configs", { method: "POST", body: jsonBody({name: name.value.trim(), content}) });
    } else {
      await request("/custom-configs/" + encodeURIComponent(file.name), {
        method: "PUT", body: jsonBody({ name: file.name, content, expected_content: props.baseFile?.content ?? file.content }),
      });
    }
    emit("saved", apply);
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <dialog ref="dialog" :class="{'block-dialog': block}" aria-labelledby="source-param-title" @cancel.prevent="confirmReturn ? (confirmReturn = false) : requestClose()">
    <form @submit.prevent="save(($event as SubmitEvent).submitter?.getAttribute('value') === 'apply')">
      <header>
        <h2 id="source-param-title">{{ create ? "导入配置参数" : (block ? "编辑 · " : "编辑配置参数 · ") + entry.title }}</h2>
        <div v-if="confirmReturn" class="actions">
          <span role="status">当前修改尚未保存。</span>
          <button type="button" class="button secondary" @click="confirmReturn = false">继续编辑</button>
          <button type="button" class="button ghost" @click="emit('close')">放弃修改并返回</button>
        </div>
        <div v-else class="actions">
          <button v-if="block && !block.error && openRuleForm && !fromRuleForm" type="button" class="button ghost small" :disabled="dirty || saving" @click="openForm">打开规则表单</button>
          <button type="button" class="button ghost" :disabled="saving" @click="requestClose">{{ fromRuleForm ? "返回规则表单" : "取消" }}</button>
          <button class="button" :class="block ? 'secondary' : 'primary'" :disabled="saving || (mode === 'parameters' && !!parseError) || !canSave">{{ local ? "使用此配置" : "保存为草稿" }}</button>
          <button v-if="block" class="button primary" value="apply" :disabled="saving">保存并应用</button>
        </div>
      </header>
      <section>
        <template v-if="block">
          <div class="mode-bar">
            <div class="actions" role="group" aria-label="编辑方式">
              <button type="button" class="button small" :aria-pressed="mode === 'parameters'" :disabled="saving" @click="switchMode('parameters')">编辑参数</button>
              <button type="button" class="button small" :aria-pressed="mode === 'source'" :disabled="saving" @click="switchMode('source')">源码</button>
            </div>
            <span>{{ file.name }} · 第 {{ block.line }} 行</span>
            <NameConflictNotice v-if="!block.error" :block="block" />
          </div>
          <p v-if="formIssue" role="status">{{ formIssue }}</p>
          <template v-if="mode === 'source'">
            <label for="custom-config-content">{{ range ? '配置块源码' : '配置文件源码' }}</label>
            <ConfigTextEditor v-model="source" :readonly="saving" />
          </template>
        </template>
        <div v-if="create" class="file-name">
          <label for="source-param-name">文件名</label>
          <input id="source-param-name" v-model.trim="name" class="input" :pattern="namePattern" :title="'文件名须以 custom- 开头、以 .' + extension + ' 结尾'" required :disabled="saving" />
        </div>
        <template v-if="mode === 'parameters'">
          <p v-if="!error && !parseError && !groups.length">此配置块没有可在表单中编辑的参数，请编辑源码。</p>
          <fieldset v-for="group in groups" :key="group.fields[0].start">
            <legend>{{ group.title }} · 第 {{ group.line + (range ? block!.line - 1 : 0) }} 行</legend>
            <SourceParameterField v-for="field in group.fields" :key="field.start"
              :field="field" v-model="field.value" :stream="entry.file.name.endsWith('.stream')" :disabled="saving || field.readonly" />
          </fieldset>
        </template>
        <p v-if="error || parseError" role="alert">{{ error || parseError }}</p>
      </section>
    </form>
  </dialog>
</template>

<style scoped>
dialog { padding: 0; width: min(920px, 94vw); max-width: 94vw; border: 1px solid var(--line); border-radius: 14px; background: var(--surface, Canvas); color: inherit; }
.block-dialog { margin-block-start: 7.5dvh; }
dialog::backdrop { background: rgb(0 0 0 / 45%); }
form { display: flex; flex-direction: column; max-height: 85dvh; }
header, .actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
header { padding: 14px 18px; justify-content: space-between; border-bottom: 1px solid var(--line); }
h2 { margin: 0; font-size: 17px; overflow-wrap: anywhere; }
section { padding: 16px 18px; overflow: auto; }
.mode-bar { position: relative; display: flex; align-items: center; flex-wrap: wrap; gap: 8px 16px; margin-bottom: 14px; }
.mode-bar span { color: var(--text-muted); overflow-wrap: anywhere; }
.mode-bar [aria-pressed=true] { border-color: var(--accent); color: var(--accent); }
.file-name { display: grid; grid-template-columns: 8em minmax(0, 1fr); gap: 8px; align-items: center; margin-bottom: 16px; }
.file-name label { justify-self: end; font-weight: 600; color: var(--text); }
@media (max-width: 700px) { .file-name { grid-template-columns: 7em minmax(0, 1fr); } }
fieldset { min-width: 0; margin: 0 0 16px; padding: 0; border: 0; }
fieldset:last-child { margin-bottom: 0; }
legend { font-weight: 600; margin-bottom: 10px; overflow-wrap: anywhere; }
[role=alert] { color: var(--danger); }
</style>
