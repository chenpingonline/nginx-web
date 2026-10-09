<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import type { ACMEJob } from "../types";
const props = defineProps<{ job: ACMEJob; pending?: boolean; feedback?: { type: 'success' | 'error'; message: string } }>();
const emit = defineEmits<{ action: [id: string, action: string]; edit: [id: string]; logs: [id: string]; diagnose: [id: string] }>();
const names: Record<string, string> = { queued: '等待执行', running: '执行中', ready: '已签发', failed: '失败', letsencrypt: 'Let’s Encrypt', zerossl: 'ZeroSSL', custom: '自定义', cloudflare: 'Cloudflare', alidns: '阿里云', tencentcloud: '腾讯云 DNSPod' };
const date = (value: string) => new Date(value).toLocaleString();
const now = ref(Date.now());
const clock = window.setInterval(() => { now.value = Date.now(); }, 1000);
onBeforeUnmount(() => window.clearInterval(clock));
const retrySeconds = computed(() => Math.max(0, Math.ceil((new Date(props.job.retry_after ?? 0).getTime() - now.value) / 1000)));
const nextSeconds = computed(() => Math.max(0, Math.ceil((new Date(props.job.next_attempt).getTime() - now.value) / 1000)));
const blocked = computed(() => props.pending || props.job.status === 'running' || !props.job.enabled || retrySeconds.value > 0);
const confirmDelete = ref(false);
watch(() => props.job.id, () => { confirmDelete.value = false; });
function remove() {
  confirmDelete.value = false;
  emit('action', props.job.id, 'delete');
}
</script>
<template>
    <div class="acme-job">
      <div class="acme-heading"><strong>{{ job.name }}</strong><span class="badge" :class="!job.enabled || job.status === 'failed' ? 'danger' : job.status === 'ready' ? 'success' : 'info'">{{ job.enabled ? names[job.status] : job.certificate_id ? '续期已暂停' : '申请已暂停' }}</span></div>
      <div class="field-help">{{ job.domains.join('、') }} · {{ names[job.ca] }} · {{ names[job.provider] || job.provider }}</div>
      <p class="acme-message" :class="job.status === 'failed' ? 'is-error' : job.status === 'ready' ? 'is-success' : 'is-progress'" role="status">{{ job.message }}</p>
      <p v-if="job.cleanup_message" class="field-help">清理结果：{{ job.cleanup_message }}</p>
      <p class="field-help">{{ job.enabled ? '自动申请 / 续期已启用' : '自动申请 / 续期已暂停' }}<template v-if="job.enabled && job.status === 'failed'"> · 自动重试{{ nextSeconds ? `约 ${Math.ceil(nextSeconds / 60)} 分钟后执行` : '等待执行' }}</template></p>
      <div class="field-help">{{ job.not_after ? `证书到期：${date(job.not_after)} · ` : '' }}{{ job.enabled ? `下次检查：${date(job.next_attempt)}` : job.certificate_id ? '已签发的证书仍可使用' : '申请已暂停；可编辑配置后重试' }}</div>
      <div class="acme-actions">
        <button type="button" class="button small" @click="emit('logs', job.id)">查看日志</button>
        <button type="button" class="button small" :disabled="pending" @click="emit('diagnose', job.id)">诊断配置</button>
        <button type="button" class="button small" :disabled="pending || job.status === 'running'" @click="emit('edit', job.id)">编辑配置</button>
        <button class="button small acme-renew-button" :disabled="blocked" @click="emit('action', job.id, job.deployment_pending ? 'retry_deploy' : 'retry')">{{ job.deployment_pending ? '重试部署' : job.certificate_id ? '立即续期' : '重试申请' }}</button>
        <button class="button small acme-renewal-toggle" :class="job.enabled ? 'is-pause' : 'is-resume'" :disabled="pending || job.status === 'running'" @click="emit('action', job.id, job.enabled ? 'pause' : 'resume')">{{ job.enabled ? '暂停续期' : '启用续期' }}</button>
        <button class="button danger-ghost small" :disabled="pending || job.status === 'running'" @click="confirmDelete = true">移除任务</button>
        <span v-if="retrySeconds && job.enabled && job.status !== 'running'" class="field-help">{{ retrySeconds }} 秒后可手动重试</span>
        <span v-if="confirmDelete" class="acme-delete-confirm" role="alert">
          确定移除？
          <button type="button" class="button danger small" :disabled="pending" @click="remove">确认</button>
          <button type="button" class="button ghost small" :disabled="pending" @click="confirmDelete = false">取消</button>
        </span>
        <span v-if="feedback" class="acme-action-feedback" :class="feedback.type" :role="feedback.type === 'error' ? 'alert' : 'status'">{{ feedback.type === 'success' ? '✓' : '!' }} {{ feedback.message }}</span>
      </div>
    </div>
</template>
<style scoped>
.acme-heading, .acme-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.acme-heading h3 { margin: 0; font-size: 16px; }
.acme-heading { justify-content: space-between; }
.acme-job { padding-top: 14px; margin-top: 14px; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
.acme-message { font-size: 14px; margin: 8px 0; font-weight: 560; }
.acme-message.is-success { color: var(--accent-dark); }
.acme-message.is-error { color: var(--danger); }
.acme-message.is-progress { color: var(--text-muted); }
.acme-actions { margin-top: 10px; }
.acme-renew-button { color: var(--accent-dark); border-color: var(--accent); background: var(--accent-soft); }
.acme-renewal-toggle.is-pause { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 45%, var(--line)); background: var(--danger-soft); }
.acme-renewal-toggle.is-resume { color: white; border-color: var(--accent); background: var(--accent); }
.acme-delete-confirm { display: inline-flex; align-items: center; gap: 6px; color: var(--danger); font-size: 13px; font-weight: 600; }
.acme-action-feedback { display: inline-flex; align-items: center; min-height: 30px; font-size: 13px; font-weight: 600; }
.acme-action-feedback.success { color: var(--accent-dark); }
.acme-action-feedback.error { color: var(--danger); }
</style>
