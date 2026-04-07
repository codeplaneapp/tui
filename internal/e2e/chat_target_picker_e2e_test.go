package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatTargetPicker_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("START_CHAT_OPENS_PICKER_AND_ESCAPE_RETURNS_TO_DASHBOARD", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		obsAddr := reserveObservabilityAddr(t)

		tui := launchFixtureTUIWithOptions(t, fixture, tuiLaunchOptions{
			env: chatTargetObservabilityEnv(obsAddr),
		})
		defer tui.Terminate()

		waitForDashboard(t, tui)
		waitForObservabilityReady(t, obsAddr, 10*time.Second)

		tui.SendKeys("c")
		require.NoError(t, tui.WaitForText("Choose how you want to chat in this workspace.", 10*time.Second))
		require.NoError(t, tui.WaitForText("Smithers", 5*time.Second))

		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Choose how you want to chat in this workspace.", 5*time.Second))
		waitForDashboard(t, tui)

		waitForTraceSpan(t, obsAddr, 10*time.Second, func(span debugTraceSpan) bool {
			return span.Name == "ui.navigation" && spanHasAttrs(span, map[string]string{
				"crush.ui.entrypoint": "dashboard",
				"crush.ui.target":     "chat_picker",
				"crush.ui.result":     "ok",
			})
		})
		waitForMetricAtLeast(t, obsAddr, "crush_ui_navigation_total", map[string]string{
			"entrypoint": "dashboard",
			"target":     "chat_picker",
			"result":     "ok",
		}, 1, 10*time.Second)
	})

	t.Run("SELECTING_SMITHERS_OPENS_EMBEDDED_CHAT_AND_RECORDS_SELECTION", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		obsAddr := reserveObservabilityAddr(t)

		tui := launchFixtureTUIWithOptions(t, fixture, tuiLaunchOptions{
			env: chatTargetObservabilityEnv(obsAddr),
		})
		defer tui.Terminate()

		waitForDashboard(t, tui)
		waitForObservabilityReady(t, obsAddr, 10*time.Second)

		tui.SendKeys("c")
		require.NoError(t, tui.WaitForText("Choose how you want to chat in this workspace.", 10*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"MCPs",
			"Ready for instructions",
			"Ready...",
			fixtureLargeModelName,
		}, 15*time.Second))

		waitForTraceSpan(t, obsAddr, 10*time.Second, func(span debugTraceSpan) bool {
			return span.Name == "ui.navigation" && spanHasAttrs(span, map[string]string{
				"crush.ui.entrypoint": "chat_picker",
				"crush.ui.target":     "chat_target_select",
				"crush.ui.result":     "ok",
				"crush.chat.target":   "smithers",
			})
		})
		waitForMetricAtLeast(t, obsAddr, "crush_ui_navigation_total", map[string]string{
			"entrypoint": "chat_picker",
			"target":     "chat_target_select",
			"result":     "ok",
		}, 1, 10*time.Second)
	})

	t.Run("EXTERNAL_AGENT_HANDOFF_RESUMES_AND_RECORDS_OBSERVABILITY", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		obsAddr := reserveObservabilityAddr(t)
		fakeDir := t.TempDir()
		markerPath := filepath.Join(t.TempDir(), "opencode.log")
		writeFakeChatAgent(t, fakeDir, "opencode", markerPath)

		tui := launchFixtureTUIWithOptions(t, fixture, tuiLaunchOptions{
			env:          chatTargetObservabilityEnv(obsAddr),
			pathPrefixes: []string{fakeDir},
		})
		defer tui.Terminate()

		waitForDashboard(t, tui)
		waitForObservabilityReady(t, obsAddr, 10*time.Second)

		tui.SendKeys("c")
		require.NoError(t, tui.WaitForText("Choose how you want to chat in this workspace.", 10*time.Second))
		require.NoError(t, tui.WaitForText("OpenCode", 10*time.Second))
		selectChatTarget(t, tui, "OpenCode")
		tui.SendKeys("\r")

		require.NoError(t, waitForFileText(markerPath, "launched", 10*time.Second))
		require.NoError(t, tui.WaitForText("Choose how you want to chat in this workspace.", 10*time.Second))

		waitForTraceSpan(t, obsAddr, 10*time.Second, func(span debugTraceSpan) bool {
			return span.Name == "ui.navigation" && spanHasAttrs(span, map[string]string{
				"crush.ui.entrypoint": "chat_picker",
				"crush.ui.target":     "chat_target_handoff",
				"crush.ui.result":     "ok",
				"crush.chat.target":   "opencode",
			})
		})
		waitForMetricAtLeast(t, obsAddr, "crush_ui_navigation_total", map[string]string{
			"entrypoint": "chat_picker",
			"target":     "chat_target_handoff",
			"result":     "ok",
		}, 1, 10*time.Second)
	})
}

func chatTargetObservabilityEnv(addr string) map[string]string {
	return map[string]string{
		"CRUSH_OBSERVABILITY_ADDR":               addr,
		"CRUSH_OBSERVABILITY_TRACE_BUFFER_SIZE":  "128",
		"CRUSH_OBSERVABILITY_TRACE_SAMPLE_RATIO": "1",
	}
}

func selectChatTarget(t *testing.T, tui *TUITestInstance, name string) {
	t.Helper()

	selectedMarker := "▸ " + name
	for range 8 {
		if strings.Contains(tui.bufferText(), selectedMarker) {
			return
		}
		tui.SendKeys("j")
		time.Sleep(150 * time.Millisecond)
	}

	t.Fatalf("chat target %q was not selectable\nBuffer:\n%s", name, tui.Snapshot())
}

func writeFakeChatAgent(t *testing.T, dir, binaryName, markerPath string) string {
	t.Helper()

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "%s 0.1.0"
  exit 0
fi
printf 'launched\n' >> %s
exit 0
`, binaryName, shellQuote(markerPath))

	path := filepath.Join(dir, binaryName)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}
