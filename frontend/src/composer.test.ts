import assert from "node:assert/strict";
import { test } from "node:test";
import { placeholder, tabAction } from "./composer.ts";
import { setLanguage } from "./i18n.ts";

test("a pane the user is not in gets the short placeholder", () => {
  assert.equal(placeholder(false, false), "Ask for a change…");
  assert.equal(placeholder(true, false), "Queue a message");
  assert.ok(placeholder(false, true).includes("Type @"));
});

// GUI-048: the placeholder follows the language, and still names @ and /.
test("the placeholder is in the language set", () => {
  setLanguage("de");
  assert.equal(placeholder(false, true), "Um eine Änderung, eine Korrektur, eine Erklärung bitten… @ hängt eine Datei an, / zeigt die Befehle.");
  assert.equal(placeholder(false, false), "Um eine Änderung bitten…");
  assert.equal(placeholder(true, false), "Nachricht einreihen");
  setLanguage("nl");
  assert.equal(placeholder(true, true), "Bericht in de wachtrij zetten voor het einde van deze beurt");
  setLanguage("en");
  assert.equal(placeholder(false, true), "Ask for a change, a fix, an explanation… Type @ to attach a file, / for commands.");
  assert.equal(placeholder(true, true), "Queue a message for when this turn ends");
});

// The user's report: Tab, pressed to take a suggestion, moved the focus to the model
// and effort picker. It takes whatever is offered, and moves on only when nothing is.
test("Tab takes what the box offers, and moves on only when it offers nothing", () => {
  const none = { shift: false, history: null, commands: 0, mention: null, rest: "" };
  assert.equal(tabAction({ ...none, rest: " read the notes" }), "suggestion");
  assert.equal(tabAction({ ...none, history: 2 }), "history");
  assert.equal(tabAction({ ...none, commands: 3 }), "command");
  assert.equal(tabAction({ ...none, mention: 4 }), "mention");
  // The file list is still on its way: Tab waits for it rather than leaving the box.
  assert.equal(tabAction({ ...none, mention: "loading" }), "wait");
  // An open list wins over the grey text, as it does on screen.
  assert.equal(tabAction({ ...none, commands: 1, rest: "x" }), "command");
  // Nothing offered: the focus moves on, so the box never holds the keyboard.
  assert.equal(tabAction(none), "move");
  assert.equal(tabAction({ ...none, history: 0 }), "move");
  assert.equal(tabAction({ ...none, mention: 0 }), "move");
  // Shift+Tab always goes back.
  assert.equal(tabAction({ ...none, shift: true, rest: "x", mention: 2 }), "move");
});
