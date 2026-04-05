package smithers

// TableInfo describes a table in the Smithers SQLite database.
// Used by the SQL Browser table sidebar (PRD §6.11).
type TableInfo struct {
	Name     string `json:"name"`
	RowCount int64  `json:"rowCount"`
	Type     string `json:"type"` // "table" | "view" | "index"
}

// Column describes a single column within a database table.
type Column struct {
	CID          int     `json:"cid"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	NotNull      bool    `json:"notNull"`
	DefaultValue *string `json:"defaultValue,omitempty"`
	PrimaryKey   bool    `json:"primaryKey"`
}

// TableSchema holds the full schema for a single table.
type TableSchema struct {
	TableName string   `json:"tableName"`
	Columns   []Column `json:"columns"`
}

// MetricsFilter carries optional time-range and grouping filters for
// analytics queries (token usage, latency, cost tracking).
type MetricsFilter struct {
	// WorkflowPath restricts results to a specific workflow, e.g. "review.tsx".
	// Empty means all workflows.
	WorkflowPath string `json:"workflowPath,omitempty"`

	// RunID restricts results to a specific run. Empty means all runs.
	RunID string `json:"runId,omitempty"`

	// NodeID restricts results to a specific node. Empty means all nodes.
	NodeID string `json:"nodeId,omitempty"`

	// StartMs is the start of the time window (Unix ms, inclusive). 0 means no lower bound.
	StartMs int64 `json:"startMs,omitempty"`

	// EndMs is the end of the time window (Unix ms, inclusive). 0 means no upper bound.
	EndMs int64 `json:"endMs,omitempty"`

	// GroupBy controls how results are aggregated. Values: "run" | "workflow" | "day" | "hour".
	// Empty defaults to "run".
	GroupBy string `json:"groupBy,omitempty"`
}

// TokenMetrics holds token usage statistics for one or more runs.
// Derived from _smithers_chat_attempts and scored data.
// Maps to SCORES_TOKEN_USAGE_METRICS feature flag.
type TokenMetrics struct {
	TotalInputTokens  int64              `json:"totalInputTokens"`
	TotalOutputTokens int64              `json:"totalOutputTokens"`
	TotalTokens       int64              `json:"totalTokens"`
	CacheReadTokens   int64              `json:"cacheReadTokens"`
	CacheWriteTokens  int64              `json:"cacheWriteTokens"`
	ByPeriod          []TokenPeriodBatch `json:"byPeriod,omitempty"`
}

// TokenPeriodBatch holds token counts for a single grouping period (run, day, etc.).
type TokenPeriodBatch struct {
	Label            string `json:"label"` // run ID, workflow path, or date string
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  int64  `json:"cacheReadTokens"`
	CacheWriteTokens int64  `json:"cacheWriteTokens"`
}

// LatencyMetrics holds timing statistics for agent node executions.
// Maps to SCORES_LATENCY_METRICS feature flag.
type LatencyMetrics struct {
	// Count is the total number of node executions measured.
	Count int `json:"count"`

	// MeanMs is the arithmetic mean latency in milliseconds.
	MeanMs float64 `json:"meanMs"`

	// MinMs is the minimum latency observed.
	MinMs float64 `json:"minMs"`

	// MaxMs is the maximum latency observed.
	MaxMs float64 `json:"maxMs"`

	// P50Ms is the median latency.
	P50Ms float64 `json:"p50Ms"`

	// P95Ms is the 95th-percentile latency.
	P95Ms float64 `json:"p95Ms"`

	// ByPeriod holds per-group latency summaries when GroupBy is set.
	ByPeriod []LatencyPeriodBatch `json:"byPeriod,omitempty"`
}

// LatencyPeriodBatch holds latency summary for a single grouping period.
type LatencyPeriodBatch struct {
	Label  string  `json:"label"`
	Count  int     `json:"count"`
	MeanMs float64 `json:"meanMs"`
	P50Ms  float64 `json:"p50Ms"`
	P95Ms  float64 `json:"p95Ms"`
}

// CostReport holds estimated cost information for one or more runs.
// Maps to SCORES_COST_TRACKING feature flag.
// Costs are expressed in USD.
type CostReport struct {
	// TotalCostUSD is the estimated total cost across all included runs.
	TotalCostUSD float64 `json:"totalCostUsd"`

	// InputCostUSD is the estimated cost for input/prompt tokens.
	InputCostUSD float64 `json:"inputCostUsd"`

	// OutputCostUSD is the estimated cost for output/completion tokens.
	OutputCostUSD float64 `json:"outputCostUsd"`

	// RunCount is the number of runs included in this report.
	RunCount int `json:"runCount"`

	// ByPeriod holds per-group cost summaries when GroupBy is set.
	ByPeriod []CostPeriodBatch `json:"byPeriod,omitempty"`
}

// CostPeriodBatch holds cost breakdown for a single grouping period.
type CostPeriodBatch struct {
	Label         string  `json:"label"`
	TotalCostUSD  float64 `json:"totalCostUsd"`
	InputCostUSD  float64 `json:"inputCostUsd"`
	OutputCostUSD float64 `json:"outputCostUsd"`
	RunCount      int     `json:"runCount"`
}
