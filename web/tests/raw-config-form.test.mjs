import assert from "node:assert/strict";
import { after, test } from "node:test";
import { createServer } from "vite";

const vite = await createServer({ configFile: false, optimizeDeps: { noDiscovery: true }, server: { middlewareMode: true, hmr: false }, appType: "custom" });
after(() => vite.close());
const { rawFormEquivalent, rawPool, rawStream, rawRouting } = await vite.ssrLoadModule("/src/rawConfigForm.ts");
const { parseConfig } = await vite.ssrLoadModule("/src/nginxConfig.ts");

test("upstream default server options, HTTP port, random two and duration units", () => {
  const pool = body => `upstream fnproxy_up_0123456789ab { ${body} }`;
  const source = pool("random two; server [::1] fail_timeout=1m weight=2; server 127.0.0.1; keepalive 4;");
  const generated = pool("random two least_conn; server [::1]:80 weight=2 max_fails=1 fail_timeout=60s; server 127.0.0.1:80 weight=1 max_fails=1 fail_timeout=10s; keepalive 4; keepalive_requests 1000; keepalive_time 3600s; keepalive_timeout 60s;");
  assert.ok(rawFormEquivalent("upstreams", source, generated));
  assert.equal(rawPool(parseConfig(source)[0]).value.servers[0].port, 80);
  for (const changed of [source.replace("weight=2", "weight=3"), source.replace("1m", "1s"), source.replace("server 127.0.0.1", "server 127.0.0.2"), source.replace("random two", "random")])
    assert.equal(rawFormEquivalent("upstreams", changed, generated), false);
  const implicit = pool("server 127.0.0.1;");
  assert.ok(rawFormEquivalent("upstreams", implicit, pool("server 127.0.0.1; keepalive 32 local;")));
  assert.equal(rawFormEquivalent("upstreams", implicit, pool("server 127.0.0.1; keepalive 32;")), false);
  for (const invalid of ["server;", "keepalive_timeout 29s; server host;", "least_conn; ip_hash;", "keepalive 0;", "server host weight=1=2;", "server host max_fails=0;", "server host down down;", "server host resolve;"])
    assert.throws(() => rawPool(parseConfig(pool(invalid))[0]), /无法完整还原/);
  assert.throws(() => rawPool(parseConfig("upstream fnproxy_stream_0123456789ab { server host; }")[0]));
});

test("stream comparison normalizes timeout and listener spellings, retaining ACL order and inherited settings", () => {
  const source = "server { proxy_timeout 1h; allow 10.0.0.0/8; deny all; proxy_pass 127.0.0.1:9000; listen *:9001 proxy_protocol ssl; proxy_connect_timeout 5000ms; access_log off; }";
  const generated = "server { listen 0.0.0.0:9001 ssl proxy_protocol; allow 10.0.0.0/8; deny all; proxy_pass 127.0.0.1:9000; proxy_connect_timeout 5s; proxy_timeout 3600s; access_log off; }";
  assert.ok(rawFormEquivalent("streams", source, generated));
  for (const changed of [source.replace("5000ms", "5500ms"), source.replace("access_log off;", ""), source.replace("allow 10.0.0.0/8; deny all;", "deny all; allow 10.0.0.0/8;"), source.replace("proxy_protocol ssl", "ssl")])
    assert.equal(rawFormEquivalent("streams", changed, generated), false);
  const instance = {stream_rules: [{id:"abcdefabcdef", name:"existing", protocol:"tcp", listen_port:9001, listen_address:"0.0.0.0"}]};
  const rule = rawStream(parseConfig(source.replace(" proxy_protocol ssl", ""))[0], instance).value;
  assert.equal(rule.id, "abcdefabcdef");
  assert.equal(rule.connect_timeout_seconds, 5);
  assert.equal(rule.listen_address, "0.0.0.0");
});

test("routing variables update an existing name case-insensitively and reject conflicting owners", () => {
  const state = {settings:{routing:{maps:[], splits:[], geos:[{name:"source area", variable:"$area"}]}}};
  const form = input => rawRouting(parseConfig(input), state);
  const updated = form("geo $AREA { default local; }").value.routing.geos;
  assert.equal(updated.length, 1);
  assert.equal(updated[0].name, "source area");
  assert.equal(updated[0].default, "local");
  assert.throws(() => form("map $host $AREA { default blue; }"), /第 1 行.*重名/);
  assert.throws(() => form("map $host $pick { default blue; }\ngeo $PICK { default local; }"), /第 2 行.*重名/);
  assert.equal(state.settings.routing.geos[0].variable, "$area");
});
