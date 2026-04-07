package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOnboardingAndGlobalDialogs_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ONBOARDING_MODELS_DIALOG_OPENS_ON_STARTUP", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		require.NoError(t, tui.WaitForText("To start, let's choose a provider and model.", 15*time.Second))
		require.NoError(t, tui.WaitForText("Find your fave", 10*time.Second))
	})

	t.Run("ONBOARDING_FILTERS_MODELS", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		require.NoError(t, tui.WaitForText("Find your fave", 15*time.Second))
		tui.SendText("claude")
		require.NoError(t, tui.WaitForText("Claude", 10*time.Second))
	})

	t.Run("ONBOARDING_SELECTION_OPENS_API_KEY_DIALOG", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		require.NoError(t, tui.WaitForText("Find your fave", 15*time.Second))
		tui.SendText("claude")
		require.NoError(t, tui.WaitForText("Claude", 10*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForText("Enter your Anthropic Key.", 10*time.Second))
		require.NoError(t, tui.WaitForText("Enter your API key...", 10*time.Second))
	})

	t.Run("ONBOARDING_ESCAPE_RETURNS_TO_MODEL_SELECTION", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		require.NoError(t, tui.WaitForText("Find your fave", 15*time.Second))
		tui.SendText("claude")
		require.NoError(t, tui.WaitForText("Claude", 10*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForText("Enter your Anthropic Key.", 10*time.Second))
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("To start, let's choose a provider and model.", 10*time.Second))
		require.NoError(t, tui.WaitForText("Find your fave", 10*time.Second))
	})

	t.Run("SLASH_ON_EMPTY_EDITOR_OPENS_COMMANDS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		tui.SendKeys("/")
		require.NoError(t, tui.WaitForText("Commands", 5*time.Second))
		require.NoError(t, tui.WaitForText("Switch Model", 5*time.Second))
	})

	t.Run("CTRL_P_OPENS_COMMANDS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openCommandsPalette(t, tui)
	})

	t.Run("COMMANDS_FILTER_AND_NAVIGATE_TO_RUNS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openCommandsPalette(t, tui)
		tui.SendText("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForText("SMITHERS › Runs", 10*time.Second))
	})

	t.Run("CTRL_L_OPENS_MODELS_DIALOG", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openModelsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Large Task", 5*time.Second))
	})

	t.Run("MODELS_TAB_SWITCHES_PLACEHOLDER", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openModelsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Choose a model for large, complex tasks", 5*time.Second))
		tui.SendKeys("\t")
		require.NoError(t, tui.WaitForText("Choose a model for small, simple tasks", 5*time.Second))
	})

	t.Run("MODELS_FILTER_SELECTS_NEW_MODEL", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openModelsDialog(t, tui)
		require.NoError(t, tui.WaitForText(fixtureLargeModelName, 5*time.Second))
		require.NoError(t, tui.WaitForText(fixtureSmallModelName, 5*time.Second))
		tui.SendText("mini")
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Switch Model", 10*time.Second))
		require.NoError(t, tui.WaitForText("large model changed to reason-mini", 10*time.Second))
	})

	t.Run("CTRL_G_EXPANDS_HELPBAR", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		tui.SendKeys("\x07") // ctrl+g
		require.NoError(t, tui.WaitForText("ctrl+c", 5*time.Second))
	})

	t.Run("QUIT_DIALOG_CANCELS_WITH_ESCAPE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openQuitDialog(t, tui)
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Are you sure you want to quit?", 5*time.Second))
		require.NoError(t, tui.WaitForText("Start Chat", 5*time.Second))
	})

	t.Run("QUIT_DIALOG_CONFIRMS_EXIT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openQuitDialog(t, tui)
		tui.SendKeys("y")
		require.NoError(t, tui.WaitForText("[crush exited: 0]", 10*time.Second))
	})
}

func TestSessionsPersistence_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSIONS_DIALOG_LISTS_REAL_SESSIONS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Alpha Session"},
			seededSession{title: "Beta Session"},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Alpha Session", 5*time.Second))
		require.NoError(t, tui.WaitForText("Beta Session", 5*time.Second))
	})

	t.Run("SESSIONS_RENAME_PERSISTS_AFTER_REOPEN", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Keep Session"},
			seededSession{title: "Rename Session"},
		)
		tui := launchFixtureTUI(t, fixture)

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Rename Session", 5*time.Second))
		tui.SendKeys("\x12") // ctrl+r
		require.NoError(t, tui.WaitForText("Rename this session?", 5*time.Second))
		tui.SendText("Retitled Session")
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Rename this session?", 5*time.Second))
		waitForSessionTitleState(t, fixture.workspaceDataDir(), "Retitled Session", true, 5*time.Second)
		tui.Terminate()

		tui = launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Retitled Session", 5*time.Second))
	})

	t.Run("SESSIONS_DELETE_PERSISTS_AFTER_REOPEN", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Survivor Session"},
			seededSession{title: "Delete Session"},
		)
		tui := launchFixtureTUI(t, fixture)

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Delete Session", 5*time.Second))
		tui.SendKeys("\x18") // ctrl+x
		require.NoError(t, tui.WaitForText("Delete this session?", 5*time.Second))
		tui.SendKeys("y")
		require.NoError(t, tui.WaitForNoText("Delete this session?", 5*time.Second))
		waitForSessionTitleState(t, fixture.workspaceDataDir(), "Delete Session", false, 5*time.Second)
		tui.Terminate()

		tui = launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Survivor Session", 5*time.Second))
		require.NoError(t, tui.WaitForNoText("Delete Session", 5*time.Second))
	})

	t.Run("CONTINUE_FLAG_LOADS_LAST_SESSION_MESSAGES", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(), seededSession{
			title:    "Continue Session",
			messages: []string{"seeded continue message"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForConfiguredLanding(t, tui)
		require.NoError(t, tui.WaitForText("seeded continue message", 15*time.Second))
	})
}

func TestAttachmentsAndCompletions_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CTRL_F_OPENS_ADD_IMAGE_DIALOG", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writePNGFixture(t, fixture.workingDir, "pixel.png")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		require.NoError(t, tui.WaitForText("pixel.png", 5*time.Second))
	})

	t.Run("SELECTING_IMAGE_ADDS_ATTACHMENT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writePNGFixture(t, fixture.workingDir, "pixel.png")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		tui.SendKeys("j")
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Add Image", 10*time.Second))
		require.NoError(t, tui.WaitForText("pixel.png", 10*time.Second))
	})

	t.Run("AT_COMPLETION_INSERTS_PATH_AND_ATTACHMENT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writeTextFixture(t, fixture.workingDir, "context.txt", "fixture context")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		tui.SendText("@cont")
		require.NoError(t, tui.WaitForText("context.txt", 10*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForText("context.txt", 10*time.Second))
	})
}
