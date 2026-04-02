// smithers-source: seeded
// smithers-display-name: Plan
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import PlanPrompt from "../prompts/plan.mdx";

const planOutputSchema = z.object({
  summary: z.string(),
  steps: z.array(z.string()).default([]),
}).passthrough();

const { Workflow, Task, smithers } = createSmithers({
  plan: planOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="plan">
    <Task id="plan" output={planOutputSchema} agent={pickAgent("plan")}>
      <PlanPrompt prompt={ctx.input.prompt ?? "Create an implementation plan."} />
    </Task>
  </Workflow>
));
