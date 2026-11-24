import assert from "node:assert/strict";
import test from "node:test";
import { parseConfig } from "../src/nginxConfig.ts";
import { routingDirectivesMatch } from "../src/routingComparison.ts";

const matches = (input, rendered) => routingDirectivesMatch(parseConfig(input)[0], parseConfig(rendered)[0]);

test("map and geo support an implicit empty default or a default after entries", () => {
  for (const [block, entry] of [["map $host $pick", "a.test blue;"], ["geo $area", "127.0.0.1 local;"]]) {
    assert.ok(matches(block + " { " + entry + " }", block + ' { default ""; ' + entry + " }"));
    assert.ok(matches(block + " { " + entry + " default gray; }", block + " { default gray; " + entry + " }"));
    assert.ok(!matches(block + " { " + entry + " }", block + " { default gray; " + entry + " }"));
    const repeated = block + " { default blue; default gray; }";
    assert.ok(!matches(repeated, repeated));
  }
});

test("preserve regex precedence, /0 defaults, and split-client ranges", () => {
  assert.ok(!matches("map $host $a { ~a x; ~ab y; default z; }", "map $host $a { default z; ~ab y; ~a x; }"));
  assert.ok(!matches("geo $a { 0.0.0.0/0 x; default y; }", "geo $a { default y; 0.0.0.0/0 x; }"));
  assert.ok(!matches("geo $a { ::/0 x; default y; }", "geo $a { default y; ::/0 x; }"));
  assert.ok(!matches("split_clients $key $v { 10% a; 5% b; * c; }", "split_clients $key $v { 5% b; 10% a; * c; }"));
  assert.ok(!matches("map $src $a { default x; }", "map $other $a { default x; }"));
});
