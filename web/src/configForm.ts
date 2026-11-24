import { parseConfig, ruleDirectives, importBlocks } from "./nginxConfig";
import { request } from "./api";
import type {
  AuthProfile,
  ProxyRule,
  RateLimitPolicy,
  RuleGroup,
  Settings,
  State,
  StreamRule,
  UpstreamPool,
  WAFInstance,
} from "./types";

export type ConfigForm =
  | { kind: "rule-groups"; id: string; value: RuleGroup }
  | { kind: "rules"; id: string; value: ProxyRule }
  | { kind: "streams"; id: string; value: StreamRule }
  | { kind: "upstreams"; id: string; value: UpstreamPool }
  | { kind: "rate-limits"; id: string; value: RateLimitPolicy }
  | { kind: "waf"; id: string; value: WAFInstance }
  | { kind: "auth-profiles"; id: string; value: AuthProfile }
  | { kind: "settings"; id: string; value: Settings };

export const formPrefix = "# nginx-web-form-v1 ";
export function configBody(text: string) {
  const lines = text.split("\n");
  let first = 0;
  while (lines[first]?.startsWith(formPrefix)) first++;
  return lines.slice(first).join("\n");
}

// Old exports do not carry form metadata. Match their actual directives before
// reusing fields (e.g. disabled locations) that exist only in the saved model.
export async function matchSavedConfig(
  text: string,
  state: State,
  firstLine = 1,
): Promise<ConfigForm | undefined> {
  const nodes = parseConfig(text, firstLine);
  const candidates: { kind: ConfigForm["kind"]; id: string }[] = [];
  if (nodes.length === 1 && nodes[0].words[0] === "server") {
    const body = nodes[0].children ?? [];
    const tracked = body.find(
      (d) => d.words[0] === "set" && d.words[1] === "$fnproxy_rule_id",
    );
    if (tracked && state.rules.some((r) => r.id === tracked.words[2]))
      candidates.push({ kind: "rules", id: tracked.words[2] });
    if (body.some((d) => d.words[0] === "proxy_pass")) {
      const listener = body.find((d) => d.words[0] === "listen")?.words[1];
      const port = Number(listener?.split(":").pop());
      for (const r of state.stream_rules.filter((r) => r.listen_port === port))
        candidates.push({ kind: "streams", id: r.id });
    }
  } else if (nodes.length === 1 && nodes[0].words[0] === "upstream") {
    const id = nodes[0].words[1]?.match(
      /^fnproxy_(?:up|stream)_([0-9a-f]+)$/,
    )?.[1];
    if (id && state.upstream_pools.some((p) => p.id === id))
      candidates.push({ kind: "upstreams", id });
  }
  const first = nodes[0]?.words;
  if (first?.[0] === "listen") {
    const port = Number(first[1]?.split(":").pop());
    for (const group of state.rule_groups ?? [])
      if (group.listen_port === port) candidates.push({kind: "rule-groups", id: group.id});
  } else if (first?.[0] === "limit_req_zone") {
    const id = first[2]?.match(/^zone=fnproxy_req_([a-f0-9]+):1m$/)?.[1];
    if (id && state.rate_limit_policies.some(policy => policy.id === id))
      candidates.push({kind: "rate-limits", id});
  } else if (first?.[0] === "proxy_pass") {
    const id = first[1]?.match(/:\/internal\/basic-auth\/([a-f0-9]+)$/)?.[1];
    if (id && state.auth_profiles.some(profile => profile.id === id))
      candidates.push({kind: "auth-profiles", id});
  } else if (first?.[0] === "location") {
    const site = text.match(/^# server ([a-f0-9]+)\s*$/m)?.[1];
    if (site) for (const waf of state.waf_instances ?? [])
      if (waf.sites.includes(site)) candidates.push({kind: "waf", id: waf.id});
  } else if (first?.[0] === "daemon") candidates.push({kind: "settings", id: ""});

  const signature = ruleDirectives(text);
  let matched: string | undefined;
  for (const c of candidates) {
    const { content } = await request<{ content: string }>(
      `/config-form?kind=${c.kind}&id=${c.id}`,
    );
    const body = configBody(content);
    // Settings exports carry routing blocks in comments.
    const same = c.kind === "settings" || c.kind === "waf"
      ? body.trim().replaceAll("\r\n", "\n") === text.trim().replaceAll("\r\n", "\n")
      : ruleDirectives(body) === signature;
    if (!same) continue;
    if (matched) throw Error("多条已保存的配置与原文一致，请复制含表单注释的配置以选择对应记录");
    matched = content;
  }
  if (matched) return request<ConfigForm>("/config-form", {
    method: "POST", body: JSON.stringify({ content: matched }),
  });
  const { rawPool, rawStream, rawRouting, rawRatePolicy, rawGroup } = await import("./rawConfigForm");
  if (first?.[0] === "limit_req_zone" || first?.[0] === "listen") {
    const candidate = first[0] === "listen" ? rawGroup(text, firstLine) : rawRatePolicy(nodes, state);
    const preview = await request<{ form: ConfigForm; content: string }>("/config-form/preview", {
      method: "POST", body: JSON.stringify(candidate),
    });
    if (ruleDirectives(preview.content) !== signature) throw Error("表单生成的指令与原文不同");
    return preview.form;
  }
  if (
    nodes.length &&
    nodes.every((n) => ["map", "geo", "split_clients"].includes(n.words[0]))
  ) {
    const candidate = rawRouting(nodes, state);
    const preview = await request<{ form: ConfigForm; content: string }>(
      "/config-form/preview",
      { method: "POST", body: JSON.stringify(candidate) },
    );
    const routing = preview.content.split("\n# 001-runtime-and-upstreams.conf\n").at(1)?.replace(/^# /gm, "") ?? "";
    const generated = parseConfig(routing);
    for (const n of nodes) {
      const matches = generated.filter(
        (d) => d.words[0] === n.words[0] && d.words.at(-1) === n.words.at(-1),
      );
      if (
        !matches.some(
          (d) =>
            ruleDirectives(routing.slice(d.start, d.end)) ===
            ruleDirectives(text.slice(n.start, n.end)),
        )
      )
        throw Error("动态路由变量的生成结果与原文不同");
    }
    return preview.form;
  }
  if (nodes.length !== 1) return;
  const node = nodes[0];
  let candidate;
  if (node.words[0] === "upstream") candidate = rawPool(node);
  else if (
    node.words[0] === "server" &&
    node.children?.some((d) => d.words[0] === "proxy_pass")
  )
    candidate = rawStream(node, state);
  else if (
    node.words[0] === "server" &&
    node.children?.some(
      (d) => d.words[0] === "set" && d.words[1] === "$fnproxy_rule_id",
    )
  ) {
    const rule = importBlocks(text, "", firstLine).rules?.[0];
    if (rule) candidate = { kind: "rules", id: rule.id, value: rule };
  }
  if (!candidate) return;
  const preview = await request<{ form: ConfigForm; content: string }>(
    "/config-form/preview",
    { method: "POST", body: JSON.stringify(candidate) },
  );
  if (ruleDirectives(preview.content) !== signature)
    throw Error(
      "这段配置包含无法完整还原的指令，请核对原文或复制包含表单注释的配置",
    );
  return preview.form;
}
