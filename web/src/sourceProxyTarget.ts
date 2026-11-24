import { parseConfig } from "./nginxConfig";

interface TargetSyntax {
  quote: string;
  scheme: string;
}
export type SourceProxyTarget = TargetSyntax & (
  | { kind: "expression"; expression: string }
  | { kind: "unix"; prefix: string; path: string; uri: string; separator: boolean; stream: boolean }
);

function isSingleArgument(raw: string): boolean {
  try {
    const [node, ...rest] = parseConfig("proxy_pass " + raw + ";");
    return !!node && !rest.length && !node.children && node.words.length === 2 &&
      node.argsStart === 11 && node.argsEnd === 11 + raw.length;
  } catch {
    return false;
  }
}

// Keep slices of the original token, including variable braces and escape sequences.
// The generic source field handles tokens with unusual quoting.
export function parseSourceProxyTarget(raw: string, stream = false): SourceProxyTarget | undefined {
  if (!/unix:|\$/i.test(raw) || !isSingleArgument(raw) || /[\0\r\n]/.test(raw)) return;
  const quote = /^["']/.test(raw) ? raw[0] : "";
  if (quote && !raw.endsWith(quote)) return;
  const token = quote ? raw.slice(1, -1) : raw;
  const unescaped = token.replace(/\\./g, "");
  if (quote ? unescaped.includes(quote) : /['"]/.test(unescaped)) return;

  const scheme = stream ? "" : /^https?(?=:\/\/)/i.exec(token)?.[0] ?? "";
  const address = scheme ? token.slice(scheme.length + 3) : token;
  const unix = /^(unix:)(.+)$/i.exec(address);
  if (unix && (scheme || stream)) {
    const socket = unix[2];
    // In HTTP the first colon after the socket name starts the optional URI.
    // Stream has no URI part, so a colon there belongs to the socket name.
    const colon = stream ? -1 : socket.indexOf(":");
    if (colon === 0) return;
    return {kind: "unix", scheme, quote, prefix: unix[1], stream,
      path: colon < 0 ? socket : socket.slice(0, colon),
      uri: colon < 0 ? "" : socket.slice(colon + 1), separator: colon >= 0};
  }
  if (/\$(?:\w|\{)/.test(address))
    return {kind: "expression", scheme, quote, expression: address};
}

export function formatSourceProxyTarget(target: SourceProxyTarget): string {
  let token = target.scheme ? target.scheme + "://" : "";
  if (target.kind === "unix") {
    if (!target.path || (!target.stream && target.path.includes(":"))) throw Error("请填写有效的 Unix socket 路径");
    token += target.prefix + target.path;
    if (!target.stream && (target.separator || target.uri)) token += ":" + target.uri;
  } else {
    if (!target.expression) throw Error("请填写上游表达式");
    token += target.expression;
  }
  if (/[\0\r\n]/.test(token)) throw Error("转发地址只能占一行");
  const quote = target.quote || (isSingleArgument(token) && !/['"]/.test(token.replace(/\\./g, "")) ? "" : '"');
  if (!quote) return token;
  // Escape newly typed delimiter quotes without changing existing escapes.
  const escaped = token.replace(/\\.|["']/g, part => part === quote ? "\\" + part : part);
  const raw = quote + escaped + quote;
  if (!isSingleArgument(raw)) throw Error("请检查转发地址的引号和转义");
  return raw;
}
