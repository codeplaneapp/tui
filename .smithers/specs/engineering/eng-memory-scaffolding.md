# Engineering Spec: Scaffold Memory Browser View

**Ticket**: `eng-memory-scaffolding`
**Group**: Systems And Analytics
**Dependencies**: `eng-systems-api-client`
**Features**: `MEMORY_BROWSER`, `MEMORY_FACT_LIST`

---

## Objective

Create the base Bubble Tea model, view routing, and command-palette integration for the `/memory` view in the Smithers TUI. This scaffolding ticket delivers a navigable list of memory facts fetched from the Smithers backend (via SQLite or CLI exec fallback), following the same view pattern established by `AgentsView` (`internal/ui/views/agents.go`) and `TicketsView` (`internal/ui/views/tickets.go`). On completion, users can type `/memory` or select "Memory Browser" from the command palette to open a read-only fact list with namespace, key, value preview, and timestamp.

---

## Scope

### In Scope

1. **`internal/ui/views/memory.go`** — New `MemoryView` struct implementing the `views.View` interface (`Init`, `Update`, `View`, `Name`, `ShortHelp`).
2. **Command-palette entry** — `ActionOpenMemoryView` action type in `internal/ui/dialog/actions.go`, wired to a "Memory Browser" command item in `internal/ui/dialog/commands.go`.
3. **Router integration** — Handler case in `internal/ui/model/ui.go` that pushes `MemoryView` onto `viewRouter` and transitions to `uiSmithersView` state.
4. **Data fetching** — `MemoryView.Init()` calls `smithers.Client.ListMemoryFacts()` (already implemented in `internal/smithers/client.go:356`) with a default namespace. Loading, error, and empty states are handled.
5. **Fact list rendering** — Cursor-navigable list showing each fact's namespace, key, a truncated JSON value preview, and relative age. Styled with lipgloss, consistent with agents/tickets views.
6. **Keybindings** — `↑`/`↓`/`k`/`j` navigation, `r` refresh, `Esc` back (pop view), `Enter` reserved for future detail expansion.
7. **Tests** — Unit tests for the view, terminal E2E test, and a VHS recording test.

### Out of Scope

- Semantic recall UI (covered by future `feat-memory-semantic-recall` ticket).
- Cross-run message history browsing (covered by `feat-memory-cross-run-message-history`).
- Fact detail panel / split-pane layout (future enhancement).
- Write/delete/edit operations on facts.
- Namespace picker or filtering UI (future enhancement; the scaffolding uses a default namespace).

---

## Implementation Plan

### Slice 1: MemoryView struct and View interface

**File**: `internal/ui/views/memory.go`

Create the view following the exact pattern from `AgentsView`:

```go
package views

// Compile-time interface check.
var _ View = (*MemoryView)(nil)

type memoryLoadedMsg struct {
    facts []smithers.MemoryFact
}

type memoryErrorMsg struct {
    err error
}

type MemoryView struct {
    client    *smithers.Client
    facts     []smithers.MemoryFact
    cursor    int
    width     int
    height    int
    loading   bool
    err       error
    namespace string // default: "default"
}

func NewMemoryView(client *smithers.Client) *MemoryView {
    return &MemoryView{
        client:    client,
        loading:   true,
        namespace: "default",
    }
}
```

**Init**: Spawn async command calling `v.client.ListMemoryFacts(ctx, v.namespace, "")`.

**Update**: Handle `memoryLoadedMsg`, `memoryErrorMsg`, `tea.WindowSizeMsg`, `tea.KeyPressMsg` (esc → `PopViewMsg`, up/k, down/j, r → reload, enter → no-op placeholder).

**View**: Render header line `SMITHERS › Memory` with `[Esc] Back` right-aligned. Loading state → `"  Loading memory facts..."`. Error state → `"  Error: ..."`. Empty state → `"  No memory facts found."`. Otherwise, render cursor-navigable list:

```
▸ workflow:code-review / reviewer-preference
    {"style":"thorough","language":"typescript"}     2m ago
  workflow:deploy / last-deploy-sha
    "a1b2c3d"                                        1h ago
```

Each row: cursor indicator (`▸` or `  `), bold namespace+key line, faint truncated value (max 60 chars) + relative timestamp.

**Name**: Return `"memory"`.

**ShortHelp**: Return `[]string{"[Enter] View", "[r] Refresh", "[Esc] Back"}`.

**Helper function**: `factValuePreview(valueJSON string, maxLen int) string` — Truncates JSON value string for display. `factAge(updatedAtMs int64) string` — Returns relative time string ("2m ago", "1h ago", "3d ago").

### Slice 2: Action type and command-palette registration

**File**: `internal/ui/dialog/actions.go`

Add to the existing action type block:

```go
// ActionOpenMemoryView is a message to navigate to the memory browser view.
ActionOpenMemoryView struct{}
```

**File**: `internal/ui/dialog/commands.go`

Add alongside existing agents/tickets entries (around line 527):

```go
NewCommandItem(c.com.Styles, "memory", "Memory Browser", "", ActionOpenMemoryView{}),
```

### Slice 3: Router handler in UI model

**File**: `internal/ui/model/ui.go`

Add a case alongside the existing `ActionOpenAgentsView` and `ActionOpenTicketsView` handlers (around line 1443):

```go
case dialog.ActionOpenMemoryView:
    m.dialog.CloseDialog(dialog.CommandsID)
    memoryView := views.NewMemoryView(m.smithersClient)
    cmd := m.viewRouter.Push(memoryView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

This follows the identical pattern at `ui.go:1436-1448`.

### Slice 4: Unit tests

**File**: `internal/ui/views/memory_test.go`

Test cases:

1. **`TestMemoryView_Init`** — Verify `Init()` returns a non-nil `tea.Cmd`.
2. **`TestMemoryView_LoadedMsg`** — Send `memoryLoadedMsg` with sample facts, verify `loading` becomes false, `facts` is populated, `err` is nil.
3. **`TestMemoryView_ErrorMsg`** — Send `memoryErrorMsg`, verify error is stored and rendered.
4. **`TestMemoryView_EmptyState`** — Send `memoryLoadedMsg` with empty slice, verify "No memory facts found" appears in `View()`.
5. **`TestMemoryView_CursorNavigation`** — Load 3 facts, send down-key twice, verify cursor is 2. Send up-key, verify cursor is 1.
6. **`TestMemoryView_EscPopView`** — Send Esc key, verify returned `tea.Cmd` produces `PopViewMsg`.
7. **`TestMemoryView_Refresh`** — Send `r` key, verify `loading` becomes true and a new `tea.Cmd` is returned.
8. **`TestMemoryView_Name`** — Verify `Name()` returns `"memory"`.
9. **`TestFactValuePreview`** — Test truncation at boundary, short values unchanged, long JSON truncated with `...`.
10. **`TestFactAge`** — Test relative time formatting for various durations.

Use the same test structure as `internal/smithers/client_test.go` — construct the view directly, send messages via `Update()`, assert on `View()` output and state.

### Slice 5: Terminal E2E test (tui-test harness style)

**File**: `tests/tui_memory_e2e_test.go` (or equivalent test runner file)

Model the test on the upstream harness pattern from `smithers/tests/tui.e2e.test.ts` + `tui-helpers.ts`:

1. **Setup**: Spawn the TUI binary as a subprocess. Set `TERM=xterm-256color`. Seed the Smithers SQLite DB with at least 2 `_smithers_memory_facts` rows (namespace `"default"`, keys `"test-fact-1"` and `"test-fact-2"` with JSON values and timestamps).
2. **Navigate to Memory**: Send command-palette keystrokes (type `/memory` + Enter, or use the Ctrl+P palette → type "memory" → Enter).
3. **Assert header**: Wait for text `"SMITHERS › Memory"` to appear in the terminal buffer.
4. **Assert fact list**: Wait for text `"test-fact-1"` and `"test-fact-2"`.
5. **Navigate**: Send `j` (down), verify cursor moves. Send `k` (up), verify cursor returns.
6. **Refresh**: Send `r`, wait for `"Loading memory facts..."` briefly, then wait for facts to reappear.
7. **Exit**: Send `Esc`, verify the view pops (header text no longer present, back to chat or landing).
8. **Teardown**: Terminate subprocess, clean up temp DB.

The test helper should:
- Strip ANSI escape codes before text matching (same as `tui-helpers.ts`).
- Use `waitForText(text, timeoutMs)` polling pattern with 100ms intervals and 10s default timeout.
- Capture terminal snapshot on assertion failure for debugging.

### Slice 6: VHS happy-path recording test

**File**: `tests/vhs/memory-browser.tape`

```tape
# Memory Browser — happy-path smoke test
Output tests/vhs/memory-browser.gif
Set Shell "bash"
Set FontSize 14
Set Width 120
Set Height 30
Set Padding 10

# Ensure test Smithers DB is seeded
Type "SMITHERS_DB=tests/fixtures/memory-test.db smithers-tui"
Enter
Sleep 2s

# Open command palette and navigate to memory
Type "/"
Sleep 500ms
Type "memory"
Sleep 500ms
Enter
Sleep 2s

# Verify memory view is visible with facts
Screenshot tests/vhs/memory-browser-list.png

# Navigate down through facts
Down
Sleep 300ms
Down
Sleep 300ms

# Refresh
Type "r"
Sleep 1s

# Go back
Escape
Sleep 1s

# Exit
Type "q"
```

This tape generates a `.gif` recording for visual regression and a `.png` screenshot for CI assertions. The test fixture DB (`tests/fixtures/memory-test.db`) must be pre-seeded with `_smithers_memory_facts` rows.

---

## Validation

### Automated Checks

| Check | Command | Expected |
|-------|---------|----------|
| Unit tests pass | `go test ./internal/ui/views/ -run TestMemory -v` | All 10 test cases pass |
| Helper tests pass | `go test ./internal/ui/views/ -run TestFact -v` | `factValuePreview` and `factAge` pass |
| Full test suite | `go test ./...` | No regressions |
| Build succeeds | `go build ./...` | Clean build, no compile errors |
| Vet passes | `go vet ./...` | No issues |
| E2E memory test | `go test ./tests/ -run TestMemoryE2E -timeout 30s -v` | Subprocess launches, navigates to memory, asserts facts visible, exits cleanly |
| VHS recording | `vhs tests/vhs/memory-browser.tape` | Generates `.gif` and `.png` without errors |

### Manual Verification

1. **Launch TUI**: Run `go run .` in the crush directory (with a Smithers project that has memory facts).
2. **Command palette**: Press `/` or `Ctrl+P`, type "memory", press Enter. Verify the memory browser view appears with header `SMITHERS › Memory`.
3. **Fact display**: Verify facts show namespace/key, truncated value preview, and relative timestamp.
4. **Navigation**: Press `j`/`k` or `↑`/`↓` to move cursor. Verify cursor indicator (`▸`) moves correctly.
5. **Refresh**: Press `r`. Verify loading indicator appears briefly, then facts reload.
6. **Empty state**: Test with no memory facts in DB. Verify "No memory facts found." message.
7. **Error state**: Test with unreachable DB / no smithers binary. Verify error message renders.
8. **Back navigation**: Press `Esc`. Verify view pops and returns to previous screen (chat or landing).
9. **Help bar**: Verify bottom help bar shows `[Enter] View  [r] Refresh  [Esc] Back` when memory view is active.

### Terminal E2E Coverage (tui-test harness)

Following the upstream `@microsoft/tui-test` pattern from `smithers/tests/tui.e2e.test.ts`:

- **Test file**: `tests/tui_memory_e2e_test.go`
- **Harness**: Go subprocess spawning with `exec.Command`, reading stdout buffer, ANSI stripping, `waitForText` polling.
- **Coverage**:
  - Opens memory view via command palette
  - Verifies fact list renders with seeded data
  - Cursor navigation (j/k) changes visible selection
  - Refresh (r) reloads data
  - Esc returns to previous view
  - Snapshot capture on failure

### VHS Recording Test

- **Tape file**: `tests/vhs/memory-browser.tape`
- **Output**: `tests/vhs/memory-browser.gif` (visual recording), `tests/vhs/memory-browser-list.png` (screenshot)
- **CI integration**: `vhs tests/vhs/memory-browser.tape` runs in CI, exit code 0 = pass
- **Fixture**: `tests/fixtures/memory-test.db` pre-seeded SQLite DB with `_smithers_memory_facts` rows

---

## Risks

### 1. Dependency on `eng-systems-api-client` readiness

The ticket declares `eng-systems-api-client` as a dependency. The Smithers client methods `ListMemoryFacts` and `RecallMemory` are already implemented in `internal/smithers/client.go:354-396`, including SQLite direct access and exec fallback. However, the systems API client ticket may refactor transport or types. **Mitigation**: The scaffolding only calls `ListMemoryFacts()` which is stable. If the API client refactors, the view's Init function call signature change is mechanical.

### 2. SQLite DB availability for testing

The E2E and VHS tests require a pre-seeded `_smithers_memory_facts` table in a SQLite database. The Smithers DB schema is managed by the TypeScript runtime (`smithers/src/db/ensure.ts`), not by the Go code. **Mitigation**: Create a test fixture script that initializes the DB and inserts test rows using plain SQL (`CREATE TABLE IF NOT EXISTS _smithers_memory_facts ...`). The schema is simple (7 columns, no foreign keys for this table). Alternatively, use the `ExecuteSQL` client method to create test data if a Smithers instance is available.

### 3. Namespace discovery — default "default" may not match real data

The scaffolding hardcodes `namespace: "default"` for the initial load. Real Smithers memory uses 4 namespace kinds (`workflow:<id>`, `agent:<id>`, `user:<id>`, `global`). If the user has no facts in the `"default"` namespace, they'll see an empty list. **Mitigation**: This is acceptable for scaffolding — the future `feat-memory-fact-list` ticket should add namespace browsing/filtering. For now, document that users should pass a specific namespace or the view will show all facts (consider changing the initial query to omit the namespace filter and return all facts across namespaces — the `_smithers_memory_facts` query can drop the `WHERE namespace = ?` clause).

### 4. Mismatch: Upstream Smithers has no HTTP memory routes

The GUI reference (`smithers_tmp/gui-ref/apps/daemon/src/server/routes/`) does not include memory-specific HTTP routes. The Smithers daemon exposes runs, workflows, approvals, settings, etc., but not memory. The Go client's `ListMemoryFacts` method compensates by using direct SQLite access as the primary transport (falling back to exec). This means the memory view works without the Smithers HTTP server running, as long as the SQLite DB is accessible. **Impact on scaffolding**: None — the current client implementation handles this gracefully. Future tickets adding write operations or semantic recall will need to ensure the exec fallback path works or the HTTP routes are added upstream.

### 5. No existing Go E2E test infrastructure

Crush does not currently have a Go-based terminal E2E test harness equivalent to the upstream TypeScript one (`tui-helpers.ts`). The E2E test slice requires building a minimal Go test helper that spawns the binary, reads stdout, strips ANSI, and polls for expected text. **Mitigation**: The helper is straightforward (~100 lines) and is reusable by all subsequent view scaffolding tickets. Model directly on the `tui-helpers.ts` implementation: `spawn` → `waitForText` → `sendKeys` → `snapshot` → `terminate`.

### 6. VHS binary availability in CI

VHS (`charmbracelet/vhs`) must be installed in the CI environment. If it's not available, the VHS tape test will fail. **Mitigation**: Gate the VHS test behind a build tag or environment variable check (`if which vhs > /dev/null`). The VHS test is supplementary to the E2E test, not a replacement.
