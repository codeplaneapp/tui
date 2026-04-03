# Discover Workflows From Project

## Metadata
- ID: workflows-discovery-from-project
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_DISCOVERY_FROM_PROJECT
- Dependencies: eng-smithers-workflows-client

## Summary

Expose the discovery mechanism to fetch and parse workflows located in the .smithers/workflows/ directory.

## Acceptance Criteria

- The client successfully requests the project's discovered workflows.
- Workflow metadata, including ID, displayName, entryFile, and sourceType, is correctly parsed and passed to the TUI state.

## Source Context

- internal/smithers/client.go
- ../smithers/src/cli/workflows.ts

## Implementation Notes

- Align with how `../smithers/src/cli/workflows.ts` reads `smithers-source:` and `smithers-display-name:` markers.
- Ensure we handle missing or unparseable metadata gracefully without crashing the discovery process.
