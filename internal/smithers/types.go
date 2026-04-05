package smithers

// Agent represents a CLI agent detected on the system.
// Maps to AgentAvailability in smithers/src/cli/agent-detection.ts
// and AgentCli in smithers/gui-ref/packages/shared/src/schemas/agent.ts.
type Agent struct {
	ID          string   // e.g. "claude-code", "codex", "gemini"
	Name        string   // Display name, e.g. "Claude Code"
	Command     string   // CLI binary name, e.g. "claude"
	BinaryPath  string   // Resolved full path, e.g. "/usr/local/bin/claude"
	Status      string   // "likely-subscription" | "api-key" | "binary-only" | "unavailable"
	HasAuth     bool     // Authentication signal detected
	HasAPIKey   bool     // API key env var present
	Usable      bool     // Agent can be launched
	Roles       []string // e.g. ["coding", "review"]
	Version     string   // from --version probe; "" while unresolved, "(unknown)" on failure
	AuthExpired bool     // true if Claude Code credentials file indicates token is expired
}

// SQLResult holds the result of an arbitrary SQL query.
type SQLResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

// ScoreRow holds a single scorer evaluation result.
// Maps to ScoreRow in smithers/src/scorers/types.ts
type ScoreRow struct {
	ID         string  `json:"id"`
	RunID      string  `json:"runId"`
	NodeID     string  `json:"nodeId"`
	Iteration  int     `json:"iteration"`
	Attempt    int     `json:"attempt"`
	ScorerID   string  `json:"scorerId"`
	ScorerName string  `json:"scorerName"`
	Source     string  `json:"source"` // "live" | "batch"
	Score      float64 `json:"score"`  // 0-1 normalized
	Reason     *string `json:"reason"`
	MetaJSON   *string `json:"metaJson"`
	InputJSON  *string `json:"inputJson"`
	OutputJSON *string `json:"outputJson"`
	LatencyMs  *int64  `json:"latencyMs"`
	ScoredAtMs int64   `json:"scoredAtMs"`
	DurationMs *int64  `json:"durationMs"`
}

// AggregateScore holds aggregated scorer stats for a run.
type AggregateScore struct {
	ScorerID   string  `json:"scorerId"`
	ScorerName string  `json:"scorerName"`
	Count      int     `json:"count"`
	Mean       float64 `json:"mean"`
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	P50        float64 `json:"p50"`
	StdDev     float64 `json:"stddev"`
}

// MemoryFact holds a single memory fact entry.
// Maps to MemoryFact in smithers/src/memory/types.ts
type MemoryFact struct {
	Namespace   string `json:"namespace"`
	Key         string `json:"key"`
	ValueJSON   string `json:"valueJson"`
	SchemaSig   string `json:"schemaSig,omitempty"`
	CreatedAtMs int64  `json:"createdAtMs"`
	UpdatedAtMs int64  `json:"updatedAtMs"`
	TTLMs       *int64 `json:"ttlMs,omitempty"`
}

// MemoryRecallResult holds a semantic recall hit.
type MemoryRecallResult struct {
	Score    float64     `json:"score"`
	Content  string      `json:"content"`
	Metadata interface{} `json:"metadata"`
}

// Ticket represents a discovered ticket from .smithers/tickets/.
// Maps to DiscoveredTicket in smithers/src/cli/tickets.ts
type Ticket struct {
	ID      string `json:"id"`      // Filename, e.g. "feat-tickets-list"
	Content string `json:"content"` // Full markdown content
}

// Approval represents a pending or resolved approval request.
// Maps to approval rows in smithers/src/db/internal-schema.ts
type Approval struct {
	ID           string  `json:"id"`
	RunID        string  `json:"runId"`
	NodeID       string  `json:"nodeId"`
	WorkflowPath string  `json:"workflowPath"`
	Gate         string  `json:"gate"`         // The question or gate name
	Status       string  `json:"status"`       // "pending" | "approved" | "denied"
	Payload      string  `json:"payload"`      // JSON payload with task inputs/context
	RequestedAt  int64   `json:"requestedAt"`  // Unix ms
	ResolvedAt   *int64  `json:"resolvedAt"`   // Unix ms, nil if pending
	ResolvedBy   *string `json:"resolvedBy"`   // Who resolved, nil if pending
}

// ApprovalDecision represents a historical approval decision (approved or denied).
// Derived from resolved rows in _smithers_approvals where status IN ('approved','denied').
// Maps to the daemon approval-repository shape and upstream _smithers_approvals schema.
type ApprovalDecision struct {
	ID           string  `json:"id"`
	RunID        string  `json:"runId"`
	NodeID       string  `json:"nodeId"`
	WorkflowPath string  `json:"workflowPath"`
	Gate         string  `json:"gate"`         // Gate label / question
	Decision     string  `json:"decision"`     // "approved" | "denied"
	DecidedAt    int64   `json:"decidedAt"`    // Unix ms
	DecidedBy    *string `json:"decidedBy"`    // Who made the decision (nil if unknown)
	RequestedAt  int64   `json:"requestedAt"`  // Unix ms
}

// RunContext holds lightweight run-level metadata used by the approval detail
// pane and other views that need run context without full node trees.
// Distinct from RunSummary (the canonical v1 API shape in types_runs.go):
// RunContext is populated from SQLite or exec inspect and includes progress
// counters (NodeTotal, NodesDone) and ElapsedMs for display in approval details.
type RunContext struct {
	ID           string `json:"id"`
	WorkflowPath string `json:"workflowPath"`
	WorkflowName string `json:"workflowName"` // Derived from path basename
	Status       string `json:"status"`       // "running" | "paused" | "completed" | "failed"
	NodeTotal    int    `json:"nodeTotal"`    // Total nodes in the DAG
	NodesDone    int    `json:"nodesDone"`    // Nodes that have finished
	StartedAtMs  int64  `json:"startedAtMs"`  // Unix ms
	ElapsedMs    int64  `json:"elapsedMs"`    // Duration since start
}

// CronSchedule holds a cron trigger definition.
// Maps to cron row schema in smithers/src/db/internal-schema.ts
type CronSchedule struct {
	CronID       string  `json:"cronId"`
	Pattern      string  `json:"pattern"`
	WorkflowPath string  `json:"workflowPath"`
	Enabled      bool    `json:"enabled"`
	CreatedAtMs  int64   `json:"createdAtMs"`
	LastRunAtMs  *int64  `json:"lastRunAtMs,omitempty"`
	NextRunAtMs  *int64  `json:"nextRunAtMs,omitempty"`
	ErrorJSON    *string `json:"errorJson,omitempty"`
}
