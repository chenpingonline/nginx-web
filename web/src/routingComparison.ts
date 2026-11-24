import type { parseConfig } from "./nginxConfig";

type Block = ReturnType<typeof parseConfig>[number];

function routingSignature(block: Block): string | undefined {
  if (!block.children || block.children.some(row => row.children)) return;
  const rows = block.children.map(row => row.words);
  const kind = block.words[0];
  if (kind === "split_clients") return JSON.stringify([block.words, rows]);
  if (kind !== "map" && kind !== "geo") return;

  const defaults = rows.filter(words => words[0] === "default");
  if (defaults.length > 1 || (defaults.length && defaults[0].length !== 2)) return;
  // In geo, default installs /0 entries. An explicit /0 can override it, or vice versa.
  if (kind === "geo" && rows.some(words => /\/0+$/.test(words[0])))
    return JSON.stringify([block.words, rows]);

  const entries = rows.filter(words => words[0] !== "default");
  return JSON.stringify([block.words, defaults[0]?.[1] ?? "", entries]);
}

export function routingDirectivesMatch(input: Block, rendered: Block): boolean {
  const signature = routingSignature(input);
  return signature !== undefined && signature === routingSignature(rendered);
}
