# Platform: Scaffold Thin Frontend Client

## Metadata
- ID: platform-thin-frontend-layer
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_THIN_FRONTEND_TRANSPORT_LAYER
- Dependencies: none

## Summary

Create the foundational `internal/smithers/` client package that handles communication with the Smithers CLI server via HTTP, SSE, and SQLite fallbacks.

## Acceptance Criteria

- internal/smithers package exists
- Client struct supports configuring an API URL, Token, and local SQLite DB path
- Core data types (Run, Node, Attempt, Event) are defined matching the Smithers server schemas

## Source Context

- internal/smithers/client.go
- internal/smithers/types.go

## Implementation Notes

- This is just the struct and types scaffolding; actual methods will be added in subsequent tickets.
- Use standard Go context.Context for all future client operations.
