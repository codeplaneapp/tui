// smithers-source: seeded
// smithers-display-name: Review
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import { Review, reviewOutputSchema } from "../components/Review";

const { Workflow, smithers } = createSmithers({
  review: reviewOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="review">
    <Review
      idPrefix="review"
      prompt={ctx.input.prompt ?? "Review the current repository changes."}
      agents={roleChains.review}
    />
  </Workflow>
));
