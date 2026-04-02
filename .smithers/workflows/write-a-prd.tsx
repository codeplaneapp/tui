// smithers-source: seeded
// smithers-display-name: Write a PRD
/** @jsxImportSource smithers-orchestrator */
// Inspired by Matt Pocock's write-a-prd skill (https://github.com/mattpocock/skills)
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import { WriteAPrd, prdOutputSchema } from "../components/WriteAPrd";

const { Workflow, smithers } = createSmithers({
  prd: prdOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="write-a-prd">
    <WriteAPrd
      idPrefix="write-a-prd"
      context={ctx.input.prompt ?? "Describe the feature or product you want to specify."}
      agent={pickAgent("plan")}
    />
  </Workflow>
));
