// The user's saved styles (GUI-053): each a name, the colours of both
// appearances and the two fonts. Choosing one copies its values over the colours
// and fonts in force, so colors.json and config.yaml stay what the window shows;
// a style is "changed" when the look on screen is no longer what it holds. Kept
// apart from the components so it can be tested.
import type { ColorMap } from "./colors.ts";
import type { FontChoice } from "./fonts.ts";

type Entries = { [token: string]: string | undefined } | null | undefined;

// A style as the engine sends it; the bindings type a map's entries as optional.
export interface StyleValues {
  id: string;
  name: string;
  textFont: string;
  codeFont: string;
  light: Entries;
  dark: Entries;
  // A style the app ships: chosen and saved as new like any other, never updated
  // or deleted.
  builtin?: boolean;
}

// groupStyles parts the list into the user's styles and the app's own, each in
// the order given.
export function groupStyles<T extends { builtin?: boolean }>(styles: readonly T[]): { saved: T[]; builtin: T[] } {
  return { saved: styles.filter((s) => !s.builtin), builtin: styles.filter((s) => s.builtin) };
}

export interface Look {
  light: ColorMap;
  dark: ColorMap;
  textFont: string;
  codeFont: string;
}

function colorMap(m: Entries): ColorMap {
  const out: ColorMap = {};
  for (const [k, v] of Object.entries(m ?? {})) if (v) out[k] = v;
  return out;
}

// lookOf is the look on screen: the colours in force and the fonts in config.ui.
export function lookOf(colors: { light?: Entries; dark?: Entries }, ui: { textFont?: string; codeFont?: string }): Look {
  return { light: colorMap(colors.light), dark: colorMap(colors.dark), textFont: ui.textFont ?? "", codeFont: ui.codeFont ?? "" };
}

function sameMap(a: ColorMap, b: ColorMap): boolean {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((k) => a[k] === b[k]);
}

export function sameLook(a: Look, b: Look): boolean {
  return a.textFont === b.textFont && a.codeFont === b.codeFont && sameMap(a.light, b.light) && sameMap(a.dark, b.dark);
}

// selectedStyle is the style ui.style names, or null for the app's own look,
// also when its file is gone.
export function selectedStyle<T extends { id: string }>(styles: T[] | null | undefined, id: string | undefined): T | null {
  return (id && styles?.find((s) => s.id === id)) || null;
}

export function styleColors(s: StyleValues | null): { light: ColorMap; dark: ColorMap } {
  return { light: colorMap(s?.light), dark: colorMap(s?.dark) };
}

export function styleFonts(s: StyleValues | null): FontChoice {
  return { textFont: s?.textFont ?? "", codeFont: s?.codeFont ?? "" };
}

// isChanged says whether the look on screen differs from the style chosen, or,
// with none chosen, from the app's own look.
export function isChanged(look: Look, style: StyleValues | null): boolean {
  return !sameLook(look, lookOf(styleColors(style), styleFonts(style)));
}

// lookHash is a fingerprint of a look, the same whatever order its colours come in: up
// to 16 hexadecimal digits (config's styleFingerprint). A style chosen records it, so
// the window can tell later whether the look on screen is still that style as applied.
export function lookHash(look: Look): string {
  const sorted = (m: ColorMap) => Object.entries(m).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
  const text = JSON.stringify([sorted(look.light), sorted(look.dark), look.textFont, look.codeFont]);
  // cyrb53: two 32-bit lanes, 53 bits in all; this tells looks apart, it guards nothing.
  let h1 = 0xdeadbeef;
  let h2 = 0x41c6ce57;
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    h1 = Math.imul(h1 ^ c, 2654435761);
    h2 = Math.imul(h2 ^ c, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(16);
}

// within says whether every colour of a is b's too.
const within = (a: ColorMap, b: ColorMap) => Object.entries(a).every(([k, v]) => b[k] === v);

// followsUpdate says whether a built-in style, as this version of the app ships it,
// replaces the look on screen without asking: the look differs from it, and is still
// the style as it was last applied, by the fingerprint recorded then, or, with none
// recorded, because every colour and font on screen is the style's and it only gained
// more. A colour or font the user changed keeps the look as it is, marked changed.
export function followsUpdate(look: Look, style: StyleValues | null, applied: string | undefined): boolean {
  if (!style?.builtin) return false;
  const now = lookOf(styleColors(style), styleFonts(style));
  if (sameLook(look, now)) return false;
  if (applied) return lookHash(look) === applied;
  return look.textFont === now.textFont && look.codeFont === now.codeFont && within(look.light, now.light) && within(look.dark, now.dark);
}
