# Implement Tickets API Client Methods

## Metadata
- ID: eng-tickets-api-client
- Group: Content And Prompts (content-and-prompts)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Add HTTP or MCP client methods to fetch, create, and update tickets based on the `.smithers/tickets` directory.

## Acceptance Criteria

- Client exposes `ListTickets`, `CreateTicket`, and `UpdateTicket` operations.
- Operations serialize and deserialize payloads correctly according to the backend schema.
- Terminal E2E test verifying API client capabilities for tickets.

## Source Context

- ../smithers/gui/src/api/transport.ts

## Implementation Notes

- Mirror the functionality of `fetchTickets`, `createTicket`, and `updateTicket` found in the GUI transport layer.
- Add these methods to the `internal/app/smithers` client wrapper.
