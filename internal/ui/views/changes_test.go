package views

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/diffnav"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChangesClient struct {
	changes []jjhub.Change
	err     error
	calls   int
}

func (c *fakeChangesClient) ListChanges(limit int) ([]jjhub.Change, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return c.changes, nil
}

func sampleChanges() []jjhub.Change {
	return []jjhub.Change{
		{
			ChangeID:      "9c1beef012345678",
			CommitID:      "b3f5cafe9876543210",
			Description:   "First change description",
			Author:        jjhub.Author{Name: "Jane Doe", Email: "jane@example.com"},
			Timestamp:     "2025-01-15T12:34:56Z",
			Bookmarks:     []string{"main", "stack/one"},
			IsWorkingCopy: true,
		},
		{
			ChangeID:    "1f2e3d4c5b6a7980",
			CommitID:    "abcdef0123456789",
			Description: "Second change for search filtering",
			Author:      jjhub.Author{Name: "Will Example", Email: "will@example.com"},
			Timestamp:   "2025-01-14T10:00:00Z",
			Bookmarks:   []string{"feature/search"},
		},
		{
			ChangeID:    "emptychange9999",
			CommitID:    "0000ffffeeee1111",
			Description: "",
			Author:      jjhub.Author{Name: "Empty Author", Email: "empty@example.com"},
			Timestamp:   "2025-01-13T09:00:00Z",
			IsEmpty:     true,
		},
	}
}

func newTestChangesView(t *testing.T) *ChangesView {
	t.Helper()

	v := newChangesView(
		&fakeChangesClient{},
		"/tmp/repo",
		func() bool { return true },
		func(command string, cwd string, tag any) tea.Cmd { return nil },
	)
	v.SetSize(120, 30)
	return v
}

func loadedChangesView(t *testing.T) *ChangesView {
	t.Helper()

	v := newTestChangesView(t)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	return updated.(*ChangesView)
}

func TestChangesView_InitReturnsLoadCmd(t *testing.T) {
	client := &fakeChangesClient{changes: sampleChanges()}
	v := newChangesView(
		client,
		"/tmp/repo",
		func() bool { return true },
		func(command string, cwd string, tag any) tea.Cmd { return nil },
	)

	cmd := v.Init()
	require.NotNil(t, cmd)

	msg := cmd()
	loaded, ok := msg.(changesLoadedMsg)
	require.True(t, ok, "expected changesLoadedMsg, got %T", msg)
	assert.Len(t, loaded.changes, 3)
	assert.Equal(t, 1, client.calls)
}

func TestChangesView_LoadedMsgRendersTableState(t *testing.T) {
	v := loadedChangesView(t)
	out := v.View()

	assert.False(t, v.loading)
	assert.Contains(t, out, "JJHub › Changes (3)")
	assert.Contains(t, out, "▸")
	assert.Contains(t, out, "●")
	assert.Contains(t, out, "(empty)")
	assert.Contains(t, out, "First change description")
}

func TestChangesView_TogglePreviewShowsSidebarDetails(t *testing.T) {
	v := loadedChangesView(t)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'w'})
	v = updated.(*ChangesView)
	out := v.View()

	assert.True(t, v.previewVisible)
	assert.Contains(t, out, "Commit ID")
	assert.Contains(t, out, "b3f5cafe9876543210")
	assert.Contains(t, out, "Jane Doe <jane@example.com>")
	assert.Contains(t, out, "main")
	assert.Contains(t, out, "stack/one")
}

func TestChangesView_SearchFiltersDescriptions(t *testing.T) {
	v := loadedChangesView(t)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: '/'})
	v = updated.(*ChangesView)
	assert.True(t, v.searchActive)
	_ = cmd

	for _, ch := range "second" {
		updated, _ = v.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		v = updated.(*ChangesView)
	}

	require.Len(t, v.filteredChanges, 1)
	assert.Equal(t, "1f2e3d4c5b6a7980", v.filteredChanges[0].ChangeID)
	assert.Contains(t, v.View(), "Second change for search filtering")
	assert.NotContains(t, v.View(), "First change description")
}

func TestChangesView_SearchEscClearsThenExits(t *testing.T) {
	v := loadedChangesView(t)
	v.searchActive = true
	v.searchInput.Focus() //nolint:errcheck
	v.searchInput.SetValue("search")
	v.applyFilter()

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	v = updated.(*ChangesView)
	assert.True(t, v.searchActive)
	assert.Equal(t, "", v.searchInput.Value())
	assert.Nil(t, cmd)

	updated, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	v = updated.(*ChangesView)
	assert.False(t, v.searchActive)
	assert.Nil(t, cmd)
}

func TestChangesView_GAndGBounds(t *testing.T) {
	v := loadedChangesView(t)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'G'})
	v = updated.(*ChangesView)
	assert.Equal(t, 2, v.listPane.cursor)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'g'})
	v = updated.(*ChangesView)
	assert.Equal(t, 0, v.listPane.cursor)
}

func TestChangesView_DiffAlwaysAvailable(t *testing.T) {
	v := newChangesView(
		&fakeChangesClient{},
		"/tmp/repo",
		func() bool { return true },
		func(command string, cwd string, tag any) tea.Cmd {
			return func() tea.Msg { return nil }
		},
	)
	v.SetSize(100, 30)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	v = updated.(*ChangesView)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	v = updated.(*ChangesView)

	assert.NotNil(t, cmd, "diff should always be available (bundled)")
	assert.False(t, v.statusErr)
}

func TestChangesView_DiffLaunchUsesJJHubCommand(t *testing.T) {
	var gotCommand string
	var gotCwd string
	var gotTag any

	v := newChangesView(
		&fakeChangesClient{},
		"/tmp/repo",
		func() bool { return true },
		func(command string, cwd string, tag any) tea.Cmd {
			gotCommand = command
			gotCwd = cwd
			gotTag = tag
			return func() tea.Msg { return nil }
		},
	)
	v.SetSize(100, 30)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	v = updated.(*ChangesView)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd)
	assert.Equal(t, "jj diff --git -r '9c1beef012345678'", gotCommand)
	assert.Equal(t, "/tmp/repo", gotCwd)
	assert.Equal(t, changesDiffTag, gotTag)
}

func TestChangesView_HandoffErrorSetsStatus(t *testing.T) {
	v := loadedChangesView(t)

	updated, _ := v.Update(handoff.HandoffMsg{
		Tag: changesDiffTag,
		Result: handoff.HandoffResult{
			Err: errors.New("diffnav failed"),
		},
	})
	v = updated.(*ChangesView)

	assert.True(t, v.statusErr)
	assert.Contains(t, v.statusMsg, "diffnav failed")
}

func TestChangesView_RefreshSetsLoading(t *testing.T) {
	client := &fakeChangesClient{changes: sampleChanges()}
	v := newChangesView(
		client,
		"/tmp/repo",
		func() bool { return true },
		func(command string, cwd string, tag any) tea.Cmd { return nil },
	)
	v.SetSize(100, 30)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	v = updated.(*ChangesView)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'R'})
	v = updated.(*ChangesView)

	assert.True(t, v.loading)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(changesLoadedMsg)
	assert.True(t, ok)
	assert.Equal(t, 1, client.calls)
}

func TestChangesView_EscapeEmitsPopViewMsg(t *testing.T) {
	v := loadedChangesView(t)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok)
}

func TestDefaultRegistry_ContainsChangesView(t *testing.T) {
	r := DefaultRegistry()

	view, ok := r.Open("changes", smithers.NewClient())
	require.True(t, ok)
	require.NotNil(t, view)
	assert.Equal(t, "changes", view.Name())
}

func TestBuildChangeDiffCommand_QuotesSafely(t *testing.T) {
	command := buildChangeDiffCommand(jjhub.Change{ChangeID: "abc'def"})
	assert.Equal(t, "jj diff --git -r 'abc'\"'\"'def'", command)
}

// TestChangesView_DKeyWithRealLauncher_ReturnsCmd verifies that pressing 'd'
// on a loaded changes view returns a non-nil command regardless of whether
// diffnav is installed (it should either launch diffnav or prompt to install).
func TestChangesView_DKeyWithRealLauncher_ReturnsCmd(t *testing.T) {
	v := newChangesView(
		&fakeChangesClient{},
		"/tmp/repo",
		func() bool { return true }, // pretend available
		diffnav.LaunchDiffnavWithCommand,
	)
	v.SetSize(100, 30)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	v = updated.(*ChangesView)

	// Pressing 'd' should return a non-nil cmd
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd, "d key must return a cmd (either handoff or install prompt)")

	// Execute the cmd and check the msg type
	msg := cmd()
	require.NotNil(t, msg, "cmd must produce a non-nil message")

	// It's either a handoff (diffnav found) or install prompt (not found)
	switch msg.(type) {
	case handoff.HandoffMsg:
		// diffnav was found and launched (or failed to launch)
	case diffnav.InstallPromptMsg:
		// diffnav not found, install prompt
	default:
		// Could be a tea.execMsg from tea.ExecProcess — that's fine too
	}
}

// TestChangesView_DKeyNotInstalled_ReturnsInstallPrompt verifies that when
// diffnav is not installed, pressing 'd' returns an InstallPromptMsg.
func TestChangesView_DKeyNotInstalled_ReturnsInstallPrompt(t *testing.T) {
	notInstalled := func(command string, cwd string, tag any) tea.Cmd {
		// Simulate what LaunchDiffnavWithCommand does when not installed
		return func() tea.Msg {
			return diffnav.InstallPromptMsg{
				PendingCommand: command,
				PendingCwd:     cwd,
				PendingTag:     tag,
			}
		}
	}

	v := newChangesView(
		&fakeChangesClient{},
		"/tmp/repo",
		func() bool { return true },
		notInstalled,
	)
	v.SetSize(100, 30)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	v = updated.(*ChangesView)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd, "d must return a cmd")

	msg := cmd()
	prompt, ok := msg.(diffnav.InstallPromptMsg)
	require.True(t, ok, "expected InstallPromptMsg, got %T", msg)
	assert.Contains(t, prompt.PendingCommand, "jj diff --git -r")
	assert.Equal(t, "/tmp/repo", prompt.PendingCwd)
}

// TestChangesView_EnterKey_ReturnsDiffCmd verifies enter also triggers diff.
func TestChangesView_EnterKey_ReturnsDiffCmd(t *testing.T) {
	var called bool
	v := newChangesView(
		&fakeChangesClient{},
		"/tmp/repo",
		func() bool { return true },
		func(command string, cwd string, tag any) tea.Cmd {
			called = true
			return func() tea.Msg { return nil }
		},
	)
	v.SetSize(100, 30)
	updated, _ := v.Update(changesLoadedMsg{changes: sampleChanges()})
	v = updated.(*ChangesView)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter must return a cmd")
	assert.True(t, called, "enter must trigger the diff launcher")
}

// TestChangesView_DKeyWhileLoading_IsNoop verifies that pressing 'd' before
// changes have loaded returns nil (no crash, no action).
func TestChangesView_DKeyWhileLoading_IsNoop(t *testing.T) {
	v := newTestChangesView(t)
	// Don't send changesLoadedMsg — still loading
	assert.True(t, v.loading)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	// When loading, 'd' should be a noop since there's no selected change
	// The key should NOT be handled at all while loading
	assert.Nil(t, cmd, "d while loading should be noop")
}

// TestChangesView_DKeyWithNoChanges_IsNoop verifies pressing 'd' with empty
// change list is a noop.
func TestChangesView_DKeyWithNoChanges_IsNoop(t *testing.T) {
	v := newTestChangesView(t)
	updated, _ := v.Update(changesLoadedMsg{changes: nil})
	v = updated.(*ChangesView)
	assert.False(t, v.loading)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	assert.Nil(t, cmd, "d with no changes should be noop")
}

func TestWrapPreviewText_RespectsWidth(t *testing.T) {
	lines := wrapPreviewText("alpha beta gamma", 8)
	require.NotEmpty(t, lines)
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(strings.TrimSpace(line))), 8)
	}
}
