package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChangeManager struct {
	repo    *jjhub.Repo
	changes []jjhub.Change
	details map[string]jjhub.Change
	diffs   map[string]string
	status  string
}

func (m *mockChangeManager) GetCurrentRepo() (*jjhub.Repo, error) {
	if m.repo != nil {
		return m.repo, nil
	}
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockChangeManager) ListChanges(limit int) ([]jjhub.Change, error) {
	return append([]jjhub.Change(nil), m.changes[:min(limit, len(m.changes))]...), nil
}

func (m *mockChangeManager) ViewChange(changeID string) (*jjhub.Change, error) {
	if change, ok := m.details[changeID]; ok {
		return &change, nil
	}
	return &jjhub.Change{ChangeID: changeID}, nil
}

func (m *mockChangeManager) ChangeDiff(changeID string) (string, error) {
	return m.diffs[changeID], nil
}

func (m *mockChangeManager) Status() (string, error) {
	return m.status, nil
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
