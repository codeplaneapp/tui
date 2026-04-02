// smithers-source: seeded
// smithers-display-name: Feature Enum
/** @jsxImportSource smithers-orchestrator */
import { createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import { pickAgent, roleChains } from "../agents";
import { FeatureEnum, featureEnumOutputSchema } from "../components/FeatureEnum";

const { Workflow, smithers } = createSmithers({
  featureEnum: featureEnumOutputSchema,
});

export default smithers((ctx) => (
  <Workflow name="feature-enum">
    <FeatureEnum
      idPrefix="feature-enum"
      agent={pickAgent("research")}
      refineIterations={ctx.input.refineIterations}
      existingFeatures={ctx.input.existingFeatures ?? null}
      lastCommitHash={ctx.input.lastCommitHash ?? null}
      additionalContext={ctx.input.additionalContext ?? ""}
    />
  </Workflow>
));
