# Hijack Command

## Metadata
- ID: feat-hijack-run-command
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: HIJACK_RUN_COMMAND
- Dependencies: feat-live-chat-viewer, eng-hijack-handoff-util

## Summary

Implement the command and keybinding ('h') to initiate a run hijack.

## Acceptance Criteria

- Pressing 'h' calls Client.HijackRun().
- Triggers the hijack flow.

## Source Context

- internal/ui/views/livechat.go
- internal/ui/views/runs.go

## Implementation Notes

- Add key handler for 'h' in both LiveChatView and RunsView.
- Return a `hijackSessionMsg` on success.
