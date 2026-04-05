# Combined Engineering Spec & Implementation Plan: Smithers MCP Tool Groups

Covers all 12 feature tickets:
`feat-mcp-runs-tools`, `feat-mcp-observability-tools`, `feat-mcp-control-tools`,
`feat-mcp-time-travel-tools`, `feat-mcp-workflow-tools`, `feat-mcp-agent-tools`,
`feat-mcp-ticket-tools`, `feat-mcp-prompt-tools`, `feat-mcp-memory-tools`,
`feat-mcp-scoring-tools`, `feat-mcp-cron-tools`, `feat-mcp-sql-tools`

---

## Status Snapshot

The `eng-mcp-renderer-scaffolding` ticket is **complete**. The full Smithers MCP renderer
lives in a single file, `internal/ui/chat/smithers_mcp.go`, and handles every tool in
scope for these 12 tickets. The routing in `internal/ui/chat/tools.go` already
dispatches `mcp_smithers_*` tool calls through `IsSmithersToolCall` to
`NewSmithersToolMessageItem`. The MCP server auto-discovery (default config, `smithers
--mcp` command) is in place via `internal/config/defaults.go` and `internal/config/load.go`.

**What is done:**
- All renderers wired and registered
- All underlying `smithers.Client` methods exist for every tool group
- MCP prefix routing is live: `mcp_smithers_<tool>` → `SmithersToolRenderContext.renderBody`
- Human-readable label map (`smithersToolLabels`) and primary-key map
  (`smithersPrimaryKeys`) cover the expected tool set
- Per-tool body renderers: tables (runs, workflows, crons, scores, memory, SQL),
  action cards (approve, deny, cancel, hijack, workflow_up, workflow_run, fork, replay,
  revert), inspect tree, plain text (chat, logs), and a JSON/markdown/text fallback

**What is genuinely missing or deferred:**

| Gap | Ticket(s) | Notes |
|---|---|---|
| `diff` renderer uses fallback | `feat-mcp-time-travel-tools` | TODO comment in `renderDiffFallback`; blocked on confirmed response shape |
| `agent_list` / `agent_chat` tools have no entries in `smithersToolLabels` or `smithersPrimaryKeys` | `feat-mcp-agent-tools` | No case in `renderBody` switch; falls to JSON fallback |
| `ticket_list` / `ticket_create` / `ticket_update` / `ticket_search` / `ticket_delete` have no renderer entries | `feat-mcp-ticket-tools` | Falls to JSON fallback |
| `prompt_list` / `prompt_render` / `prompt_update` have no renderer entries | `feat-mcp-prompt-tools` | Falls to JSON fallback |
| `workflow_doctor` has no renderer | `feat-mcp-workflow-tools` | Not in label map or switch; falls to JSON fallback |
| `cron_add` / `cron_rm` / `cron_toggle` have no renderer entries | `feat-mcp-cron-tools` | Mutations; should get action cards |
| `timeline` tool has no entry | `feat-mcp-time-travel-tools` | Not in label map; falls to fallback |
| `revert` is in the label map and the `fork/replay/revert` case, but not in `smithersPrimaryKeys` | `feat-mcp-time-travel-tools` | Minor: no main-param for header; low impact |

---

## Architecture

### Routing

```
tools.go NewToolMessageItem()
  └── IsSmithersToolCall(name)          // strings.HasPrefix(name, "mcp_smithers_")
       └── NewSmithersToolMessageItem()
            └── SmithersToolRenderContext.RenderTool()
                 └── renderBody(tool)   // bare tool name after stripping prefix
```

The bare tool name is the portion after `mcp_smithers_`, e.g. `mcp_smithers_runs_list`
→ `runs_list`.

### Client Layer

All tool groups have corresponding `smithers.Client` methods:

| Tool Group | Client Methods |
|---|---|
| Runs | `ListRuns`, `GetRunSummary`, `InspectRun`, `CancelRun` |
| Observability | `StreamChat`, `StreamRunEvents`, `InspectRun` |
| Control | `ApproveNode`, `DenyNode`, `HijackRun`, `CancelRun` |
| Time-Travel | `ListSnapshots`, `DiffSnapshots`, `ForkRun`, `ReplayRun` |
| Workflows | `ListWorkflows`, `RunWorkflow`, `GetWorkflowDAG` |
| Agents | `ListAgents` |
| Tickets | `GetTicket`, `CreateTicket`, `UpdateTicket`, `DeleteTicket`, `SearchTickets` |
| Prompts | `ListPrompts`, `GetPrompt`, `UpdatePrompt`, `PreviewPrompt` |
| Memory | `ListMemoryFacts`, `ListAllMemoryFacts`, `RecallMemory` |
| Scoring | `GetScores` |
| Crons | `ListCrons` |
| SQL | `ExecuteSQL` |

The MCP tool-call path bypasses the client; the Smithers MCP server returns JSON
directly as `ToolResult.Content`. Renderers parse that content directly.

---

## Implementation Plan

The work is incremental. Most gaps are small additions to `smithers_mcp.go`.

### Phase 1 — Fill Missing Renderer Cases (all 12 tickets)

#### 1a. Agent Tools (`feat-mcp-agent-tools`)

**Add to `smithersToolLabels`:**
```go
"agent_list": "Agent List",
"agent_chat": "Agent Chat",
```

**Add to `smithersPrimaryKeys`:**
```go
"agent_chat": "agentId",
```

**Add type:**
```go
type AgentEntry struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Available bool   `json:"available"`
    Roles     []string `json:"roles,omitempty"`
}
```

**Add case in `renderBody`:**
```go
case "agent_list":
    return s.renderAgentTable(sty, opts, bodyWidth)
case "agent_chat":
    return sty.Tool.Body.Render(
        toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
```

**Add `renderAgentTable`:** Columns: Name, Available (styled yes/no like cron_list),
Roles. Falls back to `renderFallback` on parse error.

#### 1b. Ticket Tools (`feat-mcp-ticket-tools`)

**Add to `smithersToolLabels`:**
```go
"ticket_list":   "Ticket List",
"ticket_create": "Create Ticket",
"ticket_update": "Update Ticket",
"ticket_delete": "Delete Ticket",
"ticket_search": "Search Tickets",
"ticket_get":    "Get Ticket",
```

**Add to `smithersPrimaryKeys`:**
```go
"ticket_get":    "ticketId",
"ticket_create": "id",
"ticket_update": "ticketId",
"ticket_delete": "ticketId",
"ticket_search": "query",
```

**Add type:**
```go
type TicketEntry struct {
    ID        string `json:"id"`
    Title     string `json:"title,omitempty"`
    Status    string `json:"status,omitempty"`
    Content   string `json:"content,omitempty"`
    CreatedAt string `json:"createdAt,omitempty"`
}
```

**Add cases in `renderBody`:**
```go
case "ticket_list", "ticket_search":
    return s.renderTicketTable(sty, opts, bodyWidth)
case "ticket_create", "ticket_update", "ticket_delete":
    return s.renderActionCard(sty, opts, bodyWidth, "DONE", sty.Tool.Smithers.CardDone)
case "ticket_get":
    return s.renderFallback(sty, opts, bodyWidth) // markdown content fits fallback
```

**Add `renderTicketTable`:** Columns: ID, Title, Status. Same pattern as existing tables.

#### 1c. Prompt Tools (`feat-mcp-prompt-tools`)

**Add to `smithersToolLabels`:**
```go
"prompt_list":   "Prompt List",
"prompt_get":    "Get Prompt",
"prompt_render": "Render Prompt",
"prompt_update": "Update Prompt",
```

**Add to `smithersPrimaryKeys`:**
```go
"prompt_get":    "promptId",
"prompt_render": "promptId",
"prompt_update": "promptId",
```

**Add type:**
```go
type PromptEntry struct {
    ID        string `json:"id"`
    EntryFile string `json:"entryFile,omitempty"`
}
```

**Add cases in `renderBody`:**
```go
case "prompt_list":
    return s.renderPromptTable(sty, opts, bodyWidth)
case "prompt_render":
    return sty.Tool.Body.Render(
        toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
case "prompt_update":
    return s.renderActionCard(sty, opts, bodyWidth, "UPDATED", sty.Tool.Smithers.CardDone)
```

**Add `renderPromptTable`:** Columns: ID, Entry File. Simple two-column table.

#### 1d. Cron Mutation Tools (`feat-mcp-cron-tools`)

The `cron_list` renderer already exists. Mutation tools need entries.

**Add to `smithersToolLabels`:**
```go
"cron_add":    "Add Cron",
"cron_rm":     "Remove Cron",
"cron_toggle": "Toggle Cron",
```

**Add to `smithersPrimaryKeys`:**
```go
"cron_add":    "workflow",
"cron_rm":     "cronId",
"cron_toggle": "cronId",
```

**Add cases in `renderBody`:**
```go
case "cron_add":
    return s.renderActionCard(sty, opts, bodyWidth, "SCHEDULED", sty.Tool.Smithers.CardStarted)
case "cron_rm":
    return s.renderActionCard(sty, opts, bodyWidth, "REMOVED", sty.Tool.Smithers.CardCanceled)
case "cron_toggle":
    return s.renderActionCard(sty, opts, bodyWidth, "TOGGLED", sty.Tool.Smithers.CardDone)
```

#### 1e. Workflow Doctor (`feat-mcp-workflow-tools`)

**Add to `smithersToolLabels`:**
```go
"workflow_doctor": "Workflow Doctor",
```

**Add type:**
```go
type WorkflowDiagnostic struct {
    Level   string `json:"level"` // "error", "warn", "info"
    Message string `json:"message"`
    File    string `json:"file,omitempty"`
    Line    int    `json:"line,omitempty"`
}
```

**Add case in `renderBody`:**
```go
case "workflow_doctor":
    return s.renderWorkflowDoctorOutput(sty, opts, bodyWidth)
```

**Add `renderWorkflowDoctorOutput`:** Iterate diagnostics array; prefix each line with
a styled level badge (error=red, warn=yellow, info=subtle). Fall back to `renderFallback`
if parsing fails.

#### 1f. Time-Travel: Diff Renderer (`feat-mcp-time-travel-tools`)

The `renderDiffFallback` currently uses the JSON fallback. Once the diff response shape
is confirmed, replace with a proper renderer.

**Planned response shape** (based on `types_timetravel.go` `SnapshotDiff`):
```go
type SnapshotDiff struct {
    FromID  string      `json:"fromId"`
    ToID    string      `json:"toId"`
    Changes []DiffEntry `json:"changes"`
}
type DiffEntry struct {
    Path   string `json:"path"`
    Before any    `json:"before,omitempty"`
    After  any    `json:"after,omitempty"`
    Op     string `json:"op"` // "add","remove","change"
}
```

**Replace `renderDiffFallback` body:**
```go
func (s *SmithersToolRenderContext) renderDiffFallback(
    sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
    var diff SnapshotDiff
    if err := json.Unmarshal([]byte(opts.Result.Content), &diff); err != nil || len(diff.Changes) == 0 {
        return s.renderFallback(sty, opts, width)
    }
    // Render each change as "op  path  before → after"
    var lines []string
    for _, ch := range diff.Changes {
        op := s.styleOp(sty, ch.Op)
        before := fmt.Sprintf("%v", ch.Before)
        after  := fmt.Sprintf("%v", ch.After)
        lines = append(lines, fmt.Sprintf("%s  %s  %s → %s", op, sty.Base.Render(ch.Path), sty.Subtle.Render(before), sty.Base.Render(after)))
    }
    return sty.Tool.Body.Render(strings.Join(lines, "\n"))
}
```

#### 1g. Time-Travel: `timeline` Tool (`feat-mcp-time-travel-tools`)

**Add to `smithersToolLabels`:**
```go
"timeline": "Timeline",
```

**Add to `smithersPrimaryKeys`:**
```go
"timeline": "runId",
```

**Add type:**
```go
type SnapshotSummary struct {
    ID         string `json:"id"`
    SnapshotNo int    `json:"snapshotNo"`
    Label      string `json:"label,omitempty"`
    NodeID     string `json:"nodeId,omitempty"`
    CreatedAt  string `json:"createdAt,omitempty"`
}
```

**Add case in `renderBody`:**
```go
case "timeline":
    return s.renderTimelineTable(sty, opts, bodyWidth)
```

**Add `renderTimelineTable`:** Columns: No., Node, Label, Created At. Sorted ascending
by snapshot number. Falls back to `renderFallback`.

#### 1h. Minor: `revert` primary key (`feat-mcp-time-travel-tools`)

`revert` is in the switch case alongside `fork` and `replay`, but has no entry in
`smithersPrimaryKeys`. Add:
```go
"revert": "runId",
```

---

### Phase 2 — Verify End-to-End per Tool Group

For each of the 12 tickets, the E2E path is:

1. Smithers MCP server (`smithers --mcp`) exposes the tool
2. Agent invokes `mcp_smithers_<tool>` with appropriate input
3. MCP client delivers result as `ToolResult.Content` (JSON string)
4. `IsSmithersToolCall` routes to `NewSmithersToolMessageItem`
5. `renderBody` dispatches to the correct renderer
6. Renderer parses content; falls back to `renderFallback` on shape mismatch

No additional wiring changes are needed beyond Phase 1. The registration path in
`tools.go` is already complete.

**Verification checklist per group:**

| Ticket | Tool(s) | Renderer Status | E2E Blocker |
|---|---|---|---|
| `feat-mcp-runs-tools` | `runs_list`, `cancel`, `workflow_up` (as start) | Done | None |
| `feat-mcp-observability-tools` | `inspect`, `logs`, `chat` | Done | None |
| `feat-mcp-control-tools` | `approve`, `deny`, `hijack`, `cancel` | Done | None |
| `feat-mcp-time-travel-tools` | `diff`, `fork`, `replay`, `revert`, `timeline` | Partial (see Phase 1e-h) | `diff` shape TBD |
| `feat-mcp-workflow-tools` | `workflow_list`, `workflow_run`, `workflow_up`, `workflow_doctor` | Partial (doctor missing) | None after Phase 1d |
| `feat-mcp-agent-tools` | `agent_list`, `agent_chat` | Missing (Phase 1a) | None |
| `feat-mcp-ticket-tools` | `ticket_list`, `ticket_search`, `ticket_create`, `ticket_update`, `ticket_delete`, `ticket_get` | Missing (Phase 1b) | None |
| `feat-mcp-prompt-tools` | `prompt_list`, `prompt_get`, `prompt_render`, `prompt_update` | Missing (Phase 1c) | None |
| `feat-mcp-memory-tools` | `memory_list`, `memory_recall` | Done | None |
| `feat-mcp-scoring-tools` | `scores` | Done | None |
| `feat-mcp-cron-tools` | `cron_list`, `cron_add`, `cron_rm`, `cron_toggle` | Partial (mutations missing, Phase 1d) | None |
| `feat-mcp-sql-tools` | `sql` | Done | None |

---

### Phase 3 — Tests

All new renderer cases should follow the pattern in `smithers_mcp_test.go`:
inject a `ToolResult.Content` JSON string, call `RenderTool`, assert the output
contains expected strings (tool label, column headers, key values).

**Files to update:** `internal/ui/chat/smithers_mcp_test.go`

**Test cases needed for each Phase 1 addition:**
- Happy path: valid JSON produces table/card/text output
- Empty result: no-rows case returns "No X found." message
- Malformed JSON: falls back to renderFallback (pretty JSON block)
- Envelope shape `{"data": [...]}`: dual-unmarshal path works (tables only)

---

## File Plan

All changes are confined to one file:

**`internal/ui/chat/smithers_mcp.go`** — the only file requiring edits:
- Add labels and primary keys for: `agent_list`, `agent_chat`, `ticket_*`,
  `prompt_*`, `cron_add/rm/toggle`, `workflow_doctor`, `timeline`
- Add `revert` to `smithersPrimaryKeys`
- Add type structs: `AgentEntry`, `TicketEntry`, `PromptEntry`,
  `WorkflowDiagnostic`, `SnapshotSummary`, `DiffEntry` (if not already in
  `types_timetravel.go`)
- Add renderer methods: `renderAgentTable`, `renderTicketTable`,
  `renderPromptTable`, `renderWorkflowDoctorOutput`, `renderTimelineTable`
- Extend `renderBody` switch with all new cases
- Replace `renderDiffFallback` body when diff shape is confirmed
- Add `cron_add/rm/toggle` action card cases

**`internal/ui/chat/smithers_mcp_test.go`** — test coverage for each new case

No changes are needed to:
- `internal/ui/chat/tools.go` (routing already wired)
- `internal/smithers/` (all client methods exist)
- `internal/config/` (MCP server discovery already done)
- Any other package

---

## Dependency Notes

- `feat-mcp-tool-discovery` is already complete; its deliverables (default config,
  MCP server auto-start, prefix routing) are prerequisites for all 12 tickets and
  are shipped.
- `eng-mcp-renderer-scaffolding` is already complete; the base types, styles, and
  dispatch logic are in place.
- `eng-mcp-integration-tests` depends on `feat-mcp-runs-tools` and
  `feat-mcp-control-tools`, both of which are already rendering correctly.

---

## Open Questions

1. **`diff` response shape**: The `renderDiffFallback` has a TODO noting the shape
   must be confirmed by `feat-mcp-tool-discovery`. Once the Smithers MCP server
   returns a concrete diff payload, `renderDiffFallback` should be replaced. The
   planned `SnapshotDiff` shape (see Phase 1f) is consistent with
   `internal/smithers/types_timetravel.go` but the MCP tool may differ from the
   direct API response.

2. **`agent_chat` tool semantics**: The ticket says this tool "prompts the user about
   native TUI handoff". If the tool result is a confirmation JSON rather than prose,
   it should use a card renderer. If it streams conversational content, plain text
   is correct. Confirm behavior before implementing.

3. **Smithers MCP tool names vs ticket assumptions**: The ticket descriptions reference
   tool names like `smithers_ps`, `smithers_up`, `smithers_down`, and `smithers_agent_list`.
   The actual MCP-exposed tool names (after the `mcp_smithers_` prefix) may differ
   (e.g., `runs_list` not `ps`). These should be validated against a live
   `smithers --mcp` tool listing before Phase 1.

4. **`workflow_doctor` response shape**: The ticket describes ESLint/Go-diagnostics-style
   output. Confirm whether the tool returns a JSON array of diagnostics or prose text.

5. **`cron_add/rm/toggle` response shapes**: Confirm these return
   `{"success": true, ...}` (compatible with `ActionConfirmation`) or a different shape.
