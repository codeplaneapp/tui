package jjhub

import (
	"context"
	"errors"
	"fmt"
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
	assert.Equal(t, []string{"-R", "owner/repo"}, args)
}

func TestClient_RepoArgs_Empty(t *testing.T) {
	c := NewClient("")
	args := c.repoArgs()
	assert.Nil(t, args)
}

func TestClient_RepoArgs_PreservesExactValue(t *testing.T) {
	tests := []struct {
		repo string
		want []string
	}{
		{"owner/repo", []string{"-R", "owner/repo"}},
		{"org/my-project", []string{"-R", "org/my-project"}},
		{"a/b", []string{"-R", "a/b"}},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			c := NewClient(tt.repo)
			assert.Equal(t, tt.want, c.repoArgs())
		})
	}
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

func TestClient_CreateIssue_TabNewlineTitle(t *testing.T) {
	c := NewClient("owner/repo")
	issue, err := c.CreateIssue(context.Background(), "\t\n", "body")
	assert.Nil(t, issue)
	require.Error(t, err)
	assert.Equal(t, "title must not be empty", err.Error())
}

func TestClient_CreateIssue_TitleValidation(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"empty", "", "title must not be empty"},
		{"spaces", "   ", "title must not be empty"},
		{"tabs", "\t\t", "title must not be empty"},
		{"newlines", "\n\n", "title must not be empty"},
		{"mixed whitespace", " \t \n ", "title must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient("owner/repo")
			issue, err := c.CreateIssue(context.Background(), tt.title, "body")
			assert.Nil(t, issue)
			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())
		})
	}
}

func TestParseCLIError_NilError(t *testing.T) {
	// When err is nil, parseCLIError should return nil regardless of output.
	assert.NoError(t, parseCLIError([]byte("some output"), nil))
	assert.NoError(t, parseCLIError(nil, nil))
	assert.NoError(t, parseCLIError([]byte(""), nil))
}

func TestParseCLIError_WithErrorPrefix(t *testing.T) {
	// When output contains "Error: <msg>", parseCLIError extracts <msg>.
	err := parseCLIError(
		[]byte("some preamble\nError: repository not found"),
		errors.New("exit status 1"),
	)
	require.Error(t, err)
	assert.Equal(t, "repository not found", err.Error())
}

func TestParseCLIError_WithErrorPrefixNoSpace(t *testing.T) {
	err := parseCLIError(
		[]byte("Error:access denied"),
		errors.New("exit status 1"),
	)
	require.Error(t, err)
	assert.Equal(t, "access denied", err.Error())
}

func TestParseCLIError_EmptyOutput_FallsBackToErr(t *testing.T) {
	// When output is empty (after trim), parseCLIError returns the original error.
	origErr := errors.New("exit status 1")
	err := parseCLIError([]byte(""), origErr)
	require.Error(t, err)
	assert.Equal(t, origErr, err)
}

func TestParseCLIError_WhitespaceOnlyOutput_FallsBackToErr(t *testing.T) {
	origErr := errors.New("exit status 1")
	err := parseCLIError([]byte("   \n  "), origErr)
	require.Error(t, err)
	assert.Equal(t, origErr, err)
}

func TestParseCLIError_OutputWithoutErrorPrefix(t *testing.T) {
	// When output has content but no "Error:" prefix, the full trimmed output
	// becomes the error message.
	err := parseCLIError(
		[]byte("something went wrong\n"),
		errors.New("exit status 1"),
	)
	require.Error(t, err)
	assert.Equal(t, "something went wrong", err.Error())
}

func TestParseCLIError_ErrorAtStartOfOutput(t *testing.T) {
	err := parseCLIError(
		[]byte("Error: not authenticated"),
		errors.New("exit status 1"),
	)
	require.Error(t, err)
	assert.Equal(t, "not authenticated", err.Error())
}

func TestParseCLIError_MultipleErrorPrefixes(t *testing.T) {
	// When multiple "Error:" appear, it extracts from the first one.
	err := parseCLIError(
		[]byte("Error: first problem\nError: second problem"),
		errors.New("exit status 1"),
	)
	require.Error(t, err)
	assert.Equal(t, "first problem\nError: second problem", err.Error())
}

func TestDecodeJSON_ValidData(t *testing.T) {
	data := []byte(`{"id": 1, "login": "alice"}`)
	result, err := decodeJSON[User](data, "user")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "alice", result.Login)
}

func TestDecodeJSON_EmptyInput(t *testing.T) {
	result, err := decodeJSON[User]([]byte(""), "user")
	assert.Nil(t, result)
	require.ErrorIs(t, err, errEmptyCLIResponse)
}

func TestDecodeJSON_WhitespaceInput(t *testing.T) {
	result, err := decodeJSON[User]([]byte("   \n  "), "user")
	assert.Nil(t, result)
	require.ErrorIs(t, err, errEmptyCLIResponse)
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	result, err := decodeJSON[User]([]byte("{invalid}"), "user")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse user:")
}

func TestDecodeJSON_WrongShape(t *testing.T) {
	// Valid JSON but fields don't match -- still parses (zero values for missing fields).
	data := []byte(`{"foo": "bar"}`)
	result, err := decodeJSON[User](data, "user")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ID)
	assert.Equal(t, "", result.Login)
}

func TestDecodeJSON_NestedStruct(t *testing.T) {
	data := []byte(`{
		"landing": {"number": 42, "title": "test"},
		"changes": [{"id": 1, "change_id": "abc"}],
		"conflicts": {"has_conflicts": false},
		"reviews": []
	}`)
	result, err := decodeJSON[LandingDetail](data, "landing detail")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 42, result.Landing.Number)
	assert.Equal(t, "test", result.Landing.Title)
	assert.Len(t, result.Changes, 1)
	assert.Equal(t, "abc", result.Changes[0].ChangeID)
	assert.False(t, result.Conflicts.HasConflicts)
}

func TestClient_Run_EmptyOutput(t *testing.T) {
	writeFakeJJHub(t, "#!/bin/sh\nexit 0\n")

	c := NewClient("owner/repo")
	out, err := c.run("repo", "view")
	assert.Nil(t, out)
	require.ErrorIs(t, err, errEmptyCLIResponse)
}

func TestClient_Run_WhitespaceOnlyOutput(t *testing.T) {
	writeFakeJJHub(t, "#!/bin/sh\nprintf '   \\n'\n")

	c := NewClient("owner/repo")
	out, err := c.run("repo", "view")
	assert.Nil(t, out)
	require.ErrorIs(t, err, errEmptyCLIResponse)
}

func TestClient_GetCurrentRepo_WithContext(t *testing.T) {
	writeFakeJJHub(t, "#!/bin/sh\nprintf '{\"full_name\":\"acme/repo\"}'\n")

	c := NewClient("owner/repo")
	repo, err := c.GetCurrentRepo(context.Background())
	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, "acme/repo", repo.FullName)
}

func TestClient_GetCurrentRepo_ContextCanceled(t *testing.T) {
	writeFakeJJHub(t, "#!/bin/sh\nsleep 1\nprintf '{\"full_name\":\"acme/repo\"}'\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := NewClient("owner/repo")
	repo, err := c.GetCurrentRepo(ctx)
	assert.Nil(t, repo)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClient_ChangeDiff_ContextCanceled(t *testing.T) {
	writeFakeJJHub(t, "#!/bin/sh\nsleep 1\nprintf 'diff output'\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := NewClient("owner/repo")
	diff, err := c.ChangeDiff(ctx, "abc123")
	assert.Empty(t, diff)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestStructTypes(t *testing.T) {
	// Ensure public types are zero-constructable.
	var landing Landing
	assert.Zero(t, landing.Number)
	assert.Empty(t, landing.Title)

	var issue Issue
	assert.Zero(t, issue.Number)
	assert.Empty(t, issue.Title)

	var repo Repo
	assert.Empty(t, repo.FullName)

	var ws Workspace
	assert.Empty(t, ws.ID)
	assert.Equal(t, 0, ws.IdleTimeoutSeconds)

	var snap WorkspaceSnapshot
	assert.Empty(t, snap.SnapshotID)

	var wf Workflow
	assert.False(t, wf.IsActive)

	var change Change
	assert.Empty(t, change.ChangeID)
	assert.False(t, change.IsEmpty)
	assert.False(t, change.IsWorkingCopy)

	var notif Notification
	assert.False(t, notif.Unread)

	_ = fmt.Sprintf("suppress unused import: %v", errors.New("ok"))
}

func writeFakeJJHub(t *testing.T, script string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("Fake jjhub helper uses a POSIX shell script.")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "jjhub")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
