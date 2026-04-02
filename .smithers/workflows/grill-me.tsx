// smithers-source: seeded
// smithers-display-name: Grill Me
/** @jsxImportSource smithers-orchestrator */
// Inspired by Matt Pocock's grill-me skill (https://github.com/mattpocock/skills)
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import { GrillMe, grillMeOutputSchema } from "../components/GrillMe";

const { Workflow, smithers } = createSmithers({
  grill: grillMeOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="grill-me">
    <GrillMe
      idPrefix="grill-me"
      context={ctx.input.prompt ?? "Describe the plan or design you want to stress-test."}
      agent={pickAgent("plan")}
    />
  </Workflow>
));
