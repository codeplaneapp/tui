# Platform: Command Palette Extensions

## Metadata
- ID: platform-palette-extensions
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SMITHERS_COMMAND_PALETTE_EXTENSIONS
- Dependencies: platform-view-router

## Summary

Extend Crush's Command Palette (accessed via / or Ctrl+P) to include navigation commands for all Smithers views, grouped by Workspace and Systems.

## Acceptance Criteria

- Palette includes options for Runs Dashboard, Workflows, Agents, etc.
- Palette includes options for SQL Browser, Triggers, etc.
- Selecting a palette option pushes the corresponding View

## Source Context

- internal/ui/model/ui.go
- internal/ui/model/commands.go

## Implementation Notes

- Crush likely has a list of palette actions. Expand this list with Smithers actions, utilizing Bubble Tea messages to trigger router pushes on selection.
