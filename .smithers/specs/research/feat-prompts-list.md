# Research: feat-prompts-list

## Existing Smithers API Client Surface

### Types — `internal/smithers/types_prompts.go`

The `Prompt` and `PromptProp` types are already defined and correct:

```go
type Prompt struct {
    ID        string       `json:"id"`
    EntryFile string       `json:"entryFile"`
    Source    string       `json:"source,omitempty"`
    Props     []PromptProp `json:"inputs,omitempty"`
}

type PromptProp struct {
    Name         string  `json:"name"`
    Type         string  `json:"type"`
    DefaultValue *string `json:"defaultValue,omitempty"`
}
```

Key observations:
- `Source` is marked `omitempty` — it is **not** populated by `ListPrompts`, only by `GetPrompt` or the filesystem tier.
- `Props` uses JSON key `"inputs"` (matching the upstream `DiscoveredPrompt.inputs[]` shape) not `"props"`.
- `DefaultValue` is a pointer (`*string`), not a plain string — it is nil when no default is declared.

### Transport Client — `internal/smithers/prompts.go`

Five methods are implemented with a three-tier transport (HTTP → filesystem → exec):

| Method | Transport route |
|--------|----------------|
| `ListPrompts(ctx)` | HTTP `GET /prompt/list` → `listPromptsFromFS()` → exec `prompt list --format json` |
| `GetPrompt(ctx, id)` | HTTP `GET /prompt/get/{id}` → `getPromptFromFS(id)` → exec `prompt get {id} --format json` |
| `UpdatePrompt(ctx, id, content)` | HTTP `POST /prompt/update/{id}` → `updatePromptOnFS(id, content)` → exec `prompt update {id} --source {content}` |
| `DiscoverPromptProps(ctx, id)` | HTTP `GET /prompt/props/{id}` → parse `getPromptFromFS(id)` |
| `PreviewPrompt(ctx, id, props)` | HTTP `POST /prompt/render/{id}` → `renderTemplate(source, props)` → exec `prompt render {id} --input {json}` |

Critical finding: **`ListPrompts` does not return `Source`**. The filesystem tier returns `Prompt` entries with only `ID` and `EntryFile` populated. The engineering spec's description of "Source populated by list results" is incorrect — the view must call `GetPrompt(ctx, id)` (or equivalent) to load the source for the selected prompt into the right pane.

The `listPromptsFromFS()` helper scans `.smithers/prompts/` for `.mdx` files and returns one `Prompt{ID, EntryFile}` per file. It does **not** read file contents during the list phase — this is the correct design for fast list loading.

The `getPromptFromFS()` helper reads the `.mdx` file, parses `{props.X}` references via `propPattern`, and returns a fully populated `Prompt{ID, EntryFile, Source, Props}`.

`parsePromptsJSON` tolerates both a direct JSON array and `{"prompts": [...]}` envelope shape.

### Props Discovery — Regex Pattern

```go
var propPattern = regexp.MustCompile(`\{props\.([A-Za-z_][A-Za-z0-9_]*)\}`)
```

This matches bare `{props.X}` syntax. It does **not** match:
- Conditional expressions: `{props.additionalContext ? ... : ""}` (see `feature-enum-scan.mdx`)
- Array method calls: `{(props.features ?? []).map(...)}` (see `audit-feature.mdx`)
- Ternary/JSX expressions with the prop embedded in a larger expression

The `discoverPropsFromSource` function correctly identifies unique prop names in order of first appearance. For the view's "inputs" display, it produces one row per unique `props.X` reference, regardless of how the prop is used in context.

---

## Prompt File Format (.mdx)

### Anatomy of a Real Prompt File

Examination of all 13 prompts in `.smithers/prompts/` reveals three patterns:

**Pattern 1 — Simple interpolation (9 of 13 prompts)**

```mdx
# Title

Prose instructions...

REQUEST:
{props.prompt}

REQUIRED OUTPUT:
{props.schema}
```

Examples: `implement.mdx`, `plan.mdx`, `research.mdx`, `review.mdx`, `ticket.mdx`, `validate.mdx`, `coverage.mdx`

These have 2–3 props, always the same (`prompt`, `schema`, and occasionally `reviewer`).

**Pattern 2 — Optional + rich interpolation (3 of 13 prompts)**

```mdx
# Title

{props.additionalContext ? `ADDITIONAL CONTEXT:\n${props.additionalContext}\n` : ""}

Prose...

{props.context}
```

Examples: `feature-enum-scan.mdx`, `audit-feature.mdx`, `write-a-prd.mdx`, `grill-me.mdx`

These use JSX-style conditional and ternary expressions. The `propPattern` regex still extracts the prop name from the `{props.X ?` prefix and from bare `{props.X}` occurrences. However, the conditional form means the prop is **optional** — a UI should not mark it as required.

**Pattern 3 — Array method call on prop**

```mdx
{(props.features ?? []).map((feature) => `- ${feature}`).join("\n")}
```

Example: `audit-feature.mdx`

The current `propPattern` does **not** match this form because the prop name is preceded by `(`. The prop `features` will not appear in the extracted `Props` list. This is a known limitation of the regex-based discovery, not a bug introduced by this ticket.

### MDX Rendering in a Terminal

MDX files in this codebase are **not processed by a full MDX/JSX compiler** for display purposes. The `renderTemplate` function in `prompts.go` does only string substitution — it replaces `{props.X}` with supplied values and leaves everything else verbatim.

For the prompts list view's "source display" right pane:
- The raw MDX source (with `{props.X}` placeholders visible) is the correct thing to display.
- No MDX compilation or Markdown rendering is needed for this ticket — the content is shown as plain text / pre-formatted source.
- Future tickets (`feat-prompts-live-preview`) will render the filled-in version; this ticket shows the template.

### Source Length Distribution

All 13 prompts examined are short (6–62 lines). The longest is `write-a-prd.mdx` at 62 lines. The constraint noted in the engineering spec ("arbitrarily long MDX source") is a theoretical future concern, not a present one given the actual data. However, the view should still handle truncation gracefully since users may create longer prompts.

### Discovery Mechanism

The filesystem discovery (`listPromptsFromFS`) is:

1. `os.Getwd()` → look for `.smithers/prompts/` relative to CWD.
2. `os.ReadDir()` → list all entries.
3. Filter: `strings.HasSuffix(name, ".mdx")` and `!entry.IsDir()`.
4. Strip extension: `strings.TrimSuffix(name, ".mdx")` → becomes the `ID`.
5. `EntryFile` = `".smithers/prompts/{name}"` (relative path).

There is no recursive scanning — only the top-level `.smithers/prompts/` directory is scanned. Subdirectory prompts would be ignored. This matches upstream behavior.

---

## Existing View Patterns

### Common Scaffold (agents.go, tickets.go, approvals.go)

All three existing views share an identical structural pattern:

```
struct fields:  client, []data, cursor, width, height, loading, err
Init():         async tea.Cmd → loadedMsg or errorMsg
Update():       switch msg type → loadedMsg / errorMsg / WindowSizeMsg / KeyPressMsg
View():         header (bold left, faint right) + loading/error/empty states + content
Name():         lowercase string identifier
ShortHelp():    []string slice of key hints
```

All three use:
- `▸` (U+25B8) as the cursor indicator, `"  "` (two spaces) for unselected rows.
- `lipgloss.NewStyle().Bold(true).Render(...)` for selected item names.
- `lipgloss.NewStyle().Faint(true).Render(...)` for secondary metadata.
- The `key.Matches(msg, key.NewBinding(key.WithKeys(...)))` pattern for key handling.
- `PopViewMsg{}` returned from `esc`/`alt+esc` to pop the view stack.

### Split-Pane Pattern (approvals.go — canonical reference)

`ApprovalsView` implements the split-pane directly without the shared component (which does not yet exist):

```go
listWidth := 30
dividerWidth := 3
detailWidth := v.width - listWidth - dividerWidth
if v.width < 80 || detailWidth < 20 {
    // compact fallback
}
// Render both panes as strings, split on "\n", join line by line
```

Key details:
- Left pane is **fixed width** (30 chars), not proportional.
- Divider is `" │ "` (3 chars) rendered with `lipgloss.NewStyle().Faint(true)`.
- Lines are joined with `padRight(left, listWidth) + divider + right + "\n"`.
- Height is capped at `v.height - 3` to leave room for the header.
- `padRight` is defined as a package-level helper in `approvals.go` using `lipgloss.Width` for correct ANSI-aware padding.
- **Compact fallback** (`renderListCompact`) shows the selected item's detail inline below its list entry, not side-by-side.

The `eng-split-pane-component` dependency targets extracting this pattern into `internal/ui/components/splitpane.go`. Until that dependency lands, inline `lipgloss.JoinHorizontal` or the manual line-by-line assembly from `approvals.go` is the correct fallback.

### Source Display Requirement

The prompts view right pane shows MDX source content. This is different from:
- `ApprovalsView.renderDetail()` — renders structured metadata fields.
- `TicketsView` — renders only an ID + snippet (no right pane at all yet).

The source pane must:
1. Display the full raw MDX text (with `{props.X}` placeholders visible).
2. Word-wrap or line-wrap to fit the pane width.
3. Be truncated at `availHeight` lines with a `"... (truncated)"` indicator if the source is longer than available space.
4. Show `"Select a prompt to view source"` when no prompt is selected.
5. Show a "Source" section header above the content.

Below the source content, the right pane should also display discovered props — the `Inputs` section. This is derived from the selected prompt's `Props` field (populated by `GetPrompt`, not `ListPrompts`).

---

## Dependency Analysis

### `eng-prompts-api-client`

**Status**: Already landed. All required methods exist in `internal/smithers/prompts.go`:
- `ListPrompts(ctx)` — returns `[]Prompt` without Source/Props.
- `GetPrompt(ctx, id)` — returns `*Prompt` with Source and Props populated.

The view should call `ListPrompts` on init, then call `GetPrompt` lazily when the cursor moves to load the selected prompt's source. An alternative is to call `GetPrompt` for all prompts upfront in `Init`, but this creates N filesystem reads on open.

**Recommended approach**: Load only IDs on init via `ListPrompts`; issue a `GetPrompt` command whenever `cursor` changes (lazy loading). Cache loaded prompts in a `map[string]*smithers.Prompt` to avoid redundant reads.

### `eng-split-pane-component`

**Status**: Not yet landed. `internal/ui/components/splitpane.go` does not exist.

The inline approach from `approvals.go` is the correct mitigation:
- Use fixed `listWidth = 30`, divider `" │ "`, `detailWidth = v.width - 33`.
- Compact fallback at `v.width < 80`.
- Refactor to the shared component once `eng-split-pane-component` lands.

The prompts view and the approvals view will share the same layout logic, so the split-pane component will unify them without functional change.

### MCP Prompts Collision (Risk #7 from engineering spec)

`internal/ui/dialog/commands.go` uses the identifier `MCPPrompts` (an enum value) and `mcpPrompts []commands.MCPPrompt` as a field. The Smithers prompts feature is entirely distinct from MCP protocol prompts.

The `/prompts` command palette entry must be labeled **"Prompt Templates"** (not "Prompts") in the command palette to avoid ambiguity with the existing "MCP" tab label in the commands dialog. The `CommandItem` ID should be `"smithers_prompts"` (not `"prompts"`) to avoid any ID collision.

---

## Navigation Wiring

### `ActionOpenPromptsView` (dialog/actions.go)

Three existing view-open actions define the pattern:

```go
// At lines 88-96 of actions.go (inside the same `type (...)` block)
ActionOpenAgentsView   struct{}
ActionOpenTicketsView  struct{}
ActionOpenApprovalsView struct{}
```

`ActionOpenPromptsView` must be added to this same block.

### Command Palette (dialog/commands.go)

The `defaultCommands()` function at lines 419–535 appends the three existing view commands at lines 527–530:

```go
NewCommandItem(c.com.Styles, "agents",    "Agents",    "", ActionOpenAgentsView{}),
NewCommandItem(c.com.Styles, "approvals", "Approvals", "", ActionOpenApprovalsView{}),
NewCommandItem(c.com.Styles, "tickets",   "Tickets",   "", ActionOpenTicketsView{}),
```

The prompts entry must be added here with ID `"smithers_prompts"` and label `"Prompt Templates"`.

### UI Model (model/ui.go)

The handler for `ActionOpenPromptsView` follows the identical three-line pattern at lines 1458–1477:

```go
case dialog.ActionOpenPromptsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    promptsView := views.NewPromptsView(m.smithersClient)
    cmd := m.viewRouter.Push(promptsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

---

## Lazy Source Loading Design

The most important architectural decision for this ticket is whether to load prompt source eagerly or lazily.

### Option A — Eager (load all sources in Init)

`Init()` calls `ListPrompts` then fans out N `GetPrompt` calls. All sources are available immediately once loaded.

Downsides: N filesystem reads on view open (N = number of `.mdx` files; currently 13). For the current dataset this is negligible, but scales poorly.

### Option B — Lazy (load source on cursor change) — Recommended

`Init()` calls `ListPrompts` only. When `cursor` changes, if the selected prompt's source is not in `loadedSources map[string]*smithers.Prompt`, issue a `tea.Cmd` to call `GetPrompt`.

Implementation fields to add:
```go
loadedSources map[string]*smithers.Prompt
loadingSource bool  // true while a GetPrompt is in flight
```

New message types:
```go
type promptSourceLoadedMsg struct{ prompt *smithers.Prompt }
type promptSourceErrorMsg  struct{ id string; err error }
```

When `loadingSource == true`, the right pane shows `"Loading..."` instead of content. The command issued is:

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

The cache (`loadedSources`) persists for the lifetime of the view — no eviction needed.

---

## Testing Infrastructure

### Existing Harness

`internal/e2e/tui_helpers_test.go` provides `TUITestInstance` with `launchTUI`, `WaitForText`, `WaitForNoText`, `SendKeys`, `Snapshot`, and `Terminate`. This is sufficient for the E2E test without any harness changes.

The `launchTUI` function uses `go run .` from the repo root with `TERM`, `COLORTERM`, `LANG` env vars set and captures combined stdout+stderr via `syncBuffer`. ANSI codes are stripped by `ansiPattern`.

For the prompts E2E test, the TUI needs `.smithers/prompts/*.mdx` fixture files accessible from its working directory. Since `launchTUI` uses the repo root as `cmd.Dir`, the test can write fixture files into a temp directory and either:
1. Set `SMITHERS_PROMPTS_DIR` env var (if the client supports it — it currently does not), or
2. Write test fixtures to the actual `.smithers/prompts/` directory in the repo (fragile — pollutes the real project).
3. Set the working directory to a temp dir that contains a `.smithers/prompts/` subdirectory.

**Recommended approach**: Modify `launchTUI` to accept a `dir` option that overrides `cmd.Dir`, allowing the test to run from a temp project root with fixture prompts. Alternatively, use `t.Setenv` to override `SMITHERS_PROMPTS_DIR` and add a `promptsDir()` override hook to the client.

The cleanest solution is to add a `WithPromptsDir(path string)` option to `Client` (a one-liner) so the E2E test can point the client to a temp directory with fixture files.

### Unit Test Fixtures

The `prompts.go` `listPromptsFromFS` and `getPromptFromFS` functions use `os.Getwd()` to locate the prompts directory. This means unit tests that call `ListPrompts` with the filesystem tier must either:
- Run from a directory that has `.smithers/prompts/` (fine for integration-style tests in the repo root).
- Override `promptsDir()` via a `WithPromptsDir` option (preferred for isolation).

---

## Files Relevant to This Ticket

| File | Status | Relevance |
|------|--------|-----------|
| `internal/smithers/types_prompts.go` | Exists, complete | `Prompt` and `PromptProp` types |
| `internal/smithers/prompts.go` | Exists, complete | `ListPrompts`, `GetPrompt`, filesystem helpers, regex |
| `internal/ui/views/prompts.go` | Does not exist | Primary deliverable — `PromptsView` |
| `internal/ui/views/approvals.go` | Exists | Split-pane reference implementation |
| `internal/ui/views/tickets.go` | Exists | List view reference implementation |
| `internal/ui/views/agents.go` | Exists | View scaffold reference |
| `internal/ui/views/router.go` | Exists, complete | View stack — no changes needed |
| `internal/ui/dialog/actions.go` | Exists | Add `ActionOpenPromptsView` to the action type block |
| `internal/ui/dialog/commands.go` | Exists | Add `"smithers_prompts"` / `"Prompt Templates"` command |
| `internal/ui/model/ui.go` | Exists | Add `ActionOpenPromptsView` case near line 1472 |
| `internal/e2e/tui_helpers_test.go` | Exists | E2E harness — no changes needed |
| `internal/e2e/prompts_list_test.go` | Does not exist | E2E test for prompts list view |
| `tests/vhs/prompts-list.tape` | Does not exist | VHS recording |
| `.smithers/prompts/*.mdx` | Exists (13 files) | Prompt source format reference |
