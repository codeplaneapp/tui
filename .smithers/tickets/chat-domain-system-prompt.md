# Smithers Domain System Prompt

## Metadata
- ID: chat-domain-system-prompt
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_DOMAIN_SYSTEM_PROMPT
- Dependencies: none

## Summary

Create and configure the Smithers-specific system prompt to instruct the agent on workflow management and Smithers TUI operations.

## Acceptance Criteria

- The agent is initialized with the Smithers system prompt instead of the default coding prompt.
- The prompt includes instructions on formatting runs, mentioning pending approvals, and using Smithers MCP tools.

## Source Context

- internal/agent/templates/smithers.md.tpl
- internal/agent/agent.go

## Implementation Notes

- Create `internal/agent/templates/smithers.md.tpl` with the content outlined in the Engineering document.
- Update `internal/agent/agent.go` or the configuration layer to load this template for the primary agent session.

---

## Objective

Replace Crush's default coding-assistant system prompt (`coder.md.tpl`) with a Smithers-domain system prompt so that the TUI's chat agent understands Smithers workflows, MCP tools, run formatting, approval gates, and orchestrator operations out of the box. This is the foundational prompt change that every other chat feature (workspace context, active run summary, custom tool renderers) builds on.

The upstream Smithers reference is `../smithers/src/cli/ask.ts` (`SYSTEM_PROMPT` constant), which defines the domain prompt injected into agents via `BaseCliAgent.systemPrompt`. Crush's target is to deliver the same domain knowledge through its Go template pipeline (`internal/agent/prompt/prompt.go` → `PromptDat` → `text/template`).

### Key Crush code paths touched

| File | Current state | Change |
|------|--------------|--------|
| `internal/agent/prompt/prompt.go:21-27` | `Prompt` struct with `name`, `template`, `now`, `platform`, `workingDir` | Add `smithersMode bool`, `smithersWorkflowDir string`, `smithersMCPServer string` |
| `internal/agent/prompt/prompt.go:29-40` | `PromptDat` struct with 10 fields (Provider…AvailSkillXML) | Add `SmithersMode bool`, `SmithersWorkflowDir string`, `SmithersMCPServer string` |
| `internal/agent/prompt/prompt.go:47-65` | Three `Option` funcs (WithTimeFunc, WithPlatform, WithWorkingDir) | Add `WithSmithersMode` option |
| `internal/agent/prompt/prompt.go:151-203` | `promptData()` builds `PromptDat` from config | Populate Smithers fields from `Prompt` struct |
| `internal/agent/prompts.go:11-26` | `coderPromptTmpl` embed + `coderPrompt()` factory | Add `smithersPromptTmpl` embed + `smithersPrompt()` |
| `internal/agent/coordinator.go:115-132` | Hardcoded `AgentCoder` lookup + `coderPrompt()` | `resolveAgent()` dispatch: Smithers → `smithersPrompt()`, else → `coderPrompt()` |
| `internal/config/config.go:61-64` | Constants `AgentCoder`, `AgentTask` | Add `AgentSmithers` |
| `internal/config/config.go:373-396` | `Config` struct (no Smithers field) | Add `Smithers *SmithersConfig` field |
| `internal/config/config.go:513-538` | `SetupAgents()` creates coder + task agents | Conditionally add smithers agent |
| `internal/agent/templates/` | `coder.md.tpl`, `task.md.tpl`, `initialize.md.tpl` | Add `smithers.md.tpl` |

## Scope

### In scope

1. **New template file** `internal/agent/templates/smithers.md.tpl` — a Go `text/template` that produces the Smithers domain system prompt.
2. **New prompt constructor** `smithersPrompt()` in `internal/agent/prompts.go` — mirrors the existing `coderPrompt()` (line 20-26) but loads the Smithers template.
3. **Coordinator wiring** — `internal/agent/coordinator.go:NewCoordinator()` (line 115-132) selects `smithersPrompt()` instead of `coderPrompt()` when running in Smithers mode.
4. **Config gate** — a new agent constant `config.AgentSmithers` (alongside `AgentCoder`/`AgentTask` at line 61-64) plus a `SmithersConfig` struct on `Config` that controls which prompt is loaded.
5. **Agent registration** — extend `SetupAgents()` (line 513-538) to conditionally create the smithers agent when `Config.Smithers` is non-nil.
6. **Template data extensions** — add optional Smithers-specific fields to both the `Prompt` struct (line 21-27) and `PromptDat` struct (line 29-40) so the template can conditionally emit Smithers-specific blocks.
7. **Unit tests** for template rendering, prompt selection, and agent resolution.

### Out of scope

- Dynamic context injection (active runs, pending approvals) — ticket `chat-workspace-context` / `chat-active-run-summary`.
- Custom tool renderers for Smithers MCP results — ticket `chat-custom-tool-renderers`.
- MCP server configuration or transport wiring — ticket `platform-mcp-transport`.
- Branding changes (logo, colors, config paths) — ticket `platform-smithers-rebrand`.

## Implementation Plan

### Slice 1: Create the Smithers system prompt template

**Goal**: A new Go template file that produces the Smithers domain system prompt.

**File**: `internal/agent/templates/smithers.md.tpl`

**Content structure** (derived from upstream `ask.ts` SYSTEM_PROMPT and 03-ENGINEERING.md §3.0.3):

```
You are the Smithers TUI assistant, a specialized agent for managing
Smithers AI workflows from within a terminal interface.

<role>
You help users monitor, control, and debug Smithers workflow runs.
You are embedded inside the Smithers orchestrator control plane TUI.
</role>

<smithers_tools>
You have access to Smithers via MCP tools:
- smithers_ps / smithers_runs_list: List active, paused, completed, and failed runs
- smithers_inspect: Detailed run state, node outputs, DAG structure
- smithers_chat: View agent conversations for a run
- smithers_logs: Event logs for a run
- smithers_approve / smithers_deny: Manage approval gates
- smithers_hijack: Take over agent sessions
- smithers_cancel: Stop runs
- smithers_workflow_up: Start a workflow run
- smithers_workflow_list: List available workflows
- smithers_workflow_run: Execute a workflow with inputs
- smithers_diff / smithers_fork / smithers_replay: Time-travel debugging
- smithers_memory_list / smithers_memory_recall: Cross-run memory
- smithers_scores: Evaluation metrics
- smithers_cron_list: Schedule management
- smithers_sql: Direct database queries

When these tools are available via MCP, prefer them over shell commands.
If MCP tools are unavailable, fall back to `smithers` CLI via bash.
</smithers_tools>

<behavior>
- When listing runs, format results as tables with status indicators.
- Proactively mention pending approval gates when they exist.
- When a run fails, suggest inspection and common fixes.
- For hijacking, confirm with the user before taking over.
- Use tool results to provide context-aware, specific answers.
- Be concise and act as an orchestrator proxy.
</behavior>

{{- if .SmithersWorkflowDir }}
<workspace>
Workflow directory: {{ .SmithersWorkflowDir }}
</workspace>
{{- end }}

<env>
Working directory: {{.WorkingDir}}
Is directory a git repo: {{if .IsGitRepo}}yes{{else}}no{{end}}
Platform: {{.Platform}}
Today's date: {{.Date}}
</env>

{{if .ContextFiles}}
<memory>
{{range .ContextFiles}}
<file path="{{.Path}}">
{{.Content}}
</file>
{{end}}
</memory>
{{end}}
```

**Key differences from `coder.md.tpl`**:
- Removes all coding-specific instruction blocks (`<critical_rules>`, `<editing_files>`, `<whitespace_and_exact_matching>`, `<testing>`, `<code_conventions>`, `<communication_style>`, `<workflow>`, `<decision_making>`, `<task_completion>`, `<error_handling>`, `<memory_instructions>`, `<bash_commands>`, `<proactiveness>`, etc.) — the Smithers agent is an orchestrator proxy, not a code editor.
- Adds `<smithers_tools>` section listing all MCP tool names to teach the model the tool surface. This mirrors upstream `ask.ts` which hardcodes the tool listing in `SYSTEM_PROMPT`.
- Adds `<behavior>` section for Smithers-domain behavioral instructions (run formatting, approval proactivity).
- Retains `<env>` and `<memory>` blocks from the coder template since `PromptDat` already supplies those fields (`WorkingDir`, `IsGitRepo`, `Platform`, `Date`, `ContextFiles`).
- Adds `{{.SmithersWorkflowDir}}` template variable for workspace context.
- Omits `{{.GitStatus}}` and `{{.AvailSkillXML}}` — the Smithers agent doesn't need git diff context or Crush skill discovery.

### Slice 2: Extend Prompt and PromptDat with Smithers fields

**File**: `internal/agent/prompt/prompt.go`

**Step 2a**: Add private Smithers fields to the `Prompt` struct (line 21-27):

```go
type Prompt struct {
    name                string
    template            string
    now                 func() time.Time
    platform            string
    workingDir          string
    smithersMode        bool   // NEW
    smithersWorkflowDir string // NEW
    smithersMCPServer   string // NEW
}
```

**Step 2b**: Add exported Smithers fields to `PromptDat` (line 29-40):

```go
type PromptDat struct {
    // ... existing 10 fields unchanged ...
    SmithersMode        bool   // true when running in Smithers TUI mode
    SmithersWorkflowDir string // path to .smithers/workflows/ if present
    SmithersMCPServer   string // name of the Smithers MCP server (e.g., "smithers")
}
```

**Step 2c**: Add `WithSmithersMode` option (after line 65):

```go
func WithSmithersMode(workflowDir, mcpServer string) Option {
    return func(p *Prompt) {
        p.smithersMode = true
        p.smithersWorkflowDir = workflowDir
        p.smithersMCPServer = mcpServer
    }
}
```

This follows the existing `Option` functional-options pattern (WithTimeFunc at line 49, WithPlatform at line 55, WithWorkingDir at line 61).

**Step 2d**: Populate Smithers fields in `promptData()` (line 181-202, after the `data := PromptDat{...}` block):

```go
data := PromptDat{
    // ... existing fields ...
    SmithersMode:        p.smithersMode,
    SmithersWorkflowDir: p.smithersWorkflowDir,
    SmithersMCPServer:   p.smithersMCPServer,
}
```

**Data flow**:
```
Config.Smithers.WorkflowDir
  → coordinator reads from ConfigStore
    → passed as arg to WithSmithersMode()
      → stored on Prompt struct
        → copied to PromptDat in promptData()
          → template renders conditional {{.SmithersWorkflowDir}} block
```

### Slice 3: Register the Smithers prompt constructor

**File**: `internal/agent/prompts.go`

Add a new `//go:embed` directive and constructor after the existing `taskPrompt()` (line 34), following the identical pattern used by `coderPrompt()` (line 20-26):

```go
//go:embed templates/smithers.md.tpl
var smithersPromptTmpl []byte

func smithersPrompt(opts ...prompt.Option) (*prompt.Prompt, error) {
    systemPrompt, err := prompt.NewPrompt("smithers", string(smithersPromptTmpl), opts...)
    if err != nil {
        return nil, err
    }
    return systemPrompt, nil
}
```

After this change, `prompts.go` will have four embedded templates: `coder.md.tpl`, `task.md.tpl`, `initialize.md.tpl`, and `smithers.md.tpl`.

### Slice 4: Add Smithers agent constant and config struct

**File**: `internal/config/config.go`

**Step 4a**: Add agent constant (line 61-64):

```go
const (
    AgentCoder    string = "coder"
    AgentTask     string = "task"
    AgentSmithers string = "smithers"
)
```

**Step 4b**: Add `SmithersConfig` struct (before the `Config` struct at line 373):

```go
type SmithersConfig struct {
    DBPath      string `json:"dbPath,omitempty"`
    APIURL      string `json:"apiUrl,omitempty"`
    APIToken    string `json:"apiToken,omitempty"`
    WorkflowDir string `json:"workflowDir,omitempty"`
}
```

**Step 4c**: Add Smithers field to Config struct (line 393, after `Tools`):

```go
type Config struct {
    // ... existing fields (Schema, Models, RecentModels, Providers, MCP, LSP, Options, Permissions, Tools) ...
    Smithers *SmithersConfig `json:"smithers,omitempty"`
    Agents   map[string]Agent `json:"-"`
}
```

Default values when present: `DBPath` → `.smithers/smithers.db`, `WorkflowDir` → `.smithers/workflows`.

**Step 4d**: Extend `SetupAgents()` (line 513-538) to conditionally create the smithers agent.

**Critical detail**: The `Agents` field has tag `json:"-"` — it is NOT deserialized from JSON config. Agents are created programmatically in `SetupAgents()`. The smithers agent must be added here, gated on `Config.Smithers != nil`:

```go
func (c *Config) SetupAgents() {
    allowedTools := resolveAllowedTools(allToolNames(), c.Options.DisabledTools)

    agents := map[string]Agent{
        AgentCoder: { /* ... existing ... */ },
        AgentTask:  { /* ... existing ... */ },
    }

    // Add Smithers agent when Smithers config section is present
    if c.Smithers != nil {
        agents[AgentSmithers] = Agent{
            ID:           AgentSmithers,
            Name:         "Smithers",
            Description:  "A specialized agent for managing Smithers AI workflows.",
            Model:        SelectedModelTypeLarge,
            ContextPaths: c.Options.ContextPaths,
            AllowedTools: allowedTools,
        }
    }

    c.Agents = agents
}
```

This means the Smithers agent only exists when the user has a `"smithers": {...}` section in their config file. Existing Crush users without this section get the identical agent set as before.

### Slice 5: Wire prompt selection into coordinator

**File**: `internal/agent/coordinator.go`

Replace the hardcoded `AgentCoder` lookup at line 115-132 with dynamic agent resolution:

```go
// BEFORE (line 115-132):
agentCfg, ok := cfg.Config().Agents[config.AgentCoder]
if !ok {
    return nil, errCoderAgentNotConfigured
}
// TODO: make this dynamic when we support multiple agents
prompt, err := coderPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))

// AFTER:
agentName, agentCfg := c.resolveAgent(cfg)
var p *prompt.Prompt
var err error
switch agentName {
case config.AgentSmithers:
    var workflowDir string
    if s := cfg.Config().Smithers; s != nil {
        workflowDir = s.WorkflowDir
    }
    p, err = smithersPrompt(
        prompt.WithWorkingDir(c.cfg.WorkingDir()),
        prompt.WithSmithersMode(workflowDir, "smithers"),
    )
default:
    p, err = coderPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
}
if err != nil {
    return nil, err
}

agent, err := c.buildAgent(ctx, p, agentCfg, false)
```

The `resolveAgent` helper:

```go
func (c *coordinator) resolveAgent(cfg *config.ConfigStore) (string, config.Agent) {
    if agentCfg, ok := cfg.Config().Agents[config.AgentSmithers]; ok {
        return config.AgentSmithers, agentCfg
    }
    if agentCfg, ok := cfg.Config().Agents[config.AgentCoder]; ok {
        return config.AgentCoder, agentCfg
    }
    return config.AgentCoder, config.Agent{}
}
```

The Smithers agent takes priority when present. The existing TODO at line 120 ("make this dynamic when we support multiple agents") is resolved by this change.

### Slice 6: Test fixture config

**File**: `testdata/smithers-tui.json`

Create a minimal config fixture used by both unit and E2E tests:

```jsonc
{
  "models": {
    "large": { "model": "claude-opus-4-6", "provider": "anthropic" }
  },
  "providers": {
    "anthropic": { "apiKey": "${ANTHROPIC_API_KEY}" }
  },
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  },
  "mcpServers": {
    "smithers": {
      "type": "stdio",
      "command": "smithers",
      "args": ["mcp-serve"]
    }
  }
}
```

### Slice 7: Unit tests

**Files**:
- `internal/agent/prompts_test.go` — test `smithersPrompt()` renders without error and contains key markers.
- `internal/agent/prompt/prompt_test.go` — test `PromptDat` with Smithers fields populates template correctly.
- `internal/agent/coordinator_test.go` — test `resolveAgent()` picks smithers when configured.
- `internal/config/config_test.go` — test `SetupAgents()` conditionally creates smithers agent.

Test cases:

1. **Template renders with Smithers fields**: Create `smithersPrompt()` with `WithSmithersMode(".smithers/workflows", "smithers")`, call `Build()`, assert output contains `"smithers_ps"`, `"approval gates"`, `"orchestrator"`, and the workflow dir path `.smithers/workflows`.

2. **Template renders without Smithers fields**: Create `smithersPrompt()` without `WithSmithersMode`, call `Build()`, assert the `<workspace>` block is absent (conditional rendering works).

3. **Template does not contain coder instructions**: Assert the rendered output does NOT contain `"READ BEFORE EDITING"`, `"<editing_files>"`, `"<code_conventions>"`, or other coder-specific blocks.

4. **Agent resolution prefers Smithers**: Set up a config with both `coder` and `smithers` agents in the Agents map, call `resolveAgent()`, assert `AgentSmithers` is returned.

5. **Agent resolution falls back to coder**: Set up a config with only `coder` agent, call `resolveAgent()`, assert `AgentCoder` is returned.

6. **SetupAgents creates smithers agent when config present**: Create a `Config` with `Smithers: &SmithersConfig{...}`, call `SetupAgents()`, assert `Agents[AgentSmithers]` exists with `Name == "Smithers"`.

7. **SetupAgents omits smithers agent when config absent**: Create a `Config` with `Smithers: nil`, call `SetupAgents()`, assert `Agents[AgentSmithers]` does not exist. Only `coder` and `task` agents present.

8. **Snapshot test**: `TestSmithersPromptSnapshot` renders the full template with fixture data and compares against a golden file at `internal/agent/testdata/smithers_prompt.golden`. Run with `go test -update` to regenerate.

## Validation

### Automated checks

1. **Unit tests**: `go test ./internal/agent/... ./internal/config/...` — all existing tests pass and new prompt/config tests pass (8 test cases from Slice 7).

2. **Template compilation check**: `go build ./...` — the `//go:embed` directive for `smithers.md.tpl` compiles successfully, confirming the file exists at `internal/agent/templates/smithers.md.tpl` and is valid UTF-8.

3. **Snapshot test**: `TestSmithersPromptSnapshot` renders the full template with fixture data and compares against a golden file at `internal/agent/testdata/smithers_prompt.golden`. Run with `go test -update` to regenerate.

### Terminal E2E tests (modeled on upstream @microsoft/tui-test harness)

Model E2E tests on `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`.

The upstream pattern uses a `TUITestInstance` class with `waitForText()`, `sendKeys()`, `snapshot()`, and `terminate()` methods, spawning the TUI binary with piped I/O. The Go equivalent translates this to a test helper package:

**File**: `tests/e2e/tui_helpers_test.go`

```go
// TUITestInstance mirrors the upstream TUITestInstance from tui-helpers.ts.
// Upstream methods → Go equivalents:
//   tui.waitForText("text")     → tui.WaitForText("text", timeout)
//   tui.sendKeys("keys")        → tui.SendKeys("keys")
//   tui.snapshot()               → tui.Snapshot()
//   tui.terminate()              → tui.Terminate()
type TUITestInstance struct {
    t      *testing.T
    cmd    *exec.Cmd
    stdin  io.Writer
    buffer *syncBuffer // goroutine-safe buffer accumulating stdout+stderr
}

// launchTUI spawns the compiled binary with piped stdio.
// Mirrors upstream launchTUI(["tui"]) in tui-helpers.ts.
func launchTUI(t *testing.T, args ...string) *TUITestInstance

// WaitForText polls the accumulated output for the target string.
// Fails the test if timeout expires without a match.
func (t *TUITestInstance) WaitForText(text string, timeout time.Duration) error

// SendKeys writes raw bytes to the process's stdin.
func (t *TUITestInstance) SendKeys(keys string)

// Snapshot returns the current accumulated output as a string.
func (t *TUITestInstance) Snapshot() string

// Terminate sends SIGTERM and waits for the process to exit.
func (t *TUITestInstance) Terminate()
```

**File**: `tests/e2e/system_prompt_test.go`

```go
func TestSmithersAgentPromptLoaded(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    // 1. Launch the TUI binary with a Smithers config
    tui := launchTUI(t, "--config", "testdata/smithers-tui.json")
    defer tui.Terminate()

    // 2. Verify the chat view loads (agent ready)
    tui.WaitForText("Smithers", 5*time.Second)

    // 3. Send a prompt that will exercise the system prompt knowledge
    tui.SendKeys("What Smithers tools do you have access to?\n")

    // 4. Verify the agent response references Smithers MCP tools
    //    (this confirms the system prompt was loaded, not the coder prompt)
    tui.WaitForText("smithers_ps", 15*time.Second)

    // 5. Verify no coder-specific content leaked
    snapshot := tui.Snapshot()
    assert.NotContains(t, snapshot, "READ BEFORE EDITING")
}

func TestFallbackToCoderPromptWithoutSmithersConfig(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    // Launch with a standard Crush config (no smithers section)
    tui := launchTUI(t, "--config", "testdata/crush-default.json")
    defer tui.Terminate()

    // Verify the coder agent loads (no Smithers branding)
    tui.WaitForText("Crush", 5*time.Second)
}
```

This directly follows the upstream pattern from `tui.e2e.test.ts`:
- `launchTUI()` → upstream `launchTUI(["tui"])` (tui-helpers.ts)
- `WaitForText()` → upstream `tui.waitForText("Smithers Runs")`
- `SendKeys()` → upstream `tui.sendKeys("\r")`
- `Terminate()` → upstream `tui.terminate()`
- `Snapshot()` → upstream `tui.snapshot()`

### VHS happy-path recording test

**File**: `tests/vhs/smithers_system_prompt.tape`

```
# Smithers TUI — Domain System Prompt Happy Path
# Validates: system prompt loaded, agent references Smithers tools
Output tests/vhs/output/smithers_system_prompt.gif
Set FontSize 14
Set Width 1200
Set Height 800
Set Shell zsh

# Launch Smithers TUI with test config
Type "go run . --config testdata/smithers-tui.json"
Enter
Sleep 3s

# Verify Smithers branding is visible (not Crush)
Screenshot tests/vhs/output/smithers_boot.png

# Ask about available tools — exercises system prompt knowledge
Type "What tools can you use to manage my workflows?"
Enter
Sleep 8s

# The agent should reference Smithers MCP tools (not coding tools)
Screenshot tests/vhs/output/smithers_prompt_response.png

# Ask about approvals — exercises behavioral instructions
Type "Are there any pending approvals?"
Enter
Sleep 5s

Screenshot tests/vhs/output/smithers_approvals_check.png
```

Run with: `vhs tests/vhs/smithers_system_prompt.tape`

The recording confirms:
- The TUI boots with Smithers agent mode visible (not Crush coder mode).
- The agent's response references Smithers-domain tools (`smithers_ps`, `smithers_approve`, etc.) rather than coding tools like editing or testing.
- The agent proactively engages with approval-related queries, confirming the `<behavior>` section is in effect.

### Manual verification

1. **Build and run**: `go build -o smithers-tui . && ./smithers-tui --config testdata/smithers-tui.json`
2. **Check agent mode**: Verify the header/status shows "Smithers Agent Mode" (per 02-DESIGN.md §3.1 chat layout), not "Coder".
3. **Chat test**: Type "What can you help me with?" and verify the response mentions workflows, runs, approvals, and Smithers MCP tools rather than coding assistance.
4. **Prompt dump**: Set `CRUSH_DEBUG=1` (or equivalent) to dump the rendered system prompt to stderr/logs and verify it matches the expected Smithers template output, not the coder template.
5. **Fallback test**: Remove the `"smithers"` section from config, restart, and verify the coder prompt is loaded (backward compatibility).

## Risks

### 1. Agents map is `json:"-"` — not loaded from config JSON

**Risk**: The `Config.Agents` field (config.go:395) has tag `json:"-"`, meaning agents are created programmatically in `SetupAgents()` (config.go:513-538), not deserialized from the config file. An implementation that assumes agents can be configured via JSON will silently fail — the smithers agent will never appear in the map.

**Mitigation**: This spec explicitly addresses this in Slice 4d by adding the smithers agent creation to `SetupAgents()`, gated on `Config.Smithers != nil`. Unit test case 6 validates this path. Do NOT attempt to serialize agents in JSON config; follow the existing pattern.

### 2. Crush prompt template divergence

**Risk**: The `coder.md.tpl` template evolves upstream in Crush. The Smithers template shares structural patterns (env block, memory block, context files) with it. If upstream changes the `PromptDat` struct or template syntax, the Smithers template may silently break.

**Mitigation**: Keep Smithers-specific fields strictly additive — new optional fields on `PromptDat` and `Prompt`, never modify existing ones. The snapshot test in `TestSmithersPromptSnapshot` catches rendering regressions. The template reuses the same `PromptDat` struct and Go template syntax, so upstream cherry-picks will surface compile errors if fields are renamed.

### 3. MCP tool name drift between template and server

**Risk**: The Smithers system prompt hardcodes MCP tool names (`smithers_ps`, `smithers_approve`, etc.). If the Smithers MCP server renames or adds tools, the prompt becomes stale. This is the same pattern used in upstream `ask.ts` which also hardcodes tool names.

**Mitigation**: Add a cross-reference test that parses the Smithers template for tool names matching `smithers_\w+` and compares against an authoritative list (initially a constants file, later the MCP server's tool manifest). In the short term, accept the hardcoding — MCP's dynamic tool discovery means the model will see the actual tool list at runtime regardless. The prompt listing is instructional (teaches the model the domain vocabulary), not normative.

### 4. Smithers config section not present in existing Crush configs

**Risk**: Existing Crush users who update to this fork will not have a `"smithers"` section in their config. If the code assumes `Config.Smithers` is non-nil, it will panic.

**Mitigation**: Three layers of nil safety: (1) `SetupAgents()` only creates the smithers agent when `Config.Smithers != nil`; (2) `resolveAgent()` falls back to `AgentCoder` when no smithers agent exists in the map; (3) template conditional blocks (`{{- if .SmithersWorkflowDir }}`) gracefully handle empty strings. Unit test cases 5 and 7 validate the fallback path.

### 5. System prompt size vs. context window

**Risk**: The Smithers prompt is shorter than the ~400-line coder prompt (targeting ~80-120 lines), but future tickets (`chat-active-run-summary`, `chat-workspace-context`) will inject dynamic context that could grow the prompt significantly.

**Mitigation**: Keep the base prompt lean. Dynamic context injection should use summarized data, not raw dumps. Add a `promptTokenCount` debug log in `Build()` to monitor growth. Consider splitting dynamic sections into separate system messages (Crush already supports `SystemPromptPrefix` per provider at config.go:111) rather than embedding everything in the template.

### 6. Mismatch: Crush uses Go templates, upstream uses string constants

**Risk**: Upstream Smithers defines the system prompt as a static TypeScript string constant (`ask.ts`). Crush uses Go `text/template` with `PromptDat`. This means the Smithers TUI prompt is not a 1:1 copy — it's a Go template adaptation. Behavioral parity depends on the template faithfully capturing the upstream intent.

**Mitigation**: The template content is derived from the upstream constant but adapted for Go template capabilities (conditional blocks, context file injection). The E2E test `TestSmithersAgentPromptLoaded` validates behavioral parity by checking the agent's response reflects Smithers domain knowledge regardless of template mechanism. The snapshot golden file provides a human-reviewable rendering of the full prompt.
