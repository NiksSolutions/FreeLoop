// What a provider or an agent is, for its logo (GUI-034): read from its type and preset,
// or from the agent's preset, never from the ID or the name the user gave it. A custom
// endpoint, the demo and an agent of the user's own have none and keep their letters.

export type Vendor = "anthropic" | "claude" | "openai" | "gemini" | "ollama" | "openrouter" | "lmstudio" | "llamacpp" | "vllm" | "goose" | "opencode" | "copilot";

// Provider types (internal/provider), then the OpenAI-compatible presets by ID.
const TYPES = new Map<string, Vendor>([
  ["anthropic", "anthropic"],
  ["openai_responses", "openai"],
  ["gemini", "gemini"],
  ["ollama", "ollama"],
]);
const PRESETS = new Map<string, Vendor>([
  ["openai", "openai"],
  ["openrouter", "openrouter"],
  ["lmstudio", "lmstudio"],
  ["llamacpp", "llamacpp"],
  ["vllm", "vllm"],
  ["ollama-v1", "ollama"],
]);
// Agent presets (internal/config/presets.go) by ID.
const AGENTS = new Map<string, Vendor>([
  ["claude-code", "claude"],
  ["codex", "openai"],
  ["gemini-cli", "gemini"],
  ["goose", "goose"],
  ["opencode", "opencode"],
  ["copilot", "copilot"],
]);

export function providerVendor(p: { type?: string; preset?: string }): Vendor | null {
  if (p.type === "openai_compat") return PRESETS.get(p.preset ?? "") ?? null;
  return TYPES.get(p.type ?? "") ?? null;
}

export function agentVendor(id: string): Vendor | null {
  return AGENTS.get(id) ?? null;
}
