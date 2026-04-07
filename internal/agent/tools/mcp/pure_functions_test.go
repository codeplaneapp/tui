package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input mcp.LoggingLevel
		want  slog.Level
	}{
		{
			name:  "info maps to LevelInfo",
			input: "info",
			want:  slog.LevelInfo,
		},
		{
			name:  "notice maps to LevelInfo",
			input: "notice",
			want:  slog.LevelInfo,
		},
		{
			name:  "warning maps to LevelWarn",
			input: "warning",
			want:  slog.LevelWarn,
		},
		{
			name:  "debug falls through to default LevelDebug",
			input: "debug",
			want:  slog.LevelDebug,
		},
		{
			name:  "empty string falls through to default LevelDebug",
			input: "",
			want:  slog.LevelDebug,
		},
		{
			name:  "error falls through to default LevelDebug",
			input: "error",
			want:  slog.LevelDebug,
		},
		{
			name:  "critical falls through to default LevelDebug",
			input: "critical",
			want:  slog.LevelDebug,
		},
		{
			name:  "emergency falls through to default LevelDebug",
			input: "emergency",
			want:  slog.LevelDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseLevel(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeBase64Input(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "no whitespace unchanged",
			input: []byte("SGVsbG8gV29ybGQh"),
			want:  []byte("SGVsbG8gV29ybGQh"),
		},
		{
			name:  "trailing newline stripped",
			input: []byte("SGVsbG8=\n"),
			want:  []byte("SGVsbG8="),
		},
		{
			name:  "trailing CRLF stripped",
			input: []byte("SGVsbG8=\r\n"),
			want:  []byte("SGVsbG8="),
		},
		{
			name:  "internal spaces stripped",
			input: []byte("SGVS bG8g V29y bGQh"),
			want:  []byte("SGVSbG8gV29ybGQh"),
		},
		{
			name:  "multiple newlines stripped",
			input: []byte("SGVS\nbG8g\nV29y\nbGQh\n"),
			want:  []byte("SGVSbG8gV29ybGQh"),
		},
		{
			name:  "tabs stripped",
			input: []byte("SGVs\tbG8="),
			want:  []byte("SGVsbG8="),
		},
		{
			name:  "empty input returns empty",
			input: []byte(""),
			want:  []byte(""),
		},
		{
			name:  "only whitespace returns empty",
			input: []byte("   \n\t  "),
			want:  []byte(""),
		},
		{
			name:  "PEM-style line-wrapped base64",
			input: []byte("U0dWc2JH\nOGdWMjl5\nYkdRaA==\n"),
			want:  []byte("U0dWc2JHOGdWMjl5YkdRaA=="),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeBase64Input(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeBase64(t *testing.T) {
	t.Parallel()

	helloB64 := base64.StdEncoding.EncodeToString([]byte("Hello"))

	tests := []struct {
		name    string
		input   []byte
		wantOK  bool
		wantOut []byte // only checked when wantOK is true
	}{
		{
			name:    "valid standard encoding with padding",
			input:   []byte(helloB64),
			wantOK:  true,
			wantOut: []byte("Hello"),
		},
		{
			name:    "valid padded single char YQ==",
			input:   []byte("YQ=="),
			wantOK:  true,
			wantOut: []byte("a"),
		},
		{
			name:    "valid raw encoding without padding",
			input:   []byte("YQ"),
			wantOK:  true,
			wantOut: []byte("a"),
		},
		{
			name:    "empty input returns empty and true",
			input:   []byte{},
			wantOK:  true,
			wantOut: []byte{},
		},
		{
			name:   "binary data with high bytes fails fast",
			input:  []byte{0x89, 0x50, 0x4E, 0x47},
			wantOK: false,
		},
		{
			name:   "single high byte 0xFF fails",
			input:  []byte{0xFF},
			wantOK: false,
		},
		{
			name:   "invalid base64 characters",
			input:  []byte("!!!invalid!!!"),
			wantOK: false,
		},
		{
			name:    "Hello World roundtrip",
			input:   []byte("SGVsbG8gV29ybGQh"),
			wantOK:  true,
			wantOut: []byte("Hello World!"),
		},
		{
			name:   "mixed ASCII and high byte rejects",
			input:  []byte{0x41, 0x42, 0x80},
			wantOK: false,
		},
		{
			name:    "multi-line base64 after normalization",
			input:   normalizeBase64Input([]byte("SGVs\nbG8=")),
			wantOK:  true,
			wantOut: []byte("Hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := decodeBase64(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantOut, got)
			}
		})
	}
}

func TestFilterDisabledTools(t *testing.T) {
	t.Parallel()

	mkTool := func(name string) *Tool {
		return &Tool{Name: name}
	}

	tests := []struct {
		name          string
		mcpName       string
		mcpConfig     map[string]config.MCPConfig
		tools         []*Tool
		wantToolNames []string
	}{
		{
			name:    "nil disabled list returns all tools",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: nil},
			},
			tools:         []*Tool{mkTool("read"), mkTool("write"), mkTool("exec")},
			wantToolNames: []string{"read", "write", "exec"},
		},
		{
			name:    "empty disabled list returns all tools",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{}},
			},
			tools:         []*Tool{mkTool("read"), mkTool("write")},
			wantToolNames: []string{"read", "write"},
		},
		{
			name:    "filters single disabled tool",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{"write"}},
			},
			tools:         []*Tool{mkTool("read"), mkTool("write"), mkTool("exec")},
			wantToolNames: []string{"read", "exec"},
		},
		{
			name:    "filters multiple disabled tools",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{"write", "exec"}},
			},
			tools:         []*Tool{mkTool("read"), mkTool("write"), mkTool("exec")},
			wantToolNames: []string{"read"},
		},
		{
			name:    "all tools disabled returns empty slice",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{"read", "write"}},
			},
			tools:         []*Tool{mkTool("read"), mkTool("write")},
			wantToolNames: []string{},
		},
		{
			name:          "mcp not found in config returns all tools",
			mcpName:       "unknown",
			mcpConfig:     map[string]config.MCPConfig{},
			tools:         []*Tool{mkTool("read"), mkTool("write")},
			wantToolNames: []string{"read", "write"},
		},
		{
			name:    "disabled name not matching any tool is harmless",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{"nonexistent"}},
			},
			tools:         []*Tool{mkTool("read"), mkTool("write")},
			wantToolNames: []string{"read", "write"},
		},
		{
			name:    "nil tools input returns empty when disabled list present",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{"read"}},
			},
			tools:         nil,
			wantToolNames: []string{},
		},
		{
			name:    "nil tools input returns nil when no disabled list",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: nil},
			},
			tools:         nil,
			wantToolNames: nil,
		},
		{
			name:    "empty tools input returns empty",
			mcpName: "srv",
			mcpConfig: map[string]config.MCPConfig{
				"srv": {DisabledTools: []string{"read"}},
			},
			tools:         []*Tool{},
			wantToolNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := config.NewConfigStore(&config.Config{
				MCP: tt.mcpConfig,
			})
			got := filterDisabledTools(store, tt.mcpName, tt.tools)

			if tt.wantToolNames == nil {
				assert.Nil(t, got)
				return
			}

			gotNames := make([]string, len(got))
			for i, tool := range got {
				gotNames[i] = tool.Name
			}
			assert.Equal(t, tt.wantToolNames, gotNames)
		})
	}
}

func TestMaybeTimeoutErr(t *testing.T) {
	t.Parallel()

	t.Run("context.Canceled becomes timeout message", func(t *testing.T) {
		t.Parallel()
		timeout := 30 * time.Second
		got := maybeTimeoutErr(context.Canceled, timeout)
		require.Error(t, got)
		assert.Contains(t, got.Error(), "timed out after 30s")
	})

	t.Run("non-canceled error passes through unchanged", func(t *testing.T) {
		t.Parallel()
		original := fmt.Errorf("connection refused")
		got := maybeTimeoutErr(original, 15*time.Second)
		assert.Equal(t, original, got)
	})

	t.Run("wrapped canceled error also becomes timeout", func(t *testing.T) {
		t.Parallel()
		wrapped := fmt.Errorf("inner: %w", context.Canceled)
		got := maybeTimeoutErr(wrapped, 10*time.Second)
		require.Error(t, got)
		assert.Contains(t, got.Error(), "timed out after 10s")
	})

	t.Run("deadline exceeded passes through unchanged", func(t *testing.T) {
		t.Parallel()
		got := maybeTimeoutErr(context.DeadlineExceeded, 5*time.Second)
		assert.ErrorIs(t, got, context.DeadlineExceeded)
	})
}

func TestMcpTimeout(t *testing.T) {
	t.Parallel()

	t.Run("uses configured timeout", func(t *testing.T) {
		t.Parallel()
		m := config.MCPConfig{Timeout: 30}
		assert.Equal(t, 30*time.Second, mcpTimeout(m))
	})

	t.Run("zero defaults to 15 seconds", func(t *testing.T) {
		t.Parallel()
		m := config.MCPConfig{Timeout: 0}
		assert.Equal(t, 15*time.Second, mcpTimeout(m))
	})

	t.Run("large custom timeout", func(t *testing.T) {
		t.Parallel()
		m := config.MCPConfig{Timeout: 120}
		assert.Equal(t, 120*time.Second, mcpTimeout(m))
	})
}

func TestStateStringUnknown(t *testing.T) {
	t.Parallel()

	// Out-of-range State values should return "unknown".
	assert.Equal(t, "unknown", State(99).String())
	assert.Equal(t, "unknown", State(-1).String())
}

func TestIsMethodNotFoundError(t *testing.T) {
	t.Parallel()

	t.Run("nil error returns false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isMethodNotFoundError(nil))
	})

	t.Run("generic error returns false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isMethodNotFoundError(fmt.Errorf("some error")))
	})

	t.Run("jsonrpc method not found returns true", func(t *testing.T) {
		t.Parallel()
		err := &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
		assert.True(t, isMethodNotFoundError(err))
	})

	t.Run("jsonrpc other code returns false", func(t *testing.T) {
		t.Parallel()
		err := &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "internal error"}
		assert.False(t, isMethodNotFoundError(err))
	})

	t.Run("wrapped jsonrpc method not found returns true", func(t *testing.T) {
		t.Parallel()
		inner := &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
		wrapped := fmt.Errorf("outer: %w", inner)
		assert.True(t, isMethodNotFoundError(wrapped))
	})
}

func TestUpdatePrompts(t *testing.T) {
	t.Parallel()

	t.Run("set prompts stores them", func(t *testing.T) {
		t.Parallel()
		name := "test-prompts-set"
		prompts := []*Prompt{{Name: "greeting"}, {Name: "farewell"}}
		updatePrompts(name, prompts)
		defer allPrompts.Del(name)

		got, ok := allPrompts.Get(name)
		require.True(t, ok)
		assert.Len(t, got, 2)
	})

	t.Run("nil prompts deletes entry", func(t *testing.T) {
		t.Parallel()
		name := "test-prompts-nil"
		// Seed with a value first.
		allPrompts.Set(name, []*Prompt{{Name: "a"}})
		updatePrompts(name, nil)

		_, ok := allPrompts.Get(name)
		assert.False(t, ok, "nil prompts should remove the entry")
	})

	t.Run("empty prompts deletes entry", func(t *testing.T) {
		t.Parallel()
		name := "test-prompts-empty"
		allPrompts.Set(name, []*Prompt{{Name: "a"}})
		updatePrompts(name, []*Prompt{})

		_, ok := allPrompts.Get(name)
		assert.False(t, ok, "empty prompts should remove the entry")
	})
}

func TestUpdateResources(t *testing.T) {
	t.Parallel()

	t.Run("set resources returns count", func(t *testing.T) {
		t.Parallel()
		name := "test-resources-set"
		resources := []*Resource{{URI: "file:///a.txt"}, {URI: "file:///b.txt"}}
		count := updateResources(name, resources)
		defer allResources.Del(name)

		assert.Equal(t, 2, count)
		got, ok := allResources.Get(name)
		require.True(t, ok)
		assert.Len(t, got, 2)
	})

	t.Run("nil resources deletes entry and returns 0", func(t *testing.T) {
		t.Parallel()
		name := "test-resources-nil"
		allResources.Set(name, []*Resource{{URI: "file:///a.txt"}})
		count := updateResources(name, nil)

		assert.Equal(t, 0, count)
		_, ok := allResources.Get(name)
		assert.False(t, ok)
	})

	t.Run("empty resources deletes entry and returns 0", func(t *testing.T) {
		t.Parallel()
		name := "test-resources-empty"
		allResources.Set(name, []*Resource{{URI: "file:///a.txt"}})
		count := updateResources(name, []*Resource{})

		assert.Equal(t, 0, count)
		_, ok := allResources.Get(name)
		assert.False(t, ok)
	})
}
