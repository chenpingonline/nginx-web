import type { CustomConfig, LocationSettings } from "./types";

interface Directive {
  words: string[];
  argsStart: number;
  argsEnd: number;
  children?: Directive[];
  start: number;
  line: number;
}
function fail(line: number, reason: string): never {
  throw new Error(`第 ${line} 行 · ${reason}`);
}

// Keep file offsets for opening the original block. Only unquoted delimiters
// end directives; variable braces, escapes and comments are part of the lexer.
export function parseConfig(source: string): Directive[] {
  if (source.length > 1048576) throw new Error("配置不能超过 1 MB");
  const root: Directive[] = [],
    stack: Directive[][] = [root];
  let words: string[] = [],
    word = "",
    quote = "",
    active = false;
  let line = 1,
    start = 0,
    startLine = 1,
    variable = false;
  let argsStart = 0, argsEnd = 0;
  function finish(end: number) {
    if (active) {
      words.push(word);
      argsEnd = end;
    }
    word = "";
    active = false;
  }
  for (let i = 0; i < source.length; i++) {
    const c = source[i];
    if (!quote && !variable && c === "#" && !active) {
      while (i + 1 < source.length && source[i + 1] !== "\n") i++;
      continue;
    }
    if (!quote && !variable && /\s/.test(c)) {
      finish(i);
      if (c === "\n") line++;
      continue;
    }
    if (!active && !words.length) {
      start = i;
      startLine = line;
    }
    if (!quote && !variable && c === "{" && word.endsWith("$")) variable = true;
    else if (!quote && !variable && "{};".includes(c)) {
      finish(i);
      if (c === "}") {
        if (words.length || stack.length === 1)
          fail(line, "多余的右括号或缺少分号");
        stack.pop();
      } else {
        if (!words.length) fail(line, "缺少指令名称");
        const node: Directive = { words, start, line: startLine, argsStart, argsEnd };
        stack[stack.length - 1].push(node);
        if (c === "{") {
          if (stack.length > 128) fail(line, "配置嵌套过深");
          node.children = [];
          stack.push(node.children);
        }
        words = [];
      }
      continue;
    } else if (variable && c === "}") variable = false;
    if (!active && words.length === 1) argsStart = i;
    active = true;
    if (c === "\\") {
      if (++i === source.length) fail(line, "末尾转义不完整");
      word += c + source[i];
      if (source[i] === "\n") line++;
    } else if (quote && c === quote) quote = "";
    else if (!quote && (c === "'" || c === '"')) quote = c;
    else {
      word += c;
      if (c === "\n") line++;
    }
  }
  if (quote || variable || active || words.length || stack.length !== 1)
    fail(line, "配置块未闭合或缺少分号");
  return root;
}

export interface SourceEntry {
  file: CustomConfig;
  line: number;
  start: number;
  title: string;
  entrances: string[];
  targets: string[];
  listeners: string[];
  routes: string[];
  protocols: string[];
  error?: string;
}
export function sourceEntries(files: CustomConfig[]): SourceEntry[] {
  return files.flatMap((file) => {
    const entries: SourceEntry[] = [];
    try {
      for (const block of parseConfig(file.content)) {
        if (block.words[0] !== "server" || !block.children) continue;
        const values = (name: string) =>
          block
            .children!.filter((n) => n.words[0] === name)
            .map((n) => n.words.slice(1).join(" "));
        const listens = values("listen");
        const stream = file.name.endsWith(".stream");
        const names = block.children
          .filter((node) => node.words[0] === "server_name")
          .flatMap((node) => node.words.slice(1));
        const targets = new Set(values("proxy_pass"));
        const routes: string[] = [];
        function collectRoutes(nodes: Directive[]) {
          for (const node of nodes) {
            if (node.words[0] !== "location") continue;
            const passes = (node.children || [])
              .filter((child) => child.words[0] === "proxy_pass")
              .map((child) => child.words.slice(1).join(" "));
            passes.forEach((target) => targets.add(target));
            routes.push([node.words.join(" "), ...passes].join(" → "));
            if (node.children) collectRoutes(node.children);
          }
        }
        collectRoutes(block.children);
        const entrances = new Set<string>();
        for (const listen of listens) {
          const [address, ...options] = listen.split(/\s+/);
          const protocol = stream ? (options.includes("udp") ? "udp" : "tcp")
            : (options.includes("ssl") ? "https" : "http");
          const port = /^\d+$/.test(address) ? address : /:(\d+)$/.exec(address)?.[1];
          if (stream || !port || !names.length || address.startsWith("unix:")) {
            entrances.add(protocol.toUpperCase() + " · " + address);
          } else {
            for (const name of names) entrances.add(protocol + "://" + name + ":" + port);
          }
        }
        entries.push({
          file,
          line: block.line,
          start: block.start,
          title: values("server_name").join(" ") || file.name,
          entrances: [...entrances],
          targets: [...targets],
          listeners: listens,
          routes,
          protocols: (listens.length ? listens : [""]).map((v) =>
            stream
              ? /\budp\b/.test(v)
                ? "udp"
                : "tcp"
              : /\bssl\b/.test(v)
                ? "https"
                : "http",
          ),
        });
      }
    } catch (e) {
      entries.push({
        file,
        line: Number(/第 (\d+) 行/.exec(String(e))?.[1] || 1),
        start: 0,
        title: file.name,
        entrances: [], targets: [], listeners: [], routes: [],
        protocols: [],
        error: (e as Error).message,
      });
    }
    return entries;
  });
}
const parameterLabels: Record<string, string> = {
  listen: "监听地址与选项", server_name: "域名", location: "匹配路径",
  proxy_pass: "转发地址", proxy_set_header: "转发请求头", proxy_http_version: "转发 HTTP 版本",
  proxy_read_timeout: "读取超时", proxy_connect_timeout: "连接超时", proxy_send_timeout: "发送超时",
  ssl_certificate: "证书路径", ssl_certificate_key: "私钥路径", client_max_body_size: "请求体大小上限",
  http2: "HTTP/2", return: "返回响应", root: "站点目录", alias: "路径别名",
};
interface Parameter {
  label: string;
  start: number;
  end: number;
  original: string;
  value: string;
}
export function parameterGroups(entry: SourceEntry) {
  const source = entry.file.content;
  const groups: { title: string; fields: Parameter[] }[] = [];
  const server = parseConfig(source).find(node => node.start === entry.start && node.words[0] === "server");
  function visit(block: Directive) {
    const fields: Parameter[] = [];
    const nodes = block.children ?? [];
    for (const node of [block, ...nodes.filter(n => !n.children)]) {
      const name = node.words[0];
      if (!Object.prototype.hasOwnProperty.call(parameterLabels, name) || node.words.length < 2) continue;
      const original = source.slice(node.argsStart, node.argsEnd);
      // Multiline arguments may contain comments between tokens. Leave them in the source editor.
      if (/[\r\n]/.test(original)) continue;
      fields.push({ label: parameterLabels[name], start: node.argsStart, end: node.argsEnd, original, value: original });
    }
    if (fields.length) groups.push({ title: block.words.join(" "), fields });
    for (const node of nodes) if (node.children && node.words[0] === "location") visit(node);
  }
  if (server) visit(server);
  return groups;
}
export function editParameters(source: string, fields: Parameter[]) {
  let result = source;
  const changed = fields.filter(f => f.original !== f.value).sort((a, b) => b.start - a.start);
  for (const field of changed) {
    const value = field.value.trim();
    try {
      const nodes = parseConfig("param " + value + ";");
      if (nodes.length !== 1 || nodes[0].children || nodes[0].words.length < 2 ||
          nodes[0].argsStart !== 6 || nodes[0].argsEnd !== 6 + value.length || /[\r\n]/.test(value)) throw Error();
    } catch {
      throw new Error(field.label + "需要填写一条指令的参数");
    }
    result = result.slice(0, field.start) + value + result.slice(field.end);
  }
  return result;
}

export function matchSource(
  entry: SourceEntry,
  search: string,
  protocol: string,
  enabled: string,
) {
  return (
    enabled !== "disabled" &&
    (protocol === "all" || entry.protocols.includes(protocol)) &&
    [entry.file.name, entry.title, ...entry.entrances, ...entry.targets, ...entry.listeners, ...entry.routes, entry.error]
      .join(" ")
      .toLowerCase()
      .includes(search.trim().toLowerCase())
  );
}

function newID() {
  return Array.from(crypto.getRandomValues(new Uint8Array(8)), (n) =>
    n.toString(16).padStart(2, "0"),
  ).join("");
}
function unsupported(node: Directive): never {
  return fail(node.line, `请保留为原文：${node.words.join(" ")}`);
}
const timeoutKeys = new Map<
  string,
  "read_timeout_seconds" | "connect_timeout_seconds" | "send_timeout_seconds"
>([
  ["proxy_read_timeout", "read_timeout_seconds"],
  ["proxy_connect_timeout", "connect_timeout_seconds"],
  ["proxy_send_timeout", "send_timeout_seconds"],
]);
function convertLocation(node: Directive) {
  const [, ...args] = node.words;
  const path = args[args.length - 1];
  const match =
    args.length === 1
      ? "prefix"
      : args[0] === "="
        ? "exact"
        : args[0] === "~"
          ? "regex"
          : undefined;
  if (
    !match ||
    !node.children ||
    args.length > 2 ||
    !path ||
    /[\\$`]/.test(path)
  )
    unsupported(node);
  const settings: Partial<LocationSettings> = { backend_type: "proxy" };
  const seen = new Set<string>();
  for (const d of node.children) {
    const [name, value] = d.words;
    if (name === "add_header" || name === "proxy_set_header") {
      const [, header, content, flag] = d.words;
      const response = name === "add_header";
      if (
        d.children ||
        d.words.length < 3 ||
        d.words.length > 4 ||
        (flag !== undefined && (!response || flag !== "always")) ||
        /[\\]/.test(content) ||
        !/^[A-Za-z0-9-]+$/.test(header)
      )
        unsupported(d);
      if (
        !response &&
        /^(host|x-real-ip|x-forwarded-(?:for|proto|host|port)|upgrade|connection)$/i.test(
          header,
        )
      )
        unsupported(d);
      const key = response ? "response_headers" : "request_headers";
      (settings[key] ??= []).push({
        name: header,
        value: content,
        always: flag === "always",
      });
      continue;
    }
    if (d.children || d.words.length !== 2 || seen.has(name)) unsupported(d);
    seen.add(name);
    if (name === "proxy_pass") {
      const target =
        /^(https?):\/\/(\[[a-fA-F0-9:]+\]|[a-zA-Z0-9.-]+)(?::(\d+))?$/.exec(
          value,
        );
      // Bare names may refer to an upstream group in a different file.
      if (
        !target ||
        (!target[3] && !target[2].includes(".") && !target[2].startsWith("["))
      )
        unsupported(d);
      settings.upstream_scheme = target[1] as "http" | "https";
      settings.upstream_host = target[2].replace(/^\[|\]$/g, "");
      settings.upstream_port = Number(
        target[3] ?? (target[1] === "https" ? 443 : 80),
      );
    } else if (name === "expires") settings.expires = value;
    else {
      const key = timeoutKeys.get(name);
      const duration = /^(\d+)(s|m|h)?$/.exec(value);
      if (!key || !duration || !Number(duration[1])) unsupported(d);
      settings[key] =
        Number(duration[1]) *
        (duration[2] === "h" ? 3600 : duration[2] === "m" ? 60 : 1);
    }
  }
  if (!settings.upstream_host)
    fail(node.line, "当前只支持转换含 proxy_pass 的路径");
  return { id: newID(), name: path, path, match, enabled: true, settings };
}
export function importBlocks(source: string, target: string) {
  const blocks = parseConfig(source);
  if (!blocks.length) throw new Error("请输入 server 或 location 配置块");
  if (blocks.length === 1 && blocks[0].words[0] === "location") {
    if (!target) throw new Error("请选择目标代理");
    return { target, location: convertLocation(blocks[0]) };
  }
  const rules = blocks.map((block) => {
    if (block.words.join(" ") !== "server" || !block.children)
      unsupported(block);
    let port = 0,
      domains: string[] = [];
    const locations: ReturnType<typeof convertLocation>[] = [];
    for (const d of block.children) {
      if (d.words[0] === "location") locations.push(convertLocation(d));
      else if (
        d.words[0] === "listen" &&
        !d.children &&
        !port &&
        d.words.length === 2 &&
        /^[1-9]\d*$/.test(d.words[1])
      )
        port = Number(d.words[1]);
      else if (
        d.words[0] === "server_name" &&
        !d.children &&
        !domains.length &&
        d.words.length > 1
      )
        domains = d.words.slice(1);
      else unsupported(d);
    }
    const roots = locations.filter(
      (l) => l.path === "/" && l.match === "prefix",
    );
    if (!port || !domains.length || roots.length !== 1)
      fail(
        block.line,
        "结构化导入需要明确的 listen、server_name 和一个根代理路径",
      );
    const root = roots[0].settings;
    return {
      id: newID(),
      enabled: true,
      name: domains[0],
      domains,
      listen_port: port,
      upstream_host: root.upstream_host,
      upstream_port: root.upstream_port,
      upstream_scheme: root.upstream_scheme,
      root_location: root,
      locations: locations.filter((l) => l !== roots[0]),
    };
  });
  return { rules };
}
