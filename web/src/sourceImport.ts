import { formMetadataStart, parseConfig, sourceEntries, type SourceEntry } from "./nginxConfig";
import type { CustomConfig } from "./types";

type Scope = "http" | "stream";
type Node = ReturnType<typeof parseConfig>[number];
type Context = "global" | "server" | "location" | "server-if" | "location-if" | "limit_except" | "upstream" | "data" | "split_clients" | "module";
type Match = "plain" | "advanced" | "invalid";

const globalBlocks = new Set(["map", "geo", "split_clients", "types"]);
const globalDirectives = new Set([
  "access_log", "error_log", "log_format", "resolver", "resolver_timeout", "limit_conn_zone",
  "variables_hash_bucket_size", "variables_hash_max_size", "server_names_hash_bucket_size", "server_names_hash_max_size",
  "map_hash_bucket_size", "map_hash_max_size", "open_log_file_cache", "tcp_nodelay",
  "allow", "deny", "set_real_ip_from", "limit_conn", "limit_conn_dry_run", "limit_conn_log_level",
  "proxy_bind", "proxy_connect_timeout", "proxy_next_upstream", "proxy_next_upstream_timeout", "proxy_next_upstream_tries",
  "proxy_socket_keepalive", "proxy_ssl_ciphers", "proxy_ssl_certificate", "proxy_ssl_certificate_key", "proxy_ssl_name",
  "proxy_ssl_protocols", "proxy_ssl_server_name", "proxy_ssl_session_reuse",
  "proxy_ssl_trusted_certificate", "proxy_ssl_verify", "proxy_ssl_verify_depth", "proxy_ssl_conf_command",
  "ssl_certificate", "ssl_certificate_key", "ssl_ciphers", "ssl_conf_command", "ssl_dhparam", "ssl_ecdh_curve",
  "ssl_prefer_server_ciphers", "ssl_protocols", "ssl_session_cache", "ssl_session_tickets", "ssl_session_timeout",
  "ssl_client_certificate", "ssl_trusted_certificate", "ssl_verify_client", "ssl_verify_depth",
]);
const httpGlobal = new Set([
  "limit_req_zone", "proxy_cache_path", "fastcgi_cache_path", "uwsgi_cache_path", "scgi_cache_path",
  "add_header", "add_trailer", "add_header_inherit", "default_type", "sendfile", "sendfile_max_chunk",
  "keepalive_timeout", "keepalive_requests", "keepalive_time", "client_max_body_size",
  "client_body_buffer_size", "client_body_temp_path", "client_body_timeout", "client_header_buffer_size",
  "client_header_timeout", "large_client_header_buffers", "ignore_invalid_headers", "underscores_in_headers",
  "gzip", "gzip_buffers", "gzip_comp_level", "gzip_disable", "gzip_http_version", "gzip_min_length",
  "gzip_proxied", "gzip_static", "gzip_types", "gzip_vary", "gunzip", "gunzip_buffers",
  "proxy_set_header", "proxy_set_body", "proxy_read_timeout", "proxy_send_timeout", "proxy_http_version",
  "proxy_headers_hash_bucket_size", "proxy_headers_hash_max_size", "proxy_hide_header", "proxy_pass_header",
  "proxy_buffering", "proxy_buffers", "proxy_buffer_size", "proxy_busy_buffers_size", "proxy_request_buffering",
  "proxy_cache", "proxy_cache_key", "proxy_cache_valid", "proxy_cache_use_stale", "proxy_cache_revalidate", "proxy_cache_lock",
  "proxy_cache_bypass", "proxy_no_cache", "proxy_temp_path", "proxy_temp_file_write_size", "proxy_max_temp_file_size",
  "real_ip_header", "real_ip_recursive", "server_tokens", "server_name_in_redirect", "port_in_redirect", "absolute_redirect",
  "tcp_nopush", "send_timeout", "reset_timedout_connection", "root", "index", "autoindex", "error_page",
  "charset", "charset_types", "types_hash_max_size", "types_hash_bucket_size", "etag", "expires",
  "open_file_cache", "open_file_cache_errors", "open_file_cache_min_uses", "open_file_cache_valid",
  "limit_req", "limit_req_dry_run", "limit_req_log_level", "limit_req_status", "limit_conn_status",
  "http2", "ssl_stapling", "ssl_stapling_verify",
]);
const streamGlobal = new Set([
  "ssl_preread", "proxy_timeout", "proxy_buffer_size", "proxy_download_rate", "proxy_upload_rate", "proxy_half_close",
  "proxy_protocol", "proxy_requests", "proxy_responses", "proxy_session_drop", "proxy_ssl", "preread_buffer_size",
  "preread_timeout", "proxy_protocol_timeout", "ssl_handshake_timeout",
]);
const mainDirectives = new Set([
  "http", "stream", "events", "mail", "load_module", "user", "daemon", "master_process", "env", "pid",
  "lock_file", "worker_processes", "worker_connections", "worker_cpu_affinity", "worker_priority",
  "worker_rlimit_core", "worker_rlimit_nofile", "worker_shutdown_timeout", "working_directory",
  "accept_mutex", "accept_mutex_delay", "multi_accept", "use", "debug_connection",
  "error_log_memory", "thread_pool", "pcre_jit", "timer_resolution", "ssl_engine",
]);

// Check where the pasted text can be inserted. Module-specific validation of
// the resulting file, including files referenced by include, belongs to nginx.
function inspect(nodes: Node[], context: Context, scope: Scope, knownGlobalContext = false): Match {
  let match: Match = "plain";
  for (const node of nodes) {
    const { words, children } = node;
    const [name, value = ""] = words;
    let next: Context | undefined;
    let advanced = false;
    if (name === "include") {
      if (children || words.length !== 2 || !value || context === "split_clients") return "invalid";
      advanced = true;
    } else if (context === "data" || context === "split_clients") {
      // Map keys and MIME types are values, even when they spell a directive.
      if (children) return "invalid";
    } else if (mainDirectives.has(name)) {
      return "invalid";
    } else if (name === "server") {
      if (context === "upstream" && !children && value) advanced = /^unix:/i.test(value);
      else if (context === "global" && children && words.length === 1) next = "server";
      else return "invalid";
    } else if (name === "upstream") {
      if (context !== "global" || !children || words.length !== 2 || !value) return "invalid";
      next = "upstream";
    } else if (name === "location") {
      if (scope !== "http" || !children || !value || (context !== "server" && context !== "location")) return "invalid";
      next = "location";
    } else if (name === "if") {
      if (scope !== "http" || !children || !value.startsWith("(") || !words.at(-1)?.endsWith(")") ||
          (context !== "server" && context !== "location")) return "invalid";
      next = context === "location" ? "location-if" : "server-if";
      advanced = true;
    } else if (name === "limit_except") {
      if (scope !== "http" || !children || !value || context !== "location") return "invalid";
      next = "limit_except";
    } else if (globalBlocks.has(name)) {
      const types = name === "types";
      if (!children || (types && (scope !== "http" || !["global", "server", "location"].includes(context))) ||
          (!types && context !== "global")) return "invalid";
      next = name === "split_clients" ? "split_clients" : "data";
    } else if (name === "proxy_pass") {
      const variable = /\$(?:\w|\{)/.test(value);
      const valid = scope === "stream" ? context === "server" && !/^https?:\/\//i.test(value)
        : ["location", "location-if", "limit_except"].includes(context) && (variable || /^https?:\/\//i.test(value));
      if (!valid || children || words.length !== 2 || !value) return "invalid";
      advanced = variable || /^(?:https?:\/\/)?unix:/i.test(value);
    } else if (scope === "stream" && name === "return" && words.length !== 2) {
      return "invalid";
    } else if (name === "listen") {
      if (context !== "server" || !value || children || (scope === "http" && words.includes("udp"))) return "invalid";
      advanced = /^unix:/i.test(value);
    } else if (context === "global") {
      const scoped = scope === "http" ? httpGlobal : streamGlobal;
      if (!globalDirectives.has(name) && !scoped.has(name)) {
        const other = scope === "http" ? streamGlobal : httpGlobal;
        // A wrapper or a sibling site supplies the context of module directives
        // unknown to the managed form. Nginx checks module availability on apply.
        if (!knownGlobalContext || other.has(name) ||
            ["set", "return", "rewrite", "break", "server_name", "alias", "try_files"].includes(name)) return "invalid";
        if (children) next = "module";
      } else if (children) return "invalid";
    } else {
      const local = scope === "http" ? httpGlobal : streamGlobal;
      const other = scope === "http" ? streamGlobal : httpGlobal;
      if ((other.has(name) && !local.has(name)) ||
          (scope === "stream" && ["alias", "try_files", "rewrite"].includes(name))) return "invalid";
      if (children) next = "module";
    }
    if (next && children) {
      const child = inspect(children, next, scope);
      if (child === "invalid") return child;
      advanced ||= child === "advanced";
    }
    if (advanced) match = "advanced";
  }
  return match;
}

export function sourceImportMode(nodes: Node[], scope: Scope, fallback = false, wrapped = false): "file" | "server-fragment" | undefined {
  if (!nodes.length) return;
  const fragment = nodes.some(n => n.words[0] === "location" || n.words[0] === "if");
  if (fragment && (scope !== "http" || !nodes.every(n => ["location", "if", "include", "set", "rewrite", "return", "break"].includes(n.words[0])))) return;
  const hasSite = nodes.some(n => n.words[0] === "server" && n.children);
  const match = inspect(nodes, fragment ? "server" : "global", scope, wrapped || hasSite);
  // Stream routing declarations belong to the stream include. The managed
  // settings form generates routing variables in the HTTP context only.
  const streamRouting = scope === "stream" && nodes.some(n => ["map", "geo", "split_clients"].includes(n.words[0]));
  if (match === "advanced" || (match === "plain" && (fallback || streamRouting))) {
    return fragment ? "server-fragment" : "file";
  }
}

export function sourceTargets(files: CustomConfig[]): SourceEntry[] {
  return files.filter(file => /^custom-[A-Za-z0-9][A-Za-z0-9._-]{0,71}\.conf$/.test(file.name)).flatMap(file => {
    const hasMetadata = file.content.includes("# nginx-web-form-v1 ");
    return sourceEntries([file]).filter(entry => !entry.error && (!hasMetadata || formMetadataStart(file.content, entry.start) === undefined));
  });
}

export function targetKey(entry: SourceEntry): string {
  return entry.file.name + ":" + entry.start;
}

interface SourcePlan {
  entry: { file: CustomConfig; title: string; start?: number };
  baseFile?: CustomConfig;
}

export function sourceImportPlan(text: string, nodes: Node[], scope: Scope, files: CustomConfig[], target: string | SourceEntry, wrapped = false): SourcePlan {
  const mode = sourceImportMode(nodes, scope, true, wrapped);
  if (!mode) throw Error("这段配置无法放入所选作用域");
  if (mode === "file") {
    const extension = scope === "http" ? ".conf" : ".stream";
    const names = new Set(files.map(f => f.name.toLowerCase()));
    let name = "custom-import" + extension;
    for (let n = 2; names.has(name); n++) name = "custom-import-" + n + extension;
    return { entry: { file: { name, content: text }, title: name } };
  }
  const key = typeof target === "string" ? target : targetKey(target);
  const entry = sourceTargets(files).find(candidate => targetKey(candidate) === key);
  if (!entry) throw Error("请选择目标自定义 HTTP 站点，或粘贴完整 server 块");
  const baseFile = entry.file;
  const server = parseConfig(baseFile.content).find(n => n.start === entry.start)!;
  const closing = server.end - 1;
  // Insert before indentation on a closing-brace line, preserving both the
  // pasted fragment and the original site's comments, whitespace and siblings.
  const content = baseFile.content;
  const newline = content.includes("\r\n") ? "\r\n" : "\n";
  const lineStart = content.lastIndexOf("\n", closing - 1) + 1;
  const at = /^[ \t]*$/.test(content.slice(lineStart, closing)) ? lineStart : closing;
  const before = content.slice(0, at);
  const inserted = (before.endsWith("\n") ? "" : newline) + text + (text.endsWith("\n") ? "" : newline);
  return { baseFile, entry: { title: entry.title, start: entry.start,
    file: { name: baseFile.name, content: before + inserted + content.slice(at) } } };
}
