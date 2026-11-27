import { test } from "node:test";
import assert from "node:assert/strict";
import { applyPatch, createSaver, patchOf, type Patch } from "./configSave.ts";

// What is due to run, has.
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

test("a patch holds only what changed, and a key gone is null", () => {
  const before = { ui: { fontSize: 14, language: "en", shortcuts: { textLarger: "Meta+Shift+L" } }, hooks: [{ id: "a" }], web: { searchURL: "" } };
  const after = { ui: { fontSize: 14, language: "de", shortcuts: {} }, hooks: [{ id: "a" }, { id: "b" }], web: { searchURL: "" } };
  assert.deepEqual(patchOf(before, after), { ui: { language: "de", shortcuts: { textLarger: null } }, hooks: [{ id: "a" }, { id: "b" }] });
  assert.equal(patchOf(before, structuredClone(before)), null);
  assert.deepEqual(patchOf({ a: null }, { a: { b: 1 } }), { a: { b: 1 } });
  assert.deepEqual(patchOf({ a: [1] }, { a: null }), { a: null });
});

test("a patch applied gives what it was taken from, as the engine merges it", () => {
  const before = { ui: { fontSize: 14, language: "en", shortcuts: { textLarger: "Meta+Shift+L", palette: "Meta+K" } }, filters: [{ name: "x" }] };
  const after = { ui: { fontSize: 15, language: "en", shortcuts: { palette: "Meta+P" } }, filters: [] };
  assert.deepEqual(applyPatch(before, patchOf(before, after)!), after);
  assert.deepEqual(before.ui.shortcuts, { textLarger: "Meta+Shift+L", palette: "Meta+K" }, "the target is left as it was");
});

// The race CI's runner found: a shortcut recorded, then Reset while its save is under
// way. Compared with what will be saved, the Reset is a change and is sent after it.
test("a change back to how things were, made while a save is under way, is saved after it", async () => {
  let disk: Record<string, unknown> = { ui: { fontSize: 14, shortcuts: {} } };
  const sent: Patch[] = [];
  const answers: Array<() => void> = [];
  const delivered: unknown[] = [];
  const saver = createSaver(
    (p) => {
      sent.push(p);
      return new Promise((resolve) => answers.push(() => resolve((disk = applyPatch(disk, p)))));
    },
    (c) => delivered.push(c),
  );
  const config = structuredClone(disk);
  const recorded = { ui: { fontSize: 14, shortcuts: { textLarger: "Meta+Shift+L" } } };
  const first = saver.change(patchOf(saver.withPending(config), recorded));
  const reset = saver.change(patchOf(saver.withPending(config), { ui: { fontSize: 14, shortcuts: {} } }));
  assert.ok(first && reset, "the Reset is a change against what is being saved");
  await settle();
  assert.equal(sent.length, 1, "one save at a time");
  answers[0]();
  await first;
  await settle();
  assert.equal(delivered.length, 0, "the first answer is not handed on while the Reset waits");
  assert.equal(sent.length, 2);
  answers[1]();
  await reset;
  assert.deepEqual(disk, { ui: { fontSize: 14, shortcuts: {} } });
  assert.deepEqual(delivered, [disk]);
});

test("what waits is merged into a config, and a failed change stops waiting", async () => {
  const fail = new Error("no");
  const saver = createSaver<Record<string, unknown>>((p) => ("bad" in p ? Promise.reject(fail) : Promise.resolve(p)), () => {});
  const good = saver.change({ ui: { fontSize: 16 } });
  const bad = saver.change({ bad: 1 });
  assert.deepEqual(saver.withPending({ ui: { fontSize: 14, language: "en" } }), { ui: { fontSize: 16, language: "en" }, bad: 1 });
  await good;
  await assert.rejects(bad!, fail);
  assert.deepEqual(saver.withPending({ ui: { fontSize: 14 } }), { ui: { fontSize: 14 } });
  assert.equal(saver.change(null), null);
});

test("a change shows as soon as it waits, and a failed one has the window read again", async () => {
  const calls: string[] = [];
  let fail = false;
  const saver = createSaver<Record<string, unknown>>(
    (p) => (fail ? Promise.reject(new Error("no")) : Promise.resolve(p)),
    () => calls.push("delivered"),
    { queued: () => calls.push("queued"), failed: () => calls.push("failed") },
  );
  const first = saver.change({ ui: { language: "de" } });
  assert.deepEqual(calls, ["queued"], "told at once, before any answer");
  await first;
  fail = true;
  await assert.rejects(saver.change({ ui: { language: "nl" } }));
  assert.deepEqual(calls, ["queued", "delivered", "queued", "failed"]);
});
