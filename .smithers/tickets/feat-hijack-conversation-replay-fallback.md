# Conversation Replay Fallback

## Metadata
- ID: feat-hijack-conversation-replay-fallback
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: HIJACK_CONVERSATION_REPLAY_FALLBACK
- Dependencies: feat-hijack-native-cli-resume

## Summary

Provide a fallback to replaying the chat in-TUI if the target agent has no native TUI resume support.

## Acceptance Criteria

- If agent lacks --resume, fall back to in-TUI conversation loading.
- User can still interact with the session.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Check agent metadata for resume support.
- If missing, route the history into Crush's native chat model instead of executing an external process.
