package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func renderSmithersPrompt(t *testing.T, opts ...prompt.Option) string {
	t.Helper()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	store.Config().Options.ContextPaths = nil
	store.Config().Options.SkillsPaths = nil

	const renderedWorkingDir = "/tmp/smithers-workspace"

	baseOptions := []prompt.Option{
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("darwin"),
		prompt.WithWorkingDir(renderedWorkingDir),
	}
	baseOptions = append(baseOptions, opts...)

	systemPrompt, err := smithersPrompt(baseOptions...)
	require.NoError(t, err)

	rendered, err := systemPrompt.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	return rendered
}

func TestSmithersPromptIncludesDomainInstructions(t *testing.T) {
	t.Parallel()

	rendered := renderSmithersPrompt(t, prompt.WithSmithersMode(".smithers/workflows", "smithers"))

	require.Contains(t, rendered, "Smithers TUI assistant")
	require.Contains(t, rendered, "format results as tables")
	require.Contains(t, rendered, "pending approval gates")
	require.Contains(t, rendered, "mcp_smithers_runs_list")
	require.Contains(t, rendered, "Workflow directory: .smithers/workflows")
	require.NotContains(t, rendered, "READ BEFORE EDITING")
	require.NotContains(t, rendered, "<editing_files>")
}

func TestSmithersPromptOmitsWorkspaceWithoutWorkflowDir(t *testing.T) {
	t.Parallel()

	rendered := renderSmithersPrompt(t)

	require.NotContains(t, rendered, "<workspace>")
	require.NotContains(t, rendered, "Workflow directory:")
}

func TestSmithersPromptSnapshot(t *testing.T) {
	t.Parallel()

	rendered := renderSmithersPrompt(t, prompt.WithSmithersMode(".smithers/workflows", "smithers"))
	goldenPath := filepath.Join("testdata", "smithers_prompt.golden")
	if os.Getenv("CRUSH_UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(rendered), 0o644))
	}

	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(golden), rendered)
}
