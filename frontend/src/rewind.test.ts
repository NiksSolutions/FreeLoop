import assert from "node:assert/strict";
import { test } from "node:test";
import { setLanguage } from "./i18n.ts";
import { rewindCheck } from "./rewind.ts";

test("a rewind beside another session's turn is refused, naming it", () => {
  setLanguage("en");
  const c = rewindCheck({ others: [{ sessionId: "s2", title: "Tests" }], busy: "Build" });
  assert.match(c.refuse ?? "", /"Build" is working in this folder/);
  assert.equal(c.ask, undefined);
});

test("a rewind that takes other sessions' changes asks, naming them", () => {
  setLanguage("en");
  const c = rewindCheck({ others: [{ sessionId: "s2", title: "Tests" }, { sessionId: "s3", title: "Docs" }] });
  assert.equal(c.refuse, undefined);
  assert.match(c.ask ?? "", /"Tests", "Docs"\. Rewind anyway\?/);
  setLanguage("de");
  assert.match(rewindCheck({ others: [{ sessionId: "s2", title: "Tests" }] }).ask ?? "", /Trotzdem zurückspulen\?/);
  setLanguage("en");
});

test("a rewind that touches no other session asks nothing more", () => {
  assert.deepEqual(rewindCheck({ others: [] }), {});
  assert.deepEqual(rewindCheck({ others: null }), {});
  assert.deepEqual(rewindCheck(null), {});
});
