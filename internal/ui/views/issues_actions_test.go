package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockIssueManager struct {
	issues []jjhub.Issue

	createdTitle string
	createdBody  string
	closedNumber int
	closedBody   string
}

func (m *mockIssueManager) GetCurrentRepo() (*jjhub.Repo, error) {
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockIssueManager) ListIssues(state string, limit int) ([]jjhub.Issue, error) {
	return append([]jjhub.Issue(nil), m.issues[:min(limit, len(m.issues))]...), nil
}

func (m *mockIssueManager) CreateIssue(title, body string) (*jjhub.Issue, error) {
	m.createdTitle = title
	m.createdBody = body
	issue := jjhub.Issue{
		Number: 101,
		Title:  title,
		Body:   body,
		State:  "open",
	}
	m.issues = append([]jjhub.Issue{issue}, m.issues...)
	return &issue, nil
}

func (m *mockIssueManager) CloseIssue(number int, comment string) (*jjhub.Issue, error) {
	m.closedNumber = number
	m.closedBody = comment
	return &jjhub.Issue{
		Number: number,
		Title:  "Existing issue",
		State:  "closed",
	}, nil
}

func (m *mockIssueManager) ViewIssue(number int) (*jjhub.Issue, error) {
	for _, issue := range m.issues {
		if issue.Number == number {
			issueCopy := issue
			return &issueCopy, nil
		}
	}
	return &jjhub.Issue{Number: number}, nil
}

func TestIssuesView_CreateIssuePrompt(t *testing.T) {
	t.Parallel()

	manager := &mockIssueManager{}
	v := newIssuesViewWithClient(manager)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	iv := updated.(*IssuesView)
	require.True(t, iv.prompt.active)

	iv.prompt.titleInput.SetValue("Add JJHub status panel")
	iv.prompt.bodyInput.SetValue("Show working copy status in the TUI.")

	updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv = updated.(*IssuesView)
	require.NotNil(t, cmd)

	updated, cmd = iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv = updated.(*IssuesView)
	require.NotNil(t, cmd)

	updated, refreshCmd := iv.Update(cmd())
	iv = updated.(*IssuesView)
	require.NotNil(t, refreshCmd)
	assert.False(t, iv.prompt.active)
	assert.Equal(t, "Add JJHub status panel", manager.createdTitle)
	assert.Equal(t, "Show working copy status in the TUI.", manager.createdBody)
	assert.Contains(t, iv.actionMsg, "Created #101")
}

func TestIssuesView_CloseIssuePrompt(t *testing.T) {
	t.Parallel()

	manager := &mockIssueManager{
		issues: []jjhub.Issue{{
			Number: 7,
			Title:  "Existing issue",
			State:  "open",
		}},
	}

	v := newIssuesViewWithClient(manager)
	v.issues = manager.issues

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'x'})
	iv := updated.(*IssuesView)
	require.True(t, iv.prompt.active)

	iv.prompt.bodyInput.SetValue("Closing from the TUI.")

	updated, cmd := iv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv = updated.(*IssuesView)
	require.NotNil(t, cmd)

	updated, refreshCmd := iv.Update(cmd())
	iv = updated.(*IssuesView)
	require.NotNil(t, refreshCmd)
	assert.False(t, iv.prompt.active)
	assert.Equal(t, 7, manager.closedNumber)
	assert.Equal(t, "Closing from the TUI.", manager.closedBody)
	assert.Contains(t, iv.actionMsg, "Closed #7")
}
