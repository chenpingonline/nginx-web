<script setup lang="ts">
import { computed, ref, useId, watch } from "vue";
import AppSelect from "./AppSelect.vue";
import type { Parameter } from "../nginxConfig";

const props = defineProps<{ field: Parameter; disabled: boolean; stream: boolean }>();
const value = defineModel<string>({ required: true });
const id = useId();
const { directive, original } = props.field;
const listener = directive === "listen" && /^(\[::\]:)?(\d+)(\s+ssl)?$/.exec(original);
const target = directive === "proxy_pass" && /^(https?):\/\/(\[[\da-fA-F:]+\]|[a-zA-Z0-9.-]+):(\d+)$/.exec(original);
const switchable = ["http2", "proxy_buffering", "proxy_request_buffering"].includes(directive)
  && ["on", "off"].includes(original);
const port = ref(listener ? listener[2] : target ? target[3] : "");
const tls = ref(Boolean(listener && listener[3]));
const scheme = ref(target ? target[1] : "");
const host = ref(target ? target[2] : "");
const portLabel = computed(() => listener ? (listener[1] ? "IPv6 监听端口" : "IPv4 监听端口") : "目标端口");
watch([port, tls, scheme, host], () => {
  if (listener) value.value = (listener[1] || "") + port.value + (tls.value ? listener[3] || " ssl" : "");
  if (target) value.value = scheme.value + "://" + host.value + ":" + port.value;
});
</script>

<template>
  <div class="parameter">
    <template v-if="target">
      <label :for="id + '-scheme'">后端协议</label>
      <AppSelect :id="id + '-scheme'" v-model="scheme" :disabled="disabled">
        <option value="http">HTTP</option><option value="https">HTTPS</option>
      </AppSelect>
      <label :for="id + '-host'">目标主机</label>
      <input :id="id + '-host'" v-model="host" class="input" required
        :pattern="String.raw`(?:[a-zA-Z0-9.\-]+|\[[a-fA-F0-9:]+\])`" :disabled="disabled">
    </template>
    <template v-if="listener || target">
      <label :for="id">{{ portLabel }}</label>
      <input :id="id" :value="port" class="input" required type="number" min="1" max="65535" step="1"
        @input="port = ($event.target as HTMLInputElement).value" :disabled="disabled">
      <template v-if="listener">
        <label :for="id + '-tls'">{{ stream ? "启用 TLS" : "启用 HTTPS" }}</label>
        <input :id="id + '-tls'" v-model="tls" type="checkbox" :disabled="disabled">
      </template>
    </template>
    <template v-else>
      <label :for="id">{{ field.label }}</label>
      <input v-if="switchable" :id="id" type="checkbox" :checked="value === 'on'" :disabled="disabled"
        @change="value = ($event.target as HTMLInputElement).checked ? 'on' : 'off'">
      <textarea v-else-if="/[\r\n]/.test(original)" :id="id" v-model="value" class="textarea wide" rows="3" required spellcheck="false" :disabled="disabled" />
      <input v-else :id="id" v-model="value" class="input wide" required spellcheck="false" :disabled="disabled">
    </template>
  </div>
</template>

<style scoped>
.parameter { display: grid; grid-template-columns: 8em minmax(0, 1fr) 8em minmax(0, 1fr); gap: 8px; align-items: center; margin-bottom: 8px; }
label { justify-self: end; text-align: right; font-weight: 600; overflow-wrap: anywhere; max-width: 100%; }
.input, .textarea { min-width: 0; width: 100%; }
.wide { grid-column: 2 / -1; }
input[type=checkbox] { width: 17px; height: 17px; min-height: 0; margin: 0; }
@media (max-width: 700px) { .parameter { grid-template-columns: 7em minmax(0, 1fr); } }
</style>
