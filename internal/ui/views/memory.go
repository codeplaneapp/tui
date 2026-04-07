package views

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*MemoryView)(nil)

type memoryLoadedMsg struct {
	facts []smithers.MemoryFact
}

type memoryErrorMsg struct {
	err error
}

type memoryRecallResultMsg struct {
	results []smithers.MemoryRecallResult
}

type memoryRecallErrorMsg struct {
	err error
}

// memoryViewMode represents the current display mode.
type memoryViewMode int

const (
	memoryModeList    memoryViewMode = iota // Default: navigable list
	memoryModeDetail                        // Detail pane for selected fact
	memoryModeRecall                        // Semantic recall prompt
	memoryModeResults                       // Recall results display
)

// MemoryView displays a navigable list of memory facts across all namespaces.
type MemoryView struct {
	client  *smithers.Client
	facts   []smithers.MemoryFact
	cursor  int
	width   int
	height  int
	loading bool
	err     error

	// Detail / recall mode.
	mode          memoryViewMode
	recallQuery   string // Accumulated input while typing recall query.
	recallResults []smithers.MemoryRecallResult
	recallErr     error
	recallLoading bool

	// Namespace filtering.
	namespaces      []string // Sorted unique namespaces.
	activeNamespace string   // "" means all namespaces.
	nsFilterCursor  int      // Cursor for namespace selector (when filtering).
}

// NewMemoryView creates a new memory browser view.
func NewMemoryView(client *smithers.Client) *MemoryView {
	return &MemoryView{
		client:  client,
		loading: true,
	}
}

// Init loads memory facts from the client.
func (v *MemoryView) Init() tea.Cmd {
	return func() tea.Msg {
		facts, err := v.client.ListAllMemoryFacts(context.Background())
		if err != nil {
			return memoryErrorMsg{err: err}
		}
		return memoryLoadedMsg{facts: facts}
	}
}

// filteredFacts returns facts matching the active namespace filter (all if empty).
func (v *MemoryView) filteredFacts() []smithers.MemoryFact {
	if v.activeNamespace == "" {
		return v.facts
	}
	var out []smithers.MemoryFact
	for _, f := range v.facts {
		if f.Namespace == v.activeNamespace {
			out = append(out, f)
		}
	}
	return out
}

// extractNamespaces returns a sorted, deduplicated list of namespaces from v.facts.
func extractNamespaces(facts []smithers.MemoryFact) []string {
	seen := make(map[string]struct{})
	for _, f := range facts {
		seen[f.Namespace] = struct{}{}
	}
	ns := make([]string, 0, len(seen))
	for k := range seen {
		ns = append(ns, k)
	}
	sort.Strings(ns)
	return ns
}

// Update handles messages for the memory browser view.
func (v *MemoryView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case memoryLoadedMsg:
		v.facts = msg.facts
		v.namespaces = extractNamespaces(v.facts)
		v.loading = false
		return v, nil

	case memoryErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case memoryRecallResultMsg:
		v.recallResults = msg.results
		v.recallLoading = false
		v.mode = memoryModeResults
		return v, nil

	case memoryRecallErrorMsg:
		v.recallErr = msg.err
		v.recallLoading = false
		v.mode = memoryModeResults
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

// handleKey dispatches key events depending on the current view mode.
func (v *MemoryView) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch v.mode {
	case memoryModeRecall:
		return v.handleRecallInput(msg)
	case memoryModeDetail:
		return v.handleDetailKey(msg)
	case memoryModeResults:
		return v.handleResultsKey(msg)
	default:
		return v.handleListKey(msg)
	}
}

// handleListKey handles keys in the default list mode.
func (v *MemoryView) handleListKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
		return v, func() tea.Msg { return PopViewMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		facts := v.filteredFacts()
		if v.cursor > 0 {
			v.cursor--
		}
		_ = facts
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		facts := v.filteredFacts()
		if v.cursor < len(facts)-1 {
			v.cursor++
		}
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		v.loading = true
		v.err = nil
		v.activeNamespace = ""
		v.cursor = 0
		return v, v.Init()

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		facts := v.filteredFacts()
		if len(facts) > 0 && v.cursor < len(facts) {
			v.mode = memoryModeDetail
		}
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		v.mode = memoryModeRecall
		v.recallQuery = ""
		v.recallErr = nil
		v.recallResults = nil
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
		// Cycle through namespace filters: "" → ns[0] → ns[1] → ... → "".
		v.cursor = 0
		if v.activeNamespace == "" {
			if len(v.namespaces) > 0 {
				v.activeNamespace = v.namespaces[0]
				v.nsFilterCursor = 0
			}
		} else {
			// Find current index and advance.
			found := false
			for i, ns := range v.namespaces {
				if ns == v.activeNamespace {
					next := i + 1
					if next >= len(v.namespaces) {
						v.activeNamespace = "" // wrap back to all
					} else {
						v.activeNamespace = v.namespaces[next]
						v.nsFilterCursor = next
					}
					found = true
					break
				}
			}
			if !found {
				v.activeNamespace = ""
			}
		}
		return v, nil
	}
	return v, nil
}

// handleDetailKey handles keys while viewing a fact's detail pane.
func (v *MemoryView) handleDetailKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
		v.mode = memoryModeList
	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		v.mode = memoryModeRecall
		v.recallQuery = ""
		v.recallErr = nil
		v.recallResults = nil
	}
	return v, nil
}

// handleResultsKey handles keys on the recall results screen.
func (v *MemoryView) handleResultsKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
		v.mode = memoryModeList
	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		v.mode = memoryModeRecall
		v.recallQuery = ""
		v.recallErr = nil
		v.recallResults = nil
	}
	return v, nil
}

// handleRecallInput handles key presses while the recall query prompt is active.
func (v *MemoryView) handleRecallInput(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		v.mode = memoryModeList
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if strings.TrimSpace(v.recallQuery) == "" {
			v.mode = memoryModeList
			return v, nil
		}
		v.recallLoading = true
		v.recallErr = nil
		v.recallResults = nil
		query := v.recallQuery
		var ns *string
		if v.activeNamespace != "" {
			nsCopy := v.activeNamespace
			ns = &nsCopy
		}
		client := v.client
		return v, func() tea.Msg {
			results, err := client.RecallMemory(context.Background(), query, ns, 10)
			if err != nil {
				return memoryRecallErrorMsg{err: err}
			}
			return memoryRecallResultMsg{results: results}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if len(v.recallQuery) > 0 {
			runes := []rune(v.recallQuery)
			v.recallQuery = string(runes[:len(runes)-1])
		}
		return v, nil

	default:
		// Append printable character to the query buffer.
		if ch := msg.String(); ch != "" && !strings.HasPrefix(ch, "ctrl+") && !strings.HasPrefix(ch, "alt+") {
			v.recallQuery += ch
		}
		return v, nil
	}
}

// View renders the memory fact list.
func (v *MemoryView) View() string {
	switch v.mode {
	case memoryModeDetail:
		return v.renderDetail()
	case memoryModeRecall:
		return v.renderRecallPrompt()
	case memoryModeResults:
		return v.renderRecallResults()
	default:
		return v.renderList()
	}
}

// renderList renders the default navigable fact list.
func (v *MemoryView) renderList() string {
	var b strings.Builder

	// Header line with right-aligned [Esc] Back hint.
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS \u203a Memory")
	nsTag := ""
	if v.activeNamespace != "" {
		nsTag = lipgloss.NewStyle().Faint(true).Render(" [" + v.activeNamespace + "]")
	}
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLeft := header + nsTag
	headerLine := headerLeft
	if v.width > 0 {
		gap := v.width - lipgloss.Width(headerLeft) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = headerLeft + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine)
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading memory facts...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	facts := v.filteredFacts()
	if len(facts) == 0 {
		if v.activeNamespace != "" {
			b.WriteString("  No memory facts in namespace: " + v.activeNamespace + "\n")
		} else {
			b.WriteString("  No memory facts found.\n")
		}
		return b.String()
	}

	faint := lipgloss.NewStyle().Faint(true)

	for i, fact := range facts {
		cursor := "  "
		nsStyle := faint
		keyStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "\u25b8 "
			keyStyle = keyStyle.Bold(true)
		}

		// Line 1: [cursor] [namespace] / [key]
		b.WriteString(cursor + nsStyle.Render(fact.Namespace+" / ") + keyStyle.Render(fact.Key) + "\n")

		// Line 2: truncated value preview + relative age.
		preview := factValuePreview(fact.ValueJSON, 60)
		age := factAge(fact.UpdatedAtMs)

		previewStr := "    " + faint.Render(preview)
		ageStr := faint.Render(age)

		if v.width > 0 {
			// Right-align age within available width.
			previewVisualLen := lipgloss.Width(previewStr)
			ageVisualLen := lipgloss.Width(ageStr)
			gap := v.width - previewVisualLen - ageVisualLen - 2
			if gap > 0 {
				b.WriteString(previewStr + strings.Repeat(" ", gap) + ageStr + "\n")
			} else {
				b.WriteString(previewStr + "  " + ageStr + "\n")
			}
		} else {
			b.WriteString(previewStr + "  " + ageStr + "\n")
		}

		if i < len(facts)-1 {
			b.WriteString("\n")
		}
	}

	// Footer help bar.
	b.WriteString("\n")
	footerStyle := lipgloss.NewStyle().Faint(true)
	var hints []string
	for _, binding := range v.ShortHelp() {
		h := binding.Help()
		if h.Key != "" && h.Desc != "" {
			hints = append(hints, "["+h.Key+"] "+h.Desc)
		}
	}
	b.WriteString(footerStyle.Render(strings.Join(hints, "  ")))
	b.WriteString("\n")

	return b.String()
}

// renderDetail renders the full-value detail pane for the selected fact.
func (v *MemoryView) renderDetail() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	facts := v.filteredFacts()
	if v.cursor >= len(facts) {
		b.WriteString("  No fact selected.\n")
		return b.String()
	}
	fact := facts[v.cursor]

	header := bold.Render("SMITHERS \u203a Memory \u203a Detail")
	helpHint := faint.Render("[Esc/q] Back  [s] Recall")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine + "\n\n")

	// Namespace / key row.
	b.WriteString(faint.Render("Namespace: ") + fact.Namespace + "\n")
	b.WriteString(faint.Render("Key:       ") + bold.Render(fact.Key) + "\n")

	// Timestamps.
	if fact.UpdatedAtMs > 0 {
		ts := time.UnixMilli(fact.UpdatedAtMs).Format("2006-01-02 15:04:05")
		b.WriteString(faint.Render("Updated:   ") + ts + "  " + faint.Render("("+factAge(fact.UpdatedAtMs)+")") + "\n")
	}
	if fact.CreatedAtMs > 0 {
		ts := time.UnixMilli(fact.CreatedAtMs).Format("2006-01-02 15:04:05")
		b.WriteString(faint.Render("Created:   ") + ts + "\n")
	}
	if fact.TTLMs != nil {
		b.WriteString(faint.Render("TTL:       ") + fmt.Sprintf("%dms", *fact.TTLMs) + "\n")
	}

	divWidth := 40
	if v.width > 2 {
		divWidth = v.width - 2
	}
	b.WriteString("\n" + faint.Render(strings.Repeat("─", divWidth)) + "\n")

	// Pretty-print value; fall back to raw.
	b.WriteString(bold.Render("Value:") + "\n\n")
	b.WriteString(formatFactValue(fact.ValueJSON, v.width) + "\n")

	return b.String()
}

// renderRecallPrompt renders the inline query input for semantic recall.
func (v *MemoryView) renderRecallPrompt() string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)

	b.WriteString(ViewHeader(packageCom.Styles, "SMITHERS", "Memory › Recall", v.width, "[Enter] Search  [Esc] Cancel") + "\n\n")

	nsLabel := "all namespaces"
	if v.activeNamespace != "" {
		nsLabel = v.activeNamespace
	}
	b.WriteString(faint.Render("Semantic recall in: ") + nsLabel + "\n\n")
	b.WriteString("  Query: " + v.recallQuery + "█\n")

	return b.String()
}

// renderRecallResults renders semantic recall results.
func (v *MemoryView) renderRecallResults() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	header := bold.Render("SMITHERS \u203a Memory \u203a Recall Results")
	helpHint := faint.Render("[Esc/q] Back  [s] New query")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine + "\n\n")

	b.WriteString(faint.Render("Query: ") + v.recallQuery + "\n\n")

	if v.recallLoading {
		b.WriteString("  Searching...\n")
		return b.String()
	}

	if v.recallErr != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.recallErr))
		return b.String()
	}

	if len(v.recallResults) == 0 {
		b.WriteString("  No results found.\n")
		return b.String()
	}

	divWidth := 40
	if v.width > 2 {
		divWidth = v.width - 2
	}

	for i, r := range v.recallResults {
		scoreStr := fmt.Sprintf("%.3f", r.Score)
		b.WriteString(fmt.Sprintf("  %d. ", i+1) + bold.Render(scoreStr) + "\n")
		b.WriteString(faint.Render(strings.Repeat("─", divWidth)) + "\n")
		b.WriteString(wrapText(r.Content, v.width-4) + "\n\n")
	}

	return b.String()
}

// Name returns the view name.
func (v *MemoryView) Name() string {
	return "memory"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *MemoryView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *MemoryView) ShortHelp() []key.Binding {
	switch v.mode {
	case memoryModeDetail:
		return []key.Binding{
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "recall")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc/q", "back")),
		}
	case memoryModeRecall:
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	case memoryModeResults:
		return []key.Binding{
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "new query")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc/q", "back")),
		}
	default:
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "select")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "recall")),
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "filter ns")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
}

// --- Helpers ---

// factValuePreview returns a display-friendly preview of a JSON value string.
// If the value is a JSON string literal (begins and ends with '"'), the outer
// quotes are stripped for readability. The result is truncated to maxLen runes.
func factValuePreview(valueJSON string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 60
	}
	s := strings.TrimSpace(valueJSON)
	if s == "" {
		return ""
	}

	// Strip outer quotes from JSON string literals.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// formatFactValue pretty-prints a JSON value for the detail pane.
// Falls back to indented raw text for non-JSON values.
func formatFactValue(valueJSON string, width int) string {
	s := strings.TrimSpace(valueJSON)
	if s == "" {
		return "  (empty)"
	}
	// Attempt to pretty-print JSON.
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		pretty, err := json.MarshalIndent(v, "  ", "  ")
		if err == nil {
			return "  " + string(pretty)
		}
	}
	// Not valid JSON (or marshal failed); indent raw.
	return wrapText(s, width)
}

// factAge returns a human-readable relative age string for a Unix millisecond timestamp.
// Examples: "45s ago", "3m ago", "2h ago", "5d ago".
func factAge(updatedAtMs int64) string {
	if updatedAtMs <= 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(updatedAtMs))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
