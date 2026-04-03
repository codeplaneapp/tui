# Engineering Spec: Run Dashboard Base View

## Metadata
- Ticket: `runs-dashboard`
- Feature: `RUNS_DASHBOARD`
- Group: Runs And Inspection
- Dependencies: `eng-smithers-client-runs` (Go HTTP client for `/v1/runs`)
- Target files:
  - `internal/ui/views/runs.go` (new)
  - `internal/ui/components/runtable.go` (new)
  - `internal/ui/dialog/actions.go` (modify — add `ActionOpenRunsView`)
  - `internal/ui/dialog/commands.go` (modify — add Runs entry to command palette)
  - `internal/ui/model/ui.go` (modify — wire Ctrl+R and `ActionOpenRunsView`)
  - `tests/runs_dashboard_e2e_test.go` (new)
  - `tests/runs_dashboard.tape` (new — VHS recording)

---

## Objective

Deliver the foundational Run Dashboard view — the first non-trivial data-driven Smithers view in the Crush-based TUI. Users access it via `/runs` in the command palette or `Ctrl+R` from chat and see a tabular list of all Smithers runs with workflow name, status, node progress, and elapsed time. The view supports cursor-based navigation and lays the groundwork for downstream features (inline details, quick actions, filtering, live chat drill-in).

This corresponds to the GUI's `RunsList.tsx` page at `smithers_tmp/gui-ref/apps/web/src/app/routes/workspace/runs/page.tsx` and the `RUNS_DASHBOARD` feature in `docs/smithers-tui/features.ts:42`.

---

## Scope

### In scope
- A `RunsView` struct implementing the `views.View` interface (`internal/ui/views/router.go:6-12`)
- A reusable `RunTable` component for rendering tabular run data (`internal/ui/components/runtable.go`)
- Async data loading via `smithers.Client.ListRuns()` (from `eng-smithers-client-runs`)
- Cursor-based Up/Down row navigation
- Navigation entry via `Ctrl+R` keybinding and command palette `/runs` item
- `ActionOpenRunsView` dialog action wired into `ui.go`
- Column rendering: Run ID (truncated), Workflow Name, Status (with color), Node Progress (e.g. `3/5`), Elapsed Time
- Loading and error states matching the pattern in `AgentsView` (`internal/ui/views/agents.go:44-53`)
- Manual refresh via `r` key
- Terminal E2E test using `@microsoft/tui-test` harness patterns
- VHS happy-path recording test

### Out of scope
- Real-time SSE streaming (`RUNS_REALTIME_STATUS_UPDATES` — separate ticket)
- Status sectioning / grouping rows by Active/Completed/Failed (`RUNS_STATUS_SECTIONING`)
- Filtering by status/workflow/date (`RUNS_FILTER_BY_*`)
- Inline run details expansion (`RUNS_INLINE_RUN_DETAILS`)
- Progress bar visualization (`RUNS_PROGRESS_VISUALIZATION`)
- Quick actions: approve, deny, cancel, hijack (`RUNS_QUICK_*`)
- Drill-in to run inspector, live chat, or timeline
- Split-pane layout

---

## Implementation Plan

### Slice 1: `ActionOpenRunsView` dialog action and keybinding

**Files**: `internal/ui/dialog/actions.go`, `internal/ui/dialog/commands.go`, `internal/ui/model/ui.go`

Add the navigation plumbing so the view can be reached before implementing the view itself.

1. Add `ActionOpenRunsView struct{}` to `internal/ui/dialog/actions.go:92` (next to the existing `ActionOpenAgentsView` and `ActionOpenTicketsView`).

2. Add a "Runs" entry in the commands dialog (`internal/ui/dialog/commands.go`) so that typing `/runs` in the command palette triggers `ActionOpenRunsView`. Follow the same pattern as the existing "Agents" and "Tickets" entries.

3. In `internal/ui/model/ui.go`, handle `ActionOpenRunsView` in the dialog-action switch block (around line 1436):

```go
case dialog.ActionOpenRunsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    runsView := views.NewRunsView(m.smithersClient)
    cmd := m.viewRouter.Push(runsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

4. Add a `Ctrl+R` keybinding in the key-handling section of `ui.go` that pushes the runs view directly (bypassing the command palette), matching the design doc's `ctrl+r  runs` shortcut (`docs/smithers-tui/02-DESIGN.md:117`).

**Verification**: Build compiles. Pressing `Ctrl+R` or selecting "Runs" from the palette transitions to `uiSmithersView`. Since the view doesn't exist yet, wire it to a stub `RunsView` that renders "Loading runs…".

---

### Slice 2: `RunTable` reusable component

**File**: `internal/ui/components/runtable.go`

A stateless rendering component that takes a slice of `smithers.Run` and a cursor index and produces a formatted table string. Separated from the view to enable reuse in the chat agent's tool renderers and in future split-pane layouts.

```go
package components

// RunTable renders a tabular list of runs.
type RunTable struct {
    Runs     []smithers.Run
    Cursor   int
    Width    int  // available terminal width
    Selected int  // index of highlighted row, -1 for none
}

// View renders the table as a string.
func (t RunTable) View() string { ... }
```

**Columns** (derived from the design doc wireframe at `02-DESIGN.md:137-172`):

| Column | Source field | Width | Notes |
|--------|-------------|-------|-------|
| Cursor | (computed) | 2 | `▸ ` or `  ` |
| ID | `Run.RunID` | 8 | Truncated to first 8 chars |
| Workflow | `Run.WorkflowName` | flex | Fills remaining space |
| Status | `Run.Status` | 18 | Color-coded via lipgloss: green=running, yellow=waiting-approval, red=failed, dim=cancelled, bold=finished |
| Progress | `Run.Summary` | 7 | `n/m` computed from summary map (`completed+failed / total`) |
| Time | `Run.StartedAtMs` | 8 | Relative duration from now (e.g. `2m 14s`) using `time.Since()` |

**Color mapping** (mirrors GUI badge colors from `gui-ref/apps/web/src/app/routes/workspace/runs/page.tsx`):

| Status | lipgloss style |
|--------|---------------|
| `running` | Green foreground |
| `waiting-approval` | Yellow foreground, bold |
| `finished` | Dim/faint |
| `failed` | Red foreground |
| `cancelled` | Dim/faint, strikethrough |

**Header row**: Rendered once at the top with faint styling, matching the wireframe column headers (`ID`, `Workflow`, `Status`, `Progress`, `Time`).

**Verification**: Unit test in `internal/ui/components/runtable_test.go` that passes a slice of 3 `smithers.Run` structs (running, waiting-approval, failed) and asserts the output contains the correct column values and cursor indicator.

---

### Slice 3: `RunsView` implementing `views.View`

**File**: `internal/ui/views/runs.go`

The view struct follows the established pattern from `AgentsView` (`internal/ui/views/agents.go`):

```go
package views

// Compile-time interface check.
var _ View = (*RunsView)(nil)

type runsLoadedMsg struct {
    runs []smithers.Run
}

type runsErrorMsg struct {
    err error
}

type RunsView struct {
    client  *smithers.Client
    runs    []smithers.Run
    cursor  int
    width   int
    height  int
    loading bool
    err     error
}
```

**Lifecycle**:

1. `Init()` — dispatches an async `tea.Cmd` that calls `client.ListRuns(ctx)` with default `RunFilter{Limit: 50}` and returns `runsLoadedMsg` or `runsErrorMsg`.

2. `Update(msg)` — handles:
   - `runsLoadedMsg`: stores runs, sets `loading = false`
   - `runsErrorMsg`: stores error, sets `loading = false`
   - `tea.WindowSizeMsg`: updates width/height
   - `tea.KeyPressMsg`:
     - `esc` / `alt+esc` → returns `PopViewMsg{}`
     - `up` / `k` → decrements cursor (clamped to 0)
     - `down` / `j` → increments cursor (clamped to `len(runs)-1`)
     - `r` → sets `loading = true`, re-dispatches `Init()`
     - `enter` → no-op for now (future: drill into run inspector)

3. `View()` — renders:
   - **Header line**: `SMITHERS › Runs` left-aligned, `[Esc] Back` right-aligned (matching `AgentsView.View()` pattern at `agents.go:100-113`)
   - **Loading state**: `"  Loading runs..."` (matching agents pattern)
   - **Error state**: `"  Error: <msg>"` (matching agents pattern)
   - **Empty state**: `"  No runs found."`
   - **Table body**: Delegates to `components.RunTable{Runs: v.runs, Cursor: v.cursor, Width: v.width}.View()`

4. `Name()` → `"runs"`

5. `ShortHelp()` → `[]string{"[Enter] Inspect", "[r] Refresh", "[Esc] Back"}`

**Data flow**:
```
User presses Ctrl+R
  → ui.go handles KeyPressMsg, pushes RunsView onto viewRouter
  → RunsView.Init() fires async tea.Cmd
  → tea.Cmd calls smithersClient.ListRuns(ctx)
  → smithers.Client hits GET /v1/runs?limit=50 (HTTP) or falls back to exec("smithers ps --json")
  → Returns runsLoadedMsg{runs: [...]}
  → RunsView.Update() stores runs, triggers re-render
  → RunsView.View() delegates to RunTable.View()
  → User sees table; navigates with Up/Down, refreshes with r, exits with Esc
```

**Verification**: Build and run manually. Open the TUI, press `Ctrl+R`, verify the header renders. If a Smithers server is running, verify runs appear. If not, verify the error state renders gracefully.

---

### Slice 4: Wire the `RunsView` constructor into `ui.go`

**File**: `internal/ui/model/ui.go`

This slice connects the plumbing from Slice 1 to the real `RunsView` from Slice 3, replacing any stub.

1. Import the `views` package (already imported) and ensure `NewRunsView` is used in both:
   - The `ActionOpenRunsView` dialog-action handler
   - The `Ctrl+R` direct keybinding handler

2. Add the `Ctrl+R` key match inside the `uiChat` key-handling block (around the area where other global shortcuts are handled, near `ui.go:1700+`):

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
    runsView := views.NewRunsView(m.smithersClient)
    cmd := m.viewRouter.Push(runsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

3. Ensure `Ctrl+R` only fires when in `uiChat` or `uiLanding` states — not when already in a Smithers view or when the editor is focused.

**Verification**: From the chat view, press `Ctrl+R` → runs view appears. Press `Esc` → returns to chat. Open command palette → select "Runs" → runs view appears.

---

### Slice 5: Terminal E2E test (tui-test harness)

**File**: `tests/runs_dashboard_e2e_test.go`

Model this test on the upstream `@microsoft/tui-test` harness patterns from `smithers_tmp/tests/tui.e2e.test.ts` and `smithers_tmp/tests/tui-helpers.ts`.

The upstream harness:
- Spawns the TUI as a child process (`BunSpawnBackend` in `tui-helpers.ts`)
- Strips ANSI escape sequences from output
- Sends keystrokes (Enter, Esc, arrow keys) via stdin
- Captures buffer snapshots for assertion
- Uses 15-second timeout with buffer dump on failure

For the Go E2E test, we adapt this pattern:

```go
package tests

import (
    "os/exec"
    "testing"
    "time"
    // ...
)

func TestRunsDashboard_E2E(t *testing.T) {
    // 1. Start a mock Smithers HTTP server that returns canned /v1/runs JSON
    srv := startMockSmithersServer(t, mockRunsResponse)
    defer srv.Close()

    // 2. Launch the TUI binary with SMITHERS_API_URL pointed at the mock
    cmd := exec.Command("go", "run", ".", "--api-url", srv.URL)
    cmd.Env = append(os.Environ(), "SMITHERS_API_URL="+srv.URL)
    // ... set up PTY via creack/pty or similar

    // 3. Wait for initial render (chat view)
    waitForOutput(t, pty, "SMITHERS", 5*time.Second)

    // 4. Send Ctrl+R to navigate to runs view
    sendKey(pty, ctrlR)

    // 5. Assert table headers render
    waitForOutput(t, pty, "Workflow", 5*time.Second)
    waitForOutput(t, pty, "Status", 5*time.Second)

    // 6. Assert run data renders (from mock response)
    waitForOutput(t, pty, "code-review", 3*time.Second)
    waitForOutput(t, pty, "running", 3*time.Second)

    // 7. Send Down arrow, verify cursor moves
    sendKey(pty, arrowDown)
    snapshot := captureBuffer(pty)
    assertContains(t, snapshot, "▸") // cursor on second row

    // 8. Send Esc, verify return to chat
    sendKey(pty, escape)
    waitForOutput(t, pty, "Ready", 3*time.Second)
}
```

**Mock server fixture**: Returns a JSON array of 3 runs matching the design wireframe:
- `abc123` / `code-review` / `running` / 3 of 5 nodes
- `def456` / `deploy-staging` / `waiting-approval` / 4 of 6 nodes
- `ghi789` / `test-suite` / `running` / 1 of 3 nodes

**Test assertions** (mapped from acceptance criteria):
1. View is accessible via `Ctrl+R` — assert header "SMITHERS › Runs" appears
2. Table columns "Workflow" and "Status" are rendered
3. Run data from mock server appears in the table
4. Up/Down navigation moves the cursor indicator (`▸`)
5. Esc returns to chat view

**Helper utilities** (in `tests/helpers_test.go`):
- `startMockSmithersServer(t, handler)` — `httptest.Server` wrapper
- `waitForOutput(t, pty, text, timeout)` — polls PTY output with ANSI stripping
- `sendKey(pty, key)` — writes escape sequences for special keys
- `captureBuffer(pty)` — reads current terminal buffer

These helpers mirror the upstream `tui-helpers.ts` functions: `BunSpawnBackend`, ANSI stripping, text matching with whitespace normalization, and key input simulation.

**Verification**: `go test ./tests/ -run TestRunsDashboard_E2E -v` passes.

---

### Slice 6: VHS happy-path recording test

**File**: `tests/runs_dashboard.tape`

A [VHS](https://github.com/charmbracelet/vhs) tape file that records the happy path of opening the runs dashboard, navigating, and returning to chat. VHS is Charm's terminal recording tool that produces GIF/MP4 from declarative scripts.

```tape
# runs_dashboard.tape — Happy-path recording for the Runs Dashboard view
Output tests/runs_dashboard.gif
Set FontSize 14
Set Width 120
Set Height 40
Set Shell "bash"
Set TypingSpeed 50ms

# Start the TUI with mock server
Type "SMITHERS_API_URL=http://localhost:7331 go run ."
Enter
Sleep 3s

# Navigate to runs dashboard
Ctrl+R
Sleep 2s

# Verify table is visible (visual check in recording)
# Navigate down through runs
Down
Sleep 500ms
Down
Sleep 500ms
Up
Sleep 500ms

# Refresh the list
Type "r"
Sleep 2s

# Return to chat
Escape
Sleep 1s
```

**CI integration**: Add a Makefile target or CI step:
```bash
# Validate the tape parses and runs (--validate flag)
vhs validate tests/runs_dashboard.tape

# Generate recording (optional, for documentation)
vhs tests/runs_dashboard.tape
```

**Verification**: `vhs validate tests/runs_dashboard.tape` exits 0. Running `vhs tests/runs_dashboard.tape` produces a GIF showing the full navigation flow.

---

## Validation

### Automated checks

| Check | Command | What it verifies |
|-------|---------|-----------------|
| Build | `go build ./...` | All new files compile, no import cycles |
| Unit tests | `go test ./internal/ui/components/ -run TestRunTable -v` | `RunTable` renders correct columns, cursor, colors |
| Unit tests | `go test ./internal/ui/views/ -run TestRunsView -v` | `RunsView` handles loaded/error/empty states correctly |
| E2E test | `go test ./tests/ -run TestRunsDashboard_E2E -v -timeout 30s` | Full flow: launch → Ctrl+R → table renders → navigate → Esc → chat |
| VHS validate | `vhs validate tests/runs_dashboard.tape` | Tape file syntax is valid |
| VHS record | `vhs tests/runs_dashboard.tape` | Produces `tests/runs_dashboard.gif` showing happy path |

### Manual verification paths

1. **With Smithers server running** (`smithers up --serve`):
   - Launch the TUI
   - Press `Ctrl+R` — should see real runs from the server
   - Verify columns: ID, Workflow, Status, Progress, Time
   - Navigate with Up/Down — cursor indicator (`▸`) moves
   - Press `r` — "Loading runs…" flashes, then table refreshes
   - Press `Esc` — returns to chat view

2. **Without Smithers server** (exec fallback):
   - Launch the TUI without a running server
   - Press `Ctrl+R` — should either show runs via `smithers ps --json` fallback or display a graceful error message
   - Verify error state does not crash the TUI

3. **Command palette**:
   - Press `/` or `Ctrl+P` to open command palette
   - Type "runs" — "Runs" entry should appear in filtered results
   - Select it — should navigate to the runs dashboard

4. **Empty state**:
   - With server running but no runs active
   - Press `Ctrl+R` — should display "No runs found."

### E2E test coverage mapping (tui-test harness)

| Acceptance criterion | E2E assertion |
|---------------------|---------------|
| Accessible via Ctrl+R | `sendKey(pty, ctrlR)` + `waitForOutput("SMITHERS › Runs")` |
| Displays tabular list | `waitForOutput("Workflow")` + `waitForOutput("Status")` |
| Fetches data via Smithers Client | Mock server receives `GET /v1/runs` request |
| Up/Down navigation | `sendKey(pty, arrowDown)` + assert cursor position changes |
| VHS recording | `vhs validate tests/runs_dashboard.tape` exits 0 |

---

## Risks

### 1. `eng-smithers-client-runs` not yet landed

**Impact**: `RunsView.Init()` calls `client.ListRuns()` which doesn't exist yet in `internal/smithers/client.go`.

**Mitigation**: Slice 3 can be developed against a local stub method on `Client`:
```go
func (c *Client) ListRuns(ctx context.Context) ([]Run, error) {
    return nil, ErrNoTransport
}
```
Replace with the real implementation when `eng-smithers-client-runs` lands. The `RunsView` already handles the error case gracefully.

### 2. Transport envelope mismatch

**Impact**: The existing `smithers.Client` uses a `{ok, data, error}` response envelope (`client.go` `apiEnvelope` type), but the upstream `/v1/runs` API returns direct JSON arrays without the envelope wrapper (per `smithers_tmp/gui-ref/apps/daemon/src/server/routes/run-routes.ts`).

**Mitigation**: The `eng-smithers-client-runs` spec already identifies this mismatch and plans a separate `httpGetDirect[T]()` helper for direct-JSON endpoints. The runs dashboard depends on that fix landing first. If it doesn't, the exec fallback (`smithers ps --json`) is the stopgap.

### 3. No Smithers server in development

**Impact**: Developers working on the TUI may not have a running Smithers server, making manual testing difficult.

**Mitigation**: The E2E test uses a mock `httptest.Server` that returns canned JSON. For manual testing, add a `--mock-runs` flag or `SMITHERS_MOCK=1` env var that injects fixture data, similar to how `gui-ref/apps/daemon/src/domain/workspaces/mock-data.ts` provides mock workspace data.

### 4. PTY-based E2E tests are flaky on CI

**Impact**: Terminal E2E tests that rely on PTY output timing can be brittle across CI environments with varying CPU/IO speeds.

**Mitigation**: Use generous timeouts (15s per assertion, matching upstream `tui.e2e.test.ts`), retry-with-backoff on assertion polling, and dump the terminal buffer on failure for debugging. The upstream test harness (`tui-helpers.ts`) demonstrates this pattern with buffer snapshot capture on timeout.

### 5. Ctrl+R conflicts with existing shell reverse-search

**Impact**: Users running the TUI inside shells where `Ctrl+R` triggers reverse-search may experience key conflicts.

**Mitigation**: Crush's Bubble Tea program runs in raw mode and captures all key events before the shell sees them. This is a non-issue once the TUI is running — the conflict only exists if the user hasn't entered the TUI yet. The command palette `/runs` provides an alternative entry point.

### 6. `RunTable` width calculation on narrow terminals

**Impact**: Terminals narrower than ~60 columns may not have enough space for all columns, causing wrapping or truncation artifacts.

**Mitigation**: `RunTable` should gracefully degrade: hide the Progress and Time columns below 80 columns, and truncate the Workflow column with ellipsis below 60 columns. This follows the `sidebarCompactModeBreakpoint` pattern in `dialog/commands.go:30`.
