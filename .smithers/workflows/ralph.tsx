// smithers-source: seeded
// smithers-display-name: Ralph
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";

const ralphOutputSchema = z.object({
  summary: z.string(),
}).passthrough();

const { Workflow, Task, Loop, smithers } = createSmithers({
  ralph: ralphOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="ralph">
    <Loop until={false} maxIterations={Infinity}>
      <Task id="ralph" output={ralphOutputSchema} agent={roleChains.implement}>
        {ctx.input.prompt ?? "Continue working on the current task."}
      </Task>
    </Loop>
  </Workflow>
));
