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
    () => importBlocks(raw.replace(":9000;", ":9000/app/;"), ""),
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
  assert.deepEqual(rule.locations.map(l => l.settings.proxy_pass_trailing_slash), [true, false]);
  const rootProxy = importBlocks(content.replace("return 404;", "proxy_pass https://[::1]/;"), "").rules[0];
  assert.equal(rootProxy.root_location.proxy_pass_trailing_slash, true);
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
    lines[34] = "    proxy_pass http://127.0.0.1:9000/app/;";
    lines[35] = "} }";
    const entries = sourceEntries([{ name: "custom-test.conf", content: lines.join(newline) }]);
    assert.equal(entries[2].line, 21);
    assert.throws(() => sourceRuleCandidate(entries[2], []),
      error => error instanceof ConfigLineError && error.line === 35 && error.message.includes("第 35 行"));
  }
});
