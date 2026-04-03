# Platform: Split Pane Layouts

## Metadata
- ID: platform-split-pane
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SPLIT_PANE_LAYOUTS
- Dependencies: none

## Summary

Create a reusable Split Pane Bubble Tea component that renders two sub-views side-by-side with proportional widths.

## Acceptance Criteria

- SplitPane component takes a left and right tea.Model
- Handles WindowSizeMsg by dividing horizontal space according to a configurable ratio (e.g. 30/70)
- Passes relevant updates to both children, or focuses one

## Source Context

- internal/ui/components/splitpane.go

## Implementation Notes

- Use Lip Gloss's `JoinHorizontal` to render the side-by-side panes. Pass `WindowSizeMsg` down to children after mutating the `Width` to fit their pane bounds.
