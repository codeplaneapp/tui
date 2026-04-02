// smithers-source: seeded
// smithers-display-name: Tickets Create
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";

const ticketsCreateOutputSchema = z.object({
  summary: z.string(),
  tickets: z.array(z.object({
    title: z.string(),
    description: z.string(),
    acceptanceCriteria: z.array(z.string()).default([]),
  })).default([]),
}).passthrough();

const { Workflow, Task, smithers } = createSmithers({
  tickets: ticketsCreateOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="tickets-create">
    <Task id="tickets" output={ticketsCreateOutputSchema} agent={pickAgent("plan")}>
      {`Break the following request into well-defined tickets with titles, descriptions, and acceptance criteria.\n\nRequest: ${ctx.input.prompt ?? "Create tickets for the requested work."}`}
    </Task>
  </Workflow>
));
