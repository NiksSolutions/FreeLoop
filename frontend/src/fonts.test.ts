import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { applyFonts, CODE_FONTS, fontPresent, fontStack, isFontName, isWeight, TEXT_FONTS, WEIGHTS } from "./fonts.ts";

test("only family names pass", () => {
  for (const v of ["Menlo", "SF Mono", "JetBrains Mono NL", "Source Sans 3", "M+ 1m", "Noto_Sans.Mono-Bold", "ヒラギノ角ゴシック", "3270"]) {
    assert.equal(isFontName(v), true, v);
  }
  for (const v of [
    "",
    " Menlo",
    "Menlo ",
    "-Menlo",
    'Menlo"',
    "Menlo'",
    "Menlo;",
    "Menlo, serif",
    "a{b}",
    "a\\b",
    "a/b",
    "a:b",
    'x"; } body { background: url(https://example.com/x) } a { b: "',
    "url(https://example.com/x)",
    "var(--text)",
    "Menlo\nserif",
    "Menlo\0",
    "a".repeat(65),
  ]) {
    assert.equal(isFontName(v), false, v);
  }
});

test("the length counts characters, as the engine does", () => {
  assert.equal(isFontName("角".repeat(64)), true);
  assert.equal(isFontName("角".repeat(65)), false);
});

test("the choices offered are all valid names", () => {
  for (const v of [...TEXT_FONTS, ...CODE_FONTS]) assert.equal(isFontName(v), true, v);
});

test("a name comes with the app's own font behind it, so a missing one falls back", () => {
  assert.equal(fontStack("Fira Code", "mono-default"), '"Fira Code", var(--mono-default)');
  assert.equal(fontStack("", "mono-default"), null);
  assert.equal(fontStack(undefined, "mono-default"), null);
  assert.equal(fontStack('x"; } * { background: url(y) }', "mono-default"), null);
});

function fakeStyle() {
  const set = new Map<string, string>();
  return { set, setProperty: (k: string, v: string | null) => void set.set(k, v ?? ""), removeProperty: (k: string) => (set.delete(k), "") };
}

test("fonts go on the page, and clearing one goes back to the app's own", () => {
  const style = fakeStyle();
  applyFonts(style, { textFont: "Avenir Next", codeFont: "Menlo" });
  assert.equal(style.set.get("--font-text"), '"Avenir Next", var(--font-text-default)');
  assert.equal(style.set.get("--mono"), '"Menlo", var(--mono-default)');
  applyFonts(style, { textFont: "", codeFont: "Menlo" });
  assert.equal(style.set.has("--font-text"), false);
  assert.equal(style.set.has("--mono"), true);
});

test("a bad name never reaches the page, and does not keep the font before it", () => {
  const style = fakeStyle();
  applyFonts(style, { textFont: "Avenir Next", codeFont: "Menlo" });
  applyFonts(style, { textFont: "url(https://example.com/x)", codeFont: 'a"; } * { color: red } b { c: "' });
  assert.equal(style.set.size, 0);
});

test("a font is present when text in it measures differently from a fallback", () => {
  const widths: Record<string, number> = { monospace: 100, serif: 90, "sans-serif": 95 };
  // A missing family measures as the fallback named after it.
  const measure = (installed: Record<string, number>) => (font: string) => {
    const families = font.replace(/^72px /, "").split(", ");
    const first = families[0].replaceAll('"', "");
    return installed[first] ?? widths[families[families.length - 1]];
  };
  assert.equal(fontPresent("Nope", measure({})), false);
  assert.equal(fontPresent("Fira Code", measure({ "Fira Code": 120 })), true);
  // The same width as one fallback, as a font that is that fallback has.
  assert.equal(fontPresent("Menlo", measure({ Menlo: 100 })), true);
  // Nothing can be measured, so nothing is held back.
  assert.equal(fontPresent("Fira Code", () => 0), true);
});

test("every font in app.css goes through the variables", () => {
  const css = readFileSync(new URL("./app.css", import.meta.url), "utf8");
  const used = [...css.matchAll(/font-family:\s*([^;]+);/g)].map((m) => m[1].trim());
  assert.ok(used.length > 5);
  for (const v of used) assert.ok(["var(--font-text)", "var(--mono)", "inherit"].includes(v), v);
  for (const m of css.matchAll(/^\s*font:\s*([^;]+);/gm)) assert.equal(m[1].trim(), "inherit");
});

test("weights go on the page as CSS counts them, and only those", () => {
  for (const w of [100, 300, 400, 600, 900]) assert.equal(isWeight(w), true, String(w));
  for (const w of [0, 50, 350, 1000, -100, 400.5, "500", null]) assert.equal(isWeight(w), false, String(w));
  // What the editor offers is the app's own and weights the settings take.
  for (const w of WEIGHTS) assert.ok(w === 0 || isWeight(w), String(w));
  const style = fakeStyle();
  applyFonts(style, { textWeight: 500, codeWeight: 300 });
  assert.equal(style.set.get("--weight-text"), "500");
  assert.equal(style.set.get("--weight-code"), "300");
  // The app's own, and anything else, goes back to app.css's.
  applyFonts(style, { textWeight: 0, codeWeight: 450 });
  assert.equal(style.set.size, 0);
});

test("every weight in app.css follows the text's or the code's", () => {
  // A fixed weight would not move with the user's: a label at 600 under text at 600
  // would no longer stand out.
  const css = readFileSync(new URL("./app.css", import.meta.url), "utf8");
  const used = [...css.matchAll(/font-weight:\s*([^;]+);/g)].map((m) => m[1].trim());
  assert.ok(used.length > 30);
  for (const v of used) assert.ok(/^(var\(--weight-[a-z]+\)|inherit|min\(900, calc\(var\(--weight-text\) \+ \d+\)\))$/.test(v), v);
  // A rule that sets the code font sets the code's weight, or a step of its own.
  for (const m of css.matchAll(/\{[^{}]*font-family:\s*var\(--mono\);[^{}]*\}/g)) assert.match(m[0], /font-weight:/, m[0]);
});
