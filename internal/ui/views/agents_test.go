package views

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func testAgent(id, name, status string, usable bool) smithers.Agent {
	return smithers.Agent{
		ID:         id,
		Name:       name,
		Command:    id,
		Status:     status,
		Usable:     usable,
		BinaryPath: "/usr/local/bin/" + id,
		Roles:      []string{"coding"},
	}
}

func newTestAgentsView() *AgentsView {
	c := smithers.NewClient()
	v := NewAgentsView(c)
	return v
}

// seedAgents sends an agentsLoadedMsg to populate the view with test agents.
func seedAgents(v *AgentsView, agents []smithers.Agent) *AgentsView {
	updated, _ := v.Update(agentsLoadedMsg{agents: agents})
	return updated.(*AgentsView)
}

// --- Interface compliance ---

func TestAgentsView_ImplementsView(t *testing.T) {
	var _ View = (*AgentsView)(nil)
}

// --- Constructor ---

func TestAgentsView_Init_SetsLoading(t *testing.T) {
	v := newTestAgentsView()
	assert.True(t, v.loading, "should start in loading state")
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil command")
}

// --- Update: loaded/error messages ---

func TestAgentsView_LoadedMsg_PopulatesAgents(t *testing.T) {
	v := newTestAgentsView()
	agents := []smithers.Agent{
		testAgent("claude-code", "Claude Code", "likely-subscription", true),
		testAgent("codex", "Codex", "unavailable", false),
	}
	updated, cmd := v.Update(agentsLoadedMsg{agents: agents})
	assert.Nil(t, cmd)

	av := updated.(*AgentsView)
	assert.False(t, av.loading)
	assert.Len(t, av.agents, 2)
}

func TestAgentsView_ErrorMsg_SetsErr(t *testing.T) {
	v := newTestAgentsView()
	someErr := errors.New("network error")
	updated, cmd := v.Update(agentsErrorMsg{err: someErr})
	assert.Nil(t, cmd)

	av := updated.(*AgentsView)
	assert.False(t, av.loading)
	assert.Equal(t, someErr, av.err)
}

// --- Update: keyboard navigation ---

func TestAgentsView_CursorNavigation_Down(t *testing.T) {
	v := newTestAgentsView()
	agents := []smithers.Agent{
		testAgent("a", "Agent A", "likely-subscription", true),
		testAgent("b", "Agent B", "api-key", true),
		testAgent("c", "Agent C", "unavailable", false),
	}
	v = seedAgents(v, agents)
	assert.Equal(t, 0, v.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	av := updated.(*AgentsView)
	assert.Equal(t, 1, av.cursor, "j should move cursor down")

	updated2, _ := av.Update(tea.KeyPressMsg{Code: 'j'})
	av2 := updated2.(*AgentsView)
	assert.Equal(t, 2, av2.cursor)

	// At the end — should not go past last agent.
	updated3, _ := av2.Update(tea.KeyPressMsg{Code: 'j'})
	av3 := updated3.(*AgentsView)
	assert.Equal(t, 2, av3.cursor, "cursor should not exceed agent count")
}

func TestAgentsView_CursorNavigation_Up(t *testing.T) {
	v := newTestAgentsView()
	agents := []smithers.Agent{
		testAgent("a", "Agent A", "likely-subscription", true),
		testAgent("b", "Agent B", "api-key", true),
	}
	v = seedAgents(v, agents)
	v.cursor = 1

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	av := updated.(*AgentsView)
	assert.Equal(t, 0, av.cursor, "k should move cursor up")

	// At the top — should not go negative.
	updated2, _ := av.Update(tea.KeyPressMsg{Code: 'k'})
	av2 := updated2.(*AgentsView)
	assert.Equal(t, 0, av2.cursor, "cursor should not go below zero")
}

func TestAgentsView_ArrowKeys_Navigate(t *testing.T) {
	v := newTestAgentsView()
	agents := []smithers.Agent{
		testAgent("a", "Agent A", "likely-subscription", true),
		testAgent("b", "Agent B", "api-key", true),
	}
	v = seedAgents(v, agents)

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	av := updated.(*AgentsView)
	assert.Equal(t, 1, av.cursor)

	updated2, _ := av.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	av2 := updated2.(*AgentsView)
	assert.Equal(t, 0, av2.cursor)
}

// --- Update: Esc key ---

func TestAgentsView_Esc_ReturnsPopViewMsg(t *testing.T) {
	v := newTestAgentsView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "Esc should return a non-nil command")

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

// --- Update: r (refresh) ---

func TestAgentsView_Refresh_ReloadsAgents(t *testing.T) {
	v := newTestAgentsView()
	v = seedAgents(v, []smithers.Agent{testAgent("a", "A", "unavailable", false)})
	assert.False(t, v.loading)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	av := updated.(*AgentsView)
	assert.True(t, av.loading, "'r' should set loading = true")
	assert.NotNil(t, cmd, "'r' should return a reload command")
}

// --- Update: Enter key ---

func TestAgentsView_Enter_UsableAgent_SetsLaunching(t *testing.T) {
	v := newTestAgentsView()
	agents := []smithers.Agent{
		testAgent("claude-code", "Claude Code", "likely-subscription", true),
	}
	v = seedAgents(v, agents)
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	av := updated.(*AgentsView)
	assert.True(t, av.launching, "Enter on usable agent should set launching = true")
	assert.Equal(t, "Claude Code", av.launchingName)
}

func TestAgentsView_Enter_UnavailableAgent_NoHandoff(t *testing.T) {
	v := newTestAgentsView()
	// All unavailable agents — cursor is in the "not detected" group.
	agents := []smithers.Agent{
		testAgent("kimi", "Kimi", "unavailable", false),
	}
	v = seedAgents(v, agents)
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	av := updated.(*AgentsView)
	assert.False(t, av.launching, "Enter on unavailable agent should not set launching")
	assert.Nil(t, cmd)
}

// --- Update: HandoffMsg return ---

func TestAgentsView_HandoffMsg_RefreshesAgents(t *testing.T) {
	v := newTestAgentsView()
	v = seedAgents(v, []smithers.Agent{testAgent("a", "A", "likely-subscription", true)})
	v.launching = true
	v.launchingName = "A"

	updated, cmd := v.Update(handoff.HandoffMsg{
		Tag:    "a",
		Result: handoff.HandoffResult{ExitCode: 0},
	})
	av := updated.(*AgentsView)
	assert.False(t, av.launching)
	assert.Empty(t, av.launchingName)
	assert.True(t, av.loading, "should refresh agents after handoff")
	assert.NotNil(t, cmd)
}

func TestAgentsView_HandoffMsg_WithError_SetsErr(t *testing.T) {
	v := newTestAgentsView()
	v = seedAgents(v, []smithers.Agent{testAgent("a", "A", "likely-subscription", true)})

	launchErr := errors.New("binary not found")
	updated, _ := v.Update(handoff.HandoffMsg{
		Tag:    "a",
		Result: handoff.HandoffResult{ExitCode: 1, Err: launchErr},
	})
	av := updated.(*AgentsView)
	assert.NotNil(t, av.err)
}

// --- Update: window resize ---

func TestAgentsView_WindowResize_UpdatesDimensions(t *testing.T) {
	v := newTestAgentsView()
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Nil(t, cmd)

	av := updated.(*AgentsView)
	assert.Equal(t, 120, av.width)
	assert.Equal(t, 40, av.height)
}

// --- View() rendering ---

func TestAgentsView_View_HeaderText(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 24
	out := v.View()
	assert.Contains(t, out, "SMITHERS › Agents")
}

func TestAgentsView_View_LoadingState(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	out := v.View()
	assert.Contains(t, out, "Loading agents...")
}

func TestAgentsView_View_LaunchingState(t *testing.T) {
	v := newTestAgentsView()
	v.loading = false
	v.launching = true
	v.launchingName = "Claude Code"
	out := v.View()
	assert.Contains(t, out, "Launching Claude Code...")
	assert.Contains(t, out, "Smithers TUI will resume")
}

func TestAgentsView_View_ErrorState(t *testing.T) {
	v := newTestAgentsView()
	v.loading = false
	v.err = errors.New("detection failed")
	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "detection failed")
}

func TestAgentsView_View_EmptyState(t *testing.T) {
	v := newTestAgentsView()
	v = seedAgents(v, []smithers.Agent{})
	out := v.View()
	assert.Contains(t, out, "No agents found")
}

func TestAgentsView_View_ShowsGroups(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		testAgent("claude", "Claude Code", "likely-subscription", true),
		testAgent("kimi", "Kimi", "unavailable", false),
	}
	v = seedAgents(v, agents)
	out := v.View()
	assert.Contains(t, out, "Available")
	assert.Contains(t, out, "Not Detected")
}

func TestAgentsView_View_StatusIcons(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		testAgent("claude", "Claude Code", "likely-subscription", true),
		testAgent("kimi", "Kimi", "unavailable", false),
	}
	v = seedAgents(v, agents)
	out := v.View()
	// Filled dot for subscription/available.
	assert.Contains(t, out, "●")
	// Empty circle for unavailable.
	assert.Contains(t, out, "○")
}

func TestAgentsView_View_CursorIndicator(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		testAgent("claude", "Claude Code", "likely-subscription", true),
	}
	v = seedAgents(v, agents)
	v.cursor = 0
	out := v.View()
	assert.Contains(t, out, "▸")
}

func TestAgentsView_View_WideTerminal_TwoColumns(t *testing.T) {
	v := newTestAgentsView()
	v.width = 120
	v.height = 40
	agents := []smithers.Agent{
		testAgent("claude", "Claude Code", "likely-subscription", true),
		testAgent("kimi", "Kimi", "unavailable", false),
	}
	v = seedAgents(v, agents)
	out := v.View()
	// Wide layout should contain a column separator.
	assert.Contains(t, out, "│")
}

func TestAgentsView_View_NarrowTerminal_SingleColumn(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		testAgent("claude", "Claude Code", "likely-subscription", true),
	}
	v = seedAgents(v, agents)
	out := v.View()
	// Narrow layout should still show agent name.
	assert.Contains(t, out, "Claude Code")
}

// --- feat-agents-binary-path-display ---

func TestAgentsView_View_BinaryPath_Shown(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "claude", Name: "Claude Code", Command: "claude",
			BinaryPath: "/usr/local/bin/claude",
			Status:     "likely-subscription", Usable: true,
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	assert.Contains(t, out, "Binary: /usr/local/bin/claude")
}

func TestAgentsView_View_BinaryPath_NotFound_Dash(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "kimi", Name: "Kimi", Command: "kimi",
			BinaryPath: "", Status: "unavailable", Usable: false,
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	// Unavailable agents are shown in the "Not Detected" group; they are not
	// marked detailed==true in the narrow row renderer, so no Binary line is
	// emitted.  This test verifies the absence of "(not found)".
	assert.NotContains(t, out, "(not found)")
}

func TestAgentsView_View_Wide_BinaryPath_Dash_WhenMissing(t *testing.T) {
	v := newTestAgentsView()
	v.width = 120
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "kimi", Name: "Kimi", Command: "kimi",
			BinaryPath: "", Status: "unavailable", Usable: false,
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	// Wide layout detail pane should use "—" (em dash) for a missing binary
	// and must not fall back to the old "(not found)" text.
	// The dash is wrapped in lipgloss Faint ANSI codes so we check the
	// label and the dash character independently.
	assert.Contains(t, out, "Binary:")
	assert.Contains(t, out, "—")
	assert.NotContains(t, out, "(not found)")
}

// --- feat-agents-auth-status-classification ---

func TestAgentsView_View_AuthStatusLabels(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "claude", Name: "Claude Code", Command: "claude",
			BinaryPath: "/usr/bin/claude",
			Status:     "likely-subscription", HasAuth: true, HasAPIKey: false,
			Usable: true,
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	assert.Contains(t, out, "Auth:")
	assert.Contains(t, out, "API Key:")
}

func TestAgentsView_View_Wide_AuthStatusLabels(t *testing.T) {
	v := newTestAgentsView()
	v.width = 120
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "claude", Name: "Claude Code", Command: "claude",
			BinaryPath: "/usr/bin/claude",
			Status:     "likely-subscription", HasAuth: true, HasAPIKey: true,
			Usable: true,
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	assert.Contains(t, out, "Auth:")
	assert.Contains(t, out, "API Key:")
}

// --- feat-agents-role-display ---

func TestAgentsView_View_RolesCapitalized(t *testing.T) {
	v := newTestAgentsView()
	v.width = 80
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "claude", Name: "Claude Code", Command: "claude",
			BinaryPath: "/usr/bin/claude",
			Status:     "likely-subscription", HasAuth: true,
			Usable: true, Roles: []string{"coding", "review"},
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	assert.Contains(t, out, "Roles: Coding, Review")
}

func TestAgentsView_View_Wide_RolesCapitalized(t *testing.T) {
	v := newTestAgentsView()
	v.width = 120
	v.height = 40
	agents := []smithers.Agent{
		{
			ID: "claude", Name: "Claude Code", Command: "claude",
			BinaryPath: "/usr/bin/claude",
			Status:     "likely-subscription", HasAuth: true,
			Usable: true, Roles: []string{"coding", "research"},
		},
	}
	v = seedAgents(v, agents)
	out := v.View()
	assert.Contains(t, out, "Coding, Research")
}

// --- capitalizeRoles helper ---

func TestCapitalizeRoles(t *testing.T) {
	assert.Equal(t, []string{"Coding", "Review", "Research"}, capitalizeRoles([]string{"coding", "review", "research"}))
	assert.Equal(t, []string{}, capitalizeRoles([]string{}))
	assert.Equal(t, []string{""}, capitalizeRoles([]string{""}))
	assert.Equal(t, []string{"X"}, capitalizeRoles([]string{"x"}))
}

// --- agentStatusIcon helper ---

func TestAgentStatusIcon(t *testing.T) {
	assert.Equal(t, "●", agentStatusIcon("likely-subscription"))
	assert.Equal(t, "●", agentStatusIcon("api-key"))
	assert.Equal(t, "◐", agentStatusIcon("binary-only"))
	assert.Equal(t, "○", agentStatusIcon("unavailable"))
	assert.Equal(t, "○", agentStatusIcon(""))
}

// --- groupAgents helper ---

func TestGroupAgents(t *testing.T) {
	agents := []smithers.Agent{
		{ID: "a", Usable: true},
		{ID: "b", Usable: false},
		{ID: "c", Usable: true},
		{ID: "d", Usable: false},
	}
	available, unavailable := groupAgents(agents)
	require.Len(t, available, 2)
	require.Len(t, unavailable, 2)
	assert.Equal(t, "a", available[0].ID)
	assert.Equal(t, "c", available[1].ID)
	assert.Equal(t, "b", unavailable[0].ID)
	assert.Equal(t, "d", unavailable[1].ID)
}

// --- Name / SetSize / ShortHelp ---

func TestAgentsView_Name(t *testing.T) {
	v := newTestAgentsView()
	assert.Equal(t, "agents", v.Name())
}

func TestAgentsView_SetSize(t *testing.T) {
	v := newTestAgentsView()
	v.SetSize(100, 50)
	assert.Equal(t, 100, v.width)
	assert.Equal(t, 50, v.height)
}

func TestAgentsView_ShortHelp_NotEmpty(t *testing.T) {
	v := newTestAgentsView()
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	var allDesc []string
	for _, b := range help {
		allDesc = append(allDesc, b.Help().Desc)
	}
	joined := strings.Join(allDesc, " ")
	assert.Contains(t, joined, "launch")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")
}
