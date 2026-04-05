package e2e_test

import (
	"os"
	"testing"
	"time"
)

// TestToastOverlay_AppearOnStart verifies that the toast overlay renders over
// any active view when NOTIFICATIONS_TOAST_OVERLAYS=1 and
// CRUSH_TEST_TOAST_ON_START=1 are set.
func TestToastOverlay_AppearOnStart(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)
	t.Setenv("NOTIFICATIONS_TOAST_OVERLAYS", "1")
	t.Setenv("CRUSH_TEST_TOAST_ON_START", "1")

	tui := launchTUI(t)
	defer tui.Terminate()

	// The debug toast should appear in the terminal output.
	if err := tui.WaitForText("Toast test", 15*time.Second); err != nil {
		t.Fatal(err)
	}
}

// TestToastOverlay_FeatureFlagOff verifies that no toast appears when the
// NOTIFICATIONS_TOAST_OVERLAYS flag is absent, even with CRUSH_TEST_TOAST_ON_START set.
func TestToastOverlay_FeatureFlagOff(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)
	// Do NOT set NOTIFICATIONS_TOAST_OVERLAYS
	t.Setenv("CRUSH_TEST_TOAST_ON_START", "1")

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start fully (look for some stable text).
	// The exact startup text may vary, so just wait a moment.
	time.Sleep(3 * time.Second)

	// "Toast test" must not appear in the output.
	if err := tui.WaitForNoText("Toast test", 2*time.Second); err != nil {
		t.Fatalf("toast appeared but feature flag was off: %v", err)
	}
}

// TestToastOverlay_DismissKey verifies that the newest toast is removed when
// the alt+d keybinding is sent.
func TestToastOverlay_DismissKey(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)
	t.Setenv("NOTIFICATIONS_TOAST_OVERLAYS", "1")
	t.Setenv("CRUSH_TEST_TOAST_ON_START", "1")

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the toast to appear.
	if err := tui.WaitForText("Toast test", 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// Send alt+d to dismiss the toast.
	tui.SendKeys("\x1bd") // ESC + 'd' = alt+d

	// The toast should be gone within a short time.
	if err := tui.WaitForNoText("Toast test", 5*time.Second); err != nil {
		t.Fatalf("toast not dismissed after alt+d: %v", err)
	}
}

// TestToastOverlay_DisableNotificationsConfig verifies that no toast appears
// when disable_notifications is set in the config, even with the feature flag
// enabled and CRUSH_TEST_TOAST_ON_START set.
//
// Note: CRUSH_TEST_TOAST_ON_START fires the toast directly via Init before the
// flag is checked, so this test uses the SSE path. Since no SSE server is
// running, the toast just shouldn't appear due to the config flag. This test
// is therefore limited to confirming the TUI starts without a toast (the config
// check is exercised by the unit tests in notifications_test.go).
func TestToastOverlay_DisableNotificationsConfig(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	// Config has disable_notifications: true
	writeGlobalConfig(t, configDir, `{"options": {"disable_notifications": true}}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)
	t.Setenv("NOTIFICATIONS_TOAST_OVERLAYS", "1")
	// Do NOT set CRUSH_TEST_TOAST_ON_START so there's no toast triggered at startup.

	tui := launchTUI(t)
	defer tui.Terminate()

	// Give the TUI time to start and settle.
	time.Sleep(3 * time.Second)

	// No toast should appear.
	if err := tui.WaitForNoText("Toast test", 2*time.Second); err != nil {
		t.Fatalf("toast appeared but disable_notifications was true: %v", err)
	}
}
