import { parseConfig } from "./nginxConfig";
import type { ConfigComment } from "./types";

type Block = ReturnType<typeof parseConfig>[number];

// Anchors identify a directive inside one form, independently of source line
// numbers or the order in which the managed renderer writes other directives.
export function readCommentedBlocks(content: string) {
  const comments: {offset: number; text: string}[] = [];
  const nodes = parseConfig(content, 1, offset => {
    const newline = content.indexOf("\n", offset);
    comments.push({offset, text: content.slice(offset, newline < 0 ? content.length : newline).replace(/\r$/, "")});
  });
  const blocks = nodes.map(node => ({node, comments: [] as (ConfigComment & {offset: number})[]}));
  if (!comments.length || !nodes.length) return blocks;
  type Owner = typeof blocks[number];
  const marks: {at: number; anchor: string; owner: Owner}[] = [];
  function visit(node: Block, anchor: string, owner: Owner) {
    marks.push({at: node.start, anchor, owner});
    if (node.children) {
      const counts = new Map<string, number>();
      for (const child of node.children) {
        const key = child.words[0] === "location" ? child.words.join(" ") : child.words[0];
        const index = counts.get(key) ?? 0;
        counts.set(key, index + 1);
        visit(child, anchor + "/" + key + ":" + index, owner);
      }
      marks.push({at: node.end - 1, anchor: anchor + "/}", owner});
    } else if (node.end > node.start + 1) {
      marks.push({at: node.end - 1, anchor, owner});
    }
  }
  for (const block of blocks) visit(block.node, "/" + block.node.words[0] + ":0", block);
  let i = 0;
  for (const {offset, text} of comments) {
    while (i < marks.length && marks[i].at < offset) i++;
    const previous = marks[i - 1], next = marks[i];
    const inline = !!previous && content.lastIndexOf("\n", offset) < previous.at;
    const mark = inline || !next ? previous : next;
    mark.owner.comments.push({offset, text, anchor:mark.anchor, inline, after:!next && !inline});
  }
  return blocks;
}
