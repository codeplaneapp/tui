# Engineering: feat-agents-cli-detection

## Metadata
- ID: feat-agents-cli-detection
- Group: Agents (agents)
- Type: feature
- Depends on: feat-agents-browser (DONE)
- Phase: P1

## Summary

Extend the Agents Browser with three capabilities on top of the already-shipped
`feat-agents-browser` foundation:

1. **Binary version detection** — run each installed agent's `--version`
   subprocess in the background after initial detection and surface the version
   string in the detail pane.
2. **Auth token validation** — for Claude Code, read `~/.claude/.credentials.json`
   and check the `expiresAt` field so the UI can distinguish a valid session from
   an expired one.
3. **Sequence-guarded refresh** — prevent stale messages from a prior refresh
   cycle from overwriting the current list when the user presses `r` rapidly.

The three acceptance criteria in the ticket (`ListAgents()` populates the list,
up/down navigation works, agent names are rendered prominently) are already met
by `feat-agents-browser`. This spec documents the enhancements that the ticket
description calls for under those criteria.

---

## Acceptance Criteria (complete set)

1. `ListAgents()` populates the agents list dynamically. *(already met)*
2. Up/down (arrow keys + `j`/`k`) navigation works. *(already met)*
3. Agent names are rendered prominently. *(already met)*
4. The detail pane shows a `Version:` row for each available agent. While the
   background version probe is in flight the row reads `(detecting…)`; after
   completion it shows the semver string or `(unknown)` on failure.
5. For Claude Code, the detail pane shows `Auth: ✓ (active)` or `Auth: ✗
   (expired)` based on the credentials file, not just directory presence.
6. Pressing `r` re-probes all agents. Rapid `r` presses do not produce
   duplicate or stale list entries.

---

## Source Context

- `internal/smithers/client.go` — `ListAgents`, `knownAgents`, manifest, injectable funcs
- `internal/smithers/types.go` — `Agent` struct, `agentManifestEntry`
- `internal/ui/views/agents.go` — `AgentsView`, `renderWide`, `writeAgentRow`
- `internal/smithers/client_test.go` — existing detection tests
- `internal/ui/views/agents_test.go` — existing view tests
- `tests/vhs/` — VHS recording pattern (reference: `tickets-list.tape`)

---

## Data Model Changes

### `internal/smithers/types.go`

Add two fields to `Agent`:

```go
Version     string // from --version probe; "" while unresolved, "(unknown)" on failure
AuthExpired bool   // true if ~/.claude/.credentials.json expiresAt is in the past
```

No JSON tags needed (these are TUI-only; no HTTP transport for agents).

### `internal/smithers/client.go`

#### `agentManifestEntry` additions

```go
type agentManifestEntry struct {
    id           string
    name         string
    command      string
    roles        []string
    authDir      string
    apiKeyEnv    string
    versionFlag  string // if empty, skip version probe (e.g. kimi has no --version)
    credFile     string // path relative to authDir for credentials JSON, e.g. ".credentials.json"
}
```

Update `knownAgents`:

```go
var knownAgents = []agentManifestEntry{
    {
        id: "claude-code", name: "Claude Code", command: "claude",
        roles: []string{"coding", "review", "spec"},
        authDir: ".claude", apiKeyEnv: "ANTHROPIC_API_KEY",
        versionFlag: "--version", credFile: ".credentials.json",
    },
    {
        id: "codex", name: "Codex", command: "codex",
        roles: []string{"coding", "implement"},
        authDir: ".codex", apiKeyEnv: "OPENAI_API_KEY",
        versionFlag: "--version",
    },
    {
        id: "gemini", name: "Gemini", command: "gemini",
        roles: []string{"coding", "research"},
        authDir: ".gemini", apiKeyEnv: "GEMINI_API_KEY",
        versionFlag: "--version",
    },
    {
        id: "kimi", name: "Kimi", command: "kimi",
        roles: []string{"research", "plan"},
        apiKeyEnv: "KIMI_API_KEY",
        // versionFlag intentionally omitted
    },
    {
        id: "amp", name: "Amp", command: "amp",
        roles: []string{"coding", "validate"},
        authDir: ".amp",
        versionFlag: "--version",
    },
    {
        id: "forge", name: "Forge", command: "forge",
        roles: []string{"coding"},
        apiKeyEnv: "FORGE_API_KEY",
        versionFlag: "--version",
    },
}
```

#### `runBinaryFunc` injection field

Add alongside existing injectable fields:

```go
// runBinaryFunc runs an arbitrary binary with args and returns combined output.
// Defaults to a real subprocess; injectable for tests.
runBinaryFunc func(ctx context.Context, binary string, args ...string) ([]byte, error)
```

Add `withRunBinaryFunc` ClientOption (unexported, for tests only). Default
implementation:

```go
func defaultRunBinary(ctx context.Context, binary string, args ...string) ([]byte, error) {
    cmd := exec.CommandContext(ctx, binary, args...)
    return cmd.CombinedOutput()
}
```

#### `EnrichAgentVersions` method

A new exported method on `Client` that accepts a slice of detected agents and
populates `Version` (and `AuthExpired` for Claude Code) in parallel. Called by
the view in a background goroutine after `ListAgents` returns:

```go
// EnrichAgentVersions populates Version (and AuthExpired for Claude Code) on
// each usable agent in the slice. It fans out one goroutine per agent and
// collects results. Non-usable agents are skipped (no binary). The enriched
// slice is returned; the input slice is not mutated.
func (c *Client) EnrichAgentVersions(ctx context.Context, agents []Agent) []Agent
```

Internal per-agent logic:

1. If `!agent.Usable` or `manifest.versionFlag == ""`, skip version probe.
2. Run `c.runBinaryFunc(ctx, agent.BinaryPath, manifest.versionFlag)` with a
   2-second timeout.
3. Extract the first token matching `\d+\.\d+[\.\d]*` from combined output.
   Store in `agent.Version`; on any failure store `"(unknown)"`.
4. If `manifest.credFile != ""` and `homeDir != ""`:
   - Build path: `filepath.Join(homeDir, manifest.authDir, manifest.credFile)`.
   - Read and JSON-decode the file. On any error, skip.
   - If decoded struct has `expiresAt` (RFC3339 string) and it is before
     `time.Now()`, set `agent.AuthExpired = true`.

Return the enriched slice.

---

## View Changes

### `internal/ui/views/agents.go`

#### New private message type

```go
// agentsEnrichedMsg carries the version-enriched agent slice.
type agentsEnrichedMsg struct {
    agents []smithers.Agent
    seq    int // matches AgentsView.refreshSeq at dispatch time
}
```

#### Sequence guard

Add `refreshSeq int` field to `AgentsView`. Increment in any code path that
calls `v.Init()`:

```go
v.refreshSeq++
seq := v.refreshSeq
return v, tea.Batch(v.Init(), v.enrichCmd(seq))
```

The `enrichCmd(seq int) tea.Cmd` fires only after `Init()` returns agents — but
since `Init` and `enrichCmd` are separate goroutines, the enrichment must wait
on agents. A cleaner approach: fire `enrichCmd` from inside `agentsLoadedMsg`
handling, not from the key handler:

```go
case agentsLoadedMsg:
    if msg.seq != v.refreshSeq {
        return v, nil // stale
    }
    v.agents = msg.agents
    v.loading = false
    seq := v.refreshSeq
    agents := msg.agents
    client := v.client
    return v, func() tea.Msg {
        enriched := client.EnrichAgentVersions(context.Background(), agents)
        return agentsEnrichedMsg{agents: enriched, seq: seq}
    }
```

Handle `agentsEnrichedMsg` in `Update`:

```go
case agentsEnrichedMsg:
    if msg.seq != v.refreshSeq {
        return v, nil // stale enrichment
    }
    v.agents = msg.agents
    return v, nil
```

The `agentsLoadedMsg` must also carry `seq`. Update `Init()` to capture and
send the current sequence number.

#### Updated `Init()` signature

```go
func (v *AgentsView) Init() tea.Cmd {
    seq := v.refreshSeq
    client := v.client
    return func() tea.Msg {
        agents, err := client.ListAgents(context.Background())
        if err != nil {
            return agentsErrorMsg{err: err, seq: seq}
        }
        return agentsLoadedMsg{agents: agents, seq: seq}
    }
}
```

Update `agentsLoadedMsg` and `agentsErrorMsg` to carry `seq int`.

#### `renderWide` detail pane update

After the `Status:` row, insert:

```go
// Version row
versionStr := a.Version
if versionStr == "" {
    versionStr = lipgloss.NewStyle().Faint(true).Render("(detecting…)")
}
rightLines = append(rightLines, "Version: "+versionStr)

// Auth row with expiry awareness
authStr := "✗"
authDetail := ""
authStyle := lipgloss.NewStyle()
if a.HasAuth {
    if a.AuthExpired {
        authStr = "✗"
        authDetail = " (expired)"
        authStyle = authStyle.Foreground(lipgloss.Color("1")) // red
    } else {
        authStr = "✓"
        authDetail = " (active)"
        authStyle = authStyle.Foreground(lipgloss.Color("2")) // green
    }
}
rightLines = append(rightLines,
    "Auth:   "+authStyle.Render(authStr+authDetail),
)
```

#### `writeAgentRow` update (narrow layout)

After the binary path line, add an inline version when available:

```go
if detailed && agent.BinaryPath != "" {
    versionSuffix := ""
    if agent.Version != "" && agent.Version != "(unknown)" {
        versionSuffix = "   " + lipgloss.NewStyle().Faint(true).Render(agent.Version)
    }
    b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(agent.BinaryPath) + versionSuffix + "\n")
}
```

---

## Unit Tests

### `internal/smithers/client_test.go` additions

#### `TestEnrichAgentVersions_PopulatesVersion`

Inject `withRunBinaryFunc` returning `"claude 1.5.3\n"`. Call
`EnrichAgentVersions` with a usable claude-code agent. Assert
`agent.Version == "1.5.3"`.

#### `TestEnrichAgentVersions_SubprocessFailure`

Inject `withRunBinaryFunc` returning an error. Assert `agent.Version == "(unknown)"`.

#### `TestEnrichAgentVersions_SkipsUnavailableAgents`

Pass an agent with `Usable: false`. Assert `runBinaryFunc` is never called
(inject a func that calls `t.Fatal`).

#### `TestEnrichAgentVersions_SkipsAgentWithNoVersionFlag`

Pass a kimi agent (no `versionFlag`). Assert `runBinaryFunc` is never called.

#### `TestEnrichAgentVersions_AuthExpired`

Provide a temp directory as `homeDir`, write a fake
`.claude/.credentials.json` with `expiresAt` in the past. Assert
`agent.AuthExpired == true`.

#### `TestEnrichAgentVersions_AuthActive`

Same as above but `expiresAt` in the future. Assert `agent.AuthExpired == false`.

#### `TestEnrichAgentVersions_MissingCredFile`

No `.credentials.json` file. Assert `agent.AuthExpired == false`, no error.

#### `TestEnrichAgentVersions_Concurrent`

Pass all six agents, inject a `runBinaryFunc` with a 10 ms sleep, assert the
total wall time is under 200 ms (tests concurrent execution).

### `internal/ui/views/agents_test.go` additions

#### `TestAgentsView_EnrichedMsg_PopulatesVersion`

Send `agentsEnrichedMsg{agents: [{...Version: "1.2.3"}], seq: 0}`. Assert
`v.agents[0].Version == "1.2.3"`.

#### `TestAgentsView_EnrichedMsg_StaleSeqDiscarded`

Send `agentsEnrichedMsg{seq: 0}` after setting `v.refreshSeq = 1`. Assert
agents are not updated.

#### `TestAgentsView_LoadedMsg_SeqGuard`

Send `agentsLoadedMsg{seq: 0}` after setting `v.refreshSeq = 1`. Assert agents
are not updated and `v.loading` remains `true`.

#### `TestAgentsView_Refresh_IncrementsSeq`

Press `r`. Assert `v.refreshSeq` is one higher than before the keypress.

#### `TestAgentsView_View_ShowsVersionDetecting`

Seed agents with `Version == ""` (unresolved). Assert `View()` output contains
`"detecting"` on wide terminal.

#### `TestAgentsView_View_ShowsVersion`

Seed agents with `Version == "1.5.3"`. Assert `View()` output contains `"1.5.3"`.

#### `TestAgentsView_View_ShowsAuthExpired`

Seed a Claude Code agent with `HasAuth: true, AuthExpired: true`. Assert
`View()` output contains `"expired"`.

---

## VHS Tape

**File**: `tests/vhs/agents-view.tape`

Pattern follows `tests/vhs/tickets-list.tape`.

```
Output tests/vhs/output/agents-view.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-agents go run ."
Enter
Sleep 3s

# Open command palette and navigate to agents view
Ctrl+p
Sleep 500ms
Type "agents"
Sleep 500ms
Enter
Sleep 2s

# Agents list loaded
Screenshot tests/vhs/output/agents-view-loaded.png

# Navigate down to first agent
Down
Sleep 300ms

Screenshot tests/vhs/output/agents-view-selected.png

# Navigate down again
Down
Sleep 300ms

# Trigger manual refresh
Type "r"
Sleep 2s

Screenshot tests/vhs/output/agents-view-refreshed.png

# Return to chat view
Escape
Sleep 1s

Screenshot tests/vhs/output/agents-view-back.png

Ctrl+c
Sleep 1s
```

---

## File Plan

| File | Status | Changes |
|------|--------|---------|
| `internal/smithers/types.go` | Modify | Add `Version string`, `AuthExpired bool` to `Agent` |
| `internal/smithers/client.go` | Modify | Add `versionFlag`/`credFile` to `agentManifestEntry`; add `runBinaryFunc` field + `withRunBinaryFunc` option; add `EnrichAgentVersions` method; update `knownAgents` manifest |
| `internal/smithers/client_test.go` | Modify | Add 8 `TestEnrichAgentVersions_*` unit tests |
| `internal/ui/views/agents.go` | Modify | Add `refreshSeq int` field; update `agentsLoadedMsg`/`agentsErrorMsg` to carry `seq`; add `agentsEnrichedMsg`; update `Init()`, `Update()`, `renderWide()`, `writeAgentRow()` |
| `internal/ui/views/agents_test.go` | Modify | Add 8 `TestAgentsView_*` tests for enrichment and seq guard |
| `tests/vhs/agents-view.tape` | Create | New VHS recording tape |

---

## Validation

### Unit tests
```bash
go test ./internal/smithers/... -run TestEnrichAgentVersions -v
go test ./internal/ui/views/... -run TestAgentsView -v
```

### Build check
```bash
go build ./...
go vet ./internal/smithers/... ./internal/ui/views/...
```

### VHS recording
```bash
vhs tests/vhs/agents-view.tape
# Verify tests/vhs/output/agents-view-loaded.png is non-empty
```

### Manual smoke test
1. `go run .`
2. Navigate to `/agents` via command palette.
3. Wait for the list to load. Verify agents are grouped correctly.
4. Wait ~1 second. Verify `Version:` row in the detail pane (wide terminal)
   transitions from `(detecting…)` to a semver string for installed agents.
5. On a machine with `claude` installed and `~/.claude/.credentials.json`:
   verify `Auth: ✓ (active)` or `Auth: ✗ (expired)` depending on token state.
6. Press `r` multiple times rapidly. Verify the list loads cleanly once —
   no duplicate entries or flicker.
7. Press `r` once and wait. Verify the detail pane re-populates the version
   string after the fresh load.

---

## Open Questions

1. **Credentials file path for Claude Code**: The exact path may vary between
   `~/.claude/.credentials.json` and `~/.claude/credentials.json` depending on
   the installed version. The implementation should try both, or read from the
   manifest's `credFile` field which can enumerate alternatives.

2. **Version probe for `amp`**: `amp --version` may return a JSON object rather
   than a plain string in some versions. The regex extractor handles plain
   semver; if amp returns JSON we may need a secondary JSON parse path.

3. **Auth enrichment for non-Claude agents**: v1 only enriches Claude Code auth
   via `credFile`. Future ticket `feat-agents-auth-status-classification` will
   surface per-agent auth detail. Ensure `AuthExpired` field is designed for
   extension rather than Claude-only use.

4. **`homeDir` injection for tests**: The credentials file path requires a
   real or fake `homeDir`. The `EnrichAgentVersions` signature could accept an
   optional `homeDir string` parameter, or read from a `Client`-level field set
   via `WithHomeDir(dir string) ClientOption` (similar to `withStatFunc`).
   Recommend `ClientOption` for consistency with the existing injection pattern.
