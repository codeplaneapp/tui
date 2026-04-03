# Resume to Automation on Exit

## Metadata
- ID: feat-hijack-resume-to-automation
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: HIJACK_RESUME_TO_AUTOMATION
- Dependencies: feat-hijack-native-cli-resume

## Summary

Automatically refresh the Smithers run state and chat history when the user exits the native agent TUI.

## Acceptance Criteria

- Upon agent TUI exit, Live Chat Viewer immediately reflects new state.
- User is not left with stale pre-hijack state.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- On `hijackReturnMsg`, call `v.refreshRunState()` to pull the latest events from the Smithers API.
