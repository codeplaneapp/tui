## Goal
Wire the Smithers specialized agent end-to-end so that the default chat session uses the Smithers system prompt, loads workspace context, restricts tools to the Smithers MCP server, picks the configured large model, and is verified by unit tests, a coordinator-level integration test, and an updated E2E/VHS harness.

## Current State

The infrastructure for the Smithers specialized agent already exists in production code. Before starting implementation, understand what is shipped and what is missing:

### What Is Already Shipped

- `internal/config/defaults.go` — `SmithersMCPName = "smithers"`, `DefaultSmithersMCPConfig()` (stdio, `smithers --mcp`), `DefaultDisabledTools()` (`["sourcegraph"]`).
- `internal/config/load.go` — `setDefaults` auto-injects the Smithers MCP entry when absent and applies default disabled tools. `SetupAgents()` is called after provider configuration.
- `internal/config/config.go` — `SetupAgents()` builds `AgentSmithers` when `cfg.Smithers != nil`; the agent sets `AllowedMCP: map[string][]string{"smithers": nil}` (all tools from that server) and filters out `sourcegraph` and `multiedit`.
- `internal/agent/coordinator.go` — `resolveAgent` prefers `AgentSmithers` when present. `NewCoordinator` calls `smithersPrompt` when the Smithers agent is selected, passing `workflowDir` from `cfg.Config().Smithers.WorkflowDir` and hardcoding MCP server name `"smithers"`. `buildTools` filters by `agent.AllowedTools` then `agent.AllowedMCP`.
- `internal/agent/prompts.go` — `smithersPrompt` builds a prompt using the embedded `smithers.md.tpl`.
- `internal/agent/templates/smithers.md.tpl` — Full Smithers system prompt with role, `<smithers_tools>`, `<behavior>`, `<workspace>`, `<env>`, `<memory>` sections. MCP server name is injected via `{{.SmithersMCPServer}}`.
- `internal/agent/prompt/prompt.go` — `WithSmithersMode(workflowDir, mcpServer)` option; `PromptDat` carries `SmithersMode`, `SmithersWorkflowDir`, `SmithersMCPServer`. Context files loaded from `cfg.Options.ContextPaths` and skills from `cfg.Options.SkillsPaths`.
- `internal/agent/prompts_test.go` — Unit tests for template rendering including a golden file snapshot.
- `internal/config/defaults_test.go` — Covers default MCP injection, user override, and disabled-tool logic.
- `internal/agent/tools/mcp/smithers_discovery_test.go` — MCP state transition and discovery flow tests.
- `internal/e2e/tui_helpers_test.go` — `launchTUI` harness with `WaitForText`, `WaitForNoText`, `SendKeys`, `Snapshot`, `Terminate`.
- `internal/e2e/chat_domain_system_prompt_test.go` — Two E2E tests: one with Smithers config present, one without. Both only assert `"SMITHERS"` header visible; neither asserts agent mode or tool availability.
- `tests/vhs/smithers-domain-system-prompt.tape` — VHS recording tape that **still uses** `CRUSH_GLOBAL_CONFIG` / `CRUSH_GLOBAL_DATA` env vars instead of `SMITHERS_TUI_GLOBAL_CONFIG` / `SMITHERS_TUI_GLOBAL_DATA`.

### Known Gaps

1. **Coordinator hardcodes the MCP server name** as the string literal `"smithers"` instead of reading `SmithersMCPName` from the config package. When the user configures a differently-named MCP entry, the template injects the wrong tool names.
2. **`smithers.NewClient()` is constructed without Smithers config** in `internal/ui/model/ui.go` (line 342). The client receives no `apiURL`, `apiToken`, or `dbPath` from the loaded config, so HTTP API fallback and SQLite fallback are always inert.
3. **`AgentSmithers` only exists when `cfg.Smithers != nil`**. When no `"smithers"` block is present in any config file, `SetupAgents` skips the Smithers agent and `resolveAgent` falls back to `AgentCoder`. This is correct behavior, but there is no test asserting the fallback path populates the correct system prompt.
4. **E2E tests are thin**: both `TestSmithersDomainSystemPrompt_TUI` and `TestSmithersDomainSystemPrompt_CoderFallback_TUI` only wait for the SMITHERS header, not for agent-mode text, MCP status, or tool availability.
5. **VHS tape uses wrong env var names**: `CRUSH_GLOBAL_CONFIG` / `CRUSH_GLOBAL_DATA` should be `SMITHERS_TUI_GLOBAL_CONFIG` / `SMITHERS_TUI_GLOBAL_DATA`.
6. **No test for the coordinator's `smithersPrompt` dispatch path** — there is no test verifying that when `AgentSmithers` is resolved, `smithersPrompt` is called and `buildTools` produces only the expected tools.

## Steps

### Step 1 — Fix coordinator MCP server name injection

The coordinator at `internal/agent/coordinator.go:129` hardcodes `"smithers"` as the MCP server name passed to `WithSmithersMode`. Change it to read `config.SmithersMCPName` so the template always reflects the actual configured key.

**Before:**
```go
systemPrompt, err = smithersPrompt(append(promptOpts, prompt.WithSmithersMode(workflowDir, "smithers"))...)
```

**After:**
```go
systemPrompt, err = smithersPrompt(append(promptOpts, prompt.WithSmithersMode(workflowDir, config.SmithersMCPName))...)
```

This is a one-line change with no behaviour impact when `SmithersMCPName == "smithers"` (the current default), but makes the code correct by construction.

### Step 2 — Wire Smithers config into the client constructor in ui.go

`internal/ui/model/ui.go` constructs `smithers.NewClient()` with no options. The `NewUI` function already receives `cfg *config.ConfigStore` via `app.App`. Pass the Smithers config fields into the client constructor so HTTP API and DB fallback work when configured.

Locate the `NewUI` function signature and add a config-driven client construction:

```go
// build Smithers client options from loaded config
var clientOpts []smithers.ClientOption
if sc := cfg.Config().Smithers; sc != nil {
    if sc.APIURL != "" {
        clientOpts = append(clientOpts, smithers.WithAPIURL(sc.APIURL))
    }
    if sc.APIToken != "" {
        clientOpts = append(clientOpts, smithers.WithAPIToken(sc.APIToken))
    }
    if sc.DBPath != "" {
        clientOpts = append(clientOpts, smithers.WithDBPath(sc.DBPath))
    }
}
// ...
smithersClient: smithers.NewClient(clientOpts...),
```

The `smithers.NewClient` already accepts `...ClientOption` — no changes to the client package are needed.

### Step 3 — Add a coordinator-level unit test for agent dispatch

Add `TestCoordinatorSmithersAgentDispatch` in `internal/agent/coordinator_test.go` (or a new file `internal/agent/smithers_agent_test.go`). This test:

1. Builds a minimal `config.ConfigStore` with a `Smithers` block and a mock provider.
2. Calls `resolveAgent` directly and asserts `AgentSmithers` is returned.
3. Verifies that the resulting agent's `AllowedMCP` map contains `"smithers"` and that `AllowedTools` does not contain `"sourcegraph"` or `"multiedit"`.
4. Builds the system prompt string and asserts it contains `"Smithers TUI assistant"` and the correct `mcp_smithers_` tool names.

Use `config.Init` (already used in `prompts_test.go`) with a temp dir that contains a minimal `smithers-tui.json` with a `"smithers": {}` block to trigger `AgentSmithers` creation.

### Step 4 — Add a fallback path test

Add `TestCoordinatorCoderFallbackWhenNoSmithersConfig` asserting that when no `"smithers"` block is present, `resolveAgent` returns `AgentCoder` and the system prompt does not contain `"Smithers TUI assistant"`.

### Step 5 — Strengthen E2E tests

Extend `internal/e2e/chat_domain_system_prompt_test.go`:

**`TestSmithersDomainSystemPrompt_TUI`** — after `WaitForText("SMITHERS", ...)`, add:
```go
// The Smithers agent mode label is shown in the header status area
require.NoError(t, tui.WaitForText("Smithers Agent Mode", 10*time.Second))
// The smithers MCP entry appears in the MCP status area
require.NoError(t, tui.WaitForText("smithers", 5*time.Second))
```

**`TestSmithersDomainSystemPrompt_CoderFallback_TUI`** — rename to `TestCoderAgentFallback_TUI` and assert:
```go
// Without smithers config the TUI still loads
require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
// But Smithers Agent Mode label should NOT appear
require.NoError(t, tui.WaitForNoText("Smithers Agent Mode", 3*time.Second))
```

Both tests remain gated on `SMITHERS_TUI_E2E=1`.

### Step 6 — Fix the VHS tape env vars

`tests/vhs/smithers-domain-system-prompt.tape` uses `CRUSH_GLOBAL_CONFIG` and `CRUSH_GLOBAL_DATA` which no longer exist. Update the tape to use the correct env var names and point to the correct fixtures directory:

**Before:**
```
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
```

**After:**
```
Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=/tmp/smithers-tui-vhs go run ."
```

Also verify that `tests/vhs/fixtures/smithers-tui.json` exists and contains a valid provider and `"smithers": {}` block. If it does not exist, create it with enough config to get past the onboarding screen (see the existing fixture pattern in `tests/vhs/` for reference).

### Step 7 — Add workspace context injection test

Add `TestSmithersPromptContextFilesLoaded` in `internal/agent/prompts_test.go` (or `prompt/prompt_test.go`) to verify that when context files exist in the working directory (e.g., `smithers-tui.md`, `AGENTS.md`), they are included in the rendered prompt under the `<memory>` section. Use a temp dir with a pre-written file and confirm `<file path="...">` appears in the rendered output.

## File Plan

### Modify

- [`internal/agent/coordinator.go`](/Users/williamcory/crush/internal/agent/coordinator.go)
  - Line 129: Replace `"smithers"` literal with `config.SmithersMCPName`.

- [`internal/ui/model/ui.go`](/Users/williamcory/crush/internal/ui/model/ui.go)
  - Near line 342: Wire `cfg.Config().Smithers` fields into `smithers.NewClient(clientOpts...)` instead of zero-arg construction.

- [`internal/e2e/chat_domain_system_prompt_test.go`](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go)
  - Strengthen `TestSmithersDomainSystemPrompt_TUI` to assert agent mode label and MCP name.
  - Rename/strengthen `TestSmithersDomainSystemPrompt_CoderFallback_TUI` to assert absence of Smithers agent mode.

- [`tests/vhs/smithers-domain-system-prompt.tape`](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape)
  - Replace `CRUSH_GLOBAL_CONFIG` / `CRUSH_GLOBAL_DATA` with `SMITHERS_TUI_GLOBAL_CONFIG` / `SMITHERS_TUI_GLOBAL_DATA`.

### New

- [`internal/agent/smithers_agent_test.go`](/Users/williamcory/crush/internal/agent/smithers_agent_test.go)
  - `TestCoordinatorSmithersAgentDispatch` — resolves Smithers agent, checks tool filtering, checks system prompt contains domain instructions.
  - `TestCoordinatorCoderFallbackWhenNoSmithersConfig` — verifies coder prompt used when no `"smithers"` config block.

- [`internal/agent/prompts_test.go`](/Users/williamcory/crush/internal/agent/prompts_test.go) _(extend existing file)_
  - `TestSmithersPromptContextFilesLoaded` — verifies workspace context files appear in `<memory>` section.

- [`tests/vhs/fixtures/smithers-tui.json`](/Users/williamcory/crush/tests/vhs/fixtures/smithers-tui.json) _(create if absent)_
  - Minimal config with a valid provider entry and `"smithers": {}` block.

## Validation

```bash
# 1. Format
gofumpt -w internal/agent internal/ui/model internal/e2e

# 2. Coordinator and prompt unit tests
go test ./internal/agent/... -count=1 -run 'TestCoordinator|TestSmithers' -v

# 3. Config defaults tests (ensure no regressions)
go test ./internal/config/... -count=1 -v

# 4. MCP discovery tests
go test ./internal/agent/tools/mcp/... -count=1 -v

# 5. Full test suite
go test ./... -count=1

# 6. E2E tests (requires a valid provider API key in env)
SMITHERS_TUI_E2E=1 go test ./internal/e2e/... -count=1 -v -timeout 60s

# 7. VHS smoke recording
vhs tests/vhs/smithers-domain-system-prompt.tape

# 8. Manual smoke check — launch with Smithers config and confirm:
#    - Header shows "Smithers Agent Mode"
#    - MCP status shows "● smithers  connected"
#    - Agent responds to "What workflows are available?" using mcp_smithers_workflow_list
SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures go run .
```

## Open Questions

1. **`AgentSmithers` without a running MCP server**: When `smithers` is on `$PATH` but `smithers --mcp` is not yet implemented (pre-P0), the MCP entry connects, fails, and the agent falls back to bash tools. Should the system prompt's `<smithers_tools>` section be conditionally rendered based on whether the MCP server is in `StateConnected`? Current behavior (always render tool names) is acceptable for P0 since the prompt still describes the bash fallback.

2. **MCP server name flexibility**: `SmithersMCPName = "smithers"` is a constant but the `AllowedMCP` in `SetupAgents` also hard-references it as the string `"smithers"`. If a user configures the MCP server under a different key (e.g., `"smithers-dev"`), the Smithers agent will not allow it. Should `SetupAgents` be parameterized on the actual key found in `cfg.MCP`? Deferring this to a follow-on config-namespace ticket is acceptable.

3. **`smithers NewClient` in ui.go at init vs. app startup**: `NewUI` is called before the config is fully available in some test paths. Confirm that `cfg.Config().Smithers` is always non-nil by the time `NewUI` is called in production, or add a nil guard.

4. **Golden file update**: After fixing the `SmithersMCPName` constant reference (a no-op change in current defaults), the golden file `internal/agent/testdata/smithers_prompt.golden` will not change. If the MCP server name is ever changed, update the golden via `SMITHERS_TUI_UPDATE_GOLDEN=1 go test ./internal/agent/... -run TestSmithersPromptSnapshot`.
