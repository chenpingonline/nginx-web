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
  await assert.rejects(previewImport(input.replace("upstream app", "add_header X a; upstream app"), "", {}, "http"), /作用域/);
  await assert.rejects(previewImport(input.replace("upstream app", "upstream APP {server 127.0.0.1:1;} upstream app"), "", {}, "http"), /名称重复/);
  assert.equal(previews, 1);
});
