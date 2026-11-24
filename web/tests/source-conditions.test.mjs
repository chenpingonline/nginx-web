import assert from "node:assert/strict";
import { after, test } from "node:test";
import { createServer } from "vite";

const vite = await createServer({ configFile: false, optimizeDeps: { noDiscovery: true }, server: { middlewareMode: true, hmr: false }, appType: "custom" });
after(() => vite.close());
const { includePathField, splitCondition, validateSourceConditions } = await vite.ssrLoadModule("/src/sourceCondition.ts");
const { parameterGroups, editParameters } = await vite.ssrLoadModule("/src/nginxConfig.ts");

test("if and include fields preserve scope, comments, quotes, CRLF and directive order", () => {
  const text = [
    "# upstream is defined in a separate file", "server {", "  include \"/tmp/test snippets/*.conf\";",
    "  if (  $arg_mode  !=  \"no data\"  ) { return 403; }",
    "  location / {", "    if (!-f $request_filename) { return 404; }",
    "    if ($arg_on) { return 202; }", "    include conf.d/x*.inc; # last", "  }", "}", "",
  ].join("\r\n");
  const fields = parameterGroups({file: {name: "custom-site.conf", content: text}}).flatMap(g => g.fields);
  validateSourceConditions(fields, true);
  assert.equal(editParameters(text, fields), text);
  const [binary, file, variable] = fields.filter(f => f.directive === "if");
  assert.deepEqual([binary,file,variable].map(f => splitCondition(f.original).kind), ["comparison", "file", "variable"]);
  const comparison = splitCondition(binary.original);
  binary.value = comparison.format(comparison.left, "~*", '"^ok(;|$)"');
  const fileCheck = splitCondition(file.original);
  file.value = fileCheck.format("", "-d", fileCheck.right);
  const include = fields.find(f => f.value === "conf.d/x*.inc");
  include.value = "conf.d/y*.inc";
  validateSourceConditions(fields, false);
  assert.equal(editParameters(text, fields), text.replace('!=  "no data"', '~*  "^ok(;|$)"').replace("!-f", "-d").replace("x*.inc", "y*.inc"));
});

test("validate changed condition and include argument boundaries", () => {
  const validate = (directive, value) => validateSourceConditions([{directive, value, original: "previous value"}], false);
  for (const expr of ['($arg_x = "")', "($http_host ~* ^(a|b)$)", "(!-x /tmp/app)", "(\n # condition\n $query_string # comment\n)", "($1)", '"($arg_on)"', '"(" $arg_on ")"'])
    assert.doesNotThrow(() => validate("if", expr));
  for (const expr of ["$arg_x = x", "()", "($arg_x ! = ok)", "($arg_x = two words)", "($arg_x; return 204)"])
    assert.throws(() => validate("if", expr));
  for (const name of ["conf.d/*.inc", '"/tmp/path with spaces/mime.types"', '"quoted;filename.inc"']) assert.doesNotThrow(() => validate("include", name));
  for (const name of ["", '""', "a b", "part; error_log off", "foo {}"]) assert.throws(() => validate("include", name));
});

test("edit include paths as filenames and retain the existing quote style", () => {
  for (const raw of ["conf.d/*.inc", "'with space/*.inc'", '"file;name"', String.raw`"escaped\"name"`]) {
    const field = includePathField(raw);
    assert.equal(field.format(field.path), raw);
    const next = `/tmp/a "test"; 'site'/file\\name/*.inc`;
    const edited = field.format(next);
    assert.equal(includePathField(edited).path, next);
    assert.equal(edited[0], raw.startsWith("'") ? "'" : '"');
  }
});
