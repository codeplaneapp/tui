# Platform: Shell Out Fallback

## Metadata
- ID: platform-shell-out-fallback
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SHELL_OUT_FALLBACK
- Dependencies: platform-thin-frontend-layer

## Summary

Implement direct CLI shell-out methods as a fallback for mutations when the HTTP API is unavailable.

## Acceptance Criteria

- Client can invoke `exec.Command("smithers", ...)` for mutations
- Returns parsed JSON output if the CLI is invoked with --json

## Source Context

- internal/smithers/exec.go
- internal/smithers/client.go

## Implementation Notes

- Wrap os/exec calls, capturing stdout for JSON unmarshaling and mapping stderr to Go errors.
