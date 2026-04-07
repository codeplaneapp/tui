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

func TestSmithersPromptActiveRunsInjected(t *testing.T) {
	t.Parallel()

	activeRuns := []prompt.ActiveRunContext{
		{RunID: "run-abc", WorkflowName: "code-review", WorkflowPath: "workflows/code-review.tsx", Status: "running"},
		{RunID: "run-def", WorkflowName: "deploy", WorkflowPath: "workflows/deploy.tsx", Status: "waiting-approval"},
	}
	rendered := renderSmithersPrompt(t,
		prompt.WithSmithersMode(".smithers/workflows", "smithers"),
		prompt.WithSmithersActiveRuns(activeRuns, 1),
	)

	require.Contains(t, rendered, "Active runs (2 total, 1 pending approval)")
	require.Contains(t, rendered, "run-abc: code-review (running)")
	require.Contains(t, rendered, "run-def: deploy (waiting-approval)")
}

func TestSmithersPromptActiveRunsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	rendered := renderSmithersPrompt(t,
		prompt.WithSmithersMode(".smithers/workflows", "smithers"),
		prompt.WithSmithersActiveRuns(nil, 0),
	)

	require.NotContains(t, rendered, "Active runs")
	require.Contains(t, rendered, "Workflow directory: .smithers/workflows")
}

func TestSmithersPromptPendingApprovalsOnlyWhenNoActiveRuns(t *testing.T) {
	t.Parallel()

	// This tests the edge case where pending approvals exist but the active runs list
	// itself is empty (shouldn't normally happen, but defensive test).
	rendered := renderSmithersPrompt(t,
		prompt.WithSmithersMode(".smithers/workflows", "smithers"),
		prompt.WithSmithersActiveRuns(nil, 3),
	)

	require.Contains(t, rendered, "Pending approvals: 3")
}

func TestSmithersPromptSnapshot(t *testing.T) {
	t.Parallel()

	rendered := renderSmithersPrompt(t, prompt.WithSmithersMode(".smithers/workflows", "smithers"))
	goldenPath := filepath.Join("testdata", "smithers_prompt.golden")
	if os.Getenv("SMITHERS_TUI_UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(rendered), 0o644))
	}

	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(golden), rendered)
}

func TestSmithersPromptSnapshotWithActiveRuns(t *testing.T) {
	t.Parallel()

	activeRuns := []prompt.ActiveRunContext{
		{RunID: "run-001", WorkflowName: "ci-pipeline", WorkflowPath: ".smithers/workflows/ci.tsx", Status: "running"},
		{RunID: "run-002", WorkflowName: "deploy-prod", WorkflowPath: ".smithers/workflows/deploy.tsx", Status: "waiting-approval"},
	}
	rendered := renderSmithersPrompt(t,
		prompt.WithSmithersMode(".smithers/workflows", "smithers"),
		prompt.WithSmithersActiveRuns(activeRuns, 1),
	)

	require.Contains(t, rendered, "Active runs (2 total, 1 pending approval)")
	require.Contains(t, rendered, "run-001: ci-pipeline (running)")
	require.Contains(t, rendered, "run-002: deploy-prod (waiting-approval)")
	require.Contains(t, rendered, "Workflow directory: .smithers/workflows")
}

// --- Coder prompt tests ---

func renderCoderPrompt(t *testing.T, opts ...prompt.Option) string {
	t.Helper()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	store.Config().Options.ContextPaths = nil
	store.Config().Options.SkillsPaths = nil

	baseOptions := []prompt.Option{
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("linux"),
		prompt.WithWorkingDir("/tmp/test-workspace"),
	}
	baseOptions = append(baseOptions, opts...)

	systemPrompt, err := coderPrompt(baseOptions...)
	require.NoError(t, err)

	rendered, err := systemPrompt.Build(context.Background(), "anthropic", "claude-opus-4", store)
	require.NoError(t, err)
	return rendered
}

func TestCoderPromptIncludesCriticalRules(t *testing.T) {
	t.Parallel()

	rendered := renderCoderPrompt(t)

	require.Contains(t, rendered, "READ BEFORE EDITING")
	require.Contains(t, rendered, "BE AUTONOMOUS")
	require.Contains(t, rendered, "TEST AFTER CHANGES")
}

func TestCoderPromptIncludesEnvironmentInfo(t *testing.T) {
	t.Parallel()

	rendered := renderCoderPrompt(t)

	require.Contains(t, rendered, "/tmp/test-workspace")
	require.Contains(t, rendered, "linux")
	require.Contains(t, rendered, "4/6/2026")
}

func TestCoderPromptDoesNotIncludeSmithersInstructions(t *testing.T) {
	t.Parallel()

	rendered := renderCoderPrompt(t)

	require.NotContains(t, rendered, "Smithers TUI assistant")
	require.NotContains(t, rendered, "mcp_smithers_runs_list")
}

// --- Task prompt tests ---

func renderTaskPrompt(t *testing.T, opts ...prompt.Option) string {
	t.Helper()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	store.Config().Options.ContextPaths = nil
	store.Config().Options.SkillsPaths = nil

	baseOptions := []prompt.Option{
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("darwin"),
		prompt.WithWorkingDir("/tmp/task-workspace"),
	}
	baseOptions = append(baseOptions, opts...)

	systemPrompt, err := taskPrompt(baseOptions...)
	require.NoError(t, err)

	rendered, err := systemPrompt.Build(context.Background(), "anthropic", "claude-opus-4", store)
	require.NoError(t, err)
	return rendered
}

func TestTaskPromptRendersSuccessfully(t *testing.T) {
	t.Parallel()

	rendered := renderTaskPrompt(t)
	require.NotEmpty(t, rendered)
	// Task prompt should include environment info
	require.Contains(t, rendered, "/tmp/task-workspace")
}

// --- InitializePrompt tests ---

func TestInitializePromptRendersSuccessfully(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store, err := config.Init(tmpDir, "", false)
	require.NoError(t, err)

	rendered, err := InitializePrompt(store)
	require.NoError(t, err)
	require.NotEmpty(t, rendered)
	require.Contains(t, rendered, store.Config().Options.InitializeAs)
}
