package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*ScoresView)(nil)

type scoresLoadedMsg struct {
	scores []smithers.ScoreRow
	agg    []smithers.AggregateScore
}

type scoresErrorMsg struct {
	err error
}

type scoresMetricsLoadedMsg struct {
	tokens  *smithers.TokenMetrics
	latency *smithers.LatencyMetrics
	cost    *smithers.CostReport
}

type scoresMetricsErrorMsg struct {
	err error
}

// scoresTab represents the active display tab.
type scoresTab int

const (
	scoresTabSummary scoresTab = iota // Aggregated Today's Summary + Scorer table
	scoresTabDetail                   // Token usage, latency, and cost details
)

// ScoresView renders the Scores / ROI dashboard (Design §3.16).
// Three sections: Today's Summary, Scorer Summary table, Recent Evaluations.
type ScoresView struct {
	client  *smithers.Client
	scores  []smithers.ScoreRow
	agg     []smithers.AggregateScore
	width   int
	height  int
	loading bool
	err     error

	// Metrics tab fields.
	activeTab      scoresTab
	metricsLoading bool
	metricsErr     error
	tokenMetrics   *smithers.TokenMetrics
	latencyMetrics *smithers.LatencyMetrics
	costReport     *smithers.CostReport
}

// NewScoresView creates a new scores view.
func NewScoresView(client *smithers.Client) *ScoresView {
	return &ScoresView{
		client:  client,
		loading: true,
	}
}

// Init loads scores from the client asynchronously.
func (v *ScoresView) Init() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		scores, err := v.client.ListRecentScores(ctx, 100)
		if err != nil {
			return scoresErrorMsg{err: err}
		}
		agg, err := v.client.AggregateAllScores(ctx, 100)
		if err != nil {
			return scoresErrorMsg{err: err}
		}
		return scoresLoadedMsg{scores: scores, agg: agg}
	}
}

// initMetrics loads token, latency, and cost metrics asynchronously.
func (v *ScoresView) initMetrics() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		filters := smithers.MetricsFilter{}

		tokens, err := v.client.GetTokenUsageMetrics(ctx, filters)
		if err != nil {
			return scoresMetricsErrorMsg{err: err}
		}
		latency, err := v.client.GetLatencyMetrics(ctx, filters)
		if err != nil {
			return scoresMetricsErrorMsg{err: err}
		}
		cost, err := v.client.GetCostTracking(ctx, filters)
		if err != nil {
			return scoresMetricsErrorMsg{err: err}
		}
		return scoresMetricsLoadedMsg{tokens: tokens, latency: latency, cost: cost}
	}
}

// Update handles messages for the scores view.
func (v *ScoresView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case scoresLoadedMsg:
		v.scores = msg.scores
		v.agg = msg.agg
		v.loading = false
		return v, nil

	case scoresErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case scoresMetricsLoadedMsg:
		v.tokenMetrics = msg.tokens
		v.latencyMetrics = msg.latency
		v.costReport = msg.cost
		v.metricsLoading = false
		return v, nil

	case scoresMetricsErrorMsg:
		v.metricsErr = msg.err
		v.metricsLoading = false
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			v.err = nil
			v.metricsErr = nil
			if v.activeTab == scoresTabDetail {
				v.metricsLoading = true
				return v, tea.Batch(v.Init(), v.initMetrics())
			}
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			if v.activeTab == scoresTabSummary {
				v.activeTab = scoresTabDetail
				// Lazily load metrics on first switch to detail tab.
				if v.tokenMetrics == nil && v.latencyMetrics == nil && v.costReport == nil && !v.metricsLoading {
					v.metricsLoading = true
					return v, v.initMetrics()
				}
			} else {
				v.activeTab = scoresTabSummary
			}
			return v, nil
		}
	}
	return v, nil
}

// View renders the scores dashboard.
func (v *ScoresView) View() string {
	var b strings.Builder

	// Header line: "SMITHERS › Scores" left, "[Esc] Back" right.
	tabLabel := "[Summary]"
	if v.activeTab == scoresTabDetail {
		tabLabel = "[Details]"
	}
	viewName := "Scores " + tabLabel
	b.WriteString(ViewHeader(packageCom.Styles, "SMITHERS", viewName, v.width, "[Tab] Switch  [Esc] Back") + "\n\n")

	if v.loading {
		b.WriteString("  Loading scores...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}
	if len(v.scores) == 0 && len(v.agg) == 0 {
		b.WriteString("  No score data available.\n")
		return b.String()
	}

	switch v.activeTab {
	case scoresTabDetail:
		b.WriteString(v.renderMetricsDetail())
	default:
		b.WriteString(v.renderSummary())
		b.WriteString("\n")
		b.WriteString(v.renderScorerTable())
		b.WriteString("\n")
		b.WriteString(v.renderRecentScores())
	}

	// Footer help hints.
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

// Name returns the view name.
func (v *ScoresView) Name() string { return "scores" }

// SetSize stores the terminal dimensions for use during rendering.
func (v *ScoresView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *ScoresView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch view")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

// renderSummary renders Section 1: Today's Summary.
func (v *ScoresView) renderSummary() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	b.WriteString(bold.Render("Today's Summary") + "\n")
	divWidth := 40
	if v.width > 2 {
		divWidth = v.width - 2
	}
	b.WriteString(faint.Render(strings.Repeat("─", divWidth)) + "\n")

	// Total evaluations and mean score from loaded data.
	total := len(v.scores)
	mean := 0.0
	if total > 0 {
		sum := 0.0
		for _, s := range v.scores {
			sum += s.Score
		}
		mean = sum / float64(total)
	}

	b.WriteString(fmt.Sprintf("  Evaluations: %d   Mean score: %.2f\n", total, mean))

	// Token and cost summary — populated if metrics are available, else placeholder.
	tokStr := "—"
	cacheStr := "—"
	costStr := "—"
	durationStr := "—"

	if v.tokenMetrics != nil {
		tokStr = formatTokenCount(v.tokenMetrics.TotalTokens)
		if v.tokenMetrics.TotalTokens > 0 {
			hitRate := float64(v.tokenMetrics.CacheReadTokens) / float64(v.tokenMetrics.TotalTokens) * 100
			cacheStr = fmt.Sprintf("%.1f%%", hitRate)
		}
	}
	if v.costReport != nil {
		costStr = fmt.Sprintf("$%.4f", v.costReport.TotalCostUSD)
	}
	if v.latencyMetrics != nil && v.latencyMetrics.Count > 0 {
		durationStr = formatDurationMs(v.latencyMetrics.MeanMs)
	}

	b.WriteString(fmt.Sprintf("  Tokens: %s   Avg duration: %s   Cache hit rate: %s\n",
		faint.Render(tokStr), faint.Render(durationStr), faint.Render(cacheStr)))
	b.WriteString(fmt.Sprintf("  Est. cost: %s\n", faint.Render(costStr)))

	return b.String()
}

// renderScorerTable renders Section 2: Scorer Summary table.
func (v *ScoresView) renderScorerTable() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	b.WriteString(bold.Render("Scorer Summary") + "\n")
	divWidth := 40
	if v.width > 2 {
		divWidth = v.width - 2
	}
	b.WriteString(faint.Render(strings.Repeat("─", divWidth)) + "\n")

	if len(v.agg) == 0 {
		b.WriteString(faint.Render("  No scorer data.") + "\n")
		return b.String()
	}

	// Column widths: Scorer(20) Count(6) Mean(6) Min(6) Max(6) [P50(6) if width >= 60]
	showP50 := v.width == 0 || v.width >= 60
	header := fmt.Sprintf("  %-20s  %5s  %5s  %5s  %5s", "Scorer", "Count", "Mean", "Min", "Max")
	if showP50 {
		header += fmt.Sprintf("  %5s", "P50")
	}
	b.WriteString(faint.Render(header) + "\n")

	for _, a := range v.agg {
		name := a.ScorerName
		if name == "" {
			name = a.ScorerID
		}
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		row := fmt.Sprintf("  %-20s  %5d  %5.2f  %5.2f  %5.2f",
			name, a.Count, a.Mean, a.Min, a.Max)
		if showP50 {
			row += fmt.Sprintf("  %5.2f", a.P50)
		}
		b.WriteString(row + "\n")
	}

	return b.String()
}

// renderRecentScores renders Section 3: Recent Evaluations.
func (v *ScoresView) renderRecentScores() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	b.WriteString(bold.Render("Recent Evaluations") + "\n")
	divWidth := 40
	if v.width > 2 {
		divWidth = v.width - 2
	}
	b.WriteString(faint.Render(strings.Repeat("─", divWidth)) + "\n")

	if len(v.scores) == 0 {
		b.WriteString(faint.Render("  No evaluations.") + "\n")
		return b.String()
	}

	// Header row
	b.WriteString(faint.Render(fmt.Sprintf("  %-8s  %-16s  %-16s  %5s  %s",
		"Run", "Node", "Scorer", "Score", "Source")) + "\n")

	// Last 10 entries (scores are already DESC-ordered from ListRecentScores).
	limit := 10
	if len(v.scores) < limit {
		limit = len(v.scores)
	}
	for _, s := range v.scores[:limit] {
		runID := s.RunID
		if len(runID) > 8 {
			runID = runID[:8]
		}
		nodeID := s.NodeID
		if len(nodeID) > 16 {
			nodeID = nodeID[:13] + "..."
		}
		scorer := s.ScorerName
		if scorer == "" {
			scorer = s.ScorerID
		}
		if len(scorer) > 16 {
			scorer = scorer[:13] + "..."
		}
		b.WriteString(fmt.Sprintf("  %-8s  %-16s  %-16s  %5.2f  %s\n",
			runID, nodeID, scorer, s.Score, s.Source))
	}

	return b.String()
}

// renderMetricsDetail renders the full metrics detail tab: tokens, latency, cost.
func (v *ScoresView) renderMetricsDetail() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	if v.metricsLoading {
		b.WriteString("  Loading metrics...\n")
		return b.String()
	}
	if v.metricsErr != nil {
		b.WriteString(fmt.Sprintf("  Error loading metrics: %v\n", v.metricsErr))
		return b.String()
	}

	divWidth := 40
	if v.width > 2 {
		divWidth = v.width - 2
	}
	div := faint.Render(strings.Repeat("─", divWidth))

	// --- Token Usage ---
	b.WriteString(bold.Render("Token Usage") + "\n")
	b.WriteString(div + "\n")
	if v.tokenMetrics == nil {
		b.WriteString(faint.Render("  No token data available.") + "\n")
	} else {
		tm := v.tokenMetrics
		b.WriteString(fmt.Sprintf("  Total:        %s\n", formatTokenCount(tm.TotalTokens)))
		b.WriteString(fmt.Sprintf("  Input:        %s\n", formatTokenCount(tm.TotalInputTokens)))
		b.WriteString(fmt.Sprintf("  Output:       %s\n", formatTokenCount(tm.TotalOutputTokens)))
		if tm.CacheReadTokens > 0 || tm.CacheWriteTokens > 0 {
			b.WriteString(fmt.Sprintf("  Cache read:   %s\n", formatTokenCount(tm.CacheReadTokens)))
			b.WriteString(fmt.Sprintf("  Cache write:  %s\n", formatTokenCount(tm.CacheWriteTokens)))
			if tm.TotalTokens > 0 {
				hitRate := float64(tm.CacheReadTokens) / float64(tm.TotalTokens) * 100
				b.WriteString(fmt.Sprintf("  Cache hit %%:  %.1f%%\n", hitRate))
			}
		}
		// Per-period breakdown if available.
		if len(tm.ByPeriod) > 0 {
			b.WriteString("\n")
			b.WriteString(faint.Render(fmt.Sprintf("  %-30s  %10s  %10s", "Period", "Input", "Output")) + "\n")
			for _, p := range tm.ByPeriod {
				label := truncate(p.Label, 30)
				b.WriteString(fmt.Sprintf("  %-30s  %10s  %10s\n",
					label,
					formatTokenCount(p.InputTokens),
					formatTokenCount(p.OutputTokens)))
			}
		}
	}

	// --- Latency Percentiles ---
	b.WriteString("\n" + bold.Render("Latency") + "\n")
	b.WriteString(div + "\n")
	if v.latencyMetrics == nil || v.latencyMetrics.Count == 0 {
		b.WriteString(faint.Render("  No latency data available.") + "\n")
	} else {
		lm := v.latencyMetrics
		b.WriteString(fmt.Sprintf("  Count:   %d nodes\n", lm.Count))
		b.WriteString(fmt.Sprintf("  Mean:    %s\n", formatDurationMs(lm.MeanMs)))
		b.WriteString(fmt.Sprintf("  Min:     %s\n", formatDurationMs(lm.MinMs)))
		b.WriteString(fmt.Sprintf("  P50:     %s\n", formatDurationMs(lm.P50Ms)))
		b.WriteString(fmt.Sprintf("  P95:     %s\n", formatDurationMs(lm.P95Ms)))
		b.WriteString(fmt.Sprintf("  Max:     %s\n", formatDurationMs(lm.MaxMs)))
	}

	// --- Cost Tracking ---
	b.WriteString("\n" + bold.Render("Cost Tracking") + "\n")
	b.WriteString(div + "\n")
	if v.costReport == nil {
		b.WriteString(faint.Render("  No cost data available.") + "\n")
	} else {
		cr := v.costReport
		b.WriteString(fmt.Sprintf("  Total:   $%.6f USD\n", cr.TotalCostUSD))
		b.WriteString(fmt.Sprintf("  Input:   $%.6f USD\n", cr.InputCostUSD))
		b.WriteString(fmt.Sprintf("  Output:  $%.6f USD\n", cr.OutputCostUSD))
		if cr.RunCount > 0 {
			b.WriteString(fmt.Sprintf("  Runs:    %d\n", cr.RunCount))
			perRun := cr.TotalCostUSD / float64(cr.RunCount)
			b.WriteString(fmt.Sprintf("  Per run: $%.6f USD\n", perRun))
		}
		// Per-period cost breakdown if available.
		if len(cr.ByPeriod) > 0 {
			b.WriteString("\n")
			b.WriteString(faint.Render(fmt.Sprintf("  %-20s  %12s  %6s", "Period", "Total", "Runs")) + "\n")
			for _, p := range cr.ByPeriod {
				label := truncate(p.Label, 20)
				b.WriteString(fmt.Sprintf("  %-20s  $%11.6f  %6d\n",
					label, p.TotalCostUSD, p.RunCount))
			}
		}
	}

	// --- Daily / Weekly Summaries ---
	b.WriteString("\n" + bold.Render("Summaries") + "\n")
	b.WriteString(div + "\n")
	b.WriteString(v.renderDailyWeeklySummary())

	return b.String()
}

// renderDailyWeeklySummary renders daily and weekly cost and token roll-ups.
// When per-period data is available from costReport or tokenMetrics it's used;
// otherwise a concise aggregate row is shown.
func (v *ScoresView) renderDailyWeeklySummary() string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)

	// Build daily total from ByPeriod if available.
	if v.costReport != nil && len(v.costReport.ByPeriod) > 0 {
		todayLabel := time.Now().Format("2006-01-02")
		weekAgo := time.Now().AddDate(0, 0, -7)

		dailyCost := 0.0
		weeklyCost := 0.0
		weeklyRuns := 0

		for _, p := range v.costReport.ByPeriod {
			if p.Label == todayLabel {
				dailyCost += p.TotalCostUSD
			}
			// Try parsing period label as a date for weekly roll-up.
			if t, err := time.Parse("2006-01-02", p.Label); err == nil {
				if !t.Before(weekAgo) {
					weeklyCost += p.TotalCostUSD
					weeklyRuns += p.RunCount
				}
			}
		}

		b.WriteString(fmt.Sprintf("  Daily cost (today):   $%.6f USD\n", dailyCost))
		b.WriteString(fmt.Sprintf("  Weekly cost (7d):     $%.6f USD  (%d runs)\n", weeklyCost, weeklyRuns))
	} else if v.costReport != nil {
		// No per-period data — show aggregate total.
		b.WriteString(fmt.Sprintf("  Aggregate total:      $%.6f USD  (%d runs)\n",
			v.costReport.TotalCostUSD, v.costReport.RunCount))
		b.WriteString(faint.Render("  Per-period breakdown not available.") + "\n")
	} else {
		b.WriteString(faint.Render("  No summary data available.") + "\n")
	}

	return b.String()
}

// --- Format helpers ---

// formatTokenCount formats large token counts with K/M suffixes.
func formatTokenCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// formatDurationMs formats a millisecond duration as a human-readable string.
func formatDurationMs(ms float64) string {
	switch {
	case ms >= 60_000:
		return fmt.Sprintf("%.1fm", ms/60_000)
	case ms >= 1_000:
		return fmt.Sprintf("%.2fs", ms/1_000)
	default:
		return fmt.Sprintf("%.0fms", ms)
	}
}
