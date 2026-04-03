# Engineering Spec: Prompts List View

**Ticket**: `feat-prompts-list`
**Feature**: `PROMPTS_LIST`
**Group**: Content And Prompts
**Dependencies**: `eng-prompts-api-client`, `eng-split-pane-component`

---

## Objective

Implement the `/prompts` view — a split-pane, keyboard-navigable list of Smithers MDX prompts discovered from `.smithers/prompts/`. The left pane shows a selectable list of prompts; the right pane shows the source content of the selected prompt. This delivers the first slice of the Prompt Editor & Preview feature (PRD §6.10, Design §3.9), establishing the foundational view that subsequent tickets (`feat-prompts-source-edit`, `feat-prompts-props-discovery`, `feat-prompts-live-preview`, `feat-prompts-save`) will extend.

The view mirrors the GUI's `PromptsList.tsx` (located at `smithers_tmp/gui-src/ui/PromptsList.tsx`) but scoped to read-only browsing for this ticket.

---

## Scope

### In scope

1. **New view file** `internal/ui/views/prompts.go` implementing the `views.View` interface.
2. **Split-pane layout**: Left pane = prompt list (fixed width ~30 chars), right pane = read-only source display of the selected prompt's MDX content.
3. **Data loading** via `smithers.Client.ListPrompts()` (provided by dependency `eng-prompts-api-client`).
4. **Layout component usage**: Consume the shared split-pane component from `internal/ui/components/splitpane.go` (provided by dependency `eng-split-pane-component`).
5. **Navigation wiring**: Register the view under `dialog.ActionOpenPromptsView`, accessible via command palette `/prompts`.
6. **Terminal E2E test** for navigating to the prompts list and verifying content renders.
7. **VHS happy-path recording** demonstrating the prompts list workflow.

### Out of scope

- Editing prompt source (`feat-prompts-source-edit`)
- Props discovery and dynamic form inputs (`feat-prompts-props-discovery`)
- Live preview / render (`feat-prompts-live-preview`)
- Save functionality (`feat-prompts-save`)
- External editor handoff via `Ctrl+O` (`feat-prompts-external-editor-handoff`)

---

## Implementation Plan

### Slice 1: Prompt and PromptInput types in `internal/smithers/types.go`

**Prerequisite**: Part of `eng-prompts-api-client`, but documented here for context.

Add types matching the upstream `DiscoveredPrompt` from `smithers/src/cli/prompts.ts`:

```go
// Prompt represents a discovered MDX prompt from .smithers/prompts/.
// Maps to DiscoveredPrompt in smithers/src/cli/prompts.ts
type Prompt struct {
    ID        string        `json:"id"`        // Filename without .mdx
    EntryFile string        `json:"entryFile"` // Full path to .mdx file
    Source    string        `json:"source"`    // Full MDX source content
    Inputs    []PromptInput `json:"inputs"`    // Extracted props.* references
}

// PromptInput represents a discovered input variable in a prompt.
type PromptInput struct {
    Name         string `json:"name"`
    Type         string `json:"type"`         // Always "string" upstream
    DefaultValue string `json:"defaultValue"` // Always "" upstream
}
```

**Upstream contract**: `GET /prompt/list` returns `{ prompts: Prompt[] }`.

### Slice 2: `ListPrompts()` client method in `internal/smithers/client.go`

**Prerequisite**: Part of `eng-prompts-api-client`, but documented here for context.

Follow the established multi-tier transport pattern (see `ListTickets`, `ListCrons`):

```go
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
    // 1. Try HTTP: GET /prompt/list
    if c.isServerAvailable() {
        var resp struct {
            Prompts []Prompt `json:"prompts"`
        }
        err := c.httpGetJSON(ctx, "/prompt/list", &resp)
        if err == nil {
            return resp.Prompts, nil
        }
    }
    // 2. Fall back to exec: smithers prompt list --format json
    out, err := c.execSmithers(ctx, "prompt", "list", "--format", "json")
    if err != nil {
        return nil, err
    }
    return parsePromptsJSON(out)
}
```

Note: The HTTP envelope for prompts wraps the array inside `{ prompts: [...] }`, unlike tickets which return a bare array. The `httpGetJSON` helper decodes the `data` field from the `apiEnvelope`, so the `resp` struct must have a `Prompts` field to match the inner shape. This matches the GUI transport at `gui-src/ui/api/transport.ts:122-125`.

### Slice 3: `PromptsView` in `internal/ui/views/prompts.go`

Create a new file following the exact patterns established by `TicketsView` (`internal/ui/views/tickets.go`) and `ApprovalsView` (`internal/ui/views/approvals.go`).

**Struct definition:**

```go
type PromptsView struct {
    client   *smithers.Client
    prompts  []smithers.Prompt
    cursor   int
    width    int
    height   int
    loading  bool
    err      error
}
```

**Private message types** (matching the convention in tickets.go):

```go
type promptsLoadedMsg struct {
    prompts []smithers.Prompt
}

type promptsErrorMsg struct {
    err error
}
```

**Init**: Async-loads prompts via `client.ListPrompts(ctx)`, returns a `tea.Cmd` that sends `promptsLoadedMsg` or `promptsErrorMsg`.

**Update**: Handles:
- `promptsLoadedMsg` / `promptsErrorMsg` — set state, clear loading
- `tea.WindowSizeMsg` — track width/height for responsive rendering
- `tea.KeyPressMsg`:
  - `esc` / `alt+esc` → return `PopViewMsg{}`
  - `up` / `k` → cursor up
  - `down` / `j` → cursor down
  - `r` → reload (set loading=true, return Init())
  - `enter` → no-op for now (future: focus source pane for editing)

**View (rendering)**: Two-column layout using the split-pane component.

- **Header**: `"SMITHERS › Prompts"` left-aligned, `"[Esc] Back"` right-aligned (matches tickets/approvals pattern).
- **Left pane** (~30 char fixed width):
  - Each prompt rendered as `▸ {prompt.ID}` (selected) or `  {prompt.ID}` (unselected).
  - Below the ID, a faint line showing input count: e.g., `  2 inputs: lang, focus`.
  - Vertical scroll if list exceeds available height.
- **Right pane** (remaining width):
  - Section header: `"Source"` (bold).
  - Full MDX source text of `prompts[cursor].Source`, line-wrapped to fit pane width.
  - If no prompt selected or list empty, show `"Select a prompt to view source"`.
- **Responsive fallback**: If terminal width < 80, fall back to list-only mode (no split pane), matching the pattern in `ApprovalsView.renderListCompact()`.

**ShortHelp**: `[]string{"[↑↓] Navigate", "[r] Refresh", "[Esc] Back"}`

**Name**: `"prompts"`

**Compile-time check**: `var _ View = (*PromptsView)(nil)`

### Slice 4: Wire `ActionOpenPromptsView` into dialog actions and UI model

**File: `internal/ui/dialog/actions.go`** — Add:

```go
// ActionOpenPromptsView is a message to navigate to the prompts view.
ActionOpenPromptsView struct{}
```

alongside the existing `ActionOpenAgentsView`, `ActionOpenTicketsView`, `ActionOpenApprovalsView`.

**File: `internal/ui/dialog/commands.go`** — Register the `/prompts` command in the command palette. Add a new entry in the command list that emits `ActionOpenPromptsView`. Follow the pattern of the existing `/agents`, `/tickets`, `/approvals` commands.

**File: `internal/ui/model/ui.go`** — Add handler in the `Update` method's action dispatch switch:

```go
case dialog.ActionOpenPromptsView:
    promptsView := views.NewPromptsView(m.smithersClient)
    cmd := m.viewRouter.Push(promptsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

This mirrors the existing `ActionOpenTicketsView` handler at approximately line 1449 of `ui.go`.

### Slice 5: Terminal E2E test

**File: `internal/e2e/prompts_list_test.go`**

Modeled on the existing `chat_domain_system_prompt_test.go` harness and the upstream `@microsoft/tui-test` pattern from `smithers/tests/tui.e2e.test.ts`.

```go
func TestPromptsListView_TUI(t *testing.T) {
    if os.Getenv("SMITHERS_TUI_E2E") == "" {
        t.Skip("set SMITHERS_TUI_E2E=1 to run TUI E2E tests")
    }

    // 1. Setup: create temp config dir with smithers-tui.json
    //    pointing to a .smithers/prompts/ dir with 2-3 test .mdx files.
    // 2. Launch TUI via launchTUI(t)
    // 3. WaitForText("CRUSH", 15s) to confirm startup
    // 4. SendKeys("/prompts\n") to open prompts view via command palette
    // 5. WaitForText("SMITHERS › Prompts", 5s) to confirm view loaded
    // 6. WaitForText("<test-prompt-id>", 5s) to confirm prompts listed
    // 7. SendKeys("j") to navigate down
    // 8. SendKeys("k") to navigate back up
    // 9. WaitForText("Source", 5s) to confirm right pane renders
    // 10. SendKeys("escape") to pop view
    // 11. WaitForNoText("SMITHERS › Prompts", 5s) to confirm navigation back
    // 12. Terminate()
}
```

**Test fixture**: Create `.smithers/prompts/test-review.mdx` and `.smithers/prompts/test-deploy.mdx` in the temp directory with minimal MDX content containing `{props.lang}` references, so the view can parse and display inputs.

**Key assertions** (mapping to `@microsoft/tui-test` style):
- View header renders after navigation command
- Prompt IDs appear in the list
- Cursor movement works (selected prompt changes)
- Source pane shows MDX content for the selected prompt
- `Esc` pops back to the previous view

### Slice 6: VHS happy-path recording

**File: `tests/vhs/prompts-list.tape`**

```tape
Output tests/vhs/output/prompts-list.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Setup: ensure test prompts exist
Type "mkdir -p .smithers/prompts && echo '# Review\n\nReview {props.lang} code for {props.focus}' > .smithers/prompts/code-review.mdx"
Enter
Sleep 1s

# Launch TUI
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Navigate to prompts view
Type "/prompts"
Enter
Sleep 2s

# Navigate the list
Type "j"
Sleep 500ms
Type "k"
Sleep 500ms

Screenshot tests/vhs/output/prompts-list.png

# Return to chat
Type "escape"
Sleep 1s

Ctrl+c
Sleep 1s
```

---

## Validation

### Automated checks

| Check | Command | What it proves |
|-------|---------|----------------|
| Unit compilation | `go build ./internal/ui/views/` | `PromptsView` compiles and satisfies `View` interface |
| Client compilation | `go build ./internal/smithers/` | `Prompt`/`PromptInput` types and `ListPrompts` compile |
| Full build | `go build .` | All wiring (actions, commands, UI model) compiles |
| Unit tests | `go test ./internal/smithers/...` | `ListPrompts` HTTP, exec, and parse helpers work |
| E2E test | `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestPromptsListView_TUI -v -timeout 60s` | Full TUI flow: launch → `/prompts` → list renders → navigate → esc back |
| VHS recording | `vhs tests/vhs/prompts-list.tape` | Happy-path recording generates without errors |

### Terminal E2E coverage (modeled on upstream `@microsoft/tui-test` harness)

The E2E test in `internal/e2e/prompts_list_test.go` follows the same architecture as the upstream Smithers `tests/tui.e2e.test.ts` and `tests/tui-helpers.ts`:

- **Process lifecycle**: Launch TUI as subprocess, interact via stdin/stdout pipes, terminate cleanly.
- **Text assertion**: `WaitForText` / `WaitForNoText` with ANSI stripping and configurable timeouts (analogous to upstream's `waitForText` helper).
- **Keyboard interaction**: `SendKeys` for both regular text input and control sequences (analogous to upstream's `sendInput`).
- **Snapshot capability**: `Snapshot()` for debugging test failures (analogous to upstream's `getScreenshot`).
- **Environment isolation**: Temp config dirs, custom env vars, fixture data (analogous to upstream's workspace isolation pattern).

### VHS recording coverage

The `tests/vhs/prompts-list.tape` file produces:
- `tests/vhs/output/prompts-list.gif` — animated recording of the full flow
- `tests/vhs/output/prompts-list.png` — static screenshot of the prompts list view

This supplements the E2E test with a visual regression artifact.

### Manual verification

1. `go run . ` → type `/prompts` in chat → verify prompts list appears with correct header.
2. With `.smithers/prompts/` containing `.mdx` files, verify all prompts appear in left pane.
3. Arrow keys move cursor; selected prompt's MDX source appears in right pane.
4. `Esc` returns to chat view.
5. Empty state: remove all `.mdx` files → verify "No prompts found." message.
6. Error state: configure invalid API URL with no fallback → verify error message renders gracefully.
7. Narrow terminal (<80 cols): verify compact/list-only mode renders without layout breakage.

---

## Risks

### 1. Dependency sequencing: `eng-prompts-api-client` not yet landed

The `ListPrompts()` client method and `Prompt`/`PromptInput` types are prerequisites. If this dependency hasn't been merged, the view cannot load real data. **Mitigation**: Implement `PromptsView` with stub data first (hardcoded prompts slice), then swap to `client.ListPrompts()` once the API client lands. The view code and the client code are cleanly separated.

### 2. Dependency sequencing: `eng-split-pane-component` not yet landed

The split-pane component (`internal/ui/components/splitpane.go`) is a prerequisite for the two-column layout. **Mitigation**: Implement the split using inline `lipgloss.JoinHorizontal` initially (matching the approach in `ApprovalsView.View()`), then refactor to use the shared component once it lands.

### 3. Prompt list response envelope mismatch

The upstream `GET /prompt/list` wraps prompts in `{ prompts: [...] }` inside the standard `{ ok, data }` envelope (see `transport.ts:122-125`). This differs from other list endpoints like `GET /ticket/list` which return a bare array in `data`. The `ListPrompts()` client method must handle this nested shape. **Mitigation**: Use a struct wrapper `struct { Prompts []Prompt }` when decoding, as shown in Slice 2.

### 4. Prompts are file-backed, not database-backed

Unlike approvals, scores, or crons, prompts live on disk in `.smithers/prompts/`. There is no SQLite table for prompts — the middle transport tier (direct SQLite) is unavailable for this data type. **Mitigation**: The client falls through HTTP → exec only (skipping the SQLite tier), matching how `ListTickets` works since tickets are also file-backed.

### 5. Large MDX source rendering in constrained terminal

Prompt source files can be arbitrarily long. Rendering the full source in the right pane without scrolling support will clip content. **Mitigation**: For this ticket, truncate displayed source to the available height with a `"... (truncated)"` indicator. A proper viewport with scroll support can be added in `feat-prompts-source-edit`.

### 6. No `ActionOpenPromptsView` in upstream Crush

Crush has no concept of a prompts view. The action type, command palette entry, and UI model handler are all net-new code with no upstream equivalent to reference. **Mitigation**: Follow the exact pattern of `ActionOpenTicketsView` / `ActionOpenAgentsView` / `ActionOpenApprovalsView` — the three existing Smithers view actions provide a clear template.

### 7. MCP prompts name collision

Crush already has `mcpPrompts` (MCP protocol prompt templates from connected servers) in `internal/ui/model/ui.go` and `dialog/commands.go`. The Smithers "prompts" feature is conceptually different — these are `.mdx` prompt files from the Smithers project, not MCP protocol prompts. **Mitigation**: Use distinct naming throughout: `PromptsView` (Smithers prompts view), `ActionOpenPromptsView` (navigation action), and keep `mcpPrompts` untouched. The `/prompts` command palette entry should be labeled "Smithers Prompts" or "Prompt Templates" to disambiguate from the existing "MCP Prompts" radio tab in the commands dialog.
