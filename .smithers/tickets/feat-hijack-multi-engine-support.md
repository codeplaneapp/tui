# Multi-Engine Support

## Metadata
- ID: feat-hijack-multi-engine-support
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: HIJACK_MULTI_ENGINE_SUPPORT
- Dependencies: feat-hijack-native-cli-resume

## Summary

Support different agent binaries and resume flag combinations for claude-code, codex, amp, etc.

## Acceptance Criteria

- Agent-specific arguments (e.g., --resume <tok>) are applied correctly based on the engine.

## Source Context

- internal/smithers/types.go
- internal/ui/views/livechat.go

## Implementation Notes

- Map engine types to specific argument formatting logic inside `HijackSession.ResumeArgs()`.
