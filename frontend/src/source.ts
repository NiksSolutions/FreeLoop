// What a session runs on, and the mark that shows it at a glance (GUI-034): an external
// agent if it has one, else its provider. A mark is a colour and the vendor's logo, or two
// letters where there is no logo or the user chose letters; colour alone would fail anyone
// who cannot tell the colours apart. Pure, so source.test.ts covers it.
import { locale, t } from "./i18n.ts";
import { agentVendor, providerVendor, type Vendor } from "./vendors.ts";

export const SLOTS = 8;

export type Named = { id: string; name: string };
// A provider as the engine lists it: its type and preset say whose it is.
export type ProviderNamed = Named & { type?: string; preset?: string };
type Runs = { harnessId?: string | null; providerId?: string | null };

export interface SourceChoice {
  slot?: number;
  letters?: string;
  // A PNG as a data: URL (config.ValidSourceIcon).
  icon?: string;
}
export type SourceChoices = Record<string, SourceChoice> | null | undefined;

export interface Source {
  id: string;
  name: string;
  letters: string;
  // 1 to SLOTS; 0 is the neutral colour, for what no longer exists.
  slot: number;
  // Whose logo the mark shows instead of the letters; none for a custom endpoint, the
  // demo, an agent of the user's own, or letters the user chose.
  vendor?: Vendor | null;
  // The picture the user chose, shown before anything else.
  icon?: string | null;
}

const external = (r: Runs) => !!r.harnessId && r.harnessId !== "builtin";

// sourceName is the one rule for what a session or a row of usage runs on.
export function sourceName(r: Runs, providers: readonly Named[], harnesses: readonly Named[]): string {
  return external(r)
    ? (harnesses.find((h) => h.id === r.harnessId)?.name ?? r.harnessId!)
    : (providers.find((p) => p.id === r.providerId)?.name ?? (r.providerId || t("app.source.unknown")));
}

export function sourceId(r: Runs): string {
  return (external(r) ? r.harnessId : r.providerId) || "";
}

// defaultLetters are a name's initials, or the start of one word.
export function defaultLetters(name: string): string {
  const words = name.split(/[\s._/-]+/u).filter(Boolean);
  const letters = words.length >= 2 ? words[0][0] + words[1][0] : Array.from(words[0] ?? "?").slice(0, 2).join("");
  return letters.toLocaleUpperCase(locale());
}

// validIcon is the start of config.ValidSourceIcon: the engine checks the rest.
export function validIcon(v: unknown): v is string {
  return typeof v === "string" && v.startsWith("data:image/png;base64,");
}

// validLetters is config.validSourceLetters: at most two letters or digits.
export function validLetters(s: string): boolean {
  return /^[\p{L}\p{N}]{1,2}$/u.test(s);
}

const validSlot = (n: unknown): n is number => typeof n === "number" && Number.isInteger(n) && n >= 1 && n <= SLOTS;

// A provider's own slot comes from its ID, so a colour does not depend on what else there
// is or how much it is used.
function hash(id: string): number {
  let h = 2166136261;
  for (const c of id) h = Math.imul(h ^ c.codePointAt(0)!, 16777619);
  return (h >>> 0) % SLOTS;
}

// assignSlots gives each ID a slot: the user's choice first, then its own, moving on to the
// next free one when taken, in the order of the IDs. With more IDs than slots, they share.
export function assignSlots(ids: readonly string[], choices: SourceChoices): Map<string, number> {
  const out = new Map<string, number>();
  const taken = new Set<number>();
  const sorted = [...new Set(ids)].filter(Boolean).sort();
  for (const id of sorted) {
    const chosen = choices?.[id]?.slot;
    if (validSlot(chosen)) {
      out.set(id, chosen);
      taken.add(chosen);
    }
  }
  for (const id of sorted) {
    if (out.has(id)) continue;
    let at = hash(id);
    for (let tried = 0; tried < SLOTS && taken.has(at + 1); tried++) at = (at + 1) % SLOTS;
    out.set(id, at + 1);
    taken.add(at + 1);
  }
  return out;
}

// sourceIndex makes the function the views ask. What no longer exists is neutral, and says
// its ID, since its name is gone with it. A list that has not arrived yet says nothing of
// the kind: the agents take a moment, each being looked for through the login shell, and a
// session must not look deleted until they have.
export function sourceIndex(providers: readonly ProviderNamed[], harnesses: readonly Named[], sessions: readonly Runs[], choices: SourceChoices): (r: Runs) => Source {
  const known = [...providers.map((p) => p.id), ...harnesses.filter((h) => h.id !== "builtin").map((h) => h.id)];
  const waiting = sessions.filter((r) => (external(r) ? harnesses.length === 0 : providers.length === 0)).map(sourceId);
  const slots = assignSlots([...known, ...waiting], choices);
  return (r) => {
    const id = sourceId(r);
    const name = sourceName(r, providers, harnesses);
    const chosen = choices?.[id]?.letters;
    const letters = chosen && validLetters(chosen) ? chosen : "";
    const icon = validIcon(choices?.[id]?.icon) ? choices![id]!.icon! : null;
    return { id, name, letters: letters || defaultLetters(name), slot: slots.get(id) ?? 0, vendor: icon || letters ? null : vendorOf(r, providers), icon };
  };
}

// vendorOf is whose a session's source is, for its logo.
export function vendorOf(r: Runs, providers: readonly ProviderNamed[]): Vendor | null {
  if (external(r)) return agentVendor(r.harnessId!);
  const p = providers.find((x) => x.id === r.providerId);
  return p ? providerVendor(p) : null;
}

// sourceColor is a slot's colour, as a style sheet value.
export function sourceColor(slot: number): string {
  return slot >= 1 && slot <= SLOTS ? `var(--source-${slot})` : "var(--source-none)";
}
