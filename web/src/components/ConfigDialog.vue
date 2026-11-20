<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { importBlocks, parseConfig } from "../nginxConfig";
import { request, jsonBody, errorMessage } from "../api";
import type { ProxyRule } from "../types";
import AppSelect from "./AppSelect.vue";

const props = defineProps<{
  rules: ProxyRule[];
  preview?: { content: string; name: string; draft: boolean };
}>();
const emit = defineEmits<{
  close: [];
  saved: [];
  raw: [source: string];
  copy: [value: string, label: string];
}>();
const dialog = ref<HTMLDialogElement>();
const source = ref(""),
  target = ref(""),
  error = ref(""),
  working = ref(false);
const generated = ref<string[]>([]);
let candidate: ReturnType<typeof importBlocks> | undefined;
const blocks = computed(() => {
  try {
    return parseConfig(source.value);
  } catch {
    return [];
  }
});
const isLocation = computed(() => blocks.value[0]?.words[0] === "location");
const dependencies = computed(() => {
  const content = props.preview?.content || "";
  const references =
    content.match(
      /\$connection_upgrade\b|\$fnproxy_(?!rule_id\b)\w+|\bfnproxy_(?:up|req|conn|cache|stream)_\w+|(?:ssl_certificate(?:_key)?|auth_basic_user_file|root|alias|access_log)\s+[^;]+|http:\/\/unix:[^";]+/g,
    ) ?? [];
  return [...new Set(references)];
});
watch([source, target], () => {
  candidate = undefined;
  generated.value = [];
  error.value = "";
});
onMounted(() => dialog.value?.showModal());
async function run(save: boolean) {
  working.value = true;
  error.value = "";
  try {
    if (save && candidate && generated.value.length) {
      await request("/config-import/save", {
        method: "POST",
        body: jsonBody({ ...candidate, expected: generated.value }),
      });
      emit("saved");
    } else {
      candidate = importBlocks(source.value, target.value);
      generated.value = await request<string[]>("/config-import/preview", {
        method: "POST",
        body: jsonBody(candidate),
      });
    }
  } catch (e) {
    error.value = errorMessage(e);
    generated.value = [];
    candidate = undefined;
  } finally {
    working.value = false;
  }
}
</script>
<template>
  <dialog
    ref="dialog"
    class="config-dialog"
    aria-labelledby="config-dialog-title"
    @cancel.prevent="!working && emit('close')"
  >
    <h2 id="config-dialog-title">
      {{
        preview
          ? (preview.draft ? "草稿配置" : "规则配置") + " · " + preview.name
          : "从 Nginx 配置导入"
      }}
    </h2>
    <template v-if="preview">
      <pre>{{ preview.content }}</pre>
      <details v-if="dependencies.length">
        <summary>迁移时需要的依赖（{{ dependencies.length }}）</summary>
        <ul>
          <li v-for="value in dependencies" :key="value">
            <code>{{ value }}</code>
          </li>
        </ul>
      </details>
    </template>
    <template v-else>
      <label for="import-source">Nginx 配置块</label>
      <textarea
        id="import-source"
        v-model="source"
        autofocus
        class="textarea"
        :disabled="working"
        spellcheck="false"
        maxlength="1048576"
        placeholder="粘贴 server 或 location 块"
      ></textarea>
      <div v-if="isLocation" class="import-target">
        <label for="import-target">目标代理</label
        ><AppSelect
          id="import-target"
          v-model="target"
          aria-label="目标代理"
          :disabled="working"
          ><option value="">请选择</option>
          <option v-for="rule in rules" :key="rule.id" :value="rule.id">
            {{ rule.name }}
          </option></AppSelect
        >
      </div>
      <p v-if="error" role="alert">
        {{ error
        }}<template v-if="isLocation"
          >。按原文保存时请粘贴包含该路径的完整 server 块。</template
        >
      </p>
      <template v-if="generated.length"
        ><h3>将保存的配置</h3>
        <p>请核对下方完整配置。应用会生成请求头、超时及日志相关指令。</p>
        <pre v-for="(content, i) in generated" :key="i">{{ content }}</pre>
      </template>
    </template>
    <footer>
      <button class="button ghost" :disabled="working" @click="emit('close')">
        关闭
      </button>
      <button
        v-if="preview"
        class="button primary"
        @click="emit('copy', preview.content, 'Nginx 配置')"
      >
        复制配置
      </button>
      <template v-else>
        <button
          v-if="!isLocation"
          class="button secondary"
          :disabled="working || !source.trim()"
          @click="emit('raw', source)"
        >
          保留为自定义配置
        </button>
        <button
          class="button primary"
          :disabled="working || !source.trim()"
          @click="run(generated.length > 0)"
        >
          {{
            working
              ? "处理中…"
              : generated.length
                ? "确认保存为草稿"
                : "预览导入"
          }}
        </button>
      </template>
    </footer>
  </dialog>
</template>
<style scoped>
.config-dialog {
  box-sizing: border-box;
  width: min(800px, calc(100% - 24px));
  max-height: 90dvh;
  overflow: auto;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 18px;
}
.config-dialog::backdrop {
  background: #0008;
}
h2 {
  margin: 0 0 14px;
  font-size: 18px;
}
textarea {
  display: block;
  width: 100%;
  min-height: 160px;
  margin: 6px 0;
  font: 13px monospace;
}
pre {
  overflow: auto;
  max-height: 42vh;
  white-space: pre;
  background: var(--frame);
  padding: 12px;
}
code,
p {
  overflow-wrap: anywhere;
}
footer,
.import-target {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 14px;
}
footer {
  justify-content: flex-end;
}
</style>
