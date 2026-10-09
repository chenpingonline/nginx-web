<script setup lang="ts">
import { computed, nextTick, ref, onMounted, onBeforeUnmount } from 'vue';
import ACMEJobDetails from './ACMEJobDetails.vue';
import ACMEJobEditor from './ACMEJobEditor.vue';
import ACMEJobLogs from './ACMEJobLogs.vue';
import ACMEJobDiagnostics from './ACMEJobDiagnostics.vue';
import { request, errorMessage } from '../api';
import type { ACMEJob, CertificateMeta } from '../types';
const props = defineProps<{ certificates: CertificateMeta[] }>();
const emit = defineEmits<{ changed: [] }>();
const jobs = ref<ACMEJob[]>([]);
const error = ref('');
let timer: ReturnType<typeof setTimeout>;
let stopped = false;
let controller: AbortController | undefined;
let fingerprint = '';
type ActionFeedback = { type: 'success' | 'error'; message: string };
const actionPendingId = ref('');
const actionFeedback = ref<Record<string, ActionFeedback>>({});
const feedbackTimers = new Map<string, number>();

function setActionFeedback(id: string, feedback: ActionFeedback) {
  actionFeedback.value = { ...actionFeedback.value, [id]: feedback };
  const previous = feedbackTimers.get(id);
  if (previous) window.clearTimeout(previous);
  feedbackTimers.set(id, window.setTimeout(() => {
    const next = { ...actionFeedback.value };
    delete next[id];
    actionFeedback.value = next;
    feedbackTimers.delete(id);
  }, 2800));
}

async function performAction(id: string, action: string) {
  if (actionPendingId.value) return;
  actionPendingId.value = id;
  try {
    await request(`/acme/${id}${action === 'delete' ? '' : '/' + action}`, { method: action === 'delete' ? 'DELETE' : 'POST' });
    const messages: Record<string, string> = {
      retry: '申请 / 续期任务已提交',
      retry_deploy: '部署任务已提交，复用已签发证书',
      pause: '自动续期已暂停',
      resume: '自动续期已启用',
      delete: 'ACME 任务已移除',
    };
    jobs.value = jobs.value.map(job => {
      if (job.id !== id) return job;
      if (action === 'retry' || action === 'retry_deploy') return { ...job, status: 'queued', message: '等待执行' };
      if (action === 'pause') return { ...job, enabled: false, message: job.status === 'failed' ? job.message : '自动续期已暂停' };
      if (action === 'resume') return { ...job, enabled: true, message: job.status === 'failed' ? job.message : '自动续期已启用' };
      return job;
    });
    setActionFeedback(id, { type: 'success', message: messages[action] || '任务已更新' });
    emit('changed');
  } catch (e) {
    setActionFeedback(id, { type: 'error', message: errorMessage(e) });
  } finally {
    actionPendingId.value = '';
  }
}

const selectedId = ref('');
const editorId = ref('');
const logsId = ref('');
const diagnosticsId = ref('');
const diagnosticsName = computed(() => jobs.value.find(j => j.id === diagnosticsId.value)?.name ?? '');
const editMessage = ref('');
const editError = ref('');
const logsName = computed(() => jobs.value.find(j => j.id === logsId.value)?.name ?? '');
function openEditor(id: string) { detail.value?.close(); editorId.value = id; }
function openLogs(id: string) { detail.value?.close(); logsId.value = id; }
function openDiagnostics(id: string) { detail.value?.close(); diagnosticsId.value = id; }
async function showEdit(id: string) {
  try {
    jobs.value = await request<ACMEJob[]>('/acme');
    const job = jobs.value.find(j => j.certificate_id === id);
    if (job) openEditor(job.id);
    else { await showDetails(id); editError.value = '此证书未关联 ACME 申请配置，请创建新的申请任务或重新导入证书。'; }
  } catch(e) { error.value = errorMessage(e); }
}
function saved(job: ACMEJob, retry: boolean) {
  jobs.value = jobs.value.map(j => j.id === job.id ? job : j);
  editorId.value = '';
  setActionFeedback(job.id, { type: 'success', message: retry ? '配置已保存，任务已提交' : '配置已保存，将按原计划检查' });
  editMessage.value = retry ? '配置已保存，任务已提交；原证书继续使用。' : '配置已保存，将按原计划检查。';
  emit('changed');
}

const detail = ref<HTMLDialogElement>();
let returnFocus: HTMLElement | null = null;
const selected = computed(() => props.certificates.find(c => c.id === selectedId.value));
const pendingJobs = computed(() => jobs.value.filter(j => !props.certificates.some(c => c.id === j.certificate_id)));
const selectedJobs = computed(() => jobs.value.filter(j => j.certificate_id === selectedId.value));
async function showDetails(id: string) {
  editError.value = ''; editMessage.value = '';
  if (!detail.value?.open) returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  selectedId.value = id;
  await nextTick();
  detail.value?.showModal();
}
function closeDetails() { detail.value?.close(); selectedId.value = ''; returnFocus?.focus(); }
defineExpose({ showDetails, showEdit });
const date = (value: string) => new Date(value).toLocaleString();
async function load() {
  controller = new AbortController();
  try {
    jobs.value = await request<ACMEJob[]>('/acme', { signal: controller.signal });
    error.value = '';
    const next = JSON.stringify(jobs.value.map(j => [j.certificate_id, j.not_after, j.status]));
    if (fingerprint && next !== fingerprint) emit('changed');
    fingerprint = next;
  } catch (e) { if (!stopped) error.value = errorMessage(e); }
  finally { if (!stopped) timer = setTimeout(() => void load(), 3000); }
}
onMounted(() => void load());
onBeforeUnmount(() => {
  stopped = true;
  clearTimeout(timer);
  controller?.abort();
  feedbackTimers.forEach(value => window.clearTimeout(value));
  feedbackTimers.clear();
});
</script>
<template>
  <p v-if="editMessage" class="notice success" role="status">{{ editMessage }}</p>
  <article v-if="pendingJobs.length || error" class="card acme-jobs">
    <header class="acme-heading"><h3>ACME 自动申请与续期</h3><span class="field-help">{{ pendingJobs.length }} 个任务 · 自动刷新</span></header>
    <p v-if="error" class="notice warning" role="alert">{{ error }}</p>
    <ACMEJobDetails v-for="job in pendingJobs" :key="job.id" :job="job" :pending="actionPendingId === job.id" :feedback="actionFeedback[job.id]" @action="performAction" @edit="openEditor" @logs="openLogs" @diagnose="openDiagnostics" />
  </article>
  <dialog ref="detail" class="modal acme-detail-dialog" aria-labelledby="acme-detail-title" @cancel.prevent="closeDetails">
    <header class="modal-header"><div><h2 id="acme-detail-title">证书详情</h2><p>{{ selected?.name }}</p></div><button type="button" class="icon-button" aria-label="关闭证书详情" autofocus @click="closeDetails">×</button></header>
    <div v-if="selected" class="modal-body">
      <dl class="certificate-details">
        <dt>域名 / IP</dt><dd>{{ [...(selected.dns_names ?? []), ...(selected.ip_addresses ?? [])].join('、') || '—' }}</dd>
        <dt>主体</dt><dd>{{ selected.subject || '—' }}</dd>
        <dt>有效期</dt><dd>{{ date(selected.not_before) }} ～ {{ date(selected.not_after) }}</dd>
        <dt>序列号</dt><dd>{{ selected.serial_number }}</dd>
        <dt>指纹</dt><dd>{{ selected.fingerprint }}</dd>
      </dl>
      <p v-if="editMessage" class="notice success" role="status">{{ editMessage }}</p>
      <p v-if="editError" class="notice danger" role="alert">{{ editError }}</p>
      <h3>ACME 自动申请与续期</h3>
      <p v-if="error" class="notice warning" role="alert">{{ error }}</p>
      <ACMEJobDetails v-for="job in selectedJobs" :key="job.id" :job="job" :pending="actionPendingId === job.id" :feedback="actionFeedback[job.id]" @action="performAction" @edit="openEditor" @logs="openLogs" @diagnose="openDiagnostics" />
      <p v-if="!selectedJobs.length && !error" class="field-help">此证书未关联 ACME 自动续期任务。</p>
    </div>
    <footer class="modal-footer">
      <button type="button" class="button ghost" @click="closeDetails">关闭</button>
      <button v-if="selectedJobs.length" type="button" class="button primary" :disabled="selectedJobs.some(j => j.status === 'running')" @click="showEdit(selectedId)">编辑申请配置</button>
    </footer>
  </dialog>
  <ACMEJobEditor v-if="editorId" :key="editorId" :id="editorId" @close="editorId = ''" @saved="saved" />
  <ACMEJobLogs v-if="logsId" :key="logsId" :id="logsId" :name="logsName" @close="logsId = ''" />
  <ACMEJobDiagnostics v-if="diagnosticsId" :key="diagnosticsId" :id="diagnosticsId" :name="diagnosticsName" @close="diagnosticsId = ''" />
</template>
<style scoped>
.acme-jobs { padding: 16px; margin-bottom: 16px; }
.acme-heading, .acme-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.acme-heading h3 { margin: 0; font-size: 16px; }
.acme-heading { justify-content: space-between; }
.acme-job { padding-top: 14px; margin-top: 14px; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
.acme-message { font-size: 14px; margin: 8px 0; }
.acme-actions { margin-top: 10px; }
.acme-detail-dialog { position: fixed; inset: 0; margin: auto; padding: 0; width: min(680px, calc(100vw - 32px)); max-height: calc(100dvh - 48px); color: var(--text); }
.acme-detail-dialog:not([open]) { display: none; }
.acme-detail-dialog::backdrop { background: rgba(18,25,31,.58); }
.certificate-details { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 10px 14px; margin: 0; }
.certificate-details dt { color: var(--text-muted); }
.certificate-details dd { margin: 0; overflow-wrap: anywhere; }
</style>
