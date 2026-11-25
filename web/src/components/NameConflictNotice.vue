<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { PhWarning } from "@phosphor-icons/vue";
import { errorMessage, jsonBody, request } from "../api";
import { parseConfig, type SourceEntry } from "../nginxConfig";

const props = defineProps<{block?: SourceEntry; ruleId?: string}>();
const messages = ref<string[]>([]);
const error = ref("");
const title = computed(() => messages.value.length ? "已保存配置中的域名冲突" : error.value ? "域名检查失败" : "");
const details = ref<HTMLDetailsElement>();
function closeDetails() {
  if (details.value) details.value.open = false;
}
onMounted(async () => {
  try {
    if (props.block) {
      const {file, line, start} = props.block;
      const occurrence = parseConfig(file.content).filter(node =>
        node.words[0] === "server" && node.line === line && node.start <= start).length;
      messages.value = await request<string[]>("/custom-configs/" + encodeURIComponent(file.name) + "/name-warnings", {
        method: "POST", body: jsonBody({line, occurrence, expected_content: file.content}),
      });
    } else if (props.ruleId) {
      messages.value = await request<string[]>("/rules/" + encodeURIComponent(props.ruleId) + "/name-warnings");
    }
  } catch (e) {
    error.value = "当前草稿无法完成域名冲突检查。" + errorMessage(e);
  }
});
</script>

<template>
  <div class="name-check-slot" role="status">
    <details v-if="title" ref="details" @focusout="closeDetails" @keydown.esc.stop.prevent="closeDetails">
      <summary :title="title" :aria-label="title"><PhWarning :size="18" weight="fill" /></summary>
      <span class="name-check-result notice warning">
        <strong>{{ title }}</strong>
        <span v-for="(message, i) in messages" :key="i">{{ message }}</span>
        <span v-if="error">{{ error }}</span>
      </span>
    </details>
  </div>
</template>

<style scoped>
.name-check-slot { display: inline-block; width: 24px; height: 24px; flex: none; vertical-align: middle; }
summary { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 4px; cursor: pointer; color: var(--warning); }
summary::-webkit-details-marker { display: none; }
summary:focus-visible { outline: 2px solid var(--accent); }
.name-check-result { display: grid; gap: 4px; position: absolute; z-index: 5; top: 100%; left: 0; right: 0; max-height: 30vh; overflow: auto; overflow-wrap: anywhere; margin: 0; background: var(--surface-solid); box-shadow: var(--shadow); }
</style>
