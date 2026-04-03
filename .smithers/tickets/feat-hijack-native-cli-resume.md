# Native CLI Resume Execution

## Metadata
- ID: feat-hijack-native-cli-resume
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: HIJACK_NATIVE_CLI_RESUME
- Dependencies: feat-hijack-seamless-transition

## Summary

Pass the appropriate resume tokens and spawn the agent CLI directly, suspending the Smithers TUI.

## Acceptance Criteria

- The external CLI starts correctly with the right directory and token.
- Smithers TUI fully relinquishes the TTY.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Extract `AgentBinary`, `ResumeArgs()`, and `CWD` from the `HijackSession` message.
- Execute using `handoffToProgram`.
