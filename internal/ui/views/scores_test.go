package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScoresView_InterfaceCompliance verifies the compile-time interface check.
func TestScoresView_InterfaceCompliance(t *testing.T) {
	// Compile-time check is already in scores.go; this is belt-and-suspenders.
	var _ View = (*ScoresView)(nil)
}

// TestScoresView_InitialState verifies the initial state after construction.
func TestScoresView_InitialState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	assert.True(t, v.loading)
	assert.Nil(t, v.err)
	assert.Empty(t, v.scores)
	assert.Empty(t, v.agg)
}

// TestScoresView_LoadedMsg verifies that scoresLoadedMsg sets data and clears loading.
func TestScoresView_LoadedMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = true

	scores := []smithers.ScoreRow{
		{ID: "s1", RunID: "run-abc123", NodeID: "node-1", ScorerID: "relevancy",
			ScorerName: "Relevancy", Source: "live", Score: 0.92, ScoredAtMs: 1000},
	}
	agg := []smithers.AggregateScore{
		{ScorerID: "relevancy", ScorerName: "Relevancy", Count: 1, Mean: 0.92,
			Min: 0.92, Max: 0.92, P50: 0.92},
	}

	v2, _ := v.Update(scoresLoadedMsg{scores: scores, agg: agg})
	sv := v2.(*ScoresView)

	assert.False(t, sv.loading)
	assert.Nil(t, sv.err)
	require.Len(t, sv.scores, 1)
	require.Len(t, sv.agg, 1)
}

// TestScoresView_ErrorMsg verifies that scoresErrorMsg sets error and clears loading.
func TestScoresView_ErrorMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = true

	v2, _ := v.Update(scoresErrorMsg{err: errors.New("db error")})
	sv := v2.(*ScoresView)

	assert.False(t, sv.loading)
	require.NotNil(t, sv.err)
	assert.Contains(t, sv.err.Error(), "db error")
}

// TestScoresView_WindowSizeMsg verifies that window size is stored.
func TestScoresView_WindowSizeMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v2, _ := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	sv := v2.(*ScoresView)
	assert.Equal(t, 120, sv.width)
	assert.Equal(t, 40, sv.height)
}

// TestScoresView_EscReturnsPopMsg verifies that Esc sends PopViewMsg.
func TestScoresView_EscReturnsPopMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok)
}

// TestScoresView_RefreshTriggersInit verifies that 'r' sets loading and returns a cmd.
func TestScoresView_RefreshTriggersInit(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	// After pressing r, loading should be set and a cmd returned.
	assert.True(t, v.loading)
	assert.NotNil(t, cmd)
}

// TestScoresView_ViewLoadingState verifies the loading state renders correctly.
func TestScoresView_ViewLoadingState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = true
	out := v.View()
	assert.Contains(t, ansi.Strip(out), "SMITHERS › Scores")
	assert.Contains(t, out, "Loading scores")
}

// TestScoresView_ViewErrorState verifies the error state renders correctly.
func TestScoresView_ViewErrorState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.err = errors.New("connection refused")
	out := v.View()
	assert.Contains(t, out, "Error: connection refused")
}

// TestScoresView_ViewEmptyState verifies the empty state renders correctly.
func TestScoresView_ViewEmptyState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "No score data available")
}

// TestScoresView_ViewWithData verifies all three sections render when data is present.
func TestScoresView_ViewWithData(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{
		{RunID: "abc12345", NodeID: "review", ScorerID: "rel", ScorerName: "Relevancy",
			Source: "live", Score: 0.95, ScoredAtMs: 9999},
	}
	v.agg = []smithers.AggregateScore{
		{ScorerID: "rel", ScorerName: "Relevancy", Count: 1, Mean: 0.95,
			Min: 0.95, Max: 0.95, P50: 0.95},
	}
	v.width = 120
	out := v.View()
	assert.Contains(t, out, "Today's Summary")
	assert.Contains(t, out, "Scorer Summary")
	assert.Contains(t, out, "Recent Evaluations")
	assert.Contains(t, out, "Relevancy")
	assert.Contains(t, out, "0.95")
	assert.Contains(t, out, "abc12345")
}

// TestScoresView_NarrowTerminalHidesP50 verifies P50 is hidden below 60 columns.
func TestScoresView_NarrowTerminalHidesP50(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.agg = []smithers.AggregateScore{
		{ScorerID: "q", ScorerName: "Quality", Count: 5, Mean: 0.8, Min: 0.6, Max: 1.0, P50: 0.82},
	}
	v.width = 55 // below 60-column threshold
	out := v.renderScorerTable()
	// P50 column should not appear at narrow widths.
	assert.NotContains(t, out, "P50")
}

// TestScoresView_ScorerNameTruncation verifies long scorer names are truncated with ellipsis.
func TestScoresView_ScorerNameTruncation(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.agg = []smithers.AggregateScore{
		{ScorerID: "x", ScorerName: "A Very Long Scorer Name That Exceeds Limit",
			Count: 1, Mean: 0.5, Min: 0.5, Max: 0.5, P50: 0.5},
	}
	v.width = 120
	out := v.renderScorerTable()
	assert.Contains(t, out, "...")
}

// TestScoresView_Name verifies the view name.
func TestScoresView_Name(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	assert.Equal(t, "scores", v.Name())
}

// TestScoresView_ShortHelp verifies the keybinding hints are present.
func TestScoresView_ShortHelp(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	var allDesc []string
	for _, b := range help {
		allDesc = append(allDesc, b.Help().Desc)
	}
	joined := strings.Join(allDesc, " ")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")
}

// TestScoresView_SetSize verifies SetSize propagates dimensions.
func TestScoresView_SetSize(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.SetSize(80, 24)
	assert.Equal(t, 80, v.width)
	assert.Equal(t, 24, v.height)
}

// --- Tab switching (summary / detail) ---

func TestScoresView_TabSwitchesToDetail(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}

	assert.Equal(t, scoresTabSummary, v.activeTab, "starts on summary tab")
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	sv := updated.(*ScoresView)

	assert.Equal(t, scoresTabDetail, sv.activeTab, "tab should switch to detail")
	assert.NotNil(t, cmd, "switching to detail should issue metrics load command")
	assert.True(t, sv.metricsLoading, "metricsLoading should be true after tab switch")
}

func TestScoresView_TabSwitchesBackToSummary(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	sv := updated.(*ScoresView)
	assert.Equal(t, scoresTabSummary, sv.activeTab, "tab from detail should return to summary")
}

func TestScoresView_DetailTabDoesNotReloadWhenAlreadyLoaded(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	// Pre-populate metrics so they don't need to be reloaded.
	v.tokenMetrics = &smithers.TokenMetrics{TotalTokens: 100}
	v.latencyMetrics = &smithers.LatencyMetrics{Count: 5}
	v.costReport = &smithers.CostReport{TotalCostUSD: 0.001}

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// When metrics are already loaded, no new command should be issued.
	assert.Nil(t, cmd, "should not re-load metrics when already populated")
}

func TestScoresView_DetailTabShowsLoading(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.metricsLoading = true

	out := v.View()
	assert.Contains(t, out, "Loading metrics")
}

func TestScoresView_DetailTabShowsMetricsError(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.metricsErr = errors.New("db timeout")

	out := v.View()
	assert.Contains(t, out, "Error loading metrics")
	assert.Contains(t, out, "db timeout")
}

// --- scoresMetricsLoadedMsg ---

func TestScoresView_MetricsLoadedMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.metricsLoading = true

	tokens := &smithers.TokenMetrics{
		TotalInputTokens:  50000,
		TotalOutputTokens: 15000,
		TotalTokens:       65000,
		CacheReadTokens:   8000,
		CacheWriteTokens:  2000,
	}
	latency := &smithers.LatencyMetrics{
		Count:  42,
		MeanMs: 1234.5,
		P50Ms:  980.0,
		P95Ms:  3200.0,
		MinMs:  200.0,
		MaxMs:  5100.0,
	}
	cost := &smithers.CostReport{
		TotalCostUSD:  0.001234,
		InputCostUSD:  0.00015,
		OutputCostUSD: 0.00225,
		RunCount:      3,
	}

	updated, _ := v.Update(scoresMetricsLoadedMsg{tokens: tokens, latency: latency, cost: cost})
	sv := updated.(*ScoresView)

	assert.False(t, sv.metricsLoading)
	require.NotNil(t, sv.tokenMetrics)
	require.NotNil(t, sv.latencyMetrics)
	require.NotNil(t, sv.costReport)
	assert.Equal(t, int64(65000), sv.tokenMetrics.TotalTokens)
	assert.Equal(t, 42, sv.latencyMetrics.Count)
	assert.InDelta(t, 0.001234, sv.costReport.TotalCostUSD, 1e-9)
}

func TestScoresView_MetricsErrorMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.metricsLoading = true

	updated, _ := v.Update(scoresMetricsErrorMsg{err: errors.New("metrics unavailable")})
	sv := updated.(*ScoresView)

	assert.False(t, sv.metricsLoading)
	require.NotNil(t, sv.metricsErr)
	assert.Contains(t, sv.metricsErr.Error(), "metrics unavailable")
}

// --- Token usage metrics display ---

func TestScoresView_TokenMetricsInDetailTab(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.tokenMetrics = &smithers.TokenMetrics{
		TotalInputTokens:  1_234_567,
		TotalOutputTokens: 456_789,
		TotalTokens:       1_691_356,
		CacheReadTokens:   100_000,
		CacheWriteTokens:  50_000,
	}
	v.latencyMetrics = &smithers.LatencyMetrics{}
	v.costReport = &smithers.CostReport{}

	out := v.View()
	assert.Contains(t, out, "Token Usage")
	assert.Contains(t, out, "Total:")
	assert.Contains(t, out, "Input:")
	assert.Contains(t, out, "Output:")
	assert.Contains(t, out, "Cache read:")
	assert.Contains(t, out, "Cache write:")
	assert.Contains(t, out, "Cache hit %:")
}

func TestScoresView_TokenMetricsNoData(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.tokenMetrics = nil
	v.latencyMetrics = nil
	v.costReport = nil

	out := v.View()
	assert.Contains(t, out, "No token data available.")
}

// --- Latency percentiles display ---

func TestScoresView_LatencyMetricsInDetailTab(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.tokenMetrics = &smithers.TokenMetrics{}
	v.latencyMetrics = &smithers.LatencyMetrics{
		Count:  100,
		MeanMs: 1250.0,
		MinMs:  100.0,
		P50Ms:  1100.0,
		P95Ms:  3200.0,
		MaxMs:  8000.0,
	}
	v.costReport = &smithers.CostReport{}

	out := v.View()
	assert.Contains(t, out, "Latency")
	assert.Contains(t, out, "Count:")
	assert.Contains(t, out, "Mean:")
	assert.Contains(t, out, "P50:")
	assert.Contains(t, out, "P95:")
	assert.Contains(t, out, "Min:")
	assert.Contains(t, out, "Max:")
}

func TestScoresView_LatencyMetricsNoData(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.latencyMetrics = &smithers.LatencyMetrics{Count: 0}

	out := v.View()
	assert.Contains(t, out, "No latency data available.")
}

// --- Cost tracking display ---

func TestScoresView_CostTrackingInDetailTab(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.tokenMetrics = &smithers.TokenMetrics{}
	v.latencyMetrics = &smithers.LatencyMetrics{}
	v.costReport = &smithers.CostReport{
		TotalCostUSD:  0.0456,
		InputCostUSD:  0.0123,
		OutputCostUSD: 0.0333,
		RunCount:      5,
	}

	out := v.View()
	assert.Contains(t, out, "Cost Tracking")
	assert.Contains(t, out, "Total:")
	assert.Contains(t, out, "Runs:    5")
	assert.Contains(t, out, "Per run:")
}

func TestScoresView_CostTrackingNoData(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.tokenMetrics = nil
	v.latencyMetrics = nil
	v.costReport = nil

	out := v.View()
	assert.Contains(t, out, "No cost data available.")
}

// --- Daily/weekly summaries ---

func TestScoresView_DailyWeeklySummaryWithByPeriod(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	v.costReport = &smithers.CostReport{
		TotalCostUSD: 0.123,
		RunCount:     3,
		ByPeriod: []smithers.CostPeriodBatch{
			{Label: today, TotalCostUSD: 0.05, RunCount: 1},
			{Label: yesterday, TotalCostUSD: 0.073, RunCount: 2},
		},
	}
	v.tokenMetrics = &smithers.TokenMetrics{}
	v.latencyMetrics = &smithers.LatencyMetrics{}

	out := v.renderDailyWeeklySummary()
	assert.Contains(t, out, "Daily cost (today):")
	assert.Contains(t, out, "Weekly cost (7d):")
}

func TestScoresView_DailyWeeklySummaryNoByPeriod(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.costReport = &smithers.CostReport{
		TotalCostUSD: 0.042,
		RunCount:     7,
	}
	out := v.renderDailyWeeklySummary()
	assert.Contains(t, out, "Aggregate total:")
	assert.Contains(t, out, "Per-period breakdown not available.")
}

func TestScoresView_DailyWeeklySummaryNilCostReport(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.costReport = nil
	out := v.renderDailyWeeklySummary()
	assert.Contains(t, out, "No summary data available.")
}

// --- Summary section with real metrics ---

func TestScoresView_SummaryShowsTokenMetricsWhenAvailable(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{
		{RunID: "x", Score: 0.9},
	}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.tokenMetrics = &smithers.TokenMetrics{
		TotalTokens:     50_000,
		CacheReadTokens: 5_000,
	}
	v.latencyMetrics = &smithers.LatencyMetrics{Count: 10, MeanMs: 2000}
	v.costReport = &smithers.CostReport{TotalCostUSD: 0.002}

	out := v.renderSummary()
	// With real metrics, the placeholder dashes should be replaced by actual values.
	assert.Contains(t, out, "Tokens:")
	assert.Contains(t, out, "50.0K") // 50000 formatted as 50.0K
}

// --- Header shows current tab label ---

func TestScoresView_HeaderShowsSummaryLabel(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabSummary

	out := v.View()
	assert.Contains(t, out, "[Summary]")
}

func TestScoresView_HeaderShowsDetailsLabel(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}
	v.activeTab = scoresTabDetail
	v.metricsLoading = false
	v.tokenMetrics = &smithers.TokenMetrics{}
	v.latencyMetrics = &smithers.LatencyMetrics{}
	v.costReport = &smithers.CostReport{}

	out := v.View()
	assert.Contains(t, out, "[Details]")
}

// --- Tab shown in ShortHelp ---

func TestScoresView_ShortHelpIncludesTabHint(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	help := v.ShortHelp()
	var descs []string
	for _, b := range help {
		descs = append(descs, b.Help().Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "switch view", "ShortHelp should mention tab to switch view")
}

// --- Refresh on detail tab reloads both scores and metrics ---

func TestScoresView_RefreshOnDetailTabLoadsBoth(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.activeTab = scoresTabDetail
	v.scores = []smithers.ScoreRow{{RunID: "x", Score: 0.8}}
	v.agg = []smithers.AggregateScore{{ScorerID: "s", Count: 1}}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	sv := updated.(*ScoresView)

	assert.True(t, sv.loading, "'r' on detail tab should set loading")
	assert.True(t, sv.metricsLoading, "'r' on detail tab should set metricsLoading")
	assert.NotNil(t, cmd, "'r' should return a reload command")
}

// --- formatTokenCount helper ---

func TestFormatTokenCount_LessThanThousand(t *testing.T) {
	assert.Equal(t, "999", formatTokenCount(999))
}

func TestFormatTokenCount_Thousands(t *testing.T) {
	result := formatTokenCount(1500)
	assert.Equal(t, "1.5K", result)
}

func TestFormatTokenCount_Millions(t *testing.T) {
	result := formatTokenCount(2_500_000)
	assert.Equal(t, "2.50M", result)
}

func TestFormatTokenCount_Zero(t *testing.T) {
	assert.Equal(t, "0", formatTokenCount(0))
}

// --- formatDurationMs helper ---

func TestFormatDurationMs_Milliseconds(t *testing.T) {
	result := formatDurationMs(450)
	assert.Equal(t, "450ms", result)
}

func TestFormatDurationMs_Seconds(t *testing.T) {
	result := formatDurationMs(2500)
	assert.Equal(t, "2.50s", result)
}

func TestFormatDurationMs_Minutes(t *testing.T) {
	result := formatDurationMs(90_000) // 1.5 minutes
	assert.Equal(t, "1.5m", result)
}

// TestScoresView_RenderSummaryPlaceholders verifies placeholder metrics render.
func TestScoresView_RenderSummaryPlaceholders(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{
		{RunID: "run-1", NodeID: "node", ScorerID: "s", ScorerName: "S",
			Source: "live", Score: 0.75, ScoredAtMs: 1},
	}
	v.agg = []smithers.AggregateScore{
		{ScorerID: "s", ScorerName: "S", Count: 1, Mean: 0.75, Min: 0.75, Max: 0.75, P50: 0.75},
	}
	out := v.renderSummary()
	// The placeholder values are wrapped in ANSI faint escape codes when no
	// metrics are loaded; check for the label text and the dash marker.
	assert.Contains(t, out, "Tokens:")
	assert.Contains(t, out, "Avg duration:")
	assert.Contains(t, out, "Cache hit rate:")
	assert.Contains(t, out, "Est. cost:")
	assert.Contains(t, out, "—") // dash placeholder present somewhere
	assert.Contains(t, out, "Evaluations: 1")
}
