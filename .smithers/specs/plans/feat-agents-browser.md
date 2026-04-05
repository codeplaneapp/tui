# Implementation Plan: feat-agents-browser

## Goal

Complete the Agents Browser view: replace the hardcoded `ListAgents` stub with real system detection, add rich status indicators and a detail pane to the view, group agents by availability, and wire up the `Enter` key to hand off to the selected agent's native CLI/TUI via `tea.ExecProcess`. The result mirrors the GUI's `AgentsList.tsx` experience in the terminal.

---

## Steps

### Step 1 — Real agent detection in `ListAgents`

**File**: `internal/smithers/client.go`

Replace the hardcoded stub with pure-Go binary detection. Define a package-level manifest of known agents and iterate it on each call:

```go
type agentManifest struct {
    id         string
    name       string
    command    string
    roles      []string
    authDir    string   // relative to $HOME, e.g. ".claude"
    apiKeyEnv  string   // e.g. "ANTHROPIC_API_KEY"
}

var knownAgents = []agentManifest{
    {id: "claude-code", name: "Claude Code", command: "claude",
     roles: []string{"coding", "review", "spec"},
     authDir: ".claude", apiKeyEnv: "ANTHROPIC_API_KEY"},
    {id: "codex", name: "Codex", command: "codex",
     roles: []string{"coding", "implement"},
     authDir: ".codex", apiKeyEnv: "OPENAI_API_KEY"},
    {id: "gemini", name: "Gemini", command: "gemini",
     roles: []string{"coding", "research"},
     authDir: ".gemini", apiKeyEnv: "GEMINI_API_KEY"},
    {id: "kimi", name: "Kimi", command: "kimi",
     roles: []string{"research", "plan"},
     authDir: "", apiKeyEnv: "KIMI_API_KEY"},
    {id: "amp", name: "Amp", command: "amp",
     roles: []string{"coding", "validate"},
     authDir: ".amp", apiKeyEnv: ""},
    {id: "forge", name: "Forge", command: "forge",
     roles: []string{"coding"},
     authDir: "", apiKeyEnv: "FORGE_API_KEY"},
}
```

Detection logic per manifest entry:

1. `exec.LookPath(m.command)` → `BinaryPath`. If error, `Status = "unavailable"`, `Usable = false`, skip auth checks.
2. If `m.authDir != ""`, call `os.Stat(filepath.Join(homeDir, m.authDir))`. If it succeeds, `HasAuth = true`.
3. If `m.apiKeyEnv != ""`, call `os.Getenv(m.apiKeyEnv)`. If non-empty, `HasAPIKey = true`.
4. Compute `Status`:
   - `HasAuth == true` → `"likely-subscription"`
   - `HasAPIKey == true` → `"api-key"`
   - binary found but no auth signal → `"binary-only"`
   - binary not found → `"unavailable"`
5. `Usable = Status != "unavailable"`

Make `lookPath` and `statFunc` injectable for testability:

```go
// Client fields (add alongside execFunc)
lookPath func(file string) (string, error)
statFunc func(name string) (os.FileInfo, error)
```

Default to `exec.LookPath` and `os.Stat`. Tests override via `withLookPath` / `withStatFunc` options.

The context parameter (`_ context.Context`) can remain unused for now (detection is synchronous), but retain it in the signature for future HTTP/exec fallback compatibility.

---

### Step 2 — Status icons and availability grouping in the view

**File**: `internal/ui/views/agents.go`

#### 2a — Status icon helper

```go
func agentStatusIcon(status string) string {
    switch status {
    case "likely-subscription":
        return "●" // filled, green-tinted via lipgloss
    case "api-key":
        return "●" // filled, amber-tinted
    case "binary-only":
        return "◐" // half-filled, dim
    default:
        return "○" // empty
    }
}

func agentStatusStyle(status string) lipgloss.Style {
    switch status {
    case "likely-subscription":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))   // green
    case "api-key":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))   // yellow
    case "binary-only":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))   // dim
    default:
        return lipgloss.NewStyle().Faint(true)
    }
}
```

#### 2b — Grouped rendering

Split agents into two slices before rendering:

```go
var available, unavailable []smithers.Agent
for _, a := range v.agents {
    if a.Usable {
        available = append(available, a)
    } else {
        unavailable = append(unavailable, a)
    }
}
```

Render two sections:

```
Available (3)

▸ claude-code
  /usr/local/bin/claude
  ● likely-subscription   Auth: ✓  API Key: ✓   Roles: coding, review

  codex
  /usr/local/bin/codex
  ● api-key               Auth: ✗  API Key: ✓   Roles: coding

─────────────────────

Not Detected (3)

  kimi         ○ unavailable
  forge        ○ unavailable
  pi           ○ unavailable
```

The section divider (`─────`) uses `strings.Repeat("─", v.width-4)` padded to terminal width.

The `cursor` index tracks position across the combined list (available first, then unavailable). Cursor movement should skip unavailable agents when pressing `Enter`, but still allow selection for visual inspection.

#### 2c — Detail pane (wide terminals)

When `v.width >= 100`, split into two columns using a fixed left-pane width of 36 characters:

```
┌── Agent List (36) ───┬── Detail Pane (remainder) ──┐
│ ▸ claude-code        │ claude-code                  │
│   codex              │ Binary: /usr/local/bin/claude│
│   ...                │ Status: ● likely-subscription│
│                      │ Auth:   ✓ (~/.claude found)  │
│ Not Detected         │ APIKey: ✓ (ANTHROPIC_API_KEY)│
│   kimi               │ Roles:  coding, review, spec │
│   forge              │                              │
│                      │ [Enter] Launch TUI           │
└──────────────────────┴──────────────────────────────┘
```

Use `lipgloss.JoinHorizontal` to assemble the two panes. When `v.width < 100`, fall back to the single-column layout already present.

---

### Step 3 — `Enter` key: TUI handoff

**File**: `internal/ui/views/agents.go`

When `Enter` is pressed on a `Usable` agent:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
    if v.cursor < len(v.agents) {
        agent := v.selectedAgent() // returns the agent at cursor position
        if !agent.Usable {
            // Show inline "not available" hint; no-op
            return v, nil
        }
        v.launching = true
        v.launchingName = agent.Name
        return v, handoffToAgent(agent)
    }
```

Add `launching bool` and `launchingName string` to `AgentsView`. When `launching == true`, replace the main content with a brief launch message:

```
  Launching claude-code...
  Smithers TUI will resume when you exit.
```

This message is shown for the ~100ms between `Enter` and the terminal flip; it also serves as a visible transition cue.

#### Handoff command builder

```go
func handoffToAgent(agent smithers.Agent) tea.Cmd {
    binary := agent.BinaryPath
    if binary == "" {
        binary = agent.Command // fallback (LookPath should have resolved this)
    }
    cmd := exec.Command(binary)
    return tea.ExecProcess(cmd, func(err error) tea.Msg {
        return agentHandoffReturnMsg{agentID: agent.ID, err: err}
    })
}
```

If `HandoffToProgram` from `eng-hijack-handoff-util` is available, delegate to it instead and use `HandoffReturnMsg` as the return type. The agents view update loop handles both forms:

```go
case agentHandoffReturnMsg:
    v.launching = false
    v.launchingName = ""
    if msg.err != nil {
        v.err = fmt.Errorf("launch %s: %w", msg.agentID, msg.err)
    }
    // Refresh agent list — auth state may have changed during the session
    v.loading = true
    return v, v.Init()
```

---

### Step 4 — Unit tests for detection logic

**File**: `internal/smithers/client_test.go` (extend existing file)

Test cases for the new `ListAgents` implementation using injected `lookPath` and `statFunc`:

- `TestListAgents_BinaryFound_WithAuthDir`: mock `lookPath` returns a path, mock `statFunc` succeeds → expect `Status: "likely-subscription"`, `HasAuth: true`, `Usable: true`.
- `TestListAgents_BinaryFound_WithAPIKey`: mock `lookPath` returns path, `statFunc` fails (no auth dir), env var set → expect `Status: "api-key"`, `HasAPIKey: true`.
- `TestListAgents_BinaryFound_NoAuth`: both checks fail → expect `Status: "binary-only"`, `Usable: true`.
- `TestListAgents_BinaryNotFound`: `lookPath` returns error → expect `Status: "unavailable"`, `Usable: false`, `BinaryPath: ""`.
- `TestListAgents_AllSix`: all six agents returned even if some are unavailable.
- `TestListAgents_ContextCancelled`: cancelled context does not panic (detection is synchronous; cancellation support is future work).

---

### Step 5 — Unit tests for the view

**File**: `internal/ui/views/agents_test.go` (new file)

Test the Bubble Tea model lifecycle:

- `TestAgentsView_Init_SetsLoading`: `NewAgentsView(client)` → `loading == true`, `Init()` returns a non-nil `tea.Cmd`.
- `TestAgentsView_LoadedMsg_PopulatesAgents`: send `agentsLoadedMsg{agents: testAgents}` → `loading == false`, `agents` has expected length.
- `TestAgentsView_ErrorMsg_SetsErr`: send `agentsErrorMsg{err: someErr}` → `loading == false`, `err != nil`.
- `TestAgentsView_CursorNavigation`: down/up key presses move cursor within bounds; cursor does not go negative or past last agent.
- `TestAgentsView_Esc_ReturnsPopViewMsg`: `Esc` key → returned `tea.Cmd` produces `PopViewMsg{}` when invoked.
- `TestAgentsView_Enter_UsableAgent_SetsLaunching`: `Enter` on a usable agent → `v.launching == true`.
- `TestAgentsView_Enter_UnavailableAgent_NoHandoff`: `Enter` on `Usable: false` agent → `v.launching == false`.
- `TestAgentsView_View_HeaderText`: `View()` output contains `"SMITHERS › Agents"`.
- `TestAgentsView_View_ShowsGroups`: `View()` output contains `"Available"` and `"Not Detected"` when both groups are non-empty.
- `TestAgentsView_View_StatusIcons`: `View()` output contains `"●"` for `likely-subscription` agents and `"○"` for `unavailable`.
- `TestAgentsView_Refresh_ReloadsAgents`: `r` key press → `loading == true` and `Init()` command is returned.

---

### Step 6 — E2E terminal harness and test

**File**: `tests/tui/helpers_test.go` (new, shared harness)

This is the Go equivalent of `../smithers/tests/tui-helpers.ts`. Spawn the TUI binary via `exec.Command`, attach a pipe to stdin, capture stdout, strip ANSI codes, and provide polling helpers:

```go
type TUIHarness struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    buf    *ansiBuffer // thread-safe, strips ANSI sequences
}

func NewTUIHarness(t *testing.T, args ...string) *TUIHarness
func (h *TUIHarness) WaitForText(t *testing.T, text string, timeout time.Duration) bool
func (h *TUIHarness) SendKeys(t *testing.T, keys string)
func (h *TUIHarness) Snapshot() string
func (h *TUIHarness) Close(t *testing.T)
```

**File**: `tests/tui/agents_e2e_test.go` (new)

```go
func TestAgentsView_Navigation(t *testing.T) {
    h := NewTUIHarness(t)
    defer h.Close(t)

    // Open command palette
    h.SendKeys(t, "/")
    h.WaitForText(t, "agents", 5*time.Second)

    // Navigate to agents view
    h.SendKeys(t, "agents\r")
    h.WaitForText(t, "SMITHERS › Agents", 5*time.Second)

    // View should show the groups
    snap := h.Snapshot()
    assert.Contains(t, snap, "Available")

    // Move cursor down
    h.SendKeys(t, "j")

    // Escape returns to chat
    h.SendKeys(t, "\x1b")
    h.WaitForText(t, "SMITHERS", 3*time.Second)
    // Should no longer show the agents header
    assert.NotContains(t, h.Snapshot(), "SMITHERS › Agents")
}
```

The E2E test requires the binary to be built first (`go build -o /tmp/smithers-tui .`) and `SMITHERS_TEST_BINARY=/tmp/smithers-tui` set, or a `TestMain` that builds it.

---

### Step 7 — VHS tape for visual regression

**File**: `tests/vhs/agents_view.tape` (new)

```
Output tests/vhs/agents_view.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 800

Type "go run . --no-mcp"
Enter
Sleep 2s

# Open command palette
Ctrl+P
Sleep 500ms
Type "agents"
Sleep 300ms
Enter
Sleep 1s

# Navigate agents list
Down
Sleep 300ms
Down
Sleep 300ms
Up
Sleep 500ms

# Escape back
Escape
Sleep 1s
```

The tape should be run with `vhs tests/vhs/agents_view.tape` and produces `tests/vhs/agents_view.gif`.

---

## File Plan

| File | Status | Changes |
|------|--------|---------|
| `internal/smithers/client.go` | Modify | Replace `ListAgents` stub with pure-Go detection; add `lookPath`/`statFunc` injectable fields and `withLookPath`/`withStatFunc` test options |
| `internal/smithers/types.go` | No change | `Agent` struct is already complete |
| `internal/smithers/client_test.go` | Modify | Add 6 `TestListAgents_*` unit tests using injected fakes |
| `internal/ui/views/agents.go` | Modify | Add status icons, grouping, detail pane, `Enter` handoff, `launching` state, `HandoffReturnMsg` handler |
| `internal/ui/views/agents_test.go` | Create | ~12 unit tests for view lifecycle and rendering |
| `internal/ui/util/handoff.go` | Dependency | Must exist before `Enter` handoff; implement inline fallback if not ready |
| `internal/ui/dialog/commands.go` | Verify | Confirm `/agents` command triggers `ActionOpenAgentsView` and push |
| `tests/tui/helpers_test.go` | Create | TUI E2E harness (shared across views) |
| `tests/tui/agents_e2e_test.go` | Create | Agents view navigation E2E test |
| `tests/vhs/agents_view.tape` | Create | VHS visual regression tape |

---

## Validation

### Unit tests
```bash
go test ./internal/smithers/... -run TestListAgents -v
go test ./internal/ui/views/... -run TestAgentsView -v
```

### Build check
```bash
go build ./...
go vet ./internal/smithers/... ./internal/ui/views/...
```

### E2E terminal test
```bash
go build -o /tmp/smithers-tui .
SMITHERS_TEST_BINARY=/tmp/smithers-tui go test ./tests/tui/... -run TestAgentsView -timeout 30s -v
```

### VHS recording
```bash
vhs tests/vhs/agents_view.tape
# Verify tests/vhs/agents_view.gif is non-empty
```

### Manual smoke test
1. `go run .`
2. Press `/` or `Ctrl+P` to open the command palette.
3. Type `agents`, press `Enter`.
4. Verify the "SMITHERS › Agents" header appears.
5. Verify agents are grouped into "Available" and "Not Detected" sections.
6. Verify status icons (●/○/◐) match actual system state.
7. Press `↑`/`↓` to navigate — cursor should move smoothly.
8. Press `Enter` on an available agent — TUI should suspend and launch the agent CLI.
9. Exit the agent CLI — Smithers TUI should resume on the agents view.
10. Press `r` — agent list should refresh (loading spinner then updated list).
11. Press `Esc` — should return to the chat/console view.

---

## Open Questions

1. **`HandoffToProgram` dependency**: If `eng-hijack-handoff-util` has not shipped when this ticket starts, should we implement an inline `tea.ExecProcess` call in `agents.go` (with a `LookPath` guard), or block on the dependency? Recommendation: implement the inline version with a `// TODO: migrate to HandoffToProgram` comment, to avoid blocking delivery.

2. **Version display**: Should the detail pane show the agent's version string (from `--version` flag)? This requires an additional subprocess call per agent on view open, adding latency. Recommendation: omit version from v1; add as a follow-up.

3. **`pi` agent binary name**: The `.smithers/agents.ts` defines a `PiAgent` with `provider: "openai"` and no clear binary name. Confirm whether `pi` maps to a real CLI binary or should be excluded from the manifest.

4. **Cursor position across groups**: Should pressing `↓` on the last "Available" agent jump to the first "Not Detected" entry, or stop at the boundary? Recommendation: allow continuous navigation across the boundary (standard list behavior), since users may want to inspect unavailable agents' detail pane.

5. **Auth re-check on return from handoff**: After a handoff session, the agent may have completed auth setup (e.g., user ran `claude auth login`). The plan calls for `Init()` re-run on `agentHandoffReturnMsg`. Confirm this is the desired behavior (it adds a small refresh delay on return).
