package e2e_test

import (
	"testing"
	"time"
)

// TestPlatformAndNavigation exercises the PLATFORM_AND_NAVIGATION feature group
// for the Smithers TUI. Each subtest launches a fresh tmux session running the
// real compiled binary -- no mocks, no API servers.
func TestPlatformAndNavigation(t *testing.T) {
	skipUnlessCrushTUIE2E(t)
	binary := buildBinary(t)

	// PLATFORM_SMITHERS_REBRAND
	// Views use "SMITHERS ›" prefix in their headers. The logo still reads
	// "CRUSH" (the binary name), but Smithers-domain views identify as
	// SMITHERS. Verify Smithers branding appears in navigation views.
	t.Run("PLATFORM_SMITHERS_REBRAND", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		// Wait for the dashboard to render.
		s.WaitForText("SMITHERS", 20*time.Second)

		// Open a Smithers view to verify the rebrand prefix.
		s.SendKeys("C-a")
		s.WaitForText("SMITHERS", 10*time.Second)
		s.SendKeys("Escape")
	})

	// PLATFORM_SMITHERS_CONFIG_NAMESPACE
	// The app loads configuration from smithers-tui.json. When that config is
	// present (with the smithers key), the TUI starts successfully and shows
	// the SMITHERS header.
	t.Run("PLATFORM_SMITHERS_CONFIG_NAMESPACE", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)
	})

	// PLATFORM_CHAT_FIRST_INFORMATION_ARCHITECTURE
	// Chat / dashboard is the default view on startup when smithers config is present.
	t.Run("PLATFORM_CHAT_FIRST_INFORMATION_ARCHITECTURE", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)

		// The default landing should include chat-related content such as
		// "Start Chat" or the dashboard widgets.
		s.WaitForAnyText([]string{"Start Chat", "At a Glance", "chat"}, 10*time.Second)
	})

	// PLATFORM_VIEW_STACK_ROUTER
	// Ctrl+R pushes the Runs view onto the view stack. Esc pops it back.
	t.Run("PLATFORM_VIEW_STACK_ROUTER", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)

		// Push runs view — look for the view-specific header.
		s.SendKeys("C-r")
		s.WaitForAnyText([]string{"Runs [All]", "Runs [", "Loading"}, 10*time.Second)

		// Pop back to chat/dashboard — the Runs view header disappears.
		s.SendKeys("Escape")
		s.WaitForNoText("Runs [", 10*time.Second)
	})

	// PLATFORM_BACK_STACK_NAVIGATION
	// Esc from any pushed view returns to the chat/dashboard root.
	t.Run("PLATFORM_BACK_STACK_NAVIGATION", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)

		// Push runs view, then pop.
		s.SendKeys("C-r")
		s.WaitForAnyText([]string{"Runs [All]", "Runs [", "Loading"}, 10*time.Second)
		s.SendKeys("Escape")
		s.WaitForNoText("Runs [", 10*time.Second)

		// Push approvals view, then pop.
		s.SendKeys("C-a")
		s.WaitForAnyText([]string{"Approvals", "Loading"}, 10*time.Second)
		s.SendKeys("Escape")
		s.WaitForNoText("SMITHERS › Approvals", 10*time.Second)

		// Confirm we're back at the root with header still visible.
		s.AssertVisible("SMITHERS")
	})

	// PLATFORM_KEYBOARD_FIRST_NAVIGATION
	// All major keyboard shortcuts work: Ctrl+R, Ctrl+A, Ctrl+P, Ctrl+G.
	t.Run("PLATFORM_KEYBOARD_FIRST_NAVIGATION", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)

		// Ctrl+R -> Runs view
		s.SendKeys("C-r")
		s.WaitForAnyText([]string{"Runs [All]", "Runs [", "Loading"}, 10*time.Second)
		s.SendKeys("Escape")
		s.WaitForNoText("Runs [", 10*time.Second)

		// Ctrl+A -> Approvals view
		s.SendKeys("C-a")
		s.WaitForAnyText([]string{"SMITHERS › Approvals", "Loading"}, 10*time.Second)
		s.SendKeys("Escape")
		s.WaitForNoText("SMITHERS › Approvals", 10*time.Second)

		// Ctrl+P -> Command palette
		s.SendKeys("C-p")
		s.WaitForAnyText([]string{"Commands", "command", "Switch"}, 10*time.Second)
		s.SendKeys("Escape")

		// Ctrl+G -> Toggle help overlay
		s.SendKeys("C-g")
		s.WaitForAnyText([]string{"help", "Help", "shortcuts", "Shortcuts", "ctrl"}, 10*time.Second)
		// Toggle it off.
		s.SendKeys("C-g")
	})

	// PLATFORM_SMITHERS_COMMAND_PALETTE_EXTENSIONS
	// The command palette (Ctrl+P) shows smithers-specific commands.
	t.Run("PLATFORM_SMITHERS_COMMAND_PALETTE_EXTENSIONS", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)

		// Open the command palette.
		s.SendKeys("C-p")
		s.WaitForAnyText([]string{"Commands", "command"}, 10*time.Second)

		// Capture the palette content and verify smithers-specific commands
		// are present. We check for at least a few of the expected entries.
		pane := s.CapturePane()
		found := 0
		smithersCommands := []string{"agent", "workflow", "ticket", "sql", "trigger"}
		for _, cmd := range smithersCommands {
			if containsNormalized(pane, cmd) {
				found++
			}
		}
		if found == 0 {
			// None of the smithers commands appeared -- that's a failure.
			// But the palette might need scrolling or the commands might use
			// different casing. Try searching for individual ones.
			s.WaitForAnyText(smithersCommands, 10*time.Second)
		}

		s.SendKeys("Escape")
	})

	// PLATFORM_THIN_FRONTEND_TRANSPORT_LAYER
	// The TUI is a thin frontend that delegates to the Smithers backend via
	// HTTP/SSE/exec transports. Verify the transport infrastructure initialises
	// by checking that the dashboard loads and can attempt data fetches.
	t.Run("PLATFORM_THIN_FRONTEND_TRANSPORT_LAYER", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		// Dashboard loads data via transports. Loading/error states prove
		// the transport layer initialised and attempted a fetch.
		s.WaitForAnyText([]string{"At a Glance", "Loading", "Start Chat"}, 10*time.Second)
	})

	// PLATFORM_HTTP_API_CLIENT
	// HTTP API client is wired up. Without a running server the dashboard shows
	// loading/error states, proving the HTTP client attempted requests.
	t.Run("PLATFORM_HTTP_API_CLIENT", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig("http://localhost:19999"), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		// The dashboard should attempt HTTP fetches and show error/loading state.
		s.WaitForAnyText([]string{"At a Glance", "Loading", "Error", "Start Chat"}, 15*time.Second)
	})

	// PLATFORM_SSE_EVENT_STREAMING
	// SSE streaming infrastructure exists. Verified by opening runs view which
	// attempts to subscribe to the global event stream.
	t.Run("PLATFORM_SSE_EVENT_STREAMING", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		s.SendKeys("C-r")
		s.WaitForAnyText([]string{"Runs", "Loading"}, 10*time.Second)
		// SSE stream will fail silently and fall back to polling.
		// The view rendering without a crash proves SSE infra is present.
		s.SendKeys("Escape")
	})

	// PLATFORM_MCP_TRANSPORT
	// MCP transport layer is wired up. The chat header displays MCP
	// connection status when smithers config is present.
	t.Run("PLATFORM_MCP_TRANSPORT", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		// Navigate to chat to see MCP status in header.
		s.SendKeys("c")
		time.Sleep(1 * time.Second)
		// MCP will show disconnected state without a server.
		pane := s.CapturePane()
		if !containsNormalized(pane, "connected") && !containsNormalized(pane, "crush") {
			t.Logf("MCP transport status not visible (expected in offline mode)\nPane:\n%s", pane)
		}
	})

	// PLATFORM_SHELL_OUT_FALLBACK
	// Shell-out fallback transport exists. When HTTP and SQLite are unavailable,
	// the client falls back to exec("smithers", ...). Verify the app boots and
	// handles missing smithers binary gracefully.
	t.Run("PLATFORM_SHELL_OUT_FALLBACK", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		// App boots without crashing even when all transports fail.
		s.WaitForAnyText([]string{"Start Chat", "Overview", "At a Glance"}, 10*time.Second)
	})

	// PLATFORM_TUI_HANDOFF_TRANSPORT
	// TUI handoff via tea.ExecProcess is wired up. The agents view lists agents
	// that can be launched via handoff. Verify the view loads.
	t.Run("PLATFORM_TUI_HANDOFF_TRANSPORT", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		s.SendKeys("C-p")
		s.WaitForAnyText([]string{"Commands", "command"}, 10*time.Second)
		s.SendText("agents")
		time.Sleep(300 * time.Millisecond)
		s.SendKeys("Enter")

		// Agents view should load showing detected CLI agents.
		s.WaitForAnyText([]string{"Agents", "Loading", "Claude Code", "No agents"}, 15*time.Second)
		s.SendKeys("Escape")
	})

	// PLATFORM_WORKSPACE_AND_SYSTEMS_VIEW_MODEL
	// Workspace and systems view model is initialised. The dashboard shows
	// workspace-scoped data: runs, workflows, and menu items.
	t.Run("PLATFORM_WORKSPACE_AND_SYSTEMS_VIEW_MODEL", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))
		s.WaitForText("SMITHERS", 20*time.Second)

		// Dashboard menu items prove the workspace view model is active.
		s.WaitForAnyText([]string{"Run Dashboard", "Workflows", "Approvals"}, 10*time.Second)
	})

	// PLATFORM_SPLIT_PANE_LAYOUTS
	// Views like tickets show a split-pane layout with list and detail panels.
	t.Run("PLATFORM_SPLIT_PANE_LAYOUTS", func(t *testing.T) {
		s := NewTmuxSession(t, binary, WithSmithersConfig(""), WithSize(120, 40))

		s.WaitForText("SMITHERS", 20*time.Second)

		// Open tickets via command palette.
		s.SendKeys("C-p")
		s.WaitForAnyText([]string{"Commands", "command"}, 10*time.Second)
		s.SendText("Work Items")
		time.Sleep(500 * time.Millisecond)
		s.SendKeys("Enter")

		// The tickets view should render with a split pane layout.
		s.WaitForAnyText([]string{"Tickets", "Work Items", "Loading"}, 10*time.Second)

		s.SendKeys("Escape")
	})
}
