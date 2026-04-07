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

type mockIssueManager struct {
	repo        *jjhub.Repo
	issues      []jjhub.Issue
	createCalls []struct {
		title string
		body  string
	}
	closeCalls []struct {
		number  int
		comment string
	}
	nextNumber int
	listErr    error
	viewErr    error
}

func (m *mockIssueManager) GetCurrentRepo(context.Context) (*jjhub.Repo, error) {
	if m.repo != nil {
		return m.repo, nil
	}
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockIssueManager) ListIssues(_ context.Context, state string, limit int) ([]jjhub.Issue, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	filtered := make([]jjhub.Issue, 0, len(m.issues))
	for _, issue := range m.issues {
		if state != "all" && issue.State != state {
			continue
		}
		filtered = append(filtered, issue)
	}
	if limit < len(filtered) {
		filtered = filtered[:limit]
	}
	return append([]jjhub.Issue(nil), filtered...), nil
}

func (m *mockIssueManager) ViewIssue(_ context.Context, number int) (*jjhub.Issue, error) {
	if m.viewErr != nil {
		return nil, m.viewErr
	}
	for _, issue := range m.issues {
		if issue.Number == number {
			copy := issue
			return &copy, nil
		}
	}
	return &jjhub.Issue{Number: number}, nil
}

func (m *mockIssueManager) CreateIssue(_ context.Context, title, body string) (*jjhub.Issue, error) {
	m.createCalls = append(m.createCalls, struct {
		title string
		body  string
	}{title: title, body: body})

	number := m.nextNumber
	if number == 0 {
		number = len(m.issues) + 1
	}

	issue := jjhub.Issue{
		Number: number,
		Title:  title,
		Body:   body,
		State:  "open",
		Author: jjhub.User{Login: "tester"},
	}
	m.issues = append([]jjhub.Issue{issue}, m.issues...)
	m.nextNumber = number + 1

	return &issue, nil
}

func (m *mockIssueManager) CloseIssue(_ context.Context, number int, comment string) (*jjhub.Issue, error) {
	m.closeCalls = append(m.closeCalls, struct {
		number  int
		comment string
	}{number: number, comment: comment})

	for i := range m.issues {
		if m.issues[i].Number == number {
			m.issues[i].State = "closed"
			copy := m.issues[i]
			return &copy, nil
		}
	}

	return &jjhub.Issue{Number: number, State: "closed"}, nil
}

func TestIssuesView_CreateFlowCreatesIssue(t *testing.T) {
	t.Parallel()

	manager := &mockIssueManager{
		issues: []jjhub.Issue{{
			Number: 4,
			Title:  "Existing closed issue",
			State:  "closed",
		}},
		nextNumber: 9,
	}

	v := newIssuesViewWithClient(manager)
	v.stateFilter = "closed"
	v.issues = manager.issues

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	iv := updated.(*IssuesView)
	require.True(t, iv.prompt.active)
	assert.Equal(t, issuePromptCreateTitle, iv.prompt.kind)

	iv.prompt.input.SetValue("Ship parity")
	updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv = updated.(*IssuesView)
	require.NotNil(t, cmd)
	assert.Equal(t, issuePromptCreateBody, iv.prompt.kind)
	assert.Equal(t, "Ship parity", iv.prompt.title)

	iv.prompt.input.SetValue("Bring issue actions into the TUI.")
	updated, cmd = iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv = updated.(*IssuesView)
	require.NotNil(t, cmd)

	msg, ok := cmd().(issueActionDoneMsg)
	require.True(t, ok)
	assert.Equal(t, 1, len(manager.createCalls))
	assert.Equal(t, "Ship parity", manager.createCalls[0].title)
	assert.Equal(t, "Bring issue actions into the TUI.", manager.createCalls[0].body)

	updated, refreshCmd := iv.Update(msg)
	iv = updated.(*IssuesView)
	assert.False(t, iv.prompt.active)
	assert.Equal(t, "open", iv.stateFilter)
	assert.Equal(t, 9, iv.pendingSelectIssue)
	assert.Contains(t, iv.actionMsg, "Created issue #9")
	require.NotNil(t, refreshCmd)
}

func TestIssuesView_CloseFlowClosesSelectedIssue(t *testing.T) {
	t.Parallel()

	manager := &mockIssueManager{
		issues: []jjhub.Issue{{
			Number: 7,
			Title:  "Fix dashboard route",
			State:  "open",
		}},
	}

	v := newIssuesViewWithClient(manager)
	v.issues = manager.issues

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'x'})
	iv := updated.(*IssuesView)
	require.True(t, iv.prompt.active)
	assert.Equal(t, issuePromptCloseComment, iv.prompt.kind)

	iv.prompt.input.SetValue("Addressed in the dashboard parity pass.")
	updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv = updated.(*IssuesView)
	require.NotNil(t, cmd)

	msg, ok := cmd().(issueActionDoneMsg)
	require.True(t, ok)
	assert.Equal(t, 1, len(manager.closeCalls))
	assert.Equal(t, 7, manager.closeCalls[0].number)
	assert.Equal(t, "Addressed in the dashboard parity pass.", manager.closeCalls[0].comment)

	updated, refreshCmd := iv.Update(msg)
	iv = updated.(*IssuesView)
	assert.False(t, iv.prompt.active)
	assert.Equal(t, 7, iv.pendingSelectIssue)
	assert.Contains(t, iv.actionMsg, "Closed issue #7")
	assert.Equal(t, "closed", manager.issues[0].State)
	require.NotNil(t, refreshCmd)
}

func TestIssuesView_DetailCacheAndErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("detail error stored in detailErr map", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{
			issues: []jjhub.Issue{{
				Number: 5,
				Title:  "Broken detail",
				State:  "open",
			}},
			viewErr: errors.New("network timeout"),
		}

		v := newIssuesViewWithClient(manager)
		v.issues = manager.issues

		// loadSelectedDetailCmd should fire a cmd that yields issueDetailErrorMsg
		cmd := v.loadSelectedDetailCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		errMsg, ok := msg.(issueDetailErrorMsg)
		require.True(t, ok)
		assert.Equal(t, 5, errMsg.number)
		assert.Contains(t, errMsg.err.Error(), "network timeout")

		// Process the error message through Update
		updated, _ := v.Update(errMsg)
		iv := updated.(*IssuesView)
		assert.False(t, iv.detailLoading[5])
		assert.NotNil(t, iv.detailErr[5])
		assert.Nil(t, iv.detailCache[5])
	})

	t.Run("cached detail skips reload", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{
			issues: []jjhub.Issue{{
				Number: 3,
				Title:  "Already cached",
				State:  "open",
			}},
		}

		v := newIssuesViewWithClient(manager)
		v.issues = manager.issues
		v.detailCache[3] = &jjhub.Issue{Number: 3, Title: "Already cached", Body: "cached body"}

		// With cache populated, loadSelectedDetailCmd should return nil (no reload)
		cmd := v.loadSelectedDetailCmd()
		assert.Nil(t, cmd)
	})

	t.Run("detail success clears error and populates cache", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{
			issues: []jjhub.Issue{{
				Number: 10,
				Title:  "Fetch detail",
				State:  "open",
				Body:   "detailed body",
			}},
		}

		v := newIssuesViewWithClient(manager)
		v.issues = manager.issues
		// Simulate a previous error
		v.detailErr[10] = errors.New("old error")

		cmd := v.loadSelectedDetailCmd()
		require.NotNil(t, cmd)

		msg := cmd()
		detailMsg, ok := msg.(issueDetailLoadedMsg)
		require.True(t, ok)
		assert.Equal(t, 10, detailMsg.number)

		updated, _ := v.Update(detailMsg)
		iv := updated.(*IssuesView)
		assert.NotNil(t, iv.detailCache[10])
		assert.Nil(t, iv.detailErr[10])
		assert.False(t, iv.detailLoading[10])
	})
}

func TestIssuesView_CycleStateFilter(t *testing.T) {
	t.Parallel()

	t.Run("cycles open -> closed -> all -> open", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{
			issues: []jjhub.Issue{
				{Number: 1, Title: "Open issue", State: "open"},
				{Number: 2, Title: "Closed issue", State: "closed"},
			},
		}

		v := newIssuesViewWithClient(manager)
		v.issues = manager.issues
		v.cursor = 1

		assert.Equal(t, "open", v.stateFilter)

		v.cycleStateFilter()
		assert.Equal(t, "closed", v.stateFilter)
		assert.Equal(t, 0, v.cursor, "cursor resets on state cycle")

		v.cycleStateFilter()
		assert.Equal(t, "all", v.stateFilter)
		assert.Equal(t, 0, v.cursor)

		v.cycleStateFilter()
		assert.Equal(t, "open", v.stateFilter)
	})

	t.Run("pressing s key cycles filter and triggers refresh", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{
			issues: []jjhub.Issue{
				{Number: 1, Title: "Issue", State: "open"},
			},
		}

		v := newIssuesViewWithClient(manager)
		v.issues = manager.issues

		updated, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
		iv := updated.(*IssuesView)
		assert.Equal(t, "closed", iv.stateFilter)
		require.NotNil(t, cmd, "refresh command should be returned")
		assert.True(t, iv.loading)
	})

	t.Run("unknown filter falls back to open", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{}
		v := newIssuesViewWithClient(manager)
		v.stateFilter = "unknown"

		v.cycleStateFilter()
		assert.Equal(t, "open", v.stateFilter)
	})
}

func TestIssuesView_EmptyTitleValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty title shows validation error", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{
			issues: []jjhub.Issue{{Number: 1, Title: "Existing", State: "open"}},
		}

		v := newIssuesViewWithClient(manager)
		v.issues = manager.issues

		// Open the create prompt
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
		iv := updated.(*IssuesView)
		require.True(t, iv.prompt.active)
		assert.Equal(t, issuePromptCreateTitle, iv.prompt.kind)

		// Submit with empty title
		iv.prompt.input.SetValue("")
		updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		iv = updated.(*IssuesView)

		// Should stay in title prompt with validation error, no cmd returned
		assert.Nil(t, cmd)
		assert.True(t, iv.prompt.active)
		assert.Equal(t, issuePromptCreateTitle, iv.prompt.kind)
		require.NotNil(t, iv.prompt.err)
		assert.Contains(t, iv.prompt.err.Error(), "Title must not be empty")
	})

	t.Run("whitespace-only title also rejected", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{}
		v := newIssuesViewWithClient(manager)
		v.issues = []jjhub.Issue{{Number: 1, Title: "Existing", State: "open"}}

		updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
		iv := updated.(*IssuesView)

		iv.prompt.input.SetValue("   ")
		updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		iv = updated.(*IssuesView)

		assert.Nil(t, cmd)
		assert.Equal(t, issuePromptCreateTitle, iv.prompt.kind)
		require.NotNil(t, iv.prompt.err)
		assert.Contains(t, iv.prompt.err.Error(), "Title must not be empty")
	})

	t.Run("valid title advances to body prompt", func(t *testing.T) {
		t.Parallel()

		manager := &mockIssueManager{}
		v := newIssuesViewWithClient(manager)
		v.issues = []jjhub.Issue{{Number: 1, Title: "Existing", State: "open"}}

		updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
		iv := updated.(*IssuesView)

		iv.prompt.input.SetValue("Valid title")
		updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		iv = updated.(*IssuesView)

		require.NotNil(t, cmd, "should return focus cmd for body input")
		assert.Equal(t, issuePromptCreateBody, iv.prompt.kind)
		assert.Equal(t, "Valid title", iv.prompt.title)
		assert.Nil(t, iv.prompt.err)
	})
}
