# Create Shared Split Pane Layout Component

## Metadata
- ID: eng-split-pane-component
- Group: Content And Prompts (content-and-prompts)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Implement a reusable Bubble Tea component for side-by-side layouts, supporting a fixed-width left pane and a responsive right pane.

## Acceptance Criteria

- Component can render arbitrary left and right Bubble Tea views.
- Handles viewport resizing correctly.

## Source Context

- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Create `internal/ui/components/splitpane.go`.
- Use `lipgloss.JoinHorizontal` to stitch views. The left view should typically have a hardcoded max width.
