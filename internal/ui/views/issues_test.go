package views

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleIssue(number int, state, title string) jjhub.Issue {
	return jjhub.Issue{
		Number:       number,
		Title:        title,
		Body:         "## Details\n\n" + title,
		State:        state,
		Author:       jjhub.User{Login: "will"},
		Assignees:    []jjhub.User{{Login: "dev1"}},
		CommentCount: 3,
		Labels:       []jjhub.Label{{Name: "bug", Color: "#f87171"}},
		CreatedAt:    time.Now().Add(-4 * time.Hour).Format(time.RFC3339),
		UpdatedAt:    time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}
}

func newTestIssuesView() *IssuesView {
	return NewIssuesView(smithers.NewClient())
}

func seedIssuesView(v *IssuesView, issues []jjhub.Issue) *IssuesView {
	updated, _ := v.Update(issuesLoadedMsg{issues: issues})
	return updated.(*IssuesView)
}

func TestIssuesView_ImplementsView(t *testing.T) {
	t.Parallel()
	var _ View = (*IssuesView)(nil)
}

func TestIssuesView_FilterCycle(t *testing.T) {
	t.Parallel()

	v := seedIssuesView(newTestIssuesView(), []jjhub.Issue{
		sampleIssue(1, "open", "Open issue"),
		sampleIssue(2, "closed", "Closed issue"),
	})

	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	iv := updated.(*IssuesView)

	assert.Equal(t, "closed", iv.currentFilter())
	assert.Len(t, iv.issues, 1)
	assert.Equal(t, 2, iv.issues[0].Number)
}

func TestIssuesView_SearchApply(t *testing.T) {
	t.Parallel()

	v := seedIssuesView(newTestIssuesView(), []jjhub.Issue{
		sampleIssue(1, "open", "Alpha"),
		sampleIssue(2, "open", "Beta"),
	})
	v.search.active = true
	v.search.input.SetValue("alpha")

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	iv := updated.(*IssuesView)

	assert.Equal(t, "alpha", iv.searchQuery)
	assert.Len(t, iv.issues, 1)
	assert.Equal(t, "Alpha", iv.issues[0].Title)
}

func TestIssuesView_EnterReturnsDetailView(t *testing.T) {
	t.Parallel()

	v := seedIssuesView(newTestIssuesView(), []jjhub.Issue{sampleIssue(1, "open", "Alpha")})
	v.width = 120
	v.height = 40

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.IsType(t, &IssueDetailView{}, updated)
	require.NotNil(t, cmd)
}

func TestIssueDetailView_EscReturnsParent(t *testing.T) {
	t.Parallel()

	parent := seedIssuesView(newTestIssuesView(), []jjhub.Issue{sampleIssue(1, "open", "Alpha")})
	detail := NewIssueDetailView(parent, jjhub.NewClient(""), nil, styles.DefaultStyles(), sampleIssue(1, "open", "Alpha"), nil)
	detail.SetSize(120, 40)

	updated, cmd := detail.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	require.Nil(t, cmd)
	assert.Same(t, parent, updated)
}
