// Whitespace is the Nginx separator. Keep commas accepted for earlier form input.
export function parseTryFiles(input: string): string[] {
  const files: string[] = [];
  let word = "", quote = "";
  for (let i = 0; i < input.length; i++) {
    const ch = input[i];
    if (ch === "\\" && i + 1 < input.length && /[\s,'"\\]/.test(input[i + 1])) {
      word += input[++i];
    } else if (quote) {
      if (ch === quote) quote = "";
      else word += ch;
    } else if (!word && (ch === '"' || ch === "'")) {
      quote = ch;
    } else if (ch === "," || /\s/.test(ch)) {
      if (word) files.push(word);
      word = "";
    } else word += ch;
  }
  if (quote) throw Error("Try Files 的引号未闭合");
  if (word) files.push(word);
  if (files.length === 1) throw Error("Try Files 至少需要一个候选路径和一个回退目标");
  return files;
}

export function formatTryFiles(files: string[]): string {
  return files.map(file => /[\s,'"\\]/.test(file) ? `"${file.replace(/["\\]/g, "\\$&")}"` : file).join(" ");
}
