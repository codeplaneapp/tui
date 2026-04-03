# Stream Real-time Run Updates

## Metadata
- ID: runs-realtime-status-updates
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_REALTIME_STATUS_UPDATES
- Dependencies: runs-dashboard

## Summary

Subscribe to SSE events in the Run Dashboard to update run states dynamically without polling.

## Acceptance Criteria

- Dashboard subscribes to the event stream when active
- Run state changes (e.g., pending -> active -> completed) reflect instantly
- SSE connection is cleanly closed when navigating away

## Source Context

- internal/ui/views/runs.go
- internal/smithers/events.go

## Implementation Notes

- Use a Bubble Tea Cmd to listen for SSE messages and return them as tea.Msg updates.
