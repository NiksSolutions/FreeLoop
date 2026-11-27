import { test } from "node:test";
import assert from "node:assert/strict";
import { followsUpdate, groupStyles, isChanged, lookHash, lookOf, sameLook, selectedStyle, styleColors, styleFonts } from "./styles.ts";

const paper = { id: "paper", name: "Paper", textFont: "Inter", codeFont: "", light: { accent: "#7c3aed", bg: "#ffffff" }, dark: null };
const night = { id: "night", name: "Night", textFont: "", codeFont: "Menlo", light: {}, dark: { bg: "#000000" } };

test("a look is the colours of both appearances and the two fonts", () => {
  const look = lookOf({ light: { accent: "#7c3aed" } }, { textFont: "Inter", codeFont: undefined, fontSize: 16 });
  assert.deepEqual(look, { light: { accent: "#7c3aed" }, dark: {}, textFont: "Inter", codeFont: "" });
});

test("looks compare by value, whatever the order and however empty is written", () => {
  const a = lookOf({ light: { bg: "#ffffff", accent: "#7c3aed" }, dark: null }, { textFont: "Inter" });
  const b = lookOf({ light: { accent: "#7c3aed", bg: "#ffffff" }, dark: {} }, { textFont: "Inter", codeFont: "" });
  assert.ok(sameLook(a, b));
  assert.ok(!sameLook(a, { ...b, codeFont: "Menlo" }));
  assert.ok(!sameLook(a, { ...b, light: { accent: "#7c3aed" } }));
  assert.ok(!sameLook(a, { ...b, light: { accent: "#7c3aed", bg: "#fffffe" } }));
  // An entry the bindings type as possibly missing counts as not set.
  assert.ok(sameLook(lookOf({ light: { accent: undefined as unknown as string } }, {}), lookOf({}, {})));
});

test("the style chosen is found by its ID, and a missing one is the app's own look", () => {
  assert.equal(selectedStyle([paper, night], "night"), night);
  assert.equal(selectedStyle([paper, night], ""), null);
  assert.equal(selectedStyle([paper, night], undefined), null);
  assert.equal(selectedStyle([paper, night], "gone"), null);
  assert.equal(selectedStyle(null, "paper"), null);
});

test("a style is changed when the look on screen is not what it holds", () => {
  const onScreen = lookOf({ light: { bg: "#ffffff", accent: "#7c3aed" } }, { textFont: "Inter" });
  assert.ok(!isChanged(onScreen, paper));
  assert.ok(isChanged({ ...onScreen, textFont: "" }, paper));
  assert.ok(isChanged(onScreen, night));
  // The app's own look is changed by any colour or font of the user's.
  assert.ok(!isChanged(lookOf({}, {}), null));
  assert.ok(isChanged(lookOf({ dark: { bg: "#000000" } }, {}), null));
  assert.ok(isChanged(lookOf({}, { codeFont: "Menlo" }), null));
});

test("applying a style puts all of its values in place, and the app's own clears them", () => {
  assert.deepEqual(styleColors(paper), { light: { accent: "#7c3aed", bg: "#ffffff" }, dark: {} });
  assert.deepEqual(styleFonts(paper), { textFont: "Inter", codeFont: "" });
  assert.deepEqual(styleColors(null), { light: {}, dark: {} });
  assert.deepEqual(styleFonts(null), { textFont: "", codeFont: "" });
  // What is applied is what the style holds, so it is not shown as changed.
  assert.ok(!isChanged(lookOf(styleColors(night), styleFonts(night)), night));
});

test("the list shows the user's styles apart from the app's own, each in its order", () => {
  const nord = { ...night, id: "nord", name: "Nord", builtin: true };
  const niks = { ...night, id: "niks", name: "Niks Theme", builtin: true };
  assert.deepEqual(groupStyles([niks, night, nord, paper]), { saved: [night, paper], builtin: [niks, nord] });
  assert.deepEqual(groupStyles([]), { saved: [], builtin: [] });
});

test("a look's fingerprint is the same in any order, and differs with any colour or font", () => {
  const a = { light: { bg: "#fff", text: "#000" }, dark: { bg: "#000" }, textFont: "", codeFont: "" };
  const b = { light: { text: "#000", bg: "#fff" }, dark: { bg: "#000" }, textFont: "", codeFont: "" };
  assert.equal(lookHash(a), lookHash(b));
  assert.match(lookHash(a), /^[0-9a-f]{1,16}$/);
  assert.notEqual(lookHash(a), lookHash({ ...a, dark: { bg: "#111" } }));
  assert.notEqual(lookHash(a), lookHash({ ...a, codeFont: "Menlo" }));
});

test("a built-in style updated by the app replaces the look only while the user has changed nothing", () => {
  const nord = { id: "nord", name: "Nord", builtin: true, textFont: "", codeFont: "", light: { bg: "#eceff4", accent: "#4c6d97", "series-1": "#4c6d97" }, dark: { bg: "#2e3440", accent: "#88c0d0" } };
  const before = { light: { bg: "#eceff4", accent: "#4c6d97" }, dark: { bg: "#2e3440", accent: "#88c0d0" }, textFont: "", codeFont: "" };
  // With the fingerprint of what was applied then.
  assert.equal(followsUpdate(before, nord, lookHash(before)), true);
  const changed = { ...before, dark: { ...before.dark, accent: "#ff0000" } };
  assert.equal(followsUpdate(changed, nord, lookHash(before)), false);
  // With none recorded: only a look that is the style's, short of what it gained.
  assert.equal(followsUpdate(before, nord, undefined), true);
  assert.equal(followsUpdate(changed, nord, undefined), false);
  assert.equal(followsUpdate({ ...before, textFont: "Inter" }, nord, undefined), false);
  // Nothing to do when the look is the style already, nor for a style of the user's.
  const now = lookOf(styleColors(nord), styleFonts(nord));
  assert.equal(followsUpdate(now, nord, lookHash(before)), false);
  assert.equal(followsUpdate(before, { ...nord, builtin: false }, lookHash(before)), false);
  assert.equal(followsUpdate(before, null, lookHash(before)), false);
});
