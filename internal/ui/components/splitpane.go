package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Pane is the interface that child panes must satisfy.
// It is intentionally more minimal than views.View (no Name(), no ShortHelp())
// to avoid coupling the layout component to the router.
type Pane interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Pane, tea.Cmd)
	View() string
	SetSize(width, height int)
}

// FocusSide identifies which pane of a SplitPane is focused.
type FocusSide int

const (
	FocusLeft  FocusSide = iota
	FocusRight
)

// SplitPaneOpts configures a SplitPane.
type SplitPaneOpts struct {
	// LeftWidth is the fixed width of the left pane. Default: 30.
	LeftWidth int
	// DividerWidth is the width of the vertical divider gutter. Default: 1.
	DividerWidth int
	// CompactBreakpoint is the width below which the split collapses to a
	// single pane showing only the focused side. Default: 80.
	CompactBreakpoint int
	// FocusedBorderColor is the color of the focused-pane left border accent.
	// Default: "99" (purple).
	FocusedBorderColor string
	// DividerColor is the foreground color of the divider character.
	// Default: "240" (dim gray).
	DividerColor string
}

// SplitPane renders two Pane children side-by-side with a configurable
// fixed-width left pane and a responsive right pane.
//
// Focus management: Tab toggles focus; key/mouse events route only to the
// focused pane. In compact mode (width < CompactBreakpoint), only the
// focused pane is visible.
type SplitPane struct {
	left, right   Pane
	focus         FocusSide
	opts          SplitPaneOpts
	width, height int
	compact       bool
}

// NewSplitPane constructs a SplitPane with sensible defaults.
func NewSplitPane(left, right Pane, opts SplitPaneOpts) *SplitPane {
	if opts.LeftWidth == 0 {
		opts.LeftWidth = 30
	}
	if opts.DividerWidth == 0 {
		opts.DividerWidth = 1
	}
	if opts.CompactBreakpoint == 0 {
		opts.CompactBreakpoint = 80
	}
	if opts.FocusedBorderColor == "" {
		opts.FocusedBorderColor = "99"
	}
	if opts.DividerColor == "" {
		opts.DividerColor = "240"
	}
	return &SplitPane{
		left:  left,
		right: right,
		focus: FocusLeft,
		opts:  opts,
	}
}

// Init calls Init on both child panes and batches their commands.
func (sp *SplitPane) Init() tea.Cmd {
	return tea.Batch(sp.left.Init(), sp.right.Init())
}

// SetSize propagates dimensions to children, applying compact-mode logic.
func (sp *SplitPane) SetSize(width, height int) {
	sp.width = width
	sp.height = height
	sp.compact = width < sp.opts.CompactBreakpoint

	if sp.compact {
		// Single-pane mode: give all space to the focused pane only.
		switch sp.focus {
		case FocusLeft:
			sp.left.SetSize(width, height)
		case FocusRight:
			sp.right.SetSize(width, height)
		}
		return
	}

	leftWidth := sp.clampLeftWidth(width)
	rightWidth := width - leftWidth - sp.opts.DividerWidth
	if rightWidth < 0 {
		rightWidth = 0
	}
	// Left pane: subtract 1 column for the focus-border accent on the active pane.
	if sp.focus == FocusLeft {
		sp.left.SetSize(leftWidth-1, height)
	} else {
		sp.left.SetSize(leftWidth, height)
	}
	if sp.focus == FocusRight {
		sp.right.SetSize(rightWidth-1, height)
	} else {
		sp.right.SetSize(rightWidth, height)
	}
}

// clampLeftWidth ensures the left pane never exceeds half the available width.
func (sp *SplitPane) clampLeftWidth(totalWidth int) int {
	lw := sp.opts.LeftWidth
	if lw > totalWidth/2 {
		lw = totalWidth / 2
	}
	return lw
}

// Update routes messages. Tab/Shift+Tab toggle focus; all other messages are
// forwarded to the focused pane only. tea.WindowSizeMsg is handled by calling
// SetSize and is NOT forwarded to children.
func (sp *SplitPane) Update(msg tea.Msg) (*SplitPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		sp.SetSize(msg.Width, msg.Height)
		return sp, nil
	case tea.KeyPressMsg:
		tabBinding := key.NewBinding(key.WithKeys("tab"))
		shiftTabBinding := key.NewBinding(key.WithKeys("shift+tab"))
		if key.Matches(msg, tabBinding) || key.Matches(msg, shiftTabBinding) {
			sp.ToggleFocus()
			return sp, nil
		}
	}

	var cmd tea.Cmd
	switch sp.focus {
	case FocusLeft:
		newLeft, c := sp.left.Update(msg)
		sp.left = newLeft
		cmd = c
	case FocusRight:
		newRight, c := sp.right.Update(msg)
		sp.right = newRight
		cmd = c
	}
	return sp, cmd
}

// ToggleFocus flips focus between left and right panes.
// In compact mode, SetSize is re-called so the newly-focused pane gets space.
func (sp *SplitPane) ToggleFocus() {
	if sp.focus == FocusLeft {
		sp.focus = FocusRight
	} else {
		sp.focus = FocusLeft
	}
	if sp.compact {
		// Swap which pane gets the full width in compact mode.
		sp.SetSize(sp.width, sp.height)
	} else {
		// Re-propagate sizes so the border accent moves to the new focused pane.
		sp.SetSize(sp.width, sp.height)
	}
}

// View renders the split pane using lipgloss.JoinHorizontal.
// In compact mode, only the focused pane is rendered (no divider).
func (sp *SplitPane) View() string {
	if sp.compact {
		switch sp.focus {
		case FocusLeft:
			return sp.left.View()
		default:
			return sp.right.View()
		}
	}

	leftWidth := sp.clampLeftWidth(sp.width)
	rightWidth := sp.width - leftWidth - sp.opts.DividerWidth
	if rightWidth < 0 {
		rightWidth = 0
	}

	accentColor := lipgloss.Color(sp.opts.FocusedBorderColor)

	var leftStyled string
	if sp.focus == FocusLeft {
		// Focused: left thick-border accent (1 col) + content (leftWidth-1 cols).
		accent := lipgloss.NewStyle().
			Foreground(accentColor).
			Height(sp.height).
			Render(strings.Repeat("┃\n", max(0, sp.height-1)) + "┃")
		content := lipgloss.NewStyle().
			Width(leftWidth - 1).MaxWidth(leftWidth - 1).Height(sp.height).
			Render(sp.left.View())
		leftStyled = lipgloss.JoinHorizontal(lipgloss.Top, accent, content)
	} else {
		leftStyled = lipgloss.NewStyle().
			Width(leftWidth).MaxWidth(leftWidth).Height(sp.height).
			Render(sp.left.View())
	}

	divider := sp.renderDivider()

	var rightStyled string
	if sp.focus == FocusRight {
		// Focused: left thick-border accent (1 col) + content (rightWidth-1 cols).
		accent := lipgloss.NewStyle().
			Foreground(accentColor).
			Height(sp.height).
			Render(strings.Repeat("┃\n", max(0, sp.height-1)) + "┃")
		content := lipgloss.NewStyle().
			Width(rightWidth - 1).MaxWidth(rightWidth - 1).Height(sp.height).
			Render(sp.right.View())
		rightStyled = lipgloss.JoinHorizontal(lipgloss.Top, accent, content)
	} else {
		rightStyled = lipgloss.NewStyle().
			Width(rightWidth).MaxWidth(rightWidth).Height(sp.height).
			Render(sp.right.View())
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, divider, rightStyled)
}

// renderDivider renders the vertical divider column.
func (sp *SplitPane) renderDivider() string {
	divChar := strings.Repeat("│\n", max(0, sp.height-1)) + "│"
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(sp.opts.DividerColor)).
		Width(sp.opts.DividerWidth).
		Height(sp.height).
		Render(divChar)
}

// --- Public accessors ---

// Focus returns which pane is currently focused.
func (sp *SplitPane) Focus() FocusSide { return sp.focus }

// SetFocus programmatically sets the focused pane and re-propagates sizes.
func (sp *SplitPane) SetFocus(f FocusSide) {
	sp.focus = f
	sp.SetSize(sp.width, sp.height)
}

// IsCompact reports whether the split pane is in compact (single-pane) mode.
func (sp *SplitPane) IsCompact() bool { return sp.compact }

// Left returns the left child pane.
func (sp *SplitPane) Left() Pane { return sp.left }

// Right returns the right child pane.
func (sp *SplitPane) Right() Pane { return sp.right }

// Width returns the total allocated width.
func (sp *SplitPane) Width() int { return sp.width }

// Height returns the allocated height.
func (sp *SplitPane) Height() int { return sp.height }

// ShortHelp returns key bindings shown in contextual help.
func (sp *SplitPane) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
	}
}
