# Platform: Rebrand TUI to Smithers

## Metadata
- ID: platform-smithers-rebrand
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SMITHERS_REBRAND
- Dependencies: none

## Summary

Fork Crush and rebrand it to Smithers TUI. This requires renaming the Go module, binary names, textual headers, and replacing Crush's ASCII art with Smithers branding.

## Acceptance Criteria

- Go module is renamed to github.com/anthropic/smithers-tui
- Binary is named smithers-tui
- Header displays SMITHERS instead of Charm CRUSH
- ASCII art logo is updated
- Terminal color scheme matches Smithers brand (cyan/green/magenta palette)

## Source Context

- go.mod
- main.go
- internal/cmd/root.go
- internal/ui/logo/logo.go
- internal/ui/styles/styles.go

## Implementation Notes

- Use go mod edit to change the module path.
- Update the lipgloss definitions in internal/ui/styles/styles.go.
- Replace the ASCII string in internal/ui/logo/logo.go.
