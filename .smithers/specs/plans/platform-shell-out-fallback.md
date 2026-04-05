## Goal

Harden and complete the CLI shell-out transport tier in `internal/smithers/client.go` so that every `Client` method can fall back to `exec.Command("smithers", ...)` when the HTTP server is unavailable, with structured errors, a configurable binary path, timeout and working-directory support, and full unit test coverage using the existing mock pattern.

---

## Steps

### 1. Extract exec infrastructure into exec.go

Move `execSmithers` from `client.go:248-262` into a new `internal/smithers/exec.go` file and extend it with structured errors, binary resolution, and configuration options.

New error types in `exec.go`:

```go
var ErrBinaryNotFound = errors.New("smithers binary not found")

type ExecError struct {
    Command string
    Stderr  string
    Exit    int
}
func (e *ExecError) Error() string { ... }

type JSONParseError struct {
    Command string
    Output  []byte
    Err     error
}
func (e *JSONParseError) Error() string { ... }
func (e *JSONParseError) Unwrap() error { return e.Err }
```

New client fields (added to `Client` struct):

```go
binaryPath  string        // default "smithers"
execTimeout time.Duration // 0 = no default timeout
workingDir  string        // "" = inherit TUI process cwd
logger      Logger
```

New client options:

```go
func WithBinaryPath(path string) ClientOption
func WithExecTimeout(d time.Duration) ClientOption
func WithWorkingDir(dir string) ClientOption
func WithLogger(l Logger) ClientOption
```

Logger interface (defined in `exec.go`, not reused from shell package):

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
}
```

Refactored `execSmithers`:
- Call `exec.LookPath(c.binaryPath)` before invoking; return `ErrBinaryNotFound` if not found.
- Set `cmd.Dir = c.workingDir` if non-empty.
- If `ctx` has no deadline and `c.execTimeout > 0`, wrap with `context.WithTimeout`.
- Wrap non-zero exits as `*ExecError` (not just a format string).
- Log: Debug on invocation and completion (with duration); Warn on binary-not-found and HTTP-to-exec fallback.

Binary check helper:

```go
func (c *Client) hasBinary() bool {
    _, err := exec.LookPath(c.binaryPath)
    return err == nil
}
```

`NewClient` default: set `c.binaryPath = "smithers"` in the constructor.

### 2. Add exec fallback for ListRuns and GetRun

These methods don't yet exist (tracked by `eng-smithers-client-runs`). This ticket either extends those methods with exec fallbacks, or provides standalone exec implementations that the HTTP-primary methods delegate to.

CLI surface:

| Method | CLI command | Parse helper |
|---|---|---|
| `ListRuns(ctx, RunFilter)` | `smithers ps --format json [--status s] [--limit n]` | `parseRunsJSON` |
| `GetRun(ctx, runID)` | `smithers inspect <runID> --format json` | `parseRunJSON` |

Parse helpers wrap failures in `&JSONParseError{Command: "ps", Output: data, Err: err}`.

If `eng-smithers-client-runs` has already implemented `ListRuns`/`GetRun` with HTTP, this ticket adds only the exec fallback branch and the parse helpers.

### 3. Add exec fallback for mutations: Approve, Deny, Cancel

These are the most critical exec paths — mutations cannot use the SQLite read-only tier, so exec is the only fallback when the server is down.

CLI surface:

| Method | CLI args | Notes |
|---|---|---|
| `Approve(ctx, runID, nodeID, iteration, note)` | `approve <runID> --node <nodeID> --iteration <i> --format json [--note <s>]` | omit `--note` if empty |
| `Deny(ctx, runID, nodeID, iteration, reason)` | `deny <runID> --node <nodeID> --iteration <i> --format json [--reason <s>]` | omit `--reason` if empty |
| `Cancel(ctx, runID)` | `cancel <runID> --format json` | discard output, return error only |

All three follow the HTTP-first then exec cascade. None have a SQLite tier.

### 4. Replace ListAgents stub with real exec

Replace the hardcoded 6-element stub at `client.go:108-117` with a real two-tier implementation:

1. HTTP `GET /agent/list` → `[]Agent` (if server available).
2. Exec `smithers agent list --format json` → `parseAgentsJSON`.

Graceful degradation: if exec fails with `ErrBinaryNotFound`, return `nil, nil` (empty agent list, no error). The agents view already handles an empty list without crashing. All other exec errors are propagated normally.

New parse helper:

```go
func parseAgentsJSON(data []byte) ([]Agent, error) {
    var agents []Agent
    if err := json.Unmarshal(data, &agents); err != nil {
        return nil, &JSONParseError{Command: "agent list", Output: data, Err: err}
    }
    return agents, nil
}
```

Downstream compatibility note: the existing `TestListAgents_NoOptions` test asserts 6 agents from the stub. After this change the test must be updated — with no exec func mock and no HTTP, the new implementation will call `hasBinary()` and either use exec or return empty. The test should be split into `TestListAgents_Stub_BackwardCompat` (kept for zero-config client) and `TestListAgents_Exec` (new mock-based test).

### 5. Add exec fallback for workflow operations

New types in `types.go`:

```go
type Workflow struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    Description string `json:"description,omitempty"`
}

type WorkflowRunResult struct {
    RunID string `json:"runId"`
}

type DoctorResult struct {
    OK     bool          `json:"ok"`
    Issues []DoctorIssue `json:"issues,omitempty"`
}

type DoctorIssue struct {
    Severity string `json:"severity"` // "error" | "warning" | "info"
    Message  string `json:"message"`
    Path     string `json:"path,omitempty"`
}
```

New client methods:

| Method | CLI command | Parse helper |
|---|---|---|
| `ListWorkflows(ctx)` | `workflow list --format json` | `parseWorkflowsJSON` |
| `RunWorkflow(ctx, path, inputJSON)` | `up <path> --input '<json>' --format json -d` | `parseWorkflowRunResultJSON` |
| `WorkflowDoctor(ctx, path)` | `workflow doctor <path> --format json` | `parseDoctorResultJSON` |

`RunWorkflow` uses `-d` (detached mode) so exec returns after the run starts, not after it completes. Input is marshaled to a JSON string and passed as a single `--input` argument. If `inputJSON` is empty, omit the `--input` flag.

### 6. Update existing parse helpers to use JSONParseError

Update the 9 existing parse helpers to wrap JSON failures in `*JSONParseError` instead of bare `fmt.Errorf`. This is a non-breaking change — `JSONParseError` implements `error` and wraps the underlying `err` via `Unwrap()`.

Methods to update: `parseSQLResultJSON`, `parseScoreRowsJSON`, `parseMemoryFactsJSON`, `parseRecallResultsJSON`, `parseCronSchedulesJSON`, `parseCronScheduleJSON`, `parseTicketsJSON`, `parseApprovalsJSON`, plus the new helpers from slices 2-5.

### 7. Wire exec diagnostics via Logger

Log at these call sites within `execSmithers`:
- **Debug**: `"exec invocation"` with `command`, `args`, `workingDir` before `cmd.Output()`.
- **Debug**: `"exec completed"` with `duration` and `outputBytes` after success.
- **Warn**: `"smithers binary not found"` with `binaryPath` when `hasBinary()` returns false.
- **Warn**: `"falling back to exec"` with `method` and `reason` at the call site in each method (log before the exec call).

Log calls are no-ops when no logger is configured (nil check or a `noopLogger` that satisfies the interface).

---

## Output Parsing Strategy

**Preferred format**: `--format json` is appended to every exec invocation that is expected to produce parseable output. All existing methods use this convention. New methods must do the same.

**JSON array vs. JSON object**: parse helpers are written to match the expected CLI output shape. When the shape is uncertain (e.g., `smithers ps` may return an array or a paginated wrapper), use the dual-parse pattern from `parseSQLResultJSON`:

```go
// Try the primary shape; if it fails with wrong structure, try the alternate.
var primary PrimaryType
if err := json.Unmarshal(data, &primary); err == nil && primary.isValid() {
    return &primary, nil
}
// Fall through to alternate shape.
```

**Fire-and-forget mutations**: commands that return `{ok: true}` or nothing on success should discard stdout and return only the exec error. Never try to parse output for these unless the parsed value is used by the caller.

**Empty responses**: a zero-byte response from exec is valid for some commands (e.g., `cron rm`). Parse helpers for array returns should treat empty input as an empty slice, not a parse error.

---

## Error Handling and Classification

After this ticket, callers can distinguish four exec error classes:

| Error | Type | How to detect |
|---|---|---|
| Binary not found | `ErrBinaryNotFound` (sentinel) | `errors.Is(err, ErrBinaryNotFound)` |
| Non-zero exit | `*ExecError` | `errors.As(err, &execErr)` |
| JSON parse failure | `*JSONParseError` | `errors.As(err, &parseErr)` |
| Context timeout/cancel | `context.DeadlineExceeded` / `context.Canceled` | `errors.Is(err, context.DeadlineExceeded)` |

View-layer error handling guidance:
- `ErrBinaryNotFound` → show "Smithers CLI not found. Install with: ..." prompt, render empty view.
- `*ExecError` with `Exit != 0` → show `execErr.Stderr` as the error message.
- `*JSONParseError` → show "Unexpected output from smithers CLI" + log `parseErr.Output` for debugging.
- `context.DeadlineExceeded` → show "Request timed out" with retry affordance.

---

## Testing Approach

### Unit tests using mock exec

All new exec paths use the `newExecClient(fn)` helper established in `client_test.go:49`:

```go
func newExecClient(fn func(ctx context.Context, args ...string) ([]byte, error)) *Client {
    return NewClient(withExecFunc(fn))
}
```

The `withExecFunc` option (already in `client.go:53`) bypasses the real `exec.CommandContext` call entirely. Test mocks return canned JSON or errors.

Test matrix for each new method:
1. **Happy path**: mock returns valid JSON; assert parsed result matches expected struct.
2. **Empty result**: mock returns `[]byte("[]")`; assert empty slice returned, no error.
3. **JSON parse error**: mock returns `[]byte("not json")`; assert `*JSONParseError` returned.
4. **Exec error**: mock returns `(nil, &ExecError{...})`; assert error propagated.
5. **Correct args**: use `assert.Equal(t, expectedArgs, args)` to verify CLI flag construction.

### Unit tests for exec infrastructure (exec_test.go)

New tests for the infrastructure layer itself:

```
TestExecSmithers_BinaryNotFound   — WithBinaryPath("/nonexistent") → ErrBinaryNotFound
TestExecSmithers_CustomBinaryPath — WithBinaryPath override is used in cmd construction
TestHasBinary_Found               — hasBinary() true when binary exists
TestHasBinary_NotFound            — hasBinary() false for non-existent path
TestExecError_Format              — ExecError.Error() includes Command, Exit, Stderr
TestJSONParseError_Format         — JSONParseError.Error() includes Command, underlying err
TestJSONParseError_Unwrap         — errors.Is/As works through JSONParseError
TestExecTimeout_ContextWrapped    — WithExecTimeout wraps background ctx with deadline
TestWorkingDir_SetOnCmd           — WithWorkingDir sets cmd.Dir (use a test binary that prints os.Getwd())
TestLogger_DebugOnInvocation      — mock Logger receives Debug call on exec
TestLogger_WarnOnBinaryNotFound   — mock Logger receives Warn on ErrBinaryNotFound
```

### Terminal E2E test

File: `tests/e2e/shell_out_fallback_e2e_test.go`

The test launches the TUI binary with no API URL configured (forces exec fallback), navigates to the agents view, and asserts the agents list renders. Skip if `smithers` binary is not on PATH. Uses the `TUIInstance` harness defined in `tests/e2e/tui_helpers_test.go` (modeled on upstream `../smithers/tests/tui-helpers.ts` semantics).

Run: `go test ./tests/e2e/... -run TestShellOutFallback -v -timeout 60s`

### VHS happy-path recording test

File: `tests/vhs/shell-out-fallback.tape`

Produces a GIF recording demonstrating: launch with no server → agents view (exec fallback) → workflows view (exec fallback) → quit. Run: `vhs tests/vhs/shell-out-fallback.tape`

---

## File Plan

- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go) — add exec fallbacks for ListRuns, GetRun, Approve, Deny, Cancel, ListAgents (replace stub), ListWorkflows, RunWorkflow, WorkflowDoctor; migrate parse helpers to JSONParseError
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go) — add Workflow, WorkflowRunResult, DoctorResult, DoctorIssue (coordinate with eng-smithers-client-runs for Run/RunFilter)
- `/Users/williamcory/crush/internal/smithers/exec.go` (new) — ErrBinaryNotFound, ExecError, JSONParseError, Logger, WithBinaryPath, WithExecTimeout, WithWorkingDir, WithLogger, hasBinary, refactored execSmithers
- `/Users/williamcory/crush/internal/smithers/exec_test.go` (new) — infrastructure unit tests
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go) — add exec tests for new methods, update TestListAgents_NoOptions
- `/Users/williamcory/crush/tests/e2e/shell_out_fallback_e2e_test.go` (new) — terminal E2E test
- `/Users/williamcory/crush/tests/e2e/tui_helpers_test.go` (new, if not already created by eng-smithers-client-runs) — TUIInstance harness
- `/Users/williamcory/crush/tests/vhs/shell-out-fallback.tape` (new) — VHS recording test

---

## Validation

1. `gofumpt -w internal/smithers/`
2. `go vet ./internal/smithers/...`
3. `go test ./internal/smithers/... -count=1 -v` — all existing tests pass; new tests pass
4. `go test ./internal/smithers/... -run 'TestExecSmithers|TestHasBinary|TestExecError|TestJSONParseError|TestExecTimeout|TestWorkingDir|TestLogger' -count=1 -v` — exec infrastructure tests
5. `go test ./internal/smithers/... -run 'TestListRuns_Exec|TestGetRun_Exec|TestApprove_Exec|TestDeny_Exec|TestCancel_Exec|TestListAgents_Exec|TestListAgents_BinaryNotFound|TestListWorkflows_Exec|TestRunWorkflow_Exec|TestWorkflowDoctor_Exec' -count=1 -v` — new method tests
6. `go test ./tests/e2e/... -run TestShellOutFallback -v -timeout 60s` (skip if smithers binary absent)
7. `vhs tests/vhs/shell-out-fallback.tape` (skip if vhs not installed)
8. `go test ./...` — full suite, no regressions

Manual verification:
- `SMITHERS_API_URL= go run . ` with smithers on PATH — agents view shows real CLI detections, workflow list shows discovered workflows.
- `PATH=/empty go run . ` — agents view shows empty list without crash; no panic in any view.
- Configure `smithers.binaryPath` in smithers-tui.json to a non-default path — verify exec uses configured path.

---

## Open Questions

1. Does the Smithers CLI `smithers agent list --format json` already exist and return the `Agent` struct shape, or does it need to be added to the TypeScript CLI first?
2. Does `smithers workflow list --format json` return the same shape as the `Workflow` struct proposed here, or does the CLI return a different envelope?
3. For `RunWorkflow`, does `smithers up <path> -d --format json` return `{runId}` immediately after starting the run, or does it block until the run completes?
4. Should `ListRuns` and `GetRun` exec fallbacks be implemented in this ticket (ahead of the HTTP paths in `eng-smithers-client-runs`), or should this ticket be sequenced after that one to avoid duplicate work?
5. The `TestListAgents_NoOptions` test currently asserts the 6-element stub. After replacing the stub with real exec, how should the zero-config backward compat test behave — should it assert empty result (no binary found), or should we preserve the stub as a final fallback when both HTTP and exec fail?
