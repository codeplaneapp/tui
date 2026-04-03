# Time Travel API Client and Model Scaffolding

## Metadata
- ID: eng-time-travel-api-and-model
- Group: Time Travel (time-travel)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Add Smithers client methods for snapshot operations and basic Bubble Tea model scaffolding for the Timeline view.

## Acceptance Criteria

- Client contains ListSnapshots, DiffSnapshots, ForkRun, and ReplayRun methods.
- Timeline struct and essential Bubble Tea Msg types are defined.
- E2E test mock is available for the snapshot APIs.

## Source Context

- docs/smithers-tui/03-ENGINEERING.md
- internal/app/provider.go
- ../smithers/tests/tui.e2e.test.ts

## Implementation Notes

- Model the client APIs after the mock definitions in 03-ENGINEERING.md (ListSnapshots, DiffSnapshots, ForkRun, ReplayRun).
- Create base Bubble Tea structs in internal/ui/model/timeline.go or internal/ui/views/timeline.go.
- Include mock implementations for terminal E2E tests modeled on ../smithers/tests/tui-helpers.ts.
