package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPromptsListView_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	// Create a temp project root with fixture .mdx prompts.
	projectRoot := t.TempDir()
	promptsDir := filepath.Join(projectRoot, ".smithers", "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0o755))

	// Fixture 1: test-review.mdx — two props: lang, focus
	require.NoError(t, os.WriteFile(
		filepath.Join(promptsDir, "test-review.mdx"),
		[]byte("# Review\n\nReview {props.lang} code for {props.focus}.\n"),
		0o644,
	))

	// Fixture 2: test-deploy.mdx — three props: service, env, schema
	require.NoError(t, os.WriteFile(
		filepath.Join(promptsDir, "test-deploy.mdx"),
		[]byte("# Deploy\n\nDeploy {props.service} to {props.env}.\n\nREQUIRED OUTPUT:\n{props.schema}\n"),
		0o644,
	))

	// Create a minimal global config.
	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	// Launch TUI with the temp project root as CWD so that
	// listPromptsFromFS() finds the fixture .mdx files.
	tui := launchTUI(t, "--cwd", projectRoot)
	defer tui.Terminate()

	// 1. Wait for the TUI to fully start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// 2. Open the command palette and filter to "Prompt Templates".
	tui.SendKeys("/")
	require.NoError(t, tui.WaitForText("Commands", 5*time.Second))
	tui.SendKeys("Prompt")
	time.Sleep(300 * time.Millisecond)
	tui.SendKeys("\r")

	// 3. Verify the prompts view header appears.
	require.NoError(t, tui.WaitForText("Prompts", 5*time.Second))

	// 4. Verify that at least one fixture prompt ID appears in the list.
	require.NoError(t, tui.WaitForText("test-review", 5*time.Second))

	// 5. Navigate down to the second prompt.
	tui.SendKeys("j")
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, tui.WaitForText("test-deploy", 3*time.Second))

	// 6. Navigate back up to the first prompt.
	tui.SendKeys("k")
	time.Sleep(300 * time.Millisecond)

	// 7. The source pane should show the "Source" section header once loaded.
	require.NoError(t, tui.WaitForText("Source", 3*time.Second))

	// 8. Verify a prop from test-review appears in the Inputs section.
	require.NoError(t, tui.WaitForText("lang", 3*time.Second))

	// 9. Press Escape to return to the previous view.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("Prompts", 3*time.Second))
}
