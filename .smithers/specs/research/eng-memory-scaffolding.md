## Summary

Research for `eng-memory-scaffolding` — scaffold a read-only `MemoryView` that lists `_smithers_memory_facts` rows, wired to the command palette and view router, following the pattern established by `AgentsView`, `TicketsView`, and `ApprovalsView`.

---

## Existing Client Surface

### Transport tier (`internal/smithers/client.go`)

**`ListMemoryFacts(ctx, namespace, workflowPath string) ([]MemoryFact, error)`** — lines 388–413.

- Primary transport: direct SQLite (`c.db != nil`) with query:
  ```sql
  SELECT namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms, ttl_ms
  FROM _smithers_memory_facts WHERE namespace = ?
  ```
- Fallback transport: `exec smithers memory list <namespace> --format json`, with optional `--workflow <workflowPath>`.
- **Critical gap**: the SQL query has a hard `WHERE namespace = ?` predicate. Passing an empty string `""` as namespace will produce zero rows unless a fact was stored with an empty namespace. There is no "list all namespaces" path. The scaffolding must either: (a) pass a real default namespace, (b) add a `ListAllMemoryFacts` client method that omits the `WHERE` clause, or (c) modify `ListMemoryFacts` to treat `namespace == ""` as "all". Option (b) is the safest — it is non-breaking and mirrors the principle of thin, composable transport wrappers already used throughout the client.

**`RecallMemory(ctx, query string, namespace *string, topK int) ([]MemoryRecallResult, error)`** — lines 415–430.

- Always exec — no HTTP, no SQLite path. This is documented: vector similarity search requires the TypeScript runtime.
- **Impact on scaffolding**: `RecallMemory` cannot be used in the initial scaffold's `Init()` (no guarantee smithers binary is available or a vector index exists). It is the right candidate for the follow-on `feat-memory-semantic-recall` ticket.
- The method signature accepts an optional `*string` namespace and `topK int`. If `topK == 0`, no `--topK` flag is passed (the client code checks `topK > 0`).

**Scanner / parser helpers** — lines 761–794:

- `scanMemoryFacts(rows)` — maps the 7-column SQLite result to `[]MemoryFact`. The scan order exactly matches the SELECT column order.
- `parseMemoryFactsJSON(data)` — unmarshals a `[]MemoryFact` JSON array from exec output.
- `parseRecallResultsJSON(data)` — unmarshals a `[]MemoryRecallResult` JSON array.

### Type surface (`internal/smithers/types.go`)

**`MemoryFact`** — lines 57–67:

```go
type MemoryFact struct {
    Namespace   string `json:"namespace"`
    Key         string `json:"key"`
    ValueJSON   string `json:"valueJson"`
    SchemaSig   string `json:"schemaSig,omitempty"`
    CreatedAtMs int64  `json:"createdAtMs"`
    UpdatedAtMs int64  `json:"updatedAtMs"`
    TTLMs       *int64 `json:"ttlMs,omitempty"`
}
```

All fields needed for the fact list row are present: `Namespace`, `Key`, `ValueJSON`, and `UpdatedAtMs` (relative age). `TTLMs` is optional (pointer). `SchemaSig` is metadata that can be ignored at this scaffolding layer.

**`MemoryRecallResult`** — lines 69–74:

```go
type MemoryRecallResult struct {
    Score    float64     `json:"score"`
    Content  string      `json:"content"`
    Metadata interface{} `json:"metadata"`
}
```

Used only by `RecallMemory` — out of scope for this ticket.

---

## Existing View Patterns

All four concrete Smithers views follow an identical structural pattern:

| File | View | Data method | Msg types |
|------|------|------------|-----------|
| `agents.go` | `AgentsView` | `ListAgents` | `agentsLoadedMsg`, `agentsErrorMsg` |
| `tickets.go` | `TicketsView` | `ListTickets` | `ticketsLoadedMsg`, `ticketsErrorMsg` |
| `approvals.go` | `ApprovalsView` | `ListPendingApprovals` | `approvalsLoadedMsg`, `approvalsErrorMsg` |
| — (new) | `MemoryView` | `ListAllMemoryFacts` | `memoryLoadedMsg`, `memoryErrorMsg` |

Every view:
1. Holds `client *smithers.Client`, `cursor int`, `width/height int`, `loading bool`, `err error`.
2. `Init()` returns a `tea.Cmd` closure that calls the client method and returns a typed loaded/error msg.
3. `Update()` is a switch on msg type: loaded → populate + `loading=false`; error → store + `loading=false`; `tea.WindowSizeMsg` → store dimensions; `tea.KeyPressMsg` → `esc/alt+esc` → `PopViewMsg`, `up/k` → decrement cursor, `down/j` → increment cursor, `r` → set `loading=true` + re-call `Init()`, `enter` → no-op placeholder.
4. `View()` renders: right-aligned `[Esc] Back` header; loading/error/empty sentinel states; then cursor-navigable list with `▸` indicator for selected row, bold name, faint secondary text.
5. `Name()` returns the slug. `ShortHelp()` returns hint strings for the help bar.
6. Compile-time interface check: `var _ View = (*XView)(nil)`.

The `MemoryView` will follow this pattern identically. The only structural addition is a two-line row layout (namespace+key bold on line 1, truncated value preview + relative age faint on line 2) versus the single-line layout used by agents/tickets.

**`ApprovalsView`** adds a split-pane (`renderList` + `renderDetail` + line-by-line join) and the `padRight` helper for fixed-width padding. The memory view does not need split-pane at this scaffolding stage — a single-pane list with inline secondary text is sufficient and matches the ticket's stated scope.

---

## Navigation Plumbing

### Router (`internal/ui/views/router.go`)

`Router.Push(v)` appends to the stack and calls `v.Init()`. `Pop()` removes the top view, protecting the last entry. `PopViewMsg` triggers the pop from `ui.go`. No changes needed here.

### Action type (`internal/ui/dialog/actions.go` line 96)

Existing Smithers actions block:
```go
ActionOpenAgentsView    struct{}
ActionOpenTicketsView   struct{}
ActionOpenApprovalsView struct{}
```
Add `ActionOpenMemoryView struct{}` after `ActionOpenApprovalsView`.

### Command palette (`internal/ui/dialog/commands.go` line 528)

Current Smithers entries:
```go
NewCommandItem(c.com.Styles, "agents",    "Agents",    "",       ActionOpenAgentsView{}),
NewCommandItem(c.com.Styles, "approvals", "Approvals", "",       ActionOpenApprovalsView{}),
NewCommandItem(c.com.Styles, "tickets",   "Tickets",   "",       ActionOpenTicketsView{}),
NewCommandItem(c.com.Styles, "quit",      "Quit",      "ctrl+c", tea.QuitMsg{}),
```
Add `memory` entry before `quit`. No dedicated keybinding is assigned for this ticket — the PRD lists `/memory` as a "detail view" reached from the command palette, not a top-level shortcut.

### Router handler (`internal/ui/model/ui.go` lines 1458–1478)

The handler block for agents/tickets/approvals is at lines 1458–1477. The `memory` case follows the identical three-line pattern:
```go
case dialog.ActionOpenMemoryView:
    m.dialog.CloseDialog(dialog.CommandsID)
    memoryView := views.NewMemoryView(m.smithersClient)
    cmd := m.viewRouter.Push(memoryView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

---

## Data Query Gap — `ListAllMemoryFacts`

The current `ListMemoryFacts` SQL requires a non-empty namespace match. Real Smithers memory uses four namespace patterns:

| Pattern | Example |
|---------|---------|
| `workflow:<id>` | `workflow:code-review` |
| `agent:<id>` | `agent:claude-code` |
| `user:<id>` | `user:wcory` |
| `global` | `global` |

A bare `/memory` view that always passes `namespace: "default"` will typically return zero rows (the string `"default"` is not a real namespace pattern in Smithers). The scaffolding ticket must introduce a `ListAllMemoryFacts` client method that drops the `WHERE namespace = ?` predicate:

```go
// ListAllMemoryFacts lists all memory facts across all namespaces.
// Routes: SQLite → exec smithers memory list --all.
func (c *Client) ListAllMemoryFacts(ctx context.Context) ([]MemoryFact, error) {
    if c.db != nil {
        rows, err := c.queryDB(ctx,
            `SELECT namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms, ttl_ms
            FROM _smithers_memory_facts ORDER BY updated_at_ms DESC`)
        if err != nil {
            return nil, err
        }
        return scanMemoryFacts(rows)
    }
    out, err := c.execSmithers(ctx, "memory", "list", "--all", "--format", "json")
    if err != nil {
        return nil, err
    }
    return parseMemoryFactsJSON(out)
}
```

This is a small, non-breaking addition. The `MemoryView.Init()` calls `ListAllMemoryFacts` rather than `ListMemoryFacts`, which unblocks an immediately useful view and sidesteps the hardcoded-namespace problem documented in the engineering spec's Risk §3.

---

## View Rendering Requirements

### Row layout

Each fact renders as a two-line row:

```
▸ workflow:code-review / reviewer-preference
    {"style":"thorough","language":"typescript"}     2m ago
  workflow:deploy / last-deploy-sha
    "a1b2c3d"                                        1h ago
```

- Line 1: `[cursor] [namespace] / [key]` — namespace + `/` divider in faint style, key in bold when selected.
- Line 2: `    [valuePreview]` left-padded, faint; `[relativeAge]` right-aligned within available width (or simply appended with spaces).
- Row separator: blank line between rows, same as agents/tickets pattern.

### Helper functions

Two pure helper functions (unit-testable without a client):

**`factValuePreview(valueJSON string, maxLen int) string`** — returns at most `maxLen` runes of the JSON string. Typical display budget: 60 characters. If the value is a JSON string literal (starts with `"`), strip the outer quotes for readability. Example: `"\"a1b2c3d\""` → `"a1b2c3d"`. For objects/arrays, return the raw JSON truncated with `...`.

**`factAge(updatedAtMs int64) string`** — relative age from `time.Now()`:
- `< 60s` → `"Xs ago"` (seconds)
- `60s–3600s` → `"Xm ago"` (minutes)
- `3600s–86400s` → `"Xh ago"` (hours)
- `> 86400s` → `"Xd ago"` (days)

The `elapsedStr` helper in `internal/ui/components/runtable.go` (runs-dashboard plan) provides a reference for the time formatting pattern, but `factAge` is simpler because it always measures from now and rounds to a single unit.

---

## Test Infrastructure Assessment

### Unit tests

`internal/ui/views/router_test.go` exists and uses `package views_test`. No view-specific unit test files exist yet — `agents.go`, `tickets.go`, `approvals.go` all lack test files. The scaffolding ticket should establish the unit test pattern for `MemoryView` at `internal/ui/views/memory_test.go` — this becomes the reference for all future view unit tests.

The test pattern follows `internal/smithers/client_test.go`: construct the struct directly, send messages via `Update()`, assert on `View()` string output and struct field state. No mock client infrastructure is needed for loading/error state tests — the test can send the msg directly (bypassing `Init()`). For `Init()` testing, the `withExecFunc` option on `smithers.NewClient` can stub the exec transport.

### Terminal E2E tests

No Go terminal E2E harness exists yet. The `eng-approvals-view-scaffolding` ticket also called for building this harness. The research for that ticket described the pattern: Go subprocess spawn, `exec.Command`, stdout buffer read, ANSI strip, `waitForText` polling at 100ms intervals with a 10s timeout. The `tests/` directory exists but has no Go test files — only VHS tapes in `tests/vhs/`.

The memory scaffolding ticket should build or reuse this harness. If the approvals ticket has already landed, the harness at `tests/tui_helpers_test.go` (or equivalent) can be imported. If not, the memory ticket must build it as part of Slice 5. Either way, the helper file is a reusable package-level artifact, not specific to the memory view.

### VHS tapes

Four tapes exist in `tests/vhs/`:
- `smithers-domain-system-prompt.tape` — uses `CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures` + `go run .` launch pattern. Outputs `.gif` + `.png` to `tests/vhs/output/`.
- `branding-status.tape`, `helpbar-shortcuts.tape`, `mcp-tool-discovery.tape` — similar pattern.

The memory tape should follow the same structure. The fixture DB at `tests/fixtures/memory-test.db` needs pre-seeded rows — this is the only new fixture required. The schema for `_smithers_memory_facts` is fixed at 7 columns (namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms, ttl_ms) and has no foreign key dependencies, so a small Python or shell script can create it with plain SQL.

---

## Gaps Summary

| Gap | Notes |
|-----|-------|
| No `ListAllMemoryFacts` client method | `ListMemoryFacts` hardcodes `WHERE namespace = ?`; passing `""` yields zero rows for real data |
| No `internal/ui/views/memory.go` | New file; follows AgentsView/TicketsView pattern exactly |
| No `ActionOpenMemoryView` action | Add after existing Smithers actions in `dialog/actions.go` |
| No `"memory"` command palette entry | Add alongside agents/approvals/tickets in `dialog/commands.go` |
| No `ActionOpenMemoryView` router case | Add alongside ActionOpenAgentsView block in `ui.go` |
| No `internal/ui/views/memory_test.go` | First view-layer unit tests; establishes pattern for future views |
| No Go terminal E2E harness | `tests/tui_helpers_test.go` needed; shared with other view scaffolding tickets |
| No `tests/fixtures/memory-test.db` | SQLite fixture with `_smithers_memory_facts` rows for E2E and VHS |
| No `tests/vhs/memory-browser.tape` | VHS recording; follows existing tape patterns |

---

## Files To Touch

| File | Action |
|------|--------|
| `/Users/williamcory/crush/internal/smithers/client.go` | Add `ListAllMemoryFacts` method after `RecallMemory` |
| `/Users/williamcory/crush/internal/ui/views/memory.go` | New — `MemoryView` + `memoryLoadedMsg` + `memoryErrorMsg` + helpers |
| `/Users/williamcory/crush/internal/ui/views/memory_test.go` | New — 10 unit test cases + helper function tests |
| `/Users/williamcory/crush/internal/ui/dialog/actions.go` | Add `ActionOpenMemoryView struct{}` |
| `/Users/williamcory/crush/internal/ui/dialog/commands.go` | Add `"memory"` command item |
| `/Users/williamcory/crush/internal/ui/model/ui.go` | Add `case dialog.ActionOpenMemoryView` handler |
| `/Users/williamcory/crush/tests/tui_helpers_test.go` | New (or reuse if approvals ticket landed) — Go E2E harness |
| `/Users/williamcory/crush/tests/tui_memory_e2e_test.go` | New — terminal E2E test |
| `/Users/williamcory/crush/tests/fixtures/memory-test.db` | New — pre-seeded SQLite fixture |
| `/Users/williamcory/crush/tests/vhs/memory-browser.tape` | New — VHS happy-path tape |

Research complete.
