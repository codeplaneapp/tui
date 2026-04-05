## Existing Crush Surface

### View system patterns
- The `views.View` interface is defined at [internal/ui/views/router.go:6-12](/Users/williamcory/crush/internal/ui/views/router.go#L6) and requires `Init() tea.Cmd`, `Update(msg tea.Msg) (View, tea.Cmd)`, `View() string`, `Name() string`, `ShortHelp() []string`.
- The `Router` is a slice-backed view stack at [router.go:18-54](/Users/williamcory/crush/internal/ui/views/router.go#L18). `Push` calls `v.Init()` and appends; `Pop` drops the tail; `Current` reads the tail; all navigation is O(1).
- All three concrete Smithers views follow an identical structural pattern: `<domain>LoadedMsg`, `<domain>ErrorMsg`, a struct with `client`, `loading`, `err`, `cursor`, `width`, `height` fields, and `Init`/`Update`/`View`/`Name`/`ShortHelp` methods. Reference implementations: [agents.go](/Users/williamcory/crush/internal/ui/views/agents.go), [tickets.go](/Users/williamcory/crush/internal/ui/views/tickets.go), [approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go).
- `PopViewMsg{}` returned from `Update` is handled by `ui.go` around line 1474 to pop the stack and restore chat/landing state.
- Navigation wiring: a `dialog.ActionOpen<View>` struct is sent from the command palette, caught in `ui.go`'s dialog-action switch, which creates the view and calls `m.viewRouter.Push`. The keybinding path does the same without going through the palette.

### Existing AgentsView anatomy (canonical reference for RunsView)
- `AgentsView` at [agents.go:26](/Users/williamcory/crush/internal/ui/views/agents.go#L26): client pointer, `[]smithers.Agent` slice, cursor int, width/height ints, loading bool, err error.
- `Init()` at [agents.go:45](/Users/williamcory/crush/internal/ui/views/agents.go#L45): returns a `tea.Cmd` closure that calls `client.ListAgents`, dispatches `agentsLoadedMsg` or `agentsErrorMsg`.
- `Update()` at [agents.go:56](/Users/williamcory/crush/internal/ui/views/agents.go#L56): handles loaded/error msgs, `tea.WindowSizeMsg`, and key bindings (`esc`→pop, `up`/`k`, `down`/`j`, `r`→reload, `enter`→no-op stub).
- `View()` at [agents.go:99](/Users/williamcory/crush/internal/ui/views/agents.go#L99): header with right-justified `[Esc] Back`, loading/error/empty guards, then range-over-items loop.
- Header pattern (width-aware right-justification): `gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2; if gap > 0 { headerLine = header + strings.Repeat(" ", gap) + helpHint }`.

### ApprovalsView split-pane (relevant for future inline detail expansion)
- `ApprovalsView` at [approvals.go:28](/Users/williamcory/crush/internal/ui/views/approvals.go#L28): adds a split-pane with `renderList` / `renderDetail` when `v.width >= 80`; collapses to `renderListCompact` on narrow terminals.
- `padRight` helper at [approvals.go:335](/Users/williamcory/crush/internal/ui/views/approvals.go#L335) and `formatPayload` at [approvals.go:366](/Users/williamcory/crush/internal/ui/views/approvals.go#L366) are in the same file and can be shared via the `views` package.
- Section grouping pattern: `ApprovalsView.renderList` partitions items into pending/resolved sections with bold-faint section headers.

### UI wiring
- `ActionOpenAgentsView`, `ActionOpenTicketsView`, `ActionOpenApprovalsView` defined together in [actions.go:91-96](/Users/williamcory/crush/internal/ui/dialog/actions.go#L91).
- Command palette entries for Agents/Approvals/Tickets are added at [commands.go:528-532](/Users/williamcory/crush/internal/ui/dialog/commands.go#L528) via `NewCommandItem`.
- Dialog-action switch in `ui.go` at [ui.go:1453-1472](/Users/williamcory/crush/internal/ui/model/ui.go#L1453): each case closes the palette, constructs the view, pushes it, calls `m.setState(uiSmithersView, uiFocusMain)`.
- `AttachmentDeleteMode` uses `ctrl+shift+r` at [keys.go:146](/Users/williamcory/crush/internal/ui/model/keys.go#L146); `ctrl+r` is currently unbound in the keymap file. The engineering spec for this ticket claims `ctrl+r` is "attachment-delete mode" but the actual binding in keys.go is `ctrl+shift+r`. `ctrl+r` is therefore free for the runs keybinding.

### Smithers client transport tiers
- `Client` struct at [client.go:59](/Users/williamcory/crush/internal/smithers/client.go#L59): `apiURL`, `apiToken`, `dbPath`, `db *sql.DB`, `httpClient`, `execFunc`.
- Transport decision pattern: `isServerAvailable()` cached for 30s at [client.go:130](/Users/williamcory/crush/internal/smithers/client.go#L130); try HTTP → SQLite → exec in that order.
- `httpGetJSON` at [client.go:174](/Users/williamcory/crush/internal/smithers/client.go#L174): decodes into `apiEnvelope{OK, Data, Error}`. The upstream `/v1/runs` endpoint returns **direct JSON arrays** without this envelope (confirmed in `eng-smithers-client-runs` research). The `eng-smithers-client-runs` ticket must land a `httpGetDirect[T]()` helper before `ListRuns` can call the real API; a stub method returning `ErrNoTransport` is the in-progress bridge.
- `execSmithers` at [client.go:248](/Users/williamcory/crush/internal/smithers/client.go#L248): shells out to `smithers` binary. The `smithers ps --json` fallback path will work for `ListRuns` if the binary is on PATH.

### Types gap
- `internal/smithers/types.go` at [types.go:1](/Users/williamcory/crush/internal/smithers/types.go#L1) has: `Agent`, `SQLResult`, `ScoreRow`, `AggregateScore`, `MemoryFact`, `MemoryRecallResult`, `Ticket`, `Approval`, `CronSchedule`.
- No `Run`, `RunDetail`, `RunNode`, `RunStatus`, `RunFilter`, `SmithersEvent` types exist. These must be added by `eng-smithers-client-runs` before `RunsView` can compile against `client.ListRuns`.

### VHS test infrastructure
- Existing VHS tapes are in [tests/vhs/](/Users/williamcory/crush/tests/vhs/): `smithers-domain-system-prompt.tape`, `branding-status.tape`, `helpbar-shortcuts.tape`.
- Tape syntax: `Output`, `Set`, `Type`, `Enter`, `Ctrl+R`, `Sleep`, `Screenshot`, `Down`, `Up`, `Escape`. Output PNGs go to `tests/vhs/output/`.
- Fixtures are in [tests/vhs/fixtures/crush.json](/Users/williamcory/crush/tests/vhs/fixtures/crush.json) with `CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures` env var.
- No Go E2E PTY test harness exists yet (the `eng-smithers-client-runs` plan created a path for it at `tests/tui/`).

---

## Upstream Smithers Reference

### API contract
- `GET /v1/runs` at `smithers/src/server/index.ts:1029`: returns a direct JSON array of run objects. Query params include `status`, `workflowPath`, `limit`, `offset`.
- `GET /v1/runs/:id` at `index.ts:897`: returns a single run summary object (not full DAG — nodes require SQLite or `smithers inspect`).
- `GET /v1/runs/:id/events?afterSeq=-1` at `index.ts:819`: SSE stream, `event: smithers`, `data: <json>`, heartbeat `: keep-alive`. Relevant for follow-on `RUNS_REALTIME_STATUS_UPDATES` ticket; out of scope here.
- `POST /v1/runs/:id/cancel` at `index.ts:755`, `POST /v1/runs/:id/approve` at `index.ts:973`, `POST /v1/runs/:id/deny` at `index.ts:1001`: mutation endpoints, out of scope for base dashboard but required by `RUNS_QUICK_*` tickets.

### Run data shape (from upstream source)
- `RunStatus` enum values from `smithers/src/RunStatus.ts`: `running`, `waiting-approval`, `finished`, `failed`, `cancelled`, `paused`.
- Run object fields: `runId` (string), `workflowPath` (string), `workflowName` (string, derived from path), `status` (RunStatus), `startedAtMs` (number), `finishedAtMs` (number|null), `summary` (object with node completion counts), `agentId` (string|null), `input` (object|null).
- `summary` object shape: `{ completed: number, failed: number, cancelled: number, total: number }`. Progress ratio = `(completed + failed) / total`.
- `SmithersEvent` types from `SmithersEvent.ts:4`: `RunStarted`, `RunFinished`, `RunFailed`, `RunCancelled`, `NodeStarted`, `NodeFinished`, `NodeFailed`, `ApprovalRequested`, `ApprovalResolved`, `RunHijackRequested`, `RunHijacked`.

### Prior TUI run list implementations
- TypeScript TUI v1 `RunsList.tsx` at `smithers/src/cli/tui/components/RunsList.tsx:31`: polled runs on an interval, maintained local state for filter/selected run/action.
- TUI v2 `SmithersService.ts:71`: queried DB directly for runs (not HTTP). `Broker.ts:184` used `program.Send` pattern for emitting updates — the same pattern available in `app.go:549` in Crush.
- `RunDetailView.tsx:76`: hijack was triggered by keypress `h`, called `smithers hijack <runId>` via exec.

### Exec fallback
- `smithers ps --json` is the CLI fallback for listing runs when no HTTP server is available. The output format may differ from `/v1/runs` JSON — the implementation should normalize both via a common `parseRunsJSON` helper.
- `smithers ps` is the primary status command; `smithers inspect <runId>` gives full node detail.

---

## Data Flow: Smithers API → client → view model → render

```
User navigates to runs view (Ctrl+R or palette "Runs")
  │
  ▼
ui.go handles ActionOpenRunsView / Ctrl+R keypress
  │  creates RunsView, calls m.viewRouter.Push(runsView)
  │  Push calls runsView.Init()
  ▼
RunsView.Init() returns a tea.Cmd closure
  │
  ▼  (goroutine: Bubble Tea runs the Cmd)
smithers.Client.ListRuns(ctx, RunFilter{Limit: 50})
  │
  ├─[1] HTTP available? → GET /v1/runs?limit=50
  │     Response: direct JSON array → httpGetDirect[[]Run]()
  │
  ├─[2] SQLite available? → SELECT ... FROM _smithers_runs ORDER BY started_at_ms DESC LIMIT 50
  │
  └─[3] Exec fallback? → exec("smithers", "ps", "--json") → parseRunsJSON(stdout)
  │
  ▼  (returns to Bubble Tea event loop via channel)
runsLoadedMsg{runs: []smithers.Run}  or  runsErrorMsg{err: ...}
  │
  ▼
RunsView.Update(runsLoadedMsg)
  │  stores runs, sets loading=false, returns (v, nil)
  ▼
RunsView.View()
  │  delegates to components.RunTable{Runs, Cursor, Width}.View()
  ▼
RunTable.View()
  │  renders header row + per-run rows
  │  applies lipgloss status colors
  │  computes progress ratio from Run.Summary
  │  computes elapsed time from time.UnixMilli(Run.StartedAtMs)
  ▼
terminal output: ANSI-escaped table string
```

---

## Integration with Smithers Client

The `RunsView` requires `client.ListRuns(ctx context.Context, filter RunFilter) ([]Run, error)`. This method does not yet exist in `internal/smithers/client.go`. The `eng-smithers-client-runs` ticket delivers it.

**Stub bridge for parallel development:**
```go
// In internal/smithers/client.go (temporary until eng-smithers-client-runs lands)
func (c *Client) ListRuns(_ context.Context, _ RunFilter) ([]Run, error) {
    return nil, ErrNoTransport
}
```

`RunsView` already handles the error path gracefully (renders `"  Error: no smithers transport available"`), so the view can be developed and tested against the stub.

**`RunFilter` struct (to be defined in types.go by eng-smithers-client-runs):**
```go
type RunFilter struct {
    Status       string // "" = all, or specific RunStatus value
    WorkflowPath string
    Limit        int
    Offset       int
}
```

**`Run` struct (to be defined in types.go by eng-smithers-client-runs):**
```go
type Run struct {
    RunID        string     `json:"runId"`
    WorkflowPath string     `json:"workflowPath"`
    WorkflowName string     `json:"workflowName"`
    Status       string     `json:"status"`      // RunStatus values
    StartedAtMs  int64      `json:"startedAtMs"`
    FinishedAtMs *int64     `json:"finishedAtMs"`
    Summary      RunSummary `json:"summary"`
    AgentID      *string    `json:"agentId"`
    Input        *string    `json:"input"` // JSON-encoded input object
}

type RunSummary struct {
    Completed int `json:"completed"`
    Failed    int `json:"failed"`
    Cancelled int `json:"cancelled"`
    Total     int `json:"total"`
}
```

---

## Bubble Tea v2 Component Analysis

### Why a custom RunTable rather than bubbles/list or bubbles/table

Crush uses Bubble Tea v2 (`charm.land/bubbletea/v2`) and the companion `charm.land/bubbles/v2` component library. The runs view requires:

1. **Multi-column tabular layout** with variable-width columns and color-coded status cells — the `bubbles/v2/table` component supports this but uses its own model/message system that conflicts with the view's existing Update pattern.
2. **Tight control over rendering** — the design doc shows progress bars (out of scope for base view) and inline expansion (out of scope for base view), which require custom rendering anyway.
3. **Consistency with existing views** — `AgentsView`, `TicketsView`, and `ApprovalsView` all use manual `strings.Builder` rendering, not bubbles components. Adopting `bubbles/table` for runs but not the others creates inconsistency.

The `components.RunTable` struct (stateless, renders to string) follows the established `ApprovalsView.renderList` pattern and can be dropped into any view or chat tool renderer without carrying Bubble Tea model state.

### lipgloss styling for status colors

```go
// RunTable status color map (mirrors GUI badge colors from RunsList.tsx)
var statusStyles = map[string]lipgloss.Style{
    "running":          lipgloss.NewStyle().Foreground(lipgloss.Color("2")),   // green
    "waiting-approval": lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true), // yellow bold
    "finished":         lipgloss.NewStyle().Faint(true),
    "failed":           lipgloss.NewStyle().Foreground(lipgloss.Color("1")),   // red
    "cancelled":        lipgloss.NewStyle().Faint(true).Strikethrough(true),
    "paused":           lipgloss.NewStyle().Foreground(lipgloss.Color("4")),   // blue
}
```

### Width calculation and graceful degradation
- `lipgloss.Width(s)` returns the rendered display width of a string (handles ANSI), used in header right-justification.
- Column hiding below terminal width thresholds (matching `sidebarCompactModeBreakpoint` pattern in `commands.go:30`):
  - `< 80` cols: hide Progress and Time columns
  - `< 60` cols: also truncate Workflow column to available space with `...`

---

## Identified Risks

### Risk 1: eng-smithers-client-runs dependency
**Impact**: `RunsView.Init()` calls `client.ListRuns()` which does not yet exist. The view cannot compile against the real method until `eng-smithers-client-runs` lands.

**Mitigation**: Add a stub `ListRuns` returning `ErrNoTransport` to unblock parallel development. The `RunsView` error path renders a graceful error message rather than panicking. Replace stub with real implementation when the dependency lands.

**Severity**: Medium — development is blocked only if both tickets are being implemented by the same developer in the same branch.

### Risk 2: Transport envelope mismatch
**Impact**: `httpGetJSON` at [client.go:174](/Users/williamcory/crush/internal/smithers/client.go#L174) assumes `{ok, data, error}` envelope. The upstream `/v1/runs` API returns a direct JSON array. Calling `httpGetJSON` against `/v1/runs` will produce a decode error on every request.

**Mitigation**: `eng-smithers-client-runs` adds `httpGetDirect[T]()`. The runs dashboard must be tested against a mock that returns direct JSON (not envelope-wrapped), so the test will fail if the wrong helper is used — catching this at test time rather than production.

**Severity**: High if eng-smithers-client-runs is not already landed; Low otherwise.

### Risk 3: Ctrl+R keybinding
**Impact**: The design doc specifies `Ctrl+R` for the runs view. `eng-smithers-client-runs` research claimed this conflicted with attachment-delete mode, but inspection of [keys.go:146](/Users/williamcory/crush/internal/ui/model/keys.go#L146) shows the actual binding is `ctrl+shift+r`, not `ctrl+r`. However, `ctrl+r` may be used elsewhere in `ui.go` for non-keybinding purposes. The full `ui.go` file must be searched for `"ctrl+r"` before assuming it is free.

**Mitigation**: Do a targeted grep for `ctrl+r` usage across `internal/ui/` before adding the binding. If it is truly free, add it. If not, use the command palette as the only entry point for v1.

**Severity**: Low — command palette `/runs` is an adequate fallback.

### Risk 4: No runs data in development
**Impact**: Developers without a running Smithers server cannot see real data. The exec fallback (`smithers ps`) requires the Smithers binary to be installed.

**Mitigation**: The E2E test uses `httptest.Server` with canned JSON, so CI always has data. For manual testing, the `SMITHERS_MOCK=1` env var pattern (noted in engineering spec) can inject fixture runs directly in `ListRuns`.

**Severity**: Low — affects developer experience only.

### Risk 5: Real-time updates expectation mismatch
**Impact**: The PRD says "Real-time updates: Status changes stream in via SSE/polling." The base `runs-dashboard` ticket explicitly excludes SSE (`RUNS_REALTIME_STATUS_UPDATES` is a separate ticket). Users may expect live updates and be confused when the table is static.

**Mitigation**: Add a visible timestamp to the header: `"Last updated: 2s ago"` derived from when `runsLoadedMsg` arrived. The manual `r` refresh key provides an escape valve. Make the static nature clear in help text.

**Severity**: Medium — primarily a UX expectation gap. Resolved by follow-on ticket.

### Risk 6: Large run volume
**Impact**: Organizations with hundreds of runs per day will see a very long list. Without pagination or filtering (both out of scope for base view), the table will overflow the terminal height.

**Mitigation**: Default `RunFilter{Limit: 50}` caps the list. The view should clip the rendered rows to available terminal height (`v.height - headerLines`) with a "... and N more" footer line if runs exceed what fits. This is a rendering constraint, not a data constraint.

**Severity**: Low for v1. Resolved by `RUNS_STATUS_SECTIONING` and `RUNS_FILTER_BY_*` follow-on tickets.

### Risk 7: PTY E2E test flakiness
**Impact**: Terminal E2E tests using PTY output polling are timing-sensitive and may be flaky on CI with CPU contention.

**Mitigation**: Use 15-second per-assertion timeouts (matching upstream `tui.e2e.test.ts`), poll with 100ms intervals, dump the full terminal buffer on timeout for debugging. The `tests/tui/helpers_test.go` harness from `eng-smithers-client-runs` provides these helpers.

**Severity**: Medium — inherent to PTY testing; mitigated by generous timeouts and good failure output.

---

## Files To Touch

- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go) — add `Run`, `RunSummary`, `RunFilter` types (owned by eng-smithers-client-runs; stub needed here if that ticket has not landed)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go) — add `ListRuns` stub or real implementation
- `/Users/williamcory/crush/internal/ui/views/runs.go` (new) — `RunsView` implementing `views.View`
- `/Users/williamcory/crush/internal/ui/components/runtable.go` (new) — stateless `RunTable` renderer
- `/Users/williamcory/crush/internal/ui/components/runtable_test.go` (new) — unit tests for `RunTable`
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go) — add `ActionOpenRunsView struct{}`
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go) — add "Runs" command palette entry
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go) — handle `ActionOpenRunsView`, add `Ctrl+R` keybinding
- `/Users/williamcory/crush/tests/tui/runs_dashboard_e2e_test.go` (new) — PTY E2E test
- `/Users/williamcory/crush/tests/vhs/runs-dashboard.tape` (new) — VHS happy-path recording
