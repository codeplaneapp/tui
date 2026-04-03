# Platform: Update Config Namespace

## Metadata
- ID: platform-config-namespace
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SMITHERS_CONFIG_NAMESPACE
- Dependencies: platform-smithers-rebrand

## Summary

Change configuration directories and file names from `.crush/` to `.smithers-tui/` and `.smithers-tui.json` to properly isolate state from Crush.

## Acceptance Criteria

- Configuration is read from .smithers-tui.json instead of crush.json
- Data directory defaults to ~/.config/smithers-tui/ or .smithers-tui/
- Default model and tool settings are tailored for Smithers

## Source Context

- internal/config/config.go
- internal/config/defaults.go
- internal/cmd/root.go

## Implementation Notes

- Modify config dir generation logic in internal/config to resolve `.smithers-tui` and `smithers-tui.json`.
