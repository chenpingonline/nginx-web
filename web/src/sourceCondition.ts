import { parseConfig, type Parameter } from "./nginxConfig";

export const comparisonOperators = ["=", "!=", "~", "~*", "!~", "!~*"];
export const fileOperators = ["-f", "!-f", "-d", "!-d", "-e", "!-e", "-x", "!-x"];

// Capture the original separators so an operand edit only alters that operand.
export function splitCondition(raw: string) {
  if (/[\r\n#]/.test(raw)) return;
  const binary = /^(\(\s*)(\$\w+)(\s+)(!=|=|!?~\*?)(\s+)(.+?)(\s*\))$/u.exec(raw);
  if (binary) {
    const [, open, left, firstSpace, operator, secondSpace, right, close] = binary;
    return { kind: "comparison" as const, left, operator, right,
      format: (l: string, op: string, r: string) => open + l + firstSpace + op + secondSpace + r + close };
  }
  const file = /^(\(\s*)(!?-[fdex])(\s+)(.+?)(\s*\))$/u.exec(raw);
  if (file) {
    const [, open, operator, space, right, close] = file;
    return { kind: "file" as const, left: "", operator, right,
      format: (_l: string, op: string, r: string) => open + op + space + r + close };
  }
  const variable = /^(\(\s*)(\$\w+)(\s*\))$/u.exec(raw);
  if (variable) {
    const [, open, left, close] = variable;
    return { kind: "variable" as const, left, operator: "", right: "",
      format: (l: string) => open + l + close };
  }
}

// Use the same lexer as the range editor. Quoted paths and regex delimiters
// remain a single argument, and an edit cannot append a sibling directive.
function argumentsOf(raw: string): string[] {
  const source = "param " + raw + "\n;";
  const nodes = parseConfig(source);
  if (nodes.length !== 1 || nodes[0].children || nodes[0].end !== source.length) return [];
  return nodes[0].words.slice(1);
}

// In a path field, spaces and separators belong to the filename. Preserve the
// existing quoting style when possible, including single-quoted glob patterns.
export function includePathField(raw: string) {
  const args = argumentsOf(raw);
  if (args.length !== 1 || /[\r\n]/.test(args[0])) return;
  const [path] = args;
  const quote = /^["']/.test(raw) ? raw[0] : "";
  return { path, format(next: string) {
    if (next === path) return raw;
    const delimiter = quote || (/[\s#;{}"'\\]/.test(next) ? '"' : "");
    if (!delimiter) return next;
    const escaped = next.replaceAll("\\", "\\\\").replaceAll(delimiter, "\\" + delimiter);
    return delimiter + escaped + delimiter;
  } };
}

function conditionArguments(raw: string): string[] {
  if (raw.startsWith("(") && raw.endsWith(")")) return argumentsOf(raw.slice(1, -1));
  // Nginx also accepts parentheses in quoted tokens, e.g. if "(" $arg_id ")".
  const args = argumentsOf(raw);
  if (!args[0]?.startsWith("(") || !args.at(-1)?.endsWith(")")) return [];
  if (args[0] === "(") args.shift();
  else args[0] = args[0].slice(1);
  if (args.at(-1) === ")") args.pop();
  else args[args.length - 1] = args[args.length - 1].slice(0, -1);
  return args;
}

export function validateSourceConditions(fields: Parameter[], creating: boolean) {
  for (const field of fields) {
    if (!creating && field.value === field.original) continue;
    const raw = field.value.trim();
    if (field.directive === "include") {
      const args = argumentsOf(raw);
      if (field.readonly || args.length !== 1 || !args[0]) throw Error("包含文件需要一个路径或通配模式");
    }
    if (field.directive !== "if") continue;
    const args = conditionArguments(raw);
    if (args.length === 2 && fileOperators.includes(args[0])) continue;
    if (args[0]?.startsWith("$") && args[0].length > 1 &&
      (args.length === 1 || (args.length === 3 && comparisonOperators.includes(args[1])))) continue;
    throw Error("if 条件需要括号内的变量、比较表达式或文件判断");
  }
}
