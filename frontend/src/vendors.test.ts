import { test } from "node:test";
import assert from "node:assert/strict";
import { agentVendor, providerVendor } from "./vendors.ts";

test("a provider's vendor is its type's, or its preset's for an OpenAI-compatible one", () => {
  assert.equal(providerVendor({ type: "anthropic" }), "anthropic");
  assert.equal(providerVendor({ type: "openai_responses" }), "openai");
  assert.equal(providerVendor({ type: "gemini" }), "gemini");
  assert.equal(providerVendor({ type: "ollama" }), "ollama");
  assert.equal(providerVendor({ type: "openai_compat", preset: "openai" }), "openai");
  assert.equal(providerVendor({ type: "openai_compat", preset: "ollama-v1" }), "ollama");
  assert.equal(providerVendor({ type: "openai_compat", preset: "lmstudio" }), "lmstudio");
  // A preset name means nothing on another type, and what is unknown has none.
  assert.equal(providerVendor({ type: "anthropic", preset: "openrouter" }), "anthropic");
  for (const p of [{ type: "openai_compat", preset: "custom" }, { type: "openai_compat" }, { type: "scripted" }, {}, { type: "constructor" }, { type: "openai_compat", preset: "__proto__" }]) {
    assert.equal(providerVendor(p), null, JSON.stringify(p));
  }
});

test("an agent's vendor is its preset's", () => {
  assert.equal(agentVendor("claude-code"), "claude");
  assert.equal(agentVendor("codex"), "openai");
  assert.equal(agentVendor("gemini-cli"), "gemini");
  for (const id of ["goose", "opencode", "copilot"]) assert.equal(agentVendor(id), id);
  for (const id of ["mine", "builtin", "", "toString"]) assert.equal(agentVendor(id), null, id);
});
