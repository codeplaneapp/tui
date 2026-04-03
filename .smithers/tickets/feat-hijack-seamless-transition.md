# Seamless Hijack Transition

## Metadata
- ID: feat-hijack-seamless-transition
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: HIJACK_SEAMLESS_TRANSITION
- Dependencies: feat-hijack-run-command

## Summary

Handle the visual transition into and out of the hijacked session smoothly, displaying status banners.

## Acceptance Criteria

- Displays a 'Hijacking run...' message before handoff.
- Displays a summary message when returning from the native TUI.
- Must be verified with a Playwright TUI test capturing the transition text.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Update view state to render a transition banner while `tea.ExecProcess` spins up.
