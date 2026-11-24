import type { CustomConfig, LocationSettings } from "./types";

interface Directive {
  words: string[];
  end: number;
  argsStart: number;
  argsEnd: number;
  children?: Directive[];
  start: number;
  line: number;
}
export class ConfigLineError extends Error {
  readonly line: number;
  constructor(line: number, reason: string) {
    super(`第 ${line} 行 · ${reason}`);
    this.line = line;
  }
}
function fail(line: number, reason: string): never {
  throw new ConfigLineError(line, reason);
}

// Keep file offsets for opening the original block. Only unquoted delimiters
// end directives; variable braces, escapes and comments are part of the lexer.
export function parseConfig(source: string, firstLine = 1): Directive[] {
  if (source.length > 1048576) throw new Error("配置不能超过 1 MB");
  const root: Directive[] = [],
    stack: Directive[][] = [root];
  const openBlocks: Directive[] = [];
  let words: string[] = [],
    word = "",
    quote = "",
    active = false;
  let line = firstLine,
    start = 0,
    startLine = firstLine,
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
        openBlocks.pop()!.end = i + 1;
      } else {
        if (!words.length) fail(line, "缺少指令名称");
        const node: Directive = { words, start, end: i + 1, line: startLine, argsStart, argsEnd };
        stack[stack.length - 1].push(node);
        if (c === "{") {
          if (stack.length > 128) fail(line, "配置嵌套过深");
          node.children = [];
          stack.push(node.children);
          openBlocks.push(node);
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
        line: e instanceof ConfigLineError ? e.line : 1,
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
  expires: "缓存头 Expires", proxy_buffering: "响应缓冲", proxy_request_buffering: "请求缓冲",
  http2: "HTTP/2", return: "返回响应", root: "站点目录", alias: "路径别名",
};
export interface Parameter {
  readonly: boolean;
  directive: string;
  label: string;
  start: number;
  end: number;
  original: string;
  value: string;
}
export function parameterGroups(entry: { file: CustomConfig; start?: number }) {
  const source = entry.file.content;
  const roots = parseConfig(source);
  const groups: { title: string; line: number; fields: Parameter[] }[] = [];
  const dataBlocks = new Set(["map", "geo", "types", "split_clients"]);
  function field(node: Directive, dataRow: boolean): Parameter {
    const name = node.words[0];
    const noArgs = node.words.length === 1;
    const start = dataRow || noArgs ? node.start : node.argsStart;
    const original = source.slice(start, node.argsEnd);
    return {
      directive: dataRow ? "" : name,
      label: dataRow ? "映射条目" : Object.hasOwn(parameterLabels, name) ? parameterLabels[name] : name,
      readonly: noArgs && !dataRow, start, end: node.argsEnd, original, value: original,
    };
  }
  function visit(nodes: Directive[], path: string[], block?: Directive) {
    const fields: Parameter[] = [];
    if (block && block.words.length > 1) fields.push(field(block, false));
    for (const node of nodes) {
      if (!node.children) fields.push(field(node, !!block && dataBlocks.has(block.words[0])));
    }
    if (fields.length) groups.push({ title: path.join(" › ") || "文件指令", line: block?.line ?? 1, fields });
    for (const child of nodes) {
      if (child.children) visit(child.children, [...path, child.words.join(" ")], child);
    }
  }
  if (entry.start === undefined) visit(roots, []);
  else {
    const block = roots.find(node => node.start === entry.start);
    if (block?.children) visit(block.children, [block.words.join(" ")], block);
  }
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
          nodes[0].argsStart !== 6 || nodes[0].argsEnd !== 6 + value.length) throw Error();
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

export function newID() {
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
const commonParams = ["QUERY_STRING $query_string", "REQUEST_METHOD $request_method", "CONTENT_TYPE $content_type", "CONTENT_LENGTH $content_length"];
const requestParams = ["REQUEST_URI $request_uri", "SERVER_PROTOCOL $server_protocol", "REMOTE_ADDR $remote_addr"];
const gatewayParams: Record<string,string[]> = {
  fastcgi: [...commonParams, "SCRIPT_FILENAME $document_root$fastcgi_script_name", ...requestParams, "SERVER_NAME $server_name"],
  uwsgi: [...commonParams, ...requestParams, "SERVER_NAME $server_name"],
  scgi: ["CONTENT_LENGTH $content_length", "SCGI 1", "REQUEST_METHOD $request_method", "REQUEST_URI $request_uri", "QUERY_STRING $query_string", ...requestParams.slice(1)],
};
function convertLocation(node: Directive, staticRoot = "", indexFiles = ["index.html"]) {
  const [, ...args] = node.words;
  const path = args[args.length - 1];
  const modifiers = new Map([["=", "exact"], ["~", "regex"], ["~*", "regex_i"], ["^~", "priority_prefix"]]);
  const match = args.length === 1
    ? (path.startsWith("@") ? "named" : "prefix")
    : modifiers.get(args[0]);
  if (
    !match ||
    !node.children ||
    args.length > 2 ||
    !path ||
    (/[\\`]/.test(path) || path.includes("$") && !(match?.startsWith("regex") && path.indexOf("$") === path.length - 1))
  )
    unsupported(node);
  const settings: Partial<LocationSettings> = { backend_type: "proxy" };
  const handlers = node.children.filter(d => ["proxy_pass","grpc_pass","fastcgi_pass","uwsgi_pass","scgi_pass","memcached_pass","stub_status","return"].includes(d.words[0]));
  if (handlers.length > 1) unsupported(handlers[1]);
  const handler = handlers[0];
  const kind = handler?.words[0];
  const isProxy = kind === "proxy_pass";
  const localRoot = node.children.find(d => d.words[0] === "root" || d.words[0] === "alias");
  if (localRoot) {
    const value = localRoot.words[1];
    if (handler || localRoot.words.length !== 2 || !value.startsWith("/") || value.includes("$")) unsupported(localRoot);
    staticRoot = value;
  }
  if (isProxy) settings.explicit_proxy_headers = true;
  else if (handler) {
    const [name, value, target] = handler.words;
    if (handler.children) unsupported(handler);
    if (name === "return" && /^[2-5]\d{2}$/.test(value) && handler.words.length <= 3) {
      Object.assign(settings, {backend_type: "return", return_code: Number(value), return_target: target ?? ""});
    } else if (name === "stub_status" && handler.words.length === 1) settings.backend_type = "status";
    else if (name.endsWith("_pass") && handler.words.length === 2) {
      const address = /^(?:(grpcs?):\/\/)?(\[[\da-fA-F:]+\]|[\w.-]+)(?::(\d+))?$/.exec(value);
      if (!address || (name === "grpc_pass") !== !!address[1]) unsupported(handler);
      const pool = /^fnproxy_up_([a-f0-9]+)$/.exec(address[2]);
      if (!address[3] && !pool) unsupported(handler);
      Object.assign(settings,{backend_type:name.slice(0,-5),upstream_scheme:address[1] === "grpcs" ? "https" : "http",
        upstream_host:address[2].replace(/^\[|\]$/g,""),upstream_port:Number(address[3] || 0),upstream_pool_id:pool?.[1] ?? ""});
    } else unsupported(handler);
  } else if (staticRoot) Object.assign(settings, {backend_type:"static", static_path:staticRoot, static_alias:localRoot?.words[0] === "alias", index_files:indexFiles});
  else unsupported(node);
  const dav = node.children.find(d => d.words[0] === "dav_methods");
  if (dav) settings.dav = {enabled: true, methods: dav.words.slice(1), create_full_put_path: false, min_delete_depth: 0};
  const hasSubFilter = node.children.some(d => d.words[0] === "sub_filter");
  if (hasSubFilter) settings.sub_filter_once = true;
  const params: string[] = [];
  const seen = new Set<string>();
  for (const d of node.children) {
    const [name, value] = d.words;
    if (d === handler && !isProxy) continue;
    if (!d.children && name === settings.backend_type + "_param" && ["fastcgi","uwsgi","scgi"].includes(settings.backend_type ?? "")) {
      params.push(d.words.slice(1).join(" ")); continue;
    }
    if (!d.children && settings.backend_type === "memcached" && d.words.join(" ") === "set $memcached_key $uri" && !seen.has("memcached_key")) { seen.add("memcached_key"); continue; }
    if (!d.children && (name === "allow" || name === "deny") && d.words.length === 2) {
      if (name === "allow" && settings.deny?.length) unsupported(d);
      (settings[name] ??= []).push(value); continue;
    }
    if (!d.children && name === "rewrite" && d.words.length === 4 && ["last","break","redirect","permanent"].includes(d.words[3])) {
      if (kind === "return" && d.start > handler!.start) unsupported(d);
      (settings.rewrites ??= []).push({pattern:value,replacement:d.words[2],flag:d.words[3] as "last"|"break"|"redirect"|"permanent"}); continue;
    }
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
      if (!response && !isProxy) unsupported(d);
      const key = response ? "response_headers" : "request_headers";
      (settings[key] ??= []).push({
        name: header,
        value: content,
        always: flag === "always",
      });
      continue;
    }
    if (!d.children && name === "sub_filter" && d.words.length === 3) {
      (settings.sub_filters ??= []).push({search: value, replacement: d.words[2]});
      continue;
    }
    if (d.children || seen.has(name)) unsupported(d);
    if (d === dav || name === "valid_referers" || (name === "sub_filter_types" && hasSubFilter)) {
      if (d.words.length < 2) unsupported(d);
      if (name === "valid_referers") settings.valid_referers = d.words.slice(1);
      if (name === "sub_filter_types") settings.sub_filter_types = d.words.slice(1);
      seen.add(name);
      continue;
    }
    if (d === localRoot) {
      seen.add(name);
      continue;
    }
    if (settings.backend_type === "static" && name === "index" && d.words.length > 1) {
      settings.index_files = d.words.slice(1);
      seen.add(name);
      continue;
    }
    if (settings.backend_type === "static" && name === "try_files" && d.words.length >= 3) {
      settings.try_files = d.words.slice(1);
      seen.add(name);
      continue;
    }
    if (d.words.length !== 2) unsupported(d);
    seen.add(name);
    if (name === "autoindex" && settings.backend_type === "static" && ["on","off"].includes(value)) settings.autoindex = value === "on";
    else if (name === "ssi" && ["on","off"].includes(value)) settings.ssi = value === "on";
    else if (name === "sub_filter_once" && hasSubFilter && ["on","off"].includes(value)) settings.sub_filter_once = value === "on";
    else if (settings.dav && name === "create_full_put_path" && ["on","off"].includes(value)) settings.dav.create_full_put_path = value === "on";
    else if (settings.dav && name === "min_delete_depth" && /^\d+$/.test(value)) settings.dav.min_delete_depth = Number(value);
    else if (name === "add_before_body") settings.addition_before = value;
    else if (name === "add_after_body") settings.addition_after = value;
    else if (name === "mirror") { settings.mirror = value; settings.mirror_request_body ??= true; }
    else if (name === "mirror_request_body" && ["on","off"].includes(value) && node.children.some(child => child.words[0] === "mirror")) settings.mirror_request_body = value === "on";
    else if (name === "auth_request") settings.auth_request = value;
    else if (name === "auth_basic" && value !== "off") Object.assign(settings, {basic_auth:true,basic_auth_realm:value});
    else if (name === "auth_basic_user_file") settings.basic_auth_file = value;
    else if (name === "proxy_ssl_server_name" && isProxy && ["on","off"].includes(value)) settings.proxy_ssl_server_name = value === "on";
    else if (name === "proxy_ssl_name" && isProxy && value === node.children.find(d=>d.words[0]==="proxy_pass")?.words[1]?.match(/^https:\/\/(\[[^\]]+\]|[^:/]+)/)?.[1].replace(/^\[|\]$/g,"")) continue;
    else if (name === "proxy_ssl_verify" && isProxy && value === "off") continue;
    else if (name === "proxy_pass") {
      const target =
        /^(https?):\/\/(\[[a-fA-F0-9:]+\]|[a-zA-Z0-9.-]+)(?::(\d+))?(\/[a-zA-Z0-9/._~+\-]*)?$/.exec(
          value,
        );
      // Bare names may refer to an upstream group in a different file.
      if (
        !target || (["regex", "regex_i", "named"].includes(match) && !!target[4]) ||
        (!target[3] && !target[2].includes(".") && !target[2].startsWith("["))
      )
        unsupported(d);
      settings.upstream_path = target[4] ?? "";
      settings.upstream_scheme = target[1] as "http" | "https";
      settings.upstream_host = target[2].replace(/^\[|\]$/g, "");
      settings.upstream_port = Number(
        target[3] ?? (target[1] === "https" ? 443 : 80),
      );
    } else if (isProxy && name === "proxy_http_version" && (value === "1.1" || value === "1.0")) {
      settings.proxy_http_version = value;
    } else if (name === "expires" && (isProxy || settings.backend_type === "static")) settings.expires = value;
    else {
      const key = timeoutKeys.get(name);
      const duration = /^(\d+)(s|m|h)?$/.exec(value);
      if (!isProxy || !key || !duration || !Number(duration[1])) unsupported(d);
      settings[key] =
        Number(duration[1]) *
        (duration[2] === "h" ? 3600 : duration[2] === "m" ? 60 : 1);
    }
  }

  const defaults = gatewayParams[settings.backend_type ?? ""];
  if (defaults && (params.length !== defaults.length || params.some((p,i) => p !== defaults[i]))) unsupported(node);
  if (settings.backend_type === "memcached" && !seen.has("memcached_key")) unsupported(node);
  if (settings.basic_auth_file && !settings.basic_auth) unsupported(node);
  if (settings.upstream_scheme === "https" && isProxy && settings.proxy_ssl_server_name === undefined)
    settings.proxy_ssl_server_name = false;
  return { id: newID(), name: path, path, match, enabled: true, settings };
}
export function importBlocks(source: string, target: string, firstLine = 1) {
  const blocks = parseConfig(source, firstLine);
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
    const families = new Set<string>();
    const paths: Directive[] = [];
    let tls: boolean | undefined, catchAll: boolean | undefined, http2 = false, cert = "", key = "", tracking = "", redirectPort = 0;
    let staticRoot = "", indexFiles: string[] | undefined, bodyLimit: number | undefined;
    for (const d of block.children) {
      const [name, value] = d.words;
      if (name === "location") paths.push(d);
      else if (d.children) unsupported(d);
      else if (name === "set" && d.words.length === 3 && value === "$fnproxy_rule_id" && !tracking && /^[a-f0-9]{12,64}$/.test(d.words[2])) tracking = d.words[2];
      else if (name === "http2" && value === "on" && d.words.length === 2 && !http2) http2 = true;
      else if (name === "ssl_certificate" && !cert && d.words.length === 2) cert = value;
      else if (name === "ssl_certificate_key" && !key && d.words.length === 2) key = value;
      else if (name === "return" && value === "308" && d.words.length === 3 && !redirectPort) {
        const target = /^https:\/\/\$host(?::([0-9]+))?\$request_uri$/.exec(d.words[2]);
        if (!target) unsupported(d);
        redirectPort = Number(target[1] || 443);
      } else if (name === "listen" && d.words.slice(2).every(option=>option==="ssl" || option==="default_server")) {
        const secure = d.words.includes("ssl"), fallback = d.words.includes("default_server");
        if ((tls !== undefined && tls !== secure) || (catchAll !== undefined && catchAll !== fallback)) unsupported(d);
        tls = secure; catchAll = fallback;
        const address = /^(?:(0\.0\.0\.0|\[::\]):)?([1-9]\d*)$/.exec(value);
        if (!address) unsupported(d);
        const family = address[1] === "[::]" ? "ipv6" : "ipv4";
        if (families.has(family) || (port && port !== Number(address[2]))) unsupported(d);
        families.add(family);
        port = Number(address[2]);
      } else if (name === "root" && d.words.length === 2 && !staticRoot && value.startsWith("/") && !value.includes("$")) {
        staticRoot = value;
      } else if (name === "index" && d.words.length > 1 && !indexFiles) {
        indexFiles = d.words.slice(1);
      } else if (name === "client_max_body_size" && d.words.length === 2 && bodyLimit === undefined && /^(0|[1-9]\d*[mM])$/.test(value)) {
        bodyLimit = parseInt(value, 10);
      }
      else if (
        d.words[0] === "server_name" &&
        !d.children &&
        !domains.length &&
        d.words.length > 1
      )
        domains = d.words.slice(1);
      else unsupported(d);
    }
    const certID = cert.match(/\/([a-f0-9]{12,64})\/fullchain\.pem$/)?.[1];
    if (tls ? !certID || key !== cert.replace(/fullchain\.pem$/, "privkey.pem") : !!(cert || key || http2))
      fail(block.line, "HTTPS 规则需要目标机已导入的证书和匹配的私钥路径");
    if (catchAll && domains.join(" ") !== "_") fail(block.line,"默认站点域名无法完整还原");
    const locations = paths.map(path => convertLocation(path, staticRoot, indexFiles));
    if ((staticRoot || indexFiles) && !locations.some(l => l.settings.backend_type === "static"))
      fail(block.line, "静态目录和首页需要对应的静态文件路径");
    const roots = locations.filter(
      (l) => l.path === "/" && l.match === "prefix",
    );
    if (!port || !domains.length || (redirectPort ? paths.length > 0 : roots.length !== 1))
      fail(
        block.line,
        "结构化导入需要明确的 listen、server_name 和一个根代理路径",
      );
    const root: Partial<LocationSettings> = roots[0]?.settings ?? {};
    return {
      id: tracking || newID(),
      enabled: true, tls: !!tls, http2, certificate_id: certID || "", redirect_to_https: !!redirectPort,
      redirect_https_port: redirectPort || 443, name: domains[0],
      domains: catchAll ? ["*"] : domains,
      listen_port: port,
      listen_type: families.size === 2 ? "dual" : families.has("ipv6") ? "ipv6" : "ipv4",
      client_max_body_mb: bodyLimit ?? 1,
      connect_timeout_seconds: 60, read_timeout_seconds: 60, send_timeout_seconds: 60,
      upstream_host: root.upstream_host ?? "127.0.0.1",
      upstream_port: root.upstream_port ?? 8080,
      upstream_scheme: root.upstream_scheme, upstream_pool_id: root.upstream_pool_id,
      root_location: root,
      locations: locations.filter((l) => l !== roots[0]),
    };
  });
  return { rules };
}

// Compare directives in order. A copied server receives its own tracking ID.
export function ruleDirectives(content: string): string {
  function signature(nodes: Directive[]): unknown[] {
    return nodes.map(({ words, children }) => [
      words[0] === "set" && words[1] === "$fnproxy_rule_id" && words.length === 3 ? words.slice(0, 2) : words,
      children && signature(children),
    ]);
  }
  return JSON.stringify(signature(parseConfig(content)));
}

export function sourceBlock(entry: SourceEntry) {
  const file = entry.file;
  const block = parseConfig(file.content).find(node => node.start === entry.start && node.words[0] === "server");
  if (!block?.children) throw new Error("请选择 HTTP server 块");
  const raw = file.content.slice(block.start, block.end);
  const bytes = new TextEncoder();
  const source = { name: file.name, expected: file.content, start: bytes.encode(file.content.slice(0, block.start)).length, end: bytes.encode(file.content.slice(0, block.end)).length };
  return { raw, source, nodes: block.children };
}

// A versioned export comment belongs to the immediately following server only.
export function formSourceBlock(entry: SourceEntry) {
  const block = sourceBlock(entry);
  const prefix = entry.file.content.slice(0,entry.start);
  const header = /(?:^|\n)((?:# nginx-web-form-v1 [A-Za-z0-9+/]+\r?\n)+)[ \t]*$/.exec(prefix)?.[1];
  if (!header) return {...block, annotated:false};
  const start = prefix.lastIndexOf(header);
  return {...block, annotated:true, raw:entry.file.content.slice(start,entry.start) + block.raw,
    source:{...block.source,start:new TextEncoder().encode(prefix.slice(0,start)).length}};
}
export function replaceSourceRange(source: ReturnType<typeof sourceBlock>["source"], block: string) {
  const encoder = new TextEncoder(), decoder = new TextDecoder("utf-8", {ignoreBOM:true});
  const bytes = encoder.encode(source.expected);
  return { content:decoder.decode(bytes.subarray(0,source.start)) + block + decoder.decode(bytes.subarray(source.end)),
    end:source.start+encoder.encode(block).length };
}

// Source ranges use UTF-8 offsets to match the server's optimistic concurrency check.
export function updateSourceBlock(source: ReturnType<typeof sourceBlock>["source"], rendered: string) {
  const encoder = new TextEncoder();
  const decoder = new TextDecoder("utf-8", { ignoreBOM: true });
  const bytes = encoder.encode(source.expected);
  const original = decoder.decode(bytes.subarray(source.start, source.end));
  const tracking = parseConfig(original)[0].children?.find(n => n.words[0] === "set" && n.words[1] === "$fnproxy_rule_id");
  const trackingLine = tracking ? "    " + original.slice(tracking.start, tracking.end) + "\n" : "";
  const block = rendered.trimEnd().replace(/^    set \$fnproxy_rule_id [^;]+;\n/m, () => trackingLine);
  return replaceSourceRange(source, block);
}

export function sourceRuleCandidate(entry: SourceEntry, rules: import("./types").ProxyRule[]) {
  const { raw, source, nodes } = sourceBlock(entry);
  if (raw.includes("#")) fail(entry.line + raw.slice(0, raw.indexOf("#")).split("\n").length - 1, "配置块含注释或特殊字符，请编辑源文件以保留原文");
  const args = (name: string) => nodes.find(n => n.words[0] === name)?.words.slice(1) ?? [];
  const originalID = nodes.find(n => n.words[0] === "set" && n.words[1] === "$fnproxy_rule_id")?.words[2];
  if (!originalID) {
    const candidate = importBlocks(raw, "", entry.line).rules?.[0];
    if (!candidate) throw new Error("无法还原为代理规则");
    return { source, raw, rule: candidate, generated: false };
  }
  const tracking = nodes.find(n => n.words[0] === "set" && n.words[1] === "$fnproxy_rule_id")!;
  // Generated static sites carry their root and proxy settings in each location.
  if (nodes.some(n => n.words[0] === "location" && n.children?.some(d => d.words[0] === "root"))) {
    const offset = entry.start!;
    const withoutTracking = raw.slice(0, tracking.start - offset) + raw.slice(tracking.start - offset, tracking.end - offset).replace(/[^\n]/g, " ") + raw.slice(tracking.end - offset);
    try {
      const candidate = importBlocks(withoutTracking, "", entry.line).rules?.[0];
      if (candidate) return { source, raw, rule: candidate, generated: true };
    } catch {
      // TLS and other managed directives can still be restored from the original rule below.
    }
  }
  const original = rules.find(r => r.id === originalID);
  const listens = nodes.filter(n => n.words[0] === "listen");
  const addresses = listens.map(n => n.words[1]);
  const port = Number(addresses[0]?.split(":").pop());
  const domains = args("server_name");
  const root = nodes.find(n => n.words[0] === "location" && n.words[1] === "/")?.children ?? [];
  const directive = (name: string) => root.find(n => n.words[0] === name)?.words[1];
  const target = /^(https?):\/\/(\[[\da-f:]+\]|[\w.-]+):(\d+)(\/[a-zA-Z0-9/._~+\-]*)?$/i.exec(directive("proxy_pass") || "");
  const backend = target ? { upstream_scheme: target[1], upstream_host: target[2].replace(/^\[|\]$/g, ""), upstream_port: Number(target[3]), upstream_pool_id: "" } : {};
  const cert = args("ssl_certificate")[0]?.match(/\/([^/]+)\/fullchain\.pem$/)?.[1] ?? "";
  const rule = {
    ...original, id: newID(), group_id: "", inherit_fields: [], enabled: true, domains, name: domains[0],
    listen_port: port, listen_type: addresses.length === 2 ? "dual" : addresses[0]?.startsWith("[") ? "ipv6" : "ipv4",
    tls: listens.some(n => n.words.includes("ssl")), http2: args("http2")[0] === "on", certificate_id: cert,
    client_max_body_mb: Number.parseInt(args("client_max_body_size")[0] || "0", 10),
    preserve_host: root.some(n => n.words.join(" ") === "proxy_set_header Host $fnproxy_client_host"),
    websocket: root.some(n => n.words.join(" ") === "proxy_set_header Upgrade $http_upgrade"),
    streaming: directive("proxy_request_buffering") === "off",
    verify_upstream_tls: directive("proxy_ssl_verify") === "on",
    connect_timeout_seconds: parseInt(directive("proxy_connect_timeout") || "0", 10),
    read_timeout_seconds: parseInt(directive("proxy_read_timeout") || "0", 10),
    send_timeout_seconds: parseInt(directive("proxy_send_timeout") || "0", 10),
    ...backend, root_location: { ...original?.root_location, ...backend, ...(target && { upstream_path: target[4] ?? "" }) },
  };
  return { source, raw, rule, generated: true };
}
