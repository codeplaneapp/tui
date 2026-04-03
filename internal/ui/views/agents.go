package views

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*AgentsView)(nil)

type agentsLoadedMsg struct {
	agents []smithers.Agent
}

type agentsErrorMsg struct {
	err error
}

// AgentsView displays a selectable list of CLI agents.
type AgentsView struct {
	client  *smithers.Client
	agents  []smithers.Agent
	cursor  int
	width   int
	height  int
	loading bool
	err     error
}

// NewAgentsView creates a new agents view.
func NewAgentsView(client *smithers.Client) *AgentsView {
	return &AgentsView{
		client:  client,
		loading: true,
	}
}

// Init loads agents from the client.
func (v *AgentsView) Init() tea.Cmd {
	return func() tea.Msg {
		agents, err := v.client.ListAgents(context.Background())
		if err != nil {
			return agentsErrorMsg{err: err}
		}
		return agentsLoadedMsg{agents: agents}
	}
}

// Update handles messages for the agents view.
func (v *AgentsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case agentsLoadedMsg:
		v.agents = msg.agents
		v.loading = false
		return v, nil

	case agentsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.agents)-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// No-op for now; future: TUI handoff.
		}
	}
	return v, nil
}

// View renders the agents list.
func (v *AgentsView) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Agents")
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine)
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading agents...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.agents) == 0 {
		b.WriteString("  No agents found.\n")
		return b.String()
	}

	for i, agent := range v.agents {
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			nameStyle = nameStyle.Bold(true)
		}

		b.WriteString(cursor + nameStyle.Render(agent.Name) + "\n")

		statusIcon := "○"
		b.WriteString("  Status: " + statusIcon + " " + agent.Status + "\n")

		if i < len(v.agents)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// Name returns the view name.
func (v *AgentsView) Name() string {
	return "agents"
}

// ShortHelp returns keybinding hints for the help bar.
func (v *AgentsView) ShortHelp() []string {
	return []string{"[Enter] Launch", "[r] Refresh", "[Esc] Back"}
}
