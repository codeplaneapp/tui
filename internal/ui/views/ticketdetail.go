package views

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

// Compile-time interface check.
var _ View = (*TicketDetailView)(nil)

// --- Private message types ---

type ticketDetailReloadedMsg struct {
	ticket smithers.Ticket
}

type ticketDetailErrorMsg struct {
	err error
}

// TicketDetailView is a full-screen view for a single ticket.
type TicketDetailView struct {
	client *smithers.Client
	sty    *styles.Styles
	ticket smithers.Ticket

	rendered      []string
	renderedWidth int
	scrollOffset  int

	width  int
	height int

	loading  bool
	err      error
	tmpPath  string
	autoEdit bool // when true, Init() immediately fires the external editor
}

// NewTicketDetailView creates a new full-screen detail view for the given ticket.
func NewTicketDetailView(client *smithers.Client, sty *styles.Styles, ticket smithers.Ticket) *TicketDetailView {
	return &TicketDetailView{
		client: client,
		sty:    sty,
		ticket: ticket,
	}
}

// NewTicketDetailViewEditMode creates a detail view that immediately launches
// the external editor when Init is called.
func NewTicketDetailViewEditMode(client *smithers.Client, sty *styles.Styles, ticket smithers.Ticket) *TicketDetailView {
	return &TicketDetailView{
		client:   client,
		sty:      sty,
		ticket:   ticket,
		autoEdit: true,
	}
}

// Init launches the external editor immediately when autoEdit is set;
// otherwise it is a no-op (content is already in memory).
func (v *TicketDetailView) Init() tea.Cmd {
	if v.autoEdit {
		v.autoEdit = false // only fire once
		return v.startEditor()
	}
	return nil
}

// Name returns the view name.
func (v *TicketDetailView) Name() string { return "ticket-detail" }

// SetSize stores the terminal dimensions and invalidates the render cache.
func (v *TicketDetailView) SetSize(width, height int) {
	v.width = width
	v.height = height
	// Invalidate render cache so next View() call re-renders at new width.
	v.renderedWidth = 0
}

// Update handles messages for the ticket detail view.
func (v *TicketDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case ticketDetailReloadedMsg:
		v.ticket = msg.ticket
		v.loading = false
		v.err = nil
		v.renderedWidth = 0 // invalidate cache
		v.scrollOffset = 0  // reset scroll on reload
		return v, nil

	case ticketDetailErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case handoff.HandoffMsg:
		if msg.Tag != "ticket-edit" {
			return v, nil
		}
		tmpPath := v.tmpPath
		v.tmpPath = ""

		if msg.Result.Err != nil {
			_ = os.Remove(tmpPath)
			v.err = fmt.Errorf("editor: %w", msg.Result.Err)
			return v, nil
		}
		newContentBytes, err := os.ReadFile(tmpPath)
		_ = os.Remove(tmpPath)
		if err != nil {
			v.err = fmt.Errorf("read edited file: %w", err)
			return v, nil
		}
		newContent := string(newContentBytes)
		if newContent == v.ticket.Content {
			return v, nil // no change
		}
		v.loading = true
		ticketID := v.ticket.ID
		client := v.client
		return v, func() tea.Msg {
			ctx := context.Background()
			updated, err := client.UpdateTicket(ctx, ticketID, smithers.UpdateTicketInput{Content: newContent})
			if err != nil {
				return ticketDetailErrorMsg{err: fmt.Errorf("save ticket: %w", err)}
			}
			// Use the UpdateTicket response directly when available; otherwise reload.
			if updated != nil {
				return ticketDetailReloadedMsg{ticket: *updated}
			}
			reloaded, err := client.GetTicket(ctx, ticketID)
			if err != nil {
				return ticketDetailErrorMsg{err: fmt.Errorf("reload ticket: %w", err)}
			}
			return ticketDetailReloadedMsg{ticket: *reloaded}
		}

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.scrollOffset > 0 {
				v.scrollOffset--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if max := v.maxScrollOffset(); v.scrollOffset < max {
				v.scrollOffset++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
			v.scrollOffset -= v.visibleHeight()
			if v.scrollOffset < 0 {
				v.scrollOffset = 0
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
			v.scrollOffset += v.visibleHeight()
			if max := v.maxScrollOffset(); v.scrollOffset > max {
				v.scrollOffset = max
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			v.scrollOffset = 0

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			v.scrollOffset = v.maxScrollOffset()

		case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
			if v.loading {
				return v, nil
			}
			return v, v.startEditor()
		}
	}

	return v, nil
}

// startEditor writes ticket content to a temp file and hands off to $EDITOR.
func (v *TicketDetailView) startEditor() tea.Cmd {
	editor := resolveEditor()
	tmpFile, err := os.CreateTemp("", "ticket-*.md")
	if err != nil {
		v.err = fmt.Errorf("create temp file: %w", err)
		return nil
	}
	if _, err := tmpFile.WriteString(v.ticket.Content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		v.err = err
		return nil
	}
	_ = tmpFile.Close()
	v.tmpPath = tmpFile.Name()
	return handoff.Handoff(handoff.Options{
		Binary: editor,
		Args:   []string{v.tmpPath},
		Tag:    "ticket-edit",
	})
}

// resolveEditor returns the best available editor binary.
func resolveEditor() string {
	for _, env := range []string{"EDITOR", "VISUAL"} {
		if e := os.Getenv(env); e != "" {
			if _, err := exec.LookPath(e); err == nil {
				return e
			}
		}
	}
	return "vi"
}

// visibleHeight returns the number of content rows that fit on screen.
// Reserves: 1 header + 1 blank + 1 divider + 1 help bar = 4 rows.
func (v *TicketDetailView) visibleHeight() int {
	h := v.height - 4
	if h < 1 {
		return 1
	}
	return h
}

func (v *TicketDetailView) maxScrollOffset() int {
	n := len(v.rendered) - v.visibleHeight()
	if n < 0 {
		return 0
	}
	return n
}

// renderMarkdown renders ticket content to lines, using a cache keyed on width.
func (v *TicketDetailView) renderMarkdown() {
	if v.renderedWidth == v.width && len(v.rendered) > 0 {
		return // cache hit
	}
	var out string
	if v.sty != nil {
		renderer := common.MarkdownRenderer(v.sty, v.width)
		result, err := renderer.Render(v.ticket.Content)
		if err == nil {
			out = strings.TrimSpace(result)
		} else {
			out = v.ticket.Content
		}
	} else {
		// Fallback: plain word-wrap when styles are unavailable.
		out = wrapText(v.ticket.Content, v.width)
	}
	if out == "" {
		out = lipgloss.NewStyle().Faint(true).Render("(no content)")
	}
	v.rendered = strings.Split(out, "\n")
	v.renderedWidth = v.width
	// Clamp scroll after re-render.
	if max := v.maxScrollOffset(); v.scrollOffset > max {
		v.scrollOffset = max
	}
}

// View renders the full-screen ticket detail.
func (v *TicketDetailView) View() string {
	var b strings.Builder

	// Header
	b.WriteString(v.renderHeader())
	b.WriteString("\n")

	// Separator
	w := v.width
	if w <= 0 {
		w = 40
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", w)))
	b.WriteString("\n")

	if v.loading {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Saving..."))
		b.WriteString("\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
	}

	// Render markdown (uses cache when width unchanged).
	v.renderMarkdown()

	end := v.scrollOffset + v.visibleHeight()
	if end > len(v.rendered) {
		end = len(v.rendered)
	}
	visible := v.rendered[v.scrollOffset:end]
	b.WriteString(strings.Join(visible, "\n"))
	b.WriteString("\n")

	// Help bar
	b.WriteString(v.renderHelpBar())
	return b.String()
}

// renderHeader renders the breadcrumb header with scroll info and key hints.
func (v *TicketDetailView) renderHeader() string {
	viewName := "Tickets › " + v.ticket.ID

	var scrollInfo string
	if len(v.rendered) > 0 {
		scrollInfo = fmt.Sprintf("(%d/%d)", v.scrollOffset+1, len(v.rendered))
	}

	rightSide := "[e] Edit  [Esc] Back"
	if scrollInfo != "" {
		rightSide = scrollInfo + "  " + rightSide
	}

	return ViewHeader(packageCom.Styles, "CODEPLANE", viewName, v.width, rightSide)
}

// renderHelpBar renders the bottom key-binding help bar.
func (v *TicketDetailView) renderHelpBar() string {
	var parts []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", h.Key, h.Desc))
		}
	}
	return lipgloss.NewStyle().Faint(true).Render("  "+strings.Join(parts, "  ")) + "\n"
}

// ShortHelp returns key bindings shown in the contextual help bar.
func (v *TicketDetailView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓/jk", "scroll")),
		key.NewBinding(key.WithKeys("g"), key.WithHelp("g/G", "top/bottom")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
