// smithers-source: seeded
/** @jsxImportSource smithers-orchestrator */
import { Sequence, Task, type AgentLike } from "smithers-orchestrator";
import { z } from "zod";
import ImplementPrompt from "../prompts/implement.mdx";
import ValidatePrompt from "../prompts/validate.mdx";
import { Review } from "./Review";

export const implementOutputSchema = z.object({
  summary: z.string(),
  prompt: z.string().nullable().default(null),
  filesChanged: z.array(z.string()).default([]),
  allTestsPassing: z.boolean().default(true),
}).passthrough();

export const validateOutputSchema = z.object({
  summary: z.string(),
  allPassed: z.boolean().default(true),
  failingSummary: z.string().nullable().default(null),
}).passthrough();

type ValidationLoopProps = {
  idPrefix: string;
  prompt: unknown;
  implementAgents: AgentLike[];
  reviewAgents: AgentLike[];
  validateAgents?: AgentLike[];
};

export function ValidationLoop({
  idPrefix,
  prompt,
  implementAgents,
  reviewAgents,
  validateAgents,
}: ValidationLoopProps) {
  const validationChain = validateAgents && validateAgents.length > 0
    ? validateAgents
    : implementAgents;
  const promptText = typeof prompt === "string" ? prompt : JSON.stringify(prompt ?? null);

  return (
    <Sequence>
      <Task id={`${idPrefix}:implement`} output={implementOutputSchema} agent={implementAgents}>
        <ImplementPrompt prompt={promptText} />
      </Task>
      <Task id={`${idPrefix}:validate`} output={validateOutputSchema} agent={validationChain}>
        <ValidatePrompt prompt={promptText} />
      </Task>
      <Review idPrefix={`${idPrefix}:review`} prompt={promptText} agents={reviewAgents} />
    </Sequence>
  );
}
