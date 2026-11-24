import { request, jsonBody } from "./api";
import { matchSavedConfig, type ConfigForm } from "./configForm";
import { ConfigLineError, importBlocks, newID, parseConfig } from "./nginxConfig";
import { rawFormEquivalent, rawPool } from "./rawConfigForm";
import type { State, UpstreamPool } from "./types";

type Scope = "http" | "stream";
type Block = ReturnType<typeof parseConfig>[number];
export type ImportDraft = ReturnType<typeof importBlocks> & { pools?: UpstreamPool[] };

export function configFragment(text: string, nodes: Block[], defaultScope: Scope) {
  const [first] = nodes;
  if (nodes.length === 1 && first.children && first.words.length === 1 &&
      (first.words[0] === "http" || first.words[0] === "stream")) {
    const opening = /^(?:http|stream)\s*(?:#[^\n]*\n\s*)*\{/.exec(text.slice(first.start));
    if (opening) {
      const start = first.start + opening[0].length;
      return { scope: first.words[0] as Scope, wrapped: true,
        text: text.slice(0, first.start) + text.slice(first.start + first.words[0].length, start - 1) +
          text.slice(start, first.end - 1) + text.slice(first.end) };
    }
  }
  const hasStreamPass = nodes.some(n => n.words[0] === "server" && n.children?.some(d => d.words[0] === "proxy_pass"));
  const hasHttpSite = nodes.some(n => n.words[0] === "server" && n.children?.some(d => d.words[0] === "location"));
  const scope = hasStreamPass ? "stream" : hasHttpSite ? "http" : defaultScope;
  return { text, scope, wrapped: false };
}

export function externalPool(node: Block) {
  return node.words[0] === "upstream" && !/^fnproxy_(?:up|stream)_[a-f0-9]+$/.test(node.words[1] ?? "");
}

function namedPool(text: string, node: Block, scope: Scope) {
  const name = node.words[1];
  if (node.words.length !== 2 || !node.children || !/^[\w.-]+$/.test(name))
    throw new ConfigLineError(node.line, "转发服务组需要明确的名称与节点列表");
  const id = newID();
  const managed = `fnproxy_${scope === "http" ? "up" : "stream"}_${id}`;
  const form = rawPool({ ...node, words: ["upstream", managed] });
  form.value.name = name;
  const original = text.slice(node.start, node.argsStart) + managed + text.slice(node.argsEnd, node.end);
  return { form, original, line: node.line };
}

type Result = { form: ConfigForm } | { draft: ImportDraft; generated: string[] };

export async function previewImport(text: string, target: string, state: State, defaultScope: Scope): Promise<Result> {
  const fragment = configFragment(text, parseConfig(text), defaultScope);
  const nodes = parseConfig(fragment.text);
  if (fragment.wrapped) {
    const allowed = fragment.scope === "http" ? ["server", "upstream", "map", "geo", "split_clients"] : ["server", "upstream"];
    const other = nodes.find(node => !allowed.includes(node.words[0]));
    if (other) throw new ConfigLineError(other.line, "此作用域中的指令需按原文编辑");
  }
  const declarations = nodes.filter(externalPool);
  let draft: ImportDraft;
  if (declarations.length) {
    const pools = declarations.map(node => namedPool(fragment.text, node, fragment.scope));
    if (nodes.length === 1) {
      const [{ form, original, line }] = pools;
      const preview = await request<{ form: ConfigForm; content: string }>("/config-form/preview", {
        method: "POST", body: jsonBody(form),
      });
      if (!rawFormEquivalent("upstreams", original, preview.content))
        throw new ConfigLineError(line, "服务组生成结果与原文不同");
      return { form: preview.form };
    }
    const sites = nodes.filter(n => n.words[0] === "server");
    if (fragment.scope !== "http" || !sites.length || sites.length + pools.length !== nodes.length)
      throw Error("组合导入支持 HTTP upstream 和 server；其他指令可按原文编辑");
    const poolIds = new Map(pools.map(({form}) => [form.value.name.toLowerCase(), form.id]));
    if (poolIds.size !== pools.length) throw Error("转发服务组名称重复");
    const rules = sites.flatMap(node =>
      importBlocks(fragment.text.slice(node.start, node.end), "", node.line, poolIds).rules ?? []);
    draft = {rules, pools: pools.map(({form}) => form.value)};
    const generated = await previewDraft(draft);
    for (const [index, pool] of pools.entries()) {
      if (!rawFormEquivalent("upstreams", pool.original, generated[index]))
        throw new ConfigLineError(pool.line, "服务组生成结果与原文不同");
    }
    return {draft, generated};
  }
  const form = await matchSavedConfig(fragment.text, state);
  if (form) {
    if (fragment.wrapped && ((form.kind === "streams" || (form.kind === "upstreams" && form.value.protocol === "stream")) !== (fragment.scope === "stream")))
      throw Error("配置块与外层作用域不匹配");
    return {form};
  }
  if (fragment.wrapped && fragment.scope === "stream") throw Error("此 stream 配置需按原文编辑");
  draft = importBlocks(fragment.text, target);
  return {draft, generated: await previewDraft(draft)};
}

function previewDraft(draft: ImportDraft) {
  return request<string[]>("/config-import/preview", {method: "POST", body: jsonBody(draft)});
}
