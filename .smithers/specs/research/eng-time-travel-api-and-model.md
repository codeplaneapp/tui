# Research: eng-time-travel-api-and-model

## Existing Crush Surface

- **Client & Types**: Inspected `internal/smithers/client.go` and `internal/smithers/types.go`. There are currently no implementations for snapshot-related APIs, nor any type definitions for `Snapshot`, `Timeline`, or `Diff`.
- **UI Scaffolding**: Searched `internal/ui/model/` and `internal/ui/views/`. There is no existing scaffolding for the Timeline view (e.g., no `timeline.go` or associated Bubble Tea `Msg` types).
- **Tests**: Checked `internal/smithers/client_test.go`; no mock structures exist for timeline or time-travel behaviors.

## Upstream Smithers Reference

- **Engineering Spec**: `docs/smithers-tui/03-ENGINEERING.md` defines the exact signature mocks required for the client API, including: `ListSnapshots(ctx, runID)`, `DiffSnapshots(ctx, runID, from, to)`, `ForkRun(ctx, runID, snapshotNo)`, and `ReplayRun(ctx, runID, snapshotNo)`.
- **Smithers Source**: Inspected `../smithers/src` (specifically `src/cli/index.ts` and `src/cli/tui-v2/broker/Broker.ts`), which handles the backend time-travel implementations like `diffRawSnapshots`, `loadSnapshot`, and `buildTimeline`.
- **E2E Testing**: Examined `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`. The upstream E2E strategy involves spawning background workflows, launching the TUI process, and programmatically sending keystrokes while polling the terminal buffer for expected text updates.

## Gaps

- **Data Model**: Crush is missing `Snapshot`, `Diff`, and `Timeline` struct definitions in `internal/smithers/types.go` to represent the time-travel data.
- **Transport**: Crush's Smithers client (`internal/smithers/client.go`) lacks the required `ListSnapshots`, `DiffSnapshots`, `ForkRun`, and `ReplayRun` methods outlined in the engineering spec.
- **Rendering/UX**: There is no Bubble Tea scaffolding for the timeline view, meaning there are no `Model` structs, `Update`/`View` methods, or standard message types (e.g., `SnapshotSelectedMsg`) to handle time-travel interactions in the TUI.
- **Testing**: Crush lacks E2E mock setups for the snapshot APIs in `client_test.go`. Furthermore, it lacks a terminal E2E path modeled on the upstream Microsoft TUI test harness, and there is no VHS-style recording test for the time-travel feature.

## Recommended Direction

1. **Define Data Models**: Add the necessary `Snapshot`, `Diff`, and `Timeline` structs to `internal/smithers/types.go`.
2. **Implement Client Methods**: Add the specified methods (`ListSnapshots`, `DiffSnapshots`, `ForkRun`, `ReplayRun`) to `internal/smithers/client.go`, ensuring they map correctly to the backend JSON API or local SQLite storage.
3. **Scaffold the View**: Create `internal/ui/model/timeline.go` (or `internal/ui/views/timeline.go`). Define a base Bubble Tea `Timeline` model and essential message types (like `TimelineLoadedMsg` and `ReplayRequestedMsg`).
4. **Build E2E Mocks**: Add mock implementations for these APIs in `internal/smithers/client_test.go` to allow for offline testing.
5. **Implement Test Harnesses**: Create an E2E terminal testing path in Crush modeled after `../smithers/tests/tui-helpers.ts` (using Go's `os/exec` and a PTY buffer checker). Also, add at least one `.tape` file to serve as a VHS-style happy-path recording test for the Crush TUI.

## Files To Touch

- `internal/smithers/types.go` (Add structs: `Snapshot`, `Timeline`, `Diff`)
- `internal/smithers/client.go` (Add methods: `ListSnapshots`, `DiffSnapshots`, `ForkRun`, `ReplayRun`)
- `internal/smithers/client_test.go` (Add API mocks)
- `internal/ui/views/timeline.go` or `internal/ui/model/timeline.go` (Scaffold Bubble Tea model and Msg types)
- `tests/e2e/timeline_test.go` (New: Create terminal E2E test harness)
- `tests/vhs/timeline-happy-path.tape` (New: VHS recording test)