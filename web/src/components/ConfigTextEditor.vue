<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";

const content = defineModel<string>({ required: true });
const editor = ref<HTMLTextAreaElement>();
const scrollTop = ref(0);
const height = ref(0);
const rowHeight = 22;
const count = computed(() => content.value.split("\n").length);
const first = computed(() => Math.floor(scrollTop.value / rowHeight));
const visible = computed(() => Math.max(0, Math.min(count.value - first.value, Math.ceil(height.value / rowHeight) + 1)));
let observer: ResizeObserver | undefined;

onMounted(() => {
  const el = editor.value!;
  observer = new ResizeObserver(() => { height.value = el.clientHeight; });
  observer.observe(el);
});
onBeforeUnmount(() => observer?.disconnect());

function focusLine(line: number) {
  const el = editor.value!;
  // Textarea selection offsets refer to its LF-normalized UTF-16 value.
  const lines = el.value.split("\n");
  const index = Math.min(Math.max(line - 1, 0), lines.length - 1);
  let offset = 0;
  for (let i = 0; i < index; i++) offset += lines[i].length + 1;
  offset += lines[index].match(/^[ \t]*/)![0].length;
  el.focus();
  el.setSelectionRange(offset, offset);
  el.scrollTop = Math.max(0, index * rowHeight - el.clientHeight / 3);
  scrollTop.value = el.scrollTop;
}
defineExpose({ focusLine });
</script>

<template>
  <div class="config-text-editor" :style="{ '--row-height': rowHeight + 'px' }">
    <div class="gutter" aria-hidden="true" :style="{ width: String(count).length + 'ch' }">
      <div :style="{ transform: 'translateY(' + (8 - scrollTop % rowHeight) + 'px)' }">
        <div v-for="n in visible" :key="first + n">{{ first + n }}</div>
      </div>
    </div>
    <textarea id="custom-config-content" ref="editor" v-model="content" class="textarea" required
      spellcheck="false" wrap="off" maxlength="1048576" @scroll="scrollTop = editor!.scrollTop"></textarea>
  </div>
</template>

<style scoped>
.config-text-editor { display: flex; min-width: 0; overflow: hidden; border: 1px solid var(--line-strong); border-radius: var(--control-radius); font: 14px/var(--row-height) ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.config-text-editor:focus-within { border-color: var(--accent); }
.gutter { position: relative; overflow: hidden; box-sizing: content-box; flex: none; padding: 0 8px; border-right: 1px solid var(--line-strong); color: var(--text-muted); text-align: right; user-select: none; }
.gutter > div { position: absolute; right: 8px; top: 0; }
.config-text-editor textarea { flex: 1; min-width: 0; min-height: min(52vh, 520px); padding: 8px; border: 0; border-radius: 0; margin: 0; font: inherit; tab-size: 4; resize: vertical; white-space: pre; }
</style>
