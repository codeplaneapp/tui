// smithers-source: seeded
// Inspired by Matt Pocock's grill-me skill (https://github.com/mattpocock/skills)
/** @jsxImportSource smithers-orchestrator */
import { Loop, Task, type AgentLike } from "smithers-orchestrator";
import { z } from "zod";
import GrillMePrompt from "../prompts/grill-me.mdx";

export const grillMeOutputSchema = z.object({
  question: z.string(),
  recommendedAnswer: z.string().nullable().default(null),
  branch: z.string().nullable().default(null),
  resolved: z.boolean().default(false),
  questionsAsked: z.number().int().default(0),
  sharedUnderstanding: z.string().nullable().default(null),
}).passthrough();

type GrillMeProps = {
  idPrefix: string;
  context: string;
  agent: AgentLike | AgentLike[];
  minQuestions?: number;
  maxQuestions?: number;
  stopCondition?: string;
  recommendAnswers?: boolean;
  exploreCodebase?: boolean;
};

export function GrillMe({
  idPrefix,
  context,
  agent,
  minQuestions = 5,
  maxQuestions = 30,
  stopCondition,
  recommendAnswers = true,
  exploreCodebase = true,
}: GrillMeProps) {
  const instructions = [
    "Ask questions one at a time.",
    recommendAnswers && "For each question, provide your recommended answer.",
    exploreCodebase && "If a question can be answered by exploring the codebase, explore the codebase instead.",
    stopCondition,
    minQuestions > 0 && `Ask at least ${minQuestions} questions before concluding.`,
  ].filter(Boolean).join("\n");

  return (
    <Loop until={false} maxIterations={maxQuestions}>
      <Task
        id={`${idPrefix}:grill`}
        output={grillMeOutputSchema}
        agent={agent}
      >
        <GrillMePrompt context={context} instructions={instructions} />
      </Task>
    </Loop>
  );
}
