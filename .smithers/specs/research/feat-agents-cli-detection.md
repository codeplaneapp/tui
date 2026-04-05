# Research: feat-agents-cli-detection

## Ticket Summary

**ID**: feat-agents-cli-detection
**Title**: Agent CLI Detection and Listing
**Depends on**: feat-agents-browser (DONE)
**Blocked by**: nothing

The ticket's acceptance criteria are:

1. The agents list is populated dynamically via `SmithersClient.ListAgents()`.
2. Users can navigate the list using standard up/down arrow keys.
3. The name of each agent (e.g., claude-code, codex) is rendered prominently.

The ticket description additionally names three enhancement areas the plan must
address:

- **Binary version detection** — run `claude --version` (etc.) and surface the
  version string in the detail pane.
- **Auth token validation** — attempt actual token verification rather than
  relying solely on directory/env-var heuristics.
- **Status refresh on demand** — user can force a re-probe at any time.

---

## Current State (as of feat-agents-browser completion)

All three acceptance criteria above are **already satisfied**:

| Criterion | State |
|-----------|-------|
| `ListAgents()` populates the list dynamically | Done — pure-Go binary/auth probing via injectable `lookPath`/`statFunc` |
| Up/down (`↑↓` / `j k`) navigation | Done — `AgentsView.Update` handles all four key codes |
| Agent names rendered prominently | Done — bold name + binary path + status icon per row |

The view also already has:
- `r` key for on-demand refresh (`v.loading = true; return v, v.Init()`)
- Grouped layout (Available / Not Detected)
- Status icons (●/◐/○) with lipgloss colours
- Detail pane on wide terminals (≥ 100 cols)
- TUI handoff via `internal/ui/handoff` package
- Auto-refresh after handoff return (auth state may change during a session)
- Full unit-test coverage: 7 `TestListAgents_*` in `client_test.go` and ~20
  `TestAgentsView_*` in `agents_test.go`

The three gaps this ticket adds are:

1. **Version string** — `agentManifestEntry` has no `versionFlag` field; the
   `Agent` struct has no `Version string` field; the view's detail pane does
   not show a version.
2. **Auth token validation** — current auth signal is `os.Stat(~/.claude)`,
   which confirms the directory exists but does not verify the token is valid.
   No subprocess check (e.g. `claude --version` exit code, or reading the
   credentials file) is performed.
3. **Refresh keybinding discoverability** — `r` is wired and functional, but
   is not announced in the key-hint bar under every terminal width. Refresh
   also loads the full detection pipeline; there is no TTL / debounce to
   prevent hammering `r`.

---

## Binary Version Detection

### How agents advertise their version

| Agent | Flag | Notes |
|-------|------|-------|
| claude (Claude Code) | `--version` → `claude 1.x.y` | Standard POSIX |
| codex | `--version` → `codex x.y.z` | Standard |
| gemini | `--version` → `gemini x.y.z` | Standard |
| amp | `version` subcommand | Returns JSON in some versions |
| forge | `--version` or `version` | Varies |
| kimi | unknown | May not expose `--version` |

All of these respond within ~100 ms on a warm PATH. Spawning a subprocess is
safe but adds latency proportional to the number of installed agents.

### Timing concern

With six agents, worst-case sequential version probing is 6 × ~100 ms = ~600 ms
added to view open time. This is noticeable. Options:

**Option A — Sequential, during `Init`**: Simple but slow. Acceptable only if
agents are detected one by one and the view renders incrementally.

**Option B — Concurrent `sync.WaitGroup`**: Run all version subprocesses in
parallel goroutines, collect results. ~100 ms total overhead regardless of
agent count. Preferred.

**Option C — Lazy / on-select**: Only fetch version for the agent currently
under the cursor, on first selection. Minimal latency impact but creates a
"loading" flash in the detail pane on every cursor move.

**Option D — Background after initial load**: Load agents without version first
(instant, current behavior), then fire a background goroutine per available
agent to enrich with version. The detail pane shows "(loading…)" for `Version`
until the subprocess returns. Compatible with option B.

**Recommendation**: Option D — background enrichment after initial detection,
using a `versionResultMsg{agentID string, version string}` tea.Msg per agent.
The view updates incrementally as version strings arrive. No blocking.

### Parsing version output

Version strings are agent-specific. A safe heuristic:

1. Run `binary --version` with a 2-second timeout.
2. Capture stdout + stderr combined.
3. Extract the first whitespace-separated token that matches `\d+\.\d+[\.\d]*`
   using `regexp.MustCompile`.
4. On timeout or non-zero exit, store `"(unknown)"` in `Agent.Version`.

### Subprocess injection for testability

The existing `Client` already has `execFunc func(ctx context.Context,
args ...string) ([]byte, error)` for general exec injection. Version probing
needs a separate injectable because it must run per-agent binary path, not via
`smithers` CLI. Add:

```go
// runBinary runs an arbitrary binary path with args and returns stdout+stderr.
// Injectable for testing via Client.runBinaryFunc.
func (c *Client) runBinary(ctx context.Context, binary string, args ...string) ([]byte, error)
```

and a corresponding `withRunBinaryFunc` ClientOption. Tests inject a fake that
returns canned version strings without spawning real processes.

---

## Auth Token Validation

### Current signal: directory check

`os.Stat(filepath.Join(homeDir, m.authDir))` returns `HasAuth = true` if
`~/.claude` (etc.) exists. This is a reliable proxy for "user has run the login
flow" but does not verify the token is currently valid.

### What does "token valid" mean per agent?

| Agent | Auth artifact | Validation approach |
|-------|--------------|---------------------|
| claude | `~/.claude/` dir, plus `~/.claude/.credentials.json` (or similar) | Read `credentials.json`, check `expiresAt` field. No subprocess needed. |
| codex | `~/.codex/` + `OPENAI_API_KEY` env | Token format check (starts with `sk-`). Full validation requires an API call — out of scope. |
| gemini | `~/.gemini/` + `GEMINI_API_KEY` env | Same: env present = signal sufficient for v1. |
| amp | `~/.amp/` auth state | File read feasible; format unknown. |
| kimi, forge | API key env var only | Env present = signal sufficient. |

**Recommendation for v1**: Enhance only the Claude Code validator (highest
usage, most structured credentials file). For all other agents, the existing
`HasAuth` (directory) + `HasAPIKey` (env var) signals are sufficient. A future
ticket (`feat-agents-auth-status-classification`) will surface these per-agent.

### Claude credentials file

`~/.claude/.credentials.json` (or `~/.claude/credentials.json`) typically
contains:

```json
{
  "anthropicApiKey": "sk-ant-...",
  "expiresAt": "2026-12-31T00:00:00Z"
}
```

Validation logic:
1. Stat `~/.claude/.credentials.json`. If absent, `HasAuth = false`.
2. Read and JSON-decode the file.
3. If `expiresAt` is present and in the past, set `AuthExpired = true` on the
   `Agent` struct (new field).
4. If `anthropicApiKey` is non-empty, set `HasAPIKey = true` (supplements env
   check).

This is a read-only filesystem operation — no subprocess, no network call,
instant.

### New field: `AuthExpired bool`

Add `AuthExpired bool` to `internal/smithers/types.go Agent` struct. The
view's detail pane renders:

```
Auth:   ✓ (active)
```

or

```
Auth:   ✗ (expired — run `claude auth login`)
```

Only Claude Code gets this treatment in v1; other agents' `AuthExpired` is
always `false`.

---

## Status Refresh On Demand

### Current state

- `r` key sets `v.loading = true` and calls `v.Init()`, which re-runs the full
  `ListAgents` detection pipeline.
- After handoff return, `v.Init()` is called automatically.
- The `r` binding is in `ShortHelp()` so it appears in the footer.
- No debounce or TTL exists.

### Gaps to address

1. **No debounce**: Rapid `r` presses spawn multiple concurrent `ListAgents`
   calls. The view only processes the first `agentsLoadedMsg` that returns
   (subsequent ones overwrite the list). This is not a correctness issue but
   wastes CPU during version detection subprocesses.

   Fix: Track a `refreshSeq int` counter. Increment on each refresh. Each
   `agentsLoadedMsg` carries the `seq` value at dispatch time. Discard messages
   with `seq < v.refreshSeq`.

2. **Version enrichment restarts on refresh**: After a refresh triggered by
   `r`, the background version goroutines from the previous load may still be
   in flight and deliver `versionResultMsg` for the old sequence. The sequence
   guard above handles this automatically.

3. **Help hint placement**: `ShortHelp()` already includes `r → refresh`. No
   change needed there. The refresh cycle should show a brief spinner
   (reuse `"  Loading agents...\n"`) until results arrive.

---

## Agent Struct Changes

Add two new fields to `Agent` in `internal/smithers/types.go`:

```go
// Agent represents a CLI agent detected on the system.
type Agent struct {
    // ... existing fields ...
    Version     string // Resolved via --version, e.g. "1.5.3". Empty if unknown.
    AuthExpired bool   // True if credentials file indicates token has expired (Claude only).
}
```

These fields are zero-valued on agents that don't support them, which is safe
for all existing consumers (view rendering, tests).

---

## agentManifestEntry Changes

Add two optional fields:

```go
type agentManifestEntry struct {
    // ... existing fields ...
    versionFlag  string // Flag to pass for version (default "--version")
    credFile     string // Path relative to authDir, e.g. ".credentials.json"
}
```

Manifest entries that don't support version detection leave `versionFlag` empty
(skip version probe). The `credFile` field is set only for Claude Code.

---

## View Changes

### Detail pane (wide layout, ≥ 100 cols)

Add `Version` line after `Binary`:

```
Binary:  /usr/local/bin/claude
Version: 1.5.3
Status:  ● likely-subscription
Auth:    ✓ (active)
APIKey:  ✓
Roles:   coding, review, spec

[Enter] Launch TUI
```

If `Version == ""`, show `Version: (detecting…)` while background probing is
in progress, then update to the real string or `(unknown)` on completion.

If `AuthExpired == true`, show `Auth: ✗ (expired)` in red instead of the
normal green tick.

### Narrow layout

Add `Version` and `AuthExpired` rendering to `writeAgentRow`:

```
▸ Claude Code
  /usr/local/bin/claude   1.5.3
  ● likely-subscription   Auth: ✓  Key: ✗
```

Version string is shown inline after the binary path, space-permitting.

---

## Existing Tests: No Breakage

All existing `TestListAgents_*` tests inject `lookPath` and `statFunc` via
`withLookPath`/`withStatFunc`. Adding `versionFlag`/`credFile` manifest fields
and `Version`/`AuthExpired` Agent fields is additive — zero-valued in tests
that don't set them. No existing test signatures change.

All existing `TestAgentsView_*` tests construct `smithers.Agent{}` literals.
New fields are zero-valued by default; tests pass without modification.

The only test that will need updating is `TestListAgents_BinaryFound_WithAuthDir`
if we also start reading `credFile` — but only if we emit a different `Status`
when credentials are expired. For v1, `Status` does not change (it remains
`"likely-subscription"`); only `AuthExpired` is added, so no test changes.

---

## E2E and VHS Considerations

No VHS tape currently exists for the agents view. This ticket is the right
place to add one (it was deferred from `feat-agents-browser`).

The VHS tape should:
1. Launch the TUI.
2. Navigate to `/agents`.
3. Show the loaded list (at minimum one "Available" agent — the dev machine
   running the CI has `claude` installed).
4. Screenshot the loaded state.
5. Navigate down one entry.
6. Screenshot the detail pane with Version populated.
7. Press `r` to trigger a refresh.
8. Wait for reload, screenshot.
9. Escape back.

The tape must handle the case where no agents are installed (pure CI): the
"Not Detected" section should render correctly with `○` icons.

---

## Summary: What This Ticket Actually Builds

| Area | Work |
|------|------|
| `Agent.Version` field | Add to `types.go` |
| `Agent.AuthExpired` field | Add to `types.go` |
| `agentManifestEntry.versionFlag` | Add to `client.go` manifest |
| `agentManifestEntry.credFile` | Add to `client.go` (Claude only) |
| `Client.runBinaryFunc` injection field | Add to `client.go` |
| Background version enrichment goroutine | Add to `ListAgents` or new `enrichAgentVersions` |
| `versionResultMsg` Bubble Tea message | New private msg type in `agents.go` |
| Sequence guard (`refreshSeq`) | Add to `AgentsView` struct |
| Credentials file reader (Claude Code) | New helper in `client.go` |
| Detail pane: Version row | Update `renderWide` in `agents.go` |
| Narrow row: inline version | Update `writeAgentRow` in `agents.go` |
| Auth expired colour in detail pane | Update `renderWide` in `agents.go` |
| New unit tests (client) | `TestListAgents_WithVersion_*` |
| New unit tests (view) | `TestAgentsView_VersionEnrichment_*`, `TestAgentsView_RefreshSeq_*` |
| VHS tape | `tests/vhs/agents-view.tape` |
