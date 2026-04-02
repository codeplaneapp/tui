// smithers-source: seeded
// smithers-display-name: Test First
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
  <Workflow name="test-first">
    <ValidationLoop
      idPrefix="test-first"
      prompt={ctx.input.prompt ?? "Write or update tests before implementation."}
      implementAgents={roleChains.implement}
      validateAgents={roleChains.validate}
      reviewAgents={roleChains.review}
    />
  </Workflow>
));
