// smithers-source: seeded
// smithers-display-name: Improve Test Coverage
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
  <Workflow name="improve-test-coverage">
    <ValidationLoop
      idPrefix="improve-test-coverage"
      prompt={ctx.input.prompt ?? "Improve the test coverage for the current repository."}
      implementAgents={roleChains.implement}
      validateAgents={roleChains.validate}
      reviewAgents={roleChains.review}
    />
  </Workflow>
));
