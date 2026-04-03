# Active Run Summary

## Metadata
- ID: chat-active-run-summary
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_ACTIVE_RUN_SUMMARY
- Dependencies: chat-ui-branding-status

## Summary

Display the aggregate number of active Smithers runs in the UI header/status bar.

## Acceptance Criteria

- The header displays 'X active' when there are running workflows.
- The run count updates dynamically based on the Smithers client state.

## Source Context

- internal/ui/model/header.go
- internal/smithers/client.go

## Implementation Notes

- Implement a `renderSmithersStatus()` function in `internal/ui/model/header.go`.
- Fetch the active run count from the cached `smithersClient` state to populate the string.
