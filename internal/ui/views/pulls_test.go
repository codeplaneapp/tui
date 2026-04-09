package views

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	ghrepo "github.com/charmbracelet/crush/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPullRequestManager struct {
	repo       *ghrepo.Repo
	originRepo string
	pulls      []ghrepo.PullRequest

	createCalls []struct {
		title string
		body  string
		head  string
		base  string
		draft bool
	}
	commentCalls []struct {
		number int
		body   string
	}

	listErr    error
	createErr  error
	commentErr error
}

func (m *mockPullRequestManager) GetCurrentRepo(context.Context) (*ghrepo.Repo, error) {
	if m.repo != nil {
		return m.repo, nil
	}
	return &ghrepo.Repo{NameWithOwner: "acme/repo"}, nil
}

func (m *mockPullRequestManager) ListPullRequests(_ context.Context, state string, limit int) ([]ghrepo.PullRequest, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	filtered := make([]ghrepo.PullRequest, 0, len(m.pulls))
	for _, pr := range m.pulls {
		if state != "all" && !stringsEqualFoldOrMerged(state, pr.State) {
			continue
		}
		filtered = append(filtered, pr)
	}
	if limit < len(filtered) {
		filtered = filtered[:limit]
	}
	return append([]ghrepo.PullRequest(nil), filtered...), nil
}

func stringsEqualFoldOrMerged(filter, state string) bool {
	if filter == "merged" {
		return state == "MERGED" || state == "merged"
	}
	return equalFold(filter, state)
}

func equalFold(a, b string) bool {
	if len(a) == 0 || len(b) == 0 {
		return a == b
	}
	return strings.EqualFold(a, b)
}

func (m *mockPullRequestManager) CreatePullRequest(_ context.Context, title, body, head, base string, draft bool) (*ghrepo.PullRequest, error) {
	m.createCalls = append(m.createCalls, struct {
		title string
		body  string
		head  string
		base  string
		draft bool
	}{title: title, body: body, head: head, base: base, draft: draft})
	if m.createErr != nil {
		return nil, m.createErr
	}
	pr := ghrepo.PullRequest{Number: 11, Title: title, Body: body, State: "OPEN", IsDraft: draft, HeadRefName: head, BaseRefName: base, Author: ghrepo.User{Login: "tester"}}
	return &pr, nil
}

func (m *mockPullRequestManager) CommentPullRequest(_ context.Context, number int, body string) error {
	m.commentCalls = append(m.commentCalls, struct {
		number int
		body   string
	}{number: number, body: body})
	return m.commentErr
}

func (m *mockPullRequestManager) OriginRepository(context.Context) (string, error) {
	if m.originRepo != "" {
		return m.originRepo, nil
	}
	return "codeplaneapp/tui", nil
}

func TestPullRequestsView_EmptyStateAndCreateValidation(t *testing.T) {
	manager := &mockPullRequestManager{}
	v := newPullRequestsViewWithClient(manager)
	v.width = 100
	v.height = 30
	v.loading = false

	output := v.View()
	assert.Contains(t, output, "No pull requests found")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	pv := updated.(*PullRequestsView)
	require.True(t, pv.prompt.active)
	assert.Equal(t, pullRequestPromptCreateTitle, pv.prompt.kind)

	pv.prompt.input.SetValue("")
	updated, cmd := pv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pv = updated.(*PullRequestsView)
	assert.Nil(t, cmd)
	require.NotNil(t, pv.prompt.err)
	assert.Contains(t, pv.prompt.err.Error(), "Title must not be empty")
}

func TestPullRequestsView_CommentFlow(t *testing.T) {
	manager := &mockPullRequestManager{
		pulls: []ghrepo.PullRequest{{Number: 7, Title: "Ship it", State: "OPEN", HeadRefName: "feat/test", BaseRefName: "main", Author: ghrepo.User{Login: "acme"}}},
	}
	v := newPullRequestsViewWithClient(manager)
	v.pulls = manager.pulls
	v.loading = false

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'm'})
	pv := updated.(*PullRequestsView)
	require.True(t, pv.prompt.active)
	assert.Equal(t, pullRequestPromptComment, pv.prompt.kind)

	pv.prompt.input.SetValue("Looks good overall")
	updated, cmd := pv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pv = updated.(*PullRequestsView)
	require.NotNil(t, cmd)

	msg, ok := cmd().(pullRequestActionDoneMsg)
	require.True(t, ok)
	assert.Equal(t, 1, len(manager.commentCalls))
	assert.Equal(t, 7, manager.commentCalls[0].number)
	assert.Equal(t, "Looks good overall", manager.commentCalls[0].body)

	updated, refreshCmd := pv.Update(msg)
	pv = updated.(*PullRequestsView)
	assert.Contains(t, pv.actionMsg, "Commented on PR #7")
	require.NotNil(t, refreshCmd)
}

func TestPullRequestsView_ListError(t *testing.T) {
	v := newPullRequestsViewWithClient(&mockPullRequestManager{listErr: errors.New("gh unavailable")})
	updated, cmd := v.Update(pullRequestsErrorMsg{err: errors.New("gh unavailable")})
	pv := updated.(*PullRequestsView)
	assert.Nil(t, cmd)
	require.Error(t, pv.err)
	assert.Contains(t, pv.err.Error(), "gh unavailable")
}

func TestPullRequestsView_ShortHelp(t *testing.T) {
	v := newPullRequestsViewWithClient(&mockPullRequestManager{})
	keys := helpKeys(v.ShortHelp())
	assert.Contains(t, keys, "c")
	assert.Contains(t, keys, "m")
	assert.Contains(t, keys, "s")
}
