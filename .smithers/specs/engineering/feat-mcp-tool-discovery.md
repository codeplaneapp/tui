# Engineering Spec: Configure Smithers MCP Server Discovery

**Ticket**: feat-mcp-tool-discovery
**Feature**: MCP_TOOL_DISCOVERY_FROM_SMITHERS_SERVER
**Group**: MCP Integration (mcp-integration)
**Dependencies**: none

---

## Objective

Wire Crush's existing MCP client infrastructure to automatically discover, connect to, and expose the Smithers MCP server (`smithers --mcp`) on startup. After this work, the chat agent's tool palette includes all Smithers CLI tools (runs, observability, control, time-travel, workflows, agents, tickets, prompts, memory, scoring, cron, SQL) alongside the standard Crush built-in tools, with Smithers tools as the primary/prioritized tool set.

This is the foundational MCP integration ticket — every subsequent `MCP_*_TOOLS` ticket (runs, observability, control, etc.) depends on the discovery plumbing implemented here.

---

## Scope

### In scope

1. **Default MCP config**: Inject a `"smithers"` entry into the MCP config map during `setDefaults()` so that `smithers --mcp` is started automatically on every TUI launch via stdio transport.
2. **Default tool list adjustment**: Create `internal/config/defaults.go` with a modified default tool set that keeps essential built-in tools and disables tools irrelevant to Smithers context (e.g., `sourcegraph`).
3. **Coder agent MCP access**: Verify the default `"coder"` agent's `AllowedMCP` (`nil` = all allowed) correctly grants access to all Smithers tools. Verify the `"task"` agent's `AllowedMCP` (`{}` = none) correctly blocks MCP access.
4. **Startup connection flow**: Confirm that `mcp.Initialize()` in `app.go` picks up the injected `"smithers"` MCP config, spawns `smithers --mcp` via stdio, calls `ListTools()`, and publishes `EventStateChanged` + `EventToolsListChanged`.
5. **Graceful degradation**: If `smithers` binary is not found or `--mcp` fails, set state to `StateError` with a clear error message; do not block TUI startup.
6. **UI status verification**: Confirm the existing MCP status rendering in `internal/ui/chat/mcp.go` correctly displays `smithers: connected` or `smithers: error`.
7. **Tool naming**: Smithers MCP tools appear as `mcp_smithers_<tool>` (e.g., `mcp_smithers_ps`, `mcp_smithers_approve`, `mcp_smithers_workflow_run`). This follows the existing convention in `internal/agent/tools/mcp-tools.go:58-60`.

### Out of scope

- Implementing the `smithers --mcp` command itself (lives in the Smithers repo, TypeScript; uses the `incur` CLI framework's `cli.serve()` which auto-converts CLI commands into MCP tools over stdio).
- Custom Smithers tool renderers (covered by separate tickets per tool group).
- Smithers system prompt template (covered by `chat-domain-system-prompt`).
- HTTP API client `internal/smithers/client.go` (covered by `platform-http-api-client`).
- Per-tool-group feature tickets (`feat-mcp-runs-tools`, `feat-mcp-control-tools`, etc.).

---

## Implementation Plan

### Slice 1: Create `internal/config/defaults.go` with Smithers MCP default

**Files**: `internal/config/defaults.go` (new)

Create a new file that defines Smithers-specific configuration defaults, separate from Crush's generic defaults in `config.go` and `load.go`.

```go
package config

// SmithersMCPName is the config key for the Smithers MCP server.
const SmithersMCPName = "smithers"

// DefaultSmithersMCPConfig returns the default MCP configuration for
// the Smithers server. It uses stdio transport to spawn `smithers --mcp`.
func DefaultSmithersMCPConfig() MCPConfig {
    return MCPConfig{
        Type:    MCPStdio,
        Command: "smithers",
        Args:    []string{"--mcp"},
    }
}

// DefaultDisabledTools returns tools that are disabled by default in
// Smithers TUI context (not relevant for workflow operations).
func DefaultDisabledTools() []string {
    return []string{
        "sourcegraph",
    }
}

// IsSmithersCLIAvailable checks if the smithers binary is on PATH.
func IsSmithersCLIAvailable() bool {
    _, err := exec.LookPath("smithers")
    return err == nil
}
```

This follows the pattern established by `internal/config/docker_mcp.go` which defines `DockerMCPName` and `DockerMCPConfig()`.

**Why a separate file**: Keeps Smithers-specific defaults isolated from Crush's `config.go` and `load.go`, making upstream cherry-picks cleaner.

**Upstream Smithers note**: The actual Smithers MCP server is launched via `smithers --mcp` (detected in `src/cli/index.ts` line 2887-2895). The `incur` framework's `cli.serve(argv)` method auto-converts all registered CLI commands into MCP tools over stdio transport using `StdioServerTransport` from `@modelcontextprotocol/sdk`. Tool names are derived from command paths joined with underscores (e.g., `workflow run` → `workflow_run`, `ticket list` → `ticket_list`). Top-level commands keep their bare names (e.g., `ps`, `approve`, `inspect`).

---

### Slice 2: Inject Smithers MCP into `setDefaults()`

**Files**: `internal/config/load.go` (modify)

In `setDefaults()` (around line 395-397), after the MCP map is initialized, inject the Smithers default if no user-provided config overrides it:

```go
// In setDefaults(), after: if c.MCP == nil { c.MCP = make(map[string]MCPConfig) }

// Add default Smithers MCP server if not already configured by user.
if _, exists := c.MCP[SmithersMCPName]; !exists {
    c.MCP[SmithersMCPName] = DefaultSmithersMCPConfig()
}
```

**Key behavior**:
- If user's `smithers-tui.json` already has a `"smithers"` MCP entry (custom binary path, env vars, or `"disabled": true`), honor it — do not overwrite.
- If no user config exists, inject the default.
- This runs before `SetupAgents()`, so the Coder agent sees `"smithers"` in the MCP map.

**Config merge semantics**: Crush's config loader merges user config on top of defaults. Since we inject into `setDefaults()` which runs after user config is loaded, we check for existence before injecting.

---

### Slice 3: Apply default disabled tools

**Files**: `internal/config/load.go` (modify)

In `setDefaults()`, apply Smithers default disabled tools if the user hasn't explicitly configured a disabled tools list:

```go
// Apply Smithers default disabled tools if user hasn't set any.
if c.Options.DisabledTools == nil {
    c.Options.DisabledTools = DefaultDisabledTools()
}
```

This disables `sourcegraph` (not useful in Smithers context) by default while allowing user overrides.

---

### Slice 4: Verify MCP initialization flow in `app.go`

**Files**: `internal/app/app.go` (verify, likely no change)

The existing initialization at `app.go` line 113:

```go
go mcp.Initialize(ctx, app.Permissions, store)
```

Already iterates `store.Config().MCP`, which now includes the `"smithers"` entry. The data flow:

1. `mcp.Initialize()` finds `"smithers"` in config map (`internal/agent/tools/mcp/init.go:166-204`)
2. Spawns goroutine calling `initClient()` → `createSession()` → `createTransport()` with `MCPStdio` type
3. `createTransport()` builds `mcp.CommandTransport` with `command: "smithers"`, `args: ["--mcp"]` (`init.go:440-484`)
4. MCP session handshake establishes JSON-RPC connection over stdin/stdout pipes
5. `getTools()` calls `session.ListTools()` → discovers all Smithers CLI tools (`tools.go:133-142`)
6. `updateTools()` filters disabled tools and stores in `allTools["smithers"]` (`tools.go:144-152`)
7. State transitions: `StateStarting` → `StateConnected` (publishes `EventStateChanged`, `EventToolsListChanged`)

**No code change needed** — the existing error handling in `initClient()` (lines 234-270) already handles binary-not-found by catching the spawn failure and setting `StateError`.

**Optional enhancement**: Use `IsSmithersCLIAvailable()` from `defaults.go` to provide a friendlier error message in the UI ("Smithers CLI not found on PATH" vs. a generic connection error).

---

### Slice 5: Verify Coder agent MCP permissions

**Files**: `internal/config/config.go` (verify, no change expected)

In `SetupAgents()` (line 513-538):

- **Coder agent** (`AgentCoder`): `AllowedMCP` is `nil` → means ALL MCPs allowed. In `coordinator.buildTools()` (line 470-507), `nil` AllowedMCP passes all MCP tools through. All `mcp_smithers_*` tools are automatically available. **No change needed**.

- **Task agent** (`AgentTask`): `AllowedMCP` is `map[string][]string{}` (empty map = no MCPs). Task agents correctly cannot invoke Smithers mutation tools. **No change needed**.

The tool wrapping happens in `internal/agent/tools/mcp-tools.go:24-38` (`GetMCPTools()`), which iterates all MCP servers and creates `Tool` wrappers with names formatted as `mcp_{mcpName}_{toolName}` (line 58-60). Smithers tools will appear as: `mcp_smithers_ps`, `mcp_smithers_approve`, `mcp_smithers_workflow_run`, `mcp_smithers_ticket_list`, etc.

---

### Slice 6: Add unit tests for Smithers MCP default injection

**Files**: `internal/config/defaults_test.go` (new)

```go
func TestSmithersMCPDefaultInjected(t *testing.T) {
    cfg := &Config{}
    cfg.setDefaults(t.TempDir(), "")

    mcpCfg, exists := cfg.MCP[SmithersMCPName]
    require.True(t, exists, "smithers MCP should be injected by default")
    assert.Equal(t, MCPStdio, mcpCfg.Type)
    assert.Equal(t, "smithers", mcpCfg.Command)
    assert.Equal(t, []string{"--mcp"}, mcpCfg.Args)
    assert.False(t, mcpCfg.Disabled)
}

func TestSmithersMCPUserOverrideRespected(t *testing.T) {
    cfg := &Config{
        MCP: map[string]MCPConfig{
            SmithersMCPName: {
                Type:    MCPStdio,
                Command: "/custom/path/smithers",
                Args:    []string{"--mcp", "--verbose"},
            },
        },
    }
    cfg.setDefaults(t.TempDir(), "")

    mcpCfg := cfg.MCP[SmithersMCPName]
    assert.Equal(t, "/custom/path/smithers", mcpCfg.Command,
        "user-provided config should not be overwritten")
    assert.Equal(t, []string{"--mcp", "--verbose"}, mcpCfg.Args)
}

func TestSmithersMCPUserDisabledRespected(t *testing.T) {
    cfg := &Config{
        MCP: map[string]MCPConfig{
            SmithersMCPName: {
                Type:     MCPStdio,
                Command:  "smithers",
                Args:     []string{"--mcp"},
                Disabled: true,
            },
        },
    }
    cfg.setDefaults(t.TempDir(), "")

    mcpCfg := cfg.MCP[SmithersMCPName]
    assert.True(t, mcpCfg.Disabled,
        "user should be able to disable Smithers MCP")
}

func TestDefaultDisabledToolsApplied(t *testing.T) {
    cfg := &Config{}
    cfg.setDefaults(t.TempDir(), "")
    assert.Contains(t, cfg.Options.DisabledTools, "sourcegraph")
}
```

---

### Slice 7: Add MCP round-trip integration test

**Files**: `internal/config/mcp_smithers_integration_test.go` (new, build-tagged)

```go
//go:build integration

func TestSmithersMCPToolDiscovery(t *testing.T) {
    // 1. Create a mock MCP server binary (testdata/mock_mcp_server.go)
    //    that registers sample tools (ps, approve, workflow_run) and
    //    responds to ListTools over stdio JSON-RPC.
    // 2. Configure MCPConfig pointing to the mock binary.
    // 3. Call mcp.Initialize().
    // 4. Wait for initialization via mcp.WaitForInit().
    // 5. Verify tools are discovered:
    //    - mcp.Tools() yields entries for "smithers"
    //    - Tool names match mcp_smithers_ps, mcp_smithers_approve pattern
    // 6. Verify mcp.GetState("smithers").State == StateConnected.
    // 7. Call RunTool("ps", "{}") and verify a result is returned.
    // 8. Call mcp.Close() and verify cleanup.
}
```

**Mock server**: A small Go binary in `testdata/mock_mcp_server.go` that implements the MCP stdio protocol with hardcoded tool registrations. This avoids a CI dependency on the real Smithers CLI (TypeScript/Bun).

---

## Validation

### Unit tests

| Test | Command | Verifies |
|------|---------|----------|
| Default injection | `go test ./internal/config/ -run TestSmithersMCPDefaultInjected` | Smithers MCP config injected into defaults |
| User override | `go test ./internal/config/ -run TestSmithersMCPUserOverrideRespected` | User config not clobbered |
| Disabled respected | `go test ./internal/config/ -run TestSmithersMCPUserDisabledRespected` | `disabled: true` honored |
| Disabled tools | `go test ./internal/config/ -run TestDefaultDisabledToolsApplied` | `sourcegraph` disabled |

### Integration tests

| Test | Command | Verifies |
|------|---------|----------|
| MCP round-trip | `go test -tags integration ./internal/config/ -run TestSmithersMCPToolDiscovery` | Full discovery lifecycle with mock MCP server: spawn, handshake, ListTools, state transition |

### Terminal E2E tests (modeled on upstream `@microsoft/tui-test` harness)

The upstream Smithers repo (`../smithers/tests/tui.e2e.test.ts` + `../smithers/tests/tui-helpers.ts`) tests TUI behavior using a process-spawning terminal harness:
- `launchTUI(args)` spawns the TUI as a child process with piped I/O and `TERM=xterm-256color`
- `waitForText(text, timeout)` polls the stdout buffer for rendered content (ANSI-stripped, 100ms poll interval, 10s default timeout)
- `sendKeys(text)` writes raw keystrokes to stdin (`\r` for Enter, `\x1b` for Escape)
- `snapshot()` returns the current screen buffer for assertions

We adopt the same pattern for Crush's TUI via a Go test harness:

**File**: `tests/tui_mcp_discovery_e2e_test.go`

```
Test: "MCP status shows smithers connected on startup"
  1. Build the smithers-tui binary.
  2. Launch it pointing at a mock smithers --mcp server
     (set PATH or SMITHERS_MCP_COMMAND env to mock binary).
  3. Wait for the TUI to render the initial chat screen
     (waitForText("SMITHERS") or waitForText("Ready")).
  4. Assert that the MCP status area contains "smithers" and "connected".
  5. Assert that the status does NOT show "error" or "starting"
     after init completes (waitForNoText("starting")).

Test: "MCP status shows error when smithers binary missing"
  1. Launch TUI with PATH set to exclude smithers binary.
  2. Wait for initial render.
  3. Assert MCP status contains "smithers" and "error".
  4. Assert chat input is still interactive (TUI did not crash).

Test: "Agent can list discovered Smithers MCP tools"
  1. Launch TUI with mock MCP server registering ps, approve, workflow_run.
  2. Send chat input: "What tools do you have?"
  3. Assert agent response mentions mcp_smithers_ps or similar tool names.
```

The test harness wraps `exec.Command()` with the same spawn/poll/assert pattern as the upstream `BunSpawnBackend` in `tui-helpers.ts`. Key implementation details:
- Spawn with `stdin: pipe`, `stdout: pipe`, `stderr: pipe`
- Accumulate stdout in a buffer, strip ANSI escape sequences for assertions
- `waitForText` polls at 100ms intervals with configurable timeout
- On failure, dump buffer to `tui-buffer.txt` for debugging

### VHS happy-path recording test

**File**: `tests/vhs/mcp_discovery.tape`

```
# VHS tape: Smithers MCP Tool Discovery happy path
Output tests/vhs/mcp_discovery.gif
Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 600
Set TypingSpeed 50ms

# Launch Smithers TUI (with smithers on PATH)
Type "smithers-tui"
Enter
Sleep 3s

# Verify MCP status shows connected (visual check in recording)
Sleep 1s

# Type a query that triggers Smithers MCP tool use
Type "What runs are active?"
Enter
Sleep 5s

# Final screenshot for CI artifact
Screenshot tests/vhs/mcp_discovery_final.png
```

The VHS test produces a `.gif` recording and a final screenshot. CI validates that the recording completes without error (non-zero exit = TUI crash or MCP connection failure). The recording serves as visual documentation and regression detection.

### Manual verification

1. **With Smithers installed**: Run `smithers-tui` in a project with `smithers` on PATH. Confirm:
   - Header area shows `● smithers  connected` in the MCP status section.
   - Ask "What tools do you have?" — agent lists `mcp_smithers_*` tools.
   - Ask "List runs" — agent calls `mcp_smithers_ps` and renders result.

2. **Without Smithers installed**: Run `smithers-tui` without `smithers` on PATH. Confirm:
   - TUI starts normally (no crash, no blocking).
   - MCP status shows `● smithers  error` or similar.
   - Agent still works with built-in tools (bash, edit, view, etc.).
   - Chat is usable; no degradation beyond missing Smithers tools.

3. **User override**: Create `smithers-tui.json` with `"mcp": { "smithers": { "disabled": true } }`. Confirm:
   - No `smithers --mcp` process spawned.
   - MCP status does not show smithers at all (or shows disabled).

4. **Custom path**: Create `smithers-tui.json` with `"mcp": { "smithers": { "command": "/opt/smithers/bin/smithers", "args": ["--mcp"] } }`. Confirm:
   - TUI uses the custom binary path for MCP server.

---

## Risks

### 1. Smithers MCP invocation flag mismatch

**Risk**: The upstream Smithers repo uses `--mcp` as the flag to start the MCP server (`src/cli/index.ts` line 2887: `argv.includes("--mcp")`), while the PRD and engineering docs reference `mcp-serve` as a subcommand. The actual flag is `--mcp`, not `smithers mcp-serve`.

**Impact**: If the default config uses `args: ["mcp-serve"]` instead of `args: ["--mcp"]`, the MCP server will fail to start with a "command not found" error.

**Mitigation**: Use `args: ["--mcp"]` in `DefaultSmithersMCPConfig()` to match the actual Smithers implementation. The `incur` framework detects `--mcp` in argv and calls `cli.serve()` which bootstraps the MCP server. Verify against the current Smithers CLI before merging.

### 2. Tool naming divergence between docs and implementation

**Risk**: The engineering doc (`03-ENGINEERING.md` section 3.0.2) uses `smithers_ps`, `smithers_approve` etc. as MCP tool names. The actual Smithers MCP server (via `incur`) generates names by joining command path segments with underscores: `ps`, `approve`, `workflow_run`, `ticket_list`. After Crush's MCP wrapper prefixes them, they become `mcp_smithers_ps`, `mcp_smithers_approve`, `mcp_smithers_workflow_run`.

**Impact**: System prompt references to tool names must match the actual `mcp_smithers_*` naming, not the doc's `smithers_*` naming.

**Mitigation**: This ticket focuses on discovery plumbing, not tool naming in prompts. The system prompt ticket (`chat-domain-system-prompt`) must verify actual tool names after discovery. Add a debug log or test that prints discovered tool names.

### 3. Startup latency from MCP initialization

**Risk**: Spawning `smithers --mcp` adds a subprocess and MCP handshake to TUI startup. The Smithers CLI runs on Bun/Node.js — cold start could add 500ms–2s before tools are available.

**Mitigation**: `mcp.Initialize()` already runs in a background goroutine (`app.go` line 113). The TUI is interactive immediately; MCP tools become available asynchronously. The MCP status indicator transitions from "starting" → "connected", giving the user feedback. Default timeout is 15 seconds (configurable via `timeout` field on `MCPConfig`).

### 4. Tool count explosion

**Risk**: The Smithers MCP server registers 40+ tools (all CLI commands). Combined with Crush's 20+ built-in tools, the agent's tool palette exceeds 60 tools, potentially degrading LLM tool selection accuracy.

**Mitigation**: The `MCPConfig.DisabledTools` field allows pruning unused tools. The `Agent.AllowedMCP` map supports restricting which Smithers tools a specific agent can use (e.g., `"smithers": ["ps", "inspect", "approve"]`). This is a concern for later per-tool-group tickets, but the infrastructure supports it from day one. The system prompt (separate ticket) can guide the LLM on when to use which tools.

### 5. Binary path differences across environments

**Risk**: The default config assumes `smithers` is on PATH. In some environments (npm local install, nvm, pnpm global, custom prefixes), the binary may not be directly accessible.

**Mitigation**: Users can override the command path in `smithers-tui.json`:
```json
{ "mcp": { "smithers": { "command": "/path/to/smithers", "args": ["--mcp"] } } }
```
The `MCPConfig.Command` field supports absolute paths. The `MCPConfig.Env` field can set `PATH` or other env vars for the subprocess. Future enhancement: resolve `SMITHERS_PATH` env var in `DefaultSmithersMCPConfig()`.

### 6. `internal/config/defaults.go` does not exist yet

**Risk**: The ticket references `internal/config/defaults.go` in its source context, but this file does not exist in the current Crush codebase. Defaults are currently applied inline in `load.go:setDefaults()` and `config.go:SetupAgents()`.

**Mitigation**: Create the file as Slice 1 of this ticket. This is a clean addition — no existing code moves. It follows the established pattern of `docker_mcp.go` which similarly defines a named MCP default with its own file.

### 7. Crush upstream divergence at `setDefaults()`

**Risk**: Modifications to `load.go:setDefaults()` may conflict with upstream Crush changes if we cherry-pick updates later.

**Mitigation**: The injection point is a 3-line addition after the MCP map initialization. The new `defaults.go` file has no upstream counterpart, so no merge conflicts there. The `docker_mcp.go` pattern demonstrates this approach works cleanly for additive MCP defaults.