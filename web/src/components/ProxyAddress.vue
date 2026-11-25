<script setup lang="ts">
import { PhArrowSquareOut, PhCopy } from "@phosphor-icons/vue";
defineProps<{ value: string; entry?: boolean; openable?: boolean; copyDisabled?: boolean; unsupported?: boolean }>();
defineEmits<{ copy: [value: string, label: string] }>();
</script>

<template>
  <div class="proxy-entry">
    <span class="proxy-entry-url" :class="{ 'unsupported-entry': unsupported }"
      :title="unsupported ? value + '（当前版本不支持此监听端口）' : value">{{ value }}</span>
    <button type="button" class="icon-button proxy-entry-action" :disabled="copyDisabled"
      :aria-label="(entry ? '复制入口 ' : '复制转发服务 ') + value" :title="entry ? '复制入口地址' : '复制转发服务地址'"
      @click="$emit('copy', value, entry ? '入口地址' : '转发服务地址')"><PhCopy :size="16" aria-hidden="true" /></button>
    <a v-if="entry && openable" class="icon-button proxy-entry-action" :href="value" target="_blank" rel="noopener noreferrer"
      :aria-label="'在新窗口打开 ' + value" title="在新窗口打开"><PhArrowSquareOut :size="16" aria-hidden="true" /></a>
    <button v-else-if="entry" type="button" class="icon-button proxy-entry-action" disabled
      aria-label="该入口无法直接打开" title="该入口无法直接打开"><PhArrowSquareOut :size="16" aria-hidden="true" /></button>
  </div>
</template>
