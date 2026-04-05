## Existing Exec Fallback Audit

### What is already implemented

The exec transport tier lives entirely in `execSmithers` at [internal/smithers/client.go:248-262](/Users/williamcory/crush/internal/smithers/client.go#L248). It is a thin wrapper around `exec.CommandContext` with two behaviors: (a) honor an injected `execFunc` override for tests, or (b) run `exec.CommandContext(ctx, "smithers", args...)` and capture stdout.

The following `Client` methods already have an exec fallback as their final tier:

| Method | CLI args constructed | Parse helper |
|---|---|---|
| `ExecuteSQL` | `sql --query <q> --format json` | `parseSQLResultJSON` (double-parse: struct then array-of-maps) |
| `GetScores` | `scores <runID> --format json [--node <n>]` | `parseScoreRowsJSON` |
| `ListMemoryFacts` | `memory list <ns> --format json [--workflow <p>]` | `parseMemoryFactsJSON` |
| `RecallMemory` | `memory recall <q> --format json [--namespace <n>] [--topK <k>]` | `parseRecallResultsJSON` |
| `ListCrons` | `cron list --format json` | `parseCronSchedulesJSON` |
| `CreateCron` | `cron add <pat> <path> --format json` | `parseCronScheduleJSON` |
| `ToggleCron` | `cron toggle <id> --enabled <bool>` | none (fire-and-forget) |
| `DeleteCron` | `cron rm <id>` | none (fire-and-forget) |
| `ListTickets` | `ticket list --format json` | `parseTicketsJSON` |
| `ListPendingApprovals` | `approval list --format json` | `parseApprovalsJSON` |

The following `Client` methods have **no exec fallback**:
- `ListAgents` — returns a hardcoded 6-element stub; never calls exec.
- All runs methods (`ListRuns`, `GetRun`) — not yet implemented on `Client` at all (tracked by `eng-smithers-client-runs`).
- All mutation methods for runs (`Approve`, `Deny`, `Cancel`) — not yet implemented.
- All workflow methods (`ListWorkflows`, `RunWorkflow`, `WorkflowDoctor`) — not yet implemented.

### How execSmithers works today

```go
// client.go:248
func (c *Client) execSmithers(ctx context.Context, args ...string) ([]byte, error) {
    if c.execFunc != nil {
        return c.execFunc(ctx, args...)
    }
    cmd := exec.CommandContext(ctx, "smithers", args...)
    out, err := cmd.Output()
    if err != nil {
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {
            return nil, fmt.Errorf("smithers %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
        }
        return nil, fmt.Errorf("smithers %s: %w", strings.Join(args, " "), err)
    }
    return out, nil
}
```

Limitations of the current implementation:
1. **Binary hardcoded** — `"smithers"` is not configurable. If the CLI is installed under a different name or path, exec fails with an opaque OS error.
2. **No binary check** — the method calls `cmd.Output()` unconditionally; there is no pre-flight `exec.LookPath` to distinguish "binary not found" from other failures. This makes the error message unpredictable.
3. **Unstructured errors** — both `ExitError` and other errors are stringified and returned as opaque `error` values. Callers cannot distinguish "non-zero exit" from "timeout" from "binary not found".
4. **No timeout default** — if the caller passes a background context with no deadline, the exec call has no upper bound. A hanging `smithers` process blocks indefinitely.
5. **No working directory** — `cmd.Dir` is never set. The CLI discovers `.smithers/` project context from its working directory. If the TUI was launched from a directory outside the project root, all exec calls may return wrong or empty results.
6. **No logging** — there is no visibility into when exec is used vs. HTTP, or how long exec calls take. This makes it hard to diagnose performance regressions.

### Transport tier pattern (currently used)

Every multi-transport method follows this cascade:

```
1. if c.isServerAvailable() → HTTP (GET or POST with {ok,data,error} envelope)
2. if c.db != nil          → SQLite direct read (SELECT queries only)
3. execSmithers(...)        → CLI shell-out
```

Mutations always skip tier 2 (SQLite is read-only) and go directly from HTTP to exec. Pure exec-only operations (`RecallMemory`, `DeleteCron`) skip both HTTP and SQLite. The `isServerAvailable()` probe is cached for 30 seconds to avoid repeated HTTP round-trips on each method call.

---

## Smithers CLI Command Structure

The following subcommand surface is consumed by the exec tier (confirmed from engineering spec and existing client code):

### Query subcommands

| Subcommand | Args / Flags | Expected JSON output |
|---|---|---|
| `smithers ps` | `--format json [--status <s>] [--limit <n>]` | `[]Run` |
| `smithers inspect <runID>` | `--format json` | `Run` (with nodes) |
| `smithers approval list` | `--format json` | `[]Approval` |
| `smithers agent list` | `--format json` | `[]Agent` |
| `smithers workflow list` | `--format json` | `[]Workflow` |
| `smithers workflow doctor <path>` | `--format json` | `DoctorResult` |
| `smithers ticket list` | `--format json` | `[]Ticket` |
| `smithers cron list` | `--format json` | `[]CronSchedule` |
| `smithers scores <runID>` | `--format json [--node <n>]` | `[]ScoreRow` |
| `smithers memory list <ns>` | `--format json [--workflow <p>]` | `[]MemoryFact` |
| `smithers memory recall <q>` | `--format json [--namespace <n>] [--topK <k>]` | `[]MemoryRecallResult` |
| `smithers sql` | `--query <q> --format json` | `SQLResult` or `[]map` |

### Mutation subcommands

| Subcommand | Args / Flags | Expected JSON output |
|---|---|---|
| `smithers approve <runID>` | `--node <n> --iteration <i> [--note <s>] --format json` | `{runId, ok}` |
| `smithers deny <runID>` | `--node <n> --iteration <i> [--reason <s>] --format json` | `{runId, ok}` |
| `smithers cancel <runID>` | `--format json` | `{runId, ok}` |
| `smithers up <path>` | `--input '{}' --format json -d` | `{runId}` |
| `smithers cron add <pat> <path>` | `--format json` | `CronSchedule` |
| `smithers cron toggle <id>` | `--enabled <bool>` | none |
| `smithers cron rm <id>` | (none) | none |

### Flag convention: `--format json` vs `--json`

The engineering spec flags this as a risk: not all subcommands may support `--format json`. The existing client uses `--format json` uniformly. From code inspection, this is already in use for 10+ subcommands without complaint in tests, suggesting the convention is broadly supported. For subcommands that don't yet emit JSON (notably `cron toggle` and `cron rm`), the client ignores the output entirely and only checks for a non-zero exit code.

---

## Output Parsing Patterns

### JSON array (most query commands)

The majority of query commands return a JSON array of typed objects. The parse helpers follow a simple pattern:

```go
func parseXxxJSON(data []byte) ([]Xxx, error) {
    var items []Xxx
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, fmt.Errorf("parse xxx: %w", err)
    }
    return items, nil
}
```

All helpers in the existing client follow this pattern for: approvals, score rows, memory facts, recall results, cron schedules, tickets, and memory recall results.

### Dual-format parsing (SQL)

`parseSQLResultJSON` is the most complex parser. It handles two distinct JSON shapes from the CLI:
1. `SQLResult` struct with `columns` + `rows` arrays (columnar format).
2. `[]map[string]interface{}` (array-of-objects format — the more common CLI output).

```go
func parseSQLResultJSON(data []byte) (*SQLResult, error) {
    var result SQLResult
    if err := json.Unmarshal(data, &result); err == nil && len(result.Columns) > 0 {
        return &result, nil
    }
    var arr []map[string]interface{}
    if err := json.Unmarshal(data, &arr); err != nil {
        return nil, fmt.Errorf("parse SQL result: %w", err)
    }
    return convertResultMaps(arr), nil
}
```

This dual-parse pattern is a candidate for generalization. New parse helpers for runs and agents may need similar flexibility if the CLI format differs from the HTTP API format.

### Single-object parsing (CreateCron, RunWorkflow, GetRun)

```go
func parseCronScheduleJSON(data []byte) (*CronSchedule, error) {
    var cron CronSchedule
    if err := json.Unmarshal(data, &cron); err != nil {
        return nil, fmt.Errorf("parse cron schedule: %w", err)
    }
    return &cron, nil
}
```

### Fire-and-forget mutations (ToggleCron, DeleteCron, Cancel)

For commands that return no meaningful output on success, the client discards stdout:

```go
_, err := c.execSmithers(ctx, "cron", "rm", cronID)
return err
```

---

## Edge Cases

### Binary not found

When `smithers` is not on `$PATH`, `exec.CommandContext` returns an error whose underlying type is `*os.PathError` with `Err == exec.ErrNotFound`. The current implementation wraps this as:

```
smithers cron list --format json: exec: "smithers": executable file not found in $PATH
```

This is returned as a generic `error` — callers cannot type-switch on it. The `isServerAvailable()` check gates most fallbacks, but if both HTTP and exec fail for different reasons, the error from exec is the one returned to the caller.

Resolution: add `exec.LookPath(c.binaryPath)` as a pre-flight check and return a typed `ErrBinaryNotFound` sentinel. This allows callers (e.g., `ListAgents`) to handle the missing-binary case with a graceful empty result rather than an error.

### Non-zero exit code

A non-zero exit (e.g., `smithers approve` on an already-approved run) currently returns:

```
smithers approve run-1 --node n1 ...: <stderr content>
```

This is acceptable for mutations (caller gets context from stderr). However, for query methods, a non-zero exit almost always indicates a fatal error (no data available), and the stderr context is valuable for debugging. The engineering spec proposes a typed `ExecError` struct to capture `Command`, `Stderr`, and `Exit` separately so callers can format better user-facing messages.

### JSON parse failure

All 10 existing parse helpers wrap `json.Unmarshal` errors with `fmt.Errorf("parse xxx: %w", err)`. This loses the raw output that caused the failure, making it hard to diagnose format mismatches. The engineering spec proposes a `JSONParseError` type that captures the raw `Output []byte` and originating `Command string`.

### Context timeout / cancellation

`exec.CommandContext` already propagates context cancellation: when the context expires, the child process receives SIGKILL. The current implementation handles this transparently — the `exec.Output()` call returns a `context.DeadlineExceeded`-wrapped error. However, there is no default timeout. The fix (Slice 6 of the engineering spec) is a `WithExecTimeout` client option that wraps the context with a deadline when none is set.

### Working directory sensitivity

The `smithers` CLI discovers project configuration from the working directory (`.smithers/` directory structure). If the TUI is launched from `/tmp` or a home directory, exec commands for workflow list, ticket list, etc. may return empty results or errors rather than failing visibly. This is distinct from "binary not found" and is currently entirely silent.

Resolution: Slice 6 introduces `WithWorkingDir` which sets `cmd.Dir`. The TUI startup should detect the project root (the nearest ancestor containing `.smithers/`) and configure the client accordingly.

### Version mismatches

The Go `types.go` structs carry `json` tags that must match the CLI's output format. Additive changes (new fields) are safe with `omitempty` and pointer types. Breaking changes (field renames, type changes) silently break exec parsing while HTTP continues to work. The current types use a mix of `*string` and `*int64` pointer types for optional fields (e.g., `CronSchedule.LastRunAtMs`), which correctly absorbs nulls from JSON. New types for runs, agents, and workflows should follow the same pattern.

The test suite at [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go) uses `newExecClient` with a mock function — it does not test against real CLI output. This means version drift is not detected until the TUI is run against a live Smithers installation.

---

## Shell Package Review

The `internal/shell` package ([shell.go](/Users/williamcory/crush/internal/shell/shell.go), [background.go](/Users/williamcory/crush/internal/shell/background.go)) provides POSIX shell emulation via `mvdan.cc/sh/v3`. It is used by the Crush agent's `bash` tool for running shell commands interactively.

Key distinctions from `exec.CommandContext`:

| Dimension | `internal/shell.Shell` | `exec.CommandContext` (used by execSmithers) |
|---|---|---|
| Use case | Agent tool execution (bash tool), background jobs | One-shot CLI invocation for data transport |
| Shell parsing | Full POSIX shell syntax via `mvdan.cc/sh/v3` | Direct binary invocation, no shell |
| Environment | Inherits process env + injects `CRUSH=1`, `AGENT=crush` | Inherits process env (default) |
| CWD tracking | Stateful — shell updates its `cwd` after each `cd` | Stateless — `cmd.Dir` set per-call |
| Output | Buffered or streamed (`ExecStream`) | Buffered (`cmd.Output()`) |
| Concurrency | Protected by `sync.Mutex` per Shell | Each exec call is independent |
| Blocking | Has `BlockFunc` mechanism to deny specific commands | No blocking |

**Reuse assessment**: The shell package is not a candidate for direct reuse in `execSmithers`. The smithers client transport layer needs direct binary invocation (`exec.CommandContext`), not POSIX shell parsing. Using the shell package would add unnecessary overhead (shell parsing, env injection, mutex contention) and would require constructing shell command strings that could be fragile with argument quoting.

However, two concepts from the shell package are worth adopting:

1. **Logger interface** — `shell.Logger` (`InfoPersist(msg string, keysAndValues ...any)`) is simpler than what the engineering spec proposes (`Debug` + `Warn`). The smithers client should define its own `Logger` interface with `Debug` and `Warn` methods to allow integration with whatever logging backend the TUI uses.

2. **Working directory handling** — the shell package's `SetWorkingDir` with directory existence validation (`os.Stat`) is a good pattern for `WithWorkingDir` in the exec infrastructure.

The `shell.IsInterrupt` and `shell.ExitCode` helpers are specific to `interp.ExitStatus` from `mvdan.cc/sh/v3` and are not applicable to `exec.ExitError`. The smithers exec package needs its own exit code extraction via `(*exec.ExitError).ExitCode()`.

---

## Files To Touch

- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go) — add exec fallbacks for ListRuns, GetRun, Approve, Deny, Cancel, ListAgents (replace stub), ListWorkflows, RunWorkflow, WorkflowDoctor; update parse helpers to use JSONParseError
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go) — add Workflow, WorkflowRunResult, DoctorResult, DoctorIssue, Run, RunFilter (if not already added by eng-smithers-client-runs)
- `/Users/williamcory/crush/internal/smithers/exec.go` (new) — extract and extend execSmithers; add ErrBinaryNotFound, ExecError, JSONParseError, WithBinaryPath, WithExecTimeout, WithWorkingDir, WithLogger, hasBinary
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go) — add exec tests for new methods and JSONParseError paths
- `/Users/williamcory/crush/internal/smithers/exec_test.go` (new) — unit tests for exec infrastructure
- `/Users/williamcory/crush/tests/e2e/shell_out_fallback_e2e_test.go` (new) — terminal E2E test
- `/Users/williamcory/crush/tests/vhs/shell-out-fallback.tape` (new) — VHS recording test
