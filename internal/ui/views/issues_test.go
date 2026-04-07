package views

import (
	"context"
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
}

func (m *mockIssueManager) GetCurrentRepo(context.Context) (*jjhub.Repo, error) {
	if m.repo != nil {
		return m.repo, nil
	}
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockIssueManager) ListIssues(_ context.Context, state string, limit int) ([]jjhub.Issue, error) {
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
