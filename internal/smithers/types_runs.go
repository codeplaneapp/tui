package smithers

import (
	"encoding/json"
	"strings"
)

// RunStatus mirrors ../smithers/src/RunStatus.ts
type RunStatus string

const (
	RunStatusRunning         RunStatus = "running"
	RunStatusWaitingApproval RunStatus = "waiting-approval"
	RunStatusWaitingEvent    RunStatus = "waiting-event"
	RunStatusFinished        RunStatus = "finished"
	RunStatusFailed          RunStatus = "failed"
	RunStatusCancelled       RunStatus = "cancelled"
)

// IsTerminal reports whether the status represents a terminal (completed) state.
func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunStatusFinished, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

// TaskState mirrors node/task execution states.
type TaskState string

const (
	TaskStatePending   TaskState = "pending"
	TaskStateRunning   TaskState = "running"
	TaskStateFinished  TaskState = "finished"
	TaskStateFailed    TaskState = "failed"
	TaskStateCancelled TaskState = "cancelled"
	TaskStateSkipped   TaskState = "skipped"
	TaskStateBlocked   TaskState = "blocked"
)

// RunTask mirrors a node execution record from _smithers_nodes.
// Maps to RunNodeSummary in smithers/src/cli/tui-v2/shared/types.ts
type RunTask struct {
	NodeID      string    `json:"nodeId"`
	Label       *string   `json:"label"`
	Iteration   int       `json:"iteration"`
	State       TaskState `json:"state"`
	LastAttempt *int      `json:"lastAttempt"`
	UpdatedAtMs *int64    `json:"updatedAtMs"`
}

// RunSummary is the top-level run record returned by the v1 runs API.
// GET /v1/runs returns []RunSummary; GET /v1/runs/:id returns a single RunSummary.
// Maps to the server response shape in smithers/src/server/index.ts
type RunSummary struct {
	RunID        string         `json:"runId"`
	WorkflowName string         `json:"workflowName"`
	WorkflowPath string         `json:"workflowPath,omitempty"`
	Status       RunStatus      `json:"status"`
	StartedAtMs  *int64         `json:"startedAtMs,omitempty"`
	FinishedAtMs *int64         `json:"finishedAtMs,omitempty"`
	Summary      map[string]int `json:"summary,omitempty"` // node-state → count
	ErrorJSON    *string        `json:"errorJson,omitempty"`
}

// RunInspection is the enriched run detail returned by InspectRun.
// Composed from GET /v1/runs/:id plus node-level data from SQLite or exec.
type RunInspection struct {
	RunSummary
	Tasks    []RunTask `json:"tasks,omitempty"`
	EventSeq int       `json:"eventSeq,omitempty"` // last seen event sequence number
}

// ErrorReason returns a short human-readable error string for display,
// or an empty string when no error is present.
// It tries to unmarshal ErrorJSON as {"message":"..."} first; falls back
// to the raw string trimmed to 80 characters.
func (r RunSummary) ErrorReason() string {
	if r.ErrorJSON == nil {
		return ""
	}
	raw := *r.ErrorJSON
	var obj struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err == nil && obj.Message != "" {
		return obj.Message
	}
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > 80 {
		trimmed = trimmed[:80]
	}
	return trimmed
}

// RunFilter specifies query parameters for ListRuns.
type RunFilter struct {
	// Limit is the maximum number of runs to return. Defaults to 50 if zero.
	Limit int
	// Status filters by run status. Empty string returns all statuses.
	Status string
}

// RunEvent is the envelope for SSE payloads from GET /v1/runs/:id/events.
// The Type field is the discriminator; maps to SmithersEvent in
// smithers/src/SmithersEvent.ts
type RunEvent struct {
	Type        string          `json:"type"`
	RunID       string          `json:"runId"`
	NodeID      string          `json:"nodeId,omitempty"`
	Iteration   int             `json:"iteration,omitempty"`
	Attempt     int             `json:"attempt,omitempty"`
	Status      string          `json:"status,omitempty"`
	TimestampMs int64           `json:"timestampMs"`
	Seq         int             `json:"seq,omitempty"`
	// Raw preserves the original JSON for forwarding or debugging.
	Raw json.RawMessage `json:"-"`
}

// RunEventMsg is a tea.Msg carrying a RunEvent received from the SSE stream.
type RunEventMsg struct {
	RunID string
	Event RunEvent
}

// RunEventErrorMsg is sent when the SSE stream encounters an unrecoverable error.
type RunEventErrorMsg struct {
	RunID string
	Err   error
}

// RunEventDoneMsg is sent when the SSE stream closes because the run reached
// a terminal state and the event queue is drained.
type RunEventDoneMsg struct {
	RunID string
}

// Run is a type alias for ForkReplayRun, preserved for backward compatibility
// with existing code in timetravel.go and client.go that references Run.
// New code should prefer RunSummary for v1 API responses or ForkReplayRun for
// fork/replay operation results.
type Run = ForkReplayRun

// ChatRole identifies the author of a ChatBlock.
type ChatRole string

const (
	ChatRoleSystem    ChatRole = "system"
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
	ChatRoleTool      ChatRole = "tool"
)

// ChatBlock is one message in the agent conversation transcript for a task attempt.
// Maps to a row in _smithers_chat_attempts or the shape returned by
// GET /v1/runs/:id/chat.
type ChatBlock struct {
	// ID is an opaque per-block identifier (may be empty for stub data).
	ID string `json:"id,omitempty"`
	// RunID is the parent run.
	RunID string `json:"runId"`
	// NodeID is the DAG node that owns this block.
	NodeID string `json:"nodeId"`
	// Attempt is the retry attempt counter (0-based).
	Attempt int `json:"attempt"`
	// Role is the message author.
	Role ChatRole `json:"role"`
	// Content is the plain-text (or markdown) body of the message.
	Content string `json:"content"`
	// TimestampMs is the Unix-millisecond creation time.
	TimestampMs int64 `json:"timestampMs"`
}

// ChatBlockMsg is a tea.Msg carrying a newly-arrived ChatBlock during streaming.
type ChatBlockMsg struct {
	RunID string
	Block ChatBlock
}

// ChatStreamDoneMsg is sent when the chat stream for a run reaches end-of-stream.
type ChatStreamDoneMsg struct {
	RunID string
}

// ChatStreamErrorMsg is sent when the chat stream encounters an unrecoverable error.
type ChatStreamErrorMsg struct {
	RunID string
	Err   error
}

// RunStatusSummary is the aggregate view of active runs used by the header
// and status-bar surfaces.
type RunStatusSummary struct {
	// ActiveRuns is the count of runs whose status is running,
	// waiting-approval, or waiting-event.
	ActiveRuns int
	// PendingApprovals is the count of runs in the waiting-approval state
	// (always a subset of ActiveRuns).
	PendingApprovals int
}

// SummariseRuns derives a RunStatusSummary from a slice of RunSummary values.
// Active = running | waiting-approval | waiting-event.
// PendingApprovals = waiting-approval only (subset of Active).
func SummariseRuns(runs []RunSummary) RunStatusSummary {
	var s RunStatusSummary
	for _, r := range runs {
		switch r.Status {
		case RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingEvent:
			s.ActiveRuns++
		}
		if r.Status == RunStatusWaitingApproval {
			s.PendingApprovals++
		}
	}
	return s
}

// HijackSession carries the metadata returned by POST /v1/runs/:id/hijack.
// The client uses this to hand off the terminal to the agent's native CLI
// via tea.ExecProcess.
type HijackSession struct {
	RunID          string `json:"runId"`
	AgentEngine    string `json:"agentEngine"`    // e.g. "claude-code"
	AgentBinary    string `json:"agentBinary"`    // resolved path, e.g. "/usr/local/bin/claude"
	ResumeToken    string `json:"resumeToken"`    // session ID to pass to --resume
	CWD            string `json:"cwd"`            // working directory at time of hijack
	SupportsResume bool   `json:"supportsResume"` // whether --resume is supported
}

// ResumeArgs returns the CLI arguments for resuming the agent session.
// The argument format is engine-specific:
//
//   - claude-code / claude: --resume <token>
//   - codex:                --session-id <token>
//   - amp:                  --resume <token>
//   - gemini:               --session <token>
//   - other / unknown:      --resume <token>  (generic fallback)
//
// Returns nil when SupportsResume is false or ResumeToken is empty.
func (h *HijackSession) ResumeArgs() []string {
	if !h.SupportsResume || h.ResumeToken == "" {
		return nil
	}
	switch h.AgentEngine {
	case "codex":
		return []string{"--session-id", h.ResumeToken}
	case "gemini":
		return []string{"--session", h.ResumeToken}
	default:
		// claude-code, claude, amp, and any unknown engine use --resume.
		return []string{"--resume", h.ResumeToken}
	}
}

// v1ErrorEnvelope is the error shape returned by the v1 API:
//
//	{ "error": { "code": "...", "message": "..." } }
type v1ErrorEnvelope struct {
	Error *v1ErrorBody `json:"error"`
}

type v1ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
