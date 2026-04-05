# Implementation Plan: platform-view-model

## Goal

Harden the Smithers TUI view platform from its current scaffolding state into a production-grade view system. The changes cover:

1. Strengthening the `View` interface with explicit lifecycle methods (resize, focus, blur).
2. Wiring `WindowSizeMsg` propagation reliably through the router.
3. Fixing the `ShortHelp` type mismatch so view help integrates cleanly with Bubble Tea's `help` component.
4. Adding a view registry to decouple view construction from the root model.
5. Wiring global keybindings (`ctrl+r`, `ctrl+a`) to open their respective views.
6. Wiring the Smithers config into the client at construction time.
7. Introducing a lightweight workspace model that provides live run/approval counts to the header.
8. Establishing unit and E2E tests for the view platform.

This is the foundational platform ticket. Every subsequent view (Runs, Workflows, Prompts, SQL Browser, Triggers) depends on the interfaces defined here being stable and correct.

---

## Steps

### Step 1: Strengthen the `View` Interface

**File**: `internal/ui/views/router.go`

Change `ShortHelp() []string` to `ShortHelp() []key.Binding`. This requires importing `charm.land/bubbles/v2/key`.

Add an explicit `SetSize(width, height int)` method to the `View` interface. All views must implement it. The implementation is always the same two-liner (`v.width = width; v.height = height`), so the boilerplate cost is low and the payoff is reliable resize propagation.

The updated interface:

```go
import "charm.land/bubbles/v2/key"

type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Name() string
    SetSize(width, height int)
    ShortHelp() []key.Binding
}
```

Optionally define two companion interfaces for views that need focus/blur lifecycle. These are **not** added to `View` (to avoid forcing every view to implement them) but the router checks for them at push/pop time:

```go
// Focusable is implemented by views that need focus/blur lifecycle callbacks.
type Focusable interface {
    OnFocus() tea.Cmd
    OnBlur() tea.Cmd
}
```

Update `Router.Push` to:
1. Accept current terminal dimensions: `func (r *Router) Push(v View, width, height int) tea.Cmd`.
2. Call `v.SetSize(width, height)` before calling `v.Init()`.
3. If the previous top-of-stack view implements `Focusable`, call `OnBlur()` and batch that command.
4. If the new view implements `Focusable`, call `OnFocus()` after `Init()`.

Update `Router.Pop` to:
1. If the outgoing view implements `Focusable`, call `OnBlur()` and return that command.
2. If the new top-of-stack (after pop) implements `Focusable`, call `OnFocus()`.

`Pop` should return `tea.Cmd` (currently returns `bool`) so it can propagate the blur/focus commands back to the root model's `cmds` slice.

Update `Router.Update` — add a forwarding method so the router can be called as a delegatee:

```go
// Update forwards msg to the current view and replaces it in the stack if it changed.
func (r *Router) Update(msg tea.Msg) tea.Cmd {
    current := r.Current()
    if current == nil {
        return nil
    }
    updated, cmd := current.Update(msg)
    if updated != current {
        r.stack[len(r.stack)-1] = updated
    }
    return cmd
}
```

This eliminates the awkward `Pop()`+`Push(updated)` pattern used in `ui.go` lines 910–913.

Add `SetSize` to the router so `WindowSizeMsg` can be forwarded cleanly:

```go
func (r *Router) SetSize(width, height int) {
    r.width = width
    r.height = height
    if current := r.Current(); current != nil {
        current.SetSize(width, height)
    }
}
```

Add `width` and `height` fields to the `Router` struct so it remembers the terminal dimensions for use when a new view is pushed.

### Step 2: Update Existing Views

**Files**: `internal/ui/views/agents.go`, `approvals.go`, `tickets.go`

For each view:

1. Add `SetSize(width, height int)` implementation:
   ```go
   func (v *AgentsView) SetSize(width, height int) {
       v.width = width
       v.height = height
   }
   ```
   Remove the `case tea.WindowSizeMsg:` branch from `Update` — size is now set by the router, not by message forwarding. (Keeping it as a fallback is also acceptable; it just means the view handles resize from two paths, which is harmless.)

2. Change `ShortHelp() []string` to `ShortHelp() []key.Binding`:
   ```go
   func (v *AgentsView) ShortHelp() []key.Binding {
       return []key.Binding{
           key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "launch")),
           key.NewBinding(key.WithKeys("r"),     key.WithHelp("r",     "refresh")),
           key.NewBinding(key.WithKeys("esc"),   key.WithHelp("esc",   "back")),
       }
   }
   ```
   Do the same for `ApprovalsView` and `TicketsView`.

3. Extract shared helpers from `approvals.go` into `internal/ui/views/helpers.go` (new file):
   - `padRight(s string, width int) string`
   - `truncate(s string, maxLen int) string`
   - `formatStatus(status string) string`
   - `formatPayload(payload string, width int) string`
   - `wrapText(s string, width int) string`
   - `ticketSnippet(content string) string` (from `tickets.go`)

   These utilities will be reused by the many views that follow (Runs, Workflows, Prompts, etc.).

### Step 3: Update the Root Model's View Dispatching

**File**: `internal/ui/model/ui.go`

**3a. Explicit `WindowSizeMsg` forwarding**

In the `case tea.WindowSizeMsg:` arm (line ~674), add an explicit call after `m.updateLayoutAndSize()`:

```go
case tea.WindowSizeMsg:
    m.width, m.height = msg.Width, msg.Height
    m.updateLayoutAndSize()
    // Propagate size to active Smithers view.
    m.viewRouter.SetSize(m.width, m.height)
    // ... rest of existing code
```

**3b. Replace the awkward double-push forwarding pattern**

The current pattern at lines 903–914 is:
```go
if m.state == uiSmithersView {
    if current := m.viewRouter.Current(); current != nil {
        updated, cmd := current.Update(msg)
        if cmd != nil { cmds = append(cmds, cmd) }
        if updated != current {
            m.viewRouter.Pop()
            m.viewRouter.Push(updated)   // BUG: passes no dimensions
        }
    }
}
```

Replace with:
```go
if m.state == uiSmithersView {
    if cmd := m.viewRouter.Update(msg); cmd != nil {
        cmds = append(cmds, cmd)
    }
}
```

This uses the new `Router.Update` method from Step 1.

**3c. Update Push call sites**

The three `ActionOpen*View` cases currently call `m.viewRouter.Push(view)`. Update them to pass dimensions:

```go
case dialog.ActionOpenAgentsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    cmd := m.viewRouter.Push(views.NewAgentsView(m.smithersClient), m.width, m.height)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

**3d. Update Pop call site**

`PopViewMsg` is handled at line ~1474. Update it to collect the blur command:

```go
case views.PopViewMsg:
    if cmd := m.viewRouter.Pop(); cmd != nil {
        cmds = append(cmds, cmd)
    }
    if !m.viewRouter.HasViews() {
        if m.hasSession() {
            m.setState(uiChat, uiFocusEditor)
        } else {
            m.setState(uiLanding, uiFocusEditor)
        }
    }
```

**3e. Update ShortHelp rendering**

At lines ~2318–2323, the help bar currently wraps `[]string` hints into empty bindings:

```go
case uiSmithersView:
    if current := m.viewRouter.Current(); current != nil {
        for _, hint := range current.ShortHelp() {
            binds = append(binds, key.NewBinding(key.WithHelp("", hint)))
        }
    }
```

With `ShortHelp() []key.Binding`, this becomes:

```go
case uiSmithersView:
    if current := m.viewRouter.Current(); current != nil {
        binds = append(binds, current.ShortHelp()...)
    }
```

**3f. Wire global keybindings**

In the `uiChat` key-handling arm (or the global key arm), add cases for the two existing Smithers shortcuts that are currently defined but not dispatched:

```go
case key.Matches(msg, m.keyMap.RunDashboard):
    // TODO: push RunsView when it exists. For now, open Approvals as placeholder.
    // Replace with views.NewRunsView(m.smithersClient) once the Runs view is implemented.
    cmds = append(cmds, func() tea.Msg { return dialog.ActionOpenApprovalsView{} })

case key.Matches(msg, m.keyMap.Approvals):
    cmds = append(cmds, func() tea.Msg { return dialog.ActionOpenApprovalsView{} })
```

Once `RunsView` is implemented, replace the placeholder.

### Step 4: Wire Smithers Config into Client Construction

**File**: `internal/ui/model/ui.go` (the `New` function, line ~342) and `internal/config/config.go`.

Replace `smithers.NewClient()` with a function that reads from config:

```go
func smithersClientFromConfig(cfg *config.Config) *smithers.Client {
    if cfg == nil || cfg.Smithers == nil {
        return smithers.NewClient()
    }
    opts := []smithers.ClientOption{}
    if cfg.Smithers.APIUrl != "" {
        opts = append(opts, smithers.WithAPIURL(cfg.Smithers.APIUrl))
    }
    if cfg.Smithers.APIToken != "" {
        opts = append(opts, smithers.WithAPIToken(cfg.Smithers.APIToken))
    }
    if cfg.Smithers.DBPath != "" {
        opts = append(opts, smithers.WithDBPath(cfg.Smithers.DBPath))
    }
    return smithers.NewClient(opts...)
}
```

Call this in `UI.New()`:

```go
smithersClient: smithersClientFromConfig(com.Config()),
```

This is a standalone change that makes all views functional with real data immediately.

### Step 5: Add View Registry

**File**: `internal/ui/views/registry.go` (new)

```go
package views

import "github.com/charmbracelet/crush/internal/smithers"

// ViewFactory constructs a View given a Smithers client.
type ViewFactory func(client *smithers.Client) View

// Registry maps route names to view factories.
type Registry struct {
    factories map[string]ViewFactory
}

func NewRegistry() *Registry {
    return &Registry{factories: make(map[string]ViewFactory)}
}

func (r *Registry) Register(name string, f ViewFactory) {
    r.factories[name] = f
}

func (r *Registry) Open(name string, client *smithers.Client) (View, bool) {
    f, ok := r.factories[name]
    if !ok {
        return nil, false
    }
    return f(client), true
}

func (r *Registry) Names() []string {
    names := make([]string, 0, len(r.factories))
    for n := range r.factories {
        names = append(names, n)
    }
    sort.Strings(names)
    return names
}
```

Register all existing views in a `DefaultRegistry` function:

```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    r.Register("agents",    func(c *smithers.Client) View { return NewAgentsView(c) })
    r.Register("approvals", func(c *smithers.Client) View { return NewApprovalsView(c) })
    r.Register("tickets",   func(c *smithers.Client) View { return NewTicketsView(c) })
    return r
}
```

Add the registry to the root `UI` struct and initialize it in `New()`. Update the command palette and action handlers to use `registry.Open(name, client)` instead of direct constructor calls. This reduces each new view to a single `r.Register(...)` line.

To avoid a large refactor in one pass, the registry and direct-constructor paths can coexist during the transition: the registry is available for new views, while existing views keep their direct action handling until a cleanup pass.

### Step 6: Introduce Workspace Model

**File**: `internal/ui/workspace/model.go` (new)

Define the types:

```go
package workspace

import tea "charm.land/bubbletea/v2"

// ConnectionState describes the Smithers server connection.
type ConnectionState int

const (
    ConnectionUnknown ConnectionState = iota
    ConnectionConnected
    ConnectionDisconnected
)

// WorkspaceState holds live Smithers runtime metrics.
type WorkspaceState struct {
    ActiveRunCount      int
    PendingApprovalCount int
    ConnectionState     ConnectionState
}

// WorkspaceUpdateMsg is emitted when workspace state changes.
type WorkspaceUpdateMsg struct {
    State WorkspaceState
}
```

Define a `Model` that owns a polling loop:

```go
type Model struct {
    client   *smithers.Client
    state    WorkspaceState
    interval time.Duration
}

func New(client *smithers.Client) *Model {
    return &Model{
        client:   client,
        interval: 10 * time.Second,
    }
}

// Init starts the polling loop.
func (m *Model) Init() tea.Cmd {
    return m.poll()
}

func (m *Model) poll() tea.Cmd {
    return tea.Tick(m.interval, func(_ time.Time) tea.Msg {
        // Fetch counts from client.
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        runs, _ := m.client.ListRuns(ctx)       // returns []Run or error
        approvals, _ := m.client.ListPendingApprovals(ctx)
        connected := runs != nil // if we got data, server is reachable
        state := WorkspaceState{
            ActiveRunCount:       len(runs),
            PendingApprovalCount: len(approvals),
        }
        if connected {
            state.ConnectionState = ConnectionConnected
        } else {
            state.ConnectionState = ConnectionDisconnected
        }
        return WorkspaceUpdateMsg{State: state}
    })
}

// Update handles a WorkspaceUpdateMsg and schedules the next poll.
func (m *Model) Update(msg WorkspaceUpdateMsg) (Model, tea.Cmd) {
    m.state = msg.State
    return *m, m.poll()
}

func (m *Model) State() WorkspaceState { return m.state }
```

Add `workspace *workspace.Model` to `UI` struct. In `UI.Init()` or `UI.New()`, initialize it:

```go
m.workspace = workspace.New(m.smithersClient)
```

Handle `WorkspaceUpdateMsg` in the root `Update` function to update `m.workspace`. Pass `m.workspace.State()` to the header/status rendering code.

**Note**: `ListRuns` does not yet exist on `internal/smithers/client.go`. The workspace model can be scaffolded now using only `ListPendingApprovals` (which does exist), and `ListRuns` added when the Runs view ticket is implemented.

### Step 7: Expand the Command Palette

**File**: `internal/ui/dialog/commands.go` and `internal/ui/dialog/actions.go`

Add the following Smithers navigation commands to the palette (in addition to the three that already exist):

| Command key | Label | Action |
|-------------|-------|--------|
| `runs` | Runs | `ActionOpenView{Name: "runs"}` |
| `workflows` | Workflows | `ActionOpenView{Name: "workflows"}` |
| `prompts` | Prompts | `ActionOpenView{Name: "prompts"}` |
| `sql` | SQL Browser | `ActionOpenView{Name: "sql"}` |
| `triggers` | Triggers | `ActionOpenView{Name: "triggers"}` |

Rather than defining a separate action type for each view, introduce a single generic action:

```go
// ActionOpenView opens the named view via the view registry.
type ActionOpenView struct {
    Name string
}
```

Handle it in `ui.go`:

```go
case dialog.ActionOpenView:
    m.dialog.CloseDialog(dialog.CommandsID)
    if v, ok := m.viewRegistry.Open(msg.Name, m.smithersClient); ok {
        cmd := m.viewRouter.Push(v, m.width, m.height)
        m.setState(uiSmithersView, uiFocusMain)
        cmds = append(cmds, cmd)
    }
```

Migrate the three existing `ActionOpen*View` types to use `ActionOpenView` in a follow-up cleanup. For now they can coexist.

---

## File Plan

| File | Change |
|------|--------|
| `internal/ui/views/router.go` | Add `SetSize` to `View`; add `Focusable` interface; update `Router` with `width`/`height` fields, `SetSize`, improved `Push(v, w, h)`, `Pop() tea.Cmd`, `Update(msg)` |
| `internal/ui/views/helpers.go` | New — extract shared helpers from `approvals.go` and `tickets.go` |
| `internal/ui/views/registry.go` | New — `ViewFactory`, `Registry`, `DefaultRegistry()` |
| `internal/ui/views/agents.go` | Add `SetSize`; change `ShortHelp` return type to `[]key.Binding` |
| `internal/ui/views/approvals.go` | Add `SetSize`; change `ShortHelp` return type; remove local helpers (moved to `helpers.go`) |
| `internal/ui/views/tickets.go` | Add `SetSize`; change `ShortHelp` return type; remove `ticketSnippet` (moved to `helpers.go`) |
| `internal/ui/model/ui.go` | Wire config to client; use `router.SetSize` in `WindowSizeMsg`; use `router.Update` in forwarding; update push call sites to pass dimensions; wire `ctrl+r`/`ctrl+a` key cases; handle `ActionOpenView`; add `viewRegistry` field; add `workspace` field |
| `internal/ui/dialog/commands.go` | Add Smithers navigation commands |
| `internal/ui/dialog/actions.go` | Add `ActionOpenView{Name string}` |
| `internal/ui/workspace/model.go` | New — `WorkspaceState`, `WorkspaceUpdateMsg`, `Model` |
| `internal/ui/views/router_test.go` | New — unit tests for push/pop/resize/focus |
| `internal/ui/views/agents_test.go` | New — unit tests for `AgentsView` update cycles |
| `internal/e2e/platform_view_model_test.go` | New — E2E: command palette → view → Esc → chat |
| `tests/vhs/platform-view-navigation.tape` | New — VHS happy-path recording |

---

## Validation

### Compilation

```bash
go build ./...
```

All three existing views must compile with the new interface. The compiler will flag any missing `SetSize` or `ShortHelp() []key.Binding` implementations.

### Unit Tests: Router

```bash
go test ./internal/ui/views/... -v -run TestRouter
```

Expected coverage:
- `Push` calls `SetSize` with correct dimensions before `Init`.
- `Push` calls `OnFocus` on the new view if it is `Focusable`.
- `Pop` calls `OnBlur` on the outgoing view and `OnFocus` on the newly-exposed view.
- `SetSize` propagates to `Current()`.
- `Update` replaces the current view in-stack without pop+push.
- `HasViews` returns false after popping the last view.

### Unit Tests: Views

```bash
go test ./internal/ui/views/... -v -run TestAgentsView
go test ./internal/ui/views/... -v -run TestApprovalsView
go test ./internal/ui/views/... -v -run TestTicketsView
```

Expected coverage:
- `Init` returns a non-nil cmd that results in a loaded/error message.
- `Update` with `agentsLoadedMsg` sets `loading = false` and `agents`.
- `Update` with `tea.WindowSizeMsg` (if kept) sets width/height correctly.
- `View()` renders loading state, error state, empty state, and populated state.
- `ShortHelp()` returns `[]key.Binding` with non-empty Help strings.
- Esc keypress returns `PopViewMsg`.
- `SetSize` sets width and height.

### Unit Tests: Workspace Model

```bash
go test ./internal/ui/workspace/... -v
```

Expected coverage:
- `Init` returns a non-nil cmd.
- `Update` with a `WorkspaceUpdateMsg` updates `State()`.
- `State()` reflects zero values before first poll.

### E2E Terminal Test

```bash
go test ./internal/e2e/... -v -run TestPlatformViewNavigation -timeout 30s
```

Test scenario:
1. Launch TUI binary with `TERM=xterm-256color`.
2. Wait for chat view to appear.
3. Send `ctrl+p` to open command palette.
4. Type `agents` and press Enter.
5. Assert `SMITHERS › Agents` appears in stdout (ANSI stripped).
6. Send `esc`.
7. Assert the agents header is gone and the chat textarea is visible.
8. Repeat for `approvals` and `tickets`.

### VHS Happy-Path Recording

```bash
vhs tests/vhs/platform-view-navigation.tape
```

The tape navigates: chat → command palette → agents → back → command palette → approvals → back. Verify the generated GIF shows the header changing between `SMITHERS › Agents` and the chat view without artifacts.

### Manual Verification

1. `go run .`
2. Press `/` → type `agents` → Enter → verify agents list renders.
3. Resize the terminal window → verify the agents list reflows without a blank first frame.
4. Press `Esc` → verify return to chat.
5. Press `ctrl+a` → verify approvals view opens.
6. In `approvals` view with a narrow terminal (< 80 cols) → verify compact single-column layout.
7. Verify help bar shows key+description pairs (two distinct columns) for view-specific bindings.
8. With `.smithers-tui.json` configured with `smithers.apiUrl` → verify views attempt to connect (error or data, not empty stub).

---

## Open Questions

1. **`SetSize` vs. forwarding `WindowSizeMsg`**: Adding `SetSize` to the interface is explicit and testable, but requires every future view to implement a two-line boilerplate method. The alternative — guaranteeing `WindowSizeMsg` is always forwarded — is lower boilerplate but relies on forwarding discipline in the root model. Which pattern should be canonical?

2. **`OnFocus`/`OnBlur` optionality**: The `Focusable` interface approach means the router must do a type assertion on every push/pop. An alternative is including `OnFocus`/`OnBlur` in the core `View` interface with no-op default stubs via embedding. Go does not support default interface implementations, so this would require a `BaseView` struct to embed. Is this worth the added complexity at this stage, or should `Focusable` remain optional?

3. **Workspace model polling interval**: 10 seconds is suggested. Should this be configurable per-view (e.g., Approvals view may want 5-second polling when active, 30-second when inactive)? Or should the workspace model be subscription-based (SSE) rather than polling?

4. **`ActionOpenView` migration**: The existing three `ActionOpen*View` types can coexist with `ActionOpenView{Name}` temporarily. Should we migrate them in this ticket or a separate cleanup ticket?

5. **Registry initialization timing**: The registry needs the Smithers client, but the client needs the config, and the config is loaded asynchronously. Should `DefaultRegistry()` accept a client parameter (lazy construction), or should view factories capture the client from a closure at push time?
