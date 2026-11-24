import assert from "node:assert/strict";
import { after, test } from "node:test";
import { createServer } from "vite";

const vite = await createServer({configFile: false, optimizeDeps: {noDiscovery: true}, server: {middlewareMode: true, hmr: false}, appType: "custom"});
after(() => vite.close());
const {parseSourceProxyTarget, formatSourceProxyTarget} = await vite.ssrLoadModule("/src/sourceProxyTarget.ts");
const {editParameters, parameterGroups} = await vite.ssrLoadModule("/src/nginxConfig.ts");

test("round trips HTTP target expressions and explicit Unix URI delimiters", () => {
  for (const raw of [
    "http://$pool/O'Reilly", 'http://unix:/run/mark"one.sock:/app', "http://$pool/path}end",
    "$origin", "${scheme}://${origin}:$port$request_uri", "http://$pool$request_uri", "https://[::1]:8443/$1?x=$arg_x",
    "'HTTP://${upstream}/path?next=$uri'", "http://unix:/var/run/site.sock", "http://unix:/var/run/site.sock:",
    '"https://unix:/run/site one.sock:/a/$1?x=$args"', "http://unix:/run/site.sock:$request_uri",
    "http://unix:/run/site.sock:/a;b".replace(";", "\\;"), String.raw`http://$pool/it\'s`,
  ]) {
    const target = parseSourceProxyTarget(raw);
    assert.ok(target, raw);
    assert.equal(formatSourceProxyTarget(target), raw);
  }
  assert.equal(parseSourceProxyTarget("http://127.0.0.1:8080"), undefined);
  assert.equal(parseSourceProxyTarget("http://$pool extra_arg"), undefined);
});

test("edit a socket path without rewriting the surrounding URI, condition or comment", () => {
  const config = `server {
  if ($arg_x ~ "^a[0-9]+$") { return 204; }
  location / { proxy_pass 'http://unix:/run/web.sock:/path/$1?x=$arg_x'; # preserve this
  }
}`;
  const fields = parameterGroups({file: {name:"custom-proxy.conf", content: config}}).flatMap(g => g.fields);
  const targetField = fields.find(f => f.directive === "proxy_pass");
  const target = parseSourceProxyTarget(targetField.value);
  assert.equal(target.path, "/run/web.sock");
  assert.equal(target.uri, "/path/$1?x=$arg_x");
  target.path = "/run/web two.sock";
  targetField.value = formatSourceProxyTarget(target);
  assert.equal(editParameters(config, fields), config.replace("/run/web.sock", "/run/web two.sock"));
  target.path = "/run/web.sock:/unexpected";
  assert.throws(() => formatSourceProxyTarget(target), /Unix socket/);
});

test("edit variable targets as one argument with safe quoting", () => {
  const expression = parseSourceProxyTarget("http://$upstream");
  expression.expression = "${backend}$uri$is_args$args";
  assert.equal(formatSourceProxyTarget(expression), "http://${backend}$uri$is_args$args");
  const socket = parseSourceProxyTarget("http://unix:/run/site.sock");
  socket.path = '/run/my "site".sock';
  const raw = formatSourceProxyTarget(socket);
  assert.equal(raw, String.raw`"http://unix:/run/my \"site\".sock"`);
  assert.equal(formatSourceProxyTarget(parseSourceProxyTarget(raw)), raw);
  socket.uri = "/a;b";
  assert.ok(parseSourceProxyTarget(formatSourceProxyTarget(socket)));
  socket.path = "/run/bad\nname";
  assert.throws(() => formatSourceProxyTarget(socket));
});

test("stream Unix sockets have no URI delimiter; bare stream variables stay intact", () => {
  const target = parseSourceProxyTarget("unix:/run/backend:1.sock", true);
  assert.equal(target.path, "/run/backend:1.sock");
  assert.equal(formatSourceProxyTarget(target), "unix:/run/backend:1.sock");
  assert.equal(formatSourceProxyTarget(parseSourceProxyTarget("$sni_backend", true)), "$sni_backend");
});
