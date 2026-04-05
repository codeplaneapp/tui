# Implementation Plan: feat-agents-cli-detection

## Goal

Extend the already-shipping Agents Browser with binary version detection,
Claude Code auth-token expiry validation, and a sequence-guarded refresh cycle.
The three ticket acceptance criteria (dynamic list, navigation, prominent
names) are already met by `feat-agents-browser`; this plan delivers the three
additional capabilities named in the ticket description.

---

## Steps

### Step 1 — Data model: new Agent fields

**File**: `internal/smithers/types.go`

Add two fields after the existing `Roles` field:

```go
Version     string // from --version probe; empty while unresolved, "(unknown)" on failure
AuthExpired bool   // true if Claude Code credentials file indicates token is expired
```

No JSON tags — these fields are TUI-only and not serialized over HTTP.

---

### Step 2 — Manifest additions

**File**: `internal/smithers/client.go`

Extend `agentManifestEntry` with two optional fields:

```go
type agentManifestEntry struct {
    id          string
    name        string
    command     string
    roles       []string
    authDir     string
    apiKeyEnv   string
    versionFlag string // e.g. "--version"; empty = skip version probe
    credFile    string // relative to authDir, e.g. ".credentials.json" (Claude only)
}
```

Update `knownAgents` to populate these fields. Claude Code gets both
`versionFlag: "--version"` and `credFile: ".credentials.json"`. Kimi gets
neither (no `--version` support, no structured credentials file). All other
agents get `versionFlag: "--version"` only.

---

### Step 3 — `runBinaryFunc` injection field

**File**: `internal/smithers/client.go`

Add to `Client` struct:

```go
runBinaryFunc func(ctx context.Context, binary string, args ...string) ([]byte, error)
```

Add `withRunBinaryFunc` unexported `ClientOption` (for tests). Set the default
in `NewClient`:

```go
c.runBinaryFunc = func(ctx context.Context, binary string, args ...string) ([]byte, error) {
    return exec.CommandContext(ctx, binary, args...).CombinedOutput()
}
```

---

### Step 4 — `EnrichAgentVersions` method

**File**: `internal/smithers/client.go`

New exported method. Accepts a slice of `Agent` values (as returned by
`ListAgents`), fans out one goroutine per usable agent with a `versionFlag`,
and returns an enriched copy of the slice.

```go
// EnrichAgentVersions populates Version and AuthExpired on usable agents.
// It runs version subprocesses concurrently with a 2-second per-agent timeout.
// The input slice is not mutated; a new slice is returned.
func (c *Client) EnrichAgentVersions(ctx context.Context, agents []Agent) []Agent
```

Per-agent enrichment logic (inside a goroutine):

1. Look up manifest entry by `agent.ID`. If not found or
   `manifest.versionFlag == ""`, skip version probe.
2. Run `c.runBinaryFunc(ctxTimeout, agent.BinaryPath, manifest.versionFlag)`.
3. Extract the first `\d+\.\d+[\.\d]*` token from output.
   Set `agent.Version` to the match, or `"(unknown)"` on any failure.
4. If `manifest.credFile != ""`:
   - Compute `credPath = filepath.Join(homeDir, manifest.authDir, manifest.credFile)`.
   - Read and JSON-decode into `struct{ ExpiresAt string \`json:"expiresAt"\` }{}`.
   - If `ExpiresAt` parses as RFC3339 and is before `time.Now()`, set
     `agent.AuthExpired = true`.
   - On any read/parse error, leave `AuthExpired = false`.

Collect all goroutine results into a result slice, then merge back into a copy
of the input slice (match by `agent.ID`). Return the merged slice.

`homeDir` comes from the `Client`'s injectable `homeDirFunc` field (see Step 5).

---

### Step 5 — `homeDirFunc` injection field

**File**: `internal/smithers/client.go`

The credentials file path requires `os.UserHomeDir()`. To keep the pattern
consistent with `lookPath` and `statFunc`, inject it:

```go
homeDirFunc func() (string, error)
```

Add `withHomeDirFunc` unexported `ClientOption`. Default to `os.UserHomeDir`.
Tests override with a temp directory.

---

### Step 6 — Sequence-guarded refresh in the view

**File**: `internal/ui/views/agents.go`

#### 6a — Update message types

Add `seq int` to `agentsLoadedMsg` and `agentsErrorMsg`:

```go
type agentsLoadedMsg struct {
    agents []smithers.Agent
    seq    int
}

type agentsErrorMsg struct {
    err error
    seq int
}
```

Add new message type:

```go
type agentsEnrichedMsg struct {
    agents []smithers.Agent
    seq    int
}
```

#### 6b — Add `refreshSeq int` to `AgentsView`

```go
type AgentsView struct {
    // ... existing fields ...
    refreshSeq int
}
```

#### 6c — Update `Init()`

Capture `refreshSeq` in the closure so the message carries the correct sequence:

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

#### 6d — Update `Update()` for `agentsLoadedMsg`

Discard stale messages; fire enrichment as a follow-on command:

```go
case agentsLoadedMsg:
    if msg.seq != v.refreshSeq {
        return v, nil
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

#### 6e — Handle `agentsEnrichedMsg`

```go
case agentsEnrichedMsg:
    if msg.seq != v.refreshSeq {
        return v, nil
    }
    v.agents = msg.agents
    return v, nil
```

#### 6f — Increment `refreshSeq` on refresh (`r` key and `HandoffMsg` return)

In the `r` key handler:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
    v.loading = true
    v.refreshSeq++
    return v, v.Init()
```

In the `handoff.HandoffMsg` handler (already re-calls `v.Init()`):

```go
case handoff.HandoffMsg:
    v.launching = false
    v.launchingName = ""
    tag, _ := msg.Tag.(string)
    if msg.Result.Err != nil {
        v.err = fmt.Errorf("launch %s: %w", tag, msg.Result.Err)
    }
    v.loading = true
    v.refreshSeq++
    return v, v.Init()
```

---

### Step 7 — View rendering updates

**File**: `internal/ui/views/agents.go`

#### 7a — `renderWide` detail pane

After the `Status:` row, insert `Version:` and update `Auth:`:

```go
// Version row
versionStr := a.Version
if versionStr == "" {
    versionStr = lipgloss.NewStyle().Faint(true).Render("(detecting…)")
}
rightLines = append(rightLines, "Version: "+versionStr)

// Auth row (replaces old simple "Auth: ✓/✗")
authLabel := "✗"
authDetail := ""
authColour := lipgloss.Color("1") // red default for false
if a.HasAuth {
    if a.AuthExpired {
        authLabel = "✗"
        authDetail = " (expired)"
        authColour = lipgloss.Color("1")
    } else {
        authLabel = "✓"
        authDetail = " (active)"
        authColour = lipgloss.Color("2")
    }
}
rightLines = append(rightLines,
    "Auth:   "+lipgloss.NewStyle().Foreground(authColour).Render(authLabel+authDetail),
)
```

#### 7b — `writeAgentRow` narrow layout

Append version inline after binary path when available:

```go
if detailed && agent.BinaryPath != "" {
    line := lipgloss.NewStyle().Faint(true).Render(agent.BinaryPath)
    if agent.Version != "" && agent.Version != "(unknown)" {
        line += "   " + lipgloss.NewStyle().Faint(true).Render(agent.Version)
    }
    b.WriteString("  " + line + "\n")
}
```

---

### Step 8 — Unit tests: client

**File**: `internal/smithers/client_test.go`

Add a `newEnrichClient` helper that wires `withRunBinaryFunc` and
`withHomeDirFunc`:

```go
func newEnrichClient(
    rbf func(ctx context.Context, binary string, args ...string) ([]byte, error),
    hdf func() (string, error),
) *Client {
    return NewClient(withRunBinaryFunc(rbf), withHomeDirFunc(hdf))
}
```

Tests:

- `TestEnrichAgentVersions_PopulatesVersion`: inject `rbf` returning `"claude 1.5.3\n"`; assert `agent.Version == "1.5.3"`.
- `TestEnrichAgentVersions_SubprocessError`: inject `rbf` returning error; assert `agent.Version == "(unknown)"`.
- `TestEnrichAgentVersions_SkipsUnavailableAgent`: `Usable: false` agent; inject `rbf` that calls `t.Fatal`; no panic.
- `TestEnrichAgentVersions_SkipsNoVersionFlag`: kimi agent; inject `rbf` that calls `t.Fatal`; no panic.
- `TestEnrichAgentVersions_AuthExpired`: write `{"expiresAt":"2020-01-01T00:00:00Z"}` to temp dir; assert `AuthExpired == true`.
- `TestEnrichAgentVersions_AuthActive`: write `{"expiresAt":"2099-01-01T00:00:00Z"}`; assert `AuthExpired == false`.
- `TestEnrichAgentVersions_MissingCredFile`: no file written; assert `AuthExpired == false`, no error.
- `TestEnrichAgentVersions_Concurrent`: inject `rbf` with 10 ms sleep, pass 6 agents, assert wall time < 200 ms.

---

### Step 9 — Unit tests: view

**File**: `internal/ui/views/agents_test.go`

- `TestAgentsView_EnrichedMsg_PopulatesVersion`: send `agentsEnrichedMsg{seq:0, agents:[{Version:"1.2.3"}]}`; assert `v.agents[0].Version == "1.2.3"`.
- `TestAgentsView_EnrichedMsg_StaleDiscarded`: set `v.refreshSeq = 1`, send `agentsEnrichedMsg{seq:0}`; assert agents unchanged.
- `TestAgentsView_LoadedMsg_StaleDiscarded`: set `v.refreshSeq = 1`, send `agentsLoadedMsg{seq:0}`; assert `v.loading` still `true`.
- `TestAgentsView_Refresh_IncrementsSeq`: press `r`; assert `v.refreshSeq == 1`.
- `TestAgentsView_View_ShowsVersionDetecting`: seed agent with `Version == ""`; assert `View()` contains `"detecting"`.
- `TestAgentsView_View_ShowsVersion`: seed agent with `Version == "1.5.3"`; assert `View()` contains `"1.5.3"`.
- `TestAgentsView_View_ShowsAuthExpired`: seed claude-code with `HasAuth: true, AuthExpired: true`; assert `View()` contains `"expired"`.
- `TestAgentsView_HandoffReturn_IncrementsSeq`: send `handoff.HandoffMsg{}`; assert `v.refreshSeq` increased.

---

### Step 10 — VHS tape

**File**: `tests/vhs/agents-view.tape`

```
Output tests/vhs/output/agents-view.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-agents go run ."
Enter
Sleep 3s

Ctrl+p
Sleep 500ms
Type "agents"
Sleep 500ms
Enter
Sleep 2s

Screenshot tests/vhs/output/agents-view-loaded.png

Down
Sleep 300ms
Down
Sleep 300ms

Screenshot tests/vhs/output/agents-view-selected.png

# Wait for version detection background enrichment
Sleep 2s

Screenshot tests/vhs/output/agents-view-enriched.png

# Manual refresh
Type "r"
Sleep 3s

Screenshot tests/vhs/output/agents-view-refreshed.png

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
| `internal/smithers/client.go` | Modify | Add `versionFlag`/`credFile` to manifest; add `runBinaryFunc`/`homeDirFunc` injectable fields; add `EnrichAgentVersions`; update `knownAgents` |
| `internal/smithers/client_test.go` | Modify | Add 8 `TestEnrichAgentVersions_*` tests |
| `internal/ui/views/agents.go` | Modify | Add `refreshSeq`; update `agentsLoadedMsg`/`agentsErrorMsg`/`agentsEnrichedMsg`; update `Init`, `Update`, `renderWide`, `writeAgentRow` |
| `internal/ui/views/agents_test.go` | Modify | Add 8 `TestAgentsView_*` tests for enrichment and seq guard |
| `tests/vhs/agents-view.tape` | Create | New VHS recording |

No new files except the VHS tape. All changes are additive — no existing
function signatures change, no existing tests break.

---

## Validation

### Unit tests
```bash
go test ./internal/smithers/... -run TestEnrichAgentVersions -v
go test ./internal/smithers/... -run TestListAgents -v
go test ./internal/ui/views/... -run TestAgentsView -v
```

### Build + vet
```bash
go build ./...
go vet ./internal/smithers/... ./internal/ui/views/...
```

### VHS recording
```bash
vhs tests/vhs/agents-view.tape
# Inspect tests/vhs/output/agents-view-enriched.png: version string should be visible
```

### Manual smoke test

1. `go run .` — navigate to `/agents`.
2. Immediately after load: detail pane shows `Version: (detecting…)` for
   installed agents.
3. After ~1 second: `Version:` row updates to a semver string.
4. On a machine with an expired Claude token: detail pane shows
   `Auth: ✗ (expired)`.
5. Press `r` three times fast — list loads exactly once; no duplicate rows.
6. Press `Esc` — returns to chat view.

---

## Open Questions

1. **Credential file path variants**: `~/.claude/.credentials.json` vs.
   `~/.claude/credentials.json`. Try both; use whichever exists. Store as a
   `[]string` slice in the manifest if multiple paths need checking.

2. **`amp` version format**: Some `amp --version` outputs are JSON. Add a
   secondary JSON parse path: if no semver match in plain text, try
   `json.Unmarshal` and look for `"version"` key.

3. **`homeDirFunc` injection surface**: Decide whether `withHomeDirFunc` is
   unexported (test-only) or exported (`WithHomeDir`) for callers that embed
   the client in a container with a custom home directory. Recommend unexported
   for now (consistent with `withLookPath`, `withStatFunc`).
