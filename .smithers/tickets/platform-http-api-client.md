# Platform: HTTP API Client Operations

## Metadata
- ID: platform-http-api-client
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_HTTP_API_CLIENT
- Dependencies: platform-thin-frontend-layer

## Summary

Implement the HTTP operations on the Smithers client to query and mutate state via the Smithers CLI HTTP server.

## Acceptance Criteria

- ListRuns, GetRun, and InspectRun methods fetch JSON from /ps and /run endpoints
- Approve, Deny, and Cancel methods perform POST requests to mutate run state
- Client appropriately handles HTTP errors and authorization

## Source Context

- internal/smithers/client.go

## Implementation Notes

- Implement basic net/http client requests, returning Go structs.
- Pass apiToken in the Authorization: Bearer header.
