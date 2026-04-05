# Implementation: feat-mcp-tool-discovery

**Ticket**: feat-mcp-tool-discovery
**Feature**: MCP_TOOL_DISCOVERY_FROM_SMITHERS_SERVER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

Implemented Smithers MCP tool discovery in Crush TUI by:
1. Creating `internal/config/defaults.go` with Smithers MCP default configuration
2. Injecting Smithers MCP into application startup via `Config.setDefaults()`
3. Applying default disabled tools (sourcegraph) with user override support
4. Adding comprehensive unit and integration tests
5. Creating VHS happy-path recording for E2E validation

The implementation follows the existing Crush/Docker MCP pattern and ensures graceful degradation when the Smithers CLI is unavailable.

---

## Files Changed

### New Files
- **internal/config/defaults.go** (30 lines)
  - `SmithersMCPName` constant
  - `DefaultSmithersMCPConfig()` returns stdio transport config for `smithers --mcp`
  - `DefaultDisabledTools()` returns list of tools disabled by default (sourcegraph)
  - `IsSmithersCLIAvailable()` checks if smithers binary is on PATH

- **internal/config/defaults_test.go** (133 lines)
  - `TestSmithersMCPDefaultInjected`: verifies default config injection
  - `TestSmithersMCPUserOverrideRespected`: confirms user config not overwritten
  - `TestSmithersMCPUserDisabledRespected`: validates disabled flag honored
  - `TestDefaultDisabledToolsApplied`: verifies default tool disabling
  - `TestDefaultDisabledToolsUserOverrideRespected`: user disabled tools not overwritten
  - `TestDefaultSmithersMCPConfig`: validates default config structure
  - `TestDefaultDisabledTools`: validates default disabled tools list
  - `TestSmithersMCPFullWorkflowWithUserConfig`: full workflow integration test

- **internal/agent/tools/mcp/smithers_discovery_test.go** (109 lines)
  - `TestSmithersMCPDiscoveryFlow`: config setup verification
  - `TestSmithersMCPDefaultInjectedIntoConfig`: agent MCP permissions validation
  - `TestSmithersMCPToolDiscoveryWithMockServer`: in-memory transport E2E test
  - `TestSmithersMCPStateTransitions`: state machine validation

- **tests/vhs/mcp-tool-discovery.tape** (28 lines)
  - Happy-path VHS recording for MCP discovery
  - Verifies TUI startup with MCP connection
  - Demonstrates agent access to discovered tools
  - Shows graceful degradation when Smithers unavailable

### Modified Files
- **internal/config/load.go** (11 lines added)
  - Inject Smithers MCP config in `setDefaults()` after MCP map initialization
  - Apply default disabled tools list when user hasn't configured any
  - Preserves user overrides for both MCP config and disabled tools

- **tests/vhs/README.md** (16 lines added)
  - Documentation for running MCP tool discovery tape
  - Instructions and expected behavior

---

## Implementation Details

### Smithers MCP Default Configuration

The default configuration uses stdio transport to spawn `smithers --mcp`:

```go
MCPConfig{
    Type:    MCPStdio,
    Command: "smithers",
    Args:    []string{"--mcp"},
}
```

This matches the actual Smithers CLI implementation (`src/cli/index.ts` line 2887).

### Configuration Injection Flow

1. **Application startup**: `Load()` → `setDefaults()`
2. **MCP injection**: If user hasn't configured `mcp.smithers`, inject default
3. **Tool defaults**: If user hasn't set `options.disabled_tools`, apply defaults
4. **Agent setup**: `SetupAgents()` configures Smithers agent with `AllowedMCP: {"smithers": nil}` (all tools allowed)
5. **MCP initialization**: `app.New()` → `mcp.Initialize()` spawns `smithers --mcp` and discovers tools
6. **State transitions**: MCP client transitions through Starting → Connected (or Error)
7. **Tool registration**: Discovered tools prefixed as `mcp_smithers_<tool>` (e.g., `mcp_smithers_ps`, `mcp_smithers_approve`)

### User Configuration Preservation

All injections check for existing user config before applying defaults:

**Smithers MCP override example**:
```json
{
  "mcp": {
    "smithers": {
      "command": "/opt/smithers/bin/smithers",
      "args": ["--mcp", "--verbose"]
    }
  }
}
```

**Disable Smithers MCP example**:
```json
{
  "mcp": {
    "smithers": {
      "disabled": true
    }
  }
}
```

**Custom disabled tools example**:
```json
{
  "options": {
    "disabled_tools": ["bash", "sourcegraph"]
  }
}
```

### Graceful Degradation

- **Smithers CLI unavailable**: MCP state set to `StateError`, but TUI remains interactive
- **MCP handshake timeout**: Configurable timeout (default 15s), error logged but non-blocking
- **Tool discovery failure**: MCP state updates, agent continues with available built-in tools
- **Smithers agent not configured**: Falls back to Coder agent with all non-restricted tools

---

## Tests Added

### Unit Tests (8 tests, `internal/config/defaults_test.go`)
- ✅ Default Smithers MCP injected
- ✅ User override respected
- ✅ Disabled flag respected
- ✅ Default disabled tools applied
- ✅ User disabled tools override respected
- ✅ Default MCP config structure
- ✅ Default disabled tools list
- ✅ Full workflow integration

### Integration Tests (4 tests, `internal/agent/tools/mcp/smithers_discovery_test.go`)
- ✅ MCP discovery flow with mock config
- ✅ Default injection into agent config
- ✅ Tool discovery via in-memory transport
- ✅ MCP state transitions

### E2E Tests (1 tape, `tests/vhs/mcp-tool-discovery.tape`)
- ✅ VHS happy-path recording verifying startup and MCP status
- ✅ Documents expected behavior when Smithers CLI available/unavailable

### Regression Tests
- ✅ All existing config tests pass
- ✅ All existing agent tests pass (TestCoordinatorResolveAgent, TestSmithersPrompt*)
- ✅ All existing MCP tests pass
- ✅ Smithers agent MCP permissions verified in agent_id_test.go and load_test.go

---

## Validation Results

### Configuration Tests
```
TestSmithersMCPDefaultInjected ✅
TestSmithersMCPUserOverrideRespected ✅
TestSmithersMCPUserDisabledRespected ✅
TestDefaultDisabledToolsApplied ✅
TestDefaultDisabledToolsUserOverrideRespected ✅
TestDefaultSmithersMCPConfig ✅
TestDefaultDisabledTools ✅
TestSmithersMCPFullWorkflowWithUserConfig ✅
```

### MCP Integration Tests
```
TestSmithersMCPDiscoveryFlow ✅
TestSmithersMCPDefaultInjectedIntoConfig ✅
TestSmithersMCPToolDiscoveryWithMockServer ✅
TestSmithersMCPStateTransitions ✅
```

### Agent Tests (Regression)
```
TestCoordinatorResolveAgent ✅
  - prefers_smithers_agent_when_configured ✅
  - falls_back_to_coder_agent_when_smithers_is_not_configured ✅
  - returns_an_error_when_no_supported_agent_exists ✅
TestSmithersPromptIncludesDomainInstructions ✅
TestSmithersPromptOmitsWorkspaceWithoutWorkflowDir ✅
TestSmithersPromptSnapshot ✅
```

---

## Architecture

### Configuration Hierarchy
1. **User config** (smithers-tui.json): highest priority
2. **Workspace config** (.smithers-tui/smithers-tui.json): merged on top
3. **Defaults** (injected in setDefaults()): fallback
4. **Embedded defaults**: lowest priority

### MCP Initialization Flow
```
TUI Start
  ↓
Config.Load() → setDefaults()
  ├─ Inject Smithers MCP if not user-configured
  ├─ Apply default disabled tools if not user-configured
  ↓
app.New()
  ├─ SetupAgents() → Smithers agent configured with AllowedMCP
  ├─ mcp.Initialize(ctx, ...) [goroutine]
  │  ├─ Iterate cfg.MCP map (now includes "smithers")
  │  ├─ Spawn "smithers --mcp" via stdio transport
  │  ├─ JSON-RPC handshake: capabilities exchange
  │  ├─ Call session.ListTools()
  │  ├─ Filter disabled tools
  │  ├─ State transition: Starting → Connected (or Error)
  │  └─ Publish EventToolsListChanged
  ↓
TUI Interactive
  ├─ MCP tools available to Smithers agent
  ├─ Tool names prefixed: mcp_smithers_ps, mcp_smithers_approve, etc.
  └─ Agent system prompt references available tools
```

### Tool Naming Convention
- **Smithers MCP server tools**: `ps`, `approve`, `workflow_run`, `ticket_list`, etc.
- **Crush wrapper**: Prefixes with `mcp_<server>_` → `mcp_smithers_ps`, `mcp_smithers_approve`, etc.
- **Convention**: Follows existing pattern in `internal/agent/tools/mcp-tools.go:58-60`

---

## Known Limitations & Future Work

### Out of Scope (per engineering spec)
- Implementing `smithers --mcp` command (TypeScript/Bun, lives in Smithers repo)
- Custom Smithers tool renderers (separate per-tool-group tickets)
- Smithers system prompt customization (separate ticket: chat-domain-system-prompt)
- HTTP API client (separate ticket: platform-http-api-client)
- Per-tool-group feature tickets (feat-mcp-runs-tools, feat-mcp-control-tools, etc.)

### Potential Enhancements
1. **Environment variable resolution**: Allow `SMITHERS_PATH` env var override
2. **Health check**: Periodic ping of Smithers MCP to detect disconnection
3. **Auto-reconnect**: Attempt to reconnect if Smithers becomes available later
4. **Tool prioritization**: Reorder LLM tool selection to prefer Smithers tools for domain operations
5. **Per-agent tool restrictions**: Allow restricting which Smithers tools specific agents can access
6. **Tool usage telemetry**: Track which MCP tools are most frequently called

---

## Manual Verification

### With Smithers CLI Available
```bash
cd /path/to/smithers-tui-project
smithers-tui
# In TUI, check status bar for "smithers connected"
# Ask: "What tools do you have?" → should list mcp_smithers_* tools
```

### Without Smithers CLI
```bash
PATH=/usr/bin:/bin smithers-tui
# TUI starts normally
# Status bar shows "smithers error" or similar
# Chat still works with built-in tools (bash, edit, view, etc.)
```

### Custom Configuration
```json
{
  "mcp": {
    "smithers": {
      "command": "/custom/path/smithers",
      "args": ["--mcp", "--verbose"]
    }
  }
}
```
Then run `smithers-tui` and verify custom binary is used.

---

## Commits

1. **feat(mcp): add Smithers MCP default configuration and injection** (7537e171)
   - Creates defaults.go with config helpers
   - Injects Smithers MCP into setDefaults()
   - Applies default disabled tools

2. **test(mcp): add Smithers MCP discovery integration tests** (2fbd61a5)
   - Config flow tests
   - Mock server discovery tests
   - State machine tests

3. **test(e2e): add MCP tool discovery VHS happy-path recording** (8334c493)
   - VHS tape for happy-path validation
   - README documentation

---

## Conclusion

The Smithers MCP tool discovery feature is now fully implemented and tested. The TUI automatically discovers and exposes all Smithers CLI tools to the agent on startup, with graceful degradation when the Smithers CLI is unavailable. The implementation follows established Crush patterns and respects user configuration overrides at all levels.

This completes the foundational MCP integration work. Subsequent tickets can build on this infrastructure to add tool-group-specific features (runs, observability, control, time-travel, etc.) without needing to modify the core discovery mechanism.

All tests pass. The feature is production-ready.
