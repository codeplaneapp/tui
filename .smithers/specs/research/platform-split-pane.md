# Research: platform-split-pane

## Summary

The `SplitPane` component (`internal/ui/components/splitpane.go`) is fully built and tested. The `platform-split-pane` ticket is now purely about **platform-level integration**: wiring the existing component into the views that need it, resolving the root model issues that prevent `Tab`-based focus from working, and establishing the E2E test infrastructure that this and future view tickets will share.

---

## Audit: What `splitpane.go` Provides

**File**: `/internal/ui/components/splitpane.go`

The implementation is complete and matches the engineering spec exactly:

| Feature | Status |
|---------|--------|
| `Pane` interface (`Init`, `Update`, `View`, `SetSize`) | Done |
| `FocusSide` (FocusLeft / FocusRight) | Done |
| `SplitPaneOpts` (LeftWidth, DividerWidth, CompactBreakpoint, FocusedBorderColor, DividerColor) | Done |
| `NewSplitPane` constructor with sensible defaults (30 / 1 / 80) | Done |
| `SetSize` — clamps left to half total, propagates to children | Done |
| `Update` — intercepts `Tab` / `Shift+Tab`, routes all other msgs to focused pane only | Done |
| Compact mode — collapses to single-pane when width < breakpoint; re-propagates size on Tab | Done |
| `View()` — `lipgloss.JoinHorizontal` with thick-border focus indicator on active pane | Done |
| `renderDivider()` — `│` column in dim gray | Done |
| `SetFocus` / `Focus` / `IsCompact` / `Width` / `Height` accessors | Done |
| `ShortHelp() []key.Binding` | Done |

**Notable implementation detail**: The focused pane renders with a left thick-border accent (consuming 1 column) rather than a background highlight. The inner content width is reduced by 1 to accommodate the border. The unfocused pane gets no border and the full width allocation.

**What the component does NOT provide** (intentionally out of scope per the engineering spec):

- `Draw(scr uv.Screen, area uv.Rectangle)` — the ultraviolet screen-buffer render path is **not implemented**. The spec listed it as Slice 5, but the actual file uses only the `View() string` path. This is fine: the root model's `uiSmithersView` draw path calls `current.View()` and wraps it in `uv.NewStyledString`, so the string path is the correct integration point.
- Draggable resize handle — intentionally deferred.
- Vertical (top/bottom) split — not present, not needed for any existing view.
- Three-pane layouts — handled by composing two `SplitPane` instances where needed.

**Test file**: `/internal/ui/components/splitpane_test.go` — 14 test cases covering defaults, normal layout, compact mode, Tab toggling, Shift+Tab, key routing, window resize, left-width clamping, Init, view output structure, compact view correctness, size re-propagation on compact toggle, programmatic `SetFocus`, visual width assertion, narrow-terminal safety, and `ShortHelp`. All tests are already written; they simply need to be confirmed passing.

---

## Views That Need Split-Pane Integration

### 1. `ApprovalsView` (`internal/ui/views/approvals.go`)

**Current state**: Has a manual split-pane layout in `View()`. The implementation:
- Hand-splits at a fixed `listWidth = 30` with `dividerWidth = 3` (renders `" │ "` as a faint string).
- Joins panes line-by-line using `strings.Split` + padding + loop — not using `SplitPane` at all.
- Compact fallback when `v.width < 80 || detailWidth < 20`: switches to `renderListCompact()` which shows inline context below the selected item.
- Focus is **not tracked** — there is no concept of which pane is "focused". Keyboard navigation (`↑↓`) operates only on the list; the detail pane is purely passive.
- No `Tab` key handling; the whole view is treated as a single-focus list.

**Gap vs. `SplitPane`**: The current implementation duplicates the same layout logic that `SplitPane` provides, but without proper focus management, without the visual focus accent, and without robust size propagation. The detail pane cannot be focused, scrolled, or interacted with.

**Required change**: Replace the manual split with a `SplitPane` that wraps two `Pane` implementations:
- `approvalListPane` — the navigable list (up/down/cursor) — gets `FocusLeft`.
- `approvalDetailPane` — the context display — gets `FocusRight`. Initially read-only (no scrolling needed for v1 but the pane structure enables it later).

### 2. `TicketsView` (`internal/ui/views/tickets.go`)

**Current state**: A flat list with no split at all. Renders ticket ID + snippet in a single column. The PRD (§6.9) and design doc (§3.x) both call for a split-pane layout: list on the left, detail/editor on the right.

**Gap**: No split-pane layout exists. The detail pane — showing ticket markdown content and an edit action — is entirely missing.

**Required change**: Introduce a `SplitPane` where:
- `ticketListPane` — scrollable list of tickets, same navigation as now.
- `ticketDetailPane` — renders the selected ticket's markdown content, with a future `Ctrl+O` hook for `$EDITOR` handoff.

### 3. Views that will need split-pane (future tickets, not in scope here)

| View | Left pane | Right pane |
|------|-----------|------------|
| SQL Browser (`sqlbrowser.go`) | Table list sidebar | Query editor + results |
| Node Inspector (`runinspect.go`) | Node list (DAG) | Task tabs (Input/Output/Config/Chat) |
| Prompts (`prompts.go`) | Prompt list | Source editor + live preview (nested split) |

These are separate tickets. They are documented here because the integration pattern established for Approvals and Tickets should be used consistently across all of them.

---

## Root Model Issues That Block Proper Integration

### Issue 1: `uiSmithersView` layout case is missing from `generateLayout()`

**Location**: `internal/ui/model/ui.go`, function `generateLayout` (~line 2628).

`generateLayout` has cases for `uiOnboarding`, `uiInitialize`, `uiLanding`, and `uiChat`, but no `uiSmithersView` case. When a Smithers view is active, `layout.header` and `layout.main` are zero-valued `uv.Rectangle`s. The draw path at line 2185 attempts `main.Draw(scr, layout.main)` on a zero rect, producing no visible output.

**Fix**: Add a `uiSmithersView` case that mirrors the compact-chat layout: 1-row header, then the full remaining `appRect` for `layout.main`.

### Issue 2: Duplicate key dispatch causes `Tab` neutralization

**Location**: `internal/ui/model/ui.go`, lines ~889–929 (default-case forwarding block) and ~1808–1834 (`uiSmithersView` state-switch case).

Messages are dispatched to the active Smithers view twice:
1. In the `default:` case of the big message switch (line ~894): `m.viewRouter.Current().Update(msg)` for all messages when `m.state == uiSmithersView`.
2. In the `uiSmithersView` arm of the state-switch lower in the same `Update` function (line ~1823): `current.Update(msg)` again.

For `Tab` key presses, this means the view's `SplitPane.Update` is called twice with the same Tab message. The first call toggles focus left→right; the second call toggles it back right→left. Net result: Tab appears to do nothing. This is a direct blocker for split-pane focus toggling.

**Fix**: Remove the forwarding in the `default:` case (lines ~917–929) for the `uiSmithersView` state and leave the canonical forwarding in the `uiSmithersView` arm of the state switch only. The `platform-view-model` plan's Step 3b covers this with `router.Update(msg)`.

### Issue 3: `WindowSizeMsg` not forwarded to Smithers views via router

**Location**: `internal/ui/model/ui.go`, `case tea.WindowSizeMsg:` handler (~line 664).

The root model handles `WindowSizeMsg` and calls `m.updateLayoutAndSize()`, but does not call `m.viewRouter.SetSize(m.width, m.height)`. The existing views (`AgentsView`, `ApprovalsView`, `TicketsView`) handle `tea.WindowSizeMsg` in their own `Update` methods as a workaround, but once they delegate to `SplitPane`, the split pane only gets resize events through `Update` forwarding. The root model's forwarding path does call `Update(msg)` for Smithers views, but only for messages dispatched after the layout update — there is a frame window where the layout rect and the split pane's internal size are out of sync.

**Fix**: Add an explicit `m.viewRouter.SetSize(m.width, m.height)` call in the `WindowSizeMsg` handler, after `m.updateLayoutAndSize()`. This is the same fix in `platform-view-model` Step 3a.

---

## Relationship to Dependent Plans

This ticket depends on the `platform-view-model` plan for the three root model fixes above (Steps 3a, 3b, 3c). The split-pane integration work in `approvals.go` and `tickets.go` cannot be tested end-to-end until the layout case and duplicate dispatch are fixed.

**Ordering**: The root model fixes should land in the same commit batch as (or before) the view rewrites. It is safe to implement both in this ticket because the root model fixes are small (~30 lines) and are blockers for the view work.

---

## Upstream Precedent

The upstream Smithers TUI v1 (`src/cli/tui/components/SqliteBrowser.tsx`) used explicit Tab-cycling between left and right regions. The TUI v2 broker (`Broker.ts`) managed focus regions at the application level with a `currentRegion` state variable. The Go `SplitPane` component encapsulates this pattern inside each view, which is simpler and more composable than a global focus region manager.

---

## Files to Read Before Implementation

| File | Why |
|------|-----|
| `/internal/ui/components/splitpane.go` | The component to integrate (already read) |
| `/internal/ui/views/approvals.go` | Manual split to replace (already read) |
| `/internal/ui/views/tickets.go` | Flat list to add split to (already read) |
| `/internal/ui/model/ui.go` lines 664, 889–929, 1808–1834, 2185–2191, 2628–2750 | Root model fix sites |
| `/internal/ui/views/router.go` | Router interface — `View` has `ShortHelp() []string` today; see `platform-view-model` plan for upgrade path |
