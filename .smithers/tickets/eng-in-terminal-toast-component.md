# Build in-terminal toast notification component

## Metadata
- ID: eng-in-terminal-toast-component
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Create an in-terminal toast overlay component designed to render at the bottom-right of the TUI.

## Acceptance Criteria

- Component supports rendering Title, Body, and action hints.
- Component respects a TTL for auto-dismissal.
- Component structure lives in internal/ui/components/notification.go.

## Source Context

- internal/ui/components/notification.go

## Implementation Notes

- Unlike the existing Crush native notifications (in internal/ui/notification), this needs to be an in-terminal Bubble Tea component drawn via Lip Gloss over the existing views.

---

## Objective

Build a reusable in-terminal toast notification component (`internal/ui/components/notification.go`) that renders a styled, auto-dismissing overlay at the bottom-right of the TUI screen. This component is the foundational building block for the Smithers notification system — it is consumed by `notifications-toast-overlays` (which integrates it into the main UI loop) and then by `notifications-approval-requests`, `notifications-run-failures`, and `notifications-run-completions` (which fire specific toast events).

The toast is distinct from two existing Crush notification mechanisms:
1. **OS-level desktop notifications** (`internal/ui/notification/`) — uses `beeep` to send native OS notifications. The toast is an *in-terminal* complement, not a replacement.
2. **Status bar `InfoMsg`** (`internal/ui/model/status.go`) — renders a single-line, color-coded message over the help bar at the bottom of the screen. The toast is a richer overlay with title, body, action hints, and bottom-right positioning.

## Scope

### In scope
- `Toast` model: Bubble Tea–compatible struct with `Title`, `Body`, `Actions` (hint strings), `Level` (info/success/warning/error), `TTL`, `ID`, and creation timestamp.
- `ToastManager` model: manages a bounded stack of active toasts, handles TTL expiry via `tea.Tick`, supports adding/dismissing/clearing toasts.
- Bottom-right positioning: a `BottomRightRect` helper in `internal/ui/common/common.go` (analogous to the existing `CenterRect` and `BottomLeftRect`).
- Lip Gloss styling: a new `Toast` section in `internal/ui/styles/styles.go` with level-specific border colors derived from the existing semantic palette (`Error`, `Warning`, `Info`, `Green`).
- Draw method: renders onto a `uv.Screen` / `uv.Rectangle` using UltraViolet's positional drawing (consistent with `dialog.Overlay.Draw`).
- Unit tests for the component and manager in `internal/ui/components/notification_test.go`.

### Out of scope
- Integration into the main `UI.View()` / `UI.Draw()` loop (that is ticket `notifications-toast-overlays`).
- Firing toast events from SSE or pubsub (that is `notifications-approval-requests`, `notifications-run-failures`, `notifications-run-completions`).
- Animating fade-in/fade-out (Design doc §8 mentions "fade in from right" — deferred to polish).
- Replacing the existing desktop notification backend or status bar.

## Implementation Plan

### Slice 1 — `BottomRightRect` positioning helper

**File**: `internal/ui/common/common.go`

Add a `BottomRightRect` function alongside the existing `CenterRect` (line 50) and `BottomLeftRect` (line 62):

```go
// BottomRightRect returns a Rectangle positioned at the bottom-right
// within the given area with the specified width and height.
func BottomRightRect(area uv.Rectangle, width, height int) uv.Rectangle {
    maxX := area.Max.X
    minX := maxX - width
    maxY := area.Max.Y
    minY := maxY - height
    return image.Rect(minX, minY, maxX, maxY)
}
```

Unit test: verify coordinates for a known area/width/height triple in `internal/ui/common/common_test.go`.

### Slice 2 — Toast styles

**File**: `internal/ui/styles/styles.go`

Add a `Toast` struct inside the top-level `Styles` struct:

```go
Toast struct {
    Container   lipgloss.Style // Rounded border, BgOverlay background, padding
    Title       lipgloss.Style // Bold, foreground = level color
    Body        lipgloss.Style // Muted text
    ActionHints lipgloss.Style // Dim, small-caps hint text (e.g. "[a] Approve [v] View")
    // Per-level border colors (set at init time)
    InfoBorder    lipgloss.Style
    SuccessBorder lipgloss.Style
    WarningBorder lipgloss.Style
    ErrorBorder   lipgloss.Style
}
```

Initialize in `DefaultStyles()` using the existing color constants:
- Container: `lipgloss.RoundedBorder()`, `Background(BgOverlay)`, `Padding(1, 2)`, max width 40 columns.
- Level borders derive from `Info`, `Green`, `Warning`, `Error` colors.
- Title: `Bold(true)`, foreground set per-level at render time.
- Body: `Foreground(Muted)`.
- ActionHints: `Foreground(HalfMuted)`, `Italic(true)`.

### Slice 3 — `Toast` data model and `ToastLevel` enum

**File**: `internal/ui/components/notification.go`

```go
package components

import (
    "time"
    tea "charm.land/bubbletea/v2"
)

// ToastLevel determines the visual severity of a toast.
type ToastLevel int

const (
    ToastInfo    ToastLevel = iota
    ToastSuccess
    ToastWarning
    ToastError
)

// Toast represents a single in-terminal notification.
type Toast struct {
    ID        string
    Title     string
    Body      string
    Actions   []string   // e.g. ["[a] Approve", "[v] View"]
    Level     ToastLevel
    TTL       time.Duration
    CreatedAt time.Time
}
```

Define Bubble Tea messages:

```go
// ShowToastMsg tells the UI to display a new toast.
type ShowToastMsg struct{ Toast Toast }

// DismissToastMsg tells the UI to dismiss a specific toast by ID.
type DismissToastMsg struct{ ID string }

// toastExpiredMsg is internal — fired when a toast's TTL elapses.
type toastExpiredMsg struct{ ID string }
```

### Slice 4 — `ToastManager` model

**File**: `internal/ui/components/notification.go` (same file)

```go
const (
    MaxVisibleToasts = 3
    DefaultToastTTL  = 10 * time.Second
)

// ToastManager manages a stack of active toasts.
type ToastManager struct {
    com    *common.Common
    toasts []Toast
}

func NewToastManager(com *common.Common) *ToastManager { ... }

// Add queues a toast and returns a tea.Cmd that fires toastExpiredMsg after TTL.
func (tm *ToastManager) Add(t Toast) tea.Cmd { ... }

// Dismiss removes a toast by ID.
func (tm *ToastManager) Dismiss(id string) { ... }

// Update handles toastExpiredMsg to auto-dismiss.
func (tm *ToastManager) Update(msg tea.Msg) tea.Cmd { ... }

// HasToasts returns whether any toasts are active.
func (tm *ToastManager) HasToasts() bool { ... }

// Draw renders all active toasts stacked vertically from the bottom-right.
func (tm *ToastManager) Draw(scr uv.Screen, area uv.Rectangle) { ... }
```

**Key behaviors**:
- `Add()`: prepends to the slice, trims to `MaxVisibleToasts`, returns `tea.Tick(t.TTL, func(time.Time) tea.Msg { return toastExpiredMsg{ID: t.ID} })`.
- `Dismiss()`: removes by ID, no-op if not found.
- `Update()`: on `toastExpiredMsg`, calls `Dismiss(msg.ID)`.
- `Draw()`: iterates toasts bottom-to-top. For each toast, renders a styled Lip Gloss string (title + body + action hints), measures its height, and draws it via `uv.NewStyledString(...).Draw(scr, rect)` where `rect` is computed from `BottomRightRect` with a vertical offset accumulator so toasts stack upward. A 1-row gap separates stacked toasts.

**Rendering detail for a single toast**:

```
┌──────────────────────────────────┐
│ ⚠ Approval needed                │
│ "Deploy to staging" (def456)     │
│                                  │
│ [a] Approve  [d] Deny  [v] View │
└──────────────────────────────────┘
```

- Border color from `ToastLevel` → corresponding style in `Styles.Toast`.
- Title line: level indicator icon (ℹ, ✓, ⚠, ✗) + title text.
- Body: wrapped to container width - padding.
- Actions: joined with `  ` spacing, rendered in `ActionHints` style.
- Max width: 40 columns (Design doc §3.15 shows ~34-col toast).
- Right-margin: 2 columns from terminal edge for visual breathing room.

### Slice 5 — Unit tests

**File**: `internal/ui/components/notification_test.go`

| Test | Asserts |
|------|---------|
| `TestToastManager_Add` | After Add(), `HasToasts()` returns true; toast appears in internal slice |
| `TestToastManager_Add_MaxVisible` | Adding >3 toasts trims oldest |
| `TestToastManager_Dismiss` | Dismiss by ID removes correct toast, leaves others |
| `TestToastManager_Dismiss_NotFound` | Dismiss unknown ID is a no-op (no panic) |
| `TestToastManager_Update_Expiry` | Sending `toastExpiredMsg{ID}` removes the toast |
| `TestToastManager_Add_ReturnsTTLCmd` | `Add()` returns a non-nil `tea.Cmd` |
| `TestToast_Levels` | Each `ToastLevel` maps to a distinct style (border color) |
| `TestBottomRightRect` | Verify correct rectangle for known inputs |
| `TestToastDraw_SingleToast` | Render one toast onto a `uv.ScreenBuffer`; verify it occupies the expected bottom-right rectangle |
| `TestToastDraw_StackedToasts` | Render 3 toasts; verify they stack upward without overlapping |

Use `testify/require` for assertions (consistent with existing tests like `internal/ui/notification/notification_test.go`). Use `uv.NewScreenBuffer(width, height)` for draw tests.

## Validation

### Unit tests

```bash
go test ./internal/ui/components/... -run TestToast -v
go test ./internal/ui/common/... -run TestBottomRightRect -v
```

All tests in `notification_test.go` must pass. Coverage target: >90% of the `ToastManager` methods and `Draw` logic.

### Terminal E2E test (modeled on upstream @microsoft/tui-test harness)

The upstream Smithers E2E pattern (from `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`) uses a `TUITestInstance` that spawns the TUI process, pipes stdin, and matches stdout snapshots. Port this pattern to Go:

**File**: `tests/e2e/toast_notification_test.go`

```go
func TestToastNotification_RendersAndAutoDismisses(t *testing.T) {
    tui := launchTUI("--config", "testdata/smithers-tui.json")
    defer tui.Terminate()

    // Wait for boot
    tui.WaitForText("SMITHERS", 5*time.Second)

    // Trigger a toast via a test-only command or simulated SSE event.
    // Option A: Add a /test-toast debug command that fires a ShowToastMsg.
    tui.SendKeys("/test-toast\n")

    // Verify the toast renders in the terminal output
    tui.WaitForText("Test notification", 3*time.Second)

    // Take a snapshot and assert the toast text is present
    snap := tui.Snapshot()
    require.Contains(t, snap, "Test notification")

    // Wait for TTL expiry (default 10s, but use a short TTL for test)
    time.Sleep(3 * time.Second)

    // Verify the toast is gone
    snap2 := tui.Snapshot()
    require.NotContains(t, snap2, "Test notification")
}
```

This test depends on the `launchTUI` / `WaitForText` / `Snapshot` / `SendKeys` / `Terminate` helpers defined in `tests/e2e/tui_helpers_test.go` (see ticket `eng-live-chat-e2e-testing` for the canonical helper definition). If those helpers are not yet implemented, this test should be stubbed with `t.Skip("requires TUI E2E helpers")`.

### VHS happy-path recording test

**File**: `tests/vhs/toast_notification.tape`

```tape
Output tests/vhs/toast_notification.gif
Set FontSize 14
Set Width 1200
Set Height 800
Set Theme "Smithers"

Type "smithers-tui --config testdata/smithers-tui.json"
Enter
Sleep 3s

# Trigger a toast notification
Type "/test-toast"
Enter
Sleep 1s

# Capture the toast visible on screen
Screenshot tests/vhs/toast_visible.png

# Wait for auto-dismissal
Sleep 12s

# Capture the screen after toast dismissed
Screenshot tests/vhs/toast_dismissed.png
```

Run with: `vhs tests/vhs/toast_notification.tape`

This produces a GIF showing the toast appear and auto-dismiss, plus two PNG screenshots for visual regression comparison.

### Manual verification

1. Build and run: `go build -o smithers-tui . && ./smithers-tui`
2. Trigger a toast (via `/test-toast` debug command or by wiring a temporary `ShowToastMsg` in `ui.go`).
3. Verify:
   - Toast appears at bottom-right with rounded border.
   - Title, body, and action hints are visible.
   - Different levels (info/success/warning/error) show different border colors and indicator icons.
   - Toast auto-dismisses after TTL.
   - Multiple toasts stack upward without overlapping.
   - Toast does not block keyboard input to the underlying view.
   - Resizing the terminal repositions the toast correctly.

### Lint and vet

```bash
go vet ./internal/ui/components/...
golangci-lint run ./internal/ui/components/...
```

## Risks

### 1. UltraViolet Screen rendering is non-trivial
The existing dialog overlay system (`internal/ui/dialog/dialog.go`) uses `uv.NewStyledString(...).Draw(scr, rect)` for positional rendering. The toast must follow the same pattern. **Risk**: If the toast's Lip Gloss–rendered string contains ANSI escape codes wider than expected, the UltraViolet rect clipping could produce garbled output. **Mitigation**: Use `lipgloss.Width()` / `lipgloss.Size()` consistently (as `dialog.go:167` does) and test with multi-byte/emoji content.

### 2. `internal/ui/components/` directory does not yet exist
The engineering doc (`03-ENGINEERING.md` §2.3) plans for `internal/ui/components/` but it is not yet created in the Crush codebase. This ticket creates the first file there. **Mitigation**: Create the directory and `package components` declaration as part of this ticket. Keep the package self-contained — depend only on `internal/ui/common`, `internal/ui/styles`, and standard Bubble Tea / Lip Gloss / UltraViolet imports.

### 3. No view router yet
The toast manager's `Draw()` method needs to be called with the screen area after the active view is rendered (per `notifications-toast-overlays` ticket). The view router (`internal/ui/views/router.go`) is planned but not implemented. **Mitigation**: Design the `ToastManager.Draw(scr, area)` signature to be agnostic of the router — it takes whatever `uv.Rectangle` the caller provides. The downstream integration ticket (`notifications-toast-overlays`) handles wiring it into `UI.Draw()` after the dialog overlay.

### 4. Crush's rendering uses `Draw(scr, area)` not `View() string`
Crush moved from Lip Gloss string concatenation to UltraViolet `Screen`-based drawing in Bubble Tea v2. The toast component must use `Draw(scr uv.Screen, area uv.Rectangle)`, not `View() string`. This is consistent with the dialog overlay and status bar, but differs from many Bubble Tea v1 examples online. **Mitigation**: Follow the exact pattern in `dialog.Overlay.Draw()` (`dialog.go:199-205`) and `Status.Draw()` (`status.go:71-113`).

### 5. E2E test infrastructure may not be available yet
The terminal E2E helpers (`launchTUI`, `WaitForText`, `Snapshot`, etc.) are specified in other tickets (`eng-live-chat-e2e-testing`) and may not be implemented when this ticket is worked. **Mitigation**: Write the E2E test with a `t.Skip` guard. The unit tests in `notification_test.go` are the primary validation gate for this ticket; E2E and VHS tests are secondary and can be unblocked as infrastructure lands.

### 6. Crush vs Smithers module path
All imports currently use `github.com/charmbracelet/crush/...`. The engineering doc plans a module rename to `github.com/anthropic/smithers-tui`. The component should use the current import paths (Crush) since the rename hasn't happened yet. **Mitigation**: Use current paths; the global rename will update them in bulk.
