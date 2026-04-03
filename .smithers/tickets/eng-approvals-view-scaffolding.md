# Scaffold the approvals TUI view

## Metadata
- ID: eng-approvals-view-scaffolding
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Create the base `approvals` view and register it in the router with the `ctrl+a` keybinding.

## Acceptance Criteria

- Pressing `ctrl+a` navigates to the empty approvals view.
- Pressing `esc` returns to the previous view.

## Source Context

- internal/ui/views/approvals.go
- internal/ui/model/ui.go
- internal/ui/model/keys.go

## Implementation Notes

- Follow the new Router pattern defined in `03-ENGINEERING.md`. Add `Approvals` key to `keys.go`.

---

## Objective

Stand up the empty `approvals` view as the first non-chat view in the Smithers TUI, proving the Router/view-stack pattern defined in `03-ENGINEERING.md` §3.1.1. After this ticket, `ctrl+a` pushes an Approvals view onto the stack, `esc` pops it, and the foundation exists for follow-on tickets (`approvals-queue`, `approvals-inline-approve`, `approvals-inline-deny`, `approvals-context-display`, `approvals-recent-decisions`) to layer real content.

This ticket is intentionally narrow: it introduces the view router, the `View` interface, one keybinding, and one concrete view — nothing more. Data fetching, list rendering, and approve/deny actions are out of scope.

## Scope

### In scope

1. **`internal/ui/views/router.go`** — New `View` interface and `Router` struct with push/pop/current stack management, as specified in `03-ENGINEERING.md` §3.1.1.
2. **`internal/ui/views/approvals.go`** — Skeleton `ApprovalsView` implementing the `View` interface. Renders a static header (`SMITHERS › Approvals`) with an `[Esc] Back` hint and an empty body placeholder (`No pending approvals.`), matching the design wireframe in `02-DESIGN.md` §3.5.
3. **`internal/ui/model/keys.go`** — Add `Approvals key.Binding` to the global section of `KeyMap`, bound to `ctrl+a` with help text `"approvals"`.
4. **`internal/ui/model/ui.go`** — Integrate the `Router` into the `UI` struct: instantiate it in `New()` with the existing chat view as the bottom-of-stack, wire `ctrl+a` in the key handling path to `router.Push(approvals.New(...))`, wire `esc` (when not on chat) to `router.Pop()`, and delegate `View()`/`Draw()` to `router.Current()` when the active view is not chat.
5. **Unit tests** for the Router (push/pop/current/IsChat invariants).
6. **Terminal E2E test** exercising `ctrl+a → view renders → esc → back to chat`.
7. **VHS happy-path recording** capturing the same flow visually.

### Out of scope

- Approval data fetching, listing, or rendering (ticket `approvals-queue`).
- Inline approve/deny actions (tickets `approvals-inline-approve`, `approvals-inline-deny`).
- Approval context display (ticket `approvals-context-display`).
- Recent decisions section (ticket `approvals-recent-decisions`).
- Notification badges (ticket `approvals-pending-badges`).
- Any `internal/smithers/` client code — the view takes no data dependencies yet.
- The `approvalcard.go` component (`internal/ui/components/approvalcard.go`).

## Implementation Plan

### Slice 1: View interface and Router (`internal/ui/views/router.go`)

Create the `internal/ui/views/` package with `router.go`. This is the foundational piece that every subsequent view depends on.

**File: `internal/ui/views/router.go`**

```go
package views

import tea "charm.land/bubbletea/v2"

// View is the interface every Smithers TUI view must implement.
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Name() string
    // ShortHelp returns keybinding hints for the bottom help bar.
    ShortHelp() []string
}

// Router manages a stack of Views. The chat view is always
// at index 0 and can never be popped.
type Router struct {
    stack []View
    chat  View
}

func NewRouter(chat View) *Router {
    return &Router{
        stack: []View{chat},
        chat:  chat,
    }
}

func (r *Router) Push(v View) tea.Cmd {
    r.stack = append(r.stack, v)
    return v.Init()
}

func (r *Router) Pop() {
    if len(r.stack) > 1 {
        r.stack = r.stack[:len(r.stack)-1]
    }
}

func (r *Router) Current() View {
    return r.stack[len(r.stack)-1]
}

func (r *Router) IsChat() bool {
    return len(r.stack) == 1
}

func (r *Router) Depth() int {
    return len(r.stack)
}
```

This matches `03-ENGINEERING.md` §3.1.1 exactly. The `Depth()` helper is added for test assertions.

**Key decisions**:
- The `View` interface returns `(View, tea.Cmd)` from `Update` to allow views to replace themselves (e.g., navigation within a view). This mirrors the Bubble Tea model pattern.
- `Pop()` is a no-op when only the chat remains — the chat view is inescapable.
- `ShortHelp()` returns `[]string` (not `[]key.Binding`) to keep the interface simple and avoid coupling views to the key binding system.

### Slice 2: ApprovalsView skeleton (`internal/ui/views/approvals.go`)

**File: `internal/ui/views/approvals.go`**

```go
package views

import (
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
)

type ApprovalsView struct {
    width  int
    height int
}

func NewApprovals() *ApprovalsView {
    return &ApprovalsView{}
}

func (v *ApprovalsView) Name() string { return "Approvals" }

func (v *ApprovalsView) Init() tea.Cmd { return nil }

func (v *ApprovalsView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        v.width = msg.Width
        v.height = msg.Height
    }
    return v, nil
}

func (v *ApprovalsView) View() string {
    header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Approvals") +
        "  " +
        lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
    body := lipgloss.NewStyle().Faint(true).Render("No pending approvals.")
    return header + "\n\n" + body
}

func (v *ApprovalsView) ShortHelp() []string {
    return []string{"esc back"}
}
```

This is intentionally minimal. The view:
- Handles `WindowSizeMsg` so it knows its dimensions for when content is added later.
- Renders a static header matching the wireframe in `02-DESIGN.md` §3.5: `SMITHERS › Approvals ... [Esc] Back`.
- Shows a placeholder body: `No pending approvals.`
- Returns help hints for the bottom bar.
- Does not take a `*smithers.Client` yet (no data dependency). Follow-on tickets will add it.

### Slice 3: Keybinding registration (`internal/ui/model/keys.go`)

Add the `Approvals` binding to the global section of `KeyMap`.

**Current global keys** (lines 59-67 of `keys.go`):
```go
// Global key maps
Quit     key.Binding
Help     key.Binding
Commands key.Binding
Models   key.Binding
Suspend  key.Binding
Sessions key.Binding
Tab      key.Binding
```

**Change**: Add after `Sessions`:
```go
Approvals key.Binding
```

**In `DefaultKeyMap()`**, add after the `Sessions` binding (line 94):
```go
km.Approvals = key.NewBinding(
    key.WithKeys("ctrl+a"),
    key.WithHelp("ctrl+a", "approvals"),
)
```

**Conflict check**: `ctrl+a` is not currently bound in Crush's `DefaultKeyMap()`. The only `ctrl+` bindings in use are: `ctrl+c` (quit), `ctrl+g` (help), `ctrl+p` (commands), `ctrl+m`/`ctrl+l` (models), `ctrl+z` (suspend), `ctrl+s` (sessions), `ctrl+o` (editor), `ctrl+f` (add image), `ctrl+v` (paste), `ctrl+r` (delete attachment), `ctrl+n` (new session), `ctrl+d` (details), `ctrl+t`/`ctrl+space` (toggle tasks), `ctrl+j` (navigation/newline). `ctrl+a` is free.

**Note on `ctrl+r`**: The design doc mentions `ctrl+r` for the runs dashboard, but Crush already uses `ctrl+r` for attachment delete mode. This conflict will be addressed in the runs scaffolding ticket (`eng-runs-view-scaffolding`), not here. `ctrl+a` for approvals is conflict-free.

### Slice 4: Router integration into UI model (`internal/ui/model/ui.go`)

This is the most surgical slice — modifying the main UI model to delegate to the router.

**4a. Add Router field to `UI` struct** (around line 147):
```go
import "github.com/charmbracelet/crush/internal/ui/views"

type UI struct {
    // ... existing fields ...
    router *views.Router
}
```

**4b. Initialize Router in `New()`**:
The router needs a `View` wrapping the chat. Since Crush's chat is embedded in the `UI` struct itself (not a separate component), we create a thin `chatViewAdapter` that wraps the existing chat rendering behind the `View` interface:

```go
// chatViewAdapter wraps the existing UI chat mode as a View for the router.
type chatViewAdapter struct {
    ui *UI
}

func (c *chatViewAdapter) Name() string            { return "Chat" }
func (c *chatViewAdapter) Init() tea.Cmd            { return nil }
func (c *chatViewAdapter) Update(tea.Msg) (views.View, tea.Cmd) { return c, nil }
func (c *chatViewAdapter) View() string             { return "" } // chat rendering handled by UI directly
func (c *chatViewAdapter) ShortHelp() []string      { return nil }
```

In `New()`:
```go
ui := &UI{/* existing init */}
ui.router = views.NewRouter(&chatViewAdapter{ui: ui})
```

**4c. Wire `ctrl+a` in key handling**:
In the key press handling path (the large `switch` in `Update()` or `handleKeyPressMsg()`), add a case in the global key handling section (before state/focus-specific handling):

```go
case key.Matches(msg, m.keyMap.Approvals):
    if m.router.IsChat() { // only navigate from chat
        cmd := m.router.Push(views.NewApprovals())
        return m, cmd
    }
```

**4d. Wire `esc` to pop when not on chat**:
In the existing `esc` handling, add a guard:

```go
// Before existing esc handling:
if !m.router.IsChat() {
    m.router.Pop()
    return m, nil
}
// ... existing esc behavior (cancel, clear highlight, etc.)
```

This must come before the existing `esc` logic so that when a non-chat view is active, `esc` pops the view rather than triggering chat-specific esc behavior (cancel agent, clear highlight, etc.).

**4e. Delegate rendering to current view**:
In `View()` (or `Draw()`), check if the current view is chat:

```go
if !m.router.IsChat() {
    return m.router.Current().View()
}
// ... existing chat rendering
```

For the `Draw()` (Ultraviolet) path, the same guard applies — render the current view's string output into the screen area.

**4f. Forward messages to the active view**:
When the router is not on chat, messages (particularly `tea.WindowSizeMsg`, `tea.KeyPressMsg`) should be forwarded to the current view:

```go
if !m.router.IsChat() {
    updated, cmd := m.router.Current().Update(msg)
    m.router.stack[len(m.router.stack)-1] = updated
    return m, cmd
}
```

This requires either making `stack` exported or adding a `SetCurrent(v View)` method to the router. Prefer the method:

```go
// In router.go
func (r *Router) SetCurrent(v View) {
    r.stack[len(r.stack)-1] = v
}
```

### Slice 5: Router unit tests (`internal/ui/views/router_test.go`)

```go
package views_test

func TestRouterStartsWithChat(t *testing.T)      // IsChat() == true, Depth() == 1
func TestRouterPush(t *testing.T)                  // Push → IsChat() == false, Depth() == 2, Current() == pushed
func TestRouterPop(t *testing.T)                   // Push → Pop → IsChat() == true, Current() == chat
func TestRouterPopAtBottomIsNoop(t *testing.T)     // Pop on depth-1 → no panic, IsChat() still true
func TestRouterCurrentReturnsTopOfStack(t *testing.T) // Push A → Push B → Current() == B
func TestRouterSetCurrent(t *testing.T)            // Push → SetCurrent(replacement) → Current() == replacement
```

Use a minimal `stubView` that implements `View` with no-op methods. These tests validate the router invariants independently of the TUI.

### Slice 6: Terminal E2E test

Model the test on the upstream Smithers harness in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`. The upstream harness:

- Spawns the TUI process (`bun run src/cli/index.ts tui`)
- Aggregates stdout into a buffer, strips ANSI codes
- Provides `waitForText(text, timeout)` — polls the buffer for a string match
- Provides `sendKeys(text)` — writes to stdin
- Provides `snapshot()` — returns the full buffer for debugging
- Dumps buffer to file on failure for CI debugging

**For Crush/Smithers TUI (Go)**, create an analogous test helper:

**File: `tests/tui_helpers_test.go`** (or `internal/ui/views/approvals_e2e_test.go` with build tag `e2e`):

```go
// TUITestInstance wraps a running TUI process for E2E assertions.
type TUITestInstance struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    buffer *bytes.Buffer  // aggregated stdout, ANSI-stripped
    mu     sync.Mutex
}

func launchTUI(args ...string) (*TUITestInstance, error)
func (t *TUITestInstance) WaitForText(text string, timeout time.Duration) error
func (t *TUITestInstance) SendKeys(keys string) error
func (t *TUITestInstance) Snapshot() string
func (t *TUITestInstance) Terminate() error
```

**Test: `TestApprovalsViewE2E`**:

```go
func TestApprovalsViewE2E(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    tui, err := launchTUI()
    require.NoError(t, err)
    defer tui.Terminate()

    // Wait for initial chat view
    err = tui.WaitForText("Ready", 10*time.Second)
    require.NoError(t, err, "TUI should show chat view on startup; buffer: %s", tui.Snapshot())

    // Press ctrl+a to navigate to approvals
    tui.SendKeys("\x01") // ctrl+a

    // Verify approvals view renders
    err = tui.WaitForText("Approvals", 5*time.Second)
    require.NoError(t, err, "ctrl+a should navigate to approvals view; buffer: %s", tui.Snapshot())

    err = tui.WaitForText("No pending approvals", 5*time.Second)
    require.NoError(t, err, "approvals view should show placeholder; buffer: %s", tui.Snapshot())

    // Press esc to return to chat
    tui.SendKeys("\x1b") // esc

    // Verify back on chat
    err = tui.WaitForText("Ready", 5*time.Second)
    require.NoError(t, err, "esc should return to chat view; buffer: %s", tui.Snapshot())
}
```

**Key differences from upstream harness**:
- Written in Go (not TypeScript/Bun) since Crush is a Go project.
- Uses `exec.Command` to spawn the built binary.
- ANSI stripping via a regex filter on the stdout reader goroutine.
- Polling-based `WaitForText` with configurable timeout (same approach as upstream's 100ms poll interval).
- Snapshot dump on assertion failure (mirrors upstream `fs.writeFileSync("tui-buffer.txt", tui.snapshot())`).
- Gated behind `testing.Short()` skip or a build tag to keep `go test ./...` fast.

### Slice 7: VHS happy-path recording test

VHS (https://github.com/charmbracelet/vhs) records terminal sessions as GIFs/MP4s from a `.tape` script. Crush already uses this pattern for demo recordings.

**File: `tests/vhs/approvals-scaffolding.tape`**:

```tape
# Approvals view scaffolding — happy path
Output tests/vhs/approvals-scaffolding.gif
Set FontSize 14
Set Width 120
Set Height 30
Set Shell zsh

# Launch the TUI
Type "smithers-tui"
Enter
Sleep 3s

# Navigate to approvals
Ctrl+a
Sleep 1s

# Verify the view is visible (visual assertion — the GIF is the proof)
Screenshot tests/vhs/approvals-scaffolding-view.png

# Return to chat
Escape
Sleep 1s

# Verify back on chat
Screenshot tests/vhs/approvals-scaffolding-chat.png
```

**CI integration**: Run `vhs tests/vhs/approvals-scaffolding.tape` and assert exit code 0. The generated GIF serves as a visual regression artifact. Compare screenshots against golden files if pixel-level regression is desired (optional for scaffolding).

**Validation**: The tape file itself serves as a runnable test. If the TUI crashes or the view fails to render, VHS will error on the `Sleep` or `Screenshot` step (non-zero exit).

## Validation

### Automated checks

| Check | Command | What it proves |
|-------|---------|----------------|
| Router unit tests pass | `go test ./internal/ui/views/ -run TestRouter -v` | Push/pop/current stack invariants hold |
| Build succeeds | `go build ./...` | New files compile, imports resolve, no cycles |
| Existing tests pass | `go test ./...` | No regressions in chat, dialog, or key handling |
| Terminal E2E: approvals flow | `go test ./tests/ -run TestApprovalsViewE2E -timeout 30s` (or equivalent path) | `ctrl+a` renders the approvals view, `esc` returns to chat in a real terminal session |
| VHS recording test | `vhs tests/vhs/approvals-scaffolding.tape` (exit code 0) | Happy-path flow completes without crash; GIF artifact produced |

### Manual verification

1. **Build and run**: `go build -o smithers-tui . && ./smithers-tui`
2. **Chat loads**: Confirm the default chat view renders (Smithers branding, model info, textarea).
3. **`ctrl+a`**: Confirm the screen clears and shows `SMITHERS › Approvals` with `[Esc] Back` and `No pending approvals.`
4. **`esc`**: Confirm you return to the chat view with all state intact (messages, textarea content, scroll position).
5. **Repeat**: Press `ctrl+a` again — confirm the approvals view renders again (router can push multiple times cleanly).
6. **Dialog interaction**: Open a dialog (`ctrl+p`), confirm `ctrl+a` does not navigate while a dialog is open (dialog consumes keys first).
7. **Resize**: While on the approvals view, resize the terminal — confirm no crash, view re-renders.

### Terminal E2E coverage (modeled on upstream harness)

The E2E test in Slice 6 above directly models the pattern from:

- **`../smithers/tests/tui-helpers.ts`**: `launchTUI()` spawning pattern, `waitForText()` polling, `sendKeys()` stdin writing, `snapshot()` buffer dump.
- **`../smithers/tests/tui.e2e.test.ts`**: try/catch with snapshot dump on failure, `finally` block for terminate, timeout per test.

The Go translation preserves:
- Process spawn with `TERM=xterm-256color` env for consistent rendering.
- ANSI stripping for reliable text matching.
- Polling with 100ms interval and configurable timeout.
- Snapshot dump in assertion failure messages for CI debugging.
- Cleanup via `defer tui.Terminate()`.

### VHS recording test

The VHS tape in Slice 7 provides a visual happy-path test that:
- Launches the real TUI binary.
- Sends `ctrl+a`, captures a screenshot of the approvals view.
- Sends `esc`, captures a screenshot of the chat view.
- Produces a GIF artifact for visual inspection/regression.
- Exits non-zero if the TUI crashes at any point.

## Risks

### 1. `esc` key conflict with existing chat behavior

**Risk**: Crush's chat view uses `esc` for multiple purposes — cancel the running agent, clear text highlight, exit attachment delete mode. Adding `esc` as the back-navigation key requires careful ordering.

**Mitigation**: The router `IsChat()` guard must be checked **before** all existing `esc` handlers. When a non-chat view is active, `esc` always pops the view stack. The existing `esc` behaviors only fire when the chat view is active. This is a clean separation because the non-chat views handle their own `esc` internally (or don't), and the router pop is the only `esc` action at the UI model level for non-chat views.

**Verification**: The E2E test explicitly checks that `esc` returns to chat. Manual testing should also verify that `esc` still cancels the agent and clears highlights when on the chat view.

### 2. Ultraviolet (Direct Draw) vs. String Rendering divergence

**Risk**: Crush uses both `View() string` (traditional Bubble Tea) and `Draw(scr uv.Screen, area uv.Rectangle)` (Ultraviolet direct rendering). The `View` interface in the router uses `View() string`, but the main UI model uses `Draw()`. If the `Draw()` path is active, the router's `View()` output might be ignored.

**Mitigation**: In `UI.Draw()`, add the same `!m.router.IsChat()` guard. When a non-chat view is active, render its `View()` string output into the Ultraviolet screen area using `screen.WriteString()` or equivalent. Alternatively, extend the `View` interface to include an optional `Draw(scr uv.Screen, area uv.Rectangle)` method (with a default implementation that renders `View()` as a string). The scaffolding view is simple enough that string rendering suffices; Ultraviolet integration can be refined later.

### 3. Message routing when non-chat view is active

**Risk**: The `UI.Update()` method is a large switch that routes messages to chat sub-components (textarea, chat list, completions, dialogs). When the approvals view is active, these components should not receive keypresses, or they'll process input meant for the approvals view.

**Mitigation**: The `!m.router.IsChat()` guard at the top of key handling ensures that when a non-chat view is active, keypresses are routed to the current view's `Update()` method, not to the chat components. Only `ctrl+c` (quit) and `ctrl+z` (suspend) should remain global regardless of active view. The guard order is: quit/suspend → dialog → router (non-chat) → chat key handling.

### 4. No existing `internal/ui/views/` package

**Risk**: Crush has no `internal/ui/views/` directory. Creating a new package introduces a dependency from `internal/ui/model/` → `internal/ui/views/`. This is a new import path that could create import cycles if `views` needs access to `common`, `styles`, or other UI packages.

**Mitigation**: The `views` package should depend on `common` and `styles` (downstream), never on `model` (upstream). The `chatViewAdapter` that bridges the existing chat into the `View` interface lives in the `model` package (since it references the `UI` struct), keeping the dependency direction clean: `model → views → common/styles`.

### 5. `ctrl+a` conflicts on some systems

**Risk**: `ctrl+a` is the default `tmux` prefix key and the "select all" shortcut in some terminal contexts. Users running inside `tmux` with the default prefix will not be able to reach the approvals view via `ctrl+a`.

**Mitigation**: This matches the design doc's specification (`ctrl+a` for approvals). The command palette (`ctrl+p` → `/approvals`) provides an alternative navigation path that works regardless of terminal prefix keys. Document the `tmux` conflict in the help text. A future ticket can add user-configurable keybindings if this becomes a frequent issue.

### 6. Crush-Smithers mismatch: no view system in upstream Crush

**Impact**: Crush has no concept of multiple views or a view router. The entire TUI is a single `UI` model with state-based rendering (`uiOnboarding`, `uiInitialize`, `uiLanding`, `uiChat`). The Router pattern is new to the Smithers fork and does not exist upstream.

**Consequence**: This ticket establishes a divergence point from upstream Crush. Future cherry-picks from upstream that modify `ui.go`'s key handling or rendering will need manual conflict resolution around the router integration points. Keep the router integration surface in `ui.go` as small as possible (ideally < 20 lines of new code) to minimize merge conflicts.
