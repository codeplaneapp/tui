# Engineering: Scaffolding for Agents View

## Metadata
- ID: eng-agents-view-scaffolding
- Group: Agents (agents)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Create the structural boilerplate for the Agents view and establish the internal client method to fetch agent data from the Smithers CLI.

## Acceptance Criteria

- Create internal/ui/views/agents.go implementing the base View interface.
- Add a ListAgents() stub to internal/smithers/client.go.
- Register the /agents route in the main view router so it can be navigated to.

## Source Context

- internal/ui/views/router.go
- internal/smithers/client.go
- internal/ui/model/ui.go

## Implementation Notes

- Use Crush's existing Bubble Tea list component patterns if applicable.
- The list of agents will mirror the behavior in ../smithers/gui/src/components/AgentsList.tsx.

---

## Objective

Stand up the foundational Agents view in the Crush-based Smithers TUI so that subsequent agent feature tickets (CLI detection, availability status, auth classification, role display, native TUI launch) have a working view skeleton, data types, client stub, and route registration to build on. After this ticket, a user can navigate to `/agents` via the command palette and see a placeholder agent list; the Smithers client layer is ready to fetch real data from the Smithers HTTP API; and E2E test infrastructure exists to validate view navigation.

## Scope

### In Scope

1. **View router** — `internal/ui/views/router.go`: the `View` interface, `PopViewMsg` type, and `Router` stack manager that all Smithers views (agents, runs, workflows, etc.) will use.
2. **Agent data types** — `internal/smithers/types.go`: `Agent` struct merging upstream `AgentAvailability` (from `smithers/src/cli/agent-detection.ts`) and `AgentCli` (from `smithers/gui-ref/packages/shared/src/schemas/agent.ts`).
3. **Client stub** — `internal/smithers/client.go`: `Client` struct with `ListAgents(ctx context.Context) ([]Agent, error)`. Initial implementation returns hardcoded placeholder data; HTTP and shell-out backends wired in later tickets.
4. **Agents view** — `internal/ui/views/agents.go`: Bubble Tea view implementing `View` with cursor-navigable agent list, loading/error states, and help bar hints.
5. **Route integration** — `internal/ui/model/ui.go`: new `uiSmithersView` state, `viewRouter` and `smithersClient` fields on `UI`, message forwarding to the active view, Draw/ShortHelp delegation, and PopViewMsg handling to return to chat.
6. **Command palette entry** — `internal/ui/dialog/actions.go` + `commands.go`: `ActionOpenAgentsView` action and `/agents` command item.
7. **Tests** — Terminal E2E test verifying `/agents` navigation round-trip, plus a VHS tape recording a happy-path walkthrough.

### Out of Scope

- Real agent detection logic (ticket `feat-agents-cli-detection`).
- Availability/auth status rendering (tickets `feat-agents-availability-status`, `feat-agents-auth-status-classification`).
- Native TUI handoff on Enter (ticket `feat-agents-native-tui-launch`).
- Role display (ticket `feat-agents-role-display`).
- Binary path display (ticket `feat-agents-binary-path-display`).
- SSE-based real-time refresh.

## Implementation Plan

### Slice 1 — Agent Data Types (`internal/smithers/types.go`)

**File**: `internal/smithers/types.go` (new package)

```go
package smithers

// Agent represents a CLI agent detected on the system.
type Agent struct {
    ID         string   // e.g. "claude-code", "codex", "gemini"
    Name       string   // Display name, e.g. "Claude Code"
    Command    string   // CLI binary name, e.g. "claude"
    BinaryPath string   // Resolved full path, e.g. "/usr/local/bin/claude"
    Status     string   // "likely-subscription" | "api-key" | "binary-only" | "unavailable"
    HasAuth    bool     // Authentication signal detected
    HasAPIKey  bool     // API key env var present
    Usable     bool     // Agent can be launched
    Roles      []string // e.g. ["coding", "review"]
}
```

**Upstream field mapping**:

| Go field    | `AgentAvailability` (`smithers/src/cli/agent-detection.ts`) | `AgentCli` (`smithers/gui-ref/packages/shared/src/schemas/agent.ts`) |
|-------------|------------------------------------------|-----------------------|
| ID          | `id`                                     | `id`                  |
| Name        | —                                        | `name`                |
| Command     | `binary`                                 | `command`             |
| BinaryPath  | —                                        | `binaryPath`          |
| Status      | `status`                                 | —                     |
| HasAuth     | `hasAuthSignal`                          | —                     |
| HasAPIKey   | `hasApiKeySignal`                        | —                     |
| Usable      | `usable`                                 | —                     |
| Roles       | (derived from role preference map in `agent-detection.ts`) | —  |

**Rationale**: The GUI split agent data across `AgentCli` (display metadata) and `AgentAvailability` (detection results) because they lived in separate packages. The TUI merges them into one struct since the view and client are tightly coupled and the separation adds no value in Go.

### Slice 2 — Smithers Client Stub (`internal/smithers/client.go`)

**File**: `internal/smithers/client.go` (new package)

Create a minimal `Client` struct with `ListAgents`. The `Client` is the TUI's single entry point for all Smithers data per the thin-frontend architecture (03-ENGINEERING.md §2.2).

```go
package smithers

import "context"

type Client struct{}

func NewClient() *Client { return &Client{} }

func (c *Client) ListAgents(_ context.Context) ([]Agent, error) {
    return []Agent{
        {ID: "claude-code", Name: "Claude Code", Command: "claude", Status: "unavailable"},
        {ID: "codex", Name: "Codex", Command: "codex", Status: "unavailable"},
        {ID: "gemini", Name: "Gemini", Command: "gemini", Status: "unavailable"},
        {ID: "kimi", Name: "Kimi", Command: "kimi", Status: "unavailable"},
        {ID: "amp", Name: "Amp", Command: "amp", Status: "unavailable"},
        {ID: "forge", Name: "Forge", Command: "forge", Status: "unavailable"},
    }, nil
}
```

Returns all six known agents (matching `smithers/src/agents/index.ts` — ClaudeCode, Codex, Gemini, Kimi, Amp, Forge) with `Status: "unavailable"` so the view has data to render without any external service. The upstream endpoint is `GET /api/agents/clis` (from `smithers/gui-ref/apps/daemon/src/server/routes/agent-routes.ts`); wiring to it is deferred to `feat-agents-cli-detection`.

**Future evolution**: The `Client` struct will grow `apiURL`, `apiToken`, `dbPath` fields and a dual-mode strategy (HTTP API primary, shell-out fallback) as specified in 03-ENGINEERING.md §3.1.3. This ticket only creates the skeleton.

### Slice 3 — View Router (`internal/ui/views/router.go`)

**File**: `internal/ui/views/router.go` (new package)

Define the `View` interface and `Router` stack manager that all Smithers views share. Based on 03-ENGINEERING.md §3.1.1 but simplified — the chat is NOT a View on the stack; the router only holds non-chat views. When the stack is empty, the main model falls back to chat.

```go
package views

import tea "charm.land/bubbletea/v2"

type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Name() string
    ShortHelp() []string
}

type PopViewMsg struct{}

type Router struct {
    stack []View
}

func NewRouter() *Router              { return &Router{} }
func (r *Router) Push(v View) tea.Cmd { r.stack = append(r.stack, v); return v.Init() }
func (r *Router) Pop() bool           { /* pop if non-empty */ }
func (r *Router) Current() View       { /* top of stack or nil */ }
func (r *Router) HasViews() bool      { return len(r.stack) > 0 }
```

**Design decision**: The engineering doc (§3.1.1) shows a `chat View` field embedded in the router so chat is always at the bottom of the stack. The actual implementation uses a simpler pattern: the router only holds non-chat views, and `HasViews() == false` means "show chat." This avoids wrapping the existing monolithic chat model (which uses `Draw(scr, area)`, not `View() string`) in a View adapter. All other Smithers views use `View() string` and are bridged into the Ultraviolet pipeline via `uv.NewStyledString()`.

### Slice 4 — Agents View (`internal/ui/views/agents.go`)

**File**: `internal/ui/views/agents.go`

Implements `View` with a cursor-navigable agent list.

**Struct**:
```go
type AgentsView struct {
    client  *smithers.Client
    agents  []smithers.Agent
    cursor  int
    width   int
    height  int
    loading bool
    err     error
}
```

**Messages**: `agentsLoadedMsg{agents []smithers.Agent}`, `agentsErrorMsg{err error}` — private to this file.

**Behavior**:

| Method | Behavior |
|--------|----------|
| `Init()` | Fires async `tea.Cmd` calling `client.ListAgents(ctx)` → `agentsLoadedMsg` or `agentsErrorMsg` |
| `Update(agentsLoadedMsg)` | Stores agents, clears loading |
| `Update(agentsErrorMsg)` | Stores error, clears loading |
| `Update(tea.KeyPressMsg "up"/"k")` | Decrements cursor (clamped at 0) |
| `Update(tea.KeyPressMsg "down"/"j")` | Increments cursor (clamped at len-1) |
| `Update(tea.KeyPressMsg "esc")` | Returns `PopViewMsg` via `tea.Cmd` |
| `Update(tea.KeyPressMsg "r")` | Sets loading, re-fires `Init()` |
| `Update(tea.KeyPressMsg "enter")` | No-op (future: `feat-agents-native-tui-launch`) |
| `Update(tea.WindowSizeMsg)` | Stores width/height |
| `View()` | Renders header, agent list with cursor indicator, help bar |
| `Name()` | Returns `"agents"` |
| `ShortHelp()` | Returns `["[Enter] Launch", "[r] Refresh", "[Esc] Back"]` |

**Rendering** (matches 02-DESIGN.md §3.7 layout):

```
SMITHERS › Agents                                       [Esc] Back

▸ Claude Code
  Status: ○ unavailable

  Codex
  Status: ○ unavailable

  Gemini
  Status: ○ unavailable
  ...
```

Focused item uses `▸` prefix and `lipgloss.NewStyle().Bold(true)`. Unfocused items have plain `  ` prefix. Status uses `○` icon for all statuses in the scaffold; future tickets add colored `●` for available statuses.

**Why not use `internal/ui/list/`**: The agents list is small (6-9 items max, never paginated), so a simple slice + cursor is sufficient. Crush's `list.List` is a lazy-loading viewport-based component designed for chat messages (hundreds of items, variable height). Using it here would add unnecessary complexity and coupling to the chat-oriented rendering pipeline.

**Compile-time check**: `var _ View = (*AgentsView)(nil)` enforces interface compliance.

### Slice 5 — Route Integration (`internal/ui/model/ui.go`)

Wire the router, client, and agents view into the root Bubble Tea model.

**Changes to `internal/ui/model/ui.go`**:

1. **New state**: Add `uiSmithersView` to the `uiState` enum (after `uiChat`). This is the bridge between Crush's existing state machine and the new view router.

2. **New fields on `UI` struct**:
   ```go
   viewRouter     *views.Router
   smithersClient *smithers.Client
   ```
   Initialized in `New()`: `viewRouter: views.NewRouter()`, `smithersClient: smithers.NewClient()`.

3. **Message forwarding** (in the main `Update()` method):
   When `m.state == uiSmithersView`, forward `msg` to `m.viewRouter.Current().Update(msg)`. Handle `PopViewMsg` by popping the router and transitioning back to `uiChat`/`uiLanding`.

4. **Key handling** (in the `updateKeyMsg()` method):
   When `m.state == uiSmithersView`, delegate to the current view's `Update()`. The view handles its own keys (arrows, esc, r, enter).

5. **Draw delegation** (in `Draw()`):
   ```go
   case uiSmithersView:
       m.drawHeader(scr, layout.header)
       if current := m.viewRouter.Current(); current != nil {
           main := uv.NewStyledString(current.View())
           main.Draw(scr, layout.main)
       }
   ```
   This uses the same `uv.NewStyledString()` bridge that Crush uses for `initializeView()` and `landingView()`.

6. **ShortHelp delegation** (in `ShortHelp()`):
   When `uiSmithersView`, iterate `current.ShortHelp()` and convert to `key.Binding` entries for the help bar.

7. **Action handler**: On `dialog.ActionOpenAgentsView`, close the command dialog, push `NewAgentsView(m.smithersClient)` onto the router, and transition to `uiSmithersView`.

8. **PopViewMsg handler**: Pop the router. If `!m.viewRouter.HasViews()`, transition back to `uiChat` (or `uiLanding` if no session).

### Slice 6 — Command Palette Entry (`internal/ui/dialog/`)

**`internal/ui/dialog/actions.go`**: Add `ActionOpenAgentsView struct{}` to the action types.

**`internal/ui/dialog/commands.go`**: Add a command item in the command list:
```go
NewCommandItem(c.com.Styles, "agents", "Agents", "", ActionOpenAgentsView{}),
```

This makes `/agents` appear in the command palette (`/` or `Ctrl+P`). The `"agents"` slug is used for fuzzy matching, `"Agents"` is the display label.

### Slice 7 — Terminal E2E Test

Create a terminal E2E test modeled on the upstream `@microsoft/tui-test` harness pattern from `smithers/tests/tui.e2e.test.ts` and `smithers/tests/tui-helpers.ts`.

**File**: `tests/tui/helpers_test.go` (shared harness) + `tests/tui/agents_e2e_test.go`

**Go test harness** (mirroring `smithers/tests/tui-helpers.ts`):

```go
package tui_test

import (
    "bytes"
    "io"
    "os/exec"
    "regexp"
    "time"
)

const (
    defaultWaitTimeout = 10 * time.Second
    pollInterval       = 100 * time.Millisecond
)

// ansiRegexp strips ANSI escape sequences for text matching.
// Mirrors the regex in smithers/tests/tui-helpers.ts:
//   buffer.replace(/\x1B\[[0-9;]*[a-zA-Z]/g, '')
var ansiRegexp = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)

type TUITestInstance struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bytes.Buffer // accumulated, ANSI-stripped
    stderr *bytes.Buffer
}

func launchTUI(args ...string) (*TUITestInstance, error)
func (t *TUITestInstance) waitForText(text string, timeout ...time.Duration) error
func (t *TUITestInstance) waitForNoText(text string, timeout ...time.Duration) error
func (t *TUITestInstance) sendKeys(keys string)
func (t *TUITestInstance) snapshot() string
func (t *TUITestInstance) terminate() error
```

**Key implementation details from upstream harness**:

- `launchTUI()`: Spawns the TUI binary via `exec.Command` with `TERM=xterm-256color`, `COLORTERM=truecolor`, `LANG=en_US.UTF-8`. Starts goroutines to continuously read stdout/stderr into buffers, stripping ANSI codes. Sleeps 1s after spawn (matching upstream) to let the TUI initialize.
- `waitForText()`: Polls buffer every 100ms for up to 10s. On each poll, checks direct `strings.Contains()`, then falls back to whitespace-collapsed matching (mirroring upstream's two-pass strategy for handling UI reflow). On timeout, returns error including buffer snapshot for debugging.
- `waitForNoText()`: Inverse — polls until text is absent.
- `sendKeys()`: Writes raw bytes to stdin pipe. Special keys: `\x1b` = Esc, `\r` = Enter, `\x1b[A` = Up, `\x1b[B` = Down.
- `snapshot()`: Returns current buffer contents for debugging failed assertions (upstream writes these to files on failure).
- `terminate()`: Sends SIGTERM, waits briefly, then SIGKILL if needed.

**Test: `TestAgentsViewNavigation`**:

```go
func TestAgentsViewNavigation(t *testing.T) {
    tui, err := launchTUI()
    require.NoError(t, err)
    defer tui.terminate()

    // 1. Confirm TUI started
    require.NoError(t, tui.waitForText("SMITHERS"))

    // 2. Open command palette and navigate to agents
    tui.sendKeys("/agents\r")

    // 3. Confirm agents view rendered
    require.NoError(t, tui.waitForText("Agents"))
    require.NoError(t, tui.waitForText("Claude Code"))
    require.NoError(t, tui.waitForText("unavailable"))

    // 4. Navigate the list
    tui.sendKeys("\x1b[B") // Down arrow
    time.Sleep(200 * time.Millisecond)

    // 5. Return to chat
    tui.sendKeys("\x1b") // Esc
    require.NoError(t, tui.waitForNoText("Agents"))
}
```

**Run**: `go test ./tests/tui/... -run TestAgentsViewNavigation -timeout 30s -v`

### Slice 8 — VHS Happy-Path Recording Test

**File**: `tests/vhs/agents_view.tape`

```tape
# Agents View Happy Path
Output tests/vhs/agents_view.gif
Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 800
Set TypingSpeed 50ms

# Launch the TUI
Type "go run . 2>/dev/null"
Enter
Sleep 2s

# Navigate to agents view via command palette
Type "/agents"
Enter
Sleep 1s

# Verify the view rendered (visual inspection of GIF)
Sleep 500ms

# Navigate the agent list
Down
Sleep 300ms
Down
Sleep 300ms
Up
Sleep 300ms

# Return to chat
Escape
Sleep 1s
```

**Execution**: `vhs tests/vhs/agents_view.tape`

**CI integration**: The VHS tape produces `tests/vhs/agents_view.gif`. CI verifies: (a) `vhs` exits 0, (b) the GIF is non-empty (> 1KB), (c) optional: frame extraction + text detection via `ffmpeg` / `tesseract` for automated visual regression.

## Validation

### Automated Checks

| # | Command | What it validates |
|---|---------|-------------------|
| 1 | `go build ./...` | All new files compile; imports resolve correctly against `github.com/charmbracelet/crush` module |
| 2 | `go vet ./...` | No warnings on new code |
| 3 | `go test ./internal/smithers/... -v` | `ListAgents()` returns 6 agents, no error; `Agent` struct fields are populated |
| 4 | `go test ./internal/ui/views/... -v` | `AgentsView` satisfies `View` interface; `Init()` returns non-nil `tea.Cmd`; `Name()` returns `"agents"`; `ShortHelp()` returns 3 hints |
| 5 | `go test ./tests/tui/... -run TestAgentsViewNavigation -timeout 30s -v` | **Terminal E2E** (modeled on `smithers/tests/tui.e2e.test.ts` + `tui-helpers.ts`): TUI launches → `/agents` navigates to view → "Claude Code" and "unavailable" render → arrow keys move cursor → Esc pops view back to chat |
| 6 | `vhs tests/vhs/agents_view.tape && test -s tests/vhs/agents_view.gif` | **VHS recording**: tape runs without error; GIF output is non-empty |

### Manual Verification

1. `go run .` — launch the TUI.
2. Type `/agents` and press Enter → Agents view appears with header "SMITHERS › Agents" and 6 placeholder agents.
3. Press `↓` / `↑` → cursor indicator (`▸`) moves between agents.
4. Press `r` → "Loading agents..." flashes briefly, then list reappears.
5. Press `Esc` → returns to chat view.
6. Open command palette (`/` or `Ctrl+P`) → "Agents" appears as an option; selecting it opens the view.
7. Confirm the help bar at the bottom shows `[Enter] Launch  [r] Refresh  [Esc] Back` while in the agents view.

### Interface Compliance

Compile-time assertion in `agents.go`:
```go
var _ View = (*AgentsView)(nil)
```

### Test Harness Fidelity to Upstream

The Go test harness must match the behavioral contract of `smithers/tests/tui-helpers.ts`:

| Upstream behavior | Go equivalent |
|---|---|
| `Bun.spawn()` with `stdin: "pipe"`, `stdout: "pipe"` | `exec.Command()` with `cmd.StdinPipe()`, `cmd.StdoutPipe()` |
| `TERM=xterm-256color`, `COLORTERM=truecolor` env vars | `cmd.Env = append(os.Environ(), ...)` |
| ANSI stripping: `/\x1B\[[0-9;]*[a-zA-Z]/g` | `regexp.MustCompile(\`\x1B\[[0-9;]*[a-zA-Z]\`)` |
| Poll interval: 100ms | `pollInterval = 100 * time.Millisecond` |
| Default timeout: 10s | `defaultWaitTimeout = 10 * time.Second` |
| Whitespace-collapsed matching as fallback | `strings.Join(strings.Fields(text), " ")` comparison |
| 1s startup delay | `time.Sleep(1 * time.Second)` after spawn |

## Risks

### 1. View Router Does Not Exist Yet

**Risk**: `internal/ui/views/router.go` and the `View` interface are specified in 03-ENGINEERING.md §3.1.1 but the `internal/ui/views/` directory does not exist in the committed codebase. Crush uses a monolithic `UI` struct with state-based routing (`uiState` enum: `uiOnboarding`, `uiInitialize`, `uiLanding`, `uiChat`), not a view stack.

**Impact**: High — without the router, there's no way to push/pop the agents view.

**Mitigation**: This ticket creates the router (Slice 3). The design adds a new `uiSmithersView` state to the existing enum, bridging Crush's state machine with the new view stack. Chat stays as the primary `uiChat` state; all Smithers views route through `uiSmithersView` → `Router.Current()`. This is ~55 lines of new code and does not change existing behavior.

### 2. Smithers Client Package Does Not Exist Yet

**Risk**: `internal/smithers/` is specified in 03-ENGINEERING.md §2.3 but does not exist.

**Impact**: Medium — `ListAgents` needs a home, and the `UI` struct needs a `smithersClient` field.

**Mitigation**: This ticket creates the minimal package (Slice 1 + 2). Just `types.go` (Agent struct) and `client.go` (Client struct + ListAgents stub). No HTTP client, no SQLite wiring — those come in later tickets. The `Client` struct starts empty (no fields) and will grow `apiURL`, `apiToken`, `dbPath` fields per 03-ENGINEERING.md §3.1.3.

### 3. Crush Uses Ultraviolet Draw, Not String-Based View()

**Risk**: Crush's root model uses `Draw(scr uv.Screen, area uv.Rectangle)` (Ultraviolet screen buffer rendering), not `View() string`. The `View` interface defines `View() string`, but integrating this into the Ultraviolet pipeline requires a bridge.

**Impact**: Medium — rendering mismatch between the view system and the host model.

**Mitigation**: Bridge via `uv.NewStyledString(current.View()).Draw(scr, layout.main)` in the `Draw()` method's `uiSmithersView` case. This is the exact same pattern Crush already uses for `initializeView()` (line ~2035 in ui.go) and `landingView()` — proven and zero-risk. The agents view's lipgloss-styled output renders correctly through this bridge.

### 4. Command Palette Registration

**Risk**: Crush's command palette (`internal/ui/dialog/commands.go`) uses a specific registration pattern with `NewCommandItem()`. Adding `/agents` requires understanding the action dispatch pipeline (`dialog/actions.go` → `ui.go` Update switch).

**Impact**: Low — the pattern is well-established and other commands demonstrate it.

**Mitigation**: Follow the existing pattern: define `ActionOpenAgentsView struct{}` in `actions.go`, add `NewCommandItem(...)` in `commands.go`, handle the action in `ui.go`'s dialog action switch. This mirrors how existing commands like model selection and session management work.

### 5. Go Module Path

**Risk**: The current Go module is `github.com/charmbracelet/crush` (per `go.mod`). 03-ENGINEERING.md §1.2 specifies renaming to `github.com/anthropic/smithers-tui`, but this rename has not happened yet. New files must use the current module path.

**Impact**: Low — build failure if wrong.

**Mitigation**: All import paths in new files use `github.com/charmbracelet/crush/internal/smithers` and `github.com/charmbracelet/crush/internal/ui/views`. When the module rename happens (ticket `platform-smithers-rebrand`), a global find-replace updates all imports.

### 6. E2E Test Requires Built Binary

**Risk**: The terminal E2E test spawns the TUI as a subprocess. This requires either a pre-built binary or `go run .` startup, which is slower and may behave differently from the compiled binary.

**Impact**: Low-medium — test reliability depends on stable binary availability.

**Mitigation**: The test should `go build -o /tmp/smithers-tui-test .` in `TestMain()` and spawn that binary. This mirrors upstream's pattern of resolving the entry point path at test setup time. The built binary is deterministic and fast to launch. Alternative: use `go run .` with a longer startup timeout (3s instead of 1s).
