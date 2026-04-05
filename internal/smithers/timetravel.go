package smithers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// --- Time-Travel API ---
//
// The time-travel API enables snapshot inspection, diffing, forking, and
// replaying of workflow runs. All methods follow the three-tier transport
// pattern used elsewhere in this package:
//
//   1. HTTP API (preferred, when smithers server is reachable)
//   2. Direct SQLite (read-only fallback for list/get operations)
//   3. Exec (smithers CLI fallback for all operations)

// ListSnapshots returns all snapshots for a workflow run, ordered by
// snapshot number ascending.
//
// Routes: HTTP GET /snapshot/list?runId={runID}
//       → SQLite SELECT from _smithers_snapshots
//       → exec smithers snapshot list {runID} --format json
func (c *Client) ListSnapshots(ctx context.Context, runID string) ([]Snapshot, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var snapshots []Snapshot
		err := c.httpGetJSON(ctx, "/snapshot/list?runId="+runID, &snapshots)
		if err == nil {
			return snapshots, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT id, run_id, snapshot_no, node_id, iteration, attempt,
			label, created_at, state_json, size_bytes, parent_id
			FROM _smithers_snapshots WHERE run_id = ? ORDER BY snapshot_no ASC`,
			runID)
		if err != nil {
			return nil, err
		}
		return scanSnapshots(rows)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "snapshot", "list", runID, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseSnapshotsJSON(out)
}

// GetSnapshot retrieves a single snapshot by its ID.
//
// Routes: HTTP GET /snapshot/{snapshotID}
//       → SQLite SELECT from _smithers_snapshots
//       → exec smithers snapshot get {snapshotID} --format json
func (c *Client) GetSnapshot(ctx context.Context, snapshotID string) (*Snapshot, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var snap Snapshot
		err := c.httpGetJSON(ctx, "/snapshot/"+snapshotID, &snap)
		if err == nil {
			return &snap, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT id, run_id, snapshot_no, node_id, iteration, attempt,
			label, created_at, state_json, size_bytes, parent_id
			FROM _smithers_snapshots WHERE id = ? LIMIT 1`,
			snapshotID)
		if err != nil {
			return nil, err
		}
		snaps, err := scanSnapshots(rows)
		if err != nil {
			return nil, err
		}
		if len(snaps) == 0 {
			return nil, fmt.Errorf("snapshot not found: %s", snapshotID)
		}
		return &snaps[0], nil
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "snapshot", "get", snapshotID, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseSnapshotJSON(out)
}

// DiffSnapshots computes the difference between two snapshots.
// The diff is ordered from → to, showing what changed between the two states.
//
// Routes: HTTP GET /snapshot/diff?from={fromID}&to={toID}
//       → exec smithers snapshot diff {fromID} {toID} --format json
//
// Note: Diffing is compute-intensive and is not available via direct SQLite
// (requires the TypeScript runtime). The exec fallback is always attempted
// if HTTP is unavailable.
func (c *Client) DiffSnapshots(ctx context.Context, fromID, toID string) (*SnapshotDiff, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var diff SnapshotDiff
		err := c.httpGetJSON(ctx,
			fmt.Sprintf("/snapshot/diff?from=%s&to=%s", fromID, toID),
			&diff)
		if err == nil {
			return &diff, nil
		}
	}

	// 2. No SQLite path — diffing requires the TS runtime.

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "snapshot", "diff", fromID, toID, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseSnapshotDiffJSON(out)
}

// ForkRun creates a new workflow run branched from the given snapshot.
// The forked run starts in a paused state at the snapshot's position and
// can be resumed independently of the original run.
//
// Routes: HTTP POST /snapshot/fork
//       → exec smithers fork {snapshotID} [options] --format json
func (c *Client) ForkRun(ctx context.Context, snapshotID string, opts ForkOptions) (*ForkReplayRun, error) {
	type forkRequest struct {
		SnapshotID   string            `json:"snapshotId"`
		WorkflowPath string            `json:"workflowPath,omitempty"`
		Inputs       map[string]string `json:"inputs,omitempty"`
		Label        string            `json:"label,omitempty"`
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var run ForkReplayRun
		err := c.httpPostJSON(ctx, "/snapshot/fork", forkRequest{
			SnapshotID:   snapshotID,
			WorkflowPath: opts.WorkflowPath,
			Inputs:       opts.Inputs,
			Label:        opts.Label,
		}, &run)
		if err == nil {
			return &run, nil
		}
	}

	// 2. No SQLite path — mutations require the server or CLI.

	// 3. Fall back to exec
	args := []string{"fork", snapshotID, "--format", "json"}
	if opts.WorkflowPath != "" {
		args = append(args, "--workflow", opts.WorkflowPath)
	}
	if opts.Label != "" {
		args = append(args, "--label", opts.Label)
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseForkReplayRunJSON(out)
}

// ReplayRun re-executes a workflow run starting from the given snapshot.
// Unlike ForkRun, replay re-runs the exact same workflow with the same
// inputs, allowing deterministic reproduction of past runs.
//
// Routes: HTTP POST /snapshot/replay
//       → exec smithers replay {snapshotID} [options] --format json
func (c *Client) ReplayRun(ctx context.Context, snapshotID string, opts ReplayOptions) (*ForkReplayRun, error) {
	type replayRequest struct {
		SnapshotID string  `json:"snapshotId"`
		StopAt     *string `json:"stopAt,omitempty"`
		Speed      float64 `json:"speed,omitempty"`
		Label      string  `json:"label,omitempty"`
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var run ForkReplayRun
		err := c.httpPostJSON(ctx, "/snapshot/replay", replayRequest{
			SnapshotID: snapshotID,
			StopAt:     opts.StopAt,
			Speed:      opts.Speed,
			Label:      opts.Label,
		}, &run)
		if err == nil {
			return &run, nil
		}
	}

	// 2. No SQLite path — mutations require the server or CLI.

	// 3. Fall back to exec
	args := []string{"replay", snapshotID, "--format", "json"}
	if opts.StopAt != nil {
		args = append(args, "--stop-at", *opts.StopAt)
	}
	if opts.Speed > 0 {
		args = append(args, "--speed", fmt.Sprintf("%g", opts.Speed))
	}
	if opts.Label != "" {
		args = append(args, "--label", opts.Label)
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseForkReplayRunJSON(out)
}

// --- SQL scan helpers ---

// scanSnapshots converts sql.Rows into a Snapshot slice.
func scanSnapshots(rows *sql.Rows) ([]Snapshot, error) {
	defer rows.Close()
	var result []Snapshot
	for rows.Next() {
		var s Snapshot
		var createdAtMs int64
		var parentID *string
		if err := rows.Scan(
			&s.ID, &s.RunID, &s.SnapshotNo, &s.NodeID,
			&s.Iteration, &s.Attempt, &s.Label,
			&createdAtMs, &s.StateJSON, &s.SizeBytes, &parentID,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = msToTime(createdAtMs)
		s.ParentID = parentID
		result = append(result, s)
	}
	return result, rows.Err()
}

// msToTime converts a Unix millisecond timestamp to time.Time.
func msToTime(ms int64) time.Time {
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond)).UTC()
}

// --- JSON parse helpers ---

// parseSnapshotsJSON parses exec output into a Snapshot slice.
func parseSnapshotsJSON(data []byte) ([]Snapshot, error) {
	var snaps []Snapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		return nil, fmt.Errorf("parse snapshots: %w", err)
	}
	return snaps, nil
}

// parseSnapshotJSON parses exec output into a single Snapshot.
func parseSnapshotJSON(data []byte) (*Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	return &snap, nil
}

// parseSnapshotDiffJSON parses exec output into a SnapshotDiff.
func parseSnapshotDiffJSON(data []byte) (*SnapshotDiff, error) {
	var diff SnapshotDiff
	if err := json.Unmarshal(data, &diff); err != nil {
		return nil, fmt.Errorf("parse snapshot diff: %w", err)
	}
	return &diff, nil
}

// parseForkReplayRunJSON parses exec output into a ForkReplayRun.
func parseForkReplayRunJSON(data []byte) (*ForkReplayRun, error) {
	var run ForkReplayRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("parse run: %w", err)
	}
	return &run, nil
}
