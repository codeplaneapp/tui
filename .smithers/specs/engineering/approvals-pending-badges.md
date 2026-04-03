# Research: approvals-pending-badges

## Ticket Summary

The `approvals-pending-badges` ticket requires displaying a visual indicator (badge) in the main UI header/status bar when approvals are pending. The badge must update dynamically via SSE events.

## Acceptance Criteria

1. If `pending_count > 0`, a badge is visible on the main screen.
2. Badge updates dynamically via SSE events.

## Key Files Analyzed

### internal/ui/model/header.go
This file contains the `RenderHeader` function which builds the status bar content. It currently renders:
- Model name
- Token counts and context window percentage
- Keyboard shortcut hints (ctrl+d for open/close)
- Working directory path

All parts are joined with a dot separator (`" • "`) and truncated to the available width. This is the primary integration point for the pending approvals badge.

### internal/ui/model/status.go
Contains the status bar rendering logic and styles. Works alongside header.go to form the top-level UI chrome.

### internal/ui/views/approvals.go
New file (staged) that provides the approvals view scaffolding, which the badge feature will need to reference for navigation/linking.

### internal/smithers/client.go
New Smithers API client (staged) that will be used to fetch pending approval counts from the backend.

## Dependencies

- `approvals-queue` ticket must be completed first (provides the underlying approvals data model and queue view).
- The Smithers client (`internal/smithers/client.go`) needs endpoints for fetching pending approval counts.
- SSE streaming infrastructure (`platform-sse-streaming`) is needed for dynamic badge updates.

## Implementation Approach

1. **Add pending count to the header model**: Extend the header rendering in `RenderHeader` (internal/ui/model/header.go) to include a badge showing the pending approval count. Insert it as a new part in the `parts` slice, styled distinctly (e.g., warning/accent color) when count > 0.

2. **Fetch count from Smithers client**: Use the Smithers HTTP client to poll or subscribe (via SSE) for the current pending approval count. Store this in the UI model state.

3. **SSE integration**: Subscribe to approval-related SSE events so the badge updates in real-time without requiring manual refresh.

4. **Conditional rendering**: Only show the badge when `pending_count > 0` to avoid visual clutter when there are no pending approvals.

5. **Style the badge**: Use lipgloss styling consistent with the existing header styles (defined in the theme's `Header` struct) — likely a contrasting or warning color to draw attention.

## Risks & Considerations

- The header line is already width-constrained (`ansi.Truncate`). Adding a badge may cause truncation of other elements on narrow terminals. Need to consider priority/ordering of elements.
- SSE connection reliability — need graceful fallback if the SSE stream disconnects.
- The Smithers client and SSE infrastructure are both new/staged code that may not be fully stable yet.