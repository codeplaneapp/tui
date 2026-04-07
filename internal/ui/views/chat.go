package views

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/event"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"go.opentelemetry.io/otel/attribute"
)

var _ View = (*ChatView)(nil)

type chatTargetsLoadedMsg struct {
	agents []smithers.Agent
}

type chatTargetsErrorMsg struct {
	err error
}

type chatTargetKind int

const (
	chatTargetSmithers chatTargetKind = iota
	chatTargetAgent
)

func (k chatTargetKind) String() string {
	switch k {
	case chatTargetSmithers:
		return "smithers"
	case chatTargetAgent:
		return "external_agent"
	default:
		return "unknown"
	}
}

type chatTarget struct {
	kind        chatTargetKind
	id          string
	name        string
	desc        string
	status      string
	roles       []string
	binary      string
	recommended bool
	usable      bool
}

// ChatView lets the user choose between embedded Smithers chat and installed
// external CLI agents.
type ChatView struct {
	client *smithers.Client

	targets []chatTarget
	cursor  int
	width   int
	height  int

	loading       bool
	err           error
	launching     bool
	launchingName string
	launchingPath string
}

// NewChatView creates the chat target picker.
func NewChatView(client *smithers.Client) *ChatView {
	return &ChatView{
		client:  client,
		targets: buildChatTargets(nil),
		loading: client != nil,
	}
}

// Init records that the picker was opened and refreshes installed external
// agents when a Smithers client is available.
func (v *ChatView) Init() tea.Cmd {
	externalTargets := max(0, len(v.targets)-1)

	slog.Info("Chat target picker opened", "source", "dashboard", "has_client", v.client != nil)
	event.ChatTargetPickerOpened(
		"source", "dashboard",
		"has_client", v.client != nil,
		"total_targets", len(v.targets),
		"external_targets", externalTargets,
	)
	observability.RecordUINavigation("dashboard", "chat_picker", "ok",
		attribute.Bool("crush.chat.has_client", v.client != nil),
		attribute.Int("crush.chat.total_targets", len(v.targets)),
		attribute.Int("crush.chat.external_targets", externalTargets),
	)

	return v.loadTargetsCmd()
}

func (v *ChatView) loadTargetsCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}

	return func() tea.Msg {
		agents, err := v.client.ListAgents(context.Background())
		if err != nil {
			return chatTargetsErrorMsg{err: err}
		}
		return chatTargetsLoadedMsg{agents: agents}
	}
}

func buildChatTargets(agents []smithers.Agent) []chatTarget {
	targets := []chatTarget{
		{
			kind:        chatTargetSmithers,
			id:          "smithers",
			name:        "Smithers",
			desc:        "Use the built-in chat without leaving Smithers TUI.",
			recommended: true,
			usable:      true,
		},
	}

	for _, agent := range agents {
		if !agent.Usable {
			continue
		}
		binary := agent.BinaryPath
		if binary == "" {
			binary = agent.Command
		}
		targets = append(targets, chatTarget{
			kind:   chatTargetAgent,
			id:     agent.ID,
			name:   agent.Name,
			desc:   fmt.Sprintf("Launch the %s CLI in this terminal.", agent.Name),
			status: agent.Status,
			roles:  agent.Roles,
			binary: binary,
			usable: true,
		})
	}

	return targets
}

func (v *ChatView) clampCursor() {
	if len(v.targets) == 0 {
		v.cursor = 0
		return
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.targets) {
		v.cursor = len(v.targets) - 1
	}
}

func (v *ChatView) selectedTarget() *chatTarget {
	if v.cursor < 0 || v.cursor >= len(v.targets) {
		return nil
	}
	target := v.targets[v.cursor]
	return &target
}

func (v *ChatView) externalTargetIDs() []string {
	if len(v.targets) <= 1 {
		return nil
	}

	ids := make([]string, 0, len(v.targets)-1)
	for _, target := range v.targets[1:] {
		ids = append(ids, target.id)
	}
	return ids
}

func (v *ChatView) logTargetsLoaded() {
	externalTargets := max(0, len(v.targets)-1)
	externalIDs := v.externalTargetIDs()

	slog.Info("Chat target picker loaded",
		"total_targets", len(v.targets),
		"external_targets", externalTargets,
		"targets", externalIDs,
	)
	observability.RecordUINavigation("chat_picker", "chat_target_discovery", "ok",
		attribute.Int("crush.chat.total_targets", len(v.targets)),
		attribute.Int("crush.chat.external_targets", externalTargets),
		attribute.StringSlice("crush.chat.targets", externalIDs),
	)
}

func (v *ChatView) logTargetSelection(target chatTarget) {
	slog.Info("Chat target selected",
		"target", target.id,
		"kind", target.kind.String(),
		"status", target.status,
		"recommended", target.recommended,
		"binary", target.binary,
	)
	event.ChatTargetSelected(
		"target", target.id,
		"kind", target.kind.String(),
		"status", target.status,
		"recommended", target.recommended,
		"binary", target.binary,
	)
	observability.RecordUINavigation("chat_picker", "chat_target_select", "ok",
		attribute.String("crush.chat.target", target.id),
		attribute.String("crush.chat.kind", target.kind.String()),
		attribute.String("crush.chat.status", target.status),
		attribute.Bool("crush.chat.recommended", target.recommended),
		attribute.String("crush.chat.binary", target.binary),
	)
}

// Update handles chat target selection and external handoff lifecycle.
func (v *ChatView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case chatTargetsLoadedMsg:
		v.targets = buildChatTargets(msg.agents)
		v.loading = false
		v.err = nil
		v.clampCursor()
		v.logTargetsLoaded()
		return v, nil

	case chatTargetsErrorMsg:
		v.loading = false
		v.err = msg.err
		v.clampCursor()
		slog.Error("Failed to load chat targets", "error", msg.err)
		event.Error(msg.err, "feature", "chat_target_picker")
		observability.RecordUINavigation("chat_picker", "chat_target_discovery", "error",
			attribute.String("crush.chat.error_type", fmt.Sprintf("%T", msg.err)),
		)
		return v, nil

	case handoff.HandoffMsg:
		v.launching = false
		v.launchingName = ""
		tag, _ := msg.Tag.(string)
		binary := v.launchingPath
		v.launchingPath = ""

		if msg.Result.Err != nil {
			v.err = fmt.Errorf("launch %s: %w", tag, msg.Result.Err)
			slog.Error("External chat target failed",
				"target", tag,
				"exit_code", msg.Result.ExitCode,
				"duration_ms", msg.Result.Duration.Milliseconds(),
				"error", msg.Result.Err,
			)
			event.ChatTargetHandoffFailed(
				"target", tag,
				"exit_code", msg.Result.ExitCode,
				"duration_ms", msg.Result.Duration.Milliseconds(),
			)
			event.Error(msg.Result.Err,
				"feature", "chat_target_handoff",
				"target", tag,
				"exit_code", msg.Result.ExitCode,
			)
			observability.RecordUINavigation("chat_picker", "chat_target_handoff", "error",
				attribute.String("crush.chat.target", tag),
				attribute.String("crush.chat.kind", chatTargetAgent.String()),
				attribute.String("crush.chat.binary", binary),
				attribute.Int("crush.chat.exit_code", msg.Result.ExitCode),
				attribute.Int64("crush.chat.duration_ms", msg.Result.Duration.Milliseconds()),
			)
		} else {
			slog.Info("External chat target exited",
				"target", tag,
				"exit_code", msg.Result.ExitCode,
				"duration_ms", msg.Result.Duration.Milliseconds(),
			)
			event.ChatTargetHandoffCompleted(
				"target", tag,
				"exit_code", msg.Result.ExitCode,
				"duration_ms", msg.Result.Duration.Milliseconds(),
			)
			observability.RecordUINavigation("chat_picker", "chat_target_handoff", "ok",
				attribute.String("crush.chat.target", tag),
				attribute.String("crush.chat.kind", chatTargetAgent.String()),
				attribute.String("crush.chat.binary", binary),
				attribute.Int("crush.chat.exit_code", msg.Result.ExitCode),
				attribute.Int64("crush.chat.duration_ms", msg.Result.Duration.Milliseconds()),
			)
		}

		v.loading = v.client != nil
		if !v.loading {
			return v, nil
		}
		return v, v.loadTargetsCmd()

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			v.cursor--
			v.clampCursor()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			v.cursor++
			v.clampCursor()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = v.client != nil
			v.err = nil
			return v, v.loadTargetsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			target := v.selectedTarget()
			if target == nil || !target.usable {
				return v, nil
			}

			v.logTargetSelection(*target)

			if target.kind == chatTargetSmithers {
				slog.Info("Opening embedded Smithers chat", "target", target.id)
				return v, func() tea.Msg { return OpenChatMsg{} }
			}

			v.launching = true
			v.launchingName = target.name
			v.launchingPath = target.binary
			slog.Info("Launching external chat target",
				"target", target.id,
				"binary", target.binary,
			)
			return v, handoff.Handoff(handoff.Options{
				Binary: target.binary,
				Tag:    target.id,
			})
		}
	}

	return v, nil
}

// View renders the chat target picker.
func (v *ChatView) View() string {
	var b strings.Builder

	b.WriteString(ViewHeader(packageCom.Styles, "CODEPLANE", "Start Chat", v.width, "[Enter] Open  [Esc] Back"))
	b.WriteString("\n\n")
	b.WriteString("  Choose how you want to chat in this workspace.\n\n")

	for i, target := range v.targets {
		b.WriteString(v.renderTargetRow(target, i == v.cursor))
		b.WriteString("\n")
	}

	if v.loading {
		b.WriteString("\n  Detecting installed agents...\n")
	}

	if len(v.targets) == 1 && !v.loading {
		b.WriteString("\n  No external chat agents detected on PATH.\n")
	}

	if v.launching {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Launching %s...\n", v.launchingName))
		b.WriteString("  Smithers TUI will resume when you exit.\n")
	}

	if v.err != nil {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
	}

	return b.String()
}

func (v *ChatView) renderTargetRow(target chatTarget, selected bool) string {
	var b strings.Builder

	prefix := "  "
	if selected {
		prefix = "▸ "
	}

	nameStyle := lipgloss.NewStyle().Bold(true)
	if selected {
		nameStyle = nameStyle.Foreground(lipgloss.Color("12"))
	}

	title := nameStyle.Render(target.name)
	switch {
	case target.recommended:
		badge := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10")).
			Render("Recommended")
		title = title + "  " + badge
	case target.status != "":
		title = title + "  " + agentStatusStyle(target.status).Render(agentStatusIcon(target.status))
	}

	b.WriteString(prefix + title + "\n")
	b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(target.desc) + "\n")

	meta := "Built in"
	if target.kind == chatTargetAgent {
		parts := []string{chatTargetStatusLabel(target.status)}
		if len(target.roles) > 0 {
			parts = append(parts, strings.Join(capitalizeRoles(target.roles), ", "))
		}
		if target.binary != "" {
			parts = append(parts, target.binary)
		}
		meta = strings.Join(parts, " • ")
	}
	b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(meta))

	return b.String()
}

func chatTargetStatusLabel(status string) string {
	switch status {
	case "likely-subscription":
		return "Signed in"
	case "api-key":
		return "API key"
	case "binary-only":
		return "Binary only"
	default:
		return "Available"
	}
}

func (v *ChatView) Name() string {
	return "chat"
}

// SetSize stores the terminal dimensions for rendering.
func (v *ChatView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns contextual help for the picker.
func (v *ChatView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
