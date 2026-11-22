import { test } from "node:test";
import assert from "node:assert/strict";
import { around, endFor, SHOWN_ITEMS, shownFrom, startFor } from "./shownRuns.ts";
import type { Item, ItemRun } from "./transcript.ts";

// Runs of the given sizes, their items keyed by seq from 1.
function runsOf(sizes: number[]): ItemRun[] {
  let seq = 1;
  return sizes.map((n) => ({ items: Array.from({ length: n }, () => ({ key: String(seq++) }) as Item) }));
}

test("a conversation draws about its last SHOWN_ITEMS items at first", () => {
  const runs = runsOf(Array.from({ length: 1000 }, () => 1));
  assert.equal(shownFrom(runs, null), 1000 - SHOWN_ITEMS);
  assert.equal(shownFrom(runsOf([1, 2, 3]), null), 0, "a short one draws everything");
  assert.equal(shownFrom([], null), 0);
  // A run of many tool calls counts by its items, and is drawn whole.
  const big = runsOf([1, 1, 500, 1, 1]);
  assert.equal(shownFrom(big, null), 2);
});

test("earlier items are added about SHOWN_ITEMS at a time", () => {
  const runs = runsOf(Array.from({ length: 1000 }, () => 1));
  assert.equal(startFor(runs, 700), 700 - SHOWN_ITEMS);
  assert.equal(startFor(runs, 100), 0);
  assert.equal(startFor(runs, 0), 0);
});

test("what is drawn stays from the same item as runs are added before and after it", () => {
  const runs = runsOf([3, 1, 4, 1, 5]);
  const from = runs[3].items[0].key; // seq 9
  assert.equal(shownFrom(runs, from), 3);
  // The session's earlier events arrived: more runs before, the same keys after.
  const earlier = [...runsOf([2]).map((r) => ({ items: r.items.map((it) => ({ ...it, key: String(Number(it.key) - 10) })) })), ...runs];
  assert.equal(shownFrom(earlier, from), 4);
  // New items after it change nothing before it.
  assert.equal(shownFrom([...runs, { items: [{ key: "15" } as Item] }], from), 3);
  // The run it began merged with the one before: the merged run holds its place.
  const merged = [runs[0], runs[1], { items: [...runs[2].items, ...runs[3].items] }, runs[4]];
  assert.equal(shownFrom(merged, from), 2);
  // An item before everything held (the session was read again from a later start).
  assert.equal(shownFrom(runs.slice(2), "1"), 0);
});

test("a jump far back draws about SHOWN_ITEMS items around it, not all that follows", () => {
  const runs = runsOf(Array.from({ length: 10000 }, () => 1));
  const { start, end } = around(runs, 1000);
  assert.ok(start < 1000 && end > 1000, `${start} to ${end}`);
  assert.equal(end - start, SHOWN_ITEMS);
  assert.deepEqual(around(runs, 0), { start: 0, end: SHOWN_ITEMS / 2 });
  assert.equal(around(runs, 9999).end, 10000);
  // Later items are added about SHOWN_ITEMS at a time, up to the end.
  assert.equal(endFor(runs, 5000), 5000 + SHOWN_ITEMS);
  assert.equal(endFor(runs, 9900), 10000);
});

