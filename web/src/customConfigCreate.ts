import { APIError, jsonBody, request } from "./api";
import type { ConfigForm } from "./configForm";
import { ConfigLineError, importBlocks, parseConfig } from "./nginxConfig";
import { parseQuantity } from "./nginxQuantity";
import { managedPoolName, rawFormEquivalent, rawPool, rawStream } from "./rawConfigForm";
import { readCommentedBlocks } from "./configComments";
import { namedPool } from "./configImport";
import type { ConfigComment, CustomConfig, ProxyRule, State, StreamRule, UpstreamPool } from "./types";

type Block = ReturnType<typeof parseConfig>[number];
type HTTPRule = (NonNullable<ReturnType<typeof importBlocks>["rules"]>[number] | ProxyRule) & {comments?: ConfigComment[]};

// Different directive families can be interleaved, but relative order within
// a family matters (regular-expression locations, ACLs and rewrite phases).
function groupKey(node: Block) {
  const name = node.words[0];
  if (name === "allow" || name === "deny") return "acl";
  if (["set", "rewrite", "return", "if", "break"].includes(name)) return "rewrite-phase";
  return name;
}

function usesTrackingVariable(node: Block): boolean {
  return node.words.some(word => /\$\{?fnproxy_rule_id\b/.test(word)) || !!node.children?.some(usesTrackingVariable);
}

function comparableWords(node: Block) {
  const [name, value, ...args] = node.words;
  if (name === "listen") return [name, value.replace(/^(?:\*|0\.0\.0\.0):/, ""), ...args.map(a => a === "default" ? "default_server" : a).sort()];
  if (["proxy_connect_timeout", "proxy_send_timeout", "proxy_read_timeout", "proxy_timeout"].includes(name) && args.length === 0)
    return [name, String(parseQuantity(value, "seconds"))];
  return node.words;
}

function compareBlock(source: Block, generated: Block) {
  if (JSON.stringify(comparableWords(source)) !== JSON.stringify(comparableWords(generated)) || !!source.children !== !!generated.children)
    throw new ConfigLineError(source.line, `表单会改写 ${source.words.join(" ")}`);
  const groups = new Map<string, Block[]>();
  for (const child of generated.children ?? []) {
    // This diagnostic tag is the one addition required by managed HTTP sites.
    if (source.words[0] === "server" && child.words[0] === "set" && child.words[1] === "$fnproxy_rule_id" &&
        !source.children?.some(d => d.words[0] === "set" && d.words[1] === "$fnproxy_rule_id")) continue;
    const key = groupKey(child);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(child);
  }
  for (const child of source.children ?? []) {
    const next = groups.get(groupKey(child))?.shift();
    if (!next) throw new ConfigLineError(child.line, `表单无法保留 ${child.words.join(" ")}`);
    compareBlock(child, next);
  }
  for (const [extra] of groups.values()) {
    if (extra) throw new ConfigLineError(source.line, `${source.words.join(" ")} 中的表单会额外生成 ${extra.words.join(" ")}`);
  }
}

async function conversionRequest<T>(path: string, data: unknown, line: number) {
  try {
    return await request<T>(path, {method: "POST", body: jsonBody(data)});
  } catch (e) {
    if (e instanceof APIError && e.status === 400) throw new ConfigLineError(line, e.message);
    throw e;
  }
}

type RuleBatch = {new_file_name: string; rules: HTTPRule[]; streams: StreamRule[]; pools: UpstreamPool[]};

async function previewBatch(draft: RuleBatch, nodes: Block[]) {
  const preview = (batch: RuleBatch) => request<string[]>("/config-import/preview", {method: "POST", body: jsonBody(batch)});
  try {
    return await preview(draft);
  } catch (e) {
    if (!(e instanceof APIError) || e.status !== 400) throw e;
    // The validator also checks cross-rule conflicts. Locate the first failing
    // prefix so a problem in a later server points to the right source line.
    let first = 0, last = nodes.length - 1, reason = e.message;
    while (first < last) {
      const mid = Math.floor((first + last) / 2);
      try {
        await preview({...draft, rules: draft.rules.slice(0, mid + 1), streams: draft.streams.slice(0, mid + 1)});
        first = mid + 1;
      } catch (partial) {
        if (!(partial instanceof APIError) || partial.status !== 400) throw partial;
        last = mid;
        reason = partial.message;
      }
    }
    throw new ConfigLineError(nodes[first].line, reason);
  }
}

async function previewFile(file: CustomConfig, state: State) {
  const blocks = readCommentedBlocks(file.content);
  if (!blocks.length) throw new ConfigLineError(1, "文件里没有 server 块");
  if (blocks.length > 100) throw new ConfigLineError(blocks[100].node.line, "一次最多转换 100 个配置块");
  const stream = file.name.endsWith(".stream");
  const scope = stream ? "stream" : "http";
  const pools: UpstreamPool[] = [], rules: HTTPRule[] = [], streams: StreamRule[] = [];
  const poolIDs = new Map<string, string>();
  const declarations = blocks.filter(({node}) => node.words[0] === "upstream");
  const sites = blocks.filter(({node}) => node.words[0] !== "upstream");
  if (!sites.length) throw new ConfigLineError(blocks[0].node.line, "转发服务组需与 server 一起转换");

  async function readForm(block: typeof blocks[number]) {
    const header = block.comments.find(c => c.text.startsWith("# nginx-web-form-v1 "));
    if (!header) return;
    const last = block.comments[block.comments.length - 1];
    const end = Math.max(block.node.end, last.offset + last.text.length);
    return conversionRequest<ConfigForm>("/config-form", {content:file.content.slice(header.offset, end)}, block.node.line);
  }
  function attachComments(value: {comments?: ConfigComment[]}, block: typeof blocks[number]) {
    // ReadForm has already restored the comments covered by a versioned export.
    const header = block.comments.findIndex(c => c.text.startsWith("# nginx-web-form-v1 "));
    const ordinary = header < 0 ? block.comments : block.comments.slice(0, header);
    if (ordinary.length) value.comments = [...ordinary.map(({offset, ...c}) => c), ...(value.comments ?? [])];
  }
  for (const block of declarations) {
    const {node} = block;
    const name = node.words[1];
    const annotated = await readForm(block);
    if (annotated && annotated.kind !== "upstreams") throw new ConfigLineError(node.line, "表单类型与 upstream 不匹配");
    const pool = annotated?.value ?? (managedPoolName.test(name)
      ? rawPool(node).value : namedPool(file.content, node, scope).form.value);
    if (pool.protocol !== scope || poolIDs.has(name.toLowerCase()))
      throw new ConfigLineError(node.line, `upstream ${name} 的协议不符或名称重复`);
    pool.nginx_name = name;
    attachComments(pool, block);
    poolIDs.set(name.toLowerCase(), pool.id);
    pools.push(pool);
  }
  for (const block of sites) {
    const {node} = block;
    if (node.words.join(" ") !== "server" || !node.children)
      throw new ConfigLineError(node.line, `代理规则表单无法表示独立的 ${node.words.join(" ")}`);
    const form = await readForm(block);
    if (form && (stream ? form.kind !== "streams" : form.kind !== "rules"))
      throw new ConfigLineError(node.line, "表单类型与文件扩展名不匹配");
    if (stream) {
      const rule = form?.kind === "streams" ? {...form.value, enabled:true} : rawStream(node, state, true, poolIDs).value;
      attachComments(rule, block);
      streams.push(rule);
    } else {
      const raw = file.content.slice(node.start, node.end);
      if (usesTrackingVariable(node) && !node.children.some(d => d.words[0] === "set" && d.words[1] === "$fnproxy_rule_id"))
        throw new ConfigLineError(node.line, "此站点依赖原有的 $fnproxy_rule_id");
      const rule: HTTPRule = form?.kind === "rules" ? {...form.value, enabled:true} : importBlocks(raw, "", node.line, poolIDs, true).rules![0];
      attachComments(rule, block);
      rules.push(rule);
    }
  }
  const draft = {new_file_name: file.name, pools, rules, streams};
  const expected = await previewBatch(draft, sites.map(b => b.node));
  const ordered = [...declarations, ...sites];
  if (expected.length !== ordered.length) throw Error("表单预览返回的配置块数量不一致");
  for (const [i, {node, comments}] of ordered.entries()) {
    const emittedComments: string[] = [];
    const rendered = parseConfig(expected[i], 1, offset => {
      const end = expected[i].indexOf("\n", offset);
      emittedComments.push(expected[i].slice(offset, end < 0 ? undefined : end).replace(/\r$/, ""));
    });
    const originalComments = comments.filter(c => !c.text.startsWith("# nginx-web-form-v1 ")).map(c => c.text);
    if (JSON.stringify(originalComments.sort()) !== JSON.stringify(emittedComments.sort()))
      throw new ConfigLineError(node.line, "表单未能完整保留配置注释");
    if (rendered.length !== 1) throw new ConfigLineError(node.line, "表单生成的配置块数量不符");
    const raw = file.content.slice(node.start, node.end);
    if (i < pools.length) {
      const pool = pools[i];
      const name = "fnproxy_" + (stream ? "stream_" : "up_") + pool.id;
      const canonical = (n: Block) => rawPool({...n, words:["upstream", name]}).value;
      if (rendered[0].words.join(" ") !== node.words.join(" ") ||
          JSON.stringify(canonical(node)) !== JSON.stringify(canonical(rendered[0])))
        throw new ConfigLineError(node.line, `表单会改写 upstream ${pool.name} 的定义`);
    } else if (!stream || !rawFormEquivalent("streams", raw, expected[i])) {
      compareBlock(node, rendered[0]);
    }
  }
  return {...draft, expected};
}

export async function createCustomConfig(file: CustomConfig, state: State) {
  let preview;
  try {
    preview = await previewFile(file, state);
  } catch (e) {
    if (!(e instanceof ConfigLineError)) throw e;
    const saved = await request<CustomConfig>("/custom-configs", {method: "POST", body: jsonBody(file)});
    return {kind: "file" as const, name: saved.name, reason: e.message};
  }
  await request("/config-import/save", {method: "POST", body: jsonBody(preview)});
  return {kind: "rules" as const, count: preview.rules.length + preview.streams.length, pools: preview.pools.length};
}
