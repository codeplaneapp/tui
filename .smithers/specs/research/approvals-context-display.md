## Existing Crush Surface

### Current `ApprovalsView` detail pane (`internal/ui/views/approvals.go`)

The view already exists with a working split-pane layout: left list pane (fixed 30 cols) with `│` divider, right detail pane taking remaining width. Compact mode (< 80 cols) collapses to inline context below the selected item.

**What `renderDetail()` currently renders** (lines 289–320):

| Field | Source | How rendered |
|---|---|---|
| Gate title | `a.Gate` (fallback: `a.NodeID`) | Bold heading, first line |
| Status | `a.Status` | `formatStatus()` — "● pending", "✓ approved", "✗ denied" |
| Workflow path | `a.WorkflowPath` | Faint label + raw value |
| Run ID | `a.RunID` | Faint label + raw ID |
| Node ID | `a.NodeID` | Faint label + raw ID |
| Payload | `a.Payload` | `formatPayload()` — JSON pretty-print or `wrapText()` fallback |

**What `renderListItem()` currently renders** (lines 215–241):

- Cursor indicator (`▸` or spaces)
- Status icon (`○`, `✓`, `✗`)
- Gate label or NodeID (truncated to `width-4`)

**What `renderListCompact()` currently renders when cursor is on item** (lines 244–286):

- Same cursor + icon + label as list
- Below selected: `Workflow: <path>`, `Run: <runID>`, first 60 chars of payload (raw, not parsed)

**Gaps in current rendering**:

1. **No wait time** — `a.RequestedAt` (Unix ms int64) is present in the struct but never displayed anywhere. No elapsed time, no SLA color-coding.
2. **No step progress** — The detail pane has no "Step N of M" information because node count data (`NodeTotal`, `NodesDone`) is not part of the `Approval` struct and would require a separate API fetch.
3. **No run status** — `a.WorkflowPath` and `a.RunID` are shown but the run's current execution status ("running", "paused", "completed") is absent. A user can't tell if the run is still active without leaving the view.
4. **No workflow name** — Only the raw `WorkflowPath` (e.g., `.smithers/workflows/deploy.ts`) is shown; no human-readable `WorkflowName` derived from the basename.
5. **No resolution metadata** — `a.ResolvedAt` and `a.ResolvedBy` are in the `Approval` struct (both nullable `*int64` / `*string`) but `renderDetail()` never reads them. Decided approvals show the same layout as pending ones.
6. **No started-at/elapsed for the run** — Run start time and overall elapsed is not shown. Users can see the approval wait time once that's added, but not the total run duration.
7. **Payload not truncated to terminal height** — `formatPayload()` renders the entire payload with no line cap. A large JSON object (e.g., 200+ lines) pushes the gate header off-screen. `formatPayload()` calls `wrapText()` which also has no height limit.
8. **Compact mode payload is raw** — In `renderListCompact()`, the inline payload preview calls `truncate(a.Payload, 60)` on the raw JSON string rather than parsing it first, producing unreadable truncated JSON.
9. **No "Loading..." state for the context pane** — The detail pane is purely synchronous. There is no mechanism to show a loading indicator while enriched run data is being fetched.
10. **No per-approval dynamic update** — The view fetches all approvals once on `Init()` and does not refresh enriched context when the cursor moves. Every approval shows the same static fields regardless of cursor position.

---

### `Approval` struct (`internal/smithers/types.go`, lines 83–96)

```go
type Approval struct {
    ID           string  `json:"id"`
    RunID        string  `json:"runId"`
    NodeID       string  `json:"nodeId"`
    WorkflowPath string  `json:"workflowPath"`
    Gate         string  `json:"gate"`         // The question or gate name
    Status       string  `json:"status"`       // "pending" | "approved" | "denied"
    Payload      string  `json:"payload"`      // JSON payload with task inputs/context
    RequestedAt  int64   `json:"requestedAt"`  // Unix ms
    ResolvedAt   *int64  `json:"resolvedAt"`   // Unix ms, nil if pending
    ResolvedBy   *string `json:"resolvedBy"`   // Who resolved, nil if pending
}
```

**Fields available but not displayed**:
- `RequestedAt` — present, not rendered (enables wait time + SLA coloring)
- `ResolvedAt` — present, not rendered (enables "resolved N min ago")
- `ResolvedBy` — present, not rendered (enables "Resolved by: <user/agent>")

**Fields absent from the struct** (require a separate `RunSummary` fetch):
- `WorkflowName` (human-readable) — derivable from `WorkflowPath` via `path.Base()`
- `RunStatus` — whether the run is "running", "paused", "completed", "failed"
- `NodeTotal` / `NodesDone` — step progress (requires node count from DB or API)
- `StartedAtMs` — run start epoch (enables "started N min ago" in detail)
- `ElapsedMs` — total run duration so far

---

### Client transport (`internal/smithers/client.go`, lines 268–296)

`ListPendingApprovals` follows the standard three-tier pattern: HTTP GET `/approval/list` → direct SQLite → exec fallback. The SQLite scan query fetches all 10 `Approval` fields including `requested_at` and `resolved_at`.

**No `GetRunSummary` method exists.** The client has no per-run fetch capability. Existing methods that could serve as models:
- `GetScores()` — SQLite primary, exec fallback, keyed by `runID`
- `ListPendingApprovals()` — HTTP primary, SQLite, exec; uses `scanApprovals`
- `ExecuteSQL()` — HTTP primary, SQLite (SELECT-only guard), exec fallback

The cache pattern used by `isServerAvailable()` (`sync.RWMutex` + `serverChecked time.Time`) is the only existing example of a TTL cache on the client. There is no generic per-ID result cache; one must be added for `RunSummary`.

**`_smithers_runs` and `_smithers_nodes` tables** are referenced in the engineering spec's SQLite query but are not yet accessed by any Go client method. The schema (from upstream `src/db/internal-schema.ts`) includes:
- `_smithers_runs`: `id`, `workflow_path`, `status`, `started_at` (at minimum — full schema is in upstream TS)
- `_smithers_nodes`: `run_id`, `node_id`, `status`

The engineering spec's SQLite query counts `_smithers_nodes` using two subqueries — one for `nodeTotal` (all nodes for the run) and one for `nodesDone` (nodes with `status IN ('completed', 'failed')`). This may undercount `nodeTotal` if nodes are only inserted upon execution (lazy init). The exec fallback via `smithers inspect <runID>` provides the full DAG including unexecuted nodes and is the canonical source for accurate counts.

---

### HTTP API endpoint availability

The upstream `src/server/index.ts` exposes:
- `GET /approval/list` and `/v1/approval/list` — approval list (used by existing transport)
- `GET /v1/runs/:id/events` — SSE stream (out of scope for this ticket)
- `POST /v1/runs/:id/nodes/:nodeId/approve|deny` — approve/deny mutations (out of scope)

**Per-run fetch endpoint**: The engineering spec references `GET /v1/runs/{runID}`. This path is NOT confirmed in the direct Smithers server (`src/server/index.ts`). The server likely exposes `GET /ps` (all runs) but may not have an individual run fetch endpoint. The three-tier transport provides a natural fallback: if `/v1/runs/{runID}` doesn't exist on the server, the SQLite path (`_smithers_runs` JOIN `_smithers_nodes`) or exec path (`smithers inspect <runID>`) handles the request. This is Risk #1 in the engineering spec.

---

### Approval field-name alignment

The engineering spec (Risk #5) confirms that Crush's `Approval` struct uses different field names from the upstream GUI reference code, which used `Label` (not `Gate`), `InputJSON` (not `Payload`), `WaitMinutes` (not `RequestedAt`), `DecidedBy`/`DecidedAt` (not `ResolvedBy`/`ResolvedAt`). The Crush names are intentional and already present in the committed code — the GUI reference names are legacy. All display code should use Crush struct field names.

---

### Design spec target (02-DESIGN.md §3.5)

The wireframe shows a card-based layout where each pending approval card displays:

```
1. Deploy to staging                              8m ago
   Run: def456 (deploy-staging)
   Node: deploy · Step 4 of 6

   Context:
   The deploy workflow has completed build, test, and lint
   steps. All passed. Ready to deploy commit a1b2c3d to
   staging environment.

   Changes: 3 files modified, 47 insertions, 12 deletions

   [a] Approve    [d] Deny    [i] Inspect run
```

The engineering spec maps this wireframe to a split-pane detail pane (right side in wide mode). Key observations:
- "8m ago" = wait time from `RequestedAt`, needs SLA color (<10m green, 10-30m yellow, ≥30m red)
- "Run: def456 (deploy-staging)" = `RunID` + `WorkflowName` (derived)
- "Step 4 of 6" = `NodesDone` / `NodeTotal` from `RunSummary`
- "Context:" paragraph = likely the `Gate` field text or part of `Payload` — needs word-wrap
- "Changes: 3 files..." = key-value data extracted from `Payload` JSON
- `[a] Approve [d] Deny` = inline actions (out of scope for this ticket per spec)

The `Payload` field contains the structured input data for the approval gate. When JSON, it should be pretty-printed with indentation. The design shows selective rendering ("Changes: 3 files...") rather than raw JSON — but for this ticket, full JSON pretty-print with height truncation is the specified approach, not field-level extraction.

---

### Test infrastructure available

- `internal/smithers/client_test.go`: has `newTestServer`, `writeEnvelope`, `newExecClient` helpers. Tests follow `TestExecuteSQL_HTTP` / `TestExecuteSQL_Exec` pattern with `httptest.Server` for HTTP path and `withExecFunc` mock for exec path.
- `internal/e2e/tui_helpers_test.go`: Go E2E harness with `launchTUI`, `WaitForText` (100ms polling, ANSI-stripped), `SendKeys`, `Snapshot`, `Terminate`. Tests are gated by `SMITHERS_TUI_E2E=1` env var.
- `internal/e2e/chat_domain_system_prompt_test.go`: example E2E pattern — mock server started, per-test temp dir, global config written, view navigated, assertions on `WaitForText`.
- `tests/vhs/`: VHS tape directory with `smithers-domain-system-prompt.tape` as reference format.

**No `approvals_test.go` exists yet** under `internal/ui/views/`. No E2E test for approvals view. No VHS tape for approvals context display.

---

## Gaps Summary

| Gap | Severity | How to close |
|---|---|---|
| Wait time not displayed | High — core feature of ticket | Compute `time.Since(time.UnixMilli(a.RequestedAt))` in `renderDetail` and `renderListItem` |
| No SLA color on wait time | High | Add `slaStyle(d time.Duration)` helper: green <10m, yellow 10–30m, red ≥30m |
| Step progress missing | High — shown in design wireframe | Add `GetRunSummary` client method + `RunSummary` type; async-fetch on cursor change |
| Run status not shown | Medium | Available from `RunSummary.Status` once fetched |
| No workflow name (human-readable) | Medium | `path.Base(a.WorkflowPath)` without extension (quick); or `RunSummary.WorkflowName` (accurate) |
| `ResolvedAt`/`ResolvedBy` not rendered | Medium | Add "Resolved by: / Resolved at:" section for non-pending approvals |
| Payload not height-capped | Medium | Count used lines; truncate with "... (N more lines)" |
| Compact mode raw payload preview | Low | Parse JSON before truncating in `renderListCompact` |
| No loading state in detail pane | Medium | Add `contextLoading bool` + `contextErr error` to view state |
| No cursor-driven context update | High — core of the ticket | Emit `tea.Cmd` on cursor move; handle `runSummaryLoadedMsg` / `runSummaryErrorMsg` |
| No `GetRunSummary` on client | High — required | New method + `RunSummary` type + cache |
| No client cache for RunSummary | Medium | `sync.Map` keyed by runID, 30s TTL |
| No view unit tests | High | `internal/ui/views/approvals_test.go` |
| No client unit tests for RunSummary | High | Tests in `internal/smithers/client_test.go` |
| No E2E test | High — per spec | `internal/e2e/approvals_context_display_test.go` |
| No VHS tape | Medium — per spec | `tests/vhs/approvals-context-display.tape` |

---

## Files To Touch

- [`internal/smithers/types.go`](/Users/williamcory/crush/internal/smithers/types.go) — add `RunSummary` struct
- [`internal/smithers/client.go`](/Users/williamcory/crush/internal/smithers/client.go) — add `GetRunSummary`, `ClearRunSummaryCache`, cache fields
- [`internal/smithers/client_test.go`](/Users/williamcory/crush/internal/smithers/client_test.go) — add `TestGetRunSummary_*` tests
- [`internal/ui/views/approvals.go`](/Users/williamcory/crush/internal/ui/views/approvals.go) — rewrite `renderDetail`, update `renderListItem`, `renderListCompact`, add async fetch wiring, new helpers
- [`internal/ui/views/approvals_test.go`](/Users/williamcory/crush/internal/ui/views/approvals_test.go) — new file, 12 test cases per spec
- [`internal/e2e/approvals_context_display_test.go`](/Users/williamcory/crush/internal/e2e/approvals_context_display_test.go) — new E2E test
- [`tests/vhs/approvals-context-display.tape`](/Users/williamcory/crush/tests/vhs/approvals-context-display.tape) — new VHS tape
