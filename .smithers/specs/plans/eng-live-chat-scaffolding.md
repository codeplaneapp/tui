## Goal
Ship the first Live Chat scaffold in Crush Smithers mode so a `LiveChatView` can be pushed onto the existing router stack for a run id, render placeholder streaming chat output, and pop back cleanly with `Esc`, without regressing current chat behavior.

## Steps
1. Stabilize router message flow before new work. Add or adjust a UI test in `internal/ui/model` to confirm routed Smithers views receive key events once per message, then fix any duplicate dispatch in `internal/ui/model/ui.go` so LiveChat behavior is deterministic.
2. Add live-chat domain types and stub client surface. Extend `internal/smithers/types.go` with `Run` and `ChatBlock`, and add `GetRun`, `GetChatOutput`, and `StreamChat` methods in `internal/smithers/client.go` that return deterministic scaffold data compatible with later HTTP plus SSE tickets.
3. Implement `LiveChatView` scaffold. Create `internal/ui/views/livechat.go` implementing current `views.View` (`Init`, `Update`, `View`, `Name`, `ShortHelp`), storing `runID`, client, metadata, streamed blocks, follow state, size, and error state. `Init` starts metadata plus stream commands; `Update` handles stream messages, resize, follow toggle, and `Esc` to `views.PopViewMsg`; `View` renders header, run metadata, and timestamped chat lines.
4. Wire navigation entry points. Add a command action in `internal/ui/dialog/actions.go` and command item in `internal/ui/dialog/commands.go` to open Live Chat (either parsed `/chat <run-id>` now, or deterministic scaffold id if parsing is deferred). Handle the new action in `internal/ui/model/ui.go` by pushing `views.NewLiveChatView(...)`, switching to `uiSmithersView`, and preserving existing pop-to-chat or landing behavior.
5. Add unit coverage first, then terminal E2E. Add tests for new smithers client methods and `LiveChatView` init/update/pop behavior. Add a Go terminal harness in `tests/tui/helpers_test.go` modeled on upstream `../smithers/tests/tui-helpers.ts` semantics (`waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, `terminate`), then add `tests/tui/livechat_e2e_test.go` modeled on upstream `../smithers/tests/tui.e2e.test.ts` flow.
6. Add VHS happy-path recording. Add one tape in this repo for launch -> open Live Chat scaffold -> observe streamed output -> `Esc` back, and wire a repeatable invocation via task or documented command.

## File Plan
1. `internal/smithers/types.go`.
2. `internal/smithers/client.go`.
3. `internal/smithers/client_test.go`.
4. `internal/ui/views/livechat.go`.
5. `internal/ui/views/livechat_test.go`.
6. `internal/ui/model/ui.go`.
7. `internal/ui/dialog/actions.go`.
8. `internal/ui/dialog/commands.go`.
9. `tests/tui/helpers_test.go`.
10. `tests/tui/livechat_e2e_test.go`.
11. `tests/vhs/livechat-happy-path.tape`.
12. `Taskfile.yaml` (if adding explicit `test:tui-livechat` and `test:vhs-livechat` tasks).

## Validation
1. Unit and compile checks: `go test ./internal/smithers -run LiveChat -v`; `go test ./internal/ui/views -run LiveChat -v`; `go test ./internal/ui/model -run Smithers -v`; `go test ./...`.
2. Terminal E2E coverage modeled on upstream `@microsoft/tui-test` pattern from `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`: `go test ./tests/tui -run TestLiveChatScaffoldNavigation -v -timeout 60s` and assert launch, route open, visible streamed text, and `Esc` return.
3. VHS happy-path recording in this repo: `vhs tests/vhs/livechat-happy-path.tape`; verify recording artifact shows the full happy path.
4. Manual smoke: `go run .`, open command palette, open Live Chat scaffold, verify run header and streamed output, press `Esc`, confirm return to previous screen.

## Open Questions
1. Should this ticket keep the existing `View() string` contract and defer `Draw(scr, area)` migration to a separate platform ticket, or fold that refactor into this work?
2. Should `/chat <run-id>` parsing be in scope now, or should this scaffold use a deterministic demo run id until run-list navigation lands?
3. Which upstream reference path is canonical in this workspace for parity checks (`../smithers` vs local mirror path), so E2E modeling is unambiguous?
4. Should VHS execution be required in CI for this ticket, and if yes, do we standardize on local `vhs` binary or a containerized wrapper?