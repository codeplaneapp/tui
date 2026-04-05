# Implementation Plan: feat-prompts-list

## Goal

Implement the `/prompts` view — a split-pane, keyboard-navigable browser for Smithers MDX prompt templates discovered from `.smithers/prompts/`. The left pane shows a selectable list of prompts; the right pane shows the raw MDX source and extracted props for the selected prompt. Both dependency blockers (`eng-prompts-api-client`, `eng-split-pane-component`) are already resolved in the codebase, making this a straightforward implementation ticket.

---

## Steps

### Step 1 — Create `internal/ui/views/prompts.go`

**File**: `internal/ui/views/prompts.go` (new)

#### 1a — Struct and message types

```go
package views

import (
    "context"
    "fmt"
    "strings"

    "charm.land/bubbles/v2/key"
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    "github.com/charmbracelet/crush/internal/smithers"
)

var _ View = (*PromptsView)(nil)

type promptsLoadedMsg struct {
    prompts []smithers.Prompt
}

type promptsErrorMsg struct {
    err error
}

type promptSourceLoadedMsg struct {
    prompt *smithers.Prompt
}

type promptSourceErrorMsg struct {
    id  string
    err error
}

type PromptsView struct {
    client        *smithers.Client
    prompts       []smithers.Prompt
    loadedSources map[string]*smithers.Prompt
    cursor        int
    width         int
    height        int
    loading       bool       // list loading
    loadingSource bool       // source loading for selected prompt
    err           error      // list load error
    sourceErr     error      // source load error for selected prompt
}

func NewPromptsView(client *smithers.Client) *PromptsView {
    return &PromptsView{
        client:        client,
        loading:       true,
        loadedSources: make(map[string]*smithers.Prompt),
    }
}
```

The `loadedSources` map caches `GetPrompt` results for the lifetime of the view to avoid redundant reads when navigating back to a previously-viewed prompt.

#### 1b — Init

```go
func (v *PromptsView) Init() tea.Cmd {
    return func() tea.Msg {
        prompts, err := v.client.ListPrompts(context.Background())
        if err != nil {
            return promptsErrorMsg{err: err}
        }
        return promptsLoadedMsg{prompts: prompts}
    }
}
```

`ListPrompts` returns `Prompt{ID, EntryFile}` entries only — Source and Props are empty at this stage. Source is loaded lazily on cursor movement.

#### 1c — Update

```go
func (v *PromptsView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case promptsLoadedMsg:
        v.prompts = msg.prompts
        v.loading = false
        // Immediately load source for the first prompt.
        return v, v.loadSelectedSource()

    case promptsErrorMsg:
        v.err = msg.err
        v.loading = false
        return v, nil

    case promptSourceLoadedMsg:
        v.loadedSources[msg.prompt.ID] = msg.prompt
        v.loadingSource = false
        v.sourceErr = nil
        return v, nil

    case promptSourceErrorMsg:
        v.sourceErr = msg.err
        v.loadingSource = false
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
                return v, v.loadSelectedSource()
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
            if v.cursor < len(v.prompts)-1 {
                v.cursor++
                return v, v.loadSelectedSource()
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
            v.loading = true
            v.loadedSources = make(map[string]*smithers.Prompt)
            v.sourceErr = nil
            return v, v.Init()

        case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
            // No-op for this ticket.
            // Future: feat-prompts-source-edit will focus the source pane.
        }
    }
    return v, nil
}
```

The `loadSelectedSource()` call after `up`/`down` is a no-op if the source is already cached.

#### 1d — loadSelectedSource helper

```go
func (v *PromptsView) loadSelectedSource() tea.Cmd {
    if v.cursor < 0 || v.cursor >= len(v.prompts) {
        return nil
    }
    id := v.prompts[v.cursor].ID
    if _, ok := v.loadedSources[id]; ok {
        return nil // already cached
    }
    v.loadingSource = true
    return func() tea.Msg {
        p, err := v.client.GetPrompt(context.Background(), id)
        if err != nil {
            return promptSourceErrorMsg{id: id, err: err}
        }
        return promptSourceLoadedMsg{prompt: p}
    }
}
```

#### 1e — View (rendering)

```go
func (v *PromptsView) View() string {
    var b strings.Builder

    // Header
    header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Prompts")
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
        b.WriteString("  Loading prompts...\n")
        return b.String()
    }

    if v.err != nil {
        b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
        return b.String()
    }

    if len(v.prompts) == 0 {
        b.WriteString("  No prompts found.\n")
        return b.String()
    }

    // Split-pane layout.
    listWidth := 30
    dividerWidth := 3
    detailWidth := v.width - listWidth - dividerWidth
    if v.width < 80 || detailWidth < 20 {
        b.WriteString(v.renderListCompact())
        return b.String()
    }

    listContent := v.renderList(listWidth)
    detailContent := v.renderDetail(detailWidth)

    divider := lipgloss.NewStyle().Faint(true).Render(" │ ")

    listLines := strings.Split(listContent, "\n")
    detailLines := strings.Split(detailContent, "\n")

    maxLines := len(listLines)
    if len(detailLines) > maxLines {
        maxLines = len(detailLines)
    }

    availHeight := v.height - 3
    if availHeight > 0 && maxLines > availHeight {
        maxLines = availHeight
    }

    for i := 0; i < maxLines; i++ {
        left := ""
        if i < len(listLines) {
            left = listLines[i]
        }
        right := ""
        if i < len(detailLines) {
            right = detailLines[i]
        }
        left = padRight(left, listWidth)
        b.WriteString(left + divider + right + "\n")
    }

    return b.String()
}
```

#### 1f — renderList helper

```go
func (v *PromptsView) renderList(width int) string {
    var b strings.Builder
    faint := lipgloss.NewStyle().Faint(true)

    for i, prompt := range v.prompts {
        cursor := "  "
        nameStyle := lipgloss.NewStyle()
        if i == v.cursor {
            cursor = "▸ "
            nameStyle = nameStyle.Bold(true)
        }

        // Truncate ID if needed.
        id := prompt.ID
        if len(id) > width-4 {
            id = id[:width-7] + "..."
        }
        b.WriteString(cursor + nameStyle.Render(id) + "\n")

        // Show input count below the ID (use cached source if available).
        if loaded, ok := v.loadedSources[prompt.ID]; ok && len(loaded.Props) > 0 {
            var names []string
            for _, p := range loaded.Props {
                names = append(names, p.Name)
            }
            count := fmt.Sprintf("%d input", len(loaded.Props))
            if len(loaded.Props) != 1 {
                count += "s"
            }
            detail := count + ": " + strings.Join(names, ", ")
            if len(detail) > width-2 {
                detail = detail[:width-5] + "..."
            }
            b.WriteString("  " + faint.Render(detail) + "\n")
        }

        if i < len(v.prompts)-1 {
            b.WriteString("\n")
        }
    }
    return b.String()
}
```

#### 1g — renderDetail helper

```go
func (v *PromptsView) renderDetail(width int) string {
    if v.cursor < 0 || v.cursor >= len(v.prompts) {
        return ""
    }

    var b strings.Builder
    titleStyle := lipgloss.NewStyle().Bold(true)
    labelStyle := lipgloss.NewStyle().Faint(true)

    if v.loadingSource {
        b.WriteString(labelStyle.Render("Loading source..."))
        return b.String()
    }

    if v.sourceErr != nil {
        b.WriteString(fmt.Sprintf("Error loading source: %v", v.sourceErr))
        return b.String()
    }

    selected := v.prompts[v.cursor]
    loaded, ok := v.loadedSources[selected.ID]
    if !ok {
        b.WriteString(labelStyle.Render("Select a prompt to view source"))
        return b.String()
    }

    // Source section
    b.WriteString(titleStyle.Render("Source") + "\n")
    b.WriteString(labelStyle.Render(loaded.EntryFile) + "\n\n")

    // Render source lines wrapped to pane width.
    sourceLines := strings.Split(loaded.Source, "\n")
    // Reserve lines for props section below (estimate 3 + len(Props)*1).
    reservedForProps := 0
    if len(loaded.Props) > 0 {
        reservedForProps = 3 + len(loaded.Props)
    }
    maxSourceLines := v.height - 5 - reservedForProps
    if maxSourceLines < 5 {
        maxSourceLines = 5
    }

    printed := 0
    truncated := false
    for _, line := range sourceLines {
        if printed >= maxSourceLines {
            truncated = true
            break
        }
        // Word-wrap long lines.
        for len(line) > width {
            b.WriteString(line[:width] + "\n")
            line = line[width:]
            printed++
            if printed >= maxSourceLines {
                truncated = true
                break
            }
        }
        if truncated {
            break
        }
        b.WriteString(line + "\n")
        printed++
    }
    if truncated {
        b.WriteString(labelStyle.Render("... (truncated)") + "\n")
    }

    // Props / Inputs section
    if len(loaded.Props) > 0 {
        b.WriteString("\n" + titleStyle.Render("Inputs") + "\n")
        for _, prop := range loaded.Props {
            var defaultStr string
            if prop.DefaultValue != nil {
                defaultStr = fmt.Sprintf(" (default: %q)", *prop.DefaultValue)
            }
            b.WriteString(labelStyle.Render("  • ") + prop.Name +
                labelStyle.Render(" : "+prop.Type+defaultStr) + "\n")
        }
    }

    return b.String()
}
```

#### 1h — renderListCompact (narrow terminal fallback)

```go
func (v *PromptsView) renderListCompact() string {
    var b strings.Builder
    faint := lipgloss.NewStyle().Faint(true)

    for i, prompt := range v.prompts {
        cursor := "  "
        nameStyle := lipgloss.NewStyle()
        if i == v.cursor {
            cursor = "▸ "
            nameStyle = nameStyle.Bold(true)
        }
        b.WriteString(cursor + nameStyle.Render(prompt.ID) + "\n")

        if i == v.cursor {
            if v.loadingSource {
                b.WriteString(faint.Render("    Loading...") + "\n")
            } else if loaded, ok := v.loadedSources[prompt.ID]; ok {
                if len(loaded.Props) > 0 {
                    var names []string
                    for _, p := range loaded.Props {
                        names = append(names, p.Name)
                    }
                    b.WriteString(faint.Render("    Inputs: "+strings.Join(names, ", ")) + "\n")
                }
                // Show first 3 lines of source
                lines := strings.Split(loaded.Source, "\n")
                for j, line := range lines {
                    if j >= 3 {
                        b.WriteString(faint.Render("    ...") + "\n")
                        break
                    }
                    if strings.TrimSpace(line) != "" {
                        b.WriteString(faint.Render("    "+line) + "\n")
                    }
                }
            }
        }

        if i < len(v.prompts)-1 {
            b.WriteString("\n")
        }
    }
    return b.String()
}
```

#### 1i — Name and ShortHelp

```go
func (v *PromptsView) Name() string {
    return "prompts"
}

func (v *PromptsView) ShortHelp() []string {
    return []string{"[↑↓] Navigate", "[r] Refresh", "[Esc] Back"}
}
```

---

### Step 2 — Wire `ActionOpenPromptsView` into dialog/actions.go

**File**: `internal/ui/dialog/actions.go`

Add `ActionOpenPromptsView struct{}` to the existing action type block alongside the three existing Smithers view actions:

```go
// Before (lines 88-96):
type (
    // ...existing actions...
    ActionOpenAgentsView   struct{}
    ActionOpenTicketsView  struct{}
    ActionOpenApprovalsView struct{}
)

// After:
type (
    // ...existing actions...
    ActionOpenAgentsView    struct{}
    ActionOpenTicketsView   struct{}
    ActionOpenApprovalsView struct{}
    ActionOpenPromptsView   struct{}
)
```

No other changes to actions.go are needed.

---

### Step 3 — Register `/prompts` in the command palette (dialog/commands.go)

**File**: `internal/ui/dialog/commands.go`

In the `defaultCommands()` function, add the prompts command to the same block as agents/approvals/tickets (currently around line 527):

```go
// Before:
commands = append(commands,
    NewCommandItem(c.com.Styles, "agents",    "Agents",    "", ActionOpenAgentsView{}),
    NewCommandItem(c.com.Styles, "approvals", "Approvals", "", ActionOpenApprovalsView{}),
    NewCommandItem(c.com.Styles, "tickets",   "Tickets",   "", ActionOpenTicketsView{}),
    NewCommandItem(c.com.Styles, "quit",      "Quit",      "ctrl+c", tea.QuitMsg{}),
)

// After:
commands = append(commands,
    NewCommandItem(c.com.Styles, "agents",           "Agents",           "", ActionOpenAgentsView{}),
    NewCommandItem(c.com.Styles, "approvals",        "Approvals",        "", ActionOpenApprovalsView{}),
    NewCommandItem(c.com.Styles, "tickets",          "Tickets",          "", ActionOpenTicketsView{}),
    NewCommandItem(c.com.Styles, "smithers_prompts", "Prompt Templates", "", ActionOpenPromptsView{}),
    NewCommandItem(c.com.Styles, "quit",             "Quit",             "ctrl+c", tea.QuitMsg{}),
)
```

The ID `"smithers_prompts"` (not `"prompts"`) avoids any collision with the existing `MCPPrompts` command type. The label `"Prompt Templates"` disambiguates from the "MCP" prompts tab in the commands dialog.

---

### Step 4 — Handle `ActionOpenPromptsView` in the UI model (model/ui.go)

**File**: `internal/ui/model/ui.go`

Add the case immediately after the existing `ActionOpenApprovalsView` handler (around line 1472):

```go
case dialog.ActionOpenApprovalsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    approvalsView := views.NewApprovalsView(m.smithersClient)
    cmd := m.viewRouter.Push(approvalsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)

// Add this block:
case dialog.ActionOpenPromptsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    promptsView := views.NewPromptsView(m.smithersClient)
    cmd := m.viewRouter.Push(promptsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

No other UI model changes are needed. `m.smithersClient` is already instantiated and available.

---

### Step 5 — Unit tests for PromptsView (internal/ui/views/prompts_test.go)

**File**: `internal/ui/views/prompts_test.go` (new)

Follow the pattern established by `internal/smithers/client_test.go`. Use a mock/stub `smithers.Client` seeded with known data.

```go
package views_test

import (
    "testing"
    tea "charm.land/bubbletea/v2"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/charmbracelet/crush/internal/ui/views"
)
```

Test cases:

| Test | What it verifies |
|------|-----------------|
| `TestPromptsView_Init_SetsLoading` | `NewPromptsView(client)` starts with `loading == true`; `Init()` returns a non-nil `tea.Cmd` |
| `TestPromptsView_LoadedMsg_PopulatesPrompts` | Sending `promptsLoadedMsg{prompts: testPrompts}` sets `loading = false` and populates `prompts` slice |
| `TestPromptsView_ErrorMsg_SetsErr` | Sending `promptsErrorMsg{err: someErr}` sets `loading = false` and `err != nil` |
| `TestPromptsView_CursorDown_InBounds` | `down`/`j` key increments cursor; stops at last item |
| `TestPromptsView_CursorUp_InBounds` | `up`/`k` key decrements cursor; does not go below 0 |
| `TestPromptsView_Esc_ReturnsPopViewMsg` | `esc` key triggers `tea.Cmd` that produces `PopViewMsg{}` |
| `TestPromptsView_R_Refresh` | `r` key sets `loading = true` and returns `Init()` command; clears `loadedSources` |
| `TestPromptsView_SourceLoadedMsg_CachesSource` | `promptSourceLoadedMsg{prompt: p}` populates `loadedSources[p.ID]` and clears `loadingSource` |
| `TestPromptsView_SourceErrorMsg_SetsSourceErr` | `promptSourceErrorMsg{err: e}` sets `sourceErr != nil` and clears `loadingSource` |
| `TestPromptsView_View_HeaderText` | `View()` output contains `"SMITHERS › Prompts"` |
| `TestPromptsView_View_PromptIDInList` | With prompts loaded, `View()` contains each prompt ID |
| `TestPromptsView_View_SelectedCursor` | The cursor prompt ID is preceded by `"▸"` in the output |
| `TestPromptsView_View_SourcePane` | With source loaded for cursor prompt, right pane shows `"Source"` header and source text |
| `TestPromptsView_View_InputsSection` | With props loaded, right pane shows `"Inputs"` and each prop name |
| `TestPromptsView_View_EmptyState` | With empty prompts slice, shows `"No prompts found."` |
| `TestPromptsView_View_LoadingState` | With `loading = true`, shows `"Loading prompts..."` |
| `TestPromptsView_View_NarrowTerminal` | `width = 60` triggers compact mode (no split divider `│`) |
| `TestPromptsView_View_WideTerminal` | `width = 120` shows split divider `│` in output |
| `TestPromptsView_Name` | `Name()` returns `"prompts"` |
| `TestPromptsView_ShortHelp` | `ShortHelp()` contains navigate, refresh, and back hints |
| `TestPromptsView_InterfaceCompliance` | Compile-time check `var _ views.View = (*PromptsView)(nil)` |

To test the view without a running server, use the exec-override pattern from the existing test harness:

```go
// Seed a client that returns test prompts via exec fallback.
client := smithers.NewClient(
    smithers.WithExecFuncForTest(func(ctx context.Context, args ...string) ([]byte, error) {
        // Return test fixture JSON based on args[0] ("prompt"), args[1] ("list"/"get")
    }),
)
```

Alternatively, since `listPromptsFromFS` and `getPromptFromFS` use `os.Getwd()`, a simpler approach is to write `.smithers/prompts/*.mdx` fixtures to a temp directory and set working dir — but this is more complex for unit tests. Use the exec mock approach for unit tests and reserve filesystem access for the E2E test.

---

### Step 6 — E2E terminal test (internal/e2e/prompts_list_test.go)

**File**: `internal/e2e/prompts_list_test.go` (new)

```go
package e2e_test

import (
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestPromptsListView_TUI(t *testing.T) {
    if os.Getenv("SMITHERS_TUI_E2E") != "1" {
        t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
    }

    // Create a temp project root with fixture prompts.
    projectRoot := t.TempDir()
    promptsDir := filepath.Join(projectRoot, ".smithers", "prompts")
    require.NoError(t, os.MkdirAll(promptsDir, 0o755))

    // Write two minimal .mdx fixtures.
    require.NoError(t, os.WriteFile(
        filepath.Join(promptsDir, "test-review.mdx"),
        []byte("# Review\n\nReview {props.lang} code for {props.focus}.\n"),
        0o644,
    ))
    require.NoError(t, os.WriteFile(
        filepath.Join(promptsDir, "test-deploy.mdx"),
        []byte("# Deploy\n\nDeploy {props.service} to {props.env}.\n\nREQUIRED OUTPUT:\n{props.schema}\n"),
        0o644,
    ))

    // Create a minimal config.
    configDir := t.TempDir()
    dataDir := t.TempDir()
    writeGlobalConfig(t, configDir, `{
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

    t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
    t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

    // Launch TUI with project root as working directory.
    tui := launchTUI(t, "--cwd", projectRoot)
    defer tui.Terminate()

    // 1. Wait for startup.
    require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

    // 2. Open command palette and navigate to prompts view.
    tui.SendKeys("/")
    require.NoError(t, tui.WaitForText("Commands", 5*time.Second))
    tui.SendKeys("Prompt Templates")
    time.Sleep(300 * time.Millisecond)
    tui.SendKeys("\r")

    // 3. Verify prompts view header appears.
    require.NoError(t, tui.WaitForText("SMITHERS › Prompts", 5*time.Second))

    // 4. Verify prompt IDs appear in list.
    require.NoError(t, tui.WaitForText("test-review", 5*time.Second))

    // 5. Navigate down.
    tui.SendKeys("j")
    time.Sleep(300 * time.Millisecond)

    // 6. Verify second prompt is visible.
    require.NoError(t, tui.WaitForText("test-deploy", 3*time.Second))

    // 7. Navigate back up.
    tui.SendKeys("k")
    time.Sleep(300 * time.Millisecond)

    // 8. Verify source section renders (first prompt is selected).
    require.NoError(t, tui.WaitForText("Source", 3*time.Second))

    // 9. Verify inputs are shown (test-review has lang and focus).
    require.NoError(t, tui.WaitForText("lang", 3*time.Second))

    // 10. Escape returns to previous view.
    tui.SendKeys("\x1b")
    require.NoError(t, tui.WaitForNoText("SMITHERS › Prompts", 3*time.Second))
}
```

Note: the `launchTUI` helper currently sets `cmd.Dir = repoRoot`. This test needs to run with `projectRoot` as the working directory so that `listPromptsFromFS` finds the fixtures. Two options:

**Option A** — Add a `--cwd` flag to the TUI binary that overrides `os.Chdir` on startup. This is the cleanest approach.

**Option B** — Add an `os.Chdir(projectRoot)` call before `launchTUI` and restore it after. This is not thread-safe for parallel tests.

**Option C** — Add a `WithPromptsDir` option to `smithers.Client` and pass it via a test env var. This avoids changing the TUI binary and is isolated.

Recommendation: Option A. A `--cwd` flag is a generally useful development tool (it is already common in dev servers). The test invocation becomes `launchTUI(t, "--cwd", projectRoot)`, and the root command adds:

```go
cmd.PersistentFlags().String("cwd", "", "Change working directory before starting")
```

If Option A is too invasive for this ticket, implement Option C (add `SMITHERS_PROMPTS_DIR` env var support to `promptsDir()`) as a lower-cost alternative.

---

### Step 7 — VHS happy-path recording (tests/vhs/prompts-list.tape)

**File**: `tests/vhs/prompts-list.tape` (new)

```tape
# Prompts list view happy-path smoke recording.
Output tests/vhs/output/prompts-list.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Ensure fixture prompts exist in the local .smithers/prompts/ directory.
Type "mkdir -p .smithers/prompts"
Enter
Sleep 500ms
Type "printf '# Code Review\\n\\nReview {props.lang} code for {props.focus}.\\n' > .smithers/prompts/code-review.mdx"
Enter
Sleep 500ms
Type "printf '# Deploy\\n\\nDeploy {props.service} to {props.env}.\\n' > .smithers/prompts/deploy.mdx"
Enter
Sleep 500ms

# Launch the TUI.
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Open command palette.
Ctrl+P
Sleep 500ms

# Type to filter to "Prompt Templates".
Type "Prompt"
Sleep 300ms
Enter
Sleep 2s

# Navigate the list.
Type "j"
Sleep 500ms
Type "k"
Sleep 500ms

Screenshot tests/vhs/output/prompts-list.png

# Return to chat.
Escape
Sleep 1s

Ctrl+c
Sleep 1s
```

---

## File Plan

| File | Status | Changes |
|------|--------|---------|
| `internal/ui/views/prompts.go` | Create | Primary deliverable: `PromptsView` struct, Init/Update/View/Name/ShortHelp, lazy source loading |
| `internal/ui/views/prompts_test.go` | Create | ~20 unit tests covering view lifecycle, rendering, navigation |
| `internal/ui/dialog/actions.go` | Modify | Add `ActionOpenPromptsView struct{}` to the existing action type block |
| `internal/ui/dialog/commands.go` | Modify | Add `"smithers_prompts"` / `"Prompt Templates"` command to `defaultCommands()` |
| `internal/ui/model/ui.go` | Modify | Add `ActionOpenPromptsView` case after `ActionOpenApprovalsView` handler |
| `internal/e2e/prompts_list_test.go` | Create | E2E test: launch → navigate → list → source renders → esc back |
| `tests/vhs/prompts-list.tape` | Create | VHS recording: setup fixtures → launch → palette → list → navigate → screenshot |
| `tests/vhs/output/prompts-list.gif` | Generated | Produced by `vhs tests/vhs/prompts-list.tape` |
| `tests/vhs/output/prompts-list.png` | Generated | Produced by `vhs tests/vhs/prompts-list.tape` |

No changes needed to:
- `internal/smithers/types_prompts.go` — types are complete.
- `internal/smithers/prompts.go` — all required methods exist.
- `internal/ui/views/router.go` — wiring is correct.

---

## Validation

### Automated checks

| Check | Command | What it proves |
|-------|---------|----------------|
| Compile views | `go build ./internal/ui/views/` | `PromptsView` compiles and satisfies `View` interface |
| Compile dialog | `go build ./internal/ui/dialog/` | `ActionOpenPromptsView` compiles without conflicts |
| Full build | `go build .` | All wiring (actions, commands, UI model) compiles end-to-end |
| Unit tests | `go test ./internal/ui/views/... -run TestPromptsView -v` | View lifecycle, rendering, navigation pass |
| E2E test | `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestPromptsListView_TUI -v -timeout 60s` | Full TUI flow: launch → palette → prompts view → list → source renders → esc back |
| VHS recording | `vhs tests/vhs/prompts-list.tape` | Happy-path GIF generates without errors |
| vet | `go vet ./internal/ui/views/... ./internal/ui/dialog/... ./internal/ui/model/...` | No suspicious constructs |

### Manual smoke test

1. `go run .` → type `Prompt Templates` in command palette → press `Enter`.
2. Verify `SMITHERS › Prompts` header appears.
3. With `.smithers/prompts/*.mdx` files present, verify all prompt IDs appear in the left pane.
4. Verify the first prompt's source appears in the right pane after a brief "Loading source..." flash.
5. Press `↓` / `j` — cursor moves, source updates in right pane.
6. Verify input counts appear below the prompt ID in the left pane (e.g., `3 inputs: prompt, schema, reviewer`).
7. Press `↑` / `k` — navigates back.
8. Press `r` — list and cache reload; loading indicator reappears briefly.
9. Press `Esc` — returns to chat view; `SMITHERS › Prompts` no longer visible.
10. Narrow terminal (< 80 cols): verify compact layout renders without split divider or horizontal overflow.
11. Empty state: remove all `.mdx` files → verify `"No prompts found."` renders.
12. Error state: point client at invalid API URL with no filesystem fallback → verify error message renders.

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` or `k` | Move cursor up |
| `↓` or `j` | Move cursor down |
| `r` | Refresh prompt list and clear source cache |
| `Esc` or `Alt+Esc` | Pop view (return to chat) |
| `Enter` | No-op (reserved for `feat-prompts-source-edit`) |

Future tickets will add:
- `Ctrl+O` → external editor handoff (`feat-prompts-external-editor-handoff`)
- `e` or `Enter` → focus source pane / enter edit mode (`feat-prompts-source-edit`)

---

## External Editor Handoff (Out of Scope, Design Note)

The PRD (§6.10, §6.16) and Design doc describe `Ctrl+O` as the trigger for editing a prompt in `$EDITOR`. This is explicitly out of scope for `feat-prompts-list` (see `feat-prompts-external-editor-handoff`), but the design should be noted here so the struct is not over-engineered in this ticket.

The handoff pattern (from `ui.go:2785` and the `eng-hijack-handoff-util` plan) is:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+o"))):
    if loaded, ok := v.loadedSources[v.prompts[v.cursor].ID]; ok {
        return v, handoffToEditor(loaded.EntryFile, func(err error) tea.Msg {
            return promptEditorReturnMsg{id: loaded.ID, err: err}
        })
    }
```

On return, the view calls `GetPrompt` to reload the edited source. The `PromptsView` struct for this ticket does **not** need fields for editor state — the next ticket adds them.

---

## Open Questions

1. **`--cwd` flag vs env var for E2E test**: The E2E test needs the TUI to run from a temp project root with fixture `.mdx` files. The cleanest fix is a `--cwd` flag on the root command. Is this acceptable, or should we use an env var (`SMITHERS_PROMPTS_DIR`) instead?

2. **Prompt list ordering**: `listPromptsFromFS` returns files in `os.ReadDir` order (alphabetical by filename on most filesystems). Should the view sort by last-modified time instead? Recommendation: keep alphabetical order for this ticket; add sort options in a follow-up.

3. **`WithPromptsDir` client option**: Should `smithers.Client` expose `WithPromptsDir(path string)` to allow the E2E test (and future tests) to override the `.smithers/prompts/` scan path? This is a single-field addition to `Client`. Recommendation: add it — it improves testability at negligible cost.

4. **Source pane scrolling**: For this ticket, the source is truncated at available height with a `"... (truncated)"` indicator. Should the right pane support vertical scrolling (via a `bubbles/viewport`)? Recommendation: defer to `feat-prompts-source-edit`, which will replace the read-only source display with an editable viewport anyway.

5. **Compact mode threshold**: The split pane falls back to compact mode at `v.width < 80`. Should this match the `approvals.go` threshold exactly (it does) or be lower (e.g., 70) to allow wider source preview on narrower terminals? Recommendation: keep 80 for consistency with `ApprovalsView`.
