# Platform: Shell Out Fallback — Engineering Specification

## Metadata
- ID: platform-shell-out-fallback
- Feature: PLATFORM_SHELL_OUT_FALLBACK
- Dependencies: platform-thin-frontend-layer
- Ticket: `.smithers/tickets/platform-shell-out-fallback.md`

---

## Objective

Complete the CLI shell-out transport tier in `internal/smithers/client.go` so that **every mutation and query method** on `Client` can fall back to `exec.Command("smithers", ...)` when the Smithers HTTP server is unreachable. Today the exec fallback exists for a subset of methods (SQL, scores, memory, crons, tickets) but is missing for runs, approvals, workflows, and agents. This ticket fills those gaps, standardizes the `--json` / `--format json` flag convention, adds structured error mapping, and introduces a configurable `smithers` binary path so the TUI works when `smithers` is not on `$PATH`.

The exec transport is the *last-resort* tier (after HTTP and direct SQLite). It is the **only** transport that supports mutations when no server is running, making it critical for the "zero-server" developer experience described in PRD §11 (Architecture Principle: Thin Frontend).

---

## Scope

### In scope

- Exec-backed implementations for all `Client` methods that currently lack a CLI fallback: `ListRuns`, `GetRun`, `Approve`, `Deny`, `Cancel`, `ListAgents` (replacing the hardcoded stub), `ListWorkflows`, `RunWorkflow`, `WorkflowDoctor`
- A configurable binary path (`WithBinaryPath` option) so the TUI can locate `smithers` outside of `$PATH`
- Structured error types for common CLI failure modes (binary not found, non-zero exit, JSON parse failure, timeout)
- A `--json` / `--format json` normalization layer that tries `--format json` first (the standard Smithers flag) and detects whether the CLI supports it
- Timeout propagation from `context.Context` to `exec.CommandContext`
- Unit tests for every new exec path using the existing `newExecClient` / `withExecFunc` mock pattern
- Terminal E2E test covering the exec fallback path (no server running)
- VHS happy-path recording test for the shell-out fallback scenario

### Out of scope

- SSE streaming fallback (SSE inherently requires a running server; no exec equivalent)
- Direct SQLite write access (reads are covered by the existing `queryDB` path; writes remain server/exec only)
- New Smithers CLI subcommands — this ticket consumes existing CLI surface only
- MCP tool wrappers — covered by `feat-mcp-*-tools` tickets

---

## Implementation Plan

### Slice 1: Structured exec error types and binary resolution

**Files**: `internal/smithers/exec.go` (new), `internal/smithers/client.go`

Extract the existing `execSmithers` method from `client.go:248-262` into a dedicated `exec.go` file and extend it with:

1. **New error types**:

```go
// ErrBinaryNotFound is returned when the smithers CLI binary cannot be located.
var ErrBinaryNotFound = errors.New("smithers binary not found")

// ExecError wraps a non-zero exit from the smithers CLI with structured fields.
type ExecError struct {
    Command string   // e.g. "smithers ps --format json"
    Stderr  string   // captured stderr
    Exit    int      // exit code
}

func (e *ExecError) Error() string {
    return fmt.Sprintf("smithers %s (exit %d): %s", e.Command, e.Exit, e.Stderr)
}

// JSONParseError wraps a JSON decode failure from CLI output.
type JSONParseError struct {
    Command string
    Output  []byte
    Err     error
}

func (e *JSONParseError) Error() string {
    return fmt.Sprintf("parse output of smithers %s: %s", e.Command, e.Err)
}
```

2. **`WithBinaryPath` client option**:

```go
// WithBinaryPath sets the path to the smithers CLI binary.
// Defaults to "smithers" (resolved via $PATH).
func WithBinaryPath(path string) ClientOption {
    return func(c *Client) { c.binaryPath = path }
}
```

Add `binaryPath string` field to `Client`. Default to `"smithers"`. The `execSmithers` method uses `c.binaryPath` instead of the hardcoded `"smithers"` string.

3. **Binary existence check**:

```go
// hasBinary returns true if the smithers CLI binary can be found.
func (c *Client) hasBinary() bool {
    _, err := exec.LookPath(c.binaryPath)
    return err == nil
}
```

This is called before attempting exec fallback so methods can return `ErrNoTransport` early instead of an opaque "file not found" error.

4. **Refactored `execSmithers`**:

Move from `client.go` to `exec.go`. Add binary check, structured error wrapping, and stderr capture:

```go
func (c *Client) execSmithers(ctx context.Context, args ...string) ([]byte, error) {
    if c.execFunc != nil {
        return c.execFunc(ctx, args...)
    }
    if !c.hasBinary() {
        return nil, ErrBinaryNotFound
    }
    cmd := exec.CommandContext(ctx, c.binaryPath, args...)
    out, err := cmd.Output()
    if err != nil {
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {
            return nil, &ExecError{
                Command: strings.Join(args, " "),
                Stderr:  strings.TrimSpace(string(exitErr.Stderr)),
                Exit:    exitErr.ExitCode(),
            }
        }
        return nil, fmt.Errorf("smithers %s: %w", strings.Join(args, " "), err)
    }
    return out, nil
}
```

**Validation**: Unit tests for `ExecError`, `JSONParseError` error formatting. Test `hasBinary()` returns false when binary path is `/nonexistent/smithers`. Test that `WithBinaryPath` overrides the default.

---

### Slice 2: Exec fallback for ListRuns and GetRun

**File**: `internal/smithers/client.go`

Add exec fallback paths to the runs query methods. These methods don't exist yet on the Client (they're planned by `eng-smithers-client-runs`), so this slice either extends those methods or provides standalone exec implementations that the HTTP-primary methods can call.

The Smithers CLI surface for runs:

| Method | CLI Command | Expected Output |
|--------|------------|-----------------|
| `ListRuns` | `smithers ps --format json` | `[{ "runId", "workflowName", "status", ... }]` |
| `GetRun` | `smithers inspect <runID> --format json` | `{ "runId", "workflowName", "status", "nodes": [...] }` |

Implementation pattern (matches existing `ExecuteSQL` cascading):

```go
func (c *Client) ListRuns(ctx context.Context, filter RunFilter) ([]Run, error) {
    // 1. Try HTTP
    if c.isServerAvailable() {
        // ... HTTP path ...
    }

    // 2. Fall back to exec
    args := []string{"ps", "--format", "json"}
    if filter.Status != "" {
        args = append(args, "--status", filter.Status)
    }
    if filter.Limit > 0 {
        args = append(args, "--limit", strconv.Itoa(filter.Limit))
    }
    out, err := c.execSmithers(ctx, args...)
    if err != nil {
        return nil, err
    }
    return parseRunsJSON(out)
}
```

**JSON parse helpers** (in `client.go` near existing parse helpers):

```go
func parseRunsJSON(data []byte) ([]Run, error) {
    var runs []Run
    if err := json.Unmarshal(data, &runs); err != nil {
        return nil, &JSONParseError{Command: "ps", Output: data, Err: err}
    }
    return runs, nil
}

func parseRunJSON(data []byte) (*Run, error) {
    var run Run
    if err := json.Unmarshal(data, &run); err != nil {
        return nil, &JSONParseError{Command: "inspect", Output: data, Err: err}
    }
    return &run, nil
}
```

**Validation**: Unit test with `newExecClient` mock returning canned JSON for `ps --format json` and `inspect <id> --format json`. Verify correct arg construction with filters. Test JSON parse error path.

---

### Slice 3: Exec fallback for mutations (Approve, Deny, Cancel)

**File**: `internal/smithers/client.go`

Add exec fallback to mutation methods. These are the most important fallback paths because mutations cannot use the SQLite read-only tier.

| Method | CLI Command | Expected Output |
|--------|------------|-----------------|
| `Approve` | `smithers approve <runID> --node <nodeID> --iteration <N> --note "..." --format json` | `{ "runId": "...", "ok": true }` |
| `Deny` | `smithers deny <runID> --node <nodeID> --iteration <N> --reason "..." --format json` | `{ "runId": "...", "ok": true }` |
| `Cancel` | `smithers cancel <runID> --format json` | `{ "runId": "...", "ok": true }` |

Implementation for Approve:

```go
func (c *Client) Approve(ctx context.Context, runID, nodeID string, iteration int, note string) error {
    // 1. Try HTTP
    if c.isServerAvailable() {
        return c.httpPostJSON(ctx,
            fmt.Sprintf("/v1/runs/%s/nodes/%s/approve", runID, nodeID),
            map[string]any{"iteration": iteration, "note": note}, nil)
    }

    // 2. Fall back to exec
    args := []string{"approve", runID, "--node", nodeID,
        "--iteration", strconv.Itoa(iteration), "--format", "json"}
    if note != "" {
        args = append(args, "--note", note)
    }
    _, err := c.execSmithers(ctx, args...)
    return err
}
```

Deny and Cancel follow the same pattern.

**Validation**: Unit tests asserting correct CLI args for each mutation. Test that `--note` / `--reason` flags are omitted when the value is empty. Test error propagation from `ExecError`.

---

### Slice 4: Exec fallback for ListAgents (replace stub)

**File**: `internal/smithers/client.go`

Replace the hardcoded stub in `ListAgents` (`client.go:108-117`) with a real implementation that shells out to the `smithers` CLI for agent detection.

The Smithers CLI exposes agent detection via `smithers agent list --format json`, which returns an array of agent availability objects matching the `Agent` struct in `types.go`.

```go
func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
    // 1. Try HTTP
    if c.isServerAvailable() {
        var agents []Agent
        if err := c.httpGetJSON(ctx, "/agent/list", &agents); err == nil {
            return agents, nil
        }
    }

    // 2. Fall back to exec
    out, err := c.execSmithers(ctx, "agent", "list", "--format", "json")
    if err != nil {
        // If exec also fails (binary not found, etc.), return empty list
        // with a logged warning rather than an error — the agents view
        // should degrade gracefully.
        if errors.Is(err, ErrBinaryNotFound) {
            return nil, nil
        }
        return nil, err
    }
    return parseAgentsJSON(out)
}

func parseAgentsJSON(data []byte) ([]Agent, error) {
    var agents []Agent
    if err := json.Unmarshal(data, &agents); err != nil {
        return nil, &JSONParseError{Command: "agent list", Output: data, Err: err}
    }
    return agents, nil
}
```

**Mismatch note**: The current stub returns 6 hardcoded agents. Downstream code in `internal/ui/views/agents.go` consumes the `[]Agent` slice directly. The switch to real data is backward-compatible because the view already handles the `Agent` struct fields. However, the AgentsView currently displays the stub data unconditionally — after this change, it will show real detection results or an empty list if `smithers` is not installed.

**Validation**: Unit test with mock returning agent detection JSON. Test the `ErrBinaryNotFound` graceful degradation path (returns nil, nil). Verify `parseAgentsJSON` handles both the full agent object and minimal objects.

---

### Slice 5: Exec fallback for workflow operations

**File**: `internal/smithers/client.go`

Add exec paths for workflow management commands:

| Method | CLI Command | Expected Output |
|--------|------------|-----------------|
| `ListWorkflows` | `smithers workflow list --format json` | `[{ "name", "path", "description" }]` |
| `RunWorkflow` | `smithers up <path> --input '{}' --format json -d` | `{ "runId": "..." }` |
| `WorkflowDoctor` | `smithers workflow doctor <path> --format json` | `{ "ok": bool, "issues": [...] }` |

New types in `types.go`:

```go
// Workflow represents a discovered workflow definition.
type Workflow struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    Description string `json:"description,omitempty"`
}

// WorkflowRunResult is the response from starting a workflow.
type WorkflowRunResult struct {
    RunID string `json:"runId"`
}

// DoctorResult is the response from workflow doctor.
type DoctorResult struct {
    OK     bool           `json:"ok"`
    Issues []DoctorIssue  `json:"issues,omitempty"`
}

type DoctorIssue struct {
    Severity string `json:"severity"` // "error" | "warning" | "info"
    Message  string `json:"message"`
    Path     string `json:"path,omitempty"`
}
```

**Validation**: Unit tests for each exec path. Test `RunWorkflow` with and without input JSON. Test `WorkflowDoctor` with ok=true and ok=false responses.

---

### Slice 6: Exec timeout and working directory support

**File**: `internal/smithers/exec.go`

Add two enhancements to the exec infrastructure:

1. **Explicit timeout**: While `exec.CommandContext` respects context deadlines, add a `WithExecTimeout` option for a default timeout when the context has none:

```go
func WithExecTimeout(d time.Duration) ClientOption {
    return func(c *Client) { c.execTimeout = d }
}
```

In `execSmithers`, if `ctx` has no deadline and `c.execTimeout > 0`, wrap with `context.WithTimeout`.

2. **Working directory**: The `smithers` CLI behavior depends on the working directory (it discovers `.smithers/` from cwd). Add `WithWorkingDir`:

```go
func WithWorkingDir(dir string) ClientOption {
    return func(c *Client) { c.workingDir = dir }
}
```

In `execSmithers`, set `cmd.Dir = c.workingDir` if non-empty. This ensures the CLI discovers the correct project context.

**Validation**: Test that timeout triggers `context.DeadlineExceeded` error. Test that `cmd.Dir` is set correctly.

---

### Slice 7: Update existing exec paths to use structured errors

**File**: `internal/smithers/client.go`

Update existing methods that already use `execSmithers` to benefit from the new structured error types. The methods to update:

- `ExecuteSQL` (line 290)
- `GetScores` (line 335)
- `ListMemoryFacts` (line 374)
- `RecallMemory` (line 391)
- `ListCrons` (line 424)
- `CreateCron` (line 446)
- `ToggleCron` (line 466)
- `DeleteCron` (line 474)
- `ListTickets` (line 493)

The JSON parse helpers (`parseSQLResultJSON`, `parseScoreRowsJSON`, etc.) should wrap parse failures in `JSONParseError` for consistent error handling across all exec paths.

**Validation**: Existing tests continue to pass. Add one new test per method verifying that a malformed JSON response from exec produces a `JSONParseError`.

---

### Slice 8: Transport logging and diagnostics

**File**: `internal/smithers/exec.go`

Add an optional `Logger` interface to the client for transport-level diagnostics:

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
}

func WithLogger(l Logger) ClientOption {
    return func(c *Client) { c.logger = l }
}
```

Log at these points:
- **Debug**: Every exec invocation with command and args
- **Warn**: When falling back from HTTP to exec (server unavailable)
- **Warn**: When `smithers` binary is not found
- **Debug**: Exec duration and output size

This helps operators diagnose why the TUI is slow (exec is ~100-500ms per invocation vs ~10ms for HTTP).

**Validation**: Unit test with a mock logger asserting log calls are emitted at correct levels.

---

## Validation

### Unit tests

```bash
go test ./internal/smithers/... -v -count=1
```

Expected new test coverage:

| File | Tests |
|------|-------|
| `exec.go` | `TestExecSmithers_BinaryNotFound`, `TestExecSmithers_ExitError`, `TestExecSmithers_Timeout`, `TestExecSmithers_WorkingDir`, `TestExecSmithers_CustomBinaryPath`, `TestHasBinary` |
| `client.go` | `TestListRuns_Exec`, `TestGetRun_Exec`, `TestApprove_Exec`, `TestDeny_Exec`, `TestCancel_Exec`, `TestListAgents_Exec`, `TestListAgents_BinaryNotFound_Graceful`, `TestListWorkflows_Exec`, `TestRunWorkflow_Exec`, `TestWorkflowDoctor_Exec` |
| `client.go` (existing) | `TestExecuteSQL_Exec_JSONParseError`, `TestListCrons_Exec_JSONParseError` (updated to check `JSONParseError` type) |

All exec tests use the `newExecClient(func(...))` mock pattern established in `client_test.go:49-51`.

### Terminal E2E test (modeled on upstream @microsoft/tui-test harness)

**File**: `tests/e2e/shell_out_fallback_e2e_test.go`

The upstream Smithers E2E tests in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts` use a `BunSpawnBackend` that:
1. Launches the TUI binary as a child process
2. Provides `waitForText(text, timeout)` for asserting rendered content
3. Provides `sendKeys(text)` for simulating input
4. Strips ANSI sequences before matching
5. Takes snapshots on failure for debugging

The Crush E2E equivalent:

**File**: `tests/e2e/tui_helpers_test.go`

```go
type TUIInstance struct {
    cmd    *exec.Cmd
    stdin  io.Writer
    stdout *ANSIBuffer  // strips ANSI codes, provides WaitForText
}

func launchTUI(t *testing.T, args ...string) *TUIInstance
func (t *TUIInstance) WaitForText(text string, timeout time.Duration) error
func (t *TUIInstance) WaitForNoText(text string, timeout time.Duration) error
func (t *TUIInstance) SendKeys(text string)
func (t *TUIInstance) Snapshot() string
func (t *TUIInstance) Terminate()
```

**Test flow** (no server running, exec fallback exercised):

```go
func TestShellOutFallback_E2E(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    // 1. Ensure smithers binary is on PATH (or skip)
    if _, err := exec.LookPath("smithers"); err != nil {
        t.Skip("smithers binary not found, skipping exec fallback E2E")
    }

    // 2. Launch TUI with NO server URL (forces exec fallback)
    tui := launchTUI(t, "--smithers-api-url", "")
    defer tui.Terminate()

    // 3. Wait for initial render
    tui.WaitForText("SMITHERS", 5*time.Second)

    // 4. Open command palette and navigate to agents
    tui.SendKeys("/agents\n")
    tui.WaitForText("Agents", 5*time.Second)

    // 5. Verify agent list rendered (exec fallback fired)
    // At minimum, known agent names should appear if CLIs are installed
    tui.WaitForText("Claude Code", 10*time.Second)

    // 6. Back to chat
    tui.SendKeys("\x1b") // Escape
    tui.WaitForText("SMITHERS", 3*time.Second)
}
```

Run:
```bash
go test ./tests/e2e/... -run TestShellOutFallback -v -timeout 60s
```

### VHS happy-path recording test

**File**: `tests/vhs/shell-out-fallback.tape`

```vhs
Output shell-out-fallback.gif
Set Shell bash
Set FontSize 14
Set Width 120
Set Height 40

# Launch TUI with no server (forces exec fallback for all operations)
Type "SMITHERS_API_URL= ./smithers-tui"
Enter
Sleep 2s

# Open command palette
Type "/"
Sleep 500ms

# Navigate to agents view (will use exec fallback)
Type "agents"
Sleep 500ms
Enter
Sleep 3s

# Agents list should render via smithers agent list --format json
Sleep 2s

# Back to chat
Escape
Sleep 1s

# Try workflows view
Type "/"
Sleep 500ms
Type "workflows"
Enter
Sleep 3s

# Workflow list should render via smithers workflow list --format json
Sleep 2s

# Quit
Ctrl+C
```

Run:
```bash
vhs tests/vhs/shell-out-fallback.tape
```

The VHS test produces a GIF recording and exits 0 if the TUI renders both views without crashing. It validates the shell-out fallback happy path: launch without server → agents view via exec → workflows view via exec → quit.

### Manual verification

1. **No server running**: `SMITHERS_API_URL= go run . ` — verify agents view shows detected CLIs, workflows view shows discovered workflows
2. **Server running then stopped**: Start with server, verify HTTP transport. Kill server, retry operations — verify transparent fallback to exec with no user-visible error
3. **Binary not found**: `PATH=/empty go run . ` — verify graceful degradation (empty views, no crash)
4. **Custom binary path**: Configure `"smithers": { "binaryPath": "/opt/smithers/bin/smithers" }` — verify exec uses the configured path
5. **Timeout**: Run with a workflow that takes >30s, verify context cancellation propagates to the exec subprocess

---

## Risks

### 1. Smithers CLI `--format json` flag inconsistency

**Risk**: Not all Smithers CLI subcommands support `--format json`. Some use `--json` as a boolean flag, others have no JSON output mode. If a subcommand doesn't support the flag, the CLI may error or return human-readable text that fails JSON parsing.

**Mitigation**: Audit the Smithers CLI source (`../smithers/src/cli/`) for each subcommand used in this ticket. The `parseXxxJSON` helpers already handle parse failures gracefully. Add a comment in `exec.go` documenting which CLI commands have confirmed JSON support. For commands without JSON support, parse the human-readable output or file an upstream issue.

### 2. CLI output format drift

**Risk**: The JSON shape returned by `smithers ps --format json` may change between Smithers versions. The Go types in `types.go` assume a specific shape. A field rename or structural change silently breaks the exec path while the HTTP path (with its own schema) continues to work.

**Mitigation**: Pin expected JSON shapes in unit tests with fixture data derived from actual CLI output. Run these tests in CI against a specific Smithers version. Use `json:"...,omitempty"` and pointer types for optional fields to absorb additive changes without breaking.

### 3. Exec latency vs. HTTP latency

**Risk**: Each `exec.Command("smithers", ...)` call spawns a new process, which has ~100-500ms overhead (Node.js/Bun startup). For views that make multiple API calls (e.g., runs dashboard fetching list + details), the exec path may feel sluggish.

**Mitigation**: The exec path is explicitly a fallback — the HTTP server is the primary transport. Add transport-level logging (Slice 8) so users see "falling back to CLI" warnings and can start the server. In the future, consider batching multiple exec calls or caching exec results with a short TTL.

### 4. Concurrent exec calls and process limits

**Risk**: If the TUI fires multiple concurrent requests (e.g., loading agents + workflows + crons on startup), each spawns a separate `smithers` process. On constrained systems, this could exhaust file descriptors or process limits.

**Mitigation**: The TUI's Bubble Tea update loop is single-threaded for message processing, so concurrent exec calls only occur from `tea.Cmd` goroutines. In practice, views load sequentially (one Init per view push). If concurrency becomes an issue, add a semaphore in `execSmithers` to limit parallel exec calls (e.g., `chan struct{}` with capacity 4).

### 5. Mismatch: Crush's `execSmithers` hardcodes "smithers" binary name

**Risk**: The current `execSmithers` in `client.go:252` hardcodes `"smithers"` as the binary name. Users who install Smithers under a different name (e.g., `smithers-cli`) or in a non-PATH location cannot use the exec fallback.

**Mitigation**: Slice 1 introduces `WithBinaryPath` to make this configurable. The config file (`smithers-tui.json`) gains a `smithers.binaryPath` field. The default remains `"smithers"` for backward compatibility.

### 6. Working directory sensitivity

**Risk**: The `smithers` CLI discovers project context from the working directory (`.smithers/` directory). If the TUI is launched from a different directory than the project root, exec calls may return empty results or errors because the CLI can't find the project.

**Mitigation**: Slice 6 introduces `WithWorkingDir` to explicitly set `cmd.Dir`. The TUI should set this to the detected project root (the directory containing `.smithers/`). This matches how the HTTP server is started with a specific working directory.
