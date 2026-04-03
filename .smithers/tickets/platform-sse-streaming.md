# Platform: SSE Event Streaming Consumer

## Metadata
- ID: platform-sse-streaming
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SSE_EVENT_STREAMING
- Dependencies: platform-thin-frontend-layer

## Summary

Implement Server-Sent Events (SSE) consumption in the Smithers client to receive real-time updates for run statuses and chat streaming.

## Acceptance Criteria

- Client exposes a StreamEvents(ctx) returning a channel of Event structs
- Parses SSE format and decodes the inner JSON payloads
- Recovers connection seamlessly on disconnection

## Source Context

- internal/smithers/events.go
- internal/smithers/client.go

## Implementation Notes

- Read from the /events endpoint. Consider using an existing SSE decoder or implement a robust bufio.Scanner loop that handles 'data:' lines.
