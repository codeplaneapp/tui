package views

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/stretchr/testify/require"
)

type mockSearchManager struct {
	repo   *jjhub.Repo
	repos  *jjhub.RepositorySearchPage
	issues *jjhub.IssueSearchPage
	code   *jjhub.CodeSearchPage

	repoErr   error
	reposErr  error
	issuesErr error
	codeErr   error
}

func (m *mockSearchManager) GetCurrentRepo(context.Context) (*jjhub.Repo, error) {
	if m.repoErr != nil {
		return nil, m.repoErr
	}
	if m.repo != nil {
		return m.repo, nil
	}
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockSearchManager) SearchRepositories(context.Context, string, int) (*jjhub.RepositorySearchPage, error) {
	if m.reposErr != nil {
		return nil, m.reposErr
	}
	if m.repos != nil {
		return m.repos, nil
	}
	return &jjhub.RepositorySearchPage{}, nil
}

func (m *mockSearchManager) SearchIssues(context.Context, string, string, int) (*jjhub.IssueSearchPage, error) {
	if m.issuesErr != nil {
		return nil, m.issuesErr
	}
	if m.issues != nil {
		return m.issues, nil
	}
	return &jjhub.IssueSearchPage{}, nil
}

func (m *mockSearchManager) SearchCode(context.Context, string, int) (*jjhub.CodeSearchPage, error) {
	if m.codeErr != nil {
		return nil, m.codeErr
	}
	if m.code != nil {
		return m.code, nil
	}
	return &jjhub.CodeSearchPage{}, nil
}

func configureSearchObservability(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))
}

func requireSearchSpanAttrs(t *testing.T, action, result string) map[string]any {
	t.Helper()

	for _, span := range observability.RecentSpans(20) {
		if span.Name != "ui.action" {
			continue
		}
		if span.Attributes["codeplane.ui.view"] == "search" &&
			span.Attributes["codeplane.ui.action"] == action &&
			span.Attributes["codeplane.ui.result"] == result {
			return span.Attributes
		}
	}

	t.Fatalf("missing search ui.action span action=%q result=%q", action, result)
	return nil
}

func TestSearchView_EmptyQueryRecordsError(t *testing.T) {
	configureSearchObservability(t)

	v := newSearchViewWithClient(&mockSearchManager{})
	v.input.SetValue("   ")

	msg := v.searchCmd()()
	updated, _ := v.Update(msg)
	sv := updated.(*SearchView)

	require.EqualError(t, sv.err, "Search query must not be empty")

	attrs := requireSearchSpanAttrs(t, "query", "error")
	require.Equal(t, "repos", attrs["codeplane.search.tab"])
	require.EqualValues(t, 0, attrs["codeplane.search.query_length"])
}

func TestSearchView_SearchCodeRecordsSuccess(t *testing.T) {
	configureSearchObservability(t)

	v := newSearchViewWithClient(&mockSearchManager{
		code: &jjhub.CodeSearchPage{
			Items: []jjhub.CodeSearchItem{{
				Repository: "acme/repo",
				FilePath:   "main.go",
				TextMatches: []jjhub.CodeTextMatch{{
					Content:    "needle",
					LineNumber: 42,
				}},
			}},
		},
	})
	v.tab = searchTabCode
	v.input.SetValue("needle")

	msg := v.searchCmd()()
	updated, _ := v.Update(msg)
	sv := updated.(*SearchView)

	require.True(t, sv.hasSearched)
	require.Len(t, sv.code.Items, 1)
	require.NoError(t, sv.err)

	attrs := requireSearchSpanAttrs(t, "query", "ok")
	require.Equal(t, "code", attrs["codeplane.search.tab"])
	require.EqualValues(t, 1, attrs["codeplane.search.result_count"])
}

func TestSearchView_BackendErrorRecordsFailure(t *testing.T) {
	configureSearchObservability(t)

	v := newSearchViewWithClient(&mockSearchManager{
		issuesErr: errors.New("issue search failed"),
	})
	v.tab = searchTabIssues
	v.input.SetValue("bug")

	msg := v.searchCmd()()
	updated, _ := v.Update(msg)
	sv := updated.(*SearchView)

	require.EqualError(t, sv.err, "issue search failed")

	attrs := requireSearchSpanAttrs(t, "query", "error")
	require.Equal(t, "issues", attrs["codeplane.search.tab"])
}

func TestSearchView_RefreshAndFocusTransitions(t *testing.T) {
	v := newSearchViewWithClient(&mockSearchManager{
		repos: &jjhub.RepositorySearchPage{
			Items: []jjhub.RepositorySearchItem{{FullName: "acme/repo"}},
		},
	})
	v.input.SetValue("acme")

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	sv := updated.(*SearchView)
	require.False(t, sv.inputFocused)

	updated, focusCmd := sv.Update(tea.KeyPressMsg{Code: '/'})
	sv = updated.(*SearchView)
	require.True(t, sv.inputFocused)
	require.NotNil(t, focusCmd)

	sv.inputFocused = false
	updated, refreshCmd := sv.Update(tea.KeyPressMsg{Code: 'r'})
	sv = updated.(*SearchView)
	require.True(t, sv.loading)
	require.NotNil(t, refreshCmd)

	msg := refreshCmd()
	updated, _ = sv.Update(msg)
	sv = updated.(*SearchView)
	require.True(t, sv.hasSearched)
	require.Len(t, sv.repos.Items, 1)
}
