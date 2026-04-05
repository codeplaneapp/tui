## Goal

Extend `PromptsView` with inline MDX source editing. Replace the static right-pane source display with a `bubbles/textarea` component. Wire `Enter` to enter edit mode, `Esc` to exit (discarding edits), and stub `Ctrl+S` / `Ctrl+O` for downstream tickets. This is the structural prerequisite for `feat-prompts-save`, `feat-prompts-props-discovery`, and `feat-prompts-external-editor-handoff`.

## Steps

1. **Add focus state enum and textarea field**: In `internal/ui/views/prompts.go`, define `promptsFocus` with `focusList` and `focusEditor` values. Add `focus promptsFocus`, `editor textarea.Model`, and `dirty bool` fields to `PromptsView`. Initialise the textarea in `NewPromptsView` with `ShowLineNumbers = false`, `CharLimit = -1`.

2. **Split key routing on focus state**: Replace the current `tea.KeyPressMsg` block in `Update` with a branch on `v.focus`. Extract `updateList(msg)` for list-focus keys (existing navigation + `Enter` → `enterEditMode()`) and `updateEditor(msg)` for editor-focus keys (`Esc` → `exitEditMode()`, `Ctrl+S` → `promptSaveMsg{}` stub, `Ctrl+O` → `promptOpenEditorMsg{}` stub, all else forwarded to `v.editor.Update(msg)`).

3. **Implement `enterEditMode` and `exitEditMode`**: `enterEditMode` loads `loadedSources[id].Source` into the textarea, calls `v.editor.Focus()`, sets `v.focus = focusEditor`, and calls `v.resizeEditor()`. `exitEditMode` calls `v.editor.Blur()`, restores the textarea to the cached source (discards edits), clears `v.dirty`, and sets `v.focus = focusList`. Guard `enterEditMode` against empty `loadedSources` and narrow terminal (`v.width < 80`).

4. **Add `resizeEditor` helper and call it on `WindowSizeMsg`**: Compute `detailWidth = v.width - 33` (list 30 + divider 3), `editorHeight = v.height - 5 - reservedForProps` (mirroring the existing `maxSourceLines` logic), clamp both to safe minimums. Apply with `v.editor.SetWidth(detailWidth)` and `v.editor.MaxHeight = editorHeight`.

5. **Switch render path in `renderDetail`**: When `v.focus == focusEditor`, call `renderDetailEditor(width, loaded)` instead of the existing static text block. `renderDetailEditor` emits the "Source" header, `EntryFile` label (with `[modified]` indicator when `v.dirty`), `v.editor.View()`, and the Inputs section below.

6. **Add dirty tracking**: After forwarding a key to `textarea.Update` in `updateEditor`, compare `v.editor.Value()` against `loadedSources[id].Source` and set `v.dirty` accordingly.

7. **Stub message types**: Add unexported `promptSaveMsg struct{}` and `promptOpenEditorMsg struct{}` at the top of `prompts.go`. Handle them as no-ops in `Update` for now (downstream tickets will replace the no-op with real logic).

8. **Update `ShortHelp`**: Return context-sensitive bindings — editor focus: `ctrl+s "save"`, `ctrl+o "open editor"`, `esc "back"`; list focus: `↑↓ "navigate"`, `enter "edit"`, `r "refresh"`, `esc "back"`.

9. **Write unit tests**: Extend `internal/ui/views/prompts_test.go` with 10 new test functions covering: Enter→edit transition, Enter no-op without loaded source, Esc-from-editor returns list, Esc discards edits, dirty flag set on change, dirty flag cleared on Esc, `renderDetail` shows editor view in edit mode, `[modified]` indicator, context-sensitive `ShortHelp`, and `WindowSizeMsg` resizes editor.

10. **Add VHS recording**: Create `tests/vhs/prompts-source-edit.tape` covering: launch → `/prompts` → Enter → type edit → screenshot dirty state → Esc → screenshot clean state.

## File Plan

1. `internal/ui/views/prompts.go` — (Modify) Add `promptsFocus` enum, `editor`/`focus`/`dirty` fields, `updateList`, `updateEditor`, `enterEditMode`, `exitEditMode`, `resizeEditor`, `renderDetailEditor` methods; update `ShortHelp`; add `promptSaveMsg` and `promptOpenEditorMsg` stubs.
2. `internal/ui/views/prompts_test.go` — (Modify) Add 10 new test functions for edit mode transitions, dirty tracking, render switching, and help bar bindings.
3. `tests/vhs/prompts-source-edit.tape` — (New File) VHS tape recording the inline edit happy path.
4. `tests/tui/prompts_source_edit_e2e_test.go` — (New File) Terminal E2E tests for Enter→edit, Esc→list, and `[modified]` indicator.

## Validation

- **Unit tests**: `go test ./internal/ui/views/ -run TestPromptsView -v` — all existing plus 10 new tests pass.
- **Build check**: `go build ./...` and `go vet ./internal/ui/views/...` — no errors.
- **VHS tape**: `vhs tests/vhs/prompts-source-edit.tape` — produces `prompts-source-edit.gif` and two PNG screenshots without error.
- **Terminal E2E**: `go test ./tests/tui/ -run TestPromptsSourceEdit -v -timeout 60s` — all three scenarios pass.
- **Manual smoke**: Launch `go run .` in a repo with a `.smithers/prompts/*.mdx` file; open `/prompts`; press `Enter`; type a change; verify `[modified]`; press `Esc`; verify discard.

## Open Questions

1. Should `Enter` in compact terminal mode (`v.width < 80`) silently no-op or show a brief toast ("Widen terminal to edit")? The engineering spec proposes a silent no-op for simplicity, but a toast would improve discoverability.
2. When `feat-prompts-save` lands, should a successful save clear `v.dirty` and update `loadedSources[id].Source` to the new content, or should it trigger a full reload via `loadSelectedSource()`? A full reload is safer (avoids drift if the server normalises content) but adds latency.
3. Should the `[modified]` indicator use a terminal color (amber/yellow, lipgloss color `"3"`) or a plain ASCII `*` prefix on the EntryFile label? The engineering spec proposes color, but plain ASCII is more compatible with non-color terminals.
