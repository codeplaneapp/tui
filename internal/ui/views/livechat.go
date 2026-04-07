package views

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*LiveChatView)(nil)

// --- Messages ---

type liveChatRunLoadedMsg struct {
	run *smithers.RunSummary
}

type liveChatRunErrorMsg struct {
	err error
}

type liveChatBlocksLoadedMsg struct {
	blocks []smithers.ChatBlock
}

type liveChatBlocksErrorMsg struct {
	err error
}

type liveChatNewBlockMsg struct {
	block smithers.ChatBlock
}

type liveChatHijackSessionMsg struct {
	session *smithers.HijackSession
	err     error
}

type liveChatHijackReturnMsg struct {
	runID string
	err   error
}

// liveChatResumeToAutomationMsg is dispatched when the user confirms that they
// want to restart automation after returning from a hijack session.
type liveChatResumeToAutomationMsg struct {
	runID string
}

// --- LiveChatView ---

// LiveChatView renders a read-only, scrollable stream of agent chat messages
// for a running (or completed) Smithers task.
//
// Navigation:
//   - up / k         scroll up (disables follow mode)
//   - down / j       scroll down
//   - PgUp / PgDn    page scroll
//   - f               toggle follow mode (auto-scroll to bottom)
//   - [               previous attempt
//   - ]               next attempt
//   - h               hijack -- hand off terminal to the agent's native CLI
//   - r               refresh chat transcript from server
//   - a               (after hijack return) resume automation
//   - d               (after hijack return) dismiss banner
//   - q / Esc         pop back to the previous view
type LiveChatView struct {
	client *smithers.Client

	// Identity
	runID     string
	taskID    string // optional: filter display to a single node
	agentName string // display name for the agent field in the header

	// Run metadata (loaded once on init)
	run    *smithers.RunSummary
	runErr error

	// Loading / error state
	loadingRun    bool
	loadingBlocks bool
	blocksErr     error

	// Streaming state
	blockCh     <-chan smithers.ChatBlock
	blockCancel context.CancelFunc
	streamDone  bool

	// All blocks received (all attempts)
	blocks []smithers.ChatBlock

	// Attempt display: maps attempt number (0-based) to its blocks
	attempts       map[int][]smithers.ChatBlock
	currentAttempt int
	maxAttempt     int

	// Badge: new blocks arrived in latest attempt while viewing an older one
	newBlocksInLatest int

	// Hijack state
	hijacking bool
	hijackErr error

	// feat-hijack-seamless-transition: hijackReturned is true after the user
	// exits the native agent TUI. While true the view shows a post-return
	// status banner.
	hijackReturned bool
	// hijackReturnErr holds a non-nil error when the agent process exited
	// with a non-zero status.
	hijackReturnErr error

	// feat-hijack-conversation-replay-fallback: replayFallback is true when
	// the agent does not support --resume and we have injected recent
	// conversation history as a fallback.
	replayFallback bool

	// feat-hijack-resume-to-automation: promptResumeAutomation is true when
	// the post-hijack banner is showing and waiting for the user to press 'a'
	// (resume automation) or 'd' (dismiss).
	promptResumeAutomation bool

	// Viewport
	width      int
	height     int
	scrollLine int  // first visible line index
	follow     bool // auto-scroll to bottom when new blocks arrive

	// Rendered lines cache (rebuilt whenever blocks or attempt selection changes)
	lines      []string
	linesDirty bool

	// Side-by-side context pane (toggled by 's')
	showSidePane bool
	contextPane  *LiveChatContextPane
	splitPane    *components.SplitPane
}

// NewLiveChatView creates a new live chat view for the given run.
// taskID and agentName are optional display hints; pass empty strings to omit.
func NewLiveChatView(client *smithers.Client, runID, taskID, agentName string) *LiveChatView {
	return &LiveChatView{
		client:        client,
		contextPane:   newLiveChatContextPane(runID),
		runID:         runID,
		taskID:        taskID,
		agentName:     agentName,
		loadingRun:    true,
		loadingBlocks: true,
		follow:        true,
		linesDirty:    true,
		attempts:      make(map[int][]smithers.ChatBlock),
	}
}

// Init starts the metadata and chat-history fetches, then opens the SSE stream.
func (v *LiveChatView) Init() tea.Cmd {
	return tea.Batch(
		v.fetchRun(),
		v.fetchBlocks(),
	)
}

// fetchRun returns a Cmd that loads run metadata.
func (v *LiveChatView) fetchRun() tea.Cmd {
	runID := v.runID
	client := v.client
	return func() tea.Msg {
		run, err := client.GetRun(context.Background(), runID)
		if err != nil {
			return liveChatRunErrorMsg{err: err}
		}
		return liveChatRunLoadedMsg{run: run}
	}
}

// fetchBlocks returns a Cmd that loads the current chat transcript.
func (v *LiveChatView) fetchBlocks() tea.Cmd {
	runID := v.runID
	client := v.client
	return func() tea.Msg {
		blocks, err := client.GetChatOutput(context.Background(), runID)
		if err != nil {
			return liveChatBlocksErrorMsg{err: err}
		}
		return liveChatBlocksLoadedMsg{blocks: blocks}
	}
}

// openStreamCmd opens the SSE stream and returns the first WaitForChatBlock cmd.
// It cancels any previously active stream first.
func (v *LiveChatView) openStreamCmd() tea.Cmd {
	// Cancel any prior stream.
	if v.blockCancel != nil {
		v.blockCancel()
		v.blockCancel = nil
		v.blockCh = nil
	}
	v.streamDone = false

	runID := v.runID
	client := v.client

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := client.StreamChat(ctx, runID)
		if err != nil {
			cancel()
			// Non-fatal: streaming unavailable; show static snapshot with error hint.
			return smithers.ChatStreamErrorMsg{RunID: runID, Err: err}
		}
		// Store cancel and channel on the view via a side-channel message.
		return liveChatStreamOpenedMsg{ch: ch, cancel: cancel, runID: runID}
	}
}

// liveChatStreamOpenedMsg is an internal message that delivers the SSE channel
// to the view after the goroutine is spawned.
type liveChatStreamOpenedMsg struct {
	ch     <-chan smithers.ChatBlock
	cancel context.CancelFunc
	runID  string
}

// hijackRunCmd dispatches an HTTP request to pause the agent and returns
// session metadata for native TUI handoff.
func (v *LiveChatView) hijackRunCmd() tea.Cmd {
	runID := v.runID
	client := v.client
	return func() tea.Msg {
		session, err := client.HijackRun(context.Background(), runID)
		return liveChatHijackSessionMsg{session: session, err: err}
	}
}

// indexBlock adds a block to v.attempts and updates maxAttempt.
// It does NOT update currentAttempt -- callers manage that explicitly
// (snapshot loading advances to the latest; streaming may badge instead).
func (v *LiveChatView) indexBlock(block smithers.ChatBlock) {
	attempt := block.Attempt
	v.attempts[attempt] = append(v.attempts[attempt], block)
	if attempt > v.maxAttempt {
		v.maxAttempt = attempt
	}
}

// rebuildAttemptBlocks resets lines from v.attempts[v.currentAttempt].
func (v *LiveChatView) rebuildAttemptBlocks() {
	v.linesDirty = true
	v.newBlocksInLatest = 0
}

// Update handles all messages for the live chat view.
func (v *LiveChatView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {

	// --- Run metadata ---
	case liveChatRunLoadedMsg:
		v.run = msg.run
		v.loadingRun = false
		// Forward run metadata to the context pane so it can update its display.
		if v.contextPane != nil {
			newPane, _ := v.contextPane.Update(msg)
			if cp, ok := newPane.(*LiveChatContextPane); ok {
				v.contextPane = cp
			}
		}
		return v, nil

	case liveChatRunErrorMsg:
		v.runErr = msg.err
		v.loadingRun = false
		return v, nil

	// --- Chat blocks (initial load) ---
	case liveChatBlocksLoadedMsg:
		v.blocks = msg.blocks
		v.loadingBlocks = false
		// Rebuild attempt index from snapshot.
		v.attempts = make(map[int][]smithers.ChatBlock)
		v.maxAttempt = 0
		v.currentAttempt = 0
		for _, b := range v.blocks {
			v.indexBlock(b)
		}
		// Start on the latest attempt.
		v.currentAttempt = v.maxAttempt
		v.linesDirty = true
		if v.follow {
			v.scrollToBottom()
		}
		// Open SSE stream for live updates.
		return v, v.openStreamCmd()

	case liveChatBlocksErrorMsg:
		v.blocksErr = msg.err
		v.loadingBlocks = false
		return v, nil

	// --- SSE stream opened ---
	case liveChatStreamOpenedMsg:
		if msg.runID == v.runID {
			v.blockCh = msg.ch
			v.blockCancel = msg.cancel
			return v, smithers.WaitForChatBlock(v.runID, v.blockCh)
		}
		// Stale message from a previous runID -- cancel the stream.
		msg.cancel()
		return v, nil

	// --- Streaming new block from SSE feed ---
	case smithers.ChatBlockMsg:
		if msg.RunID != v.runID {
			return v, nil
		}
		block := msg.Block
		v.blocks = append(v.blocks, block)

		// If viewing an older attempt and this block belongs to a newer one,
		// increment the "new blocks in latest" badge.
		if block.Attempt > v.currentAttempt {
			v.newBlocksInLatest++
			v.indexBlock(block)
			// Don't rebuild display lines; user is viewing an older attempt.
			return v, smithers.WaitForChatBlock(v.runID, v.blockCh)
		}

		// Block belongs to the current (or earlier) attempt.
		v.indexBlock(block)
		v.linesDirty = true
		if v.follow {
			v.scrollToBottom()
		}
		return v, smithers.WaitForChatBlock(v.runID, v.blockCh)

	// --- Chat stream lifecycle ---
	case smithers.ChatStreamDoneMsg:
		if msg.RunID == v.runID {
			v.streamDone = true
		}
		return v, nil

	case smithers.ChatStreamErrorMsg:
		if msg.RunID == v.runID {
			v.blocksErr = msg.Err
		}
		return v, nil

	// --- liveChatNewBlockMsg: injected by tests or external callers ---
	case liveChatNewBlockMsg:
		v.blocks = append(v.blocks, msg.block)
		v.indexBlock(msg.block)
		v.linesDirty = true
		if v.follow {
			v.scrollToBottom()
		}
		return v, nil

	// --- Hijack flow ---
	case liveChatHijackSessionMsg:
		v.hijacking = false
		if msg.err != nil {
			v.hijackErr = msg.err
			return v, nil
		}
		s := msg.session

		// feat-hijack-conversation-replay-fallback: when the agent does not
		// support --resume we cannot hand off to its native CLI in a meaningful
		// way. Instead inject a contextual notice block so the user can see
		// the conversation history, and set replayFallback so the view renders
		// an explanatory banner.
		if !s.SupportsResume {
			v.replayFallback = true
			notice := fmt.Sprintf(
				"Agent %q (engine: %s) does not support native session resume.\n"+
					"The conversation history is shown below for context.",
				s.AgentBinary, s.AgentEngine,
			)
			v.blocks = append(v.blocks, smithers.ChatBlock{
				RunID:       v.runID,
				Content:     notice,
				TimestampMs: time.Now().UnixMilli(),
			})
			v.linesDirty = true
			return v, nil
		}

		// feat-hijack-native-cli-resume / feat-hijack-multi-engine-support:
		// ResumeArgs() returns engine-specific arguments:
		//   claude-code / claude / amp -> --resume <token>
		//   codex                      -> --session-id <token>
		//   gemini                     -> --session <token>
		if _, lookErr := exec.LookPath(s.AgentBinary); lookErr != nil {
			v.hijackErr = fmt.Errorf("cannot hijack: %s binary not found (%s). Install it or check PATH", s.AgentEngine, s.AgentBinary)
			return v, nil
		}
		cmd := exec.Command(s.AgentBinary, s.ResumeArgs()...) //nolint:gosec
		if s.CWD != "" {
			cmd.Dir = s.CWD
		}
		return v, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return liveChatHijackReturnMsg{runID: v.runID, err: err}
		})

	// feat-hijack-seamless-transition: on return from the native agent TUI,
	// update state for the post-return banner, append a divider, then
	// refresh run state and re-open the stream.
	//
	// feat-hijack-resume-to-automation: also set promptResumeAutomation so
	// the user can press 'a' to restart automation or 'd' to dismiss.
	case liveChatHijackReturnMsg:
		v.hijacking = false
		if msg.runID != v.runID {
			return v, nil
		}
		v.hijackReturned = true
		v.hijackReturnErr = msg.err
		v.promptResumeAutomation = true

		// Append session-ended divider.
		divider := lipgloss.NewStyle().Faint(true).Render("--------- HIJACK SESSION ENDED ---------")
		v.blocks = append(v.blocks, smithers.ChatBlock{
			RunID:       v.runID,
			Content:     divider,
			TimestampMs: time.Now().UnixMilli(),
		})
		v.linesDirty = true
		// feat-hijack-resume-to-automation: refresh run state immediately so
		// the viewer reflects the post-hijack world.
		return v, tea.Batch(v.fetchRun(), v.fetchBlocks())

	// feat-hijack-resume-to-automation: user confirmed restart automation.
	case liveChatResumeToAutomationMsg:
		if msg.runID != v.runID {
			return v, nil
		}
		v.promptResumeAutomation = false
		v.hijackReturned = false
		// Append a resuming-automation notice block.
		notice := "  Resuming automation..."
		v.blocks = append(v.blocks, smithers.ChatBlock{
			RunID:       v.runID,
			Content:     notice,
			TimestampMs: time.Now().UnixMilli(),
		})
		v.linesDirty = true
		return v, nil

	// --- Window resize ---
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.linesDirty = true
		if v.follow {
			v.scrollToBottom()
		}
		return v, nil

	// --- Keyboard ---
	case tea.KeyPressMsg:
		// feat-hijack-resume-to-automation: handle post-hijack automation prompt.
		if v.promptResumeAutomation {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
				runID := v.runID
				return v, func() tea.Msg {
					return liveChatResumeToAutomationMsg{runID: runID}
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("d", "esc"))):
				v.promptResumeAutomation = false
				v.hijackReturned = false
				return v, nil
			}
			// While the prompt is showing, suppress other keys.
			return v, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc", "alt+esc"))):
			// Cancel SSE stream on exit.
			if v.blockCancel != nil {
				v.blockCancel()
			}
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
			v.follow = !v.follow
			if v.follow {
				v.scrollToBottom()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			if !v.hijacking {
				v.hijacking = true
				v.hijackErr = nil
				v.replayFallback = false
				return v, v.hijackRunCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("["))):
			// Navigate to previous attempt.
			if v.currentAttempt > 0 {
				v.currentAttempt--
				v.rebuildAttemptBlocks()
				v.scrollToBottom()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("]"))):
			// Navigate to next attempt.
			if v.currentAttempt < v.maxAttempt {
				v.currentAttempt++
				v.rebuildAttemptBlocks()
				v.scrollToBottom()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			v.follow = false
			if v.scrollLine > 0 {
				v.scrollLine--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			lines := v.renderedLines()
			visibleHeight := v.visibleHeight()
			maxScroll := len(lines) - visibleHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if v.scrollLine < maxScroll {
				v.scrollLine++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("pgup"))):
			v.follow = false
			step := v.visibleHeight() - 1
			if step < 1 {
				step = 1
			}
			v.scrollLine -= step
			if v.scrollLine < 0 {
				v.scrollLine = 0
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("pgdown"))):
			lines := v.renderedLines()
			visibleHeight := v.visibleHeight()
			step := visibleHeight - 1
			if step < 1 {
				step = 1
			}
			maxScroll := len(lines) - visibleHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			v.scrollLine += step
			if v.scrollLine > maxScroll {
				v.scrollLine = maxScroll
				v.follow = true
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			// Cancel and re-open stream, reload transcript.
			if v.blockCancel != nil {
				v.blockCancel()
				v.blockCancel = nil
				v.blockCh = nil
			}
			v.loadingBlocks = true
			return v, v.fetchBlocks()

		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			// feat-live-chat-side-by-side: toggle context pane.
			v.showSidePane = !v.showSidePane
			if v.showSidePane {
				v.splitPane = v.buildSplitPane()
				v.splitPane.SetSize(v.width, v.height)
			} else {
				v.splitPane = nil
			}
			v.linesDirty = true
		}
	}

	// Keep the context pane in sync with all messages (run metadata, etc.).
	if v.contextPane != nil {
		newCtx, _ := v.contextPane.Update(msg)
		if cp, ok := newCtx.(*LiveChatContextPane); ok {
			v.contextPane = cp
		}
	}
	// Keep the split pane in sync when visible.
	if v.splitPane != nil {
		newSP, _ := v.splitPane.Update(msg)
		v.splitPane = newSP
	}

	return v, nil
}

// View renders the full live chat view as a string.
func (v *LiveChatView) View() string {
	var b strings.Builder

	b.WriteString(v.renderHeader())
	b.WriteString("\n")
	b.WriteString(v.renderSubHeader())
	b.WriteString("\n")
	b.WriteString(v.renderDivider())
	b.WriteString("\n")

	// feat-hijack-seamless-transition: show transition banner while waiting
	// for the hijack API call to complete.
	if v.hijacking {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Hijacking session..."))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Pausing the agent and handing off the terminal."))
		b.WriteString("\n")
		return b.String()
	}

	if v.hijackErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(
			fmt.Sprintf("  Hijack error: %v", v.hijackErr)))
		b.WriteString("\n")
	}

	// feat-hijack-conversation-replay-fallback: show banner when agent lacks resume support.
	if v.replayFallback {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render(
			"  Resume not supported - conversation history shown below."))
		b.WriteString("\n")
	}

	// feat-hijack-seamless-transition / feat-hijack-resume-to-automation:
	// show post-hijack banner with automation prompt.
	if v.promptResumeAutomation {
		b.WriteString(v.renderResumeToAutomationBanner())
		b.WriteString(v.renderBody())
		return b.String()
	}
	if v.hijackReturned {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(
			"  Returned from hijack session."))
		b.WriteString("\n")
	}

	if v.showSidePane && v.splitPane != nil {
		b.WriteString(v.splitPane.View())
	} else {
		b.WriteString(v.renderBody())
	}
	b.WriteString(v.renderHelpBar())

	return b.String()
}

// renderResumeToAutomationBanner renders the post-hijack automation prompt.
// feat-hijack-resume-to-automation
func (v *LiveChatView) renderResumeToAutomationBanner() string {
	var b strings.Builder
	errNote := ""
	if v.hijackReturnErr != nil {
		errNote = fmt.Sprintf(" (agent exited with error: %v)", v.hijackReturnErr)
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render(
		fmt.Sprintf("  Hijack session ended%s.", errNote)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"  [a] Resume automation   [d / Esc] Dismiss"))
	b.WriteString("\n")
	return b.String()
}

// Name returns the view name for the router.
func (v *LiveChatView) Name() string {
	return "livechat"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *LiveChatView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.linesDirty = true
	if v.splitPane != nil {
		v.splitPane.SetSize(width, height)
	}
}

// ShortHelp returns keybinding hints shown in the help bar.
func (v *LiveChatView) ShortHelp() []key.Binding {
	// feat-hijack-resume-to-automation: post-hijack prompt overrides normal help.
	if v.promptResumeAutomation {
		return []key.Binding{
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "resume automation")),
			key.NewBinding(key.WithKeys("d", "esc"), key.WithHelp("d/Esc", "dismiss")),
		}
	}

	followDesc := "follow: on"
	if !v.follow {
		followDesc = "follow: off"
	}
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("up/down", "scroll")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", followDesc)),
		key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hijack")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
	// Only show attempt nav hints when there are multiple attempts.
	if v.maxAttempt > 0 {
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("[", "]"), key.WithHelp("[/]", "attempt")),
		)
	}
	contextDesc := "context: off"
	if v.showSidePane {
		contextDesc = "context: on"
	}
	bindings = append(bindings,
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", contextDesc)),
		key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	)
	return bindings
}

// --- Rendering helpers ---

func (v *LiveChatView) renderHeader() string {
	runPart := v.runID
	if len(runPart) > 8 {
		runPart = runPart[:8]
	}

	viewName := "Chat › " + runPart
	if v.run != nil && v.run.WorkflowName != "" {
		viewName += " (" + v.run.WorkflowName + ")"
	}

	return ViewHeader(packageCom.Styles, "CODEPLANE", viewName, v.width, "[Esc] Back")
}

func (v *LiveChatView) renderSubHeader() string {
	faint := lipgloss.NewStyle().Faint(true)
	parts := []string{}

	agent := v.agentName
	if agent == "" && v.run != nil {
		agent = "agent"
	} else if agent == "" {
		agent = "agent"
	}
	parts = append(parts, "Agent: "+agent)

	if v.taskID != "" {
		parts = append(parts, "Node: "+v.taskID)
	}

	// Attempt indicator -- only shown when there are multiple attempts.
	if v.maxAttempt > 0 {
		parts = append(parts, fmt.Sprintf("Attempt: %d of %d", v.currentAttempt+1, v.maxAttempt+1))
	}

	if v.run != nil && v.run.StartedAtMs != nil {
		elapsed := time.Since(time.UnixMilli(*v.run.StartedAtMs)).Round(time.Second)
		parts = append(parts, "elapsed: "+elapsed.String())
	}

	// Follow-mode indicator
	if v.follow {
		parts = append(parts, "LIVE")
	}

	// New-blocks badge when viewing a past attempt.
	if v.newBlocksInLatest > 0 && v.currentAttempt < v.maxAttempt {
		badge := fmt.Sprintf("(%d new in latest attempt)", v.newBlocksInLatest)
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(badge))
	}

	return faint.Render(strings.Join(parts, " | "))
}

func (v *LiveChatView) renderDivider() string {
	w := v.width
	if w <= 0 {
		w = 40
	}
	return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("-", w))
}

// renderHelpBar returns a one-line help string built from ShortHelp bindings.
func (v *LiveChatView) renderHelpBar() string {
	var parts []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", h.Key, h.Desc))
		}
	}
	return lipgloss.NewStyle().Faint(true).Render("  "+strings.Join(parts, "  ")) + "\n"
}

func (v *LiveChatView) renderBody() string {
	// Loading / error states
	if v.loadingRun && v.loadingBlocks {
		return "  Loading...\n"
	}
	if v.runErr != nil {
		return fmt.Sprintf("  Error loading run: %v\n", v.runErr)
	}
	if v.blocksErr != nil {
		unavailNote := ""
		if v.blocksErr.Error() == smithers.ErrServerUnavailable.Error() ||
			strings.Contains(v.blocksErr.Error(), "unavailable") {
			unavailNote = " (live streaming unavailable - showing static snapshot)"
		}
		return fmt.Sprintf("  Error loading chat: %v%s\n", v.blocksErr, unavailNote)
	}
	if v.loadingBlocks {
		return "  Loading chat...\n"
	}

	// Determine which blocks to render: current attempt or all if no attempts indexed.
	displayBlocks := v.displayBlocks()
	if len(displayBlocks) == 0 {
		return "  No messages yet.\n"
	}

	lines := v.renderedLines()
	visible := v.visibleHeight()
	if visible <= 0 {
		visible = 20
	}

	// Clamp scroll
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if v.scrollLine > maxScroll {
		v.scrollLine = maxScroll
	}
	if v.scrollLine < 0 {
		v.scrollLine = 0
	}

	end := v.scrollLine + visible
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	for _, line := range lines[v.scrollLine:end] {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Streaming indicator
	if v.run != nil && !v.run.Status.IsTerminal() && !v.streamDone {
		sb.WriteString(lipgloss.NewStyle().Faint(true).Render("  (streaming...)"))
		sb.WriteString("\n")
	}

	return sb.String()
}

// displayBlocks returns the blocks that should be rendered in the current view.
// When attempts are indexed it returns the current attempt's blocks; otherwise
// it returns all blocks (pre-index state during initial load).
func (v *LiveChatView) displayBlocks() []smithers.ChatBlock {
	if len(v.attempts) == 0 {
		return v.blocks
	}
	return v.attempts[v.currentAttempt]
}

// renderedLines returns (and caches) the wrapped line buffer for the current display blocks.
func (v *LiveChatView) renderedLines() []string {
	if !v.linesDirty {
		return v.lines
	}

	contentWidth := v.width - 4 // leave room for "  | " prefix
	if contentWidth < 20 {
		contentWidth = 20
	}

	var out []string

	var runStartMs int64
	if v.run != nil && v.run.StartedAtMs != nil {
		runStartMs = *v.run.StartedAtMs
	}

	roleStyle := lipgloss.NewStyle().Bold(true)
	tsStyle := lipgloss.NewStyle().Faint(true)
	barStyle := lipgloss.NewStyle().Faint(true)
	bar := barStyle.Render("  | ")

	displayBlocks := v.displayBlocks()
	for i, block := range displayBlocks {
		// Blank separator between blocks (skip before first)
		if i > 0 {
			out = append(out, "")
		}

		// Timestamp relative to run start
		var tsLabel string
		if runStartMs > 0 {
			elapsed := time.Duration(block.TimestampMs-runStartMs) * time.Millisecond
			if elapsed < 0 {
				elapsed = 0
			}
			tsLabel = fmt.Sprintf("[%s]", fmtDuration(elapsed))
		} else if block.TimestampMs > 0 {
			tsLabel = fmt.Sprintf("[%s]",
				time.UnixMilli(block.TimestampMs).Format("15:04:05"))
		}

		// For divider blocks (empty role, used by hijack divider), render inline.
		if block.Role == "" && block.Content != "" {
			if tsLabel != "" {
				out = append(out, tsStyle.Render(tsLabel)+" "+block.Content)
			} else {
				out = append(out, block.Content)
			}
			continue
		}

		roleLabel := strings.Title(string(block.Role)) //nolint:staticcheck // acceptable for display
		header := tsStyle.Render(tsLabel) + " " + roleStyle.Render(roleLabel)
		if block.NodeID != "" && block.NodeID != v.taskID {
			header += lipgloss.NewStyle().Faint(true).Render(" . " + block.NodeID)
		}
		out = append(out, header)

		// Render tool_call and tool_result blocks with a distinct prefix.
		switch block.Role {
		case smithers.ChatRoleTool:
			for _, toolLine := range renderToolBlock(block.Content, contentWidth) {
				out = append(out, bar+toolLine)
			}
		default:
			// Body lines with word-wrap.
			for _, rawLine := range strings.Split(block.Content, "\n") {
				for len(rawLine) > contentWidth {
					out = append(out, bar+rawLine[:contentWidth])
					rawLine = rawLine[contentWidth:]
				}
				out = append(out, bar+rawLine)
			}
		}
	}

	v.lines = out
	v.linesDirty = false
	return v.lines
}

// scrollToBottom sets scrollLine so the last lines are visible.
func (v *LiveChatView) scrollToBottom() {
	lines := v.renderedLines()
	visible := v.visibleHeight()
	if visible <= 0 {
		visible = 20
	}
	v.scrollLine = len(lines) - visible
	if v.scrollLine < 0 {
		v.scrollLine = 0
	}
}

// visibleHeight returns the number of body lines that fit in the terminal.
// Reserves lines for: header (1) + sub-header (1) + divider (1) + help bar (1) + streaming (1).
func (v *LiveChatView) visibleHeight() int {
	reserved := 6
	h := v.height - reserved
	if h < 4 {
		return 4
	}
	return h
}

// fmtDuration formats a duration as mm:ss for the timestamp column.
func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// --- Side-by-side layout (feat-live-chat-side-by-side) ---

// buildSplitPane constructs a SplitPane with the context pane on the left and
// a chat-body proxy pane on the right.
func (v *LiveChatView) buildSplitPane() *components.SplitPane {
	right := &liveChatBodyPane{view: v}
	return components.NewSplitPane(v.contextPane, right, components.SplitPaneOpts{
		LeftWidth:         32,
		CompactBreakpoint: 100,
	})
}

// liveChatBodyPane adapts LiveChatView's body rendering to the components.Pane
// interface so it can be used as the right panel in a SplitPane.
type liveChatBodyPane struct {
	view *LiveChatView
}

func (p *liveChatBodyPane) Init() tea.Cmd                                 { return nil }
func (p *liveChatBodyPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) { return p, nil }
func (p *liveChatBodyPane) View() string                                  { return p.view.renderBody() }
func (p *liveChatBodyPane) SetSize(width, height int) {
	// The body pane shares the parent view's rendering; dimensions are
	// governed by the parent LiveChatView.
	_ = width
	_ = height
}

// --- Tool-call rendering (feat-live-chat-tool-call-rendering) ---

// toolCallJSON is the structured JSON shape that Smithers emits for tool blocks.
type toolCallJSON struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"`
	Error  string          `json:"error"`
}

// renderToolBlock renders a tool-call content string into display lines.
// If the content is valid toolCallJSON it produces a structured layout:
//
//	⚙ tool_name
//	in: {"key":"val"}
//	out: result text (up to 3 lines, then "…")
//
// For non-JSON content it falls back to a raw "⚙ <content>" layout.
// All lines are wrapped to contentWidth runes.
func renderToolBlock(content string, contentWidth int) []string {
	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	faintStyle := lipgloss.NewStyle().Faint(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	var tc toolCallJSON
	if err := json.Unmarshal([]byte(content), &tc); err != nil || tc.Name == "" {
		// Fallback: raw display with ⚙ prefix.
		prefix := toolStyle.Render("⚙ ")
		var lines []string
		for _, rawLine := range strings.Split(prefix+content, "\n") {
			for _, wrapped := range wrapLineToWidth(rawLine, contentWidth) {
				lines = append(lines, wrapped)
			}
		}
		return lines
	}

	var lines []string

	// Header: ⚙ tool_name
	lines = append(lines, toolStyle.Render("⚙ "+tc.Name))

	// Input line (compact JSON)
	if len(tc.Input) > 0 && string(tc.Input) != "null" {
		inputCompact := string(tc.Input)
		inLine := faintStyle.Render("in: ") + inputCompact
		for _, wrapped := range wrapLineToWidth(inLine, contentWidth) {
			lines = append(lines, wrapped)
		}
	}

	// Output or error lines
	if tc.Error != "" {
		errLine := errStyle.Render("err: ") + tc.Error
		for _, wrapped := range wrapLineToWidth(errLine, contentWidth) {
			lines = append(lines, wrapped)
		}
	} else if tc.Output != "" {
		outLines := strings.Split(tc.Output, "\n")
		const maxOutLines = 3
		shown := outLines
		truncated := false
		if len(outLines) > maxOutLines {
			shown = outLines[:maxOutLines]
			truncated = true
		}
		for i, ol := range shown {
			prefix := ""
			if i == 0 {
				prefix = faintStyle.Render("out: ")
			} else {
				prefix = "     "
			}
			for _, wrapped := range wrapLineToWidth(prefix+ol, contentWidth) {
				lines = append(lines, wrapped)
			}
		}
		if truncated {
			lines = append(lines, faintStyle.Render("     …"))
		}
	}

	return lines
}
