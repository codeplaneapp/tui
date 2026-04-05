# Research: platform-view-model

## Ticket Summary

Introduce the `View` interface representing distinct TUI screens adhering to the Workspace/Systems separation (Runs, Workflows, Agents, Prompts, Tickets, SQL, Triggers, Approvals, etc.), wire the `Router` into the root model, and establish lifecycle hooks (focus, blur, resize) that make the view layer production-grade.

---

## Existing Crush Surface

### The View Interface (`internal/ui/views/router.go`)

The current `View` interface is:

```go
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Name() string
    ShortHelp() []string
}
```

It deliberately diverges from `tea.Model` in two ways: `Update` returns `(View, tea.Cmd)` rather than `(tea.Model, tea.Cmd)`, which preserves the concrete type through the router without requiring type assertions; and `ShortHelp` returns `[]string` rather than `[]key.Binding`, which is a simpler but weaker contract (no keybinding metadata for the `help` component).

### The Router (`internal/ui/views/router.go`)

The `Router` is a pure push/pop stack:

```go
type Router struct {
    stack []View
}
func (r *Router) Push(v View) tea.Cmd  // appends + calls v.Init()
func (r *Router) Pop() bool            // shrinks slice
func (r *Router) Current() View        // stack[len-1] or nil
func (r *Router) HasViews() bool
```

There is no `Register` map, no named navigation by route string, and no `Switch` method — the earlier engineering spec's description was aspirational. The actual implementation uses imperative push/pop. Named routing (e.g., `/runs`, `/agents`) must be done by the caller constructing the view before pushing.

A `PopViewMsg` struct is defined so views can signal their own dismissal via a returned `tea.Cmd`.

### Existing Views

Three views implement the interface today:

| View | File | State | ShortHelp |
|------|------|-------|-----------|
| `AgentsView` | `agents.go` | `loading`, `agents []Agent`, `cursor`, `width`, `height`, `err` | `[Enter] Launch`, `[r] Refresh`, `[Esc] Back` |
| `ApprovalsView` | `approvals.go` | `loading`, `approvals []Approval`, `cursor`, `width`, `height`, `err` | `[↑↓] Navigate`, `[r] Refresh`, `[Esc] Back` |
| `TicketsView` | `tickets.go` | `loading`, `tickets []Ticket`, `cursor`, `width`, `height`, `err` | `[Enter] View`, `[r] Refresh`, `[Esc] Back` |

Each view receives `tea.WindowSizeMsg` and stores `width`/`height` on its own struct. All three follow the same pattern: issue a `tea.Cmd` from `Init()` that calls the Smithers client, receive a typed `loadedMsg` or `errorMsg`, and render accordingly.

`ApprovalsView` is the most sophisticated: it implements a split-pane layout (list left, detail right) that degrades to single-column when `width < 80`. It has helper functions (`padRight`, `truncate`, `formatStatus`, `formatPayload`, `wrapText`) that should be extracted to `internal/ui/components/` as the view library grows.

### Integration with the Root Model (`internal/ui/model/ui.go`)

The root `UI` struct holds:

```go
viewRouter     *views.Router      // initialized as views.NewRouter() in New()
smithersClient *smithers.Client   // initialized as smithers.NewClient() in New()
```

A dedicated UI state value `uiSmithersView` signals that a view is active. The root model has three places that key on this state:

1. **Generic message forwarding** (line ~904): All messages are forwarded to the current view when `m.state == uiSmithersView`. This includes `tea.WindowSizeMsg`, typed async messages, and key events from the section below.

2. **Key event routing for Smithers views** (line ~1787): The `uiSmithersView` arm in the key-handling switch delegates `KeyPressMsg` to `current.Update(msg)` — this is separate from the general forwarding to give the key branch precedence for specific intercepts (e.g., Esc to pop).

3. **Drawing** (line ~2150): `drawHeader` is called, then `current.View()` is rendered into the main area via `uv.NewStyledString`.

4. **ShortHelp** (line ~2318): The help bar iterates `current.ShortHelp()` and wraps each string in an empty `key.NewBinding` with only `WithHelp`. This works but discards the actual key binding information, so the help component cannot render the key and description as separate styled columns.

**Critical gap**: `WindowSizeMsg` is handled at the root level (`m.width, m.height = msg.Width, msg.Height`) but is **not explicitly forwarded** to the current view. Views only receive `WindowSizeMsg` because it falls through to the generic `if m.state == uiSmithersView` forwarding block at line 904, which runs after the main switch. This is fragile: if the root model adds an early `return` or a different code path for `WindowSizeMsg`, views will stop receiving resize events.

**Pop handling** (line ~1474): `PopViewMsg` is handled in the action/dialog switch, which calls `m.viewRouter.Pop()`. If the stack empties, the model transitions back to `uiChat` (if a session exists) or `uiLanding`. This is correct but the pop is triggered by a `tea.Cmd` that emits `PopViewMsg`, creating an indirect return path.

### Command Palette (`internal/ui/dialog/commands.go`)

Currently wired commands that open views:

```go
NewCommandItem(..., "agents",    "Agents",    "", ActionOpenAgentsView{})
NewCommandItem(..., "approvals", "Approvals", "", ActionOpenApprovalsView{})
NewCommandItem(..., "tickets",   "Tickets",   "", ActionOpenTicketsView{})
```

Missing from the palette: Runs, Workflows, Prompts, SQL Browser, Triggers, Timeline, Scores, Memory.

### Keybindings (`internal/ui/model/keys.go`)

Two Smithers-specific global bindings exist:

```go
RunDashboard: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "runs"))
Approvals:    key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "approvals"))
```

`ctrl+r` was previously used for attachment delete mode but was moved. `ctrl+a` is new. These bindings appear in the help bar during `uiChat` state (line ~2293), but the `Update` loop does not yet route `ctrl+r` or `ctrl+a` keypresses to open the respective views — that wiring is missing in the key handler.

### Smithers Client Construction (`internal/ui/model/ui.go` line ~342)

The client is created as `smithers.NewClient()` with no options. The config struct has a full `Smithers` section (`APIUrl`, `APIToken`, `DBPath`, `WorkflowDir`) defined in `internal/config/config.go`, but these values are not passed to the client at construction time. All three views therefore operate against an unconfigured client (no API URL, no auth token, no DB path).

### ShortHelp Type Mismatch

`View.ShortHelp()` returns `[]string`. The root model wraps each string as:

```go
key.NewBinding(key.WithHelp("", hint))
```

This passes the entire hint string as the "description" and leaves the key column empty. The built-in `help.Model` renders two columns: key and description. With an empty key column the rendering is functional but wastes space and does not integrate cleanly with Bubble Tea's help component styling.

The `help.KeyMap` interface expects `ShortHelp() []key.Binding`. Matching that signature would allow the view's help to be merged with the root model's help bindings and rendered consistently.

---

## Architecture Analysis: Elm Architecture Alignment

Bubble Tea implements the Elm Architecture (Model-Update-View). The current view system is mostly aligned but has gaps:

**What is well-aligned**:
- `Init() tea.Cmd` maps to Elm's `init` — fires once on view entry, returns commands for side effects.
- `Update(tea.Msg) (View, tea.Cmd)` maps to Elm's `update` — pure transformation producing a new model.
- `View() string` maps to Elm's `view` — pure render from state.
- `PopViewMsg` as a message type follows the Elm pattern of using messages to signal navigation.

**What is missing vs. standard Elm patterns**:

1. **Focus/blur lifecycle**: In Elm applications with nested components, the parent signals focus changes to children via dedicated messages. Today, a view has no way to know when it becomes active vs. when it is obscured by an overlaid dialog. Views that need to start or stop polling (e.g., a Runs view with SSE subscription) need a `focused` signal to begin polling and a `blurred` signal to pause.

2. **OnResize callback**: `tea.WindowSizeMsg` is propagated through the generic forwarding path, not guaranteed. A dedicated `SetSize(width, height int)` method (matching the `Pane` interface proposed in `eng-split-pane-component`) would make resize handling explicit and testable, and would allow the router to proactively resize a view when it is pushed (since the current window dimensions are known at push time).

3. **No registration/route map**: The router has no named route registry. Opening a view requires calling `views.NewXxxView(client)` directly from the root model. This creates tight coupling: adding a new view requires editing `internal/ui/model/ui.go`. A factory or registry pattern would decouple view construction from the root model.

4. **No workspace model**: The views operate on raw data fetched on-demand from the client. There is no shared, subscription-driven workspace model that maintains live state (active runs, pending approval count, connection status). The header and status bar currently show static or stale data. A workspace model — a background goroutine that polls or subscribes to SSE events and emits `tea.Msg` updates — would unify data freshness across all views.

---

## Gaps

### Gap 1: Window Size Not Explicitly Propagated

`tea.WindowSizeMsg` is dispatched by Bubble Tea once at startup and on every SIGWINCH. In the current code, it enters the root `Update` handler which records `m.width, m.height` and calls `m.updateLayoutAndSize()`. It does **not** explicitly forward the message to the current view. Views only receive it because the generic forwarding block (`if m.state == uiSmithersView`) runs after the `switch` and passes all messages including `WindowSizeMsg`. If the switch is ever refactored to return early on `WindowSizeMsg` (a natural refactor), views will break silently.

**Fix**: The `WindowSizeMsg` arm should explicitly call the router or pass a size to the current view. This can be done by adding an explicit forward or by adding `SetSize(w, h int)` to the `View` interface.

### Gap 2: Focus Management

When a dialog opens over a Smithers view (e.g., command palette), the view continues receiving messages — it is not "blurred." If a view has started a polling ticker via `Init()`, it continues ticking and updating the model even when the user is interacting with the dialog. Adding `OnFocus()` and `OnBlur()` lifecycle hooks would allow views to suspend background work when not focused.

Additionally, when `Push` is called (view becomes active), the view does not know its current window dimensions — it must wait for a subsequent `WindowSizeMsg` before it can render correctly. This causes a one-frame flash of wrong dimensions on the first render.

**Fix**: `Router.Push` should accept `(w, h int)` alongside the view and call `SetSize` immediately, or the push should emit a synthetic `WindowSizeMsg` to the view.

### Gap 3: ShortHelp Type Contract

`ShortHelp() []string` is not compatible with `help.KeyMap`. The help bar wrapper in the root model (`key.NewBinding(key.WithHelp("", hint))`) means key bindings from views have no key column. When a view says `[Enter] Launch`, the rendered output has an empty key field and the hint in the description field, producing misaligned rendering.

**Fix**: Change `ShortHelp() []string` to `ShortHelp() []key.Binding`. This is a small breaking change but all three existing views already have the information needed to return proper `key.Binding` values.

### Gap 4: No Route Registry / Named Navigation

Adding a new view requires:
1. Defining the view struct and type.
2. Adding a `dialog.ActionOpenXxxView{}` action type.
3. Adding a command palette entry in `commands.go`.
4. Adding a case to the action handler in `ui.go`.
5. Optionally adding a global keybinding in `keys.go`.

This is five touch points per view. With 12+ planned views, this becomes 60+ edits to core files. A `ViewFactory` map (`map[string]func(*smithers.Client) View`) registered at startup would reduce new views to one registration call.

### Gap 5: No Workspace Model

The current views fetch data lazily (on `Init`) and hold it locally in their own structs. There is no shared live model of:

- Active run count + pending approval count (for the header badge)
- Connection state to the Smithers HTTP API or SSE stream
- Background polling or SSE subscription that delivers `tea.Msg` events

The header already has a `smithersStatus *SmithersStatus` field (defined at `ui.go:236`) but it is unclear if it is populated. Without a workspace model, the chat header shows stale or empty Smithers state.

### Gap 6: Config Not Wired to Client

`smithers.NewClient()` is called with no options at `ui.go:342`. The Smithers config fields (`APIUrl`, `APIToken`, `DBPath`) exist in `internal/config/config.go` but are not applied. This means all three production views operate against an unconfigured client that cannot reach the API and cannot open the SQLite database.

### Gap 7: Key Actions Not Dispatched

`k.RunDashboard` (`ctrl+r`) and `k.Approvals` (`ctrl+a`) appear in the help bar during chat but the key handler in `Update` does not have cases for them — pressing `ctrl+r` in chat does nothing. The intended behavior (open runs or approvals view) is not wired.

---

## Recommended Direction

### 1. Strengthen the View Interface

Change `ShortHelp() []string` to `ShortHelp() []key.Binding`. Add two optional lifecycle methods as a separate interface (to avoid a breaking change to existing views):

```go
// Resizable is implemented by views that need explicit size signals.
type Resizable interface {
    SetSize(width, height int)
}

// Focusable is implemented by views that have focus/blur lifecycle behavior.
type Focusable interface {
    OnFocus() tea.Cmd
    OnBlur() tea.Cmd
}
```

`Router.Push` should check if the pushed view implements `Resizable` and call `SetSize` with the current terminal dimensions passed in. `Router` should track `width` and `height` and call `SetSize` on the current view when a `WindowSizeMsg` arrives (which it can do if `Update` is called with the message first, then the router forwards it).

Alternatively, add `SetSize(w, h int)` directly to `View` and update the three existing views — the change is mechanical and low-risk.

### 2. Add a View Registry

Define a `ViewFactory` type and a `Registry` struct in `internal/ui/views/`:

```go
type ViewFactory func(client *smithers.Client) View

type Registry struct {
    factories map[string]ViewFactory
}

func (r *Registry) Register(name string, f ViewFactory)
func (r *Registry) Open(name string, client *smithers.Client) (View, bool)
```

Wire all Smithers views through this at startup. The command palette and key handlers call `registry.Open("runs", client)` instead of reaching into view constructors directly.

### 3. Wire the Config to the Client

At `UI.New()` (or in the `Init()` command), retrieve the Smithers config section and construct the client with `WithAPIURL`, `WithAPIToken`, `WithDBPath`. This is a one-time change that makes all views operational.

### 4. Wire Global Keybindings

Add cases to the key handler in `Update` for `k.RunDashboard` and `k.Approvals` that push the respective views onto the router stack.

### 5. Add a Workspace Model

Introduce `internal/ui/workspace/model.go` with a `WorkspaceModel` struct that:
- Holds `ActiveRunCount`, `PendingApprovalCount`, `ConnectionState`.
- Starts a polling goroutine (or SSE subscriber) via its `Init()` command.
- Emits `WorkspaceUpdateMsg` on each poll cycle.
- Is owned by the root `UI` struct and updated in the generic message handler.

The header and status bar read from this model rather than from individual view state.

### 6. Expand the Command Palette

Register all Workspace and Systems view routes in `commands.go`. This is a data change, not an architectural one, but it must be done in concert with the registry approach to avoid N duplicate action types.

### 7. Testing

Each view should have a table-driven unit test that exercises `Init`, `Update` (with `agentsLoadedMsg`, `tea.WindowSizeMsg`, key events), and `View` output. The pattern from `internal/e2e/tui_helpers_test.go` (terminal harness) should be extended to exercise push/pop navigation flows: open command palette, navigate to view, verify rendering, press Esc, verify return to chat.

---

## Files To Touch

**View interface and router**:
- `internal/ui/views/router.go` — Add `SetSize` to `View` interface (or add `Resizable` interface); update `Push` to accept dimensions; add router-level `Update` method that forwards to current view.

**Existing views**:
- `internal/ui/views/agents.go` — Change `ShortHelp` return type to `[]key.Binding`.
- `internal/ui/views/approvals.go` — Same; extract `padRight`/`truncate`/`formatPayload`/`wrapText` helpers to a shared file.
- `internal/ui/views/tickets.go` — Same.

**Root model integration**:
- `internal/ui/model/ui.go` — Wire Smithers config to client; add cases for `k.RunDashboard` and `k.Approvals` in key handler; ensure `WindowSizeMsg` is explicitly forwarded to router.
- `internal/ui/model/keys.go` — No changes needed; bindings exist.

**Command palette**:
- `internal/ui/dialog/commands.go` — Add registry-driven view commands for all planned views.
- `internal/ui/dialog/actions.go` — Add corresponding action types (or move to a generic `OpenViewAction{Name string}` approach).

**Workspace model** (new):
- `internal/ui/workspace/model.go` — `WorkspaceModel`, `WorkspaceUpdateMsg`.

**View registry** (new):
- `internal/ui/views/registry.go` — `ViewFactory`, `Registry`.

**Tests**:
- `internal/ui/views/router_test.go` (new) — Unit tests for push/pop/resize/focus lifecycle.
- `internal/ui/views/agents_test.go` (new) — Unit tests for `AgentsView` message handling.
- `internal/e2e/platform_view_model_test.go` (new) — E2E: open view via command palette, navigate, Esc back.
- `tests/vhs/platform-view-navigation.tape` (new) — VHS happy-path recording.
