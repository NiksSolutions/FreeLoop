import { beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import { draftOf, extrasOf, forgetDraft, forgetSent, pruneDrafts, rememberDraft, rememberExtras, storeDraft, unsentOf, unsentText, useDraftStorage } from "./drafts.ts";

function fakeStorage() {
  const m = new Map<string, string>();
  return {
    m,
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
    key: (i: number) => [...m.keys()][i] ?? null,
    get length() {
      return m.size;
    },
  };
}
const same = async (s: string) => s;
let disk = fakeStorage();
// Each test has sessions of its own, since what is in memory outlives a view on purpose.
let n = 0;
let sid = "";
beforeEach(() => {
  disk = fakeStorage();
  useDraftStorage(disk);
  sid = `ses_${++n}`;
});

test("a draft outlives its view and belongs to its session", () => {
  rememberDraft(sid, "half a thought");
  assert.equal(draftOf(sid), "half a thought");
  assert.equal(draftOf(`${sid}_other`), "");
});

test("a stored draft comes back after a restart", async () => {
  rememberDraft(sid, "half a thought");
  await storeDraft(sid, "half a thought", same);
  assert.equal(disk.getItem(`freeloop.draft.${sid}`), "half a thought");
});

test("what is stored is redacted, while what is typed stays as typed", async () => {
  rememberDraft(sid, "key sk-secret");
  await storeDraft(sid, "key sk-secret", async (s) => s.replace("sk-secret", "[REDACTED]"));
  assert.equal(disk.getItem(`freeloop.draft.${sid}`), "key [REDACTED]");
  assert.equal(draftOf(sid), "key sk-secret");
  assert.ok(![...disk.m.values()].join().includes("sk-secret"));
});

test("if redacting fails, nothing is written", async () => {
  rememberDraft(sid, "key sk-secret");
  await storeDraft(sid, "key sk-secret", async () => {
    throw new Error("no engine");
  });
  assert.equal(disk.m.size, 0);
});

test("a draft changed while redacting is not overwritten by the older text", async () => {
  rememberDraft(sid, "older");
  const storing = storeDraft(sid, "older", same);
  rememberDraft(sid, "");
  await storing;
  assert.equal(disk.m.size, 0);
});

test("sending or deleting the text removes it everywhere", async () => {
  rememberDraft(sid, "gone soon");
  await storeDraft(sid, "gone soon", same);
  rememberExtras(sid, ["picture"]);
  forgetDraft(sid);
  assert.equal(draftOf(sid), "");
  assert.equal(extrasOf(sid), undefined);
  assert.equal(disk.m.size, 0);
  // Emptied by hand, the stored copy goes too, and no older one comes back.
  rememberDraft(sid, "again");
  await storeDraft(sid, "again", same);
  rememberDraft(sid, "");
  assert.equal(draftOf(sid), "");
  assert.equal(disk.m.size, 0);
});

test("a very long draft stays in memory only", async () => {
  const long = "x".repeat(70 * 1024);
  rememberDraft(sid, long);
  await storeDraft(sid, long, same);
  assert.equal(draftOf(sid), long);
  assert.equal(disk.m.size, 0);
});

test("drafts of deleted sessions are removed, and nothing else", () => {
  disk.setItem("freeloop.draft.kept", "a");
  disk.setItem("freeloop.draft.gone", "b");
  disk.setItem("freeloop.other", "c");
  pruneDrafts((id) => id === "kept");
  assert.deepEqual([...disk.m.keys()].sort(), ["freeloop.draft.kept", "freeloop.other"]);
});

test("without storage it still works in memory", async () => {
  useDraftStorage({
    getItem() {
      throw new Error("blocked");
    },
    setItem() {
      throw new Error("blocked");
    },
    removeItem() {
      throw new Error("blocked");
    },
    key: () => null,
    length: 0,
  });
  rememberDraft(sid, "kept");
  await storeDraft(sid, "kept", same);
  assert.equal(draftOf(sid), "kept");
  rememberDraft(sid, "");
  assert.equal(draftOf(sid), "");
});

test("once a message is taken, only what was sent leaves the box", () => {
  assert.equal(unsentText("fix the test", "fix the test"), "");
  assert.equal(unsentText("  fix the test \n", "  fix the test \n"), "");
  // Typed after it while the engine was starting the agent.
  assert.equal(unsentText("fix the test\nand the docs", "fix the test"), "and the docs");
  assert.equal(unsentText("fix the test and", "fix the test "), "and");
  // Written anew, or changed inside what was sent: nothing typed is thrown away.
  assert.equal(unsentText("something else", "fix the test"), "something else");
  assert.equal(unsentText("fix a test", "fix the test"), "fix a test");
  // A message of pictures only leaves what was typed meanwhile.
  assert.equal(unsentText("about these", ""), "about these");
  const a = { name: "a" };
  const b = { name: "b" };
  assert.deepEqual(unsentOf([a, b], [a]), [b]);
  assert.deepEqual(unsentOf([b], [a]), [b]);
});

test("what was typed and attached while a message was being taken stays, stored too", async () => {
  const a = { name: "a" };
  const b = { name: "b" };
  rememberDraft(sid, "first part");
  rememberExtras(sid, { attached: [a], pictures: [] });
  // The view sends, then the user goes on typing and attaching while the engine waits.
  rememberDraft(sid, "first part\nsecond part");
  rememberExtras(sid, { attached: [a, b], pictures: [] });
  forgetSent(sid, { text: "first part", extras: { attached: [a], pictures: [] } }, same);
  assert.equal(draftOf(sid), "second part");
  assert.deepEqual(extrasOf(sid), { attached: [b], pictures: [] });
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(disk.getItem(`freeloop.draft.${sid}`), "second part");
  // Nothing new: the box and the stored copy are empty.
  forgetSent(sid, { text: "second part", extras: { attached: [b] } }, same);
  assert.equal(draftOf(sid), "");
  assert.deepEqual(extrasOf(sid), { attached: [], pictures: [] });
  assert.equal(disk.m.size, 0);
});

test("a command that sends no attachments leaves them", () => {
  const a = { name: "a" };
  rememberDraft(sid, "/review");
  rememberExtras(sid, { attached: [a] });
  forgetSent(sid, { text: "/review" }, same);
  assert.equal(draftOf(sid), "");
  assert.deepEqual(extrasOf(sid), { attached: [a] });
});
