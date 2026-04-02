// smithers-source: seeded
// smithers-display-name: Research
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import ResearchPrompt from "../prompts/research.mdx";

const researchOutputSchema = z.object({
  summary: z.string(),
  keyFindings: z.array(z.string()).default([]),
}).passthrough();

const { Workflow, Task, smithers } = createSmithers({
  research: researchOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="research">
    <Task id="research" output={researchOutputSchema} agent={pickAgent("research")}>
      <ResearchPrompt prompt={ctx.input.prompt ?? "Research the given topic."} />
    </Task>
  </Workflow>
));
