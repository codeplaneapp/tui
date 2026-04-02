// smithers-source: generated
import { ClaudeCodeAgent, CodexAgent, GeminiAgent, PiAgent, KimiAgent, AmpAgent, type AgentLike } from "smithers-orchestrator";

export const providers = {
  claude: new ClaudeCodeAgent({ model: "claude-opus-4-6" }),
  codex: new CodexAgent({ model: "gpt-5.3-codex", skipGitRepoCheck: true }),
  gemini: new GeminiAgent({ model: "gemini-3.1-pro-preview" }),
  pi: new PiAgent({ provider: "openai", model: "gpt-5.3-codex" }),
  kimi: new KimiAgent({ model: "kimi-latest" }),
  amp: new AmpAgent(),
} as const;

export const roleChains = {
  spec: [providers.claude, providers.codex],
  research: [providers.gemini, providers.kimi, providers.codex, providers.claude],
  plan: [providers.gemini, providers.codex, providers.claude, providers.kimi],
  implement: [providers.codex, providers.amp, providers.gemini, providers.claude, providers.kimi],
  validate: [providers.codex, providers.amp, providers.gemini],
  review: [providers.claude, providers.amp, providers.codex],
} as const satisfies Record<string, AgentLike[]>;

export function pickAgent(role: keyof typeof roleChains): AgentLike {
  const agent = roleChains[role][0];
  if (!agent) throw new Error(`No agent configured for role: ${role}`);
  return agent;
}
