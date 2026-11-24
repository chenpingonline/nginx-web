import assert from "node:assert/strict";
import test from "node:test";
import {
  importBlocks,
  parseConfig,
  sourceEntries,
  sourceBlock,
  updateSourceBlock,
} from "../src/nginxConfig.ts";

test("preserve line offsets, skip maps, and handle escaped or quoted delimiters", () => {
  const content =
    '# comment\nmap $host $v { default "a; }"; }\nserver { listen 443 ssl; server_name one.test; location / { proxy_pass http://127.0.0.1:9080; } }\nserver { listen 9080; return 200 ${name}; }';
  const entries = sourceEntries([{ name: "custom-one.conf", content }]);
  assert.equal(entries.length, 2);
  assert.deepEqual(entries[0].protocols, ["https"]);
  assert.equal(entries[1].line, 4);
  assert.equal(content.slice(entries[1].start, entries[1].start + 6), "server");
  assert.throws(() => parseConfig("server { listen 80;"), /第 1 行/);
});
test("multi-server import, unsupported directives and ambiguous upstream URI", () => {
  const raw =
    "server {listen 19080;server_name test.local; location / { proxy_pass http://127.0.0.1:9000; proxy_read_timeout 5m; expires 3d; }}";
  assert.equal(
    importBlocks(raw + "\n" + raw.replace("test.local", "two.local"), "").rules
      .length,
    2,
  );
  assert.equal(
    importBlocks(raw, "").rules[0].root_location.read_timeout_seconds,
    300,
  );
  assert.throws(
    () => importBlocks(raw.replace("expires 3d;", "\ninclude extra.conf;"), ""),
    /第 2 行/,
  );
  assert.throws(
    () => importBlocks(raw.replace(":9000;", ":9000/$request_uri;"), ""),
    /proxy_pass/,
  );
});

test("preserve a proxy_pass root URI independently for each location", () => {
  const content = `server {
    listen 19080;
    server_name files.test;
    location /files/ { proxy_pass http://127.0.0.1:9000/; expires 30m; add_header Cache-Control public always; }
    location = /keep { proxy_pass https://[::1]; }
    location / { return 404; }
  }`;
  const rule = importBlocks(content, "").rules[0];
  assert.equal(rule.root_location.return_code, 404);
  assert.deepEqual(rule.locations.map(l => l.settings.upstream_path), ["/", ""]);
  const rootProxy = importBlocks(content.replace("return 404;", "proxy_pass https://[::1]/;"), "").rules[0];
  assert.equal(rootProxy.root_location.upstream_path, "/");
  assert.equal(rootProxy.root_location.upstream_port, 443);
  assert.throws(() => importBlocks(content.replace("location /files/", "location ~ /files/"), ""), /proxy_pass/);
});

test("edit only selected server parameters and preserve surrounding source", async () => {
  const { parameterGroups, editParameters } = await import("../src/nginxConfig.ts");
  const content = '# test\nserver { listen 9090 ssl; server_name a b; location / { proxy_set_header X "a;b"; proxy_pass http://old; # note\ninclude custom; } }\nserver { location / { proxy_pass http://old; } }\n';
  const groups = parameterGroups(sourceEntries([{name: "custom-test.conf", content}])[0]);
  assert.deepEqual(groups.map(g => g.title), ["server", "server › location /"]);
  const fields = groups.flatMap(g => g.fields);
  assert.equal(editParameters(content, fields), content);
  const target = fields.find(f => f.original === "http://old");
  target.value = "http://new";
  assert.equal(editParameters(content, fields), content.replace("http://old", "http://new"));
  for (const value of ["", "foo; return 200", "foo # comment", "foo {}"]) {
    target.value = value;
    assert.throws(() => editParameters(content, fields));
  }
});

test("whole-file form preserves scopes, data rows, comments and module directives", async () => {
  const { parameterGroups, editParameters } = await import("../src/nginxConfig.ts");
  const content = [
    "map $host $target {",
    '  "a b" backend; # routing',
    "}",
    "upstream backend { server 127.0.0.1:8011 weight=3; keepalive 8; }",
    "server { location / { if ($arg_test) { custom_module foo",
    "  # keep this comment",
    "  bar; } } }",
    "server { return 204; }",
  ].join("\n");
  const fields = parameterGroups({file: { name: "custom-many.conf", content }}).flatMap(g => g.fields);
  const row = fields.find(f => f.label === "映射条目");
  assert.equal(row.original, '"a b" backend');
  row.value = '"c d" backend';
  fields.find(f => f.directive === "keepalive").value = "12";
  const custom = fields.find(f => f.directive === "custom_module");
  custom.value = custom.original.replace("bar", "baz");
  assert.equal(editParameters(content, fields), content.replace('"a b"', '"c d"').replace("keepalive 8", "keepalive 12").replace("bar;", "baz;"));
  custom.value = "foo;\nadd_header X bad";
  assert.throws(() => editParameters(content, fields));
});

test("import dual-stack SPA with per-path proxy headers, caching and timeouts", () => {
  const config = `server {
    listen 18959;
    listen [::]:18959;
    server_name 192.168.31.95;
    root /vol1/test/points-admin;
    index index.html;
    client_max_body_size 55m;
    location / {
        try_files $uri $uri/ /index.html;
    }
    location /api/ {
        proxy_pass http://127.0.0.1:18080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }
    location /files/ {
        proxy_pass http://127.0.0.1:18081;
        expires 30m;
        add_header Cache-Control "public";
    }
}
`;
  const rule = importBlocks(config, "").rules[0];
  assert.equal(rule.listen_type, "dual");
  assert.equal(rule.listen_port, 18959);
  assert.equal(rule.client_max_body_mb, 55);
  assert.equal(rule.root_location.backend_type, "static");
  assert.equal(rule.root_location.static_path, "/vol1/test/points-admin");
  assert.deepEqual(rule.root_location.index_files, ["index.html"]);
  assert.deepEqual(rule.root_location.try_files, ["$uri", "$uri/", "/index.html"]);
  const [api, files] = rule.locations.map(location => location.settings);
  assert.equal(api.explicit_proxy_headers, true);
  assert.equal(api.request_headers.find(h => h.name === "Host").value, "$host");
  assert.equal(api.proxy_http_version, "1.1");
  assert.equal(api.read_timeout_seconds, 60);
  assert.equal(files.expires, "30m");
  assert.deepEqual(files.response_headers, [{name: "Cache-Control", value: "public", always: false}]);
  assert.throws(() => importBlocks(config.replace("[::]:18959", "[::]:19000"), ""), /listen/);
  assert.throws(() => importBlocks(config.replace("try_files", "unsupported_option"), ""), /unsupported_option/);
});

test("editing a custom rule replaces only its UTF-8 block and retains its tracking identity", () => {
  const prefix = "# 另一个站点\nserver { listen 9090; }\n";
  const suffix = "\n# 结束\n";
  const rendered = 'server {\n    set $fnproxy_rule_id "new";\n    listen 9092;\n}\n';
  for (const tracking of ['', '    set $fnproxy_rule_id "original";\n']) {
    const original = "server {\n" + tracking + "    listen 9091;\n}";
    const file = { name: "custom.conf", content: prefix + original + suffix };
    const entry = sourceEntries([file])[1];
    const { source } = sourceBlock(entry);
    const { content, end } = updateSourceBlock(source, rendered);
    assert.equal(content, prefix + original.replace("9091", "9092") + suffix);
    assert.equal(end, Buffer.byteLength(content.slice(0, -suffix.length)));
  }
});

test("plain HTTP configuration restores advanced location handlers and hostname order", () => {
  for (const [directives, backend] of [
    ["grpc_pass grpcs://127.0.0.1:9000;", "grpc"],
    ["set $memcached_key $uri; memcached_pass 127.0.0.1:9000;", "memcached"],
    ["stub_status;", "status"],
    ["return 302 https://other.test$request_uri;", "return"],
    ["alias /srv/files/; autoindex on;", "static"],
  ]) {
    const config = `server { listen 9002; server_name z.test a.test; location / { ${directives} } }`;
    const rule = importBlocks(config, "").rules[0];
    assert.deepEqual(rule.domains, ["z.test", "a.test"]);
    assert.equal(rule.root_location.backend_type, backend);
  }
});

test("conversion errors refer to the complete file, including preceding server blocks", async () => {
  const { sourceRuleCandidate, ConfigLineError } = await import("../src/nginxConfig.ts");
  for (const newline of ["\n", "\r\n"]) {
    const lines = Array(40).fill("");
    lines[0] = "# 中文 🚀";
    lines[2] = "server { listen 8081; } server { listen 8082; }";
    lines[20] = "server { listen 8083; server_name test.local; location / { return 404; }";
    lines[33] = "location /app/ {";
    lines[34] = "    proxy_pass http://127.0.0.1:9000/$request_uri;";
    lines[35] = "} }";
    const entries = sourceEntries([{ name: "custom-test.conf", content: lines.join(newline) }]);
    assert.equal(entries[2].line, 21);
    assert.throws(() => sourceRuleCandidate(entries[2], []),
      error => error instanceof ConfigLineError && error.line === 35 && error.message.includes("第 35 行"));
  }
});

test("import literal proxy paths without adding or removing a slash", () => {
  for (const path of ["", "/", "/backend/", "/backend"]) {
    const config = `server { listen 9080; server_name files.test;
      location / { proxy_pass http://127.0.0.1:9000${path}; }
      location /app/ { proxy_pass https://[::1]:9443${path}; } }`;
    const rule = importBlocks(config, "").rules[0];
    assert.equal(rule.root_location.upstream_path, path);
    assert.equal(rule.locations[0].settings.upstream_path, path);
  }
});

test("refuse lossy handler combinations and rewrites after return", () => {
  const location = body => importBlocks("location /demo { " + body + " }", "target").location;
  assert.throws(() => location("return 204;\nrewrite ^ /changed permanent;"), /第 2 行.*rewrite/);
  assert.throws(() => location("return 204;\nproxy_pass http://127.0.0.1:8001;"), /第 2 行.*proxy_pass/);
  const rule = location("rewrite ^ /changed permanent; return 204;");
  assert.equal(rule.settings.rewrites[0].flag, "permanent");
  assert.equal(rule.settings.return_code, 204);
});

test("import extended path matches, access lists, filters and DAV defaults", () => {
  const config = `server { listen 19091; server_name matrix.test;
    location / { mirror /copy; sub_filter old new; valid_referers server_names; return 204; }
    location ^~ /files/ { root /srv/files; dav_methods PUT DELETE; allow 127.0.0.1; deny all; }
    location ~* ^/assets { return 203; }
    location @fallback { return 404; }
  }`;
  const rule = importBlocks(config, "").rules[0];
  assert.deepEqual(rule.locations.map(l => l.match), ["priority_prefix", "regex_i", "named"]);
  assert.deepEqual(rule.locations[0].settings.deny, ["all"]);
  assert.equal(rule.locations[0].settings.dav.min_delete_depth, 0);
  assert.equal(rule.root_location.sub_filter_once, true);
  assert.equal(rule.root_location.mirror_request_body, true);
  assert.throws(() => importBlocks(config.replace("return 404", "proxy_pass http://127.0.0.1:9000/path"), ""), /proxy_pass/);
});

test("Try Files accepts Nginx arguments and legacy commas, preserving quoted filenames", async () => {
  const {parseTryFiles, formatTryFiles} = await import("../src/tryFiles.ts");
  const spa = ["$uri", "$uri/", "/index.html"];
  assert.deepEqual(parseTryFiles("$uri $uri/ /index.html"), spa);
  assert.deepEqual(parseTryFiles("$uri, $uri/, /index.html"), spa);
  assert.deepEqual(parseTryFiles(""), []);
  const paths = ["/one two", "/a,b", "/a'b", '/a"b', "/a\\b", "=404"];
  assert.deepEqual(parseTryFiles(formatTryFiles(paths)), paths);
  assert.throws(() => parseTryFiles("$uri"), /回退/);
  assert.throws(() => parseTryFiles('$uri "missing'), /引号/);
});

test("resolve declared HTTP pools without losing the original Host or URI", () => {
  const id = "abcdefabcdef";
  const pools = new Map([["api_pool", id]]);
  const body = directives => `server { listen 19092; server_name pools.test; location / {
    proxy_pass http://api_pool/backend/; ${directives}
  } }`;
  const root = importBlocks(body(""), "", 1, pools).rules[0].root_location;
  assert.equal(root.upstream_pool_id, id);
  assert.equal(root.upstream_path, "/backend/");
  const mixed = importBlocks(body("").replace("api_pool/", "API_Pool/"), "", 1, pools).rules[0].root_location;
  assert.equal(mixed.upstream_pool_id, id);
  assert.equal(mixed.request_headers[0].value, "API_Pool");
  assert.deepEqual(root.request_headers.map(h => [h.name,h.value]), [["Host", "api_pool"]]);
  const headers = importBlocks(body("proxy_set_header Host $host; add_header X-Backend $proxy_host;"), "", 1, pools).rules[0].root_location;
  assert.equal(headers.request_headers[0].value, "$host");
  assert.equal(headers.response_headers[0].value, "api_pool");
  assert.equal(importBlocks(body("").replace("api_pool", "fnproxy_up_" + id), "").rules[0].root_location.upstream_pool_id, id);
  assert.throws(() => importBlocks(body(""), ""), /proxy_pass/);
  assert.throws(() => importBlocks(body("proxy_ssl_server_name on;").replace("http:", "https:"), "", 1, pools), /proxy_pass/);
});

test("recover server-level static root and Nginx-escaped location regexes", () => {
  const source = String.raw`server { listen 19093; server_name spa.test;
    root /srv/www; index home.html;
    location ~* "\\.(js|css)$" { expires 1h; }
    location ~ \.txt$ { return 204; }
  }`;
  const rule = importBlocks(source, "").rules[0];
  assert.equal(rule.root_location.static_path, "/srv/www");
  assert.deepEqual(rule.root_location.index_files, ["home.html"]);
  assert.deepEqual(rule.locations.map(l => l.path), [String.raw`\.(js|css)$`, String.raw`\.txt$`]);
});

test("restore independent effective buffering for each location and standalone fragments", () => {
  const config = `server {listen 19094; server_name ws.test;
    location / {proxy_pass http://127.0.0.1:8080; proxy_buffering off;}
    location /mixed {proxy_pass http://127.0.0.1:8080; proxy_buffering on; proxy_request_buffering off;}
    location /inherited {proxy_pass http://127.0.0.1:8080;}
  }`;
  const rule = importBlocks(config, "").rules[0];
  assert.equal(rule.root_location.proxy_buffering, false);
  assert.equal(rule.root_location.proxy_request_buffering, undefined);
  assert.equal(rule.locations[0].settings.proxy_buffering, true);
  assert.equal(rule.locations[0].settings.proxy_request_buffering, false);
  assert.equal(rule.locations[1].settings.proxy_buffering, undefined);
  const raw = "location /ws/ {proxy_pass http://127.0.0.1:8080; proxy_request_buffering on;}";
  assert.equal(importBlocks(raw, "target").location.settings.proxy_request_buffering, true);
  assert.throws(() => importBlocks(raw.replace("on;", "maybe;"), "target"), /proxy_request_buffering/);
  assert.throws(() => importBlocks(raw.replace("on;", "on; proxy_request_buffering off;"), "target"), /proxy_request_buffering/);
});

test("convert wildcard listeners, older TLS HTTP/2 syntax and integral MiB request limits", () => {
  const cert = "/cert/abcdefabcdef/";
  const config = `server {listen *:19443 ssl http2; listen [::]:19443 ssl http2; server_name site.test;
    ssl_certificate ${cert}fullchain.pem; ssl_certificate_key ${cert}privkey.pem;
    client_max_body_size 2048k; location / { return 204; }}`;
  const rule = importBlocks(config, "").rules[0];
  assert.equal(rule.listen_type, "dual");
  assert.equal(rule.http2, true);
  assert.equal(rule.client_max_body_mb, 2);
  assert.throws(() => importBlocks(config.replace("2048k", "512k"), ""), /client_max_body_size/);
  assert.throws(() => importBlocks(config.replace("[::]:19443 ssl http2", "[::]:19443 ssl"), ""), /listen/);
});

test("persist and restore the original upstream redirect mapping", () => {
  const id = "abcdefabcdef", pools = new Map([["old_pool", id]]);
  const parse = (name, uri, redirect) => importBlocks(`location /api/ { proxy_pass http://${name}${uri}; ${redirect} }`, "target", 1, pools).location.settings;
  for (const uri of ["", "/backend/", "/backend"]) {
    const stored = parse("Old_Pool", uri, "proxy_redirect default;");
    assert.deepEqual(stored.proxy_redirect, {pool_id: id, original_host: "Old_Pool"});
    const canonical = `proxy_redirect "http://Old_Pool${uri || "/"}" "${uri ? "/api/" : "/"}";`;
    assert.deepEqual(parse("fnproxy_up_" + id, uri, canonical).proxy_redirect, stored.proxy_redirect);
  }
  assert.deepEqual(parse("Old_Pool", "/backend", "proxy_redirect off;").proxy_redirect, {pool_id: id, disabled: true});
  assert.throws(() => parse("Old_Pool", "", "proxy_redirect ~pattern replacement;"), /proxy_redirect/);
  assert.throws(() => parse("fnproxy_up_" + id, "", "proxy_redirect http://old/wrong/ /;"), /proxy_redirect/);
});

test("literal punctuation inside an unquoted filename keeps its original argument and offsets", async () => {
  const {parameterGroups, editParameters} = await import("../src/nginxConfig.ts");
  const raw = `server { location / { include /tmp/O'Reilly.inc; return 200 ends}here; } }`;
  const fields = parameterGroups({file: {name: "custom-files.conf", content: raw}}).flatMap(g => g.fields);
  assert.equal(fields.find(f => f.directive === "include").value, "/tmp/O'Reilly.inc");
  assert.equal(fields.find(f => f.directive === "return").value, "200 ends}here");
  assert.equal(editParameters(raw, fields), raw);
  assert.deepEqual(parseConfig(`if ($arg_x = "a b") {return 204;}`)[0].words, ["if", "($arg_x", "=", "a b", ")"]);
  assert.throws(() => parseConfig('include "one"two;'));
  const {parseTryFiles} = await import("../src/tryFiles.ts");
  assert.deepEqual(parseTryFiles("/O'Reilly.html =404"), ["/O'Reilly.html", "=404"]);
});

test("list the backends of conditional and nested locations under their own routes", () => {
  const source = `server {
    location /api {
      if ($arg_beta) { proxy_pass http://beta; }
      proxy_pass http://stable;
      location /api/php { fastcgi_pass unix:/run/api.sock; }
    }
    location /write { limit_except GET { proxy_pass http://writer; } }
    location /files { root /srv/www; }
  }`;
  const [site] = sourceEntries([{name: "custom-test.conf", content: source}]);
  assert.deepEqual(site.targets, ["http://beta", "http://stable", "unix:/run/api.sock", "http://writer"]);
  assert.deepEqual(site.routes, ["location /api → http://beta → http://stable", "location /api/php → unix:/run/api.sock",
    "location /write → http://writer", "location /files"]);
});
