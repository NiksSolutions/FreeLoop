// The window's one writer of the settings (issue 091). The Settings page, the text
// size keys, the fonts and style chosen, and every other page that changes config.yaml
// send what they changed as a JSON merge patch (RFC 7386), one at a time in the order
// made, and the engine merges each into what it has. So no change carries another's
// older values, and a change made while one is being saved, even one back to how
// things were, is saved after it. What the engine answers is handed on only when no
// later change waits, so the window never takes up a state older than one it had.

export type Patch = { [key: string]: unknown };

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

// patchOf is the merge patch that turns before into after, or null when there is no
// difference. A member gone from after is null; a list is sent whole.
export function patchOf(before: unknown, after: unknown): Patch | null {
  if (!isObject(before) || !isObject(after)) return null;
  const out: Patch = {};
  for (const k of new Set([...Object.keys(before), ...Object.keys(after)])) {
    const a = before[k];
    const b = after[k];
    if (b === undefined) {
      if (a !== undefined) out[k] = null;
    } else if (isObject(a) && isObject(b)) {
      const inner = patchOf(a, b);
      if (inner) out[k] = inner;
    } else if (JSON.stringify(a) !== JSON.stringify(b)) {
      out[k] = b;
    }
  }
  return Object.keys(out).length > 0 ? out : null;
}

// applyPatch is target with patch merged in, as the engine merges it.
export function applyPatch<T>(target: T, patch: Patch): T {
  const out: Record<string, unknown> = isObject(target) ? { ...target } : {};
  for (const [k, v] of Object.entries(patch)) {
    if (v === null) delete out[k];
    else out[k] = isObject(v) ? applyPatch(out[k], v) : v;
  }
  return out as T;
}

export interface Saver<C> {
  // change sends patch after every change before it; null, nothing to send.
  change(patch: Patch): Promise<C>;
  change(patch: Patch | null): Promise<C> | null;
  // withPending is c with every change not yet saved merged in: what the engine will
  // have, so a page compares a new edit with that rather than with c.
  withPending(c: C): C;
}

// Told is what a saver tells besides its answers: queued once a change waits, so the
// window can show it before the engine answers (withPending), and failed when one
// could not be saved, so the window reads the settings again.
export interface Told {
  queued?: () => void;
  failed?: () => void;
}

// createSaver sends each change with send, in turn, and hands deliver the engine's
// answer to the last of a run of them.
export function createSaver<C>(send: (patch: Patch) => Promise<C>, deliver: (c: C) => void, told: Told = {}): Saver<C> {
  const pending: Patch[] = [];
  let turn: Promise<unknown> = Promise.resolve();
  function change(patch: Patch): Promise<C>;
  function change(patch: Patch | null): Promise<C> | null;
  function change(patch: Patch | null): Promise<C> | null {
    if (!patch) return null;
    pending.push(patch);
    told.queued?.();
    const sent = turn.then(() => send(patch));
    turn = sent.catch(() => null);
    const done = () => {
      pending.splice(pending.indexOf(patch), 1);
    };
    return sent.then(
      (c) => {
        done();
        if (pending.length === 0) deliver(c);
        return c;
      },
      (err: unknown) => {
        done();
        told.failed?.();
        throw err;
      },
    );
  }
  return { change, withPending: (c) => pending.reduce((acc, p) => applyPatch(acc, p), c) };
}
