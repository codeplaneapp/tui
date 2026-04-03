// smithers-source: generated
// smithers-display-name: Specs Pipeline
/** @jsxImportSource smithers-orchestrator */
import { Branch, Loop, MergeQueue, Parallel, Sequence, Task, Worktree, createSmithers } from "smithers-orchestrator";
import { z } from "zod";
import * as fs from "node:fs/promises";
import * as fsSync from "node:fs";
import * as path from "node:path";
import { execSync } from "node:child_process";
import { pickAgent } from "../agents";
import { SmithersFeatureGroups, SmithersFeatures } from "../../docs/smithers-tui/features.ts";

const writeResultSchema = z.object({
  success: z.boolean(),
  count: z.number().int().default(0),
}).passthrough();

const groupCheckSchema = z.object({
  groupId: z.string(),
  needsGeneration: z.boolean(),
  existingContent: z.string(),
}).passthrough();

const artifactCheckSchema = z.object({
  ticketId: z.string(),
  needsWork: z.boolean(),
  existingContent: z.string(),
}).passthrough();

const generatedTicketSchema = z.object({
  id: z.string().regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/),
  title: z.string(),
  type: z.enum(["feature", "engineering"]),
  featureName: z.string().nullable(),
  description: z.string(),
  acceptanceCriteria: z.array(z.string()).default([]),
  dependencies: z.array(z.string()).default([]),
  sourceContext: z.array(z.string()).default([]),
  implementationNotes: z.array(z.string()).default([]),
}).passthrough();

const groupTicketsSchema = z.object({
  groupId: z.string(),
  groupName: z.string(),
  tickets: z.array(generatedTicketSchema).default([]),
}).passthrough();

const engineeringSpecSchema = z.object({
  document: z.string(),
}).passthrough();

const researchDocumentSchema = z.object({
  document: z.string(),
}).passthrough();

const planDocumentSchema = z.object({
  document: z.string(),
}).passthrough();

const implementationSchema = z.object({
  summary: z.string(),
  filesChanged: z.array(z.string()).default([]),
  testsRun: z.array(z.string()).default([]),
  followUp: z.array(z.string()).default([]),
}).passthrough();

const reviewVerdictSchema = z.object({
  approved: z.boolean(),
  feedback: z.string(),
  issues: z.array(z.object({
    severity: z.enum(["critical", "major", "minor", "nit"]),
    title: z.string(),
    file: z.string().nullable().default(null),
    description: z.string(),
  })).default([]),
}).passthrough();

const reportSchema = z.object({
  groupCount: z.number().int(),
  ticketCount: z.number().int(),
  selectedTicketCount: z.number().int(),
  implementationEnabled: z.boolean(),
  summary: z.string(),
  artifactPaths: z.array(z.string()).default([]),
}).passthrough();

const doneSchema = z.object({
  success: z.boolean(),
}).passthrough();

const prResultSchema = z.object({
  ticketId: z.string(),
  branch: z.string(),
  prUrl: z.string(),
}).passthrough();

const { Workflow, smithers, outputs } = createSmithers({
  writeResult: writeResultSchema,
  groupCheck: groupCheckSchema,
  artifactCheck: artifactCheckSchema,
  groupTickets: groupTicketsSchema,
  engineeringSpec: engineeringSpecSchema,
  researchDocument: researchDocumentSchema,
  planDocument: planDocumentSchema,
  implementation: implementationSchema,
  reviewVerdict: reviewVerdictSchema,
  report: reportSchema,
  done: doneSchema,
  prResult: prResultSchema,
});

type GeneratedTicket = z.infer<typeof generatedTicketSchema>;
type StoredTicket = GeneratedTicket & {
  groupId: string;
  groupName: string;
};
type GroupTickets = z.infer<typeof groupTicketsSchema>;
type TicketGroupDescriptor = {
  id: string;
  name: string;
  featureNames: string[];
};
type TicketStage = {
  id: string;
  type: "spec" | "research" | "plan" | "implement";
  ticket: StoredTicket;
  dependsOn: string[];
};

const TUI_REMOTE = "tui";
const TUI_REMOTE_URL = "https://github.com/codeplaneapp/tui.git";
const WORKTREE_BASE = ".worktrees";

// These prompts pull in multiple project docs plus upstream Smithers
// references, so aggressive timeouts cause avoidable retry churn.
const GROUP_GENERATION_TIMEOUT_MS = 30 * 60 * 1000;
const DOCUMENT_GENERATION_TIMEOUT_MS = 30 * 60 * 1000;
const DOCUMENT_REVIEW_TIMEOUT_MS = 30 * 60 * 1000;
const IMPLEMENTATION_TIMEOUT_MS = 45 * 60 * 1000;
const IMPLEMENTATION_REVIEW_TIMEOUT_MS = 30 * 60 * 1000;

const ticketGroups: TicketGroupDescriptor[] = Object.entries(SmithersFeatureGroups).map(([groupName, featureNames]) => ({
  id: toKebabCase(groupName),
  name: humanizeIdentifier(groupName),
  featureNames: [...featureNames],
}));

const featureNames = [...SmithersFeatures];

function toKebabCase(value: string) {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function humanizeIdentifier(value: string) {
  return value
    .split(/[_-]+/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1).toLowerCase())
    .join(" ");
}

function normalizeStringArray(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }

  if (typeof value === "string") {
    return value
      .split(/[,\n]/g)
      .map((item) => item.trim())
      .filter(Boolean);
  }

  return [];
}

function normalizeBoolean(value: unknown, fallback = false) {
  if (typeof value === "boolean") return value;
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    if (["true", "1", "yes", "y"].includes(normalized)) return true;
    if (["false", "0", "no", "n"].includes(normalized)) return false;
  }
  return fallback;
}

function normalizeInt(value: unknown, fallback: number) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return Math.floor(parsed);
}

function uniq(values: string[]) {
  return [...new Set(values)];
}

function safeReadFileSync(filePath: string) {
  try {
    return fsSync.readFileSync(filePath, "utf8");
  } catch {
    return "";
  }
}

function safeReadJsonSync<T>(filePath: string, fallback: T): T {
  try {
    return JSON.parse(fsSync.readFileSync(filePath, "utf8")) as T;
  } catch {
    return fallback;
  }
}

function sortStoredTickets(tickets: StoredTicket[]) {
  return [...tickets].sort((a, b) => a.id.localeCompare(b.id));
}

/**
 * Flatten a ticket DAG into a linear topological order.
 * Within the same depth level, tickets are sorted alphabetically for stability.
 * Returns the ordered array — the stacking order for implementation.
 */
function topoSortTickets(tickets: StoredTicket[]): StoredTicket[] {
  const ticketMap = new Map(tickets.map((t) => [t.id, t]));
  const inDegree = new Map<string, number>();
  const adj = new Map<string, string[]>();

  for (const t of tickets) {
    if (!inDegree.has(t.id)) inDegree.set(t.id, 0);
    for (const dep of t.dependencies) {
      if (ticketMap.has(dep)) {
        if (!adj.has(dep)) adj.set(dep, []);
        adj.get(dep)!.push(t.id);
        inDegree.set(t.id, (inDegree.get(t.id) ?? 0) + 1);
      }
    }
  }

  const queue: string[] = [];
  for (const [id, deg] of inDegree) {
    if (deg === 0) queue.push(id);
  }
  queue.sort();

  const order: StoredTicket[] = [];
  while (queue.length > 0) {
    const id = queue.shift()!;
    const ticket = ticketMap.get(id);
    if (ticket) order.push(ticket);
    const children = adj.get(id) ?? [];
    for (const child of children) {
      const newDeg = (inDegree.get(child) ?? 1) - 1;
      inDegree.set(child, newDeg);
      if (newDeg === 0) {
        queue.push(child);
        queue.sort();
      }
    }
  }

  return order;
}

/**
 * Rebase all downstream worktrees in the stack.
 * When ticket at `fromIndex` changes, every worktree after it
 * needs to be rebased onto its new parent.
 */
function rebaseDownstream(
  stackOrder: StoredTicket[],
  fromIndex: number,
  repoRoot: string,
) {
  for (let i = fromIndex + 1; i < stackOrder.length; i++) {
    const ticket = stackOrder[i];
    const parentBranch = i === 0 ? "main" : `impl/${stackOrder[i - 1].id}`;
    const wtPath = path.join(repoRoot, WORKTREE_BASE, ticket.id);
    if (!fsSync.existsSync(wtPath)) continue;
    try {
      // jj rebase cascades to descendants automatically
      execSync(`jj rebase -d "${parentBranch}" --skip-emptied`, {
        cwd: wtPath,
        stdio: "pipe",
      });
    } catch {
      // If rebase fails (e.g., worktree not yet created), that's fine —
      // it will be created from the correct base later.
    }
  }
}

function getPaths(repoRoot: string, docsRoot: string) {
  const smithersRoot = path.join(repoRoot, ".smithers");
  const specsRoot = path.join(smithersRoot, "specs");
  return {
    smithersRoot,
    ticketsDir: path.join(smithersRoot, "tickets"),
    specsRoot,
    ticketGroupsDir: path.join(specsRoot, "ticket-groups"),
    engineeringDir: path.join(specsRoot, "engineering"),
    researchDir: path.join(specsRoot, "research"),
    plansDir: path.join(specsRoot, "plans"),
    reviewsDir: path.join(specsRoot, "reviews"),
    implementationDir: path.join(specsRoot, "implementation"),
    featureGroupsPath: path.join(specsRoot, "feature-groups.json"),
    manifestPath: path.join(specsRoot, "tickets.json"),
    prdPath: path.join(repoRoot, docsRoot, "01-PRD.md"),
    designPath: path.join(repoRoot, docsRoot, "02-DESIGN.md"),
    engineeringDocPath: path.join(repoRoot, docsRoot, "03-ENGINEERING.md"),
    featuresPath: path.join(repoRoot, docsRoot, "features.ts"),
  };
}

function withGroupMetadata(group: TicketGroupDescriptor, tickets: GeneratedTicket[]): StoredTicket[] {
  return tickets.map((ticket) => ({
    ...ticket,
    groupId: group.id,
    groupName: group.name,
    dependencies: uniq(ticket.dependencies ?? []),
    acceptanceCriteria: ticket.acceptanceCriteria ?? [],
    sourceContext: ticket.sourceContext ?? [],
    implementationNotes: ticket.implementationNotes ?? [],
  }));
}

function renderTicketMarkdown(ticket: StoredTicket) {
  const acceptanceCriteria = ticket.acceptanceCriteria.length > 0
    ? ticket.acceptanceCriteria.map((item) => `- ${item}`)
    : ["- Define concrete acceptance criteria before implementation."];
  const sourceContext = ticket.sourceContext.length > 0
    ? ticket.sourceContext.map((item) => `- ${item}`)
    : ["- Read the Smithers TUI docs and inspect the matching Crush and Smithers code paths."];
  const implementationNotes = ticket.implementationNotes.length > 0
    ? ticket.implementationNotes.map((item) => `- ${item}`)
    : ["- Keep the implementation aligned with the current docs and repository layout."];

  return [
    `# ${ticket.title}`,
    "",
    "## Metadata",
    `- ID: ${ticket.id}`,
    `- Group: ${ticket.groupName} (${ticket.groupId})`,
    `- Type: ${ticket.type}`,
    `- Feature: ${ticket.featureName ?? "n/a"}`,
    `- Dependencies: ${ticket.dependencies.length > 0 ? ticket.dependencies.join(", ") : "none"}`,
    "",
    "## Summary",
    "",
    ticket.description.trim(),
    "",
    "## Acceptance Criteria",
    "",
    ...acceptanceCriteria,
    "",
    "## Source Context",
    "",
    ...sourceContext,
    "",
    "## Implementation Notes",
    "",
    ...implementationNotes,
    "",
  ].join("\n");
}

function renderImplementationSummary(ticket: StoredTicket, implementation: z.infer<typeof implementationSchema>) {
  const filesChanged = implementation.filesChanged.length > 0
    ? implementation.filesChanged.map((item) => `- ${item}`)
    : ["- No files reported."];
  const testsRun = implementation.testsRun.length > 0
    ? implementation.testsRun.map((item) => `- ${item}`)
    : ["- No tests reported."];
  const followUp = implementation.followUp.length > 0
    ? implementation.followUp.map((item) => `- ${item}`)
    : ["- None."];

  return [
    `# Implementation Summary: ${ticket.id}`,
    "",
    `- Ticket: ${ticket.title}`,
    `- Group: ${ticket.groupName} (${ticket.groupId})`,
    "",
    "## Summary",
    "",
    implementation.summary.trim(),
    "",
    "## Files Changed",
    "",
    ...filesChanged,
    "",
    "## Validation",
    "",
    ...testsRun,
    "",
    "## Follow Up",
    "",
    ...followUp,
    "",
  ].join("\n");
}

function buildSourceGuide(docsRoot: string, referenceRoot: string, additionalContext: string) {
  const lines = [
    "Authoritative planning inputs for this work:",
    `- ${path.join(docsRoot, "01-PRD.md")} for product scope and goals`,
    `- ${path.join(docsRoot, "02-DESIGN.md")} for UX, navigation, and view design`,
    `- ${path.join(docsRoot, "03-ENGINEERING.md")} for architecture and implementation direction`,
    `- ${path.join(docsRoot, "features.ts")} for the canonical Smithers TUI feature inventory`,
    "",
    "Current Crush code to inspect:",
    "- internal/app",
    "- internal/agent",
    "- internal/config",
    "- internal/ui",
    "- internal/ui/model",
    "- internal/ui/chat",
    "- internal/ui/styles",
    "",
    "Primary Smithers reference code to inspect:",
    `- ${path.join(referenceRoot, "src")}`,
    `- ${path.join(referenceRoot, "src/server")}`,
    `- ${path.join(referenceRoot, "gui/src")}`,
    `- ${path.join(referenceRoot, "gui-ref")}`,
    `- ${path.join(referenceRoot, "tests/tui.e2e.test.ts")}`,
    `- ${path.join(referenceRoot, "tests/tui-helpers.ts")}`,
    `- ${path.join(referenceRoot, "docs/guides/smithers-tui-v2-agent-handoff.md")}`,
    "",
    "Use ../smithers/gui-ref only as a reference for prior GUI behavior. Current implementation details should come from ../smithers/src and ../smithers/gui/src first.",
    "Testing expectation: plans should include both a terminal E2E path modeled on the upstream @microsoft/tui-test harness in ../smithers/tests/tui.e2e.test.ts and ../smithers/tests/tui-helpers.ts, and at least one VHS-style happy-path recording test for Crush's TUI.",
  ];

  const extra = String(additionalContext ?? "").trim();
  if (extra.length > 0) {
    lines.push("", "Additional operator context:", extra);
  }

  return lines.join("\n");
}

function buildGroupSummary(groups: TicketGroupDescriptor[]) {
  return groups
    .map((group) => `- ${group.name} (${group.id}): ${group.featureNames.join(", ")}`)
    .join("\n");
}

export default smithers((ctx) => {
  const repoRoot = process.cwd();
  const docsRoot = String(ctx.input.docsRoot ?? "docs/smithers-tui");
  const referenceRoot = String(ctx.input.referenceRoot ?? "../smithers");
  const additionalContext = String(ctx.input.additionalContext ?? "");
  const maxGroupConcurrency = normalizeInt(ctx.input.maxGroupConcurrency, 4);
  const maxTicketConcurrency = normalizeInt(ctx.input.maxTicketConcurrency, 3);
  const maxReviewIterations = normalizeInt(ctx.input.maxReviewIterations, 3);
  const regenerateGroups = new Set(normalizeStringArray(ctx.input.regenerateGroups));
  const forceTickets = new Set(normalizeStringArray(ctx.input.forceTickets));
  const selectedTicketIds = normalizeStringArray(ctx.input.ticketIds);
  const implementationEnabled = !normalizeBoolean(ctx.input.skipImplementation, false);

  const paths = getPaths(repoRoot, docsRoot);
  const sourceGuide = buildSourceGuide(docsRoot, referenceRoot, additionalContext);
  const groupSummary = buildGroupSummary(ticketGroups);

  const manifestTickets = safeReadJsonSync<StoredTicket[]>(paths.manifestPath, []);
  const selectedTickets = selectedTicketIds.length > 0
    ? manifestTickets.filter((ticket) => selectedTicketIds.includes(ticket.id))
    : manifestTickets;
  const selectedTicketSet = new Set(selectedTickets.map((ticket) => ticket.id));
  const dependencyBarrierPrefix = implementationEnabled ? "done-impl-" : "done-plan-";

  const ticketStages = selectedTickets.flatMap((ticket) => {
    const upstreamBarrierDeps = ticket.dependencies
      .filter((dependencyId) => selectedTicketSet.has(dependencyId))
      .map((dependencyId) => `${dependencyBarrierPrefix}${dependencyId}`);

    const stages: TicketStage[] = [
      {
        id: `spec-${ticket.id}`,
        type: "spec" as const,
        ticket,
        dependsOn: upstreamBarrierDeps,
      },
      {
        id: `research-${ticket.id}`,
        type: "research" as const,
        ticket,
        dependsOn: [`done-spec-${ticket.id}`],
      },
      {
        id: `plan-${ticket.id}`,
        type: "plan" as const,
        ticket,
        dependsOn: [`done-research-${ticket.id}`],
      },
    ];

    if (implementationEnabled) {
      stages.push({
        id: `implement-${ticket.id}`,
        type: "implement" as const,
        ticket,
        dependsOn: [`done-plan-${ticket.id}`],
      });
    }

    return stages;
  });

  const specResearchPlanStages = ticketStages.filter((s) => s.type !== "implement");
  // Linear topo-sorted stack order for implementation — each ticket stacks on the previous
  const stackOrder = topoSortTickets(selectedTickets);
  const stackIndex = new Map(stackOrder.map((t, i) => [t.id, i]));
  const implementStages = ticketStages.filter((s) => s.type === "implement");
  const maxImplConcurrency = normalizeInt(ctx.input.maxImplConcurrency, 2);
  const maxPreImplConcurrency = Math.max(1, maxTicketConcurrency - maxImplConcurrency);

  return (
    <Workflow name="specs">
      <Sequence>
        <Task id="init-artifacts" output={outputs.writeResult}>
          {async () => {
            await fs.mkdir(paths.ticketsDir, { recursive: true });
            await fs.mkdir(paths.specsRoot, { recursive: true });
            await fs.mkdir(paths.ticketGroupsDir, { recursive: true });
            await fs.mkdir(paths.engineeringDir, { recursive: true });
            await fs.mkdir(paths.researchDir, { recursive: true });
            await fs.mkdir(paths.plansDir, { recursive: true });
            await fs.mkdir(paths.reviewsDir, { recursive: true });
            await fs.mkdir(paths.implementationDir, { recursive: true });
            return { success: true, count: 8 };
          }}
        </Task>

        <Task id="write-feature-groups" output={outputs.writeResult} dependsOn={["init-artifacts"]}>
          {async () => {
            await fs.writeFile(paths.featureGroupsPath, JSON.stringify(ticketGroups, null, 2), "utf8");
            return { success: true, count: ticketGroups.length };
          }}
        </Task>

        <Parallel maxConcurrency={maxGroupConcurrency}>
          {ticketGroups.map((group) => {
            const check = ctx.outputMaybe(outputs.groupCheck, { nodeId: `check-group-${group.id}` });

            return (
              <Sequence key={group.id}>
                <Task id={`check-group-${group.id}`} output={outputs.groupCheck} dependsOn={["write-feature-groups"]}>
                  {async () => {
                    const groupPath = path.join(paths.ticketGroupsDir, `${group.id}.json`);
                    let existingContent = "";
                    let needsGeneration = true;

                    try {
                      existingContent = await fs.readFile(groupPath, "utf8");
                      const parsed = JSON.parse(existingContent) as GroupTickets;
                      if (Array.isArray(parsed.tickets) && parsed.tickets.length > 0) {
                        needsGeneration = false;
                      }
                    } catch {
                      needsGeneration = true;
                    }

                    if (regenerateGroups.has(group.id)) {
                      needsGeneration = true;
                    }

                    return {
                      groupId: group.id,
                      needsGeneration,
                      existingContent,
                    };
                  }}
                </Task>

                {check ? (
                  <Branch
                    if={check.needsGeneration}
                    then={
                      <Sequence>
                        <Task
                          id={`generate-group-${group.id}`}
                          output={outputs.groupTickets}
                          agent={pickAgent("plan")}
                          retries={2}
                          timeoutMs={GROUP_GENERATION_TIMEOUT_MS}
                        >
                          {[
                            "You are defining the execution DAG for one Smithers TUI feature group.",
                            "",
                            sourceGuide,
                            "",
                            `Feature group: ${group.name} (${group.id})`,
                            "Features in this group:",
                            ...group.featureNames.map((feature) => `- ${feature}`),
                            "",
                            "Global feature group summary:",
                            groupSummary,
                            "",
                            "Requirements:",
                            "1. Read the authoritative docs and inspect the real code before writing tickets.",
                            "2. Create a tight DAG of tickets for only this group.",
                            "3. Every feature in this group must be completed by exactly one feature ticket.",
                            "4. Add engineering tickets for shared prerequisites, abstractions, or scaffolding when required.",
                            "5. Ticket IDs must be lowercase kebab-case and stable.",
                            "6. Do not invent speculative work that is not supported by the current docs or code.",
                            "7. Focus on Smithers functionality we are adding to Crush now, not legacy Smithers experiments.",
                            "8. Use concrete sourceContext and implementationNotes that point at real files, directories, or subsystems to inspect.",
                            "9. Feature tickets must set featureName to the exact SmithersFeature enum name they complete.",
                            "10. Engineering tickets must set featureName to null.",
                            "",
                            `There are ${featureNames.length} total Smithers features in scope across the project.`,
                            "",
                            "Return structured JSON matching the schema.",
                          ].join("\n")}
                        </Task>

                        {ctx.outputMaybe(outputs.groupTickets, { nodeId: `generate-group-${group.id}` }) ? (
                          <Task id={`write-group-${group.id}`} output={outputs.writeResult}>
                            {async () => {
                              const generated = ctx.outputMaybe(outputs.groupTickets, { nodeId: `generate-group-${group.id}` });
                              const storedTickets = sortStoredTickets(withGroupMetadata(group, generated?.tickets ?? []));
                              const groupPayload = {
                                groupId: group.id,
                                groupName: group.name,
                                tickets: storedTickets,
                              };

                              await fs.writeFile(
                                path.join(paths.ticketGroupsDir, `${group.id}.json`),
                                JSON.stringify(groupPayload, null, 2),
                                "utf8",
                              );

                              for (const ticket of storedTickets) {
                                await fs.writeFile(
                                  path.join(paths.ticketsDir, `${ticket.id}.md`),
                                  renderTicketMarkdown(ticket),
                                  "utf8",
                                );
                              }

                              return { success: true, count: storedTickets.length };
                            }}
                          </Task>
                        ) : null}
                      </Sequence>
                    }
                    else={
                      <Task id={`write-group-${group.id}`} output={outputs.writeResult}>
                        {{ success: true, count: 0 }}
                      </Task>
                    }
                  />
                ) : null}
              </Sequence>
            );
          })}
        </Parallel>

        <Task
          id="write-ticket-manifest"
          output={outputs.writeResult}
          dependsOn={ticketGroups.map((group) => `write-group-${group.id}`)}
        >
          {async () => {
            const allTickets: StoredTicket[] = [];

            for (const group of ticketGroups) {
              const groupPath = path.join(paths.ticketGroupsDir, `${group.id}.json`);
              const parsed = safeReadJsonSync<GroupTickets | null>(groupPath, null);
              if (!parsed || !Array.isArray(parsed.tickets)) continue;

              allTickets.push(...withGroupMetadata(group, parsed.tickets));
            }

            const deduped = sortStoredTickets(
              Object.values(
                Object.fromEntries(
                  allTickets.map((ticket) => [ticket.id, ticket]),
                ),
              ) as StoredTicket[],
            );

            await fs.writeFile(paths.manifestPath, JSON.stringify(deduped, null, 2), "utf8");
            return { success: true, count: deduped.length };
          }}
        </Task>

        {ctx.outputMaybe(outputs.writeResult, { nodeId: "write-ticket-manifest" }) && ticketStages.length > 0 ? (
          <Parallel>
            <Parallel maxConcurrency={maxPreImplConcurrency}>
              {specResearchPlanStages.map((stage) => {
                const ticket = stage.ticket;

                if (stage.type === "spec") {
                const check = ctx.outputMaybe(outputs.artifactCheck, { nodeId: `check-spec-${ticket.id}` });

                return (
                  <Sequence key={stage.id}>
                    <Task id={`check-spec-${ticket.id}`} output={outputs.artifactCheck} dependsOn={stage.dependsOn}>
                      {async () => {
                        const existingContent = await fs.readFile(
                          path.join(paths.engineeringDir, `${ticket.id}.md`),
                          "utf8",
                        ).catch(() => "");
                        return {
                          ticketId: ticket.id,
                          needsWork: existingContent.length < 20 || forceTickets.has(ticket.id),
                          existingContent,
                        };
                      }}
                    </Task>

                    {check ? (
                      <Branch
                        if={check.needsWork}
                        then={
                          <Sequence>
                            <Task
                              id={`generate-spec-${ticket.id}`}
                              output={outputs.engineeringSpec}
                              agent={pickAgent("spec")}
                              retries={2}
                              timeoutMs={DOCUMENT_GENERATION_TIMEOUT_MS}
                            >
                              {[
                                `Write the detailed engineering specification for ticket ${ticket.id}.`,
                                "",
                                sourceGuide,
                                "",
                                `Ticket file: ${path.join(".smithers", "tickets", `${ticket.id}.md`)}`,
                                "",
                                "Requirements:",
                                "1. Read the ticket, the Smithers TUI docs, the relevant Crush code, and the relevant upstream Smithers references before writing.",
                                "2. Treat ../smithers as a reference implementation and Crush as the target implementation.",
                                "3. Keep the document grounded in concrete files, data flows, and UI surfaces.",
                                "4. Include these exact sections: ## Objective, ## Scope, ## Implementation Plan, ## Validation, ## Risks.",
                                "5. The implementation plan should break the ticket into concrete vertical slices, not vague themes.",
                                "6. Validation must name real commands, checks, or manual verification paths.",
                                "7. Validation must explicitly cover both terminal E2E coverage modeled on the upstream @microsoft/tui-test harness in ../smithers/tests/tui.e2e.test.ts plus ../smithers/tests/tui-helpers.ts and at least one VHS-style happy-path recording test.",
                                "8. Call out any meaningful mismatch between Crush and upstream Smithers that affects execution.",
                                "",
                                "Existing content to improve if present:",
                                check.existingContent || "None.",
                              ].join("\n")}
                            </Task>

                            {ctx.outputMaybe(outputs.engineeringSpec, { nodeId: `generate-spec-${ticket.id}` }) ? (
                              <Task id={`write-spec-${ticket.id}`} output={outputs.writeResult}>
                                {async () => {
                                  const generated = ctx.outputMaybe(outputs.engineeringSpec, { nodeId: `generate-spec-${ticket.id}` });
                                  await fs.writeFile(
                                    path.join(paths.engineeringDir, `${ticket.id}.md`),
                                    generated?.document ?? "",
                                    "utf8",
                                  );
                                  return { success: true, count: 1 };
                                }}
                              </Task>
                            ) : null}
                          </Sequence>
                        }
                        else={
                          <Task id={`write-spec-${ticket.id}`} output={outputs.writeResult}>
                            {{ success: true, count: 0 }}
                          </Task>
                        }
                      />
                    ) : null}

                    {ctx.outputMaybe(outputs.writeResult, { nodeId: `write-spec-${ticket.id}` }) ? (
                      <Task id={`done-spec-${ticket.id}`} output={outputs.done}>
                        {{ success: true }}
                      </Task>
                    ) : null}
                  </Sequence>
                );
              }

              if (stage.type === "research") {
                const check = ctx.outputMaybe(outputs.artifactCheck, { nodeId: `check-research-${ticket.id}` });

                return (
                  <Sequence key={stage.id}>
                    <Task id={`check-research-${ticket.id}`} output={outputs.artifactCheck} dependsOn={stage.dependsOn}>
                      {async () => {
                        const existingContent = await fs.readFile(
                          path.join(paths.researchDir, `${ticket.id}.md`),
                          "utf8",
                        ).catch(() => "");
                        return {
                          ticketId: ticket.id,
                          needsWork: existingContent.length < 20 || forceTickets.has(ticket.id),
                          existingContent,
                        };
                      }}
                    </Task>

                    {check ? (
                      <Branch
                        if={check.needsWork}
                        then={
                          <Sequence>
                            <Loop
                              id={`research-loop-${ticket.id}`}
                              until={ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-research-${ticket.id}` })?.approved === true}
                              maxIterations={maxReviewIterations}
                              onMaxReached="return-last"
                            >
                              <Sequence>
                                <Task
                                  id={`generate-research-${ticket.id}`}
                                  output={outputs.researchDocument}
                                  agent={pickAgent("research")}
                                  retries={2}
                                  timeoutMs={DOCUMENT_GENERATION_TIMEOUT_MS}
                                >
                                  {[
                                    `Research ticket ${ticket.id}.`,
                                    "",
                                    sourceGuide,
                                    "",
                                    `Ticket file: ${path.join(".smithers", "tickets", `${ticket.id}.md`)}`,
                                    `Engineering spec: ${path.join(".smithers", "specs", "engineering", `${ticket.id}.md`)}`,
                                    "",
                                    "Requirements:",
                                    "1. Inspect the current Crush code and the upstream Smithers reference surfaces that matter for this ticket.",
                                    "2. Cite concrete file paths and explain how they are relevant.",
                                    "3. Highlight any data-model, transport, rendering, or UX gaps between Crush and Smithers.",
                                    "4. Include these exact sections: ## Existing Crush Surface, ## Upstream Smithers Reference, ## Gaps, ## Recommended Direction, ## Files To Touch.",
                                    "5. Prefer evidence from real code over speculation.",
                                    "",
                                    ctx.iteration > 0
                                      ? `Previous review feedback to address:\n${ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-research-${ticket.id}` })?.feedback ?? ""}`
                                      : "This is the first research pass.",
                                  ].join("\n")}
                                </Task>

                                <Task
                                  id={`review-research-${ticket.id}`}
                                  output={outputs.reviewVerdict}
                                  agent={pickAgent("review")}
                                  retries={1}
                                  timeoutMs={DOCUMENT_REVIEW_TIMEOUT_MS}
                                >
                                  {[
                                    `Review the research document for ticket ${ticket.id}.`,
                                    "",
                                    "Approve only if it is specific, evidence-backed, and grounded in both the current Crush repo and the upstream Smithers references.",
                                    "Reject if it hand-waves, lacks file paths, ignores the Smithers server/gui references, or misses obvious implementation constraints.",
                                    "",
                                    "Research document:",
                                    ctx.outputMaybe(outputs.researchDocument, { nodeId: `generate-research-${ticket.id}` })?.document ?? "",
                                  ].join("\n")}
                                </Task>

                                <Task id={`write-review-research-${ticket.id}`} output={outputs.writeResult}>
                                  {async () => {
                                    const verdict = ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-research-${ticket.id}` });
                                    if (verdict && !verdict.approved) {
                                      await fs.writeFile(
                                        path.join(paths.reviewsDir, `research-${ticket.id}-iteration-${ctx.iteration + 1}.md`),
                                        verdict.feedback,
                                        "utf8",
                                      );
                                    }
                                    return { success: true, count: verdict?.approved ? 0 : 1 };
                                  }}
                                </Task>
                              </Sequence>
                            </Loop>

                            {ctx.latest("researchDocument", `generate-research-${ticket.id}`) ? (
                              <Task id={`write-research-${ticket.id}`} output={outputs.writeResult}>
                                {async () => {
                                  const latest = ctx.latest("researchDocument", `generate-research-${ticket.id}`);
                                  if (!latest) {
                                    return { success: false, count: 0 };
                                  }
                                  await fs.writeFile(
                                    path.join(paths.researchDir, `${ticket.id}.md`),
                                    latest.document,
                                    "utf8",
                                  );
                                  return { success: true, count: 1 };
                                }}
                              </Task>
                            ) : null}
                          </Sequence>
                        }
                        else={
                          <Task id={`write-research-${ticket.id}`} output={outputs.writeResult}>
                            {{ success: true, count: 0 }}
                          </Task>
                        }
                      />
                    ) : null}

                    {ctx.outputMaybe(outputs.writeResult, { nodeId: `write-research-${ticket.id}` }) ? (
                      <Task id={`done-research-${ticket.id}`} output={outputs.done}>
                        {{ success: true }}
                      </Task>
                    ) : null}
                  </Sequence>
                );
              }

              if (stage.type === "plan") {
                const check = ctx.outputMaybe(outputs.artifactCheck, { nodeId: `check-plan-${ticket.id}` });

                return (
                  <Sequence key={stage.id}>
                    <Task id={`check-plan-${ticket.id}`} output={outputs.artifactCheck} dependsOn={stage.dependsOn}>
                      {async () => {
                        const existingContent = await fs.readFile(
                          path.join(paths.plansDir, `${ticket.id}.md`),
                          "utf8",
                        ).catch(() => "");
                        return {
                          ticketId: ticket.id,
                          needsWork: existingContent.length < 20 || forceTickets.has(ticket.id),
                          existingContent,
                        };
                      }}
                    </Task>

                    {check ? (
                      <Branch
                        if={check.needsWork}
                        then={
                          <Sequence>
                            <Loop
                              id={`plan-loop-${ticket.id}`}
                              until={ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-plan-${ticket.id}` })?.approved === true}
                              maxIterations={maxReviewIterations}
                              onMaxReached="return-last"
                            >
                              <Sequence>
                                <Task
                                  id={`generate-plan-${ticket.id}`}
                                  output={outputs.planDocument}
                                  agent={pickAgent("plan")}
                                  retries={2}
                                  timeoutMs={DOCUMENT_GENERATION_TIMEOUT_MS}
                                >
                                  {[
                                    `Create the implementation plan for ticket ${ticket.id}.`,
                                    "",
                                    sourceGuide,
                                    "",
                                    `Ticket file: ${path.join(".smithers", "tickets", `${ticket.id}.md`)}`,
                                    `Engineering spec: ${path.join(".smithers", "specs", "engineering", `${ticket.id}.md`)}`,
                                    `Research: ${path.join(".smithers", "specs", "research", `${ticket.id}.md`)}`,
                                    "",
                                    "Requirements:",
                                    "1. Turn the research and engineering spec into a concrete execution plan for this repo.",
                                    "2. Include these exact sections: ## Goal, ## Steps, ## File Plan, ## Validation, ## Open Questions.",
                                    "3. File Plan must name concrete files or directories expected to change.",
                                    "4. Validation must include real commands or concrete manual checks.",
                                    "5. Validation must explicitly include terminal E2E coverage modeled on the upstream @microsoft/tui-test harness in ../smithers/tests/tui.e2e.test.ts and ../smithers/tests/tui-helpers.ts, plus at least one VHS-style happy-path recording test in this repo.",
                                    "6. Sequence the work to minimize regressions and rework.",
                                    "",
                                    ctx.iteration > 0
                                      ? `Previous review feedback to address:\n${ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-plan-${ticket.id}` })?.feedback ?? ""}`
                                      : "This is the first planning pass.",
                                  ].join("\n")}
                                </Task>

                                <Task
                                  id={`review-plan-${ticket.id}`}
                                  output={outputs.reviewVerdict}
                                  agent={pickAgent("review")}
                                  retries={1}
                                  timeoutMs={DOCUMENT_REVIEW_TIMEOUT_MS}
                                >
                                  {[
                                    `Review the implementation plan for ticket ${ticket.id}.`,
                                    "",
                                    "Approve only if the plan is actionable, sequenced correctly, references real files, and includes meaningful validation.",
                                    "Reject if it is generic, skips critical dependencies, or fails to connect the work back to the current Crush and Smithers codebases.",
                                    "",
                                    "Plan document:",
                                    ctx.outputMaybe(outputs.planDocument, { nodeId: `generate-plan-${ticket.id}` })?.document ?? "",
                                  ].join("\n")}
                                </Task>

                                <Task id={`write-review-plan-${ticket.id}`} output={outputs.writeResult}>
                                  {async () => {
                                    const verdict = ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-plan-${ticket.id}` });
                                    if (verdict && !verdict.approved) {
                                      await fs.writeFile(
                                        path.join(paths.reviewsDir, `plan-${ticket.id}-iteration-${ctx.iteration + 1}.md`),
                                        verdict.feedback,
                                        "utf8",
                                      );
                                    }
                                    return { success: true, count: verdict?.approved ? 0 : 1 };
                                  }}
                                </Task>
                              </Sequence>
                            </Loop>

                            {ctx.latest("planDocument", `generate-plan-${ticket.id}`) ? (
                              <Task id={`write-plan-${ticket.id}`} output={outputs.writeResult}>
                                {async () => {
                                  const latest = ctx.latest("planDocument", `generate-plan-${ticket.id}`);
                                  if (!latest) {
                                    return { success: false, count: 0 };
                                  }
                                  await fs.writeFile(
                                    path.join(paths.plansDir, `${ticket.id}.md`),
                                    latest.document,
                                    "utf8",
                                  );
                                  return { success: true, count: 1 };
                                }}
                              </Task>
                            ) : null}
                          </Sequence>
                        }
                        else={
                          <Task id={`write-plan-${ticket.id}`} output={outputs.writeResult}>
                            {{ success: true, count: 0 }}
                          </Task>
                        }
                      />
                    ) : null}

                    {ctx.outputMaybe(outputs.writeResult, { nodeId: `write-plan-${ticket.id}` }) ? (
                      <Task id={`done-plan-${ticket.id}`} output={outputs.done}>
                        {{ success: true }}
                      </Task>
                    ) : null}
                  </Sequence>
                );
              }

              // spec/research/plan stages should not reach here
              return null;
            })}
            </Parallel>

            <MergeQueue maxConcurrency={maxImplConcurrency}>
              {stackOrder.map((ticket, idx) => {
                const branchName = `impl/${ticket.id}`;
                const parentBranch = idx === 0 ? "main" : `impl/${stackOrder[idx - 1].id}`;
                const worktreePath = path.join(repoRoot, WORKTREE_BASE, ticket.id);
                const check = ctx.outputMaybe(outputs.artifactCheck, { nodeId: `check-impl-${ticket.id}` });
                // Implementation depends on its own plan AND the previous ticket in the stack being done
                const implDeps = [`done-plan-${ticket.id}`];
                if (idx > 0) implDeps.push(`done-impl-${stackOrder[idx - 1].id}`);

              return (
                <Sequence key={`implement-${ticket.id}`}>
                  <Task id={`check-impl-${ticket.id}`} output={outputs.artifactCheck} dependsOn={implDeps}>
                    {async () => {
                      const existingContent = await fs.readFile(
                        path.join(paths.implementationDir, `${ticket.id}.md`),
                        "utf8",
                      ).catch(() => "");
                      return {
                        ticketId: ticket.id,
                        needsWork: existingContent.length < 20 || forceTickets.has(ticket.id),
                        existingContent,
                      };
                    }}
                  </Task>

                  {check ? (
                    <Branch
                      if={check.needsWork}
                      then={
                        <Worktree path={worktreePath} branch={branchName} baseBranch={parentBranch}>
                          <Sequence>
                            <Loop
                              id={`implement-loop-${ticket.id}`}
                              until={ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-impl-${ticket.id}` })?.approved === true}
                              maxIterations={maxReviewIterations}
                              onMaxReached="return-last"
                            >
                              <Sequence>
                                <Task
                                  id={`implement-${ticket.id}`}
                                  output={outputs.implementation}
                                  agent={pickAgent("implement")}
                                  retries={1}
                                  timeoutMs={IMPLEMENTATION_TIMEOUT_MS}
                                >
                                  {[
                                    `Implement ticket ${ticket.id} in the current repository.`,
                                    "",
                                    sourceGuide,
                                    "",
                                    `Ticket file: ${path.join(".smithers", "tickets", `${ticket.id}.md`)}`,
                                    `Engineering spec: ${path.join(".smithers", "specs", "engineering", `${ticket.id}.md`)}`,
                                    `Research: ${path.join(".smithers", "specs", "research", `${ticket.id}.md`)}`,
                                    `Plan: ${path.join(".smithers", "specs", "plans", `${ticket.id}.md`)}`,
                                    "",
                                    `This ticket is part of a stacked implementation chain.`,
                                    `Parent branch: ${parentBranch}`,
                                    `Your branch: ${branchName}`,
                                    "",
                                    "Requirements:",
                                    "1. Treat the upstream Smithers repo as reference material. Make the actual code changes in this Crush repo unless the ticket explicitly says otherwise.",
                                    "2. Follow the ticket, engineering spec, research, and plan closely.",
                                    "3. Run relevant tests or validation commands before finishing.",
                                    "4. Keep changes scoped to this ticket and avoid unrelated cleanup.",
                                    "5. Return filesChanged, testsRun, and any follow-up items honestly.",
                                    "6. Commit your changes with a descriptive message before finishing.",
                                    "",
                                    ctx.iteration > 0
                                      ? `Previous review feedback to address:\n${ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-impl-${ticket.id}` })?.feedback ?? ""}`
                                      : "This is the first implementation pass.",
                                  ].join("\n")}
                                </Task>

                                <Task
                                  id={`review-impl-${ticket.id}`}
                                  output={outputs.reviewVerdict}
                                  agent={pickAgent("review")}
                                  retries={1}
                                  timeoutMs={IMPLEMENTATION_REVIEW_TIMEOUT_MS}
                                >
                                  {[
                                    `Review the implementation for ticket ${ticket.id}.`,
                                    "",
                                    "Your job:",
                                    "1. Read the modified files.",
                                    "2. Run the relevant tests or validation commands yourself.",
                                    "3. Compare the result against the ticket, engineering spec, research, and plan.",
                                    "4. Approve only if the change is complete and high quality.",
                                    "",
                                    "Implementation summary:",
                                    ctx.outputMaybe(outputs.implementation, { nodeId: `implement-${ticket.id}` })?.summary ?? "",
                                    "",
                                    "Files changed:",
                                    ...(ctx.outputMaybe(outputs.implementation, { nodeId: `implement-${ticket.id}` })?.filesChanged ?? []).map((item) => `- ${item}`),
                                    "",
                                    "Tests run by implementer:",
                                    ...(ctx.outputMaybe(outputs.implementation, { nodeId: `implement-${ticket.id}` })?.testsRun ?? []).map((item) => `- ${item}`),
                                  ].join("\n")}
                                </Task>

                                {/* After each review loop iteration, rebase all downstream worktrees */}
                                <Task id={`rebase-downstream-${ticket.id}`} output={outputs.writeResult}>
                                  {async () => {
                                    const verdict = ctx.outputMaybe(outputs.reviewVerdict, { nodeId: `review-impl-${ticket.id}` });
                                    if (verdict && !verdict.approved) {
                                      await fs.writeFile(
                                        path.join(paths.reviewsDir, `implement-${ticket.id}-iteration-${ctx.iteration + 1}.md`),
                                        verdict.feedback,
                                        "utf8",
                                      );
                                    }
                                    // Rebase everything downstream of this ticket in the stack
                                    rebaseDownstream(stackOrder, idx, repoRoot);
                                    return { success: true, count: verdict?.approved ? 0 : 1 };
                                  }}
                                </Task>
                              </Sequence>
                            </Loop>

                            {ctx.latest("implementation", `implement-${ticket.id}`) ? (
                              <Task id={`write-impl-${ticket.id}`} output={outputs.writeResult}>
                                {async () => {
                                  const latest = ctx.latest("implementation", `implement-${ticket.id}`);
                                  if (!latest) {
                                    return { success: false, count: 0 };
                                  }
                                  await fs.writeFile(
                                    path.join(paths.implementationDir, `${ticket.id}.md`),
                                    renderImplementationSummary(ticket, latest),
                                    "utf8",
                                  );
                                  return { success: true, count: 1 };
                                }}
                              </Task>
                            ) : null}

                            {ctx.outputMaybe(outputs.writeResult, { nodeId: `write-impl-${ticket.id}` })?.success ? (
                              <Task id={`push-and-pr-${ticket.id}`} output={outputs.prResult}>
                                {async () => {
                                  const impl = ctx.latest("implementation", `implement-${ticket.id}`);
                                  const summary = impl?.summary ?? `Implement ${ticket.id}`;
                                  const filesChanged = impl?.filesChanged ?? [];

                                  // Push branch to tui remote via jj
                                  execSync(`jj git push --branch ${branchName}`, {
                                    cwd: worktreePath,
                                    stdio: "pipe",
                                  });

                                  // Check if PR already exists for this branch
                                  let prUrl = "";
                                  try {
                                    prUrl = execSync(
                                      `gh pr view ${branchName} --repo codeplaneapp/tui --json url -q .url`,
                                      { cwd: worktreePath, stdio: "pipe", encoding: "utf8" },
                                    ).trim();
                                  } catch {
                                    // No existing PR — create one
                                  }

                                  const stackPosition = `Stack position: ${idx + 1}/${stackOrder.length}`;
                                  const prBody = [
                                    `## ${ticket.title}`,
                                    "",
                                    `**Ticket:** ${ticket.id}`,
                                    `**Group:** ${ticket.groupName} (${ticket.groupId})`,
                                    `**Type:** ${ticket.type}`,
                                    `**${stackPosition}**`,
                                    `**Base:** \`${parentBranch}\``,
                                    "",
                                    "## Summary",
                                    "",
                                    summary,
                                    "",
                                    "## Files Changed",
                                    "",
                                    ...filesChanged.map((f) => `- ${f}`),
                                    "",
                                    "---",
                                    "*Stacked PR — automated by Smithers Specs Pipeline*",
                                  ].join("\n");

                                  if (!prUrl) {
                                    prUrl = execSync(
                                      `gh pr create --repo codeplaneapp/tui --base ${parentBranch} --head ${branchName} --title ${JSON.stringify(`[${idx + 1}/${stackOrder.length}] ${ticket.title}`)} --body ${JSON.stringify(prBody)}`,
                                      { cwd: worktreePath, stdio: "pipe", encoding: "utf8" },
                                    ).trim();
                                  } else {
                                    // Update existing PR body with latest info
                                    execSync(
                                      `gh pr edit ${branchName} --repo codeplaneapp/tui --body ${JSON.stringify(prBody)}`,
                                      { cwd: worktreePath, stdio: "pipe" },
                                    );
                                  }

                                  return {
                                    ticketId: ticket.id,
                                    branch: branchName,
                                    prUrl,
                                  };
                                }}
                              </Task>
                            ) : null}
                          </Sequence>
                        </Worktree>
                      }
                      else={
                        <Task id={`write-impl-${ticket.id}`} output={outputs.writeResult}>
                          {{ success: true, count: 0 }}
                        </Task>
                      }
                    />
                  ) : null}

                  {ctx.outputMaybe(outputs.writeResult, { nodeId: `write-impl-${ticket.id}` }) ? (
                    <Task id={`done-impl-${ticket.id}`} output={outputs.done}>
                      {{ success: true }}
                    </Task>
                  ) : null}
                </Sequence>
              );
              })}
            </MergeQueue>
          </Parallel>
        ) : null}

        {ctx.outputMaybe(outputs.writeResult, { nodeId: "write-ticket-manifest" }) ? (
          <Task id="report" output={outputs.report}>
            {async () => {
              const tickets = safeReadJsonSync<StoredTicket[]>(paths.manifestPath, []);
              const selected = selectedTicketIds.length > 0
                ? tickets.filter((ticket) => selectedTicketIds.includes(ticket.id))
                : tickets;

              return {
                groupCount: ticketGroups.length,
                ticketCount: tickets.length,
                selectedTicketCount: selected.length,
                implementationEnabled,
                summary: implementationEnabled
                  ? `Generated ${tickets.length} ticket(s) across ${ticketGroups.length} group(s) and processed ${selected.length} selected ticket(s) through specs, research, planning, and implementation.`
                  : `Generated ${tickets.length} ticket(s) across ${ticketGroups.length} group(s) and processed ${selected.length} selected ticket(s) through specs, research, and planning.`,
                artifactPaths: [
                  paths.featureGroupsPath,
                  paths.manifestPath,
                  paths.ticketsDir,
                  paths.engineeringDir,
                  paths.researchDir,
                  paths.plansDir,
                  paths.reviewsDir,
                  paths.implementationDir,
                ],
              };
            }}
          </Task>
        ) : null}
      </Sequence>
    </Workflow>
  );
});
