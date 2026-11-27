import { test } from "node:test";
import assert from "node:assert/strict";
import { APPROVAL_GRACE_MS, ApprovalTarget, approvalClock, approvalHint, approvalKey, keyTime, approvalShortcut, formLabel, optionLabel, bindingOf, bindingProblem, comboOf, isTextField, keyLabel, optionKind, pressed, setBindings } from "./shortcuts.ts";
import { setPlatform } from "./platform.ts";
import { setLanguage } from "./i18n.ts";

// These are the Mac's shortcuts; platform.test.ts has the others.
setPlatform("MacIntel");

const k = (key: string, mods: Partial<{ metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }> = {}) => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

test("letters answer only outside text fields", () => {
  assert.equal(approvalShortcut(k("y"), false), "allow_once");
  assert.equal(approvalShortcut(k("a"), false), "allow_always");
  assert.equal(approvalShortcut(k("n"), false), "reject");
  assert.equal(approvalShortcut(k("y"), true), null);
  assert.equal(approvalShortcut(k("Y", { shiftKey: true }), false), null);
  assert.equal(approvalShortcut(k("x"), false), null);
});

test("command forms work in text fields", () => {
  assert.equal(approvalShortcut(k("Enter", { metaKey: true }), true), "allow_once");
  assert.equal(approvalShortcut(k("Enter", { metaKey: true, shiftKey: true }), true), "allow_always");
  assert.equal(approvalShortcut(k("Backspace", { metaKey: true, shiftKey: true }), true), "reject");
  assert.equal(approvalShortcut(k("Backspace", { metaKey: true }), true), null);
  assert.equal(approvalShortcut(k("Enter", { metaKey: true, ctrlKey: true }), false), null);
  assert.equal(approvalShortcut(k("y", { metaKey: true }), false), null);
});

// GUI-006, GUI-017, GUI-002: the guard every approval key passes, in the conversation
// and in Attention alike.
const event = (key: string, more: Partial<{ metaKey: boolean; shiftKey: boolean; repeat: boolean; defaultPrevented: boolean; isComposing: boolean }> = {}) => ({
  ...k(key, more),
  repeat: false,
  defaultPrevented: false,
  isComposing: false,
  ...more,
});
const at = (more: Partial<{ inField: boolean; covered: boolean; since: number; now: number }> = {}) => ({ inField: false, covered: false, since: 0, now: 10_000, ...more });

test("an approval key names its answer, and is early within the grace period", () => {
  assert.deepEqual(approvalKey(event("y"), at()), { kind: "allow_once", early: false });
  assert.deepEqual(approvalKey(event("Enter", { metaKey: true }), at({ inField: true })), { kind: "allow_once", early: false });
  assert.deepEqual(approvalKey(event("n"), at({ since: 10_000 - APPROVAL_GRACE_MS + 1 })), { kind: "reject", early: true });
  assert.deepEqual(approvalKey(event("n"), at({ since: 10_000 - APPROVAL_GRACE_MS })), { kind: "reject", early: false });
  assert.equal(approvalKey(event("x"), at()), null);
  assert.equal(approvalKey(event("y"), at({ inField: true })), null);
});

test("a held key, a key taken or composing, and any key under a dialog answer nothing", () => {
  assert.equal(approvalKey(event("y", { repeat: true }), at()), null);
  assert.equal(approvalKey(event("y", { defaultPrevented: true }), at()), null);
  assert.equal(approvalKey(event("y", { isComposing: true }), at()), null);
  assert.equal(approvalKey(event("y"), at({ covered: true })), null);
  assert.equal(approvalKey(event("Enter", { metaKey: true }), at({ covered: true, inField: true })), null);
});

test("the grace period runs from when one becomes what the keys answer, not from each redraw", () => {
  const target = new ApprovalTarget<string>();
  target.set("a", 1000);
  // A note typed on the card redraws it: its time stands.
  target.set("a", 1300);
  assert.equal(target.since("a", 1350), 1000);
  // Answering it makes the next one the target, from then.
  target.set("b", 2000);
  assert.equal(target.since("b", 2100), 2000);
  // One the keys did not know they answer starts now.
  assert.equal(target.since("c", 2500), 2500);
  target.set(null, 3000);
  assert.equal(target.since("c", 3100), 3100);
});

test("option kinds", () => {
  assert.equal(optionKind("allow_once"), "allow_once");
  assert.equal(optionKind("reject_once"), "reject");
  assert.equal(optionKind("reject_always"), "reject");
  assert.equal(optionKind("other"), null);
});

const press = (key: string, more = {}) => ({ key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...more });

test("a key press is written the way bindings are", () => {
  assert.equal(comboOf(press("k", { metaKey: true })), "Meta+K");
  assert.equal(comboOf(press("K", { metaKey: true, shiftKey: true })), "Meta+Shift+K");
  assert.equal(comboOf(press("Meta", { metaKey: true })), "");
  assert.equal(keyLabel("Meta+Shift+K"), "⌘⇧K");
  assert.equal(keyLabel("Meta+Shift++"), "⌘⇧+");
  assert.equal(keyLabel("y"), "Y");
});

test("a shortcut can be changed, and goes back when the change is gone", () => {
  assert.ok(pressed(press("k", { metaKey: true }), "palette"));
  setBindings({ palette: "Meta+P" });
  assert.ok(pressed(press("p", { metaKey: true }), "palette"));
  assert.ok(!pressed(press("k", { metaKey: true }), "palette"));
  setBindings({});
  assert.ok(pressed(press("k", { metaKey: true }), "palette"));
});

test("approval letters can be changed", () => {
  setBindings({ allow_once: "j" });
  assert.equal(approvalShortcut(press("j"), false), "allow_once");
  assert.equal(approvalShortcut(press("y"), false), null);
  assert.equal(approvalShortcut(press("j"), true), null);
  assert.equal(approvalHint("allow_once"), "J or ⌘↩");
  setBindings({});
});

test("what cannot be a shortcut is refused and says why", () => {
  assert.match(bindingProblem("palette", "Meta+C", {}), /kept for macOS/);
  assert.match(bindingProblem("palette", "Meta+1", {}), /kept for macOS/);
  assert.match(bindingProblem("palette", "K", {}), /Use ⌘/);
  assert.match(bindingProblem("palette", "Meta+,", {}), /already/);
  assert.match(bindingProblem("allow_once", "N", {}), /already/);
  assert.match(bindingProblem("allow_once", "Meta+K", {}), /one letter/);
  assert.equal(bindingProblem("palette", "Meta+P", {}), "");
  assert.equal(bindingProblem("allow_once", "J", {}), "");
  assert.equal(bindingProblem("palette", "Meta+,", { settings: "Meta+S" }), "");
});

test("a binding edited by hand into something unusable is ignored", () => {
  setBindings({ palette: "Meta+C", nonsense: "Meta+J" });
  assert.equal(bindingOf("palette"), "Meta+K");
  setBindings({});
});

test("an approval button says what its kind does, whatever the agent named it", () => {
  assert.equal(optionLabel("allow_always", "Deny"), "Allow always");
  assert.equal(optionLabel("reject_once", "Allow"), "Deny");
  assert.equal(optionLabel("other", "Something"), "Something");
});

// GUI-048: the buttons and the hint beside them are the app's words; a name an agent
// gave an option of a kind the app does not know is shown as given.
test("approval buttons and their hint follow the language", () => {
  setLanguage("de");
  assert.equal(optionLabel("allow_once", "Allow"), "Einmal erlauben");
  assert.equal(optionLabel("allow_always", "Deny"), "Immer erlauben");
  assert.equal(optionLabel("reject_always", "x"), "Immer ablehnen");
  assert.equal(optionLabel("other", "Something"), "Something");
  assert.equal(formLabel("allow"), "Senden");
  assert.equal(formLabel("reject"), "Überspringen");
  assert.equal(approvalHint("allow_once"), "Y oder ⌘↩");
  setLanguage("nl");
  assert.equal(optionLabel("reject_once", "x"), "Weigeren");
  assert.equal(formLabel("reject"), "Overslaan");
  assert.equal(approvalHint("reject"), "N of ⌘⇧⌫");
  setLanguage("en");
  assert.equal(formLabel("allow"), "Send");
  assert.equal(approvalHint("allow_once"), "Y or ⌘↩");
});

test("the pane shortcuts start as ⌃⇧ and the arrows, and can be changed", () => {
  setBindings({});
  assert.equal(pressed(k("ArrowLeft", { ctrlKey: true, shiftKey: true }), "paneLeft"), true);
  assert.equal(pressed(k("ArrowDown", { ctrlKey: true, shiftKey: true }), "paneDown"), true);
  assert.equal(pressed(k("ArrowLeft", { ctrlKey: true }), "paneLeft"), false);
  assert.equal(keyLabel(bindingOf("paneRight")), "⌃⇧→");
  setBindings({ paneLeft: "Ctrl+H" });
  assert.equal(pressed(k("h", { ctrlKey: true }), "paneLeft"), true);
  assert.equal(pressed(k("ArrowLeft", { ctrlKey: true, shiftKey: true }), "paneLeft"), false);
  // A key another one uses is refused.
  assert.match(bindingProblem("paneUp", "Ctrl+Shift+ArrowDown", {}), /already/);
  setBindings({});
});

test("why a key cannot be a shortcut is said in the language in force (GUI-048)", () => {
  setLanguage("de");
  assert.equal(bindingProblem("palette", "Meta+,", {}), "⌘, ist schon „Einstellungen öffnen“.");
  assert.equal(bindingProblem("palette", "Meta+C", {}), "⌘C ist für macOS oder die App reserviert.");
  assert.match(bindingProblem("palette", "K", {}), /^Mit ⌘ oder ⌃ kombinieren/);
  setLanguage("nl");
  assert.equal(bindingProblem("allow_once", "Meta+K", {}), "Een goedkeuringssneltoets is één letter, zonder speciale toets.");
  setLanguage("en");
});

// ⌘+ is ⌘= with Shift on a US keyboard, its own key on a German one and ⌘⇧1 on a Swiss
// one, where = is ⌘⇧0; - with Shift is _. Each of them changes the text size.
test("the text size keys work on Swiss, German and US keyboards", () => {
  setBindings({});
  for (const [key, shiftKey] of [["=", false], ["+", true], ["+", false], ["=", true]] as const) {
    assert.ok(pressed(press(key, { metaKey: true, shiftKey }), "textLarger"), `⌘${shiftKey ? "⇧" : ""}${key}`);
  }
  for (const [key, shiftKey] of [["-", false], ["-", true], ["_", true]] as const) {
    assert.ok(pressed(press(key, { metaKey: true, shiftKey }), "textSmaller"), `⌘${shiftKey ? "⇧" : ""}${key}`);
  }
  assert.ok(!pressed(press("0", { metaKey: true }), "textLarger"));
  assert.equal(keyLabel(bindingOf("textLarger")), "⌘+");
  assert.equal(keyLabel(bindingOf("textSmaller")), "⌘-");
  // Set to another key, that key alone changes it, and the spellings are free again.
  setBindings({ textLarger: "Meta+L" });
  assert.ok(!pressed(press("+", { metaKey: true, shiftKey: true }), "textLarger"));
  assert.equal(bindingProblem("palette", "Meta+Shift++", { textLarger: "Meta+L" }), "");
  setBindings({});
  assert.match(bindingProblem("palette", "Meta+Shift++", {}), /Larger text/);
  assert.match(bindingProblem("palette", "Meta+Shift+_", {}), /Smaller text/);
});

test("a key is judged by when it was pressed, not when it was handled", () => {
  // Pressed 100 ms after the request became current, handled 1.8 s after: still early.
  const y = { key: "y", metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, repeat: false, defaultPrevented: false, isComposing: false };
  assert.equal(approvalKey(y, { inField: false, covered: false, since: 1000, now: keyTime({ timeStamp: 1100 }) })?.early, true);
  assert.equal(approvalKey(y, { inField: false, covered: false, since: 1000, now: keyTime({ timeStamp: 1500 }) })?.early, false);
  // An event with no time of its own is judged now.
  const before = approvalClock();
  assert.ok(keyTime({ timeStamp: 0 }) >= before);
});
