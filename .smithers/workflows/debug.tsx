// smithers-source: seeded
// smithers-display-name: Debug
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import { ValidationLoop, implementOutputSchema, validateOutputSchema } from "../components/ValidationLoop";
import { reviewOutputSchema } from "../components/Review";

const { Workflow, smithers } = createSmithers({
  implement: implementOutputSchema,
  validate: validateOutputSchema,
  review: reviewOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="debug">
    <ValidationLoop
      idPrefix="debug"
      prompt={ctx.input.prompt ?? "Reproduce and fix the reported bug."}
      implementAgents={roleChains.implement}
      validateAgents={roleChains.validate}
      reviewAgents={roleChains.review}
    />
  </Workflow>
));
