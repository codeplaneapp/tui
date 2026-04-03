# Relative Timestamps

## Metadata
- ID: feat-live-chat-relative-timestamps
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_RELATIVE_TIMESTAMPS
- Dependencies: feat-live-chat-streaming-output

## Summary

Render timestamps on chat messages relative to the start of the run (e.g., [00:02]).

## Acceptance Criteria

- Each message block shows a relative timestamp.
- Calculated accurately from the run's start time.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Calculate time.Since(run.Started) for each block.
- Format as [MM:SS].
