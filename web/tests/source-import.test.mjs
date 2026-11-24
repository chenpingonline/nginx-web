import assert from "node:assert/strict";
import { after, test } from "node:test";
import { createServer } from "vite";

const vite = await createServer({configFile: false, optimizeDeps: {noDiscovery: true}, server: {middlewareMode: true, hmr: false}, appType: "custom"});
after(() => vite.close());
const { parseConfig, parameterGroups } = await vite.ssrLoadModule("/src/nginxConfig.ts");
const { sourceImportMode, sourceImportPlan, sourceTargets, targetKey } = await vite.ssrLoadModule("/src/sourceImport.ts");
const mode = (text, scope, fallback) => sourceImportMode(parseConfig(text), scope, fallback);

test("detects supported advanced directives in their HTTP and stream contexts", () => {
  for (const [scope, text] of [
    ["http", "include 'parts/*.conf';"],
    ["http", "upstream app { server unix:/run/app.sock weight=2; }"],
    ["http", "map $host $backend { include 'hosts/*.map'; }"],
    ["http", "geo $network { include nets.geo; } types { include mime.types; }"],
    ["http", "gzip_types text/plain; server_tokens off; real_ip_header X-Forwarded-For; map_hash_max_size 2048; server { location / { proxy_pass http://$origin; } }"],
    ["http", "server { listen unix:/run/listen.sock; }"],
    ["http", "server { if ($arg_maintenance) { return 503; } }"],
    ["http", "server { location / { limit_except GET { proxy_pass http://unix:/run/app.sock; } } }"],
    ["http", 'server { location / { if ($arg_test) { proxy_pass "${origin}$request_uri"; } } }'],
    ["http", 'server { location / { proxy_pass "http${tls}://app:9000$request_uri"; } }'],
    ["stream", "include streams/*.conf;"],
    ["stream", "upstream app { server unix:/run/stream.sock; } server { listen 9000; proxy_pass app; }"],
    ["stream", "server { listen 9000; proxy_pass unix:/run/stream.sock; }"],
    ["stream", "server { listen 9000; proxy_pass $backend; }"],
    ["stream", "ssl_preread on; proxy_timeout 10s; server { listen 9000; proxy_pass $backend; }"],
  ]) assert.equal(mode(text, scope), "file", text);
  for (const text of [
    "if ($host = site.test) { return 204; }",
    "set $block 1; if ($block) { return 403; }",
    "location ^~ /api/ { include proxy.conf; proxy_pass http://$host; }",
    "location /one { return 200; } location /two { proxy_pass http://unix:/run/app.sock:/two; }",
  ]) assert.equal(mode(text, "http"), "server-fragment", text);
});

test("keeps ordinary managed imports first, regardless of variable-looking data", () => {
  for (const text of [
    "upstream app { server 127.0.0.1:19802; }",
    "map $host $value { if x; proxy_pass y; }",
    String.raw`map $host $value { \include literal; default include; }`,
    "server { listen 19801; location / { add_header X $host; proxy_pass http://127.0.0.1:9000; } }",
    "location / { proxy_pass http://127.0.0.1:9000; }",
    "add_header X a; upstream app { server 127.0.0.1:9000; }",
  ]) assert.equal(mode(text, "http"), undefined, text);
  assert.equal(mode("server { location / { autoindex_localtime on; } }", "http", true), "file");
  assert.equal(mode("upstream app { least_time last_byte; }", "stream", true), "file");
  assert.equal(mode("location / { autoindex_localtime on; }", "http", true), "server-fragment");
});

test("rejects mixed scopes and misplaced directives even after managed conversion fails", () => {
  for (const [scope, text] of [
    ["http", "include a; events {}"],
    ["http", "http { include a; } stream { include b; }"],
    ["http", "worker_processes 1; include a;"],
    ["http", "ssl_preread on; include a;"],
    ["http", "include '';"],
    ["http", "include a b;"],
    ["http", 'split_clients $request_id $bucket { include buckets.conf; }'],
    ["http", "server { location / { events {} } include a; }"],
    ["http", "server { proxy_pass http://$host; }"],
    ["http", "if ($host) { proxy_pass http://$host; }"],
    ["http", "server { if ($host) { if ($other) { return 204; } } }"],
    ["http", "upstream app { location / { include a; } }"],
    ["http", "server { server { include a; } }"],
    ["http", "location / { include a; } server {}"],
    ["stream", "server { location / { include a; } }"],
    ["stream", "server { if ($var) { return yes; } }"],
    ["stream", "server { proxy_pass http://$host; }"],
    ["stream", "types { include a; }"],
    ["stream", "real_ip_header X-Forwarded-For; include a;"],
  ]) assert.equal(mode(text, scope, true), undefined, text);
});

test("new source files preserve comments and select an available name per scope", () => {
  const text = "# 前置\r\ninclude '目录/*.conf'; # 后置\r\n";
  const files = ["custom-import.conf", "custom-IMPORT-2.conf", "custom-import.stream"].map(name => ({name, content: "include a;"}));
  for (const [scope, name] of [["http", "custom-import-3.conf"], ["stream", "custom-import-2.stream"]]) {
    const {entry, baseFile} = sourceImportPlan(text, parseConfig(text), scope, files, "");
    assert.deepEqual(entry.file, {name, content: text});
    assert.equal(entry.start, undefined);
    assert.equal(baseFile, undefined);
  }
});

test("appends only to the selected unannotated server using the original file for optimistic locking", () => {
  const file = {name: "custom-sites.conf", content: [
    "# 中文", "# nginx-web-form-v1 YWJj", "server { listen 9000; }",
    "server { listen 9001; server_name one; }",
    "server { server_name two; # 保留", "  }", "# nginx-web-form-v1 ZGVm", " server {server_name hidden;}", "# 后记",
  ].join("\r\n")};
  const files = [file, ...[
    ["custom-tcp.stream", "server {listen 9001;}"],
    ["generated.conf", "server {}"],
    ["custom-invalid.conf", "server {} bad"],
  ].map(([name, content]) => ({name, content}))];
  const sites = sourceTargets(files);
  assert.deepEqual(sites.map(s => s.title), ["one", "two"]);
  const target = sites[1];
  const text = "# 添加\r\nif ($arg_test) { return 204; } # 片段末尾";
  const {entry, baseFile} = sourceImportPlan(text, parseConfig(text), "http", files, targetKey(target));
  assert.equal(baseFile, file);
  assert.equal(entry.start, target.start);
  assert.equal(entry.file.content, file.content.replace("  }", text + "\r\n  }"));
  const fields = parameterGroups(entry).flatMap(g => g.fields);
  assert.deepEqual(fields.filter(f => f.directive === "server_name").map(f => f.value), ["two"]);
  assert.equal(fields.find(f => f.directive === "if").value, "($arg_test)");
  const inline = sourceImportPlan(text, parseConfig(text), "http", files, sites[0]);
  assert.ok(inline.entry.file.content.includes("server_name one; \r\n" + text + "\r\n}"));
  for (const key of ["", "custom-sites.conf:1", "generated.conf:0", "custom-tcp.stream:0"]) {
    assert.throws(() => sourceImportPlan(text, parseConfig(text), "http", files, key), /粘贴完整 server/);
  }
});
