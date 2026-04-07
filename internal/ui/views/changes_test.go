package views

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChangeManager struct {
	repo        *jjhub.Repo
	changes     []jjhub.Change
	details     map[string]jjhub.Change
	diffs       map[string]string
	status      string
	bookmarks   map[string]jjhub.Bookmark
	createCalls []struct {
		name     string
		changeID string
		remote   bool
	}
	deleteCalls []struct {
		name   string
		remote bool
	}
	listErr     error
	viewErr     error
	diffErr     error
	statusErr   error
	bookmarkErr error
}

func (m *mockChangeManager) GetCurrentRepo(context.Context) (*jjhub.Repo, error) {
	if m.repo != nil {
		return m.repo, nil
	}
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockChangeManager) ListChanges(_ context.Context, limit int) ([]jjhub.Change, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return append([]jjhub.Change(nil), m.changes[:min(limit, len(m.changes))]...), nil
}

func (m *mockChangeManager) ViewChange(_ context.Context, changeID string) (*jjhub.Change, error) {
	if m.viewErr != nil {
		return nil, m.viewErr
	}
	if change, ok := m.details[changeID]; ok {
		return &change, nil
	}
	return &jjhub.Change{ChangeID: changeID}, nil
}

func (m *mockChangeManager) ChangeDiff(_ context.Context, changeID string) (string, error) {
	if m.diffErr != nil {
		return "", m.diffErr
	}
	return m.diffs[changeID], nil
}

func (m *mockChangeManager) Status(context.Context) (string, error) {
	if m.statusErr != nil {
		return "", m.statusErr
	}
	return m.status, nil
}

func (m *mockChangeManager) CreateBookmark(_ context.Context, name, changeID string, remote bool) (*jjhub.Bookmark, error) {
	if m.bookmarkErr != nil {
		return nil, m.bookmarkErr
	}
	m.createCalls = append(m.createCalls, struct {
		name     string
		changeID string
		remote   bool
	}{name: name, changeID: changeID, remote: remote})
	bookmark := jjhub.Bookmark{
		Name:           name,
		TargetChangeID: changeID,
	}
	if m.bookmarks == nil {
		m.bookmarks = make(map[string]jjhub.Bookmark)
	}
	key := name
	if remote {
		key += "@remote"
	}
	m.bookmarks[key] = bookmark
	return &bookmark, nil
}

func (m *mockChangeManager) DeleteBookmark(_ context.Context, name string, remote bool) error {
	if m.bookmarkErr != nil {
		return m.bookmarkErr
	}
	m.deleteCalls = append(m.deleteCalls, struct {
		name   string
		remote bool
	}{name: name, remote: remote})
	key := name
	if remote {
		key += "@remote"
	}
	delete(m.bookmarks, key)
	return nil
}

func TestChangesView_DKeyLoadsDiff(t *testing.T) {
	t.Parallel()

	manager := &mockChangeManager{
		changes: []jjhub.Change{{
			ChangeID:    "abc123",
			Description: "Fix login flow",
		}},
		diffs: map[string]string{
			"abc123": "diff --git a/main.go b/main.go",
		},
	}

	v := newChangesViewWithClient("changes", changesModeList, manager)
	v.changes = manager.changes

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	cv := updated.(*ChangesView)

	require.NotNil(t, cmd)
	assert.True(t, cv.showDiff)

	updated, _ = cv.Update(cmd())
	cv = updated.(*ChangesView)
	assert.Equal(t, "diff --git a/main.go b/main.go", cv.diffCache["abc123"])
}

func TestChangesView_TabLoadsWorkingCopyStatus(t *testing.T) {
	t.Parallel()

	manager := &mockChangeManager{
		status: "Modified regular file: README.md",
		diffs: map[string]string{
			"": "diff --git a/README.md b/README.md",
		},
	}

	v := newChangesViewWithClient("changes", changesModeList, manager)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	cv := updated.(*ChangesView)

	require.NotNil(t, cmd)
	assert.Equal(t, changesModeStatus, cv.mode)

	updated, _ = cv.Update(cmd())
	cv = updated.(*ChangesView)
	assert.Equal(t, "Modified regular file: README.md", cv.statusText)
	assert.Equal(t, "diff --git a/README.md b/README.md", cv.workingDiff)
}

func TestChangesView_LoadErrors(t *testing.T) {
	t.Parallel()

	t.Run("changesErrorMsg sets err and clears loading", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			listErr: errors.New("connection refused"),
		}

		v := newChangesViewWithClient("changes", changesModeList, manager)

		// Execute the load command
		cmd := v.loadChangesCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		errMsg, ok := msg.(changesErrorMsg)
		require.True(t, ok)
		assert.Contains(t, errMsg.err.Error(), "connection refused")

		// Process through Update
		updated, retCmd := v.Update(errMsg)
		cv := updated.(*ChangesView)
		assert.False(t, cv.loading)
		assert.NotNil(t, cv.err)
		assert.Contains(t, cv.err.Error(), "connection refused")
		assert.Nil(t, retCmd)
	})

	t.Run("error renders in view output", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{}
		v := newChangesViewWithClient("changes", changesModeList, manager)
		v.loading = false
		v.err = errors.New("something went wrong")
		v.width = 80
		v.height = 24

		output := v.View()
		assert.Contains(t, output, "something went wrong")
	})

	t.Run("nil client sets error on construction", func(t *testing.T) {
		t.Parallel()

		v := newChangesViewWithClient("changes", changesModeList, nil)
		assert.NotNil(t, v.err)
		assert.Contains(t, v.err.Error(), "jjhub CLI not found")
	})
}

func TestChangesView_DetailAndDiffCacheMisses(t *testing.T) {
	t.Parallel()

	t.Run("detail error stored in detailErr map", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			changes: []jjhub.Change{{
				ChangeID:    "xyz789",
				Description: "Broken detail",
			}},
			viewErr: errors.New("detail fetch failed"),
		}

		v := newChangesViewWithClient("changes", changesModeList, manager)
		v.changes = manager.changes

		cmd := v.loadSelectedDetailCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		errMsg, ok := msg.(changeDetailErrorMsg)
		require.True(t, ok)
		assert.Equal(t, "xyz789", errMsg.changeID)

		updated, _ := v.Update(errMsg)
		cv := updated.(*ChangesView)
		assert.False(t, cv.detailLoading["xyz789"])
		assert.NotNil(t, cv.detailErr["xyz789"])
		assert.Nil(t, cv.detailCache["xyz789"])
	})

	t.Run("cached detail skips reload", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			changes: []jjhub.Change{{
				ChangeID:    "cached1",
				Description: "Already cached",
			}},
		}

		v := newChangesViewWithClient("changes", changesModeList, manager)
		v.changes = manager.changes
		v.detailCache["cached1"] = &jjhub.Change{ChangeID: "cached1"}

		cmd := v.loadSelectedDetailCmd()
		assert.Nil(t, cmd, "should not reload when detail is cached")
	})

	t.Run("diff error stored in diffErr map", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			changes: []jjhub.Change{{
				ChangeID:    "diff-fail",
				Description: "Diff error test",
			}},
			diffErr: errors.New("diff command failed"),
		}

		v := newChangesViewWithClient("changes", changesModeList, manager)
		v.changes = manager.changes

		cmd := v.loadSelectedDiffCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		errMsg, ok := msg.(changeDiffErrorMsg)
		require.True(t, ok)
		assert.Equal(t, "diff-fail", errMsg.changeID)

		updated, _ := v.Update(errMsg)
		cv := updated.(*ChangesView)
		assert.False(t, cv.diffLoading["diff-fail"])
		assert.NotNil(t, cv.diffErr["diff-fail"])
	})

	t.Run("cached diff skips reload", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			changes: []jjhub.Change{{
				ChangeID:    "diff-cached",
				Description: "Already has diff",
			}},
		}

		v := newChangesViewWithClient("changes", changesModeList, manager)
		v.changes = manager.changes
		v.diffCache["diff-cached"] = "some diff content"

		cmd := v.loadSelectedDiffCmd()
		assert.Nil(t, cmd, "should not reload when diff is cached")
	})
}

func TestChangesView_WorkingCopyErrorCombinations(t *testing.T) {
	t.Parallel()

	t.Run("status error only", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			statusErr: errors.New("status failed"),
			diffs:     map[string]string{"": "diff content"},
		}

		v := newChangesViewWithClient("status", changesModeStatus, manager)

		cmd := v.loadWorkingCopyCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		wcMsg, ok := msg.(workingCopyLoadedMsg)
		require.True(t, ok)
		assert.NotNil(t, wcMsg.statusErr)
		assert.Nil(t, wcMsg.diffErr)
		assert.Equal(t, "diff content", wcMsg.diff)

		updated, _ := v.Update(wcMsg)
		cv := updated.(*ChangesView)
		assert.False(t, cv.statusLoading)
		assert.NotNil(t, cv.statusErr)
		assert.Equal(t, "diff content", cv.workingDiff)
		assert.Nil(t, cv.workingDiffErr)
	})

	t.Run("diff error only", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			status:  "Working copy clean",
			diffErr: errors.New("diff failed"),
		}

		v := newChangesViewWithClient("status", changesModeStatus, manager)

		cmd := v.loadWorkingCopyCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		wcMsg, ok := msg.(workingCopyLoadedMsg)
		require.True(t, ok)
		assert.Nil(t, wcMsg.statusErr)
		assert.NotNil(t, wcMsg.diffErr)
		assert.Equal(t, "Working copy clean", wcMsg.status)

		updated, _ := v.Update(wcMsg)
		cv := updated.(*ChangesView)
		assert.False(t, cv.statusLoading)
		assert.Equal(t, "Working copy clean", cv.statusText)
		assert.Nil(t, cv.statusErr)
		assert.NotNil(t, cv.workingDiffErr)
	})

	t.Run("both errors", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{
			statusErr: errors.New("status broken"),
			diffErr:   errors.New("diff broken"),
		}

		v := newChangesViewWithClient("status", changesModeStatus, manager)

		cmd := v.loadWorkingCopyCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		wcMsg, ok := msg.(workingCopyLoadedMsg)
		require.True(t, ok)
		assert.NotNil(t, wcMsg.statusErr)
		assert.NotNil(t, wcMsg.diffErr)

		updated, _ := v.Update(wcMsg)
		cv := updated.(*ChangesView)
		assert.False(t, cv.statusLoading)
		assert.NotNil(t, cv.statusErr)
		assert.NotNil(t, cv.workingDiffErr)
	})

	t.Run("error text renders in status mode view", func(t *testing.T) {
		t.Parallel()

		manager := &mockChangeManager{}
		v := newChangesViewWithClient("status", changesModeStatus, manager)
		v.statusLoading = false
		v.statusErr = errors.New("cannot read status")
		v.workingDiffErr = errors.New("cannot read diff")
		v.width = 80
		v.height = 24

		output := v.View()
		assert.Contains(t, output, "cannot read status")
		assert.Contains(t, output, "cannot read diff")
	})
}

func TestChangesView_CreateBookmarkFlow(t *testing.T) {
	t.Parallel()

	manager := &mockChangeManager{
		changes: []jjhub.Change{{
			ChangeID:    "abc123",
			Description: "Fix login flow",
		}},
	}

	v := newChangesViewWithClient("changes", changesModeList, manager)
	v.changes = manager.changes

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'b'})
	cv := updated.(*ChangesView)
	require.True(t, cv.prompt.active)
	assert.Equal(t, changePromptCreateBookmark, cv.prompt.kind)

	cv.prompt.input.SetValue("ship/login-fix")
	updated, cmd := cv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cv = updated.(*ChangesView)
	require.NotNil(t, cmd)

	msg, ok := cmd().(changeActionDoneMsg)
	require.True(t, ok)

	updated, refreshCmd := cv.Update(msg)
	cv = updated.(*ChangesView)
	assert.False(t, cv.prompt.active)
	assert.Contains(t, cv.actionMsg, "Created bookmark")
	_ = refreshCmd
}

func TestChangesView_DeleteBookmarkFlow(t *testing.T) {
	t.Parallel()

	manager := &mockChangeManager{
		changes: []jjhub.Change{{
			ChangeID:    "abc123",
			Description: "Fix login flow",
			Bookmarks:   []string{"ship/login-fix"},
		}},
	}

	v := newChangesViewWithClient("changes", changesModeList, manager)
	v.changes = manager.changes

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'x'})
	cv := updated.(*ChangesView)
	require.True(t, cv.prompt.active)
	assert.Equal(t, "ship/login-fix", cv.prompt.input.Value())

	updated, cmd := cv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cv = updated.(*ChangesView)
	require.NotNil(t, cmd)

	msg, ok := cmd().(changeActionDoneMsg)
	require.True(t, ok)

	updated, refreshCmd := cv.Update(msg)
	cv = updated.(*ChangesView)
	assert.False(t, cv.prompt.active)
	assert.Contains(t, cv.actionMsg, "Deleted bookmark")
	_ = refreshCmd
}
