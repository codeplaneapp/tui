package views

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
)

var _ View = (*LandingsView)(nil)

type LandingsView struct {
	client   *jjhub.Client
	repo     *jjhub.Repo
	width    int
	height   int
	loading  bool
	err      error
	showDiff bool
}

func NewLandingsView(_ *smithers.Client) *LandingsView {
	var client *jjhub.Client
	if jjhubAvailable() {
		client = jjhub.NewClient("")
	}
	v := &LandingsView{
		client:  client,
		loading: client != nil,
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

func (v *LandingsView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	v.loading = false
	return nil
}

func (v *LandingsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			v.showDiff = !v.showDiff
		}
	}
	return v, nil
}

func (v *LandingsView) View() string {
	var b strings.Builder
	b.WriteString(jjhubHeader("JJHUB › Landings", v.width, jjhubJoinNonEmpty("  ",
		jjhubRepoLabel(v.repo),
		"[Esc] Back",
	)))
	b.WriteString("\n\n")
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error())
		return b.String()
	}
	if v.showDiff {
		b.WriteString("  Diff view is not available in this build.\n\n")
		b.WriteString(jjhubMutedStyle.Render("[d] back to detail"))
		return b.String()
	}
	b.WriteString("  Landings view is not available in this build.")
	return b.String()
}

func (v *LandingsView) Name() string { return "landings" }

func (v *LandingsView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *LandingsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle diff")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
