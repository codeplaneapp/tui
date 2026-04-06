package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBashPermissionService struct {
	*pubsub.Broker[permission.PermissionRequest]
}

func (m *mockBashPermissionService) Request(ctx context.Context, req permission.CreatePermissionRequest) (bool, error) {
	return true, nil
}

func (m *mockBashPermissionService) Grant(req permission.PermissionRequest) {}

func (m *mockBashPermissionService) Deny(req permission.PermissionRequest) {}

func (m *mockBashPermissionService) GrantPersistent(req permission.PermissionRequest) {}

func (m *mockBashPermissionService) AutoApproveSession(sessionID string) {}

func (m *mockBashPermissionService) SetSkipRequests(skip bool) {}

func (m *mockBashPermissionService) SkipRequests() bool {
	return false
}

func (m *mockBashPermissionService) SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[permission.PermissionNotification] {
	return make(<-chan pubsub.Event[permission.PermissionNotification])
}

func TestBashTool_DefaultAutoBackgroundThreshold(t *testing.T) {
	workingDir := t.TempDir()
	tool := newBashToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")

	resp := runBashTool(t, tool, ctx, BashParams{
		Description: "default threshold",
		Command:     "echo done",
	})

	require.False(t, resp.IsError)
	var meta BashResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.False(t, meta.Background)
	require.Empty(t, meta.ShellID)
	require.Contains(t, meta.Output, "done")
}

func TestBashTool_CustomAutoBackgroundThreshold(t *testing.T) {
	workingDir := t.TempDir()
	tool := newBashToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")

	resp := runBashTool(t, tool, ctx, BashParams{
		Description:         "custom threshold",
		Command:             "sleep 1.5 && echo done",
		AutoBackgroundAfter: 1,
	})

	require.False(t, resp.IsError)
	var meta BashResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.True(t, meta.Background)
	require.NotEmpty(t, meta.ShellID)
	require.Contains(t, resp.Content, "moved to background")

	bgManager := shell.GetBackgroundShellManager()
	require.NoError(t, bgManager.Kill(meta.ShellID))
}

func TestFormatOutput(t *testing.T) {
	t.Parallel()

	t.Run("stdout only, no error", func(t *testing.T) {
		t.Parallel()
		result := formatOutput("hello world", "", nil)
		assert.Equal(t, "hello world", result)
	})

	t.Run("empty stdout and no error returns empty", func(t *testing.T) {
		t.Parallel()
		result := formatOutput("", "", nil)
		assert.Equal(t, "", result)
	})

	t.Run("stderr appended when present", func(t *testing.T) {
		t.Parallel()
		result := formatOutput("output", "warning: something", nil)
		assert.Contains(t, result, "output")
		assert.Contains(t, result, "warning: something")
	})

	t.Run("exit code appended on non-zero exit", func(t *testing.T) {
		t.Parallel()
		// shell.ExitCode returns the exit code from exec.ExitError.
		// We can test with a non-nil error that is not an ExitError.
		// In that case, stderr should be shown as the error message.
		result := formatOutput("", "command not found", fmt.Errorf("exit status 127"))
		assert.Contains(t, result, "command not found")
	})
}

func TestTruncateOutput(t *testing.T) {
	t.Parallel()

	t.Run("short content returned unchanged", func(t *testing.T) {
		t.Parallel()
		content := "short output"
		result := truncateOutput(content)
		assert.Equal(t, content, result)
	})

	t.Run("exactly MaxOutputLength returned unchanged", func(t *testing.T) {
		t.Parallel()
		content := strings.Repeat("a", MaxOutputLength)
		result := truncateOutput(content)
		assert.Equal(t, content, result)
	})

	t.Run("content exceeding MaxOutputLength is truncated", func(t *testing.T) {
		t.Parallel()
		content := strings.Repeat("a", MaxOutputLength+100)
		result := truncateOutput(content)
		assert.Contains(t, result, "truncated")
		assert.Less(t, len(result), len(content))
		// Start and end preserved
		halfLength := MaxOutputLength / 2
		assert.True(t, strings.HasPrefix(result, content[:halfLength]))
		assert.True(t, strings.HasSuffix(result, content[len(content)-halfLength:]))
	})

	t.Run("empty content returned unchanged", func(t *testing.T) {
		t.Parallel()
		result := truncateOutput("")
		assert.Equal(t, "", result)
	})
}

func TestCountLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty string", "", 0},
		{"single line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"trailing newline", "hello\n", 2},
		{"multiple newlines", "a\nb\nc\nd", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, countLines(tt.input))
		})
	}
}

func TestNormalizeWorkingDir(t *testing.T) {
	t.Parallel()

	t.Run("unix path returned with forward slashes", func(t *testing.T) {
		t.Parallel()
		result := normalizeWorkingDir("/home/user/project")
		assert.Equal(t, "/home/user/project", result)
	})

	t.Run("empty path returns empty", func(t *testing.T) {
		t.Parallel()
		result := normalizeWorkingDir("")
		assert.Equal(t, "", result)
	})
}

func newBashToolForTest(workingDir string) fantasy.AgentTool {
	permissions := &mockBashPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}
	attribution := &config.Attribution{TrailerStyle: config.TrailerStyleNone}
	return NewBashTool(permissions, workingDir, attribution, "test-model")
}

func runBashTool(t *testing.T, tool fantasy.AgentTool, ctx context.Context, params BashParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	call := fantasy.ToolCall{
		ID:    "test-call",
		Name:  BashToolName,
		Input: string(input),
	}

	resp, err := tool.Run(ctx, call)
	require.NoError(t, err)
	return resp
}
