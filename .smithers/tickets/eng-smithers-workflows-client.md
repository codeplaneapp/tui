# Build Smithers Workflow API Client Subsystem

## Metadata
- ID: eng-smithers-workflows-client
- Group: Workflows (workflows)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Implement the API client methods in the TUI to interact with the Smithers server's workflow endpoints, supporting workflow listing, metadata retrieval, and execution operations.

## Acceptance Criteria

- Create or update internal/smithers/client.go with ListWorkflows, GetWorkflow, and RunWorkflow methods.
- Ensure the client correctly deserializes workflow schemas and parameters from the Smithers HTTP API.
- Add unit tests for the workflow client methods simulating API responses.

## Source Context

- internal/smithers/client.go
- ../smithers/src/server/index.ts

## Implementation Notes

- Map the HTTP API payloads to Go structs mirroring the DiscoveredWorkflow and Workflow types from Smithers.
- Ensure dual-mode fallback is supported (HTTP preferred, SQLite read-only fallback) for listing.
