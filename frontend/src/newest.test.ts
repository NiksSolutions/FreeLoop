import { test } from "node:test";
import assert from "node:assert/strict";
import { newest } from "./newest.ts";

// A reply is taken only for the newest ask: an older one arriving late, or failing
// late, does not undo it.
test("only the newest read's reply is used", async () => {
  const read = newest();
  let resolveOld!: (v: string) => void;
  let rejectOlder!: (e: Error) => void;
  const older = new Promise<string>((_, reject) => (rejectOlder = reject));
  const old = new Promise<string>((resolve) => (resolveOld = resolve));
  const used: string[] = [];
  const a = read(older, (v) => used.push(v));
  const b = read(old, (v) => used.push(v));
  const c = read(Promise.resolve("new"), (v) => used.push(v));
  await c;
  resolveOld("old");
  rejectOlder(new Error("late"));
  await b;
  await a;
  assert.deepEqual(used, ["new"]);
  // The newest read's own failure is the caller's to show.
  await assert.rejects(read(Promise.reject(new Error("now")), () => {}), /now/);
});

test("each list keeps its own count", async () => {
  const projects = newest();
  const styles = newest();
  const used: string[] = [];
  await Promise.all([projects(Promise.resolve("p"), (v) => used.push(v)), styles(Promise.resolve("s"), (v) => used.push(v))]);
  assert.deepEqual(used.sort(), ["p", "s"]);
});
