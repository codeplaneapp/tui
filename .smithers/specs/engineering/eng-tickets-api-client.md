# Research Summary: eng-tickets-api-client

## Ticket Overview
The `eng-tickets-api-client` ticket requires implementing Go client methods (`ListTickets`, `CreateTicket`, `UpdateTicket`) that mirror the TypeScript transport layer's ticket operations. These methods should serialize/deserialize payloads correctly and include terminal E2E test coverage.

## Key Findings

### Existing Smithers Client (`internal/smithers/client.go`)
A Go HTTP client already exists with base infrastructure for making API calls to the Smithers backend. It includes methods for other resources and provides the foundation (base URL, HTTP client, auth) to add ticket operations.

### Existing Types (`internal/smithers/types.go`)
Type definitions exist for various Smithers domain objects. Ticket-specific types will need to be added here to match the backend schema.

### TypeScript Reference (`../smithers/gui/src/api/transport.ts`)
The frontend transport layer already implements `fetchTickets`, `createTicket`, and `updateTicket` functions. The Go implementation should mirror these endpoints, request/response shapes, and error handling patterns.

### App Architecture (`internal/app/app.go`)
The main app wires up dependencies and lifecycle. The Smithers client is already integrated into the app's dependency graph, so new ticket methods will be automatically available to UI views and commands.

## Implementation Plan
1. Add `Ticket` struct and related types to `internal/smithers/types.go`
2. Add `ListTickets()`, `CreateTicket()`, and `UpdateTicket()` methods to the client in `internal/smithers/client.go`
3. Wire ticket operations into the UI layer (views/commands) as needed
4. Add E2E tests verifying the API client capabilities

## Dependencies
None — the client infrastructure and app wiring are already in place.