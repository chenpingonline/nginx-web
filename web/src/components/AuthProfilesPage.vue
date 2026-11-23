<script setup lang="ts">
import { computed, ref } from "vue";
import { PhArrowClockwise, PhPlusCircle } from "@phosphor-icons/vue";
import AuthProfileManager from "./AuthProfileManager.vue";
import type { AuthProfile, ProxyRule } from "../types";

const props = defineProps<{ profiles: AuthProfile[]; rules: ProxyRule[]; busy: boolean }>();
defineEmits<{
  importConfig: [];
  exportConfig: [id: string, name: string];
 changed: [profiles: AuthProfile[]]; remove: [profile: AuthProfile]; refresh: [] }>();
const manager = ref<InstanceType<typeof AuthProfileManager>>();
const rows = computed(() => props.profiles.map(profile => ({
  profile,
  enabledUsers: profile.users.filter(user => user.enabled).length,
  rules: props.rules.filter(rule => rule.authentication?.enabled && rule.authentication.profile_id === profile.id),
})));
defineExpose({ show: (profile: AuthProfile) => manager.value?.show(profile) });
</script>

<template>
  <div class="toolbar compact-rule-toolbar">
    <span class="spacer"></span>
    <button class="button ghost" :disabled="busy" @click="$emit('refresh')"><PhArrowClockwise :size="16" />刷新</button>
    <button class="button ghost" @click="$emit('importConfig')">从 Nginx 配置导入</button>
    <button class="button primary" :disabled="busy" @click="manager?.show()"><PhPlusCircle :size="17" />添加认证策略</button>
  </div>
  <article class="card">
    <div v-if="rows.length" class="table-wrap">
      <table class="table">
        <thead><tr><th>策略名称</th><th>浏览器认证提示</th><th>已启用用户</th><th>使用规则</th><th aria-label="操作"></th></tr></thead>
        <tbody>
          <tr v-for="row in rows" :key="row.profile.id">
            <td class="rule-name">{{ row.profile.name }}</td><td>{{ row.profile.realm }}</td><td>{{ row.enabledUsers }} / {{ row.profile.users.length }}</td>
            <td :title="row.rules.map(rule => rule.name).join('、')">{{ row.rules.length }} 条规则</td>
            <td><div class="table-actions">
              <button class="button ghost small" @click="$emit('exportConfig', row.profile.id, row.profile.name)">复制配置</button>
                <button class="button ghost small" :disabled="busy" @click="manager?.show(row.profile)">编辑</button>
              <button class="button danger-ghost small" :disabled="busy || !!row.rules.length" :title="row.rules.length ? '请先在使用此策略的规则中关闭或切换认证' : undefined" @click="$emit('remove', row.profile)">删除</button>
            </div></td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="empty-state"><h3>还没有认证策略</h3><p>添加策略后，可在 HTTP(S) 代理规则中选择它来启用用户名密码认证。</p></div>
  </article>
  <AuthProfileManager ref="manager" :profiles="profiles" hide-trigger @changed="$emit('changed', $event)" />
</template>
