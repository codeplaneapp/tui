# Engineering Spec: eng-smithers-client-runs

## Existing Crush Surface

- **`internal/smithers/client.go`**: Contains a stub `Client` struct (`type Client struct{}`) and a `ListAgents` method returning mock data. It has no HTTP transport logic, no endpoint configuration, and no state fields like base URLs or tokens.
- **`internal/smithers/types.go`**: Defines auxiliary models such as `Agent`, `SQLResult`, `ScoreRow`, `AggregateScore`, `MemoryFact`, `MemoryRecallResult`, and `CronSchedule`. It entirely lacks definitions for the core run lifecycle (e.g., `Run`, `Node`, `Attempt`, `Approval`, `SmithersEvent`).
- **`internal/ui/model/ui.go` and `internal/ui/model/keys.go`**: The TUI routing loop defines and manages `uiSmithersView`. In `ui.go`, `uiSmithersView` is handled natively in both `Update()` (line 1735) and `Draw()` (line 2097). Specifically, `Draw()` invokes `uv.NewStyledString(current.View()).Draw(scr, layout.main)`. This proves the `Draw()` implementation for Smithers views is already functional and present, refuting the potential risk identified in the initial ticket notes.

## Upstream Smithers Reference

- **`smithers/src/server/index.ts`**: The authoritative API implementation reveals that `/v1/runs` handles `GET` requests (supporting `limit` and `status` query params) by delegating to a `serverAdapter.listRuns()` database call. `POST /v1/runs` handles creation (requiring `workflowPath`). Error structures use `HttpError` yielding `{ error: { code, message } }`.
- **`smithers/gui/src/ui/RunsList.tsx`**: The existing SolidJS GUI view displays runs using a table with `Run ID`, `Status`, and `Age` columns. Key statuses matched are `finished`, `running`, `waiting-approval`, and `failed`. It includes client-side filtering via a "Pending Approvals Only" toggle checkbox.
- **`smithers/tests/tui.e2e.test.ts` & `smithers/tests/tui-helpers.ts`**: The upstream test suite uses a `BunSpawnBackend` wrapper to spawn a CLI process (`Bun.spawn([BUN, "run", TUI_ENTRY])`) and interact via piped `stdin`/`stdout`. It verifies flows like pressing `p` to toggle the pending inbox, `a` for the ask modal, and navigating up to Level 3 detail (Node Inspector).
- **`smithers/docs/guides/smithers-tui-v2-agent-handoff.md`**: Provides architectural context on a previous OpenTUI-based v2 implementation relying on a `MockBroker.ts` for dummy token streams and workflow run updates. This has been superseded by the Crush architecture, requiring a real HTTP client bridging to Go types instead.

## Gaps

1. **Data Model**: The Go types in `internal/smithers/types.go` do not capture the Run DAG. Missing structs: `Run`, `RunStatus`, `Node`, `NodeState`, `Attempt`, `Approval`, `SmithersEvent` (SSE discriminator envelope).
2. **Transport**: The `Client` struct in `internal/smithers/client.go` lacks an actual `*http.Client`, bearer token injection, and database path configuration for the SQLite fallback. There are no methods for `ListRuns`, `GetRun`, or mutating actions (`Approve`, `Deny`, `Cancel`, `HijackRun`). SSE stream consumption is completely unhandled.
3. **Rendering & Validation**: While the Crush `ui.go` properly renders views dynamically, the backend data provisioning is non-existent. The tests lack a Go-equivalent of `BunSpawnBackend` to execute end-to-end assertions via stdin/stdout piping as mandated by the project requirements.
4. **Risk Invalidation**: The ticket warned that `uiSmithersView` might be missing its `Draw()` case in the root model. Inspection of `internal/ui/model/ui.go` (line 2097) confirms `Draw()` is complete and correctly renders `current.View()`.

## Recommended Direction

1. **Model Implementation**: Add the missing data types (`Run`, `Node`, `Attempt`, `Approval`, `SmithersEvent`) to `internal/smithers/types.go` reflecting the exact JSON shapes yielded by the Drizzle models in `smithers/src/db/internal-schema.ts`.
2. **Client Augmentation**: Upgrade `internal/smithers/client.go` to accept a `ClientConfig` (API URL, token, DB path). Implement `ListRuns` and `GetRun` using `net/http`, implementing the SQLite read-only fallback (`?mode=ro&_journal_mode=WAL`) if the server ping fails.
3. **SSE Stream Consumer**: Create `internal/smithers/events.go` using a `bufio.Scanner` to consume `GET /v1/runs/:runId/events?afterSeq=N`. Parse the server-sent events, manage heartbeats/reconnections, and yield them as a Go channel `<-chan SmithersEvent` that translates into Bubble Tea `tea.Msg` events.
4. **Mutations**: Implement `Approve()`, `Deny()`, `Cancel()`, and `HijackRun()` via HTTP POSTs. Note that these methods do not support SQLite fallback; return explicit connectivity errors if the server is unreachable.
5. **E2E Testing**: Port `BunSpawnBackend` to Go using `exec.Command`. Write a `tests/tui_runs_e2e_test.go` harness that starts an `httptest.Server`, spawns the built Crush binary, and asserts visual text output based on ANSI-stripped buffer polling. Add a `runs_dashboard.tape` VHS recording.

## Files To Touch

- `internal/smithers/types.go` (Append Run/Node/Event structs)
- `internal/smithers/client.go` (Inject HTTP client, add List/Get/Mutate methods)
- `internal/smithers/events.go` (Create SSE consumer)
- `internal/smithers/types_test.go` (Create unit tests)
- `internal/smithers/client_test.go` (Create HTTP mock tests)
- `internal/smithers/events_test.go` (Create SSE parser tests)
- `tests/tui_runs_e2e_test.go` (Create Go-based TUI harness ported from `tui-helpers.ts`)
- `tests/tapes/runs_dashboard.tape` (Create visual recording)