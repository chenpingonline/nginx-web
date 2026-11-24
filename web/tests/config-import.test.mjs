import assert from "node:assert/strict";
import { after, test } from "node:test";
import { createServer } from "vite";

globalThis.location = { pathname: "/" };
const vite = await createServer({ configFile: false, optimizeDeps: { noDiscovery: true }, server: { middlewareMode: true, hmr: false }, appType: "custom" });
after(() => { delete globalThis.location; return vite.close(); });
const { previewImport, configFragment } = await vite.ssrLoadModule("/src/configImport.ts");
const { parseConfig } = await vite.ssrLoadModule("/src/nginxConfig.ts");

test("unwrap a single scope for an include, keeping comments and original line numbers", () => {
  const source = "# header\nstream # inner scope\n{\n  # retain\n  upstream app { server 127.0.0.1:9000; }\n}\n# tail";
  const result = configFragment(source, parseConfig(source), "http");
  assert.equal(result.scope, "stream");
  assert.equal(parseConfig(result.text)[0].line, 5);
  for (const comment of ["# header", "# inner scope", "# retain", "# tail"]) assert.ok(result.text.includes(comment));
});

test("linked import validates all declarations before saving and keeps references ordered independently of source", async t => {
  const input = `http {
    server {listen 19801; server_name bundle.test; location / {proxy_pass http://APP/api/;}}
    upstream app { server 127.0.0.1:19802; }
  }`;
  let previews = 0;
  t.mock.method(globalThis, "fetch", async (url, init) => {
    assert.equal(url, "/api/config-import/preview");
    previews++;
    const {pools, rules} = JSON.parse(init.body);
    const [pool] = pools;
    assert.equal(pool.name, "app");
    assert.equal(rules[0].root_location.upstream_pool_id, pool.id);
    assert.equal(rules[0].root_location.upstream_path, "/api/");
    return Response.json([`upstream fnproxy_up_${pool.id} {server 127.0.0.1:19802;}`, "server {}"]);
  });
  const result = await previewImport(input, "", {}, "http");
  assert.equal(result.draft.rules.length, 1);
  const withHeader = await previewImport(input.replace("upstream app", "add_header X a; upstream app"), "", {}, "http");
  assert.match(withHeader.sourceForm.entry.file.content, /add_header X a; upstream app/);
  await assert.rejects(previewImport(input.replace("upstream app", "upstream APP {server 127.0.0.1:1;} upstream app"), "", {}, "http"), /名称重复/);
  assert.equal(previews, 1);
});

test("combined sources keep definitions, conditions, include order and untouched sites together", async t => {
  const input = `# first: 中文
map $host $origin {
  include "maps/blue team.inc";
  \\include "literal-key";
  default 127.0.0.1:9000;
}
upstream local { server unix:/run/site.sock; }
server {
  listen 9801;
  server_name app.test;
  include snippets/headers.inc;
  if ($arg_mode = preview) { return 202; }
  location /api/ { proxy_pass http://\${origin}$request_uri; }
  location /files/ { proxy_pass http://unix:/run/files.sock:/data/; }
}
server { listen 9802; location / { return 204; } }
`;
  t.mock.method(globalThis, "fetch", () => assert.fail("preview must not save or apply a custom file"));
  const result = await previewImport(input, "", {custom_configs: [{name: "custom-import.conf", content: "include old.inc;"}]}, "stream");
  assert.deepEqual(result.sourceForm.entry.file, {name: "custom-import-2.conf", content: input});
  const {parameterGroups, editParameters} = await vite.ssrLoadModule("/src/nginxConfig.ts");
  const fields = parameterGroups(result.sourceForm.entry).flatMap(g => g.fields);
  assert.deepEqual(fields.filter(f => f.directive === "include").map(f => f.value), ['"maps/blue team.inc"', "snippets/headers.inc"]);
  assert.ok(fields.some(f => !f.directive && f.value === '\\include "literal-key"'));
  assert.equal(editParameters(input, fields), input);
});

test("unsupported managed syntax becomes an editable source file while managed validation errors still surface", async t => {
  const simple = "server { listen 9210; server_name site.test; location / { proxy_pass http://127.0.0.1:9000; } }";
  t.mock.method(globalThis, "fetch", async () => Response.json({error: "validation failure"}, {status: 400}));
  await assert.rejects(previewImport(simple, "", {}, "http"), /validation failure/);
  for (const input of [
    simple.replace("listen 9210", "listen 127.0.0.1:9210"),
    simple.replace("proxy_pass", "proxy_intercept_errors on; proxy_pass"),
    "server {listen 9210; return 200 ok;}",
  ]) assert.equal((await previewImport(input, "", {}, "http")).sourceForm.entry.file.content, input);
  const state = {custom_configs: [{name: "custom-site.conf", content: simple}]};
  const location = "location /extra { proxy_intercept_errors on; proxy_pass http://127.0.0.1:9000; }";
  const {sourceForm} = await previewImport(location, "custom-site.conf:0", state, "stream");
  assert.equal(sourceForm.baseFile, state.custom_configs[0]);
  assert.ok(sourceForm.entry.file.content.includes(location));
});

test("an exact saved managed rule keeps its original form when it contains advanced directives", async t => {
  const id = "123456abcdef";
  const input = `server { listen 9501; set $fnproxy_rule_id ${id}; if ($arg_ok) {return 200;} }`;
  const form = {kind: "rules", id, value: {id}};
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url) => {
    calls.push(url);
    return url.endsWith("/config-form") ? Response.json(form) : Response.json({content: input});
  });
  assert.deepEqual(await previewImport(input, "", {rules: [form.value]}, "http"), {form});
  assert.equal(calls.length, 2);
});

test("main and mixed context configurations cannot become a custom HTTP or stream include", async () => {
  for (const input of [
    'events {} http {server { include site.inc; }}',
    'http {include http.inc;} stream {include stream.inc;}',
    'stream {server {location / {proxy_pass http://unix:/tmp/x.sock;}}}',
    'http {if ($arg_x) {return 200;}}',
  ]) await assert.rejects(previewImport(input, "", {}, "http"));
});

test("HTTP-only managed routing forms do not take ownership of stream routing definitions", async t => {
  t.mock.method(globalThis, "fetch", () => assert.fail("stream maps do not use the HTTP settings API"));
  const input = "map $remote_addr $origin { default 127.0.0.1:9000; }";
  for (const text of [input, "stream { " + input + " }"]) {
    const {sourceForm} = await previewImport(text, "", {}, "stream");
    assert.equal(sourceForm.entry.file.name, "custom-import.stream");
    assert.equal(sourceForm.entry.file.content.trim(), input);
  }
  const {sourceForm} = await previewImport("http { gzip on; gzip_types text/plain; }", "", {}, "stream");
  assert.equal(sourceForm.entry.file.name, "custom-import.conf");
});

test("preserves additional module directives when the pasted wrapper or site identifies the context", async () => {
  const charset = "charset_map windows-1251 utf-8 { C0 D090; }";
  const input = "http { " + charset + " }";
  assert.equal((await previewImport(input, "", {}, "http")).sourceForm.entry.file.content.trim(), charset);
  const withSite = charset + "\nserver {listen 9500; include snippets/site.inc;}";
  assert.equal((await previewImport(withSite, "", {}, "http")).sourceForm.entry.file.content, withSite);
  for (const misplaced of ["worker_processes 2", "proxy_pass http://example.test", "return 200", "ssl_preread on"]) {
    await assert.rejects(previewImport("http { include site.inc; " + misplaced + "; }", "", {}, "http"));
  }
});
