import { parseQuantity } from "./nginxQuantity";
import { ConfigLineError, UnsupportedFormError, parseConfig, newID, importBlocks } from "./nginxConfig";
import type { ConfigForm } from "./configForm";
import type { State, StreamRule, UpstreamPool, UpstreamServer } from "./types";

type Node = ReturnType<typeof parseConfig>[number];
function unsupported(node: Node): never {
  throw new UnsupportedFormError(node.line, `无法完整还原：${node.words.join(" ")}`);
}
function endpoint(value: string, node: Node, defaultPort?: number) {
  const addr = /^(?:\[([\da-f:.]+)\]|([\w.-]+))(?::(\d+))?$/i.exec(value);
  if (!value || !addr || (!addr[3] && defaultPort === undefined)) unsupported(node);
  return { host: addr[1] || addr[2], port: Number(addr[3] ?? defaultPort) };
}

function seconds(value: string, node: Node, allowMilliseconds = false) {
  if (!allowMilliseconds && value.includes("ms")) unsupported(node);
  try {
    return parseQuantity(value, "seconds");
  } catch {
    return unsupported(node);
  }
}

function listenAddress(listen: Node) {
  const match = /^(?:\*:|0\.0\.0\.0:)?(\d+)$/.exec(listen.words[1]);
  return match ? {host: "*", port: Number(match[1])} : endpoint(listen.words[1], listen);
}

export const managedPoolName = /^fnproxy_(up|stream)_([a-f0-9]{12,64})$/;

export function rawPool(node: Node): Extract<ConfigForm, {kind: "upstreams"}> {
  const target = managedPoolName.exec(node.words[1]);
  if (node.words[0] !== "upstream" || !target || node.words.length !== 2 || !node.children) unsupported(node);
  const pool: UpstreamPool = {
    id: target[2],
    name: node.words[1],
    protocol: target[1] === "up" ? "http" : "stream",
    strategy: "round_robin",
    hash_key: "$request_uri",
    keepalive: 0,
    keepalive_requests: 1000,
    keepalive_time_seconds: 3600,
    keepalive_timeout_seconds: 60,
    servers: [],
    created_at: "0001-01-01T00:00:00Z",
    updated_at: "0001-01-01T00:00:00Z",
  };
  const seen = new Set<string>();
  for (const child of node.children) {
    const [name, arg, ...rest] = child.words;
    if (child.children) unsupported(child);
    const strategy = ["least_conn", "ip_hash", "hash", "random"].includes(name);
    const key = strategy ? "strategy" : name;
    if (name !== "server" && seen.has(key)) unsupported(child);
    if (pool.protocol === "stream" && (name.startsWith("keepalive") || name === "ip_hash")) unsupported(child);
    seen.add(key);
    if (name === "server") {
      const server: UpstreamServer = {
        ...endpoint(arg, child, pool.protocol === "http" ? 80 : undefined),
        weight: 1,
        max_fails: 1,
        fail_timeout_seconds: 10,
        backup: false,
        down: false,
      };
      const options = new Set<string>();
      for (const option of rest) {
        const [key, value, extra] = option.split("=");
        if (options.has(key) || extra !== undefined) unsupported(child);
        options.add(key);
        if (option === "backup" || option === "down") server[option] = true;
        else {
          if ((key === "weight" || key === "max_fails") && /^[1-9]\d*$/.test(value))
            server[key] = Number(value);
          else if (key === "fail_timeout")
            server.fail_timeout_seconds = seconds(value, child);
          else unsupported(child);
        }
      }
      if (server.fail_timeout_seconds === 0) unsupported(child);
      pool.servers.push(server);
    } else if (["least_conn", "ip_hash"].includes(name) && child.words.length === 1)
      pool.strategy = name as "least_conn" | "ip_hash";
    else if (name === "random" && arg === "two" && (rest.length === 0 || (rest.length === 1 && rest[0] === "least_conn")))
      pool.strategy = "random";
    else if (name === "hash" && rest.length === 1 && rest[0] === "consistent") {
      pool.strategy = "hash";
      pool.hash_key = arg;
    } else if (name === "keepalive" && arg === "32" && rest.length === 1 && rest[0] === "local") {
      // Nginx 1.30 uses a per-location cache of 32 when keepalive is omitted.
      pool.keepalive = 0;
    } else if (
      !rest.length &&
      (name === "keepalive" || name === "keepalive_requests") &&
      /^[1-9]\d*$/.test(arg)
    )
      pool[name] = Number(arg);
    else if (
      !rest.length &&
      (name === "keepalive_time" || name === "keepalive_timeout")
    )
      pool[`${name}_seconds`] = seconds(arg, child, true);
    else unsupported(child);
  }
  if (pool.keepalive === 0 && (pool.keepalive_requests !== 1000 ||
      pool.keepalive_time_seconds !== 3600 || pool.keepalive_timeout_seconds !== 60)) {
    // The renderer emits keepalive tuning only with an explicit shared cache.
    unsupported(node);
  }
  return { kind: "upstreams", id: pool.id, value: pool };
}

export function rawStream(node: Node, state: State, preserveOmissions = false, pools: ReadonlyMap<string, string> = new Map()): Extract<ConfigForm, {kind: "streams"}> {
  const nodes = node.children ?? [];
  const listen = nodes.find((d) => d.words[0] === "listen"),
    pass = nodes.find((d) => d.words[0] === "proxy_pass");
  if (!listen || !pass) unsupported(node);
  const address = listenAddress(listen);
  const options = listen.words.slice(2);
  if (
    options.some(
      (o) => !["udp", "reuseport", "ssl", "proxy_protocol"].includes(o),
    )
  )
    unsupported(listen);
  const protocol = options.includes("udp") ? "udp" : "tcp";
  const existing = state.stream_rules.find(
    (r) =>
      r.protocol === protocol &&
      r.listen_port === address.port &&
      (r.listen_address === "0.0.0.0" ? "*" : r.listen_address) === address.host,
  );
  const id =
    existing?.id ||
    nodes
      .flatMap((d) => d.words)
      .join(" ")
      .match(
        /fnproxy_stream_(?:conn|metrics|backend)_([a-f0-9]{12,64})\b/,
      )?.[1] ||
    newID();
  const rule: StreamRule = {
    id,
    name: existing?.name || `${protocol.toUpperCase()} ${address.port}`,
    enabled: true,
    protocol,
    listen_address: existing?.listen_address ?? address.host,
    listen_port: address.port,
    upstream_pool_id: "",
    upstream_host: "",
    upstream_port: 0,
    connect_timeout_seconds: 60,
    proxy_timeout_seconds: 600,
    udp_responses: 0,
    accept_proxy_protocol: options.includes("proxy_protocol"),
    proxy_protocol: false,
    trusted_proxies: [],
    tls_mode: options.includes("ssl") ? "terminate" : "off",
    certificate_id: "",
    sni_routes: [],
    access_log: false,
    allow: [],
    deny: [],
    max_connections: 0,
    created_at: "0001-01-01T00:00:00Z",
    updated_at: "0001-01-01T00:00:00Z",
  };
  let cert = "",
    key = "";
  for (const d of nodes) {
    const [k, v, ...rest] = d.words;
    if (d.children) unsupported(d);
    if (d === listen) continue;
    if (d === pass && !rest.length) {
      const pool = /^fnproxy_stream_([a-f0-9]+)$/.exec(v);
      if (pools.has(v?.toLowerCase())) rule.upstream_pool_id = pools.get(v.toLowerCase())!;
      else if (pool) rule.upstream_pool_id = pool[1];
      else {
        const target = endpoint(v, d);
        if (pools.has(target.host.toLowerCase())) unsupported(d);
        rule.upstream_host = target.host;
        rule.upstream_port = target.port;
      }
    } else if (k === "access_log") rule.access_log ||= v !== "off";
    else if (
      k === "limit_conn" &&
      v === "fnproxy_stream_conn_" + id &&
      rest.length === 1
    )
      rule.max_connections = Number(rest[0]);
    else if (rest.length) unsupported(d);
    else if (k === "proxy_connect_timeout" || k === "proxy_timeout")
      rule[
        k === "proxy_timeout"
          ? "proxy_timeout_seconds"
          : "connect_timeout_seconds"
      ] = seconds(v, d, true);
    else if (k === "proxy_responses") rule.udp_responses = Number(v);
    else if (k === "set_real_ip_from") rule.trusted_proxies.push(v);
    else if (k === "allow" || k === "deny") rule[k].push(v);
    else if (k === "proxy_protocol" && v === "on") rule.proxy_protocol = true;
    else if (k === "ssl_preread" && v === "on") rule.tls_mode = "passthrough";
    else if (k === "ssl_certificate") cert = v;
    else if (k === "ssl_certificate_key") key = v;
    else unsupported(d);
  }
  if (cert || key || rule.tls_mode === "terminate") {
    const match = /\/([a-f0-9]+)\/fullchain\.pem$/.exec(cert);
    if (!match || key !== cert.replace(/fullchain\.pem$/, "privkey.pem"))
      unsupported(node);
    rule.certificate_id = match[1];
  }
  if (preserveOmissions) {
    const names = new Set(nodes.map(d => d.words[0]));
    rule.omissions = {
      connect_timeout: !names.has("proxy_connect_timeout"), proxy_timeout: !names.has("proxy_timeout"),
      access_log: !names.has("access_log"), udp_reuseport: protocol === "udp" && !options.includes("reuseport"),
    };
  }
  return { kind: "streams", id, value: rule };
}

// Compare a raw fragment to the server-rendered preview. Pool defaults are
// local to the upstream. Stream directives may inherit from stream {}, so a
// missing directive must stay different from an explicit value.
export function rawFormEquivalent(kind: "upstreams" | "streams", source: string, generated: string): boolean {
  function signature(content: string) {
    const nodes = parseConfig(content);
    if (nodes.length !== 1) return;
    const node = nodes[0];
    if (kind === "upstreams") return JSON.stringify(rawPool(node).value);
    if (node.words.length !== 1 || node.words[0] !== "server" || !node.children) return;
    const groups = new Map<string, string[][]>();
    for (const child of node.children) {
      if (child.children) return;
      const [name, value, ...rest] = child.words;
      let words = child.words;
      if (name === "listen") {
        const address = listenAddress(child);
        words = [name, address.host, String(address.port), ...rest.sort()];
      } else if (name === "proxy_timeout" || name === "proxy_connect_timeout") {
        if (rest.length) return;
        words = [name, String(seconds(value, child, true))];
      } else if (!streamDirectives.has(name)) return;
      // Access checks run in input order, regardless of whether they allow or deny.
      const key = name === "allow" || name === "deny" ? "acl" : name;
      const rows = groups.get(key) ?? [];
      rows.push(words);
      groups.set(key, rows);
    }
    return JSON.stringify([...groups].sort(([a], [b]) => a.localeCompare(b)));
  }
  try {
    const original = signature(source);
    return original !== undefined && original === signature(generated);
  } catch (error) {
    if (error instanceof ConfigLineError) return false;
    throw error;
  }
}

const streamDirectives = new Set([
  "proxy_pass", "proxy_responses", "proxy_protocol", "ssl_preread", "ssl_certificate",
  "ssl_certificate_key", "set_real_ip_from", "allow", "deny", "limit_conn", "access_log",
]);

export function rawRouting(nodes: Node[], state: State): ConfigForm {
  // Preserve unrelated worker/TLS settings and variables in the open form.
  const settings = JSON.parse(
    JSON.stringify(state.settings),
  ) as State["settings"];
  const owners = new Map<string, string>();
  for (const [kind, definitions] of [
    ["map", settings.routing.maps], ["geo", settings.routing.geos], ["split_clients", settings.routing.splits],
  ] as const) {
    for (const row of definitions) {
      const key = row.variable.toLowerCase();
      owners.set(key, owners.has(key) ? "conflict" : kind);
    }
  }
  const seen = new Set<string>();
  for (const node of nodes) {
    const [kind, ...args] = node.words;
    const variable = args.at(-1);
    if (!node.children || !variable || !/^\$\w+$/.test(variable)) unsupported(node);
    const key = variable.toLowerCase();
    if (seen.has(key) || (owners.has(key) && owners.get(key) !== kind))
      throw new ConfigLineError(node.line, `动态路由变量 ${variable} 与其他定义重名`);
    seen.add(key);
    const entries: { key: string; value: string }[] = [];
    let fallback: string | undefined,
      hostnames = false;
    for (const child of node.children) {
      const [key, value] = child.words;
      if (child.children) unsupported(child);
      if (
        kind === "map" &&
        key === "hostnames" &&
        child.words.length === 1 &&
        !hostnames
      )
        hostnames = true;
      else if (child.words.length !== 2) unsupported(child);
      else if (
        key === "default" &&
        kind !== "split_clients" &&
        fallback === undefined
      )
        fallback = value;
      else entries.push({ key, value });
    }
    if (kind === "map" && args.length === 2) {
      const row = settings.routing.maps.find((r) => r.variable.toLowerCase() === key);
      const value = {
        name: row?.name ?? variable.slice(1),
        variable,
        source: args[0],
        hostnames,
        default: fallback ?? "",
        entries,
      };
      if (row) Object.assign(row, value);
      else settings.routing.maps.push(value);
    } else if (kind === "geo" && [1, 2].includes(args.length)) {
      const row = settings.routing.geos.find((r) => r.variable.toLowerCase() === key);
      const value = {
        name: row?.name ?? variable.slice(1),
        variable,
        source: args.length === 2 ? args[0] : "",
        default: fallback ?? "",
        entries,
      };
      if (row) Object.assign(row, value);
      else settings.routing.geos.push(value);
    } else if (kind === "split_clients" && args.length === 2) {
      const row = settings.routing.splits.find((r) => r.variable.toLowerCase() === key);
      const value = {
        name: row?.name ?? variable.slice(1),
        variable,
        source: args[0],
        entries,
      };
      if (row) Object.assign(row, value);
      else settings.routing.splits.push(value);
    } else unsupported(node);
  }
  return { kind: "settings", id: "", value: settings };
}

export function rawRatePolicy(nodes: Node[], state: State): ConfigForm {
  const zone = nodes[0].words;
  const id = zone[2]?.match(/^zone=fnproxy_req_([a-f0-9]{12,64}):1m$/)?.[1];
  const rate = zone[3]?.match(/^rate=(\d+)r\/s$/)?.[1];
  if (!id || !rate) unsupported(nodes[0]);
  const existing = state.rate_limit_policies.find(policy => policy.id === id);
  const settings = { enabled: true, requests_per_second: Number(rate), burst: 0,
    no_delay: false, connections: 0, download_kbps: 0 };
  for (const node of nodes.slice(1)) {
    const [name, arg, value, flag] = node.words;
    if (name === "limit_req") {
      settings.burst = Number(value?.match(/^burst=(\d+)$/)?.[1]);
      settings.no_delay = flag === "nodelay";
    } else if (name === "limit_conn") settings.connections = Number(value);
    else if (name === "limit_rate") settings.download_kbps = Number(arg?.match(/^(\d+)k$/)?.[1]);
    else if (name !== "limit_conn_zone") unsupported(node);
  }
  return {kind: "rate-limits", id, value: { id, name: existing?.name ?? `限流 ${id}`, settings,
    created_at: existing?.created_at ?? "0001-01-01T00:00:00Z", updated_at: existing?.updated_at ?? "0001-01-01T00:00:00Z" }};
}

// The group fragment uses the same listen/TLS grammar as an HTTP rule.
export function rawGroup(text: string, firstLine: number): ConfigForm {
  const rule = importBlocks("server { " + text + "\nserver_name group.test; location / { return 204; } }", "", firstLine).rules![0];
  const id = rule.id;
  return {kind: "rule-groups", id, value: {id, name: `监听 ${rule.listen_port}`,
    listen_type: rule.listen_type as "ipv4" | "ipv6" | "dual", listen_port: rule.listen_port,
    tls: rule.tls, http2: rule.http2, certificate_id: rule.certificate_id}};
}
