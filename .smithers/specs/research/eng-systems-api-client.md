# Research Document: eng-systems-api-client

## Existing Crush Surface

The core API bindings for the Systems and Analytics views are already implemented in the Go backend:
- `internal/smithers/client.go` includes robust dual-mode transport routing logic (HTTP first, SQLite or `exec` fallback) and all 9 API methods (`ExecuteSQL`, `GetScores`, `GetAggregateScores`, `ListMemoryFacts`, `RecallMemory`, `ListCrons`, `CreateCron`, `ToggleCron`, `DeleteCron`).
- `internal/smithers/types.go` maps correctly to the upstream data structures (`SQLResult`, `ScoreRow`, `MemoryFact`, etc.).
- `internal/smithers/client_test.go` provides 20+ unit tests that validate the transport logic and data mapping.

However, a directory inspection of `internal/ui/views/` confirms that the UI view layers consuming these APIs (e.g., `sqlbrowser.go`, `triggers.go`) are absent. The `model/ui.go` file exists but lacks wiring to actual system analytics views, indicating the scope of this ticket must solely remain on verifying the data layer via test scaffolding, stopping at the UI client edge.

## Upstream Smithers Reference

- **Testing Harness:** The upstream Smithers repository uses a robust `BunSpawnBackend` found in `../smithers/tests/tui-helpers.ts`. It leverages `Bun.spawn` to spin up the TUI process and captures terminal output buffers while aggressively filtering ANSI escapes. It provides simple APIs like `waitForText` and `sendKeys` with a poll loop (100ms interval / 10s timeout).
- **Integration Flow:** `../smithers/tests/tui.e2e.test.ts` actively uses this harness to test UI layout components like the "Inspector" and "TopBar" end-to-end.
- **Feature Catalog:** `docs/smithers-tui/features.ts` clearly segments systems functionalities (`SQL_BROWSER`, `TRIGGERS_LIST`, `SCORES_AND_ROI_DASHBOARD`, `MEMORY_BROWSER`) providing a rigid taxonomy for what must eventually be rendered.
- **VHS Demos:** Upstream documentation heavily relies on VHS recordings (e.g. `demo/smithers/tapes/`) to assert standard TUI functionalities do not panic while producing visual smoke tests.

## Gaps

1. **E2E Testing Harness:** Crush lacks a native Go implementation of the `BunSpawnBackend` testing pattern. It requires an equivalent mechanism to spawn the TUI with pseudoterminal parameters (`TERM=xterm-256color`) to validate systems operations against a SQLite fixture database without mocking the process boundary.
2. **Visual/Smoke Testing:** There are currently no automated VHS tapes verifying the visual happy path of the `eng-systems-api-client` views.
3. **View Implementations:** Despite the client bindings existing, the view layers do not exist yet. E2E tests will have to be constructed carefully with view stubs to satisfy the spec requirements.
4. **Transport Mismatch:** As noted in the spec, `DeleteCron` and semantic `RecallMemory` queries do not have a 1:1 local fallback or stable HTTP equivalent to match TypeScript behavior, necessitating strict `exec.Command` fallbacks.

## Recommended Direction

1. **Port the TUI Harness:** Implement a `TUIHarness` in Go within `tests/tui_e2e_test.go`. Modeled directly after `tui-helpers.ts`, it should use `os/exec` to execute the TUI binary, continuously read `StdoutPipe`, string-strip ANSI sequences using a regex, and implement `WaitForText(text string, timeout time.Duration)` and `SendKeys(s string)`.
2. **Develop E2E Tests:** Implement `TestSystemsViews_E2E`, `TestSQLBrowser_E2E`, and `TestTriggersView_E2E`. Use a fixture database mapped to `--db` to mock the environment natively. Drive the UI via command-palette inputs.
3. **Create VHS Tape:** Construct `tests/tapes/systems-api-client.tape` to interactively document the expected SQL and Triggers navigation using keyboard commands like `Enter`, `Type "sql"`, and `Type "triggers"`.
4. **Handle View Mismatch:** Acknowledge that actual view files aren't built. Scaffold minimal view routing stubs so tests can pass, or delay E2E/VHS implementation until rendering tickets catch up. For this ticket, lay down the framework needed for testing the bindings.

## Files To Touch

- `tests/tui_e2e_test.go` (new test harness and E2E suites)
- `tests/tapes/systems-api-client.tape` (new VHS tape recording)
- `internal/ui/views/sqlbrowser.go` (new stubs, if necessary for testing)
- `internal/ui/views/triggers.go` (new stubs, if necessary for testing)