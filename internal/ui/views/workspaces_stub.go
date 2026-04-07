package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// WorkspacesView is a placeholder until the JJHub workspace view lands.
type WorkspacesView struct{}

func NewWorkspacesView(_ *smithers.Client) *WorkspacesView { return &WorkspacesView{} }

func (v *WorkspacesView) Init() tea.Cmd { return nil }

func (v *WorkspacesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))) {
			return v, func() tea.Msg { return PopViewMsg{} }
		}
	}
	return v, nil
}

func (v *WorkspacesView) View() string { return "Workspaces view coming soon." }

func (v *WorkspacesView) Name() string { return "workspaces" }

func (v *WorkspacesView) SetSize(_, _ int) {}

func (v *WorkspacesView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
