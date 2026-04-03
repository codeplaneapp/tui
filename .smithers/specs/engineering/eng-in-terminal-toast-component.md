# Research Summary: In-Terminal Toast Notification Component

## Ticket Overview
- **ID**: eng-in-terminal-toast-component
- **Goal**: Create an in-terminal toast overlay component that renders at the bottom-right of the TUI
- **Acceptance Criteria**:
  1. Component supports rendering Title, Body, and action hints
  2. Component respects a TTL for auto-dismissal
  3. Component structure lives in `internal/ui/components/notification.go`

## Key Distinction
This is NOT the existing native OS notification system (`internal/ui/notification/`). This is an **in-terminal overlay** — a bubbletea component rendered inside the TUI viewport itself.

## Existing Codebase Architecture

### Bubbletea Pattern
The project uses the standard Bubble Tea (bubbletea) architecture:
- **Model**: Holds state, implements `Init()`, `Update()`, `View()` interface
- **Messages (Msg)**: Trigger state transitions
- **Commands (Cmd)**: Side effects that return messages
- **Styles**: lipgloss-based styling with the app's theme system

### Existing Components Directory: `internal/ui/components/`
Existing components include:
- `dialog.go` — Modal dialog component (good reference for overlay pattern)
- `markdown.go` — Markdown renderer
- `pill.go` — Pill/badge component
- `spinner.go` — Spinner animation
- `table.go` — Table component
- `text_input.go` — Text input component
- `toast.go` — **Already exists but is a simple string-based toast, not the rich notification we need**

### Existing Toast (`toast.go`) Analysis
The current `toast.go` is minimal:
- Simple text message with auto-dismiss via `time.After`
- No title/body/action-hints structure
- Uses `ToastMsg` and `ClearToastMsg` message types
- Renders as a simple styled box
- Used in `internal/ui/chat/view.go` for brief status messages

### Dialog Component (`dialog.go`) — Reference Pattern
The dialog component is the best reference for the overlay pattern:
- Has title, body content, and action buttons
- Uses lipgloss for styling with borders, padding
- Handles key bindings for actions
- Positioned as an overlay on top of other content
- Uses the theme system for colors

### Theme System
- Defined in `internal/ui/theme/theme.go`
- Provides `ActiveTheme` with color constants
- Components use `theme.ActiveTheme.Foo()` for colors
- Lipgloss styles constructed from theme colors

### Overlay Rendering Pattern
In `internal/ui/chat/view.go`, overlays are rendered by:
1. Rendering the base content first
2. Using `lipgloss.Place()` to position overlay content on top
3. The dialog uses `placeOverlay()` helper for positioning

### Key Bindings Pattern
- Key bindings defined in `internal/ui/keys/keys.go`
- Components declare which bindings they respond to
- Help bar shows contextual key hints

## Implementation Plan

### File: `internal/ui/components/notification.go`

#### Model Structure
```go
type NotificationModel struct {
    title       string
    body        string
    actionHints []ActionHint  // e.g., [{Key: "enter", Label: "approve"}, {Key: "esc", Label: "dismiss"}]
    ttl         time.Duration
    visible     bool
    width       int
    height      int
}

type ActionHint struct {
    Key   string
    Label string
}
```

#### Messages
```go
type ShowNotificationMsg struct {
    Title       string
    Body        string
    ActionHints []ActionHint
    TTL         time.Duration
}

type DismissNotificationMsg struct{}
```

#### Key Methods
- `Init()` — no-op
- `Update(msg tea.Msg)` — handle ShowNotificationMsg (set visible, start TTL timer), DismissNotificationMsg (hide), key messages for action hints
- `View()` — render styled box with title, body, action hints using lipgloss; position bottom-right
- `SetSize(w, h int)` — for responsive positioning

#### Styling
- Use lipgloss border (rounded)
- Theme-aware colors from `theme.ActiveTheme`
- Title in bold, body in regular weight
- Action hints rendered as `[key] label` at bottom
- Max width ~40-50 chars, positioned at bottom-right

#### TTL Auto-Dismissal
- On `ShowNotificationMsg`, return a `tea.Tick` command for the TTL duration
- When tick fires, send `DismissNotificationMsg`
- If a new notification arrives before TTL expires, reset the timer

#### Integration Point
- The chat model (`internal/ui/chat/chat.go`) will hold a `NotificationModel`
- In the chat's `View()`, use `placeOverlay()` to render the notification on top of content at bottom-right position
- The chat's `Update()` will forward relevant messages to the notification model

## Risks & Considerations
1. **Existing toast.go**: Need to ensure the new notification component doesn't conflict with the existing simple toast. The new component is a richer, separate concept.
2. **Overlay z-ordering**: If both a dialog and notification are visible, need to handle z-order (notification should render on top or beside dialog)
3. **Terminal size**: Component needs to handle small terminal sizes gracefully
4. **Timer management**: Need to properly cancel TTL timers when notifications are replaced or dismissed early