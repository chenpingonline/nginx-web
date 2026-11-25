// The API stores whole units. Reject inputs that would lose precision on save.
export type QuantityUnit = "seconds" | "minutes" | "bytes" | "kib" | "mib";
const scale = { seconds: 1000, minutes: 60000, bytes: 1, kib: 1024, mib: 1048576 };
const suffix = { seconds: "s", minutes: "m", bytes: "", kib: "k", mib: "m" };
const timeUnits: Record<string, number> = { y: 31536000000, M: 2592000000, w: 604800000, d: 86400000, h: 3600000, m: 60000, s: 1000, ms: 1 };

export function formatQuantity(value: number, unit: QuantityUnit): string {
  return String(value) + (value === 0 ? "" : suffix[unit]);
}

export function parseQuantity(input: string, unit: QuantityUnit): number {
  const trimmed = input.trim();
  const quoted = /^(["'])(.*)\1$/.exec(trimmed);
  const text = quoted ? quoted[2] : trimmed;
  let total = 0;
  if (unit === "seconds" || unit === "minutes") {
    const part = /(\d+)(ms|[yMwdhms])? */gy;
    let offset = 0, previous = Infinity;
    while (offset < text.length) {
      const match = part.exec(text);
      if (!match || (!match[2] && part.lastIndex !== text.length)) throw Error("请输入 Nginx 时长，例如 90s 或 1h30m");
      const factor = timeUnits[match[2] || "s"];
      if (factor >= previous) throw Error("时长单位须从大到小排列");
      previous = factor;
      total += Number(match[1]) * factor;
      offset = part.lastIndex;
    }
  } else {
    const size = /^(\d+)([kKmMgG]?)$/.exec(text);
    if (!size) throw Error("请输入字节数或带单位的容量，例如 128k 或 2m");
    const factors: Record<string, number> = { "": 1, k: 1024, m: 1048576, g: 1073741824 };
    total = Number(size[1]) * factors[size[2].toLowerCase()];
  }
  const result = total / scale[unit];
  if (!text || !Number.isSafeInteger(total) || !Number.isInteger(result)) {
    throw Error("此字段只能保存 " + formatQuantity(1, unit) + " 的整数倍");
  }
  return result;
}

// Use the old MiB field when it reproduces the exact spelling. Otherwise the
// native token is authoritative, including an unqualified number of bytes.
export function requestBodySizeFields(token: string) {
  if (!/^\d+[kKmMgG]?$/.test(token)) throw Error("请输入字节数或带单位的整数，例如 512k");
  const bytes = parseQuantity(token, "bytes");
  if (bytes > 100 * 1024 ** 3) throw Error("请求体上限不能超过 100g");
  const mib = bytes / 1024 ** 2;
  if (Number.isInteger(mib) && token === formatQuantity(mib, "mib")) {
    return { client_max_body_mb: mib, client_max_body_size: undefined };
  }
  return { client_max_body_mb: Number.isInteger(mib) ? mib : 0, client_max_body_size: token };
}
