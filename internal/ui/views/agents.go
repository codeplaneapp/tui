package views

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/handoff"
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
	com           *common.Common
	client        *smithers.Client
	agents        []smithers.Agent
	cursor        int
	width         int
	height        int
	loading       bool
	err           error
	launching     bool
	launchingName string
}

// NewAgentsView creates a new agents view.
func NewAgentsView(args ...any) *AgentsView {
	com, client, _ := parseCommonAndClient(args)
	return &AgentsView{
		com:     com,
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

// groupAgents splits agents into usable and unavailable slices.
func groupAgents(agents []smithers.Agent) (available, unavailable []smithers.Agent) {
	for _, a := range agents {
		if a.Usable {
			available = append(available, a)
		} else {
			unavailable = append(unavailable, a)
		}
	}
	return
}

// selectedAgent returns the agent at the current cursor position across the
// combined available+unavailable list.
func (v *AgentsView) selectedAgent() *smithers.Agent {
	available, unavailable := groupAgents(v.agents)
	all := append(available, unavailable...) //nolint:gocritic
	if v.cursor >= 0 && v.cursor < len(all) {
		a := all[v.cursor]
		return &a
	}
	return nil
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

	case handoff.HandoffMsg:
		v.launching = false
		v.launchingName = ""
		tag, _ := msg.Tag.(string)
		if msg.Result.Err != nil {
			v.err = fmt.Errorf("launch %s: %w", tag, msg.Result.Err)
		}
		// Refresh agent list — auth state may have changed during the session.
		v.loading = true
		return v, v.Init()

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
			total := len(v.agents)
			if v.cursor < total-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			agent := v.selectedAgent()
			if agent == nil || !agent.Usable {
				// Not available — no-op.
				return v, nil
			}
			v.launching = true
			v.launchingName = agent.Name
			agentID := agent.ID
			binary := agent.BinaryPath
			if binary == "" {
				binary = agent.Command
			}
			return v, handoff.Handoff(handoff.Options{
				Binary: binary,
				Tag:    agentID,
			})
		}
	}
	return v, nil
}

// capitalizeRoles returns a copy of the roles slice with each role title-cased.
func capitalizeRoles(roles []string) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		if len(r) == 0 {
			out[i] = r
			continue
		}
		out[i] = strings.ToUpper(r[:1]) + r[1:]
	}
	return out
}

// View renders the agents list.
func (v *AgentsView) View() string {
	var b strings.Builder

	// Header
	b.WriteString(ViewHeader(v.com.Styles, "SMITHERS", "Agents", v.width, "[Esc] Back"))
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading agents...\n")
		return b.String()
	}

	if v.launching {
		b.WriteString(fmt.Sprintf("  Launching %s...\n", v.launchingName))
		b.WriteString("  Smithers TUI will resume when you exit.\n")
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

	available, unavailable := groupAgents(v.agents)

	// For wide terminals, use a two-column layout.
	if v.width >= 100 {
		v.renderWide(&b, available, unavailable)
	} else {
		v.renderNarrow(&b, available, unavailable)
	}

	return b.String()
}

// renderNarrow renders the single-column agents list.
func (v *AgentsView) renderNarrow(b *strings.Builder, available, unavailable []smithers.Agent) {
	cursorOffset := 0

	if len(available) > 0 {
		sectionHeader := lipgloss.NewStyle().Bold(true).Faint(true).
			Render(fmt.Sprintf("Available (%d)", len(available)))
		b.WriteString("  " + sectionHeader + "\n\n")

		for i, agent := range available {
			v.writeAgentRow(b, agent, cursorOffset+i, true)
		}
		cursorOffset += len(available)
	}

	if len(unavailable) > 0 {
		if len(available) > 0 {
			divWidth := v.width - 4
			if divWidth < 10 {
				divWidth = 10
			}
			b.WriteString("\n  " + strings.Repeat("─", divWidth) + "\n\n")
		}
		sectionHeader := lipgloss.NewStyle().Bold(true).Faint(true).
			Render(fmt.Sprintf("Not Detected (%d)", len(unavailable)))
		b.WriteString("  " + sectionHeader + "\n\n")

		for i, agent := range unavailable {
			v.writeAgentRow(b, agent, cursorOffset+i, false)
		}
	}
}

// writeAgentRow writes a single agent row into the builder.
func (v *AgentsView) writeAgentRow(b *strings.Builder, agent smithers.Agent, idx int, detailed bool) {
	t := v.com.Styles
	isSelected := idx == v.cursor
	cursor := "  "
	nameStyle := lipgloss.NewStyle()
	if isSelected {
		cursor = "▸ "
		nameStyle = nameStyle.Bold(true)
	}

	b.WriteString(cursor + nameStyle.Render(agent.Name) + "\n")

	if detailed {
		binaryLabel := agent.BinaryPath
		if binaryLabel == "" {
			binaryLabel = "—"
		}
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("Binary: "+binaryLabel) + "\n")
	}

	icon := agentStatusIcon(agent.Status)
	styledIcon := agentStatusStyle(t, agent.Status).Render(icon)
	b.WriteString("  " + styledIcon + " " + agent.Status)

	if detailed {
		b.WriteString(fmt.Sprintf("   Auth: %s  API Key: %s", styledCheck(t, agent.HasAuth), styledCheck(t, agent.HasAPIKey)))
		if len(agent.Roles) > 0 {
			b.WriteString("   Roles: " + strings.Join(capitalizeRoles(agent.Roles), ", "))
		}
	}
	b.WriteString("\n\n")
}

// renderWide renders a two-column layout for terminals wider than 100 columns.
func (v *AgentsView) renderWide(b *strings.Builder, available, unavailable []smithers.Agent) {
	t := v.com.Styles
	const leftWidth = 36
	rightWidth := v.width - leftWidth - 3
	if rightWidth < 20 {
		v.renderNarrow(b, available, unavailable)
		return
	}

	// Build left pane lines.
	var leftLines []string
	combined := append(available, unavailable...)

	if len(available) > 0 {
		leftLines = append(leftLines,
			lipgloss.NewStyle().Bold(true).Faint(true).
				Render(fmt.Sprintf("Available (%d)", len(available))),
			"",
		)
		for i, a := range available {
			isSelected := i == v.cursor
			prefix := "  "
			style := lipgloss.NewStyle()
			if isSelected {
				prefix = "▸ "
				style = style.Bold(true)
			}
			leftLines = append(leftLines, prefix+style.Render(a.Name))
		}
	}

	if len(unavailable) > 0 {
		if len(available) > 0 {
			leftLines = append(leftLines, "", strings.Repeat("─", leftWidth-2), "")
		}
		leftLines = append(leftLines,
			lipgloss.NewStyle().Bold(true).Faint(true).
				Render(fmt.Sprintf("Not Detected (%d)", len(unavailable))),
			"",
		)
		for i, a := range unavailable {
			isSelected := len(available)+i == v.cursor
			prefix := "  "
			style := lipgloss.NewStyle().Faint(true)
			if isSelected {
				prefix = "▸ "
				style = style.Bold(true)
			}
			leftLines = append(leftLines, prefix+style.Render(a.Name))
		}
	}

	// Build right pane (detail for selected agent).
	var rightLines []string
	if v.cursor >= 0 && v.cursor < len(combined) {
		a := combined[v.cursor]
		rightLines = append(rightLines,
			lipgloss.NewStyle().Bold(true).Render(a.Name),
			"",
		)
		binaryLabel := a.BinaryPath
		if binaryLabel == "" {
			binaryLabel = "—"
		}
		rightLines = append(rightLines, "Binary: "+lipgloss.NewStyle().Faint(true).Render(binaryLabel))

		icon := agentStatusIcon(a.Status)
		styledIcon := agentStatusStyle(t, a.Status).Render(icon)
		rightLines = append(rightLines, "Status: "+styledIcon+" "+a.Status)

		rightLines = append(rightLines,
			fmt.Sprintf("Auth:    %s", styledCheck(t, a.HasAuth)),
			fmt.Sprintf("API Key: %s", styledCheck(t, a.HasAPIKey)),
		)
		if len(a.Roles) > 0 {
			rightLines = append(rightLines, "Roles:   "+strings.Join(capitalizeRoles(a.Roles), ", "))
		}
		rightLines = append(rightLines, "")
		if a.Usable {
			rightLines = append(rightLines,
				lipgloss.NewStyle().Faint(true).Render("[Enter] Launch TUI"),
			)
		} else {
			rightLines = append(rightLines,
				lipgloss.NewStyle().Faint(true).Render("(install to launch)"),
			)
		}
	}

	// Merge the two panes side-by-side.
	leftStyle := lipgloss.NewStyle().Width(leftWidth)
	rows := len(leftLines)
	if len(rightLines) > rows {
		rows = len(rightLines)
	}
	for i := 0; i < rows; i++ {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		b.WriteString(leftStyle.Render(left) + " │ " + right + "\n")
	}
}

// Name returns the view name.
func (v *AgentsView) Name() string {
	return "agents"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *AgentsView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *AgentsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "launch")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
