# Research: feat-agents-browser

## Existing Crush Surface

### agents.go — What Works

`internal/ui/views/agents.go` has a solid scaffold that already satisfies the `View` interface:

- **Struct**: `AgentsView` holds `client *smithers.Client`, `agents []smithers.Agent`, `cursor int`, `width/height int`, `loading bool`, `err error`.
- **Init**: Issues an async command that calls `client.ListAgents(ctx)` and returns either `agentsLoadedMsg` or `agentsErrorMsg`.
- **Update**: Handles `agentsLoadedMsg`, `agentsErrorMsg`, `tea.WindowSizeMsg`, and `tea.KeyPressMsg` for `esc` (pop view), `up/k` / `down/j` (cursor), `r` (refresh), and `enter` (no-op placeholder).
- **View**: Renders a `SMITHERS › Agents` header with right-aligned `[Esc] Back` hint, a `▸` cursor against the selected row, and per-agent `Name` + `Status` lines separated by blank lines.
- **ShortHelp**: Returns `["[Enter] Launch", "[r] Refresh", "[Esc] Back"]`.
- **Compile-time check**: `var _ View = (*AgentsView)(nil)` ensures interface compliance.

### agents.go — What Is Missing

1. **Status icons are static**: `statusIcon := "○"` is hardcoded regardless of `agent.Status`. The view does not map `"likely-subscription"` → filled dot, `"api-key"` → filled dot, `"binary-only"` → dim dot, `"unavailable"` → empty circle.
2. **No detail pane**: Only `Name` and `Status` are shown. `BinaryPath`, `HasAuth`, `HasAPIKey`, and `Roles` are defined in the `Agent` struct but never rendered.
3. **No grouping**: The PRD (§6.8) and Design doc (§3.7) both expect agents to be split into "Available" and "Not Detected" sections, matching `AgentsList.tsx` in the upstream GUI reference.
4. **Enter is a no-op**: The `enter` handler has a `// No-op for now; future: TUI handoff.` comment. No `HandoffToProgram` call exists yet.
5. **No `HandoffReturnMsg` handling**: After a handoff the view needs to receive a return message and refresh state. There is no handler for this.
6. **Cursor does not skip unavailable agents**: Users can select agents with `Status: "unavailable"`, which have no binary and cannot be launched.
7. **No launch confirmation overlay**: The Design doc (§3.7) specifies a brief "Launching claude-code…" message before handing off. This is not implemented.
8. **No splitting of the detail pane**: The design shows a two-column layout (agent list left, detail right) for wider terminals. The current implementation is single-column only.

### ListAgents — Transport Gap

`internal/smithers/client.go:108` contains:

```go
func (c *Client) ListAgents(_ context.Context) ([]Agent, error) {
    return []Agent{
        {ID: "claude-code", Name: "Claude Code", Command: "claude", Status: "unavailable"},
        {ID: "codex",       Name: "Codex",       Command: "codex",  Status: "unavailable"},
        ...
    }, nil
}
```

This is a hardcoded stub. It returns six fixed entries, all with `Status: "unavailable"`, no resolved `BinaryPath`, no `HasAuth` / `HasAPIKey` signals, and no `Roles`. No real system probing occurs.

The `Agent` struct in `internal/smithers/types.go` is complete and maps correctly to the upstream detection surface:

| Go field   | Smithers upstream source                         |
|------------|--------------------------------------------------|
| `ID`       | `AgentAvailability.id` in `agent-detection.ts`   |
| `Name`     | `agentCliSchema.name` in `agent.ts`              |
| `Command`  | CLI binary name (e.g. `"claude"`, `"codex"`)     |
| `BinaryPath` | Resolved from `exec.LookPath(Command)` or detection output |
| `Status`   | `AgentAvailabilityStatus`: `likely-subscription` \| `api-key` \| `binary-only` \| `unavailable` |
| `HasAuth`  | Auth signal (e.g. `~/.claude` presence)          |
| `HasAPIKey`| API key env var present (e.g. `ANTHROPIC_API_KEY`) |
| `Usable`   | `Status != "unavailable"`                        |
| `Roles`    | From `agentCliSchema.roles` (coding, review, research, etc.) |

### Router and UI Wiring

`internal/ui/views/router.go` is a minimal stack router (`Push`, `Pop`, `Current`, `HasViews`) using `PopViewMsg` for back navigation. It is fully functional for the agents view.

`internal/ui/model/ui.go` creates the Smithers client at line 332 (`smithers.NewClient()` with no options) and handles view dispatch. The command palette entry at `dialog/commands.go:527` and `ActionOpenAgentsView` in `dialog/actions.go:88` are wired but need verification that they reach `router.Push(views.NewAgentsView(client))`.

### TUI Handoff Infrastructure

`internal/ui/util/util.go` has `ExecShell(ctx, cmdStr, callback)` which wraps `tea.ExecProcess`. The existing `openEditor` in `ui.go:2785` uses `tea.ExecProcess` directly and is the only real-world handoff in the codebase.

The research doc for `eng-hijack-handoff-util` identified that no generic `HandoffToProgram` utility exists yet (the file `internal/ui/util/handoff.go` has not been created). The plan for that ticket specifies:

- `HandoffToProgram(binary string, args []string, cwd string, env []string) tea.Cmd`
- Pre-launch `exec.LookPath` validation to avoid screen clearing before a "binary not found" failure
- `HandoffReturnMsg{Err error, Tag any}` to route the return back into the Update loop
- Environment merging (`os.Environ()` + overrides)

The agents browser is the primary consumer of this utility after the hijack feature. The `feat-agents-browser` ticket depends on `eng-hijack-handoff-util` being shipped or on implementing an inline equivalent.

---

## Upstream Smithers Reference

### Agent Definitions in `.smithers/agents.ts`

The project's `.smithers/agents.ts` defines six agent providers:

| Key     | Class           | Model                        | Binary (inferred) |
|---------|-----------------|------------------------------|-------------------|
| claude  | ClaudeCodeAgent | claude-opus-4-6              | `claude`          |
| codex   | CodexAgent      | gpt-5.3-codex                | `codex`           |
| gemini  | GeminiAgent     | gemini-3.1-pro-preview       | `gemini`          |
| pi      | PiAgent         | gpt-5.3-codex (openai prov.) | `pi` (unknown)    |
| kimi    | KimiAgent       | kimi-latest                  | `kimi`            |
| amp     | AmpAgent        | (default)                    | `amp`             |

Role chains map these agents to workflow tasks:

| Role       | Priority chain (first = preferred)                 |
|------------|----------------------------------------------------|
| spec       | claude, codex                                      |
| research   | codex, kimi, gemini, claude                        |
| plan       | codex, gemini, claude, kimi                        |
| implement  | codex, amp, gemini, claude, kimi                   |
| validate   | codex, amp, gemini                                 |
| review     | claude, amp, codex                                 |

Note `forge` appears in the PRD (§6.4, §6.8) as a supported agent but is absent from `agents.ts`. The client stub (`ListAgents`) includes `forge` in its hardcoded list. Forge should be kept in the detection manifest but its CLI binary (`forge`) needs to be confirmed.

Also note `pi` uses the openai provider but its binary name is unknown — the TUI should attempt `pi` but treat it as `unavailable` if detection fails.

### Agent Properties: CLI Binary, Status Detection, Auth Checking

Based on the `agent-detection.ts` module described in prior research and the agents.ts definitions, the detection logic per agent is:

**Binary detection** — for each agent, attempt `exec.LookPath(command)`:

| Agent ID   | Command to look up | Typical install path         |
|------------|-------------------|------------------------------|
| claude-code | `claude`          | `/usr/local/bin/claude`      |
| codex      | `codex`           | `/usr/local/bin/codex`       |
| gemini     | `gemini`          | `/usr/local/bin/gemini`      |
| kimi       | `kimi`            | (varies, often not in PATH)  |
| amp        | `amp`             | `/usr/local/bin/amp`         |
| forge      | `forge`           | (varies)                     |
| pi         | `pi`              | (likely not standard)        |

**Auth status classification** — the four-value enum used throughout the codebase:

| Status value           | Meaning                                                  |
|------------------------|----------------------------------------------------------|
| `likely-subscription`  | Binary found AND auth credential files present           |
| `api-key`              | Binary found AND relevant API key env var is set         |
| `binary-only`          | Binary found but no auth signal detected                 |
| `unavailable`          | Binary NOT found (not in PATH)                           |

**Per-agent auth signal detection**:

| Agent      | Auth file check                        | Env var check             |
|------------|----------------------------------------|---------------------------|
| claude-code | `~/.claude/` directory exists          | `ANTHROPIC_API_KEY`       |
| codex      | `~/.codex/` or `~/.openai` presence   | `OPENAI_API_KEY`          |
| gemini     | `~/.gemini/` presence or gcloud creds | `GEMINI_API_KEY` or `GOOGLE_API_KEY` |
| kimi       | (provider-specific)                    | `KIMI_API_KEY` or `MOONSHOT_API_KEY` |
| amp        | `~/.amp/` presence                    | (provider-specific key)   |
| forge      | (provider-specific)                    | `FORGE_API_KEY`           |

Auth file checks use `os.Stat` (or equivalent). Env var checks use `os.Getenv`. The status logic is:

```
if BinaryPath == "":
    Status = "unavailable", Usable = false
else if auth dir exists:
    Status = "likely-subscription", HasAuth = true, Usable = true
else if API key env var is non-empty:
    Status = "api-key", HasAPIKey = true, Usable = true
else:
    Status = "binary-only", Usable = true   // can launch, may fail at runtime
```

**Version detection** (optional, for display): `exec.Command(binary, "--version")` with a short timeout (1–2s). Used to populate a version string in the detail pane. Failure is non-fatal.

---

## Binary/Path Detection Approaches

### Option A: Pure Go detection (recommended)

Replace the `ListAgents` stub with a Go implementation that:

1. Iterates the known agent manifest (hardcoded list of `{ID, Name, Command, authDir, apiKeyEnv}`).
2. For each entry, calls `exec.LookPath(Command)` to resolve the binary path.
3. Performs auth detection via `os.Stat` + `os.Getenv`.
4. Returns populated `[]Agent` results.

Advantages: no subprocess overhead, no dependency on `smithers` CLI being installed, fast (<10ms for 7 agents), testable by mocking `lookPath` and `statFunc`.

### Option B: Delegate to `smithers agents list --json`

Call `execSmithers(ctx, "agents", "list", "--format", "json")` and parse JSON output. The existing `execSmithers` helper in `client.go` supports this pattern (used for crons, tickets, approvals).

Advantages: single source of truth, automatically picks up new agents if Smithers CLI is updated.

Disadvantages: requires `smithers` CLI to be installed and in PATH; introduces process startup latency on every open of the agents view; test mocking is more complex.

### Option C: Hybrid (HTTP first, exec fallback, pure-Go last)

Try HTTP `GET /agent/list` → exec `smithers agents list --json` → pure-Go detection. Matches the existing tiered transport pattern used for approvals, crons, etc.

Disadvantages: the Smithers server is typically not running when users just want to see what agents are installed (agents browser is for ad-hoc use); adds complexity for a feature that works fine with pure-Go detection.

**Recommended**: Option A (pure Go) for the initial implementation, structured so Option C can be added later without changing the `AgentsView` — the view only calls `client.ListAgents()` and is transport-agnostic.

---

## Native TUI Handoff Requirement

The agents view `Enter` action must hand off to the selected agent's native CLI/TUI:

- **Mechanism**: `tea.ExecProcess(cmd, callback)` via the `HandoffToProgram` utility (from `eng-hijack-handoff-util`). The utility handles `exec.LookPath` pre-validation, environment merging, and returns a `HandoffReturnMsg` to the Update loop.
- **Commands per agent**:

| Agent      | Launch command                   | Notes                        |
|------------|----------------------------------|------------------------------|
| claude-code | `claude` (no special args)      | Opens interactive session     |
| codex      | `codex`                          | Opens interactive session     |
| gemini     | `gemini`                         | Opens interactive session     |
| kimi       | `kimi`                           | Opens interactive session     |
| amp        | `amp`                            | Opens interactive session     |
| forge      | `forge`                          | Opens interactive session     |

- **Working directory**: current working directory (`os.Getwd()`) so the agent starts in the project root.
- **Guard**: only allow handoff if `agent.Usable == true` (i.e. binary was found). Show an inline error or skip-with-message for `Status: "unavailable"` agents.
- **On return**: handle `HandoffReturnMsg` in `AgentsView.Update()`. Refresh agent state (re-run `Init()`) to pick up any auth changes that may have occurred during the session.

### Dependency status of `HandoffToProgram`

As of this research, `internal/ui/util/handoff.go` does not exist. The `feat-agents-browser` ticket can:

1. **Wait for `eng-hijack-handoff-util`** to ship the utility first (clean dependency).
2. **Implement a minimal inline version** within the agents view that uses `tea.ExecProcess` directly with a pre-`LookPath` guard, accepting slightly less polish than the full utility.

Given that `eng-hijack-handoff-util` is already planned and researched, option 1 is preferred. The agents view can stub the `Enter` handler with a visible error message ("Handoff utility not yet available") until the dependency lands.

---

## Testing Infrastructure Gaps

- No unit tests exist for `AgentsView` (`internal/ui/views/agents_test.go` is absent).
- No unit tests exist for `ListAgents` parsing (`internal/smithers/client_test.go` does not cover agent detection).
- No Go terminal E2E harness exists (analogous to `../smithers/tests/tui-helpers.ts`). The `eng-agents-view-scaffolding` research and plans both call for `tests/tui/helpers_test.go` and `tests/tui/agents_e2e_test.go` but these files have not been created yet.
- No VHS tape for the agents view exists.

The testing strategy for `feat-agents-browser` extends and depends on the scaffolding from `eng-agents-view-scaffolding`. Both the transport-layer unit tests (mocked `lookPath`) and the E2E harness must be in place before the view feature is considered done.

---

## Files Relevant to This Ticket

| File | Relevance |
|------|-----------|
| `internal/ui/views/agents.go` | Primary view — needs status icons, detail pane, grouping, `Enter` handoff |
| `internal/smithers/client.go` | `ListAgents` stub needs replacing with real detection |
| `internal/smithers/types.go` | `Agent` struct is complete; no changes needed |
| `internal/ui/util/handoff.go` | Does not exist yet; dependency for `Enter` handoff |
| `internal/ui/views/router.go` | Wiring is correct; no changes needed |
| `internal/ui/model/ui.go` | Client instantiation (line 332) — may need `WithDBPath`/`WithAPIURL` injection |
| `internal/ui/dialog/commands.go` | `/agents` command palette entry (line 527) — verify it dispatches correctly |
| `internal/ui/dialog/actions.go` | `ActionOpenAgentsView` (line 88) |
| `.smithers/agents.ts` | Canonical agent manifest for this project |
