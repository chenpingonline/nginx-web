<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { request, errorMessage } from '../api';
import type { ACMEDiagnosticCheck } from '../types';
const props = defineProps<{ id: string; name: string }>();
const emit = defineEmits<{ close: [] }>();
const dialog = ref<HTMLDialogElement>();
const checks = ref<ACMEDiagnosticCheck[]>([]);
const loading = ref(false);
const error = ref('');
let controller: AbortController | undefined;
let focus: HTMLElement | null = null;
let stopped = false;
async function run() {
  if (loading.value) return;
  controller = new AbortController(); loading.value = true; error.value = '';
  try {
    const result = await request<ACMEDiagnosticCheck[]>(`/acme/${props.id}/diagnose`, { method: 'POST', signal: controller.signal });
    if (!stopped) checks.value = result;
  } catch (e) { if (!stopped) error.value = errorMessage(e); }
  finally { if (!stopped) loading.value = false; }
}
onMounted(() => { focus = document.activeElement instanceof HTMLElement ? document.activeElement : null; dialog.value?.showModal(); void run(); });
onBeforeUnmount(() => { stopped = true; controller?.abort(); dialog.value?.close(); focus?.focus(); });
</script>
<template>
  <dialog ref="dialog" class="modal acme-diagnostics" aria-labelledby="acme-diagnostics-title" @cancel.prevent="emit('close')">
    <header class="modal-header"><div><h2 id="acme-diagnostics-title">诊断配置</h2><p>{{ name }} · 使用已保存配置</p></div><button type="button" class="icon-button" aria-label="关闭诊断" autofocus @click="emit('close')">×</button></header>
    <div class="modal-body">
      <p class="field-help">检查凭据配置、CA 连通性与 TXT 查询通路。不会提交申请、创建 DNS 记录或部署证书；API 权限与 EAB 是否有效需结合实际申请日志确认。</p>
      <p v-if="loading" role="status">正在诊断，最多约 20 秒…</p>
      <p v-if="error" class="notice danger" role="alert">{{ error }}</p>
      <ol class="diagnostic-checks" aria-label="诊断结果">
        <li v-for="(check, index) in checks" :key="index"><div class="check-heading"><strong>{{ check.name }}</strong><span class="badge" :class="check.status === 'error' ? 'danger' : check.status === 'ok' ? 'success' : 'info'">{{ check.status === 'error' ? '异常' : check.status === 'ok' ? '通过' : '需结合申请状态' }}</span></div><p>{{ check.message }}</p><p class="field-help"><span v-if="check.domain">{{ check.domain }} · </span><span v-if="check.resolver">DNS {{ check.resolver }} · </span>{{ check.elapsed_ms }} ms</p></li>
      </ol>
    </div>
    <footer class="modal-footer"><button type="button" class="button" :disabled="loading" @click="run">重新诊断</button><button type="button" class="button ghost" @click="emit('close')">关闭</button></footer>
  </dialog>
</template>
<style scoped>
.acme-diagnostics { position: fixed; inset: 0; margin: auto; padding: 0; width: min(760px, calc(100vw - 28px)); max-height: calc(100dvh - 32px); color: var(--text); }
.acme-diagnostics:not([open]) { display: none; }
.diagnostic-checks { list-style: none; margin: 12px 0 0; padding: 0; }
.diagnostic-checks li { padding: 12px 0; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
.check-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.diagnostic-checks p { margin: 8px 0 0; }
</style>
