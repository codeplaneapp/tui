package smithers

import "time"

// Snapshot represents a point-in-time capture of a workflow run's state.
// Maps to snapshot records in smithers/src/db/internal-schema.ts
type Snapshot struct {
	ID           string    `json:"id"`
	RunID        string    `json:"runId"`
	SnapshotNo   int       `json:"snapshotNo"`   // Ordinal within the run (1-based)
	NodeID       string    `json:"nodeId"`        // The workflow node active at snapshot time
	Iteration    int       `json:"iteration"`     // Attempt iteration number
	Attempt      int       `json:"attempt"`       // Attempt number within iteration
	Label        string    `json:"label"`         // Human-readable label, e.g. "After tool: bash"
	CreatedAt    time.Time `json:"createdAt"`
	StateJSON    string    `json:"stateJson"`     // Serialized run state at this snapshot
	SizeBytes    int64     `json:"sizeBytes"`     // Storage size of the snapshot
	ParentID     *string   `json:"parentId"`      // ID of the snapshot this was forked from, if any
}

// DiffEntry represents a single change between two snapshots.
type DiffEntry struct {
	Path     string `json:"path"`     // JSON path of the changed field, e.g. "messages[3].content"
	Op       string `json:"op"`       // "add" | "remove" | "replace"
	OldValue any    `json:"oldValue"` // Value before the change (nil for "add")
	NewValue any    `json:"newValue"` // Value after the change (nil for "remove")
}

// SnapshotDiff holds the computed difference between two snapshots.
type SnapshotDiff struct {
	FromID      string      `json:"fromId"`
	ToID        string      `json:"toId"`
	FromNo      int         `json:"fromNo"`
	ToNo        int         `json:"toNo"`
	Entries     []DiffEntry `json:"entries"`
	AddedCount  int         `json:"addedCount"`
	RemovedCount int        `json:"removedCount"`
	ChangedCount int        `json:"changedCount"`
}

// ForkOptions configures how a new run is forked from a snapshot.
type ForkOptions struct {
	// WorkflowPath overrides the workflow file used for the forked run.
	// If empty, the original run's workflow is used.
	WorkflowPath string `json:"workflowPath,omitempty"`

	// Inputs overrides the workflow inputs for the forked run.
	// If nil, the original run's inputs are reused.
	Inputs map[string]string `json:"inputs,omitempty"`

	// Label sets an optional human-readable label for the forked run.
	Label string `json:"label,omitempty"`
}

// ReplayOptions configures how a run is replayed from a snapshot.
type ReplayOptions struct {
	// StopAt is an optional snapshot ID at which to pause the replay.
	StopAt *string `json:"stopAt,omitempty"`

	// Speed controls replay speed multiplier (1.0 = real-time, 0 = as fast as possible).
	Speed float64 `json:"speed,omitempty"`

	// Label sets an optional human-readable label for the replayed run.
	Label string `json:"label,omitempty"`
}

// ForkReplayRun represents a Smithers workflow run returned from fork/replay operations.
// For the full run schema see smithers/src/db/internal-schema.ts
// Note: Distinct from Run in types_runs.go which is the API response schema.
type ForkReplayRun struct {
	ID           string     `json:"id"`
	WorkflowPath string     `json:"workflowPath"`
	Status       string     `json:"status"`       // "active" | "paused" | "completed" | "failed" | "cancelled"
	Label        *string    `json:"label"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	ForkedFrom   *string    `json:"forkedFrom"`   // Source snapshot ID if this run was forked
}
