# Engineering Spec: Prompt Source Editor

**Ticket**: feat-prompts-source-edit
**Feature**: PROMPTS_SOURCE_EDIT
**Group**: Content And Prompts (content-and-prompts)
**Dependencies**: feat-prompts-list (already implemented as `PromptsView`)

---

## Objective

Replace the read-only MDX source pane in `PromptsView` with an editable `bubbles/textarea` component. When the user presses `Enter` on a prompt in the list, the right pane transitions from static text rendering to a focused `textarea` pre-loaded with the prompt's MDX source. The user can type and modify the source directly. Pressing `Esc` from the editor returns focus to the list (discarding unsaved edits). Pressing `Ctrl+S` saves via `client.UpdatePrompt` (covered by the separate `feat-prompts-save` ticket, but the plumbing must be present here). Pressing `Ctrl+O` hands off to `$EDITOR` (covered by `feat-prompts-external-editor-handoff`).

This ticket is the structural prerequisite for `feat-prompts-save`, `feat-prompts-props-discovery`, and `feat-prompts-external-editor-handoff`.

---

## Scope

### In scope

1. **Focus state machine**: Add a `promptsFocus` type with two states — `focusList` (default) and `focusEditor`. All key routing branches on this value.
2. **Textarea component**: Embed a `textarea.Model` in `PromptsView`. Initialise it with `charm.land/bubbles/v2/textarea`.
3. **Enter to edit**: When focus is `focusList` and the user presses `Enter`, load the selected prompt's source into the textarea, transition to `focusEditor`, and call `textarea.Focus()`.
4. **Esc to return**: When focus is `focusEditor`, `Esc` blurs the textarea, discards in-flight edits (resets textarea to the cached source), and returns to `focusList`.
5. **Textarea sizing**: On every `tea.WindowSizeMsg`, recompute textarea width from `detailWidth` and height from available terminal rows (mirroring the existing `maxSourceLines` calculation).
6. **Render switch**: `renderDetail` detects `focusEditor` and renders `v.editor.View()` in place of the static source text block. The "Source" section header and `EntryFile` label remain visible above the textarea.
7. **Help bar update**: Add `Enter` → `"edit"`, `Ctrl+S` → `"save"`, `Ctrl+O` → `"open editor"`, and `Esc` → `"back"` (context-sensitive based on focus) to `ShortHelp()`.
8. **Unsaved indicator**: When the textarea content diverges from `loadedSources[id].Source`, render a `"[modified]"` marker next to the `EntryFile` label in the detail pane header.
9. **Textarea passthrough**: When `focusEditor` is active, all key events not matching a view-level binding (`Esc`, `Ctrl+S`, `Ctrl+O`) are forwarded to `textarea.Update`.

### Out of scope

- Persisting edits to disk / the API (`feat-prompts-save`)
- `$EDITOR` handoff logic (`feat-prompts-external-editor-handoff`)
- Props discovery re-run after source change (`feat-prompts-props-discovery`)
- Syntax highlighting of MDX source (not planned; terminal-safe rendering only)

---

## Implementation Plan

### Slice 1: Add focus state and embed textarea

**File**: `internal/ui/views/prompts.go`

Add the focus enum above the struct declaration:

```go
type promptsFocus int

const (
    focusList   promptsFocus = iota
    focusEditor
)
```

Extend `PromptsView` struct:

```go
type PromptsView struct {
    // ... existing fields ...
    focus  promptsFocus
    editor textarea.Model
    dirty  bool // true when editor content != cached source
}
```

In `NewPromptsView`, initialise the textarea:

```go
ta := textarea.New()
ta.ShowLineNumbers = false
ta.CharLimit = -1
ta.SetWidth(60)    // overridden on first WindowSizeMsg
ta.SetHeight(20)   // overridden on first WindowSizeMsg
return &PromptsView{
    // ... existing fields ...
    editor: ta,
}
```

Import addition: `"charm.land/bubbles/v2/textarea"`.

---

### Slice 2: Key routing with focus state

**File**: `internal/ui/views/prompts.go` — `Update` method

Replace the current `tea.KeyPressMsg` block with a two-branch structure:

```go
case tea.KeyPressMsg:
    if v.focus == focusEditor {
        return v.updateEditor(msg)
    }
    return v.updateList(msg)
```

Introduce `updateList(msg tea.KeyPressMsg) (View, tea.Cmd)`:
- `Esc` / `alt+esc` → `PopViewMsg` (existing)
- `up` / `k` → move cursor up (existing)
- `down` / `j` → move cursor down (existing)
- `r` → refresh (existing)
- `Enter` → call `v.enterEditMode()`, return

Introduce `updateEditor(msg tea.KeyPressMsg) (View, tea.Cmd)`:
- `Esc` → call `v.exitEditMode()`, return
- `ctrl+s` → emit `promptSaveMsg{}` (consumed by `feat-prompts-save`; for now a no-op stub)
- `ctrl+o` → emit `promptOpenEditorMsg{}` (consumed by `feat-prompts-external-editor-handoff`; stub)
- everything else → forward to `v.editor.Update(msg)`; update `v.dirty`

Helper `enterEditMode() tea.Cmd`:
```go
func (v *PromptsView) enterEditMode() tea.Cmd {
    if v.cursor < 0 || v.cursor >= len(v.prompts) {
        return nil
    }
    id := v.prompts[v.cursor].ID
    loaded, ok := v.loadedSources[id]
    if !ok {
        return nil // source not yet loaded; no-op
    }
    v.editor.SetValue(loaded.Source)
    v.editor.MoveToEnd()
    v.dirty = false
    v.focus = focusEditor
    return v.editor.Focus()
}
```

Helper `exitEditMode()`:
```go
func (v *PromptsView) exitEditMode() {
    v.focus = focusList
    v.dirty = false
    // Restore textarea to cached source (discard edits).
    if v.cursor >= 0 && v.cursor < len(v.prompts) {
        if loaded, ok := v.loadedSources[v.prompts[v.cursor].ID]; ok {
            v.editor.SetValue(loaded.Source)
        }
    }
    v.editor.Blur()
}
```

---

### Slice 3: Textarea sizing on WindowSizeMsg

**File**: `internal/ui/views/prompts.go` — `Update` method, `tea.WindowSizeMsg` arm

After setting `v.width` and `v.height`, compute and apply textarea dimensions:

```go
case tea.WindowSizeMsg:
    v.width = msg.Width
    v.height = msg.Height
    v.resizeEditor()
    return v, nil
```

```go
func (v *PromptsView) resizeEditor() {
    listWidth := 30
    dividerWidth := 3
    detailWidth := v.width - listWidth - dividerWidth
    if detailWidth < 20 {
        detailWidth = 20
    }
    // Reserve header (2 lines) + inputs section footer.
    reservedForProps := 0
    if v.cursor >= 0 && v.cursor < len(v.prompts) {
        if loaded, ok := v.loadedSources[v.prompts[v.cursor].ID]; ok && len(loaded.Props) > 0 {
            reservedForProps = 3 + len(loaded.Props)
        }
    }
    editorHeight := v.height - 5 - reservedForProps
    if editorHeight < 5 {
        editorHeight = 5
    }
    v.editor.SetWidth(detailWidth)
    v.editor.MaxHeight = editorHeight
}
```

Also call `v.resizeEditor()` from `enterEditMode()` to ensure dimensions are fresh before display.

---

### Slice 4: Render switch in renderDetail

**File**: `internal/ui/views/prompts.go` — `renderDetail` method

Before the existing static source-rendering block, add:

```go
// Switch to editable textarea when in editor focus.
if v.focus == focusEditor {
    return v.renderDetailEditor(width, loaded)
}
```

Implement `renderDetailEditor(width int, loaded *smithers.Prompt) string`:

```go
func (v *PromptsView) renderDetailEditor(width int, loaded *smithers.Prompt) string {
    var b strings.Builder
    titleStyle := lipgloss.NewStyle().Bold(true)
    labelStyle := lipgloss.NewStyle().Faint(true)

    b.WriteString(titleStyle.Render("Source") + "\n")

    label := labelStyle.Render(loaded.EntryFile)
    if v.dirty {
        label += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("[modified]")
    }
    b.WriteString(label + "\n\n")

    b.WriteString(v.editor.View() + "\n")

    if len(loaded.Props) > 0 {
        b.WriteString("\n" + titleStyle.Render("Inputs") + "\n")
        for _, prop := range loaded.Props {
            var defaultStr string
            if prop.DefaultValue != nil {
                defaultStr = fmt.Sprintf(" (default: %q)", *prop.DefaultValue)
            }
            b.WriteString(labelStyle.Render("  \u2022 ") + prop.Name +
                labelStyle.Render(" : "+prop.Type+defaultStr) + "\n")
        }
    }

    return b.String()
}
```

---

### Slice 5: Dirty tracking

In `updateEditor`, after forwarding a key event to `textarea.Update`, set:

```go
if v.cursor >= 0 && v.cursor < len(v.prompts) {
    if loaded, ok := v.loadedSources[v.prompts[v.cursor].ID]; ok {
        v.dirty = v.editor.Value() != loaded.Source
    }
}
```

---

### Slice 6: Stub message types for downstream tickets

At the top of `prompts.go`, add the following unexported types so downstream tickets can hook into them without changing the `Update` signature:

```go
// promptSaveMsg is emitted when the user requests a save (Ctrl+S).
// Consumed by feat-prompts-save.
type promptSaveMsg struct{}

// promptOpenEditorMsg is emitted when the user requests an external editor (Ctrl+O).
// Consumed by feat-prompts-external-editor-handoff.
type promptOpenEditorMsg struct{}
```

---

### Slice 7: ShortHelp update

Replace `ShortHelp()` to return context-sensitive bindings:

```go
func (v *PromptsView) ShortHelp() []key.Binding {
    if v.focus == focusEditor {
        return []key.Binding{
            key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
            key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "open editor")),
            key.NewBinding(key.WithKeys("esc"),    key.WithHelp("esc", "back")),
        }
    }
    return []key.Binding{
        key.NewBinding(key.WithKeys("up", "k"),    key.WithHelp("↑↓", "navigate")),
        key.NewBinding(key.WithKeys("enter"),      key.WithHelp("enter", "edit")),
        key.NewBinding(key.WithKeys("r"),          key.WithHelp("r", "refresh")),
        key.NewBinding(key.WithKeys("esc"),        key.WithHelp("esc", "back")),
    }
}
```

---

### Slice 8: Unit tests

**File**: `internal/ui/views/prompts_test.go` (extend existing file)

New test cases to add:

| Test | What it checks |
|------|---------------|
| `TestPromptsView_Enter_TransitionsToEditor` | Pressing `Enter` with a loaded source sets `focus == focusEditor` and `editor.Value()` matches the source |
| `TestPromptsView_Enter_NoopWithoutSource` | Pressing `Enter` when source is not yet loaded leaves focus on `focusList` |
| `TestPromptsView_Esc_FromEditor_ReturnsList` | Pressing `Esc` from `focusEditor` returns `focus == focusList` |
| `TestPromptsView_Esc_FromEditor_DiscardsEdits` | After typing in the editor and pressing `Esc`, `editor.Value()` equals the original cached source |
| `TestPromptsView_EditorDirtyFlag_SetOnChange` | After modifying editor content, `dirty == true` |
| `TestPromptsView_EditorDirtyFlag_ClearedOnEsc` | After `Esc`, `dirty == false` |
| `TestPromptsView_RenderDetail_ShowsEditorInFocusEditor` | `View()` output in `focusEditor` does not contain the static `"... (truncated)"` path |
| `TestPromptsView_RenderDetail_ModifiedIndicator` | `View()` contains `"[modified]"` when `dirty == true` |
| `TestPromptsView_ShortHelp_EditorBindings` | `ShortHelp()` returns `"save"` and `"open editor"` when in `focusEditor` |
| `TestPromptsView_ShortHelp_ListBindings` | `ShortHelp()` returns `"edit"` when in `focusList` |
| `TestPromptsView_WindowSizeMsg_ResizesEditor` | After `WindowSizeMsg`, `editor.Width()` reflects `detailWidth` |

---

### Slice 9: VHS recording

**File**: `tests/vhs/prompts-source-edit.tape`

```
Output tests/vhs/prompts-source-edit.gif
Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 600
Set TypingSpeed 60ms

# Launch smithers-tui (with at least one .smithers/prompts/*.mdx file in cwd)
Type "smithers-tui"
Enter
Sleep 2s

# Navigate to prompts view
Type "/prompts"
Enter
Sleep 1s

# First prompt should be selected; press Enter to enter edit mode
Enter
Sleep 500ms

# Type a modification
Type "# edited by VHS"
Sleep 1s

# Modified indicator should appear
Screenshot tests/vhs/prompts-source-edit-dirty.png

# Esc to discard and return to list
Escape
Sleep 500ms

Screenshot tests/vhs/prompts-source-edit-final.png
```

---

## Validation

### Unit tests

```bash
go test ./internal/ui/views/ -run TestPromptsView -v
```

All new tests plus all existing `prompts_test.go` tests must pass.

### Build

```bash
go build ./...
go vet ./internal/ui/views/...
```

### Terminal E2E (modeled on upstream `@microsoft/tui-test` harness)

**File**: `tests/tui/prompts_source_edit_e2e_test.go`

```
Test: "Enter on prompt list item enters edit mode"
  1. Build and launch smithers-tui with a tmp .smithers/prompts/test.mdx file.
  2. Navigate to /prompts.
  3. Wait for prompt ID to appear.
  4. Send Enter.
  5. Assert focus indicator in help bar changes to show "save" and "open editor".

Test: "Esc from editor returns to list"
  1. Navigate to /prompts, press Enter to enter edit mode.
  2. Press Esc.
  3. Assert help bar returns to showing "edit" and "navigate".

Test: "Editing source sets modified indicator"
  1. Navigate to /prompts, press Enter on a prompt.
  2. Type "x".
  3. Assert screen contains "[modified]".
  4. Press Esc.
  5. Assert "[modified]" is no longer present.
```

### VHS visual test

```bash
vhs tests/vhs/prompts-source-edit.tape
```

Artifacts: `tests/vhs/prompts-source-edit.gif`, `tests/vhs/prompts-source-edit-dirty.png`, `tests/vhs/prompts-source-edit-final.png`.

### Manual smoke test

1. Run `go run .` in a repo with at least one `.smithers/prompts/*.mdx` file.
2. Open `/prompts`, navigate to a prompt, press `Enter`.
3. Confirm cursor appears in the source pane.
4. Type a few characters; confirm `[modified]` indicator appears.
5. Press `Esc`; confirm edits are discarded and list regains focus.
6. Confirm narrow terminal (`< 80 cols`) compact layout is not broken.

---

## Risks

### 1. `textarea.Update` swallowing global keys

**Risk**: The `bubbles/textarea` component intercepts `Ctrl+S` and `Ctrl+O` as raw runes before the view-level handler sees them.

**Mitigation**: Check for view-level bindings (`ctrl+s`, `ctrl+o`, `esc`) _before_ forwarding to `textarea.Update`. The key routing in `updateEditor` already handles this with explicit `key.Matches` guards.

### 2. Textarea height/width out of sync with split-pane

**Risk**: `resizeEditor` is called on `WindowSizeMsg`, but the detail pane width depends on whether the terminal is wide enough to split. If the terminal is narrow (`< 80 cols`), `detailWidth` could be negative or zero.

**Mitigation**: Clamp `detailWidth` to a minimum of 20 and `editorHeight` to a minimum of 5. Additionally, suppress `focusEditor` mode when the terminal is too narrow to show the split (entering edit mode in compact mode is a UX edge case we can handle by keeping the user in `focusList` when `v.width < 80`).

### 3. `dirty` flag false negatives

**Risk**: `textarea.Value()` may normalise newlines (e.g., `\r\n` → `\n`) in ways that make the string comparison unreliable.

**Mitigation**: Normalise `loaded.Source` to `\n` line endings before storing in `enterEditMode` and before comparing in dirty tracking. This mirrors how `os.ReadFile` returns the raw bytes, which will already use `\n` on POSIX. Document the assumption in a comment.

### 4. Cache invalidation after external edit

**Risk**: `feat-prompts-external-editor-handoff` will modify the file on disk then reload it. If `PromptsView` caches the old source in `loadedSources`, the textarea will be out of date.

**Mitigation**: On receipt of the `promptEditorReturnMsg` (defined in `feat-prompts-external-editor-handoff`), invalidate the cache entry for the current prompt and call `loadSelectedSource()` to reload. This is not in scope here but the architecture must not make it hard — `loadedSources` is a plain `map[string]*smithers.Prompt`, so deletion by key is trivial.

### 5. Prop section height reservation stale in editor mode

**Risk**: `resizeEditor` reads `loadedSources` to compute `reservedForProps`, but during `enterEditMode` the source may just have been loaded. Stale prop counts could make the textarea too tall.

**Mitigation**: Call `resizeEditor()` at the end of `enterEditMode()`, after `loadedSources` is confirmed to have the entry, to ensure the reservation uses fresh prop counts.
