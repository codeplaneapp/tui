# Research Summary: feat-prompts-source-edit

## Ticket Overview

Add inline MDX source editing to `PromptsView`. The right pane currently renders the prompt source as static read-only text. This ticket replaces that with a `bubbles/textarea` component, adds a focus state machine, wires `Enter` to enter edit mode and `Esc` to exit, and stubs `Ctrl+S` / `Ctrl+O` for downstream tickets.

---

## Files Examined

- `internal/ui/views/prompts.go` — Full `PromptsView` implementation (split-pane list + read-only detail)
- `internal/ui/views/prompts_test.go` — Existing unit test suite (420 lines)
- `internal/smithers/prompts.go` — `UpdatePrompt`, `GetPrompt`, `ListPrompts`, filesystem and HTTP routes
- `internal/ui/model/ui.go` — How the main Crush chat UI embeds and drives `textarea.Model`
- `docs/smithers-tui/02-DESIGN.md` — UX principles including "Native handoff over embedded clones"
- `.smithers/specs/engineering/eng-hijack-handoff-util.md` — `tea.ExecProcess` pattern for external handoff
- `.smithers/specs/plans/eng-hijack-handoff-util.md` — `HandoffToProgram` implementation plan
- `.smithers/specs/ticket-groups/content-and-prompts.json` — Full ticket graph for this feature group

---

## Key Findings

### Current PromptsView State

`PromptsView` is fully implemented for read-only display (ticket `feat-prompts-list`). The struct already has:
- `prompts []smithers.Prompt` — list from `ListPrompts`
- `loadedSources map[string]*smithers.Prompt` — lazy-loaded MDX source cache keyed by prompt ID
- `cursor int` — selected index in the list
- `loadingSource bool` — spinner state while `GetPrompt` is in flight
- Split-pane render at `>= 80 cols` width, compact fallback below

The `Enter` key case in `Update` is explicitly a no-op with a comment: `// Future: feat-prompts-source-edit will focus the source pane.` — this is the exact hook point for this ticket.

### bubbles/textarea API

The project already uses `charm.land/bubbles/v2/textarea` in `internal/ui/model/ui.go`. Relevant API surface:

```go
ta := textarea.New()
ta.ShowLineNumbers = false
ta.CharLimit = -1
ta.SetWidth(w)
ta.MaxHeight = h
ta.SetValue(source)
ta.Focus()    // returns tea.Cmd
ta.Blur()
ta.Value()    // current content
ta.View()     // rendered string
```

The component is updated by passing `tea.Msg` through `ta.Update(msg)` and replacing the model field. It handles its own cursor, scrolling, and multi-line editing.

Key observation from `ui.go`: the `textarea.MaxHeight` field (not `SetHeight`) controls the maximum rendered height. The current height adapts to content. Setting `MaxHeight` on every `WindowSizeMsg` is the correct pattern — see `updateSize()` line setting `m.textarea.MaxHeight = TextareaMaxHeight`.

### UpdatePrompt Already Exists

`internal/smithers/prompts.go` line 71–89 implements `UpdatePrompt(ctx, promptID, content string) error` with three-tier fallback: HTTP POST → filesystem write → exec. The filesystem write path in `updatePromptOnFS` verifies the file exists before writing, then uses `os.WriteFile` with `0o644`. This is ready to be called from the save handler without any changes.

### tea.ExecProcess for $EDITOR Handoff

The `eng-hijack-handoff-util` ticket defines the `HandoffToProgram` utility (`internal/ui/util/handoff.go`). The `$EDITOR` handoff for prompts (`feat-prompts-external-editor-handoff`) will use the same mechanism:
1. Resolve `os.Getenv("EDITOR")` (falling back to `$VISUAL`, then `vi`)
2. Compute the absolute path to the `.mdx` file from `loaded.EntryFile`
3. Call `HandoffToProgram(editorBin, []string{mdxPath}, cwd, onReturn)`
4. On `HandoffReturnMsg`, invalidate the source cache entry and reload

This ticket (`feat-prompts-source-edit`) only needs to emit `promptOpenEditorMsg{}` as a stub — the actual `tea.ExecProcess` call lives in `feat-prompts-external-editor-handoff`.

### Design Principle Alignment

The design doc (section 1, principle 6: "Native handoff over embedded clones") explicitly endorses launching `$EDITOR` natively rather than building a terminal text editor within the TUI. The inline `textarea` editing here is intentionally lightweight — suitable for quick tweaks — while `Ctrl+O` provides the power-user escape hatch. This is the correct architecture.

### Focus State Machine

The existing `PromptsView` has no focus enum; it is implicitly always in list-focus mode. The ticket requires adding `focusList` / `focusEditor` states. The pattern is already used in the main `UI` struct (`uiFocusEditor` / `uiFocusMain` in `internal/ui/model/ui.go`) but that is more complex (multiple focus targets). For `PromptsView`, a simple two-state enum is sufficient.

### Dirty Tracking

The `textarea.Value()` method returns the current string content. Comparing it against `loadedSources[id].Source` after each keypress is cheap and reliable for MDX files (typically < 10 KB). The only concern is newline normalisation — see risk section in engineering spec.

### Ticket Dependencies and Build Order

Ticket graph for this feature group:
```
eng-prompts-api-client  ──▶  feat-prompts-list  ──▶  feat-prompts-source-edit
                                                             │
                             ┌───────────────────────────────┤
                             ▼                               ▼                      ▼
                   feat-prompts-save          feat-prompts-props-discovery   feat-prompts-external-editor-handoff
                             │                               │
                             └───────────────────┬──────────┘
                                                 ▼
                                     feat-prompts-live-preview
```

`feat-prompts-source-edit` is the central hub — three downstream tickets depend on it. The `promptSaveMsg` and `promptOpenEditorMsg` stubs defined in this ticket give downstream work a clean extension point without breaking the `View` interface contract.

### Existing Test Infrastructure

`prompts_test.go` already has a comprehensive suite (420 lines, 18 test functions) covering constructor, list loading, source loading, keyboard navigation, view rendering, and `ShortHelp`. The new tests for this ticket extend the file in the same style — no new test helpers required, just additional test functions reusing `newPromptsView()` and `testLoadedPrompt()`.

---

## Gaps and Risks

1. **`Ctrl+S` / `Ctrl+O` key capture by textarea**: `bubbles/textarea` processes all printable and some control keys. Need to guard view-level bindings before forwarding to `textarea.Update`. Pattern already used in `ui.go` for `SendMessage` / `Newline` keys.

2. **Compact mode and edit focus**: When `v.width < 80`, the split pane collapses to a single column. Entering `focusEditor` mode in compact mode would render the textarea beneath the list, which is confusing. Suppress `Enter`→edit transition in compact mode.

3. **Cache invalidation on external edit**: `loadedSources` holds the source at time of load. After `$EDITOR` handoff returns, the file may have changed. The cache must be invalidated. This is trivially `delete(v.loadedSources, id)` followed by `v.loadSelectedSource()` — but it must be wired in `feat-prompts-external-editor-handoff`.

4. **`promptSaveMsg` / `promptOpenEditorMsg` visibility**: These are unexported types in `prompts.go`. Downstream tickets that add handlers in `PromptsView.Update` will be in the same `views` package, so unexported visibility is correct.

---

## Recommended Direction

- Add `promptsFocus` enum (`focusList` / `focusEditor`) to `PromptsView`
- Embed `textarea.Model` field `editor` in the struct
- Branch `Update`'s `tea.KeyPressMsg` on `v.focus`
- `Enter` (list focus): load source into textarea, call `textarea.Focus()`, set `focusEditor`
- `Esc` (editor focus): reset textarea to cached source, call `textarea.Blur()`, set `focusList`
- Forward remaining editor keys to `textarea.Update` with dirty tracking
- Replace static source rendering in `renderDetail` with `v.editor.View()` when in `focusEditor`
- Stub `promptSaveMsg` and `promptOpenEditorMsg` types
- Update `ShortHelp` to be context-sensitive
- Suppress edit mode in compact terminal (`v.width < 80`)
