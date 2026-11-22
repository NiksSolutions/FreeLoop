import type { ItemRun } from "./transcript.ts";

// A long session's conversation draws its last items, and earlier ones as the reader
// nears the top of what is drawn (issue 046): drawing every item of a session of 50,000
// events took seconds, while what is on screen is a few dozen.

// SHOWN_ITEMS is about how many items are drawn at first, and how many more each time.
export const SHOWN_ITEMS = 300;

// startFor is the first of the runs before end that together hold about count items.
export function startFor(runs: readonly ItemRun[], end: number, count = SHOWN_ITEMS): number {
  let i = Math.min(end, runs.length);
  for (let n = 0; i > 0 && n < count; ) n += runs[--i].items.length;
  return i;
}

// endFor is the end of the runs from start on that together hold about count items:
// the index after the last of them.
export function endFor(runs: readonly ItemRun[], start: number, count = SHOWN_ITEMS): number {
  let i = Math.max(start, 0);
  for (let n = 0; i < runs.length && n < count; ) n += runs[i++].items.length;
  return i;
}

// around is what a jump to the run at draws: about SHOWN_ITEMS items with it in the
// middle, as start and end indexes; a jump far back does not draw all that follows.
export function around(runs: readonly ItemRun[], at: number): { start: number; end: number } {
  return { start: startFor(runs, at, SHOWN_ITEMS / 2), end: endFor(runs, at, SHOWN_ITEMS / 2) };
}

// shownFrom is the index of the first run drawn: the one that holds the item keyed from,
// or, before anything is drawn, the start of about the last SHOWN_ITEMS items. Keys are
// the seqs items began at, so runs ascend by them, and an item gone since (a run that
// merged with the one before it) leaves the run that now holds its place.
export function shownFrom(runs: readonly ItemRun[], from: string | null): number {
  if (from === null || runs.length === 0) return startFor(runs, runs.length);
  const at = Number(from);
  let lo = 0;
  let hi = runs.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (Number(runs[mid].items[0].key) <= at) lo = mid + 1;
    else hi = mid;
  }
  return Math.max(lo - 1, 0);
}
