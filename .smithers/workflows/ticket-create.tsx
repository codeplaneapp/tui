// smithers-source: seeded
// smithers-display-name: Ticket Create
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import TicketPrompt from "../prompts/ticket.mdx";

const ticketCreateOutputSchema = z.object({
  title: z.string(),
  description: z.string(),
  acceptanceCriteria: z.array(z.string()).default([]),
}).passthrough();

const { Workflow, Task, smithers } = createSmithers({
  ticket: ticketCreateOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="ticket-create">
    <Task id="ticket" output={ticketCreateOutputSchema} agent={pickAgent("plan")}>
      <TicketPrompt prompt={ctx.input.prompt ?? "Create a ticket for the requested work."} />
    </Task>
  </Workflow>
));
