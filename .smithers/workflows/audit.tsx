// smithers-source: seeded
// smithers-display-name: Audit
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import { ForEachFeature, forEachFeatureMergeSchema, forEachFeatureResultSchema } from "../components/ForEachFeature";

const { Workflow, smithers } = createSmithers({
  auditFeature: forEachFeatureResultSchema,
  audit: forEachFeatureMergeSchema,
});

export default smithers((ctx) => (
  <Workflow name="audit">
    <ForEachFeature
      idPrefix="audit"
      agent={pickAgent("review")}
      features={ctx.input.features ?? {}}
      prompt={[
        `Audit for: ${ctx.input.focus ?? "code review"}.`,
        "Evaluate the provided feature scope for gaps in testing, observability, error handling, operational safety, and maintainability.",
        "Use the repository as the source of truth and report concrete findings with actionable next steps.",
        ctx.input.additionalContext ? `Additional context:\n${ctx.input.additionalContext}` : null,
      ].filter(Boolean).join("\n\n")}
      maxConcurrency={ctx.input.maxConcurrency ?? 5}
      mergeAgent={pickAgent("plan")}
    />
  </Workflow>
));
