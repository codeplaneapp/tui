package views

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	ghrepo "github.com/charmbracelet/crush/internal/github"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*TicketsView)(nil)

type ticketsLoadedMsg struct {
	tickets []smithers.Ticket
}

type ticketsErrorMsg struct {
	err error
}

// ticketCreatedMsg is sent when a new ticket has been successfully created.
type ticketCreatedMsg struct {
	ticket smithers.Ticket
}

// ticketCreateErrorMsg is sent when ticket creation fails.
type ticketCreateErrorMsg struct {
	err error
}

// OpenTicketDetailMsg signals ui.go to push a TicketDetailView.
// When EditMode is true, the detail view launches $EDITOR immediately on init.
type OpenTicketDetailMsg struct {
	Ticket   smithers.Ticket
	Client   *smithers.Client
	EditMode bool
}

// --- Private pane types ---

// ticketListPane is the left pane of the tickets split view.
// It owns cursor navigation and viewport clipping.
type ticketListPane struct {
	tickets      []smithers.Ticket
	cursor       int
	scrollOffset int
	width        int
	height       int
}

func (p *ticketListPane) Init() tea.Cmd { return nil }

func (p *ticketListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
			if p.cursor > 0 {
				p.cursor--
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
			if p.cursor < len(p.tickets)-1 {
				p.cursor++
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("home", "g"))):
			p.cursor = 0
			p.scrollOffset = 0
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("end", "G"))):
			if len(p.tickets) > 0 {
				p.cursor = len(p.tickets) - 1
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
			ps := p.pageSize()
			p.cursor -= ps
			if p.cursor < 0 {
				p.cursor = 0
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
			ps := p.pageSize()
			p.cursor += ps
			if len(p.tickets) > 0 && p.cursor >= len(p.tickets) {
				p.cursor = len(p.tickets) - 1
			}
		}
	}
	return p, nil
}

func (p *ticketListPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *ticketListPane) pageSize() int {
	const linesPerTicket = 3
	if p.height <= 0 {
		return 1
	}
	n := p.height / linesPerTicket
	if n < 1 {
		return 1
	}
	return n
}

func (p *ticketListPane) View() string {
	if len(p.tickets) == 0 {
		return ""
	}

	var b strings.Builder

	visibleCount := p.pageSize()
	if visibleCount > len(p.tickets) {
		visibleCount = len(p.tickets)
	}

	// Keep cursor visible.
	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	}
	if p.cursor >= p.scrollOffset+visibleCount {
		p.scrollOffset = p.cursor - visibleCount + 1
	}

	end := p.scrollOffset + visibleCount
	if end > len(p.tickets) {
		end = len(p.tickets)
	}

	maxSnippetLen := 80
	if p.width > 4 {
		maxSnippetLen = p.width - 4
	}

	for i := p.scrollOffset; i < end; i++ {
		t := p.tickets[i]
		cursorStr := "  "
		nameStyle := lipgloss.NewStyle()
		if i == p.cursor {
			cursorStr = "▸ "
			nameStyle = nameStyle.Bold(true)
		}
		b.WriteString(cursorStr + nameStyle.Render(t.ID) + "\n")
		if snippet := ticketSnippet(t.Content, maxSnippetLen); snippet != "" {
			b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(snippet) + "\n")
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator when list is clipped.
	if len(p.tickets) > visibleCount {
		b.WriteString(fmt.Sprintf("\n  (%d/%d)", p.cursor+1, len(p.tickets)))
	}

	return b.String()
}

// ticketDetailPane is the right pane of the tickets split view.
// It renders the full content of the currently focused ticket.
type ticketDetailPane struct {
	tickets []smithers.Ticket
	cursor  *int // points to ticketListPane.cursor; nil-safe
	width   int
	height  int
}

func (p *ticketDetailPane) Init() tea.Cmd { return nil }

func (p *ticketDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	return p, nil // read-only in v1
}

func (p *ticketDetailPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *ticketDetailPane) View() string {
	if len(p.tickets) == 0 || p.cursor == nil || *p.cursor >= len(p.tickets) {
		return lipgloss.NewStyle().Faint(true).Render("Select a ticket")
	}
	t := p.tickets[*p.cursor]
	title := lipgloss.NewStyle().Bold(true).Render(t.ID)
	body := wrapText(t.Content, p.width)
	return title + "\n\n" + body
}

// createPromptState holds the inline "new ticket" prompt state.
type createPromptState struct {
	active bool
	input  textinput.Model
	err    error // last create error, shown inline
}

// TicketsView displays a split-pane tickets browser.
// Left pane: navigable list of tickets. Right pane: detail view for the selected ticket.
type TicketsView struct {
	client     *smithers.Client
	tickets    []smithers.Ticket
	width      int
	height     int
	loading    bool
	err        error
	splitPane  *components.SplitPane
	listPane   *ticketListPane
	detailPane *ticketDetailPane

	// Inline create-ticket prompt (activated by 'n').
	createPrompt createPromptState
}

// NewTicketsView creates a new tickets view.
func NewTicketsView(client *smithers.Client) *TicketsView {
	list := &ticketListPane{}
	detail := &ticketDetailPane{cursor: &list.cursor}
	sp := components.NewSplitPane(list, detail, components.SplitPaneOpts{
		LeftWidth:         30,
		CompactBreakpoint: 79,
	})

	ti := textinput.New()
	ti.Placeholder = "ticket-id (e.g. feat-login-flow)"
	ti.SetVirtualCursor(true)

	return &TicketsView{
		client:       client,
		loading:      true,
		splitPane:    sp,
		listPane:     list,
		detailPane:   detail,
		createPrompt: createPromptState{input: ti},
	}
}

// NewTicketsViewWithSources preserves the older test constructor shape while
// tickets remain Smithers-backed in the current implementation.
func NewTicketsViewWithSources(client *smithers.Client, _ *jjhub.Client, _ *ghrepo.Client) *TicketsView {
	return NewTicketsView(client)
}

func (v *TicketsView) activeItems() []smithers.Ticket {
	return v.tickets
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

// createTicketCmd returns a tea.Cmd that calls CreateTicket with the given ID.
func (v *TicketsView) createTicketCmd(id string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		ticket, err := client.CreateTicket(context.Background(), smithers.CreateTicketInput{ID: id})
		if err != nil {
			return ticketCreateErrorMsg{err: err}
		}
		return ticketCreatedMsg{ticket: *ticket}
	}
}

// Update handles messages for the tickets view.
func (v *TicketsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case ticketsLoadedMsg:
		v.tickets = msg.tickets
		v.listPane.tickets = msg.tickets
		v.detailPane.tickets = msg.tickets
		v.loading = false
		v.splitPane.SetSize(v.width, max(0, v.height-2))
		return v, nil

	case ticketsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case ticketCreatedMsg:
		// Dismiss prompt and refresh the list so the new ticket appears.
		v.createPrompt.active = false
		v.createPrompt.input.Reset()
		v.createPrompt.err = nil
		v.loading = true
		return v, v.Init()

	case ticketCreateErrorMsg:
		// Surface the error inside the prompt so the user can correct the ID.
		v.createPrompt.err = msg.err
		return v, nil

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		// When the create prompt is active, route keys there first.
		if v.createPrompt.active {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				// Dismiss the prompt without creating anything.
				v.createPrompt.active = false
				v.createPrompt.input.Reset()
				v.createPrompt.err = nil
				return v, nil

			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				id := strings.TrimSpace(v.createPrompt.input.Value())
				if id == "" {
					return v, nil // ignore empty submit
				}
				v.createPrompt.err = nil
				return v, v.createTicketCmd(id)

			default:
				var tiCmd tea.Cmd
				v.createPrompt.input, tiCmd = v.createPrompt.input.Update(msg)
				return v, tiCmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			return v, v.Init()
		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			// Open inline create-ticket prompt.
			if !v.loading {
				v.createPrompt.active = true
				v.createPrompt.err = nil
				v.createPrompt.input.Reset()
				cmd := v.createPrompt.input.Focus()
				return v, cmd
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
			// Open the selected ticket in edit mode (pushes detail + fires editor).
			if v.splitPane.Focus() == components.FocusLeft && len(v.tickets) > 0 && v.listPane.cursor < len(v.tickets) {
				t := v.tickets[v.listPane.cursor]
				client := v.client
				return v, func() tea.Msg {
					return OpenTicketDetailMsg{Ticket: t, Client: client, EditMode: true}
				}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Only open detail when the left (list) pane has focus and tickets are loaded.
			if v.splitPane.Focus() == components.FocusLeft && len(v.tickets) > 0 && v.listPane.cursor < len(v.tickets) {
				t := v.tickets[v.listPane.cursor]
				client := v.client
				return v, func() tea.Msg {
					return OpenTicketDetailMsg{Ticket: t, Client: client}
				}
			}
		}
	}

	// All other messages (including Tab, j/k, etc.) go to the split pane.
	newSP, cmd := v.splitPane.Update(msg)
	v.splitPane = newSP
	return v, cmd
}

// View renders the tickets view.
func (v *TicketsView) View() string {
	var b strings.Builder

	// Header — include ticket count after loading.
	title := "SMITHERS \u203a Tickets"
	if !v.loading && v.err == nil {
		title = fmt.Sprintf("SMITHERS \u203a Tickets (%d)", len(v.tickets))
	}
	header := lipgloss.NewStyle().Bold(true).Render(title)
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine + "\n\n")

	if v.loading {
		b.WriteString("  Loading tickets...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	// Inline create-ticket prompt (shown when 'n' is pressed).
	if v.createPrompt.active {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("New ticket ID:") + " " + v.createPrompt.input.View() + "\n")
		if v.createPrompt.err != nil {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+v.createPrompt.err.Error()) + "\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [Enter] create  [Esc] cancel") + "\n")
		return b.String()
	}

	if len(v.tickets) == 0 {
		b.WriteString("  No tickets found.\n")
		return b.String()
	}

	// Split pane fills the remaining height.
	b.WriteString(v.splitPane.View())
	return b.String()
}

// Name returns the view name.
func (v *TicketsView) Name() string {
	return "tickets"
}

// ticketSnippet extracts a short preview from ticket content.
// Prefers text after ## Summary or ## Description headings; skips metadata lines.
func ticketSnippet(content string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	lines := strings.Split(content, "\n")
	inSummary := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if lower == "## summary" || lower == "## description" {
			inSummary = true
			continue
		}
		if inSummary && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if len(trimmed) > maxLen {
				return trimmed[:maxLen-3] + "..."
			}
			return trimmed
		}
	}
	// Fallback: first non-heading, non-metadata, non-empty line.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") || metadataLine(trimmed) {
			continue
		}
		if len(trimmed) > maxLen {
			return trimmed[:maxLen-3] + "..."
		}
		return trimmed
	}
	return ""
}

// metadataLine returns true if the line matches "- Key: Value" metadata format.
func metadataLine(line string) bool {
	if !strings.HasPrefix(line, "- ") {
		return false
	}
	rest := line[2:]
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return false
	}
	key := rest[:idx]
	return !strings.Contains(key, " ") || len(strings.Fields(key)) <= 2
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *TicketsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.splitPane.SetSize(width, max(0, height-2))
}

// ShortHelp returns keybinding hints for the help bar.
func (v *TicketsView) ShortHelp() []key.Binding {
	if v.createPrompt.active {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "create")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	if v.splitPane != nil && v.splitPane.Focus() == components.FocusRight {
		return []key.Binding{
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "list")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "select")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "detail")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
