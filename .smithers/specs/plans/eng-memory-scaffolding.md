## Goal

Deliver a read-only `MemoryView` accessible via the command palette (`/memory` → "Memory Browser"), wired into the view router with full loading/error/empty/populated states, cursor-navigable fact list showing namespace, key, truncated JSON value preview, and relative age. Follows the established `AgentsView` / `TicketsView` / `ApprovalsView` pattern exactly.

This corresponds to `MEMORY_BROWSER` and `MEMORY_FACT_LIST` in the engineering spec at `.smithers/specs/engineering/eng-memory-scaffolding.md`.

---

## Steps

### Step 1: Add `ListAllMemoryFacts` to the Smithers client

**File**: `/Users/williamcory/crush/internal/smithers/client.go`

The current `ListMemoryFacts(ctx, namespace, workflowPath)` uses `WHERE namespace = ?`. Passing `""` returns zero rows for real Smithers data (namespaces follow patterns like `workflow:code-review`, `global`, `agent:claude-code`). Add a new method that lists all facts across all namespaces.

Insert immediately after `RecallMemory` (after line 430):

```go
// ListAllMemoryFacts lists all memory facts across all namespaces.
// Routes: SQLite → exec smithers memory list --all.
func (c *Client) ListAllMemoryFacts(ctx context.Context) ([]MemoryFact, error) {
	// 1. Try direct SQLite (preferred — no dedicated HTTP endpoint)
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms, ttl_ms
			FROM _smithers_memory_facts ORDER BY updated_at_ms DESC`)
		if err != nil {
			return nil, err
		}
		return scanMemoryFacts(rows)
	}

	// 2. Fall back to exec
	out, err := c.execSmithers(ctx, "memory", "list", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseMemoryFactsJSON(out)
}
```

**Verification**: `go build ./internal/smithers/...` passes. `go vet ./internal/smithers/...` is clean.

---

### Step 2: Add `ActionOpenMemoryView` action type

**File**: `/Users/williamcory/crush/internal/ui/dialog/actions.go`

In the Smithers actions block at lines 91–97, add after `ActionOpenApprovalsView`:

```go
// ActionOpenMemoryView is a message to navigate to the memory browser view.
ActionOpenMemoryView struct{}
```

The block after the change:
```go
// ActionOpenAgentsView is a message to navigate to the agents view.
ActionOpenAgentsView struct{}
// ActionOpenTicketsView is a message to navigate to the tickets view.
ActionOpenTicketsView struct{}
// ActionOpenApprovalsView is a message to navigate to the approvals view.
ActionOpenApprovalsView struct{}
// ActionOpenMemoryView is a message to navigate to the memory browser view.
ActionOpenMemoryView struct{}
```

**Verification**: `go build ./internal/ui/dialog/...` passes.

---

### Step 3: Add "Memory Browser" entry to the command palette

**File**: `/Users/williamcory/crush/internal/ui/dialog/commands.go`

In the Smithers entries block at lines 528–533, add `"memory"` before `"quit"`:

```go
commands = append(commands,
    NewCommandItem(c.com.Styles, "agents",    "Agents",         "",       ActionOpenAgentsView{}),
    NewCommandItem(c.com.Styles, "approvals", "Approvals",      "",       ActionOpenApprovalsView{}),
    NewCommandItem(c.com.Styles, "tickets",   "Tickets",        "",       ActionOpenTicketsView{}),
    NewCommandItem(c.com.Styles, "memory",    "Memory Browser", "",       ActionOpenMemoryView{}),
    NewCommandItem(c.com.Styles, "quit",      "Quit",           "ctrl+c", tea.QuitMsg{}),
)
```

No dedicated keybinding is assigned for this ticket — memory is a detail/utility view per the PRD navigation model.

**Verification**: Build passes. Open command palette with `/` or `Ctrl+P`, type `"mem"` — "Memory Browser" appears in filtered results.

---

### Step 4: Add router handler in the UI model

**File**: `/Users/williamcory/crush/internal/ui/model/ui.go`

Add a `case` immediately after `dialog.ActionOpenApprovalsView` (around line 1472):

```go
case dialog.ActionOpenMemoryView:
	m.dialog.CloseDialog(dialog.CommandsID)
	memoryView := views.NewMemoryView(m.smithersClient)
	cmd := m.viewRouter.Push(memoryView)
	m.setState(uiSmithersView, uiFocusMain)
	cmds = append(cmds, cmd)
```

This is identical in structure to the three existing action handlers at lines 1458–1477. No additional wiring is needed — `views.PopViewMsg` already pops the router in the existing handler at line 1479.

**Verification**: `go build ./...` passes. Selecting "Memory Browser" from the command palette pushes the view and sets `uiSmithersView` state.

---

### Step 5: Build `MemoryView` in `internal/ui/views/memory.go`

**File**: `/Users/williamcory/crush/internal/ui/views/memory.go` (new)

```go
package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*MemoryView)(nil)

type memoryLoadedMsg struct {
	facts []smithers.MemoryFact
}

type memoryErrorMsg struct {
	err error
}

// MemoryView displays a navigable list of memory facts across all namespaces.
type MemoryView struct {
	client  *smithers.Client
	facts   []smithers.MemoryFact
	cursor  int
	width   int
	height  int
	loading bool
	err     error
}

// NewMemoryView creates a new memory browser view.
func NewMemoryView(client *smithers.Client) *MemoryView {
	return &MemoryView{
		client:  client,
		loading: true,
	}
}

// Init loads memory facts from the client.
func (v *MemoryView) Init() tea.Cmd {
	return func() tea.Msg {
		facts, err := v.client.ListAllMemoryFacts(context.Background())
		if err != nil {
			return memoryErrorMsg{err: err}
		}
		return memoryLoadedMsg{facts: facts}
	}
}

// Update handles messages for the memory browser view.
func (v *MemoryView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case memoryLoadedMsg:
		v.facts = msg.facts
		v.loading = false
		return v, nil

	case memoryErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.facts)-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// No-op placeholder — future: detail panel / semantic recall.
		}
	}
	return v, nil
}

// View renders the memory fact list.
func (v *MemoryView) View() string {
	var b strings.Builder

	// Header line with right-aligned [Esc] Back hint.
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Memory")
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine)
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading memory facts...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.facts) == 0 {
		b.WriteString("  No memory facts found.\n")
		return b.String()
	}

	faint := lipgloss.NewStyle().Faint(true)

	for i, fact := range v.facts {
		cursor := "  "
		nsStyle := faint
		keyStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			keyStyle = keyStyle.Bold(true)
		}

		// Line 1: [cursor] [namespace] / [key]
		b.WriteString(cursor + nsStyle.Render(fact.Namespace+" / ") + keyStyle.Render(fact.Key) + "\n")

		// Line 2: truncated value preview + relative age.
		preview := factValuePreview(fact.ValueJSON, 60)
		age := factAge(fact.UpdatedAtMs)

		previewStr := "    " + faint.Render(preview)
		ageStr := faint.Render(age)

		if v.width > 0 {
			// Right-align age within available width.
			previewVisualLen := lipgloss.Width(previewStr)
			ageVisualLen := lipgloss.Width(ageStr)
			gap := v.width - previewVisualLen - ageVisualLen - 2
			if gap > 0 {
				b.WriteString(previewStr + strings.Repeat(" ", gap) + ageStr + "\n")
			} else {
				b.WriteString(previewStr + "  " + ageStr + "\n")
			}
		} else {
			b.WriteString(previewStr + "  " + ageStr + "\n")
		}

		if i < len(v.facts)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// Name returns the view name.
func (v *MemoryView) Name() string {
	return "memory"
}

// ShortHelp returns keybinding hints for the help bar.
func (v *MemoryView) ShortHelp() []string {
	return []string{"[Enter] View", "[r] Refresh", "[Esc] Back"}
}

// --- Helpers ---

// factValuePreview returns a display-friendly preview of a JSON value string.
// If the value is a JSON string literal (begins and ends with '"'), the outer
// quotes are stripped for readability. The result is truncated to maxLen runes.
func factValuePreview(valueJSON string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 60
	}
	s := strings.TrimSpace(valueJSON)

	// Strip outer quotes from JSON string literals.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// factAge returns a human-readable relative age string for a Unix millisecond timestamp.
// Examples: "45s ago", "3m ago", "2h ago", "5d ago".
func factAge(updatedAtMs int64) string {
	if updatedAtMs <= 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(updatedAtMs))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

**Verification**: `go build ./internal/ui/views/...` passes. `go vet ./internal/ui/views/...` is clean.

---

### Step 6: Unit tests for `MemoryView` and helpers

**File**: `/Users/williamcory/crush/internal/ui/views/memory_test.go` (new)

Use `package views_test` consistent with `router_test.go`. Import `testify/assert` and `testify/require` consistent with smithers client tests.

Test cases:

**1. `TestMemoryView_Init`** — `NewMemoryView(smithers.NewClient())`.`Init()` returns a non-nil `tea.Cmd`.

**2. `TestMemoryView_LoadedMsg`** — Send `memoryLoadedMsg{facts: sampleFacts}` (2 facts) via `Update()`. Assert `v.loading == false`, `len(v.facts) == 2`, `v.err == nil`. Assert `View()` contains both fact keys.

**3. `TestMemoryView_ErrorMsg`** — Send `memoryErrorMsg{err: errors.New("db unavailable")}`. Assert `v.loading == false`, `v.err != nil`. Assert `View()` contains `"Error:"` and `"db unavailable"`.

**4. `TestMemoryView_EmptyState`** — Send `memoryLoadedMsg{facts: nil}`. Assert `View()` contains `"No memory facts found."`.

**5. `TestMemoryView_CursorNavigation`** — Load 3 facts. Send `tea.KeyPressMsg{Code: tea.KeyCodeDown}` twice via Update. Assert `v.cursor == 2`. Send up once. Assert `v.cursor == 1`. Send up three times (past boundary). Assert `v.cursor == 0` (clamps at 0).

**6. `TestMemoryView_CursorClampsAtBottom`** — Load 2 facts. Send down three times. Assert `v.cursor == 1` (clamps at `len-1`).

**7. `TestMemoryView_EscPopView`** — Load facts. Send `tea.KeyPressMsg{Code: tea.KeyCodeEscape}`. Assert returned `tea.Cmd` is non-nil. Execute the cmd, assert returned `tea.Msg` is `PopViewMsg{}`.

**8. `TestMemoryView_Refresh`** — Load facts. Send `r` key. Assert `v.loading == true` and returned `tea.Cmd` is non-nil.

**9. `TestMemoryView_Name`** — Assert `Name() == "memory"`.

**10. `TestMemoryView_WindowSize`** — Send `tea.WindowSizeMsg{Width: 120, Height: 40}`. Assert `v.width == 120`, `v.height == 40`.

**11. `TestFactValuePreview_ShortString`** — `factValuePreview(`"`hello`"`, 60)` → `"hello"` (outer quotes stripped, no truncation).

**12. `TestFactValuePreview_LongObject`** — `factValuePreview(`{"key": "value", "other": "data"}`...` beyond 60 chars, 60)` → result ends with `"..."` and has length ≤ 63 runes.

**13. `TestFactValuePreview_Truncation`** — input of exactly 63 runes (non-string JSON object), `maxLen=60` → result is 60 runes including the `...` suffix.

**14. `TestFactValuePreview_Empty`** — empty string input → `""` returned.

**15. `TestFactAge_Seconds`** — timestamp `time.Now().Add(-30*time.Second).UnixMilli()` → `"30s ago"`.

**16. `TestFactAge_Minutes`** — `time.Now().Add(-5*time.Minute).UnixMilli()` → `"5m ago"`.

**17. `TestFactAge_Hours`** — `time.Now().Add(-3*time.Hour).UnixMilli()` → `"3h ago"`.

**18. `TestFactAge_Days`** — `time.Now().Add(-48*time.Hour).UnixMilli()` → `"2d ago"`.

**19. `TestFactAge_Zero`** — `factAge(0)` → `""`.

**Sample fact helper** (shared across tests):

```go
func sampleFacts() []smithers.MemoryFact {
    now := time.Now().UnixMilli()
    return []smithers.MemoryFact{
        {Namespace: "workflow:code-review", Key: "reviewer-preference",
            ValueJSON: `{"style":"thorough"}`, UpdatedAtMs: now - 120_000},
        {Namespace: "global",               Key: "last-deploy-sha",
            ValueJSON: `"a1b2c3d"`,          UpdatedAtMs: now - 3_600_000},
        {Namespace: "agent:claude-code",    Key: "task-context",
            ValueJSON: `{"task":"review"}`,   UpdatedAtMs: now - 7_200_000},
    }
}
```

Key message-construction pattern for key events (see `router_test.go` `TestView` pattern):

```go
// Send a key press message directly to Update.
downKey := tea.KeyPressMsg{Code: tea.KeyCodeDown}
view, _ = view.Update(downKey)
```

**Verification**: `go test ./internal/ui/views/ -run TestMemory -v` → all cases pass. `go test ./internal/ui/views/ -run TestFact -v` → all helper tests pass.

---

### Step 7: Add `ListAllMemoryFacts` unit tests

**File**: existing smithers client test file, or a new `memory_test.go` in `internal/smithers/`

Since `internal/smithers/` already has separate test files per domain (`systems_test.go`, `tickets_test.go`, etc.), add `memory_test.go`:

**File**: `/Users/williamcory/crush/internal/smithers/memory_test.go` (new)

Test cases:

**1. `TestListAllMemoryFacts_SQLite`** — Use the `withExecFunc` pattern to confirm exec is NOT called when SQLite is available. Open an in-memory SQLite DB (`:memory:`), create the `_smithers_memory_facts` table, insert 2 rows, construct a client pointing at the test DB, call `ListAllMemoryFacts`. Assert 2 results, correct field values.

Construct in-memory DB:
```go
db, err := sql.Open("sqlite", ":memory:")
require.NoError(t, err)
defer db.Close()
_, err = db.Exec(`CREATE TABLE _smithers_memory_facts (
    namespace TEXT, key TEXT, value_json TEXT, schema_sig TEXT,
    created_at_ms INTEGER, updated_at_ms INTEGER, ttl_ms INTEGER)`)
require.NoError(t, err)
_, err = db.Exec(`INSERT INTO _smithers_memory_facts VALUES
    ('global','test-key','{"x":1}','',1000,2000,NULL)`)
require.NoError(t, err)
// Inject db directly via struct field (internal package test can access unexported field).
c := NewClient()
c.db = db
facts, err := c.ListAllMemoryFacts(context.Background())
```

**2. `TestListAllMemoryFacts_Exec`** — `newExecClient` returning `json.Marshal([]MemoryFact{{Namespace:"global",Key:"k1",ValueJSON:`"v"`,UpdatedAtMs:1000}})`. Assert exec args contain `"memory"`, `"list"`, `"--all"`, `"--format"`, `"json"`. Assert 1 result with correct fields.

**3. `TestListAllMemoryFacts_ExecError`** — `newExecClient` returning `nil, errors.New("not found")`. Assert `ListAllMemoryFacts` returns an error.

**4. `TestListAllMemoryFacts_EmptyResult`** — exec returns `[]byte("[]")`. Assert empty slice returned, no error.

**Verification**: `go test ./internal/smithers/ -run TestListAllMemoryFacts -v` → all pass.

---

### Step 8: Build the Go terminal E2E harness (if not already present)

**File**: `/Users/williamcory/crush/tests/tui_helpers_test.go`

If the approvals scaffolding ticket has landed and this file exists, skip to Step 9. If not, build the harness:

```go
package tests

import (
    "bytes"
    "io"
    "os"
    "os/exec"
    "regexp"
    "strings"
    "testing"
    "time"
)

// TUIHarness manages a subprocess TUI for terminal E2E testing.
type TUIHarness struct {
    t      *testing.T
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    buf    bytes.Buffer
    done   chan struct{}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHF]`)

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
    return ansiRe.ReplaceAllString(s, "")
}

// LaunchTUI starts the smithers-tui binary (or `go run .`) as a subprocess.
// env is appended to the current environment.
func LaunchTUI(t *testing.T, env []string) *TUIHarness {
    t.Helper()
    binary := os.Getenv("SMITHERS_TUI_BINARY")
    var cmd *exec.Cmd
    if binary == "" {
        // Fall back to go run in the repo root.
        cmd = exec.Command("go", "run", ".")
        cmd.Dir = ".."
    } else {
        cmd = exec.Command(binary)
    }
    cmd.Env = append(os.Environ(), env...)
    cmd.Env = append(cmd.Env, "TERM=xterm-256color")

    stdin, err := cmd.StdinPipe()
    if err != nil {
        t.Fatalf("stdin pipe: %v", err)
    }

    h := &TUIHarness{t: t, cmd: cmd, stdin: stdin, done: make(chan struct{})}
    cmd.Stdout = &h.buf
    cmd.Stderr = &h.buf

    if err := cmd.Start(); err != nil {
        t.Fatalf("start TUI: %v", err)
    }
    go func() {
        _ = cmd.Wait()
        close(h.done)
    }()
    t.Cleanup(h.Stop)
    return h
}

// SendKeys writes a string to the TUI's stdin.
func (h *TUIHarness) SendKeys(s string) {
    h.t.Helper()
    _, _ = io.WriteString(h.stdin, s)
}

// WaitForText polls the terminal buffer until text appears or timeout expires.
func (h *TUIHarness) WaitForText(text string, timeout time.Duration) bool {
    h.t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if strings.Contains(stripANSI(h.buf.String()), text) {
            return true
        }
        time.Sleep(100 * time.Millisecond)
    }
    return false
}

// Stop terminates the subprocess.
func (h *TUIHarness) Stop() {
    _ = h.cmd.Process.Kill()
    <-h.done
}

// Snapshot returns the current terminal buffer with ANSI stripped.
func (h *TUIHarness) Snapshot() string {
    return stripANSI(h.buf.String())
}
```

**Verification**: File compiles. `go test ./tests/ -list '.*'` exits 0.

---

### Step 9: Create the memory test fixture SQLite DB

**File**: `/Users/williamcory/crush/tests/fixtures/memory-test.db`

Create this fixture with a shell script or SQL initialization. The `tests/fixtures/` directory must exist:

```bash
mkdir -p tests/fixtures
sqlite3 tests/fixtures/memory-test.db <<'SQL'
CREATE TABLE IF NOT EXISTS _smithers_memory_facts (
    namespace     TEXT    NOT NULL,
    key           TEXT    NOT NULL,
    value_json    TEXT    NOT NULL,
    schema_sig    TEXT    NOT NULL DEFAULT '',
    created_at_ms INTEGER NOT NULL DEFAULT 0,
    updated_at_ms INTEGER NOT NULL DEFAULT 0,
    ttl_ms        INTEGER,
    PRIMARY KEY (namespace, key)
);

-- Seed two facts in different namespaces for E2E assertions.
INSERT OR REPLACE INTO _smithers_memory_facts
    (namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms)
VALUES
    ('workflow:code-review', 'test-fact-1', '{"style":"thorough"}',         '', 1000, strftime('%s','now','subsec') * 1000),
    ('global',               'test-fact-2', '"a1b2c3d4e5f6"',               '', 1000, strftime('%s','now','subsec') * 1000 - 3600000);
SQL
```

The fixture is committed to the repo. The E2E tests reference it via `SMITHERS_DB=tests/fixtures/memory-test.db` (or the client's `WithDBPath` option injected via env). The VHS tape uses the same fixture via `SMITHERS_DB=tests/fixtures/memory-test.db`.

**Verification**: `sqlite3 tests/fixtures/memory-test.db "SELECT count(*) FROM _smithers_memory_facts"` → `2`.

---

### Step 10: Terminal E2E test

**File**: `/Users/williamcory/crush/tests/tui_memory_e2e_test.go` (new)

```go
package tests

import (
    "testing"
    "time"
)

func TestMemoryBrowserE2E(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    h := LaunchTUI(t, []string{
        "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures",
        "CRUSH_GLOBAL_DATA=/tmp/crush-memory-e2e",
        "SMITHERS_DB=tests/fixtures/memory-test.db",
    })

    // Wait for TUI to be ready (landing or chat view).
    if !h.WaitForText("SMITHERS", 10*time.Second) {
        t.Fatalf("TUI did not start within 10s\nSnapshot:\n%s", h.Snapshot())
    }

    // Open command palette and navigate to memory browser.
    h.SendKeys("/")
    if !h.WaitForText("memory", 5*time.Second) {
        t.Fatalf("command palette did not show memory entry\nSnapshot:\n%s", h.Snapshot())
    }
    h.SendKeys("memory\r")

    // Assert memory view header.
    if !h.WaitForText("SMITHERS › Memory", 5*time.Second) {
        t.Fatalf("memory view header not found\nSnapshot:\n%s", h.Snapshot())
    }

    // Assert fact list contains seeded keys.
    if !h.WaitForText("test-fact-1", 5*time.Second) {
        t.Fatalf("test-fact-1 not found in memory view\nSnapshot:\n%s", h.Snapshot())
    }
    if !h.WaitForText("test-fact-2", 5*time.Second) {
        t.Fatalf("test-fact-2 not found in memory view\nSnapshot:\n%s", h.Snapshot())
    }

    // Navigate down.
    h.SendKeys("j")
    time.Sleep(200 * time.Millisecond)

    // Navigate up.
    h.SendKeys("k")
    time.Sleep(200 * time.Millisecond)

    // Refresh.
    h.SendKeys("r")
    if !h.WaitForText("Loading memory facts...", 3*time.Second) {
        // Loading may be instantaneous for a small fixture — that is acceptable.
        t.Log("loading state not observed (may have been instant)")
    }
    if !h.WaitForText("test-fact-1", 5*time.Second) {
        t.Fatalf("test-fact-1 not found after refresh\nSnapshot:\n%s", h.Snapshot())
    }

    // Press Esc — memory view should pop.
    h.SendKeys("\x1b")
    time.Sleep(300 * time.Millisecond)
    snap := h.Snapshot()
    if strings.Contains(snap, "SMITHERS › Memory") {
        t.Errorf("memory view still visible after Esc\nSnapshot:\n%s", snap)
    }
}
```

**Verification**: `go test ./tests/ -run TestMemoryBrowserE2E -timeout 30s -v` → passes (requires `go run .` to compile within timeout; use `SMITHERS_TUI_BINARY` env var pointing to a pre-built binary to speed up).

---

### Step 11: VHS happy-path tape

**File**: `/Users/williamcory/crush/tests/vhs/memory-browser.tape` (new)

```tape
# Memory Browser — happy-path smoke test
Output tests/vhs/output/memory-browser.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch with test fixture DB.
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-memory-vhs SMITHERS_DB=tests/fixtures/memory-test.db go run ."
Enter
Sleep 3s

# Open command palette and navigate to memory browser.
Type "/"
Sleep 500ms
Type "memory"
Sleep 500ms
Enter
Sleep 2s

# Screenshot the memory browser list.
Screenshot tests/vhs/output/memory-browser-list.png

# Navigate through facts.
Down
Sleep 300ms
Down
Sleep 300ms

# Refresh.
Type "r"
Sleep 1s

# Go back.
Escape
Sleep 1s

Ctrl+c
Sleep 500ms
```

**Verification**: `vhs tests/vhs/memory-browser.tape` exits 0. `tests/vhs/output/memory-browser.gif` and `tests/vhs/output/memory-browser-list.png` are generated.

---

## Validation Checklist

| Check | Command | Expected |
|-------|---------|----------|
| Build | `go build ./...` | Clean, zero errors |
| Vet | `go vet ./...` | No issues |
| View unit tests | `go test ./internal/ui/views/ -run TestMemory -v` | All 10 cases pass |
| Helper unit tests | `go test ./internal/ui/views/ -run TestFact -v` | All helper tests pass |
| Client unit tests | `go test ./internal/smithers/ -run TestListAllMemoryFacts -v` | All 4 cases pass |
| Full suite | `go test ./...` | No regressions |
| E2E test | `go test ./tests/ -run TestMemoryBrowserE2E -timeout 60s -v` | Subprocess navigates, asserts facts, exits cleanly |
| VHS tape | `vhs tests/vhs/memory-browser.tape` | Exit 0, GIF and PNG generated |

---

## Implementation Order

1. Step 1 (client method) → Step 2 (action type) → Step 3 (command palette) → Step 4 (router handler) — these are small mechanical changes that unblock compilation and navigation plumbing.
2. Step 5 (MemoryView) — core deliverable; do after plumbing so the whole chain compiles.
3. Step 6 (view unit tests) — establish baseline; run after Step 5.
4. Step 7 (client unit tests) — run after Step 1 is done.
5. Step 8 (E2E harness) — only needed if not already present from the approvals scaffolding ticket.
6. Step 9 (fixture DB) — needed for Steps 10 and 11.
7. Step 10 (E2E test) — requires Steps 5, 8, 9.
8. Step 11 (VHS tape) — requires Steps 5, 9.

Steps 1–7 have no external dependencies and can be done in a single sitting. Steps 8–11 require a compilable binary, so ensure `go build ./...` is green before starting them.

---

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| `ListAllMemoryFacts` exec path (`--all` flag) may not exist in current smithers CLI | Acceptable for scaffolding: the SQLite path is primary; the exec fallback produces an error that the view renders gracefully. Document the flag requirement in a `// TODO` comment. |
| E2E harness already exists under a different path from the approvals ticket | Check `tests/` before writing; reuse/extend rather than duplicate the helper. |
| VHS binary not available in CI | Gate the VHS test: `if which vhs > /dev/null 2>&1; then vhs ...; fi`. The E2E test is the authoritative automated test; VHS is supplementary. |
| Loading state is invisible for small local fixture (SQLite reads in < 1ms) | E2E test notes this as acceptable; the VHS tape's `Sleep 1s` after `r` gives enough time to observe the state on slow CI runners. |
| `factAge` is time-sensitive in tests | Use `time.Now()` relative timestamps in test helpers, not hardcoded Unix ms values, so tests are not time-dependent. |
