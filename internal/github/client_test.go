package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	c := NewClient("owner/repo")
	require.NotNil(t, c)
	assert.Equal(t, "owner/repo", c.repo)
}

func TestNewClient_Empty(t *testing.T) {
	c := NewClient("")
	require.NotNil(t, c)
	assert.Equal(t, "", c.repo)
}

func TestClient_RepoArgs_WithRepo(t *testing.T) {
	c := NewClient("owner/repo")
	args := c.repoArgs()
	assert.Equal(t, []string{"--repo", "owner/repo"}, args)
}

func TestClient_RepoArgs_Empty(t *testing.T) {
	c := NewClient("")
	args := c.repoArgs()
	assert.Nil(t, args)
}

func TestClient_ResolveRepo_WithRepo(t *testing.T) {
	c := NewClient("owner/repo")
	repo, err := c.resolveRepo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", repo)
}

func TestClient_CreateIssue_EmptyTitle(t *testing.T) {
	c := NewClient("owner/repo")
	issue, err := c.CreateIssue(context.Background(), "", "some body")
	assert.Nil(t, issue)
	require.Error(t, err)
	assert.Equal(t, "title must not be empty", err.Error())
}

func TestClient_CreateIssue_WhitespaceTitle(t *testing.T) {
	c := NewClient("owner/repo")
	issue, err := c.CreateIssue(context.Background(), "   ", "some body")
	assert.Nil(t, issue)
	require.Error(t, err)
	assert.Equal(t, "title must not be empty", err.Error())
}

func TestClient_CreateIssue_TabTitle(t *testing.T) {
	c := NewClient("owner/repo")
	issue, err := c.CreateIssue(context.Background(), "\t\n", "body")
	assert.Nil(t, issue)
	require.Error(t, err)
	assert.Equal(t, "title must not be empty", err.Error())
}

func TestClient_CreatePullRequest_EmptyTitle(t *testing.T) {
	c := NewClient("owner/repo")
	pr, err := c.CreatePullRequest(context.Background(), "", "body", "feature/test", "main", false)
	assert.Nil(t, pr)
	require.Error(t, err)
	assert.Equal(t, "title must not be empty", err.Error())
}

func TestClient_CreatePullRequest_EmptyHead(t *testing.T) {
	c := NewClient("owner/repo")
	pr, err := c.CreatePullRequest(context.Background(), "Add feature", "body", " ", "main", false)
	assert.Nil(t, pr)
	require.Error(t, err)
	assert.Equal(t, "head branch must not be empty", err.Error())
}

func TestClient_CommentPullRequest_ValidatesInputs(t *testing.T) {
	c := NewClient("owner/repo")
	err := c.CommentPullRequest(context.Background(), 0, "nice")
	require.Error(t, err)
	assert.Equal(t, "pull request number must be greater than zero", err.Error())

	err = c.CommentPullRequest(context.Background(), 7, "   ")
	require.Error(t, err)
	assert.Equal(t, "comment body must not be empty", err.Error())
}

func TestClient_ListIssues_DefaultState(t *testing.T) {
	c := NewClient("test/repo")
	assert.NotNil(t, c)
	assert.Equal(t, "test/repo", c.repo)
}

func TestClient_ListPullRequests_DefaultState(t *testing.T) {
	c := NewClient("test/repo")
	assert.NotNil(t, c)
	assert.Equal(t, "test/repo", c.repo)
}

func TestClient_RunError_TrimsOutput(t *testing.T) {
	c := NewClient("owner/repo")
	_, err := c.CreateIssue(context.Background(), "", "body")
	require.Error(t, err)
	assert.Equal(t, "title must not be empty", err.Error())
}

func TestClient_ResolveRepo_Empty_ShellsOut(t *testing.T) {
	c := NewClient("explicit/repo")
	repo, err := c.resolveRepo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "explicit/repo", repo)
}

func TestRunErrorFallback(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
		want  string
	}{
		{"empty title", "", "body", "title must not be empty"},
		{"whitespace title", "   ", "body", "title must not be empty"},
		{"newline title", "\n", "body", "title must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient("owner/repo")
			_, err := c.CreateIssue(context.Background(), tt.title, tt.body)
			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())
		})
	}
}

func TestClient_RepoArgs_PreservesExactValue(t *testing.T) {
	tests := []struct {
		repo string
		want []string
	}{
		{"owner/repo", []string{"--repo", "owner/repo"}},
		{"org/my-project", []string{"--repo", "org/my-project"}},
		{"a/b", []string{"--repo", "a/b"}},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			c := NewClient(tt.repo)
			assert.Equal(t, tt.want, c.repoArgs())
		})
	}
}

func TestClient_GetCurrentRepo_ContextCanceled(t *testing.T) {
	writeFakeGH(t, "#!/bin/sh\nsleep 1\nprintf '{\"nameWithOwner\":\"acme/repo\"}'\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := NewClient("owner/repo")
	repo, err := c.GetCurrentRepo(ctx)
	assert.Nil(t, repo)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClient_CreatePullRequest_Success(t *testing.T) {
	writeFakeGH(t, `#!/bin/sh
printf '%s\n' "$@" >"$TMPDIR/gh-args.txt"
cat <<'EOF'
{"number":12,"title":"Add bwrap workspace support","body":"Implements sandbox-backed attach flow.","state":"open","draft":true,"created_at":"2026-04-08T00:00:00Z","updated_at":"2026-04-08T00:00:00Z","html_url":"https://github.com/owner/repo/pull/12","user":{"login":"roninjin10"},"head":{"ref":"feat/test"},"base":{"ref":"main"}}
EOF
`)

	c := NewClient("owner/repo")
	pr, err := c.CreatePullRequest(context.Background(), "Add bwrap workspace support", "Implements sandbox-backed attach flow.", "feat/test", "main", true)
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 12, pr.Number)
	assert.Equal(t, "feat/test", pr.HeadRefName)
	assert.Equal(t, "main", pr.BaseRefName)
	assert.True(t, pr.IsDraft)
}

func TestClient_CommentPullRequest_Success(t *testing.T) {
	writeFakeGH(t, `#!/bin/sh
printf '%s\n' "$@" >"$TMPDIR/gh-args.txt"
cat <<'EOF'
{"id":1}
EOF
`)

	c := NewClient("owner/repo")
	err := c.CommentPullRequest(context.Background(), 12, "Please exercise the landing request flow in tmux.")
	require.NoError(t, err)
}

func TestStructTypes(t *testing.T) {
	var issue Issue
	assert.Zero(t, issue.Number)
	assert.Empty(t, issue.Title)

	var pr PullRequest
	assert.Zero(t, pr.Number)
	assert.False(t, pr.IsDraft)

	var user User
	assert.Empty(t, user.Login)
	assert.False(t, user.IsBot)

	var label Label
	assert.Empty(t, label.Name)

	var repo Repo
	assert.Empty(t, repo.NameWithOwner)

	_ = errors.New("unused")
}

func writeFakeGH(t *testing.T, script string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("Fake gh helper uses a POSIX shell script.")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMPDIR", dir)
}
