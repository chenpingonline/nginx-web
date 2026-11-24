<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { configBody, formPrefix, type ConfigForm } from "../configForm";
import { ConfigLineError, parseConfig } from "../nginxConfig";
import { request, jsonBody, errorMessage } from "../api";
import type { ProxyRule } from "../types";
import { configFragment, externalPool, importMode, previewImport, type ImportDraft, type SourceFormPlan } from "../configImport";
import { sourceTargets, targetKey } from "../sourceImport";
import SourceParameterDialog from "./SourceParameterDialog.vue";
import AppSelect from "./AppSelect.vue";

const props = defineProps<{
  state: import("../types").State;
  rules: ProxyRule[];
  streamPage: boolean;
  customConfigEnabled: boolean;
  openForm: (value: ConfigForm) => Promise<void>;
  preview?: { content: string; name: string; draft: boolean };
}>();
const emit = defineEmits<{
  close: [];
  saved: [];
  raw: [source: string, stream: boolean, line?: number];
  copy: [value: string, label: string];
}>();
const dialog = ref<HTMLDialogElement>();
const previewBody = computed(() => configBody(props.preview?.content ?? ""));
const formHeader = computed(() => {
  const content = props.preview?.content ?? "";
  return content.slice(0, content.length - previewBody.value.length);
});
const source = ref(""),
  target = ref(""),
  error = ref(""),
  working = ref(false);
const generated = ref<string[]>([]);
const errorLine = ref<number>();
let candidate: ImportDraft | undefined;
const scope = ref<"http" | "stream">(props.streamPage ? "stream" : "http");
const blocks = computed(() => {
  try {
    return parseConfig(source.value);
  } catch {
    return [];
  }
});
const fragment = computed(() => configFragment(source.value, blocks.value, scope.value));
function keepRaw(line?: number) {
  emit("raw", fragment.value.text, fragment.value.scope === "stream", line);
}
const sourceForm = ref<SourceFormPlan>();
const editingParameters = ref(false);
const annotated = computed(() => source.value.trimStart().startsWith(formPrefix));
const sourceMode = computed(() => {
  if (annotated.value) return;
  const f = fragment.value;
  return importMode(f, f.wrapped ? parseConfig(f.text) : blocks.value);
});
const singlePool = computed(() => blocks.value.length === 1 && externalPool(blocks.value[0]));
const sites = computed(() => sourceMode.value === "server-fragment" ? sourceTargets(props.state.custom_configs ?? []) : []);
const needsSite = computed(() => sourceMode.value === "server-fragment");
const savedWafCandidate = computed(() => {
  if (!needsSite.value) return false;
  const exportedSite = source.value.match(/^# server ([a-f0-9]+)\s*$/m)?.[1];
  return exportedSite && props.state.waf_instances?.some(waf => waf.sites.includes(exportedSite));
});
const isLocation = computed(() => blocks.value.length === 1 && blocks.value[0].words[0] === "location");
function acceptParameters(value: string) {
  editingParameters.value = false;
  source.value = value;
  if (!isLocation.value && !needsSite.value) keepRaw();
}
const migrationHints: [RegExp, string, string][] = [
  [/^http:\/\/unix:.*\/waf\.sock:/, "WAF 检测服务", "在目标机配置同等防护，确认 WAF 服务已启动，并将套接字路径指向该服务。"],
  [/^http:\/\/unix:.*:\/internal\/basic-auth\//, "访问认证服务", "在目标机重建这条规则的认证策略，并将认证请求指向可用的认证服务。"],
  [/^http:\/\/unix:/, "本地服务", "确认目标服务已启动，并更新 Unix 套接字路径。"],
  [/^ssl_certificate_key\s/, "TLS 私钥", "在目标机安全配置与证书匹配的私钥，更新文件路径并限制读取权限。"],
  [/^ssl_certificate\s/, "TLS 证书链", "在目标机配置该域名的证书链，并更新文件路径。"],
  [/^auth_basic_user_file\s/, "密码文件", "迁移认证文件，更新路径并确认 Nginx 可以读取。"],
  [/^(root|alias)\s/, "站点目录", "迁移站点文件，更新目录路径并确认 Nginx 可以读取。"],
  [/^access_log\s/, "访问日志", "核对目标日志路径和写入权限；使用命名日志格式时，一并迁移 log_format 定义。"],
  [/^fnproxy_req_/, "请求限流", "从 HTTP 公共配置迁移同名 limit_req_zone，核对限流键和速率。"],
  [/^fnproxy_(?:stream_)?conn_/, "连接限流", "在对应的 http 或 stream 配置中迁移同名 limit_conn_zone。"],
  [/^fnproxy_cache_/, "代理缓存", "迁移定义同名缓存区的 proxy_cache_path，并准备可写的缓存目录。"],
  [/^fnproxy_stream_metrics_/, "流量日志格式", "从 stream 公共配置迁移同名 log_format。"],
  [/^fnproxy_(up|stream)_/, "转发服务组", "迁移同名 upstream 配置，核对目标机到各后端的连接。"],
  [/^\$(connection_upgrade|fnproxy_client_host|fnproxy_waf_checked)$/, "HTTP 变量", "从主配置复制定义该变量的完整 map 块，放入目标 Nginx 的 http 块。"],
  [/^\$/, "配置变量", "在原配置中查找该变量的定义，并在目标机的相同作用域中配置。"],
];
const dependencies = computed(() => {
  const content = props.preview?.content || "";
  const references =
    content.match(
      /\$connection_upgrade\b|\$fnproxy_(?!rule_id\b)\w+|\bfnproxy_(?:up|req|conn|cache|stream)_\w+|(?:ssl_certificate(?:_key)?|auth_basic_user_file|root|alias|access_log)\s+[^;]+|http:\/\/unix:[^";]+/g,
    ) ?? [];
  return [...new Set(references)].filter(value => !/^access_log\s+off\s*$/.test(value)).map(value => {
    const hint = migrationHints.find(([pattern]) => pattern.test(value));
    return { value, title: hint?.[1] ?? "配置引用", action: hint?.[2] ?? "在目标机核对该引用的定义和路径。" };
  });
});
watch([source, target, scope], () => {
  candidate = undefined;
  generated.value = [];
  error.value = "";
  errorLine.value = undefined;
});
onMounted(() => dialog.value?.showModal());
async function run(save: boolean) {
  working.value = true;
  error.value = "";
  errorLine.value = undefined;
  try {
    if (source.value.trimStart().startsWith(formPrefix)) {
      const form = await request<ConfigForm>("/config-form", { method: "POST", body: jsonBody({content: source.value}) });
      await props.openForm(form);
    } else if (save && candidate && generated.value.length) {
      await request("/config-import/save", {
        method: "POST",
        body: jsonBody({ ...candidate, expected: generated.value }),
      });
      emit("saved");
    } else {
      const result = await previewImport(source.value, target.value, props.state, scope.value);
      if ("form" in result) { await props.openForm(result.form); return; }
      if ("sourceForm" in result) {
        if (!props.customConfigEnabled) throw Error("请先启用“Nginx 配置”菜单，以保存自定义配置草稿");
        sourceForm.value = result.sourceForm;
        return;
      }
      candidate = result.draft;
      generated.value = result.generated;
    }
  } catch (e) {
    error.value = errorMessage(e);
    errorLine.value = e instanceof ConfigLineError ? e.line : undefined;
    generated.value = [];
    candidate = undefined;
  } finally {
    working.value = false;
  }
}
</script>
<template>
  <dialog
    ref="dialog"
    class="config-dialog"
    aria-labelledby="config-dialog-title"
    @cancel.prevent="!working && emit('close')"
  >
    <header class="config-dialog-heading">
      <h2 id="config-dialog-title">{{ preview ? (preview.draft ? "草稿配置" : "规则配置") + " · " + preview.name : "从 Nginx 配置导入" }}</h2>
      <div v-if="preview" class="preview-actions">
        <button class="button ghost" @click="emit('close')">关闭</button>
        <button v-if="formHeader" class="button secondary" @click="emit('copy', preview.content, '可导入配置')">复制可导入配置</button>
        <button class="button primary" autofocus @click="emit('copy', previewBody, 'Nginx 配置')">复制 Nginx 配置</button>
      </div>
    </header>
    <template v-if="preview">
      <pre>{{ previewBody }}</pre>
      <details v-if="formHeader"><summary>可导入配置附带的表单数据</summary><pre>{{ formHeader }}</pre></details>
      <details v-if="dependencies.length">
        <summary>迁移时需要的依赖（{{ dependencies.length }}）</summary>
        <p>仅在把这段配置复制到另一台 Nginx 时逐项核对。准备好依赖后，运行 <code>nginx -t</code> 校验，再重载并测试转发。</p>
        <ul class="migration-list">
          <li v-for="item in dependencies" :key="item.value">
            <strong>{{ item.title }}</strong>
            <code>{{ item.value }}</code>
            <span>{{ item.action }}</span>
          </li>
        </ul>
      </details>
    </template>
    <template v-else>
      <label for="import-source">Nginx 配置块</label>
      <textarea
        id="import-source"
        v-model="source"
        autofocus
        class="textarea"
        :disabled="working"
        spellcheck="false"
        maxlength="1048576"
        placeholder="粘贴复制的配置、server 或 location 块"
      ></textarea>
      <div v-if="!annotated && fragment.sharedScope" class="import-target">
        <label for="import-scope">{{ singlePool ? "服务组协议" : "配置作用域" }}</label>
        <AppSelect id="import-scope" v-model="scope" :disabled="working">
          <option value="http">HTTP</option><option value="stream">TCP/UDP</option>
        </AppSelect>
      </div>
      <div v-if="needsSite" class="import-target">
        <template v-if="sites.length">
          <label for="import-site">目标站点</label>
          <AppSelect id="import-site" v-model="target" aria-label="目标站点" :disabled="working">
            <option value="">请选择</option>
            <option v-for="site in sites" :key="targetKey(site)" :value="targetKey(site)">{{ site.title }} · {{ site.file.name }} · 第 {{ site.line }} 行</option>
          </AppSelect>
        </template>
        <p v-else-if="!savedWafCandidate">请粘贴包含这段配置的完整 server 块，或先创建可追加的自定义 HTTP 站点。</p>
      </div>
      <div v-else-if="isLocation && !annotated && !sourceMode" class="import-target">
        <label for="import-target">目标代理</label
        ><AppSelect
          id="import-target"
          v-model="target"
          aria-label="目标代理"
          :disabled="working"
          ><option value="">请选择</option>
          <option v-for="rule in rules" :key="rule.id" :value="rule.id">
            {{ rule.name }}
          </option></AppSelect
        >
      </div>
      <p v-if="error" role="alert">
        {{ error
        }}<template v-if="isLocation && !needsSite"
          >。按原文保存时请粘贴包含该路径的完整 server 块。</template
        >
      </p>
      <template v-if="generated.length"
        ><h3>将保存的配置</h3>
        <p>请核对下方完整配置。应用会生成请求头、超时及日志相关指令。</p>
        <pre v-for="(content, i) in generated" :key="i">{{ content }}</pre>
      </template>
    </template>
    <footer v-if="!preview">
      <button class="button ghost" :disabled="working" @click="emit('close')">
        关闭
      </button>
      <button v-if="error && !sourceMode && blocks.length && !source.trimStart().startsWith(formPrefix)" class="button secondary"
        @click="editingParameters = true">按原文编辑参数</button>
      <button
        v-if="!isLocation && !needsSite"
        class="button secondary"
        :disabled="working || !source.trim()"
        @click="keepRaw(errorLine)"
      >
        保留为自定义配置
      </button>
      <button
        class="button primary"
        :disabled="working || !source.trim() || (needsSite && !savedWafCandidate && !sites.some(s => targetKey(s) === target))"
        @click="run(generated.length > 0)"
      >
        {{
          working
            ? "处理中…"
            : generated.length
              ? "确认保存为草稿"
              : source.trimStart().startsWith(formPrefix) ? "打开对应表单" : sourceMode ? "打开参数表单" : "预览导入"
        }}
      </button>
    </footer>
  </dialog>
  <SourceParameterDialog v-if="sourceForm" :entry="sourceForm.entry" :create="!sourceForm.baseFile" :base-file="sourceForm.baseFile"
    @close="sourceForm = undefined" @saved="emit('saved')" />
  <SourceParameterDialog v-if="editingParameters" local
    :entry="{file: {name: fragment.scope === 'stream' ? 'custom-import.stream' : 'custom-import.conf', content: source}, title: '导入配置'}"
    @close="editingParameters = false" @edited="acceptParameters" />
</template>
<style scoped>
.config-dialog {
  box-sizing: border-box;
  width: min(800px, calc(100% - 24px));
  max-height: 90dvh;
  overflow: auto;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 18px;
}
.config-dialog::backdrop {
  background: #0008;
}
.config-dialog-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.preview-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-inline-start: auto; }
h2 {
  overflow-wrap: anywhere;
  margin: 0;
  font-size: 18px;
}
textarea {
  display: block;
  width: 100%;
  min-height: 160px;
  margin: 6px 0;
  font: 13px monospace;
}
pre {
  overflow: auto;
  max-height: 42vh;
  white-space: pre;
  background: var(--frame);
  padding: 12px;
}
.migration-list { list-style: none; padding: 0; margin: 0; }
.migration-list li { display: grid; gap: 4px; padding: 10px 0; border-top: 1px solid var(--line); }
.migration-list li > code { min-width: 0; }
.migration-list li > span { color: var(--text-muted); }
code,
p {
  overflow-wrap: anywhere;
}
footer,
.import-target {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 14px;
}
footer {
  justify-content: flex-end;
}
</style>
