<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, watch } from 'vue';
import AppSelect from './AppSelect.vue';
import ACMESecretState from './ACMESecretState.vue';
import { dnsProviderLink } from '../dnsProviderLinks';
import { request, errorMessage } from '../api';
import type { ACMEInput, DNSCatalog, DNSProviderField } from '../types';
const props = defineProps<{ modelValue: ACMEInput; busy: boolean; configuredSecrets?: string[]; clearSecrets?: string[] }>();
const emit = defineEmits<{ 'update:modelValue': [value: ACMEInput]; ready: [value: boolean]; 'clear-secret': [path: string] }>();
function set<K extends keyof ACMEInput>(key: K, value: ACMEInput[K]) { emit('update:modelValue', { ...props.modelValue, [key]: value }); }
function credential(key: keyof ACMEInput['credentials'], value: string) { set('credentials', { ...props.modelValue.credentials, [key]: value }); }
const advanced = ref(false);
const configured = (path: string) => props.configuredSecrets?.includes(path) ?? false;
const clearing = (path: string) => props.clearSecrets?.includes(path) ?? false;
const secretPlaceholder = (path: string) => clearing(path) ? '保存后清除' : configured(path) ? '已配置，留空保留；填写新值替换' : '';
const resolvers = computed({ get: () => (props.modelValue.dns_resolvers ?? []).join('\n'), set: (value: string) => set('dns_resolvers', value.split(/[\s,，]+/).filter(Boolean)) });
const domains = computed({ get: () => props.modelValue.domains.join('\n'), set: (value: string) => set('domains', value.split('\n')) });

const catalog = ref<DNSCatalog>();
const loading = ref(false);
const loadError = ref('');
const controller = new AbortController();
async function loadProviders() {
 loading.value = true; loadError.value = '';
 try { catalog.value = await request<DNSCatalog>('/acme/providers', { signal: controller.signal }); }
 catch(e) { if (!controller.signal.aborted) loadError.value = errorMessage(e); }
 finally { loading.value = false; }
}
onMounted(loadProviders);
onBeforeUnmount(() => controller.abort());
const selected = computed(() => catalog.value?.providers.find(p => p.code === props.modelValue.provider));
watch(selected, value => emit('ready', !!value), { immediate: true });
function selectProvider(value: string) { emit('update:modelValue', { ...props.modelValue, provider: value, dns_config: {} }); }
function dnsValue(key: string, value: string) { set('dns_config', { ...props.modelValue.dns_config, [key]: value }); }
const alidnsFields: Record<string, { label: string; description: string; advanced: boolean }> = {
 ALICLOUD_ACCESS_KEY: { label: 'ID', description: '', advanced: false },
 ALICLOUD_SECRET_KEY: { label: 'Secret', description: '', advanced: false },
 ALICLOUD_RAM_ROLE: { label: '实例 RAM 角色（可选）', description: '仅在阿里云 ECS 上使用实例角色认证时填写；此方式无需填写 AccessKey ID 和 Secret。', advanced: true },
 ALICLOUD_SECURITY_TOKEN: { label: 'STS 临时令牌（可选）', description: '仅使用 STS 临时凭证时填写，并配套填写临时 AccessKey ID 和 Secret；普通 AccessKey 认证请留空。', advanced: true },
};
// Keep provider-specific credential names while hiding environment variable prefixes.
const domesticLabels: Record<string, string> = {
 TENCENTCLOUD_SECRET_ID: 'ID', TENCENTCLOUD_SECRET_KEY: 'Secret',
 BAIDUCLOUD_ACCESS_KEY_ID: 'ID', BAIDUCLOUD_SECRET_ACCESS_KEY: 'Secret',
 COM35_USERNAME: '用户名', COM35_PASSWORD: 'API 密码',
 DNS51_API_KEY: 'API Key', DNS51_API_SECRET: 'Secret',
 DNSLA_API_ID: 'ID', DNSLA_API_SECRET: 'Secret',
 EDGEONE_SECRET_ID: 'ID', EDGEONE_SECRET_KEY: 'Secret',
 HUAWEICLOUD_ACCESS_KEY_ID: 'ID', HUAWEICLOUD_SECRET_ACCESS_KEY: 'Secret', HUAWEICLOUD_REGION: '区域',
 JDCLOUD_ACCESS_KEY_ID: 'ID', JDCLOUD_ACCESS_KEY_SECRET: 'Secret',
 RAINYUN_API_KEY: 'API Key',
 TODAYNIC_AUTH_USER_ID: '账户 ID', TODAYNIC_API_KEY: 'API Key',
 UCLOUD_PUBLIC_KEY: '公钥', UCLOUD_PRIVATE_KEY: '私钥',
 VOLC_ACCESSKEY: 'ID', VOLC_SECRETKEY: 'Secret',
 WESTCN_USERNAME: '用户名', WESTCN_PASSWORD: 'API 密码',
 XINNET_SECRET: '应用密钥', XINNET_AGENT_ID: '代理商 ID',
};
function fieldPresentation(field: DNSProviderField) {
 if (props.modelValue.provider === 'alidns') return alidnsFields[field.key];
 if (props.modelValue.provider === 'aliesa') return alidnsFields[field.key.replace(/^ALIESA_/, 'ALICLOUD_')];
 if (props.modelValue.provider === 'cloudflare') {
   const labels: Record<string, string> = { CF_DNS_API_TOKEN: 'DNS API Token', CF_ZONE_API_TOKEN: 'Zone API Token', CF_API_EMAIL: '账户邮箱', CF_API_KEY: 'Global API Key' };
   if (labels[field.key]) return { label: labels[field.key], description: field.description, advanced: field.advanced };
 }
 if (selected.value?.group !== '国内') return undefined;
 const label = domesticLabels[field.key];
 return label ? { label, description: '', advanced: field.advanced } : undefined;
}
function isAdvanced(field: DNSProviderField) {
 const presentation = fieldPresentation(field);
 if (props.modelValue.provider === 'cloudflare') return field.key !== 'CF_DNS_API_TOKEN';
 if (presentation) return presentation.advanced;
 return field.advanced || field.description.startsWith('Alias') || (props.modelValue.provider === 'cloudflare' && field.key !== 'CF_DNS_API_TOKEN');
}
const primaryFields = computed(() => selected.value?.fields.filter(f => advanced.value || !isAdvanced(f)) ?? []);
const hasAdvanced = computed(() => selected.value?.fields.some(isAdvanced) ?? false);
const providerLink = computed(() => selected.value && dnsProviderLink(selected.value.code, selected.value.name));
</script>
<template>
  <div class="field full"><label for="acme-ca">证书颁发机构</label><AppSelect id="acme-ca" :model-value="modelValue.ca" class="select" :disabled="busy" @update:model-value="set('ca', $event)"><option value="letsencrypt">Let’s Encrypt</option><option value="zerossl">ZeroSSL</option><option value="custom">自定义 ACME 服务</option></AppSelect></div>
  <div v-if="modelValue.ca === 'custom'" class="field full"><label for="acme-url">ACME Directory URL</label><input id="acme-url" class="input" type="url" required placeholder="https://ca.example.com/directory" :value="modelValue.directory_url" :disabled="busy" @input="set('directory_url', ($event.target as HTMLInputElement).value)" /></div>
  <template v-if="modelValue.ca === 'zerossl' || modelValue.ca === 'custom'">
    <div class="field full"><label for="acme-kid">EAB KID</label><input id="acme-kid" class="input" :placeholder="secretPlaceholder('credentials.eab_kid')" :required="modelValue.ca === 'zerossl' && (!configured('credentials.eab_kid') || clearing('credentials.eab_kid'))" :value="modelValue.credentials.eab_kid" :disabled="busy || clearing('credentials.eab_kid')" autocomplete="off" @input="credential('eab_kid', ($event.target as HTMLInputElement).value)" /><ACMESecretState path="credentials.eab_kid" :configured="configured('credentials.eab_kid')" :clearing="clearing('credentials.eab_kid')" :busy="busy" @clear="emit('clear-secret', $event)" /></div>
    <div class="field full"><label for="acme-hmac">EAB HMAC Key</label><input id="acme-hmac" class="input" :placeholder="secretPlaceholder('credentials.eab_hmac')" type="password" :required="modelValue.ca === 'zerossl' && (!configured('credentials.eab_hmac') || clearing('credentials.eab_hmac'))" :value="modelValue.credentials.eab_hmac" :disabled="busy || clearing('credentials.eab_hmac')" autocomplete="new-password" @input="credential('eab_hmac', ($event.target as HTMLInputElement).value)" /><span class="field-help">{{ modelValue.ca === 'zerossl' ? '在 ZeroSSL 控制台生成 EAB 凭据，不能用 DNS Token 代替。' : '仅在证书机构要求 EAB 时填写，两项需同时提供。' }}</span><ACMESecretState path="credentials.eab_hmac" :configured="configured('credentials.eab_hmac')" :clearing="clearing('credentials.eab_hmac')" :busy="busy" @clear="emit('clear-secret', $event)" /></div>
  </template>

  <div class="field full"><label for="acme-provider">DNS 验证服务商</label><AppSelect id="acme-provider" :model-value="modelValue.provider" class="select" searchable search-placeholder="搜索 DNS 服务商" :disabled="busy || loading" @update:model-value="selectProvider"><option v-for="provider in catalog?.providers ?? []" :key="provider.code" :value="provider.code">{{ provider.name }}</option></AppSelect></div>
  <div v-if="loadError" class="full notice warning" role="alert">服务商列表加载失败：{{ loadError }} <button type="button" class="button ghost small" @click="loadProviders">重试</button></div>
  <div v-if="selected" class="full dns-provider-fields">
    <p v-if="providerLink" class="field-help"><a class="dns-credential-link" :href="providerLink.url" target="_blank" rel="noopener noreferrer">{{ providerLink.label }}</a></p>
    <div v-for="field in primaryFields" :key="field.key" class="field full dns-provider-field">
      <label :for="`dns-${field.key}`">{{ fieldPresentation(field)?.label ?? field.key }}</label>
      <textarea v-if="field.multiline" :id="`dns-${field.key}`" class="textarea" rows="3" :placeholder="secretPlaceholder(`dns_config.${field.key}`)" :value="modelValue.dns_config?.[field.key] ?? ''" :disabled="busy || clearing(`dns_config.${field.key}`)" autocomplete="off" spellcheck="false" @input="dnsValue(field.key, ($event.target as HTMLTextAreaElement).value)" />
      <input v-else :id="`dns-${field.key}`" :type="field.secret ? 'password' : 'text'" class="input" :placeholder="secretPlaceholder(`dns_config.${field.key}`)" :value="modelValue.dns_config?.[field.key] ?? ''" :disabled="busy || clearing(`dns_config.${field.key}`)" autocomplete="new-password" spellcheck="false" @input="dnsValue(field.key, ($event.target as HTMLInputElement).value)" />
      <ACMESecretState v-if="field.secret" :path="`dns_config.${field.key}`" :configured="configured(`dns_config.${field.key}`)" :clearing="clearing(`dns_config.${field.key}`)" :busy="busy" @clear="emit('clear-secret', $event)" />
      <span v-if="fieldPresentation(field)?.description ?? field.description" class="field-help">{{ fieldPresentation(field)?.description ?? field.description }}</span>
    </div>
  </div>
  <label v-if="hasAdvanced" class="checkbox-row full"><input v-model="advanced" type="checkbox" :disabled="busy" />显示 DNS 高级参数</label>
  <div class="field full"><label for="acme-domains">域名列表</label><textarea id="acme-domains" v-model="domains" class="textarea" rows="3" required spellcheck="false" :disabled="busy" placeholder="example.com&#10;*.example.com"></textarea><span class="field-help">每行一个域名，支持通配符；DNS 验证不支持 IP 地址证书。</span></div>
  <div class="field full"><label for="acme-email">电子邮箱</label><input id="acme-email" class="input" type="email" required :value="modelValue.email" :disabled="busy" @input="set('email', ($event.target as HTMLInputElement).value)" /></div>
  <div class="field full"><label for="acme-key-type">密钥算法</label><AppSelect id="acme-key-type" :model-value="modelValue.key_type" class="select" :disabled="busy" @update:model-value="set('key_type', $event)"><option value="rsa2048">RSA 2048</option><option value="rsa4096">RSA 4096</option><option value="ec256">ECDSA P-256</option><option value="ec384">ECDSA P-384</option></AppSelect></div>
  <div class="field full"><label for="acme-dns-timeout">DNS 等待时间（秒）</label><input id="acme-dns-timeout" class="input" type="number" min="30" max="1800" required :value="modelValue.propagation_seconds" :disabled="busy" @input="set('propagation_seconds', Number(($event.target as HTMLInputElement).value))" /></div>
  <div class="field full"><label for="acme-resolvers">ACME DNS 服务器</label><textarea id="acme-resolvers" v-model="resolvers" class="textarea" rows="2" :disabled="busy" spellcheck="false" placeholder="223.5.5.5&#10;223.6.6.6" /><span class="field-help">每行一个 IP，可带端口。留空使用 NAS 系统 DNS；局域网通配符解析可能干扰验证，可填写独立 DNS。仅影响本任务，不修改 NAS 网络设置。</span></div>
  <details class="full acme-dns-options">
    <summary>DNS 验证高级选项</summary>
    <div class="field full"><label for="acme-check-mode">本地预检查</label><AppSelect id="acme-check-mode" :model-value="modelValue.dns_check_mode ?? 'auto'" class="select" :disabled="busy" @update:model-value="set('dns_check_mode', $event as 'auto' | 'wait')"><option value="auto">自动检查 DNS 记录（推荐）</option><option value="wait">固定等待，跳过本地预检查</option></AppSelect></div>
    <template v-if="modelValue.dns_check_mode === 'wait'">
      <div class="field full"><label for="acme-fixed-wait">固定等待时间（秒）</label><input id="acme-fixed-wait" class="input" type="number" min="30" max="600" required :value="modelValue.dns_wait_seconds ?? 60" :disabled="busy" @input="set('dns_wait_seconds', Number(($event.target as HTMLInputElement).value))" /></div>
      <p class="notice warning">创建 TXT 后按此时间等待。Let’s Encrypt / CA 仍会独立验证公网 DNS；记录未生效时申请仍会失败。建议先配置可用的独立 DNS。</p>
    </template>
  </details>
  <label class="checkbox-row full"><input type="checkbox" :checked="modelValue.rotate_key" :disabled="busy" @change="set('rotate_key', ($event.target as HTMLInputElement).checked)" />每次续期更换私钥</label>
  <label class="checkbox-row full"><input type="checkbox" required :checked="modelValue.accept_terms" :disabled="busy" @change="set('accept_terms', ($event.target as HTMLInputElement).checked)" />我同意所选证书机构的服务条款，并授权自动申请和续期</label>
  <span class="field-help full"><a v-if="modelValue.ca === 'letsencrypt'" href="https://letsencrypt.org/repository/" target="_blank" rel="noopener noreferrer">查看 Let’s Encrypt 服务条款</a><a v-else-if="modelValue.ca === 'zerossl'" href="https://zerossl.com/terms/" target="_blank" rel="noopener noreferrer">查看 ZeroSSL 服务条款</a><template v-else>请向自定义证书机构确认服务条款。</template> 申请在后台执行；签发后可绑定到代理规则，后续自动续期。</span>
</template>

<style scoped>
.acme-dns-options { display: block; }
.acme-dns-options summary { cursor: pointer; margin-bottom: 12px; }
.acme-dns-options > .field { margin-block: 10px; }
.dns-provider-fields { min-width: 0; display: grid; grid-template-columns: subgrid; row-gap: 10px; }
.dns-provider-fields > p { margin: 0; grid-column: 1 / -1; }
.dns-provider-field { min-width: 0; }
.dns-provider-field > label { max-width: 132px; overflow-wrap: anywhere; }
.dns-provider-field .field-help { overflow-wrap: anywhere; }
</style>
