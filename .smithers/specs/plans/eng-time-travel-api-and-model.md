# Implementation Plan: eng-time-travel-api-and-model

## Goal
Add Smithers client methods for snapshot operations (ListSnapshots, DiffSnapshots, ForkRun, ReplayRun) and basic Bubble Tea model scaffolding for the Timeline view, including required E2E and VHS tests.

## Steps
1. **Define Data Models:** Add `Snapshot`, `Timeline`, and `Diff` structs to `internal/smithers/types.go`.
2. **Implement Client API:** Add `ListSnapshots`, `DiffSnapshots`, `ForkRun`, and `ReplayRun` methods to `internal/smithers/client.go` and the corresponding interface (e.g., in `internal/smithers/smithers.go`).
3. **Scaffold Bubble Tea View:** Create `internal/ui/views/timeline.go` (or `internal/ui/model/timeline.go`). Define the base `TimelineModel`, `Init`, `Update`, and `View` methods, along with essential message types like `TimelineLoadedMsg`, `SnapshotSelectedMsg`, and `ReplayRequestedMsg`.
4. **Create API Mocks:** Implement mocks for the new client methods to facilitate offline testing and E2E test setups.
5. **Implement Terminal E2E Test:** Create an E2E test for the Timeline view modeled after the upstream `@microsoft/tui-test` harness (`../smithers/tests/tui.e2e.test.ts` and `tui-helpers.ts`). This should spawn the TUI, send keystrokes, and assert on the terminal buffer output.
6. **Create VHS Recording Test:** Create a `.tape` file (e.g., `tests/vhs/timeline-happy-path.tape`) to serve as a VHS-style happy-path recording test for navigating to and interacting with the Timeline view.

## File Plan
- `internal/smithers/types.go` (Add data models)
- `internal/smithers/client.go` (Add API methods)
- `internal/smithers/smithers.go` (Update interface)
- `internal/smithers/mock.go` or `internal/smithers/client_test.go` (Add mocks)
- `internal/ui/views/timeline.go` (New Bubble Tea model and Msg types)
- `tests/e2e/timeline_test.go` (New terminal E2E test harness)
- `tests/vhs/timeline-happy-path.tape` (New VHS recording script)

## Validation
- **Compilation:** Run `go build ./...` to verify no syntax or type errors.
- **Unit Tests:** Run `go test ./internal/smithers/...` to ensure new client methods and mocks are correct.
- **E2E Test:** Run `go test ./tests/e2e/...` to execute the terminal E2E path modeled on the upstream `@microsoft/tui-test` harness, confirming keystroke processing and buffer updates.
- **VHS Test:** Run `vhs tests/vhs/timeline-happy-path.tape` manually or in CI to generate a visual recording of the timeline interaction and verify it completes without errors.

## Open Questions
- Should the Timeline view be accessible directly via a CLI flag (e.g., `crush --timeline <runID>`), or only navigated to from an active chat view?
- Do we need to handle pagination for `ListSnapshots` if a run has hundreds of snapshots, or will the backend handle truncation/filtering initially?