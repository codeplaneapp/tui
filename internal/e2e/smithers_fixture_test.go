package e2e_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func seedSmithersSnapshotsFixture(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "smithers.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open smithers fixture db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	stmts := []string{
		`CREATE TABLE _smithers_runs (
			run_id TEXT PRIMARY KEY,
			workflow_name TEXT NOT NULL,
			workflow_path TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at_ms INTEGER,
			finished_at_ms INTEGER,
			error_json TEXT
		)`,
		`CREATE TABLE _smithers_nodes (
			run_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			label TEXT,
			iteration INTEGER NOT NULL,
			state TEXT NOT NULL,
			last_attempt INTEGER,
			updated_at_ms INTEGER
		)`,
		`CREATE TABLE _smithers_snapshots (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			snapshot_no INTEGER NOT NULL,
			node_id TEXT NOT NULL,
			iteration INTEGER NOT NULL,
			attempt INTEGER NOT NULL,
			label TEXT,
			created_at INTEGER NOT NULL,
			state_json TEXT,
			size_bytes INTEGER,
			parent_id TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create smithers fixture schema: %v", err)
		}
	}

	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO _smithers_runs
			(run_id, workflow_name, workflow_path, status, started_at_ms, finished_at_ms, error_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"snapdemo",
		"snapshot-demo",
		"workflows/snapshot-demo.tsx",
		"running",
		now.Add(-3*time.Minute).UnixMilli(),
		nil,
		nil,
	); err != nil {
		t.Fatalf("insert smithers fixture run: %v", err)
	}

	nodeInserts := []struct {
		nodeID    string
		label     string
		iteration int
		state     string
		attempt   int
		updated   int64
	}{
		{nodeID: "fetch-deps", label: "Fetch deps", iteration: 1, state: "finished", attempt: 1, updated: now.Add(-2 * time.Minute).UnixMilli()},
		{nodeID: "review-auth", label: "Review auth", iteration: 1, state: "running", attempt: 1, updated: now.Add(-30 * time.Second).UnixMilli()},
	}
	for _, node := range nodeInserts {
		if _, err := db.Exec(
			`INSERT INTO _smithers_nodes
				(run_id, node_id, label, iteration, state, last_attempt, updated_at_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"snapdemo",
			node.nodeID,
			node.label,
			node.iteration,
			node.state,
			node.attempt,
			node.updated,
		); err != nil {
			t.Fatalf("insert smithers fixture node: %v", err)
		}
	}

	snapshotInserts := []struct {
		id        string
		no        int
		nodeID    string
		label     string
		createdAt time.Time
		stateJSON string
		sizeBytes int64
	}{
		{id: "snap-1", no: 1, nodeID: "workflow-start", label: "Workflow started", createdAt: now.Add(-2 * time.Minute), stateJSON: `{"step":"start"}`, sizeBytes: 128},
		{id: "snap-2", no: 2, nodeID: "fetch-deps", label: "Fetch deps complete", createdAt: now.Add(-90 * time.Second), stateJSON: `{"step":"fetch"}`, sizeBytes: 256},
		{id: "snap-3", no: 3, nodeID: "review-auth", label: "Review auth running", createdAt: now.Add(-30 * time.Second), stateJSON: `{"step":"review"}`, sizeBytes: 384},
	}
	for _, snap := range snapshotInserts {
		if _, err := db.Exec(
			`INSERT INTO _smithers_snapshots
				(id, run_id, snapshot_no, node_id, iteration, attempt, label, created_at, state_json, size_bytes, parent_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snap.id,
			"snapdemo",
			snap.no,
			snap.nodeID,
			1,
			1,
			snap.label,
			snap.createdAt.UnixMilli(),
			snap.stateJSON,
			snap.sizeBytes,
			nil,
		); err != nil {
			t.Fatalf("insert smithers fixture snapshot: %v", err)
		}
	}

	return dbPath
}
