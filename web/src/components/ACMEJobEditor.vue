<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import ACMEFields from './ACMEFields.vue';
import { request, errorMessage, jsonBody } from '../api';
import type { ACMEInput, ACMEJob, ACMESettings } from '../types';
const props = defineProps<{ id: string }>();
const emit = defineEmits<{ close: []; saved: [job: ACMEJob, retry: boolean] }>();
const dialog = ref<HTMLDialogElement>();
const original = ref<ACMESettings>();
const form = ref<ACMEInput>();
const clearSecrets = ref<string[]>([]);
const loading = ref(true);
const saving = ref(false);
const ready = ref(false);
const error = ref('');
const controller = new AbortController();
let returnFocus: HTMLElement | null = null;
const configured = computed(() => original.value?.configured_secrets.filter(path => path.startsWith('dns_config.') ? form.value?.provider === original.value?.provider : form.value?.ca === original.value?.ca && form.value?.directory_url === original.value?.directory_url) ?? []);
function clear(path: string) { clearSecrets.value = clearSecrets.value.includes(path) ? clearSecrets.value.filter(p => p !== path) : [...clearSecrets.value, path]; }
function close() { if (!saving.value) emit('close'); }
async function load() {
  loading.value = true; error.value = '';
  try {
    const data = await request<ACMESettings>(`/acme/${props.id}/config`, { signal: controller.signal });
    original.value = data;
    const { revision: _revision, configured_secrets: _secrets, ...input } = data;
    form.value = { ...input, domains: [...data.domains], dns_config: { ...data.dns_config }, credentials: { token: '', access_id: '', secret: '', eab_kid: '', eab_hmac: '' } };
  } catch(e) { if (!controller.signal.aborted) error.value = errorMessage(e); }
  finally { loading.value = false; }
}
async function save(retry: boolean) {
  if (!form.value || !original.value || saving.value || !ready.value) return;
  saving.value = true; error.value = '';
  const input = { ...form.value, domains: form.value.domains.flatMap(d => d.split(/[\s,，]+/)).filter(Boolean), dns_config: { ...form.value.dns_config }, credentials: { ...form.value.credentials } };
  try {
    const job = await request<ACMEJob>(`/acme/${props.id}/config`, { method: 'PUT', body: jsonBody({ ...input, revision: original.value.revision, retry, clear_secrets: clearSecrets.value.filter(path => configured.value.includes(path)) }) });
    emit('saved', job, retry);
  } catch(e) { error.value = errorMessage(e); }
  finally { saving.value = false; }
}
onMounted(() => {
  returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  dialog.value?.showModal(); void load();
});
onBeforeUnmount(() => { controller.abort(); dialog.value?.close(); returnFocus?.focus(); });
</script>
<template>
  <dialog ref="dialog" class="modal acme-editor-dialog" aria-labelledby="acme-editor-title" @cancel.prevent="close">
    <header class="modal-header"><div><h2 id="acme-editor-title">编辑申请配置</h2><p>保留原任务和证书引用；签发或部署失败时继续使用旧证书。</p></div><button type="button" class="icon-button" :disabled="saving" aria-label="关闭编辑配置" @click="close">×</button></header>
    <div class="modal-body">
      <p v-if="loading" role="status">正在加载配置…</p>
      <p v-if="error" class="notice danger" role="alert">{{ error }} <button v-if="!form && !loading" type="button" class="button ghost small" @click="load">重新加载</button></p>
      <form v-if="form" id="acme-edit-form" class="form-grid" @submit.prevent="save(false)">
        <div class="field full"><label for="acme-edit-name">名称</label><input id="acme-edit-name" v-model.trim="form.name" class="input" maxlength="80" :disabled="saving" /></div>
        <ACMEFields v-model="form" :busy="saving" :configured-secrets="configured" :clear-secrets="clearSecrets" @ready="ready = $event" @clear-secret="clear" />
        <p class="field-help full">保存配置按原计划检查，不立即提交新的申请。“保存并重试”会启用自动续期并立即排队执行。更换 CA、邮箱或 EAB 后会重新注册 ACME 账户。</p>
      </form>
    </div>
    <footer class="modal-footer">
      <button type="button" class="button ghost" :disabled="saving" @click="close">取消</button>
      <button type="submit" form="acme-edit-form" class="button" :disabled="loading || saving || !ready">保存配置</button>
      <button type="submit" form="acme-edit-form" class="button primary" :disabled="loading || saving || !ready" @click.prevent="dialog?.querySelector<HTMLFormElement>('form')?.reportValidity() && save(true)">{{ saving ? '提交中…' : '保存并重试' }}</button>
    </footer>
  </dialog>
</template>
<style scoped>
.acme-editor-dialog { position: fixed; inset: 0; margin: auto; padding: 0; width: min(740px, calc(100vw - 28px)); max-height: calc(100dvh - 32px); color: var(--text); }
.acme-editor-dialog:not([open]) { display: none; }
.acme-editor-dialog::backdrop { background: rgba(18,25,31,.58); }
.modal-footer { flex-wrap: wrap; }
@media (max-width: 560px) { .acme-editor-dialog .field { grid-template-columns: 1fr; } .acme-editor-dialog .field > .field-help { grid-column: 1; } }
</style>
