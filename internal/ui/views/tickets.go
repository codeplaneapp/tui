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
var _ View = (*TicketsView)(nil)

type ticketsLoadedMsg struct {
	tickets []smithers.Ticket
}

type ticketsErrorMsg struct {
	err error
}

// TicketsView displays a navigable list of tickets.
type TicketsView struct {
	client  *smithers.Client
	tickets []smithers.Ticket
	cursor  int
	width   int
	height  int
	loading bool
	err     error
}

// NewTicketsView creates a new tickets view.
func NewTicketsView(client *smithers.Client) *TicketsView {
	return &TicketsView{
		client:  client,
		loading: true,
	}
}

// Init loads tickets from the client.
func (v *TicketsView) Init() tea.Cmd {
	return func() tea.Msg {
		tickets, err := v.client.ListTickets(context.Background())
		if err != nil {
			return ticketsErrorMsg{err: err}
		}
		return ticketsLoadedMsg{tickets: tickets}
	}
}

// Update handles messages for the tickets view.
func (v *TicketsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case ticketsLoadedMsg:
		v.tickets = msg.tickets
		v.loading = false
		return v, nil

	case ticketsErrorMsg:
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
			if v.cursor < len(v.tickets)-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// No-op for now; future: detail view.
		}
	}
	return v, nil
}

// View renders the tickets list.
func (v *TicketsView) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Tickets")
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
		b.WriteString("  Loading tickets...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.tickets) == 0 {
		b.WriteString("  No tickets found.\n")
		return b.String()
	}

	for i, ticket := range v.tickets {
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			nameStyle = nameStyle.Bold(true)
		}

		b.WriteString(cursor + nameStyle.Render(ticket.ID) + "\n")

		snippet := ticketSnippet(ticket.Content)
		if snippet != "" {
			snippetStyle := lipgloss.NewStyle().Faint(true)
			b.WriteString("  " + snippetStyle.Render(snippet) + "\n")
		}

		if i < len(v.tickets)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// Name returns the view name.
func (v *TicketsView) Name() string {
	return "tickets"
}

// ShortHelp returns keybinding hints for the help bar.
func (v *TicketsView) ShortHelp() []string {
	return []string{"[Enter] View", "[r] Refresh", "[Esc] Back"}
}

// ticketSnippet returns the first non-empty, non-heading line of markdown content.
func ticketSnippet(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		if len(line) > 80 {
			return line[:77] + "..."
		}
		return line
	}
	return ""
}
