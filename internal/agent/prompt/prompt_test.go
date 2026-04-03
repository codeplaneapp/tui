package prompt

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPromptData_WithSmithersMode(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	store, err := config.Init(workingDir, "", false)
	require.NoError(t, err)

	tmpl := "{{if .SmithersMode}}smithers{{else}}default{{end}}|{{.SmithersWorkflowDir}}|{{.SmithersMCPServer}}"
	p, err := NewPrompt(
		"test-smithers",
		tmpl,
		WithWorkingDir(workingDir),
		WithTimeFunc(func() time.Time { return time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC) }),
		WithSmithersMode(filepath.ToSlash(filepath.Join(".smithers", "workflows")), "smithers"),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	require.Equal(t, "smithers|.smithers/workflows|smithers", rendered)
}

func TestPromptData_WithoutSmithersMode(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	store, err := config.Init(workingDir, "", false)
	require.NoError(t, err)

	p, err := NewPrompt(
		"test-default",
		"{{if .SmithersMode}}smithers{{else}}default{{end}}|{{.SmithersWorkflowDir}}|{{.SmithersMCPServer}}",
		WithWorkingDir(workingDir),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	require.Equal(t, "default||", rendered)
}
