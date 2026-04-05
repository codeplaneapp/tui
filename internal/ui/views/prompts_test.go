package views

import (
	"errors"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func newPromptsView() *PromptsView {
	c := smithers.NewClient() // no-op client; no server
	return NewPromptsView(c)
}

func testPrompts() []smithers.Prompt {
	return []smithers.Prompt{
		{ID: "review", EntryFile: ".smithers/prompts/review.mdx"},
		{ID: "implement", EntryFile: ".smithers/prompts/implement.mdx"},
		{ID: "plan", EntryFile: ".smithers/prompts/plan.mdx"},
	}
}

func strPtr(s string) *string { return &s }

func testLoadedPrompt(id string) *smithers.Prompt {
	return &smithers.Prompt{
		ID:        id,
		EntryFile: ".smithers/prompts/" + id + ".mdx",
		Source:    "# " + id + "\n\nReview {props.lang} code for {props.focus}.\n",
		Props: []smithers.PromptProp{
			{Name: "lang", Type: "string"},
			{Name: "focus", Type: "string"},
		},
	}
}

// --- Interface compliance ---

func TestPromptsView_InterfaceCompliance(t *testing.T) {
	var _ View = (*PromptsView)(nil)
}

// --- Constructor ---

func TestPromptsView_Init_SetsLoading(t *testing.T) {
	v := newPromptsView()
	assert.True(t, v.loading, "NewPromptsView should start in loading state")
	assert.Empty(t, v.prompts)
	assert.NotNil(t, v.loadedSources)
}

func TestPromptsView_Init_ReturnsCmd(t *testing.T) {
	v := newPromptsView()
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil tea.Cmd")
}

// --- Update: list loading ---

func TestPromptsView_LoadedMsg_PopulatesPrompts(t *testing.T) {
	v := newPromptsView()
	prompts := testPrompts()
	updated, _ := v.Update(promptsLoadedMsg{prompts: prompts})
	pv := updated.(*PromptsView)
	assert.False(t, pv.loading)
	assert.Len(t, pv.prompts, 3)
	assert.Equal(t, "review", pv.prompts[0].ID)
}

func TestPromptsView_ErrorMsg_SetsErr(t *testing.T) {
	v := newPromptsView()
	someErr := errors.New("connection refused")
	updated, cmd := v.Update(promptsErrorMsg{err: someErr})
	pv := updated.(*PromptsView)
	assert.False(t, pv.loading)
	assert.Equal(t, someErr, pv.err)
	assert.Nil(t, cmd)
}

// --- Update: source loading ---

func TestPromptsView_SourceLoadedMsg_CachesSource(t *testing.T) {
	v := newPromptsView()
	// Seed prompts first.
	v.prompts = testPrompts()
	v.loading = false
	v.loadingSource = true

	loaded := testLoadedPrompt("review")
	updated, cmd := v.Update(promptSourceLoadedMsg{prompt: loaded})
	pv := updated.(*PromptsView)

	assert.False(t, pv.loadingSource)
	assert.Nil(t, pv.sourceErr)
	assert.Nil(t, cmd)
	cached, ok := pv.loadedSources["review"]
	require.True(t, ok, "source should be cached after load")
	assert.Equal(t, "review", cached.ID)
}

func TestPromptsView_SourceErrorMsg_SetsSourceErr(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.loadingSource = true

	someErr := errors.New("file not found")
	updated, cmd := v.Update(promptSourceErrorMsg{id: "review", err: someErr})
	pv := updated.(*PromptsView)

	assert.False(t, pv.loadingSource)
	assert.Equal(t, someErr, pv.sourceErr)
	assert.Nil(t, cmd)
}

// --- Update: window resize ---

func TestPromptsView_WindowSizeMsg_UpdatesDimensions(t *testing.T) {
	v := newPromptsView()
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	pv := updated.(*PromptsView)
	assert.Equal(t, 120, pv.width)
	assert.Equal(t, 40, pv.height)
	assert.Nil(t, cmd)
}

// --- Update: keyboard navigation ---

func TestPromptsView_CursorDown_InBounds(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	// loadedSources is empty; loadSelectedSource will issue a command.

	assert.Equal(t, 0, v.cursor)
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	pv := updated.(*PromptsView)
	assert.Equal(t, 1, pv.cursor)
}

func TestPromptsView_CursorDown_StopsAtLast(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.cursor = 2 // last item

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	pv := updated.(*PromptsView)
	assert.Equal(t, 2, pv.cursor, "cursor should not exceed last index")
}

func TestPromptsView_CursorUp_InBounds(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.cursor = 2
	v.loadedSources["implement"] = testLoadedPrompt("implement")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	pv := updated.(*PromptsView)
	assert.Equal(t, 1, pv.cursor)
}

func TestPromptsView_CursorUp_StopsAtZero(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.cursor = 0
	v.loadedSources["review"] = testLoadedPrompt("review")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	pv := updated.(*PromptsView)
	assert.Equal(t, 0, pv.cursor, "cursor should not go below zero")
}

func TestPromptsView_Esc_ReturnsPopViewMsg(t *testing.T) {
	v := newPromptsView()
	v.width = 80
	v.height = 24
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

func TestPromptsView_R_Refresh(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.loadedSources["review"] = testLoadedPrompt("review")

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	pv := updated.(*PromptsView)

	assert.True(t, pv.loading, "'r' should set loading = true")
	assert.NotNil(t, cmd, "'r' should return a load command")
	assert.Empty(t, pv.loadedSources, "'r' should clear the source cache")
	assert.Nil(t, pv.sourceErr, "'r' should clear source error")
}

// --- Update: SetSize ---

func TestPromptsView_SetSize(t *testing.T) {
	v := newPromptsView()
	v.SetSize(100, 30)
	assert.Equal(t, 100, v.width)
	assert.Equal(t, 30, v.height)
}

// --- View() rendering ---

func TestPromptsView_View_HeaderText(t *testing.T) {
	v := newPromptsView()
	v.width = 80
	v.height = 24
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "SMITHERS")
	assert.Contains(t, out, "Prompts")
}

func TestPromptsView_View_LoadingState(t *testing.T) {
	v := newPromptsView()
	v.width = 80
	v.height = 24
	out := v.View()
	assert.Contains(t, out, "Loading prompts...")
}

func TestPromptsView_View_ErrorState(t *testing.T) {
	v := newPromptsView()
	v.width = 80
	v.height = 24
	v.loading = false
	v.err = errors.New("server unavailable")
	out := v.View()
	assert.Contains(t, out, "Error")
}

func TestPromptsView_View_EmptyState(t *testing.T) {
	v := newPromptsView()
	v.width = 80
	v.height = 24
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "No prompts found.")
}

func TestPromptsView_View_PromptIDInList(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	out := v.View()
	assert.Contains(t, out, "review")
	assert.Contains(t, out, "implement")
	assert.Contains(t, out, "plan")
}

func TestPromptsView_View_SelectedCursor(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.cursor = 0
	out := v.View()
	// The selected item should be preceded by the cursor indicator.
	assert.Contains(t, out, "\u25b8") // ▸
}

func TestPromptsView_View_SourcePane(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.cursor = 0
	v.loadedSources["review"] = testLoadedPrompt("review")
	out := v.View()
	assert.Contains(t, out, "Source")
	assert.Contains(t, out, "# review") // first line of source
}

func TestPromptsView_View_InputsSection(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.cursor = 0
	v.loadedSources["review"] = testLoadedPrompt("review")
	out := v.View()
	assert.Contains(t, out, "Inputs")
	assert.Contains(t, out, "lang")
	assert.Contains(t, out, "focus")
}

func TestPromptsView_View_NarrowTerminal(t *testing.T) {
	v := newPromptsView()
	v.width = 60 // triggers compact mode (< 80)
	v.height = 24
	v.loading = false
	v.prompts = testPrompts()
	out := v.View()
	// In compact mode there is no split divider.
	assert.NotContains(t, out, "\u2502", "narrow terminal should not show split divider")
	assert.Contains(t, out, "review")
}

func TestPromptsView_View_WideTerminal(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	out := v.View()
	// Wide terminal shows the split divider.
	assert.Contains(t, out, "\u2502", "wide terminal should show split divider │")
}

func TestPromptsView_View_LoadingSource(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.loadingSource = true
	v.prompts = testPrompts()
	out := v.View()
	assert.Contains(t, out, "Loading source...")
}

func TestPromptsView_View_InputCountInList(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	out := v.View()
	// Input count line should appear below the prompt ID.
	assert.Contains(t, out, "inputs")
}

// --- Name / ShortHelp ---

func TestPromptsView_Name(t *testing.T) {
	v := newPromptsView()
	assert.Equal(t, "prompts", v.Name())
}

func TestPromptsView_ShortHelp(t *testing.T) {
	v := newPromptsView()
	bindings := v.ShortHelp()
	assert.NotEmpty(t, bindings)

	// Collect all help strings.
	var helpTexts []string
	for _, b := range bindings {
		helpTexts = append(helpTexts, b.Help().Desc)
	}
	joined := strings.Join(helpTexts, " ")
	assert.Contains(t, joined, "navigate")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")
}

func TestPromptsView_ShortHelp_ReturnsKeyBindings(t *testing.T) {
	v := newPromptsView()
	bindings := v.ShortHelp()
	for _, b := range bindings {
		_, ok := interface{}(b).(key.Binding)
		assert.True(t, ok, "each element should be a key.Binding")
	}
}

// --- loadSelectedSource caching ---

func TestPromptsView_LoadSelectedSource_CacheHit(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	// Pre-populate cache.
	v.loadedSources["review"] = testLoadedPrompt("review")

	cmd := v.loadSelectedSource()
	assert.Nil(t, cmd, "loadSelectedSource should return nil when source is cached")
	assert.False(t, v.loadingSource, "loadingSource should remain false on cache hit")
}

func TestPromptsView_LoadSelectedSource_CacheMiss(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	// No cache entry for "review".

	cmd := v.loadSelectedSource()
	assert.NotNil(t, cmd, "loadSelectedSource should return a command on cache miss")
	assert.True(t, v.loadingSource, "loadingSource should be true while loading")
}

func TestPromptsView_LoadSelectedSource_EmptyList(t *testing.T) {
	v := newPromptsView()
	v.loading = false
	// No prompts at all.

	cmd := v.loadSelectedSource()
	assert.Nil(t, cmd, "loadSelectedSource should return nil when list is empty")
}

// --- Edit mode: focus transitions ---

// 1. Enter on a loaded prompt switches to editor mode.
func TestPromptsView_Enter_SwitchesToEditorMode(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pv := updated.(*PromptsView)
	assert.Equal(t, focusEditor, pv.focus, "Enter should set focus to editor")
	assert.True(t, pv.editor.Focused(), "editor should be focused after Enter")
}

// 2. Enter is a no-op when source is not yet loaded.
func TestPromptsView_Enter_NoOpWithoutLoadedSource(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.cursor = 0
	// loadedSources is empty — source not yet available.

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pv := updated.(*PromptsView)
	assert.Equal(t, focusList, pv.focus, "Enter should be no-op when source is not loaded")
}

// 3. Esc from editor returns to list focus.
func TestPromptsView_Esc_FromEditor_ReturnsList(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusEditor
	_ = v.editor.Focus()

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	pv := updated.(*PromptsView)
	assert.Equal(t, focusList, pv.focus, "Esc from editor should restore list focus")
	// cmd should be nil (no PopViewMsg) — we are returning to list, not leaving the view.
	assert.Nil(t, cmd, "Esc from editor should not emit PopViewMsg")
}

// 4. Esc from editor discards edits (restores original source in textarea).
func TestPromptsView_Esc_FromEditor_DiscardsEdits(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusEditor
	_ = v.editor.Focus()
	v.editor.SetValue("totally different content")
	v.dirty = true

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	pv := updated.(*PromptsView)

	original := testLoadedPrompt("review").Source
	assert.Equal(t, original, pv.editor.Value(), "Esc should restore the original source in the textarea")
}

// 5. Dirty flag is set when editor content differs from cached source.
func TestPromptsView_DirtyFlag_SetOnChange(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	// Type a printable character to dirty the editor.
	// The textarea uses msg.Text (not msg.Code) for printable input.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	pv := updated.(*PromptsView)
	assert.True(t, pv.dirty, "dirty flag should be set when editor content changes")
}

// 6. Dirty flag is cleared when Esc exits editor mode.
func TestPromptsView_DirtyFlag_ClearedOnEsc(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusEditor
	_ = v.editor.Focus()
	v.editor.SetValue("dirty content")
	v.dirty = true

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	pv := updated.(*PromptsView)
	assert.False(t, pv.dirty, "dirty flag should be cleared when exiting editor via Esc")
}

// --- Rendering ---

// 7. renderDetail shows editor view in edit mode.
func TestPromptsView_RenderDetail_ShowsEditorInEditMode(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	out := v.View()
	assert.Contains(t, out, "Source", "editor mode should still show Source header")
	// The editor's content should appear in the rendered output.
	assert.Contains(t, out, "review", "editor content should appear in the view")
}

// 8. [modified] indicator appears when dirty is true.
func TestPromptsView_ModifiedIndicator_WhenDirty(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusEditor
	_ = v.editor.Focus()
	v.editor.SetValue("# review\n\nchanged content\n")
	v.dirty = true

	out := v.View()
	assert.Contains(t, out, "[modified]", "dirty editor should display [modified] indicator")
}

// 9. ShortHelp is context-sensitive.
func TestPromptsView_ShortHelp_ContextSensitive(t *testing.T) {
	v := newPromptsView()

	// List focus help.
	v.focus = focusList
	listHelp := v.ShortHelp()
	var listDescs []string
	for _, b := range listHelp {
		listDescs = append(listDescs, b.Help().Desc)
	}
	joined := strings.Join(listDescs, " ")
	assert.Contains(t, joined, "navigate")
	assert.Contains(t, joined, "edit")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")

	// Editor focus help.
	v.focus = focusEditor
	editorHelp := v.ShortHelp()
	var editorDescs []string
	for _, b := range editorHelp {
		editorDescs = append(editorDescs, b.Help().Desc)
	}
	editorJoined := strings.Join(editorDescs, " ")
	assert.Contains(t, editorJoined, "save")
	assert.Contains(t, editorJoined, "open editor")
	assert.Contains(t, editorJoined, "back")
	assert.NotContains(t, editorJoined, "navigate", "editor help should not contain 'navigate'")
}

// 10. WindowSizeMsg resizes the editor.
func TestPromptsView_WindowSizeMsg_ResizesEditor(t *testing.T) {
	v := newPromptsView()
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusEditor

	updated, _ := v.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	pv := updated.(*PromptsView)

	assert.Equal(t, 160, pv.width)
	assert.Equal(t, 50, pv.height)
	// Editor width should be detailWidth = 160 - 30 - 3 = 127.
	assert.Equal(t, 127, pv.editor.Width(), "editor width should be resized on WindowSizeMsg")
}

// --- feat-prompts-save: Ctrl+S ---

// 11. Ctrl+S in editor focus emits a save command.
func TestPromptsView_CtrlS_EmitsSaveCmd(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	_, cmd := v.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd, "Ctrl+S should return a save command")
}

// 12. promptSaveMsg with nil err clears dirty flag and updates source cache.
func TestPromptsView_SaveMsg_Success_ClearsDirtyAndUpdatesCache(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()
	v.editor.SetValue("# updated content\n")
	v.dirty = true

	updated, cmd := v.Update(promptSaveMsg{err: nil})
	pv := updated.(*PromptsView)

	assert.False(t, pv.dirty, "dirty flag should be cleared after successful save")
	require.NotNil(t, cmd, "successful save should return a toast command")

	// The command should produce a ShowToastMsg.
	msg := cmd()
	toast, ok := msg.(interface{ GetTitle() string })
	_ = toast
	_ = ok

	// Verify the source cache is updated to the saved content.
	cached, exists := pv.loadedSources["review"]
	require.True(t, exists)
	assert.Equal(t, "# updated content\n", cached.Source,
		"source cache should reflect the newly saved content")
}

// 13. promptSaveMsg with non-nil err returns an error toast and preserves dirty flag.
func TestPromptsView_SaveMsg_Error_ShowsToast(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()
	v.dirty = true

	saveErr := errors.New("permission denied")
	updated, cmd := v.Update(promptSaveMsg{err: saveErr})
	pv := updated.(*PromptsView)

	assert.True(t, pv.dirty, "dirty flag should remain set after a failed save")
	require.NotNil(t, cmd, "failed save should return a toast command")
}

// 14. Successful save updates the source cache so subsequent Esc restores saved content.
func TestPromptsView_Save_EscRestoresSavedContent(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	newSource := "# review\n\nSaved content.\n"
	v.editor.SetValue(newSource)
	v.dirty = true

	// Simulate a successful save.
	updated, _ := v.Update(promptSaveMsg{err: nil})
	pv := updated.(*PromptsView)

	// Now press Esc — the textarea should restore to the saved content.
	updated2, _ := pv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	pv2 := updated2.(*PromptsView)

	assert.Equal(t, newSource, pv2.editor.Value(),
		"after a successful save, Esc should restore to the saved content")
}

// 15. Ctrl+S is a no-op when no prompt is selected (cursor out of bounds).
func TestPromptsView_CtrlS_NoOp_WhenNoCursor(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = []smithers.Prompt{} // empty list
	v.focus = focusEditor
	_ = v.editor.Focus()

	_, cmd := v.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.Nil(t, cmd, "Ctrl+S with no selection should return nil")
}

// --- feat-prompts-external-editor-handoff: Ctrl+O ---

// 16. Ctrl+O in editor focus emits a command (either handoff or error toast).
func TestPromptsView_CtrlO_EmitsCmd(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	// The command is always non-nil: either a handoff or a toast for a temp-file error.
	assert.NotNil(t, cmd, "Ctrl+O should always return a command")
}

// 17. handoff.HandoffMsg with wrong tag is ignored.
func TestPromptsView_HandoffMsg_WrongTag_Ignored(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0

	_, cmd := v.Update(handoff.HandoffMsg{Tag: "unrelated-tag", Result: handoff.HandoffResult{}})
	assert.Nil(t, cmd, "HandoffMsg with wrong tag should be ignored")
}

// 18. handoff.HandoffMsg with correct tag and an error shows an error toast.
func TestPromptsView_HandoffMsg_CorrectTag_Error_ShowsToast(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0

	handoffErr := errors.New("editor crashed")
	_, cmd := v.Update(handoff.HandoffMsg{
		Tag:    promptEditorTag,
		Result: handoff.HandoffResult{Err: handoffErr},
	})
	require.NotNil(t, cmd, "handoff error should produce a toast command")
}

// 19. handoff.HandoffMsg with correct tag reads tmp file and updates textarea.
func TestPromptsView_HandoffMsg_UpdatesTextarea(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	// Write new content into a temp file, simulating what the editor would produce.
	tmpFile, err := os.CreateTemp("", "handoff-test-*.mdx")
	require.NoError(t, err)
	newContent := "# review\n\nExternal editor updated this.\n"
	_, err = tmpFile.WriteString(newContent)
	require.NoError(t, err)
	_ = tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Set the tmpPath so the handler knows where to read from.
	v.tmpPath = tmpFile.Name()

	updated, cmd := v.Update(handoff.HandoffMsg{
		Tag:    promptEditorTag,
		Result: handoff.HandoffResult{},
	})
	pv := updated.(*PromptsView)

	assert.Nil(t, cmd, "successful handoff should not emit a command")
	assert.Equal(t, newContent, pv.editor.Value(),
		"textarea should reflect the content written by the external editor")
	assert.True(t, pv.dirty,
		"dirty flag should be set because external-editor content differs from cached source")
	assert.Empty(t, pv.tmpPath, "tmpPath should be cleared after handling HandoffMsg")
}

// 20. handoff.HandoffMsg: temp file is removed even on success.
func TestPromptsView_HandoffMsg_TempFileCleanedUp(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.enterEditMode()

	tmpFile, err := os.CreateTemp("", "handoff-cleanup-*.mdx")
	require.NoError(t, err)
	_, err = tmpFile.WriteString("content")
	require.NoError(t, err)
	_ = tmpFile.Close()
	tmpName := tmpFile.Name()

	v.tmpPath = tmpName
	v.Update(handoff.HandoffMsg{ //nolint:errcheck
		Tag:    promptEditorTag,
		Result: handoff.HandoffResult{},
	})

	_, statErr := os.Stat(tmpName)
	assert.True(t, os.IsNotExist(statErr), "temp file should be removed after handoff handling")
}

// 21. handoff.HandoffMsg with correct tag and error removes tmp file.
func TestPromptsView_HandoffMsg_Error_TempFileCleanedUp(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0

	tmpFile, err := os.CreateTemp("", "handoff-err-cleanup-*.mdx")
	require.NoError(t, err)
	_ = tmpFile.Close()
	tmpName := tmpFile.Name()

	v.tmpPath = tmpName
	v.Update(handoff.HandoffMsg{ //nolint:errcheck
		Tag:    promptEditorTag,
		Result: handoff.HandoffResult{Err: errors.New("editor crashed")},
	})

	_, statErr := os.Stat(tmpName)
	assert.True(t, os.IsNotExist(statErr), "temp file should be removed after handoff error")
}

// 22. handoff.HandoffMsg with same content as cached source does not set dirty.
func TestPromptsView_HandoffMsg_UnchangedContent_NotDirty(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	loaded := testLoadedPrompt("review")
	v.loadedSources["review"] = loaded
	v.cursor = 0
	v.enterEditMode()

	// Write the exact same content the cache holds.
	tmpFile, err := os.CreateTemp("", "handoff-nodirty-*.mdx")
	require.NoError(t, err)
	_, err = tmpFile.WriteString(loaded.Source)
	require.NoError(t, err)
	_ = tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	v.tmpPath = tmpFile.Name()
	updated, _ := v.Update(handoff.HandoffMsg{
		Tag:    promptEditorTag,
		Result: handoff.HandoffResult{},
	})
	pv := updated.(*PromptsView)
	assert.False(t, pv.dirty, "dirty flag should not be set when external editor makes no changes")
}

// ============================================================
// feat-prompts-props-discovery: 'p' key
// ============================================================

// TestPromptsView_PKey_TriggersPropDiscovery verifies 'p' dispatches a props discovery cmd.
func TestPromptsView_PKey_TriggersPropDiscovery(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'p'})
	pv := updated.(*PromptsView)
	assert.True(t, pv.discoveringProps, "'p' should set discoveringProps = true")
	assert.NotNil(t, cmd, "'p' should return a discovery command")
}

// TestPromptsView_PKey_NoopWhenNoPrompts verifies 'p' is a no-op with empty list.
func TestPromptsView_PKey_NoopWhenNoPrompts(t *testing.T) {
	v := newPromptsView()
	v.loading = false
	v.prompts = []smithers.Prompt{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'p'})
	assert.Nil(t, cmd, "'p' with empty list should return nil")
}

// TestPromptsView_PropsDiscoveredMsg_StoresProps verifies props are stored.
func TestPromptsView_PropsDiscoveredMsg_StoresProps(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.discoveringProps = true

	props := []smithers.PromptProp{
		{Name: "language", Type: "string"},
		{Name: "detail", Type: "string"},
	}
	updated, cmd := v.Update(promptPropsDiscoveredMsg{promptID: "review", props: props})
	pv := updated.(*PromptsView)

	assert.False(t, pv.discoveringProps, "discoveringProps should be false after msg")
	assert.Nil(t, pv.discoverPropsErr)
	assert.Nil(t, cmd)
	require.Len(t, pv.discoveredProps, 2)
	assert.Equal(t, "language", pv.discoveredProps[0].Name)
	assert.Equal(t, "detail", pv.discoveredProps[1].Name)
}

// TestPromptsView_PropsDiscoveredMsg_Error verifies error is stored.
func TestPromptsView_PropsDiscoveredMsg_Error(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false
	v.discoveringProps = true

	someErr := errors.New("server unavailable")
	updated, cmd := v.Update(promptPropsDiscoveredMsg{promptID: "review", err: someErr})
	pv := updated.(*PromptsView)

	assert.False(t, pv.discoveringProps)
	assert.Equal(t, someErr, pv.discoverPropsErr)
	assert.Nil(t, cmd)
}

// TestPromptsView_PropsDiscoveredMsg_PopulatesPropValues verifies prop values initialized.
func TestPromptsView_PropsDiscoveredMsg_PopulatesPropValues(t *testing.T) {
	v := newPromptsView()
	v.prompts = testPrompts()
	v.loading = false

	props := []smithers.PromptProp{
		{Name: "repo", Type: "string"},
	}
	v.Update(promptPropsDiscoveredMsg{promptID: "review", props: props}) //nolint:errcheck
	_, exists := v.propValues["repo"]
	assert.True(t, exists, "propValues should have a key for each discovered prop")
}

// TestPromptsView_View_DiscoverPropsShownInDetail verifies discovered props override in detail.
func TestPromptsView_View_DiscoverPropsShownInDetail(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.discoveredProps = []smithers.PromptProp{
		{Name: "repo", Type: "string"},
		{Name: "branch", Type: "string"},
	}

	out := v.View()
	assert.Contains(t, out, "repo")
	assert.Contains(t, out, "branch")
	assert.Contains(t, out, "discovered")
}

// TestPromptsView_ShortHelp_ContainsDiscoverProps verifies 'p' appears in help.
func TestPromptsView_ShortHelp_ContainsDiscoverProps(t *testing.T) {
	v := newPromptsView()
	v.focus = focusList
	var descs []string
	for _, b := range v.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "discover props")
}

// ============================================================
// feat-prompts-live-preview: 'v' key
// ============================================================

// TestPromptsView_VKey_EntersPreviewFocus verifies 'v' sets focus to preview.
func TestPromptsView_VKey_EntersPreviewFocus(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'v'})
	pv := updated.(*PromptsView)
	assert.Equal(t, focusPreview, pv.focus, "'v' should switch focus to preview")
	assert.True(t, pv.previewLoading, "preview should start loading")
	assert.NotNil(t, cmd, "'v' should dispatch a preview command")
}

// TestPromptsView_VKey_TogglesPreviewOff verifies second 'v' exits preview.
func TestPromptsView_VKey_TogglesPreviewOff(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusPreview

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'v'})
	pv := updated.(*PromptsView)
	assert.Equal(t, focusList, pv.focus, "second 'v' should return to list focus")
}

// TestPromptsView_PreviewMsg_StoresResult verifies preview result is stored.
func TestPromptsView_PreviewMsg_StoresResult(t *testing.T) {
	v := newPromptsView()
	v.loading = false
	v.prompts = testPrompts()
	v.previewLoading = true

	updated, cmd := v.Update(promptPreviewMsg{promptID: "review", rendered: "Hello world!"})
	pv := updated.(*PromptsView)

	assert.False(t, pv.previewLoading)
	assert.Nil(t, pv.previewErr)
	assert.Nil(t, cmd)
	assert.Equal(t, "Hello world!", pv.previewText)
}

// TestPromptsView_PreviewMsg_Error verifies preview error is stored.
func TestPromptsView_PreviewMsg_Error(t *testing.T) {
	v := newPromptsView()
	v.loading = false
	v.prompts = testPrompts()
	v.previewLoading = true

	someErr := errors.New("render failed")
	updated, cmd := v.Update(promptPreviewMsg{promptID: "review", err: someErr})
	pv := updated.(*PromptsView)

	assert.False(t, pv.previewLoading)
	assert.Equal(t, someErr, pv.previewErr)
	assert.Nil(t, cmd)
	assert.Empty(t, pv.previewText)
}

// TestPromptsView_View_PreviewFocusRendersPreviewPane verifies preview pane content.
func TestPromptsView_View_PreviewFocusRendersPreviewPane(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusPreview
	v.previewText = "Rendered output here"

	out := v.View()
	assert.Contains(t, out, "Preview")
	assert.Contains(t, out, "Rendered output here")
}

// TestPromptsView_View_PreviewLoadingState verifies loading message in preview.
func TestPromptsView_View_PreviewLoadingState(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusPreview
	v.previewLoading = true

	out := v.View()
	assert.Contains(t, out, "Loading preview...")
}

// TestPromptsView_EscFromPreview_ReturnsList verifies Esc exits preview mode.
func TestPromptsView_EscFromPreview_ReturnsList(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.cursor = 0
	v.focus = focusPreview

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	pv := updated.(*PromptsView)
	assert.Equal(t, focusList, pv.focus, "Esc from preview should return to list focus")
	assert.Nil(t, cmd, "Esc from preview should not emit a command")
}

// TestPromptsView_PreviewRefresh_RKey verifies 'r' in preview mode re-triggers preview.
func TestPromptsView_PreviewRefresh_RKey(t *testing.T) {
	v := newPromptsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.prompts = testPrompts()
	v.loadedSources["review"] = testLoadedPrompt("review")
	v.cursor = 0
	v.focus = focusPreview

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	assert.NotNil(t, cmd, "'r' in preview mode should return a preview command")
}

// TestPromptsView_ShortHelp_ContainsPreview verifies 'v' appears in help.
func TestPromptsView_ShortHelp_ContainsPreview(t *testing.T) {
	v := newPromptsView()
	v.focus = focusList
	var descs []string
	for _, b := range v.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "preview")
}

// TestPromptsView_ShortHelp_PreviewFocusHelp verifies preview-mode help differs.
func TestPromptsView_ShortHelp_PreviewFocusHelp(t *testing.T) {
	v := newPromptsView()
	v.focus = focusPreview
	var descs []string
	for _, b := range v.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "refresh preview")
	assert.Contains(t, joined, "close preview")
}
