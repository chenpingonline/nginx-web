import assert from "node:assert/strict";
import test from "node:test";
import {
  importBlocks,
  parseConfig,
  sourceEntries,
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

test("edit only selected server parameters and preserve surrounding source", async () => {
  const { parameterGroups, editParameters } = await import("../src/nginxConfig.ts");
  const content = '# test\nserver { listen 9090 ssl; server_name a b; location / { proxy_set_header X "a;b"; proxy_pass http://old; # note\ninclude custom; } }\nserver { location / { proxy_pass http://old; } }\n';
  const groups = parameterGroups(sourceEntries([{name: "custom-test.conf", content}])[0]);
  assert.deepEqual(groups.map(g => g.title), ["server", "location /"]);
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
