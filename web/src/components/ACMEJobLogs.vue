<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import AppSelect from './AppSelect.vue';
import { request, errorMessage } from '../api';
import type { ACMERun } from '../types';
const props = defineProps<{ id: string; name: string }>();
const emit = defineEmits<{ close: [] }>();
const dialog = ref<HTMLDialogElement>();
const entries = ref<HTMLElement>();
const runs = ref<ACMERun[]>([]);
const selectedId = ref('');
const following = ref(true);
const error = ref('');
const loading = ref(true);
let stopped = false;
let timer: ReturnType<typeof setTimeout>;
let controller: AbortController | undefined;
let returnFocus: HTMLElement | null = null;
const kinds: Record<string, string> = { issue: '首次申请', renew: '续期', reissue: '重新签发', deploy: '重试部署', renewal_check: '续期检查' };
const statuses: Record<string, string> = { running: '执行中', ready: '成功', failed: '失败', interrupted: '中断' };
const date = (time: string) => new Date(time).toLocaleString();
const selected = computed(() => runs.value.find(run => run.id === selectedId.value) ?? runs.value.at(-1));
const duration = computed(() => {
  if (!selected.value) return '';
  const seconds = Math.max(0, Math.round((new Date(selected.value.finished_at ?? Date.now()).getTime() - new Date(selected.value.started_at).getTime()) / 1000));
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
});
function choose(id: string) { selectedId.value = id; following.value = id === runs.value.at(-1)?.id; }
async function load() {
  controller = new AbortController();
  try {
    const data = await request<ACMERun[]>(`/acme/${props.id}/logs`, { signal: controller.signal });
    if (stopped) return;
    runs.value = data; error.value = '';
    if (following.value) selectedId.value = data.at(-1)?.id ?? '';
    await nextTick();
    if (following.value && entries.value) entries.value.scrollTop = entries.value.scrollHeight;
  } catch(e) { if (!stopped) error.value = errorMessage(e); }
  finally { loading.value = false; if (!stopped) timer = setTimeout(() => void load(), 3000); }
}
function exportLogs() {
  const text = runs.value.map(run => `${date(run.started_at)} ${kinds[run.kind]} ${statuses[run.status]}${run.omitted_entries ? `（省略 ${run.omitted_entries} 条较早日志）` : ''}\n${run.entries.map(entry => `${date(entry.time)} [${entry.level}] ${entry.message}${entry.code ? ` (${entry.code})` : ''}${entry.hint ? `；建议：${entry.hint}` : ''}${entry.detail ? `；域名：${entry.detail.fqdn || entry.detail.domain || '—'}；DNS：${entry.detail.resolver || '—'}；第 ${entry.detail.attempt || 0} 次；耗时 ${entry.detail.elapsed_ms} ms；结果 ${entry.detail.result}` : ''}`).join('\n')}`).join('\n\n');
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }));
  const link = document.createElement('a'); link.href = url; link.download = `acme-${props.id}.txt`; link.click(); URL.revokeObjectURL(url);
}
onMounted(() => { returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null; dialog.value?.showModal(); void load(); });
onBeforeUnmount(() => { stopped = true; clearTimeout(timer); controller?.abort(); dialog.value?.close(); returnFocus?.focus(); });
</script>
<template>
  <dialog ref="dialog" class="modal acme-logs-dialog" aria-labelledby="acme-logs-title" @cancel.prevent="emit('close')">
    <header class="modal-header"><div><h2 id="acme-logs-title">申请日志</h2><p>{{ name }} · 保留最近 20 次执行，每次最多 200 条 · 自动刷新</p></div><button type="button" class="icon-button" aria-label="关闭申请日志" autofocus @click="emit('close')">×</button></header>
    <div class="modal-body">
      <p v-if="error" class="notice danger" role="alert">{{ error }}</p>
      <p v-if="loading && !runs.length" role="status">正在加载日志…</p>
      <p v-else-if="!runs.length" class="field-help">暂无执行记录。升级前的申请过程无法补录，下一次申请、续期或检查后将在此显示。</p>
      <template v-if="selected">
        <label for="acme-run">执行记录</label>
        <AppSelect id="acme-run" class="select" :model-value="selected.id" @update:model-value="choose"><option v-for="run in [...runs].reverse()" :key="run.id" :value="run.id">{{ date(run.started_at) }} · {{ kinds[run.kind] }} · {{ statuses[run.status] }}</option></AppSelect>
        <div class="log-summary"><span>耗时：{{ duration }}{{ selected.finished_at ? '' : ' · 执行中' }}</span><label class="checkbox-row"><input v-model="following" type="checkbox" />跟随最新日志</label></div>
        <p v-if="selected.omitted_entries" class="field-help">已省略 {{ selected.omitted_entries }} 条较早日志，保留开始和最新记录。</p>
        <ol ref="entries" class="log-entries" aria-label="执行阶段日志" @wheel="following = false">
          <li v-for="(entry, index) in selected.entries" :key="index" :class="entry.level"><time>{{ date(entry.time) }}</time><div><strong>{{ entry.message }}</strong><small v-if="entry.code">错误类别：{{ entry.code }}</small><p v-if="entry.hint">建议：{{ entry.hint }}</p><p v-if="entry.detail" class="log-detail"><span v-if="entry.detail.fqdn || entry.detail.domain">域名：{{ entry.detail.fqdn || entry.detail.domain }}</span><span v-if="entry.detail.resolver">DNS：{{ entry.detail.resolver }}</span><span v-if="entry.detail.attempt">第 {{ entry.detail.attempt }} 次检查</span><span>耗时 {{ entry.detail.elapsed_ms }} ms</span></p></div></li>
        </ol>
      </template>
    </div>
    <footer class="modal-footer"><button type="button" class="button" :disabled="!runs.length" @click="exportLogs">导出日志</button><button type="button" class="button ghost" @click="emit('close')">关闭</button></footer>
  </dialog>
</template>
<style scoped>
.acme-logs-dialog { position: fixed; inset: 0; margin: auto; padding: 0; width: min(820px, calc(100vw - 28px)); max-height: calc(100dvh - 32px); color: var(--text); }
.acme-logs-dialog:not([open]) { display: none; }
.acme-logs-dialog::backdrop { background: rgba(18,25,31,.58); }
.log-summary { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 8px; margin-block: 12px; font-size: 13px; color: var(--text-muted); }
.log-entries { list-style: none; padding: 0; margin: 0; max-height: 48dvh; overflow: auto; border: 1px solid var(--line); border-radius: 8px; background: var(--surface-soft); }
.log-entries li { display: grid; grid-template-columns: 160px minmax(0, 1fr); gap: 12px; padding: 10px 12px; border-bottom: 1px solid var(--line); font-size: 13px; overflow-wrap: anywhere; }
.log-entries li:last-child { border-bottom: 0; }
.log-entries time, .log-entries small { color: var(--text-muted); }
.log-entries strong { font-weight: 550; }
.log-entries small { display: block; margin-top: 4px; }
.log-entries p { margin: 5px 0 0; }
.log-detail { display: flex; flex-wrap: wrap; gap: 4px 12px; color: var(--text-muted); }
.log-entries .error { color: var(--danger); }
.log-entries .success { color: var(--accent-dark); }
.log-entries .warning { color: var(--danger); }
@media (max-width: 560px) { .log-entries li { grid-template-columns: 1fr; gap: 4px; } }
</style>
