package smithers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fixtures ---

func fixtureSnapshot(id, runID string, no int) Snapshot {
	return Snapshot{
		ID:         id,
		RunID:      runID,
		SnapshotNo: no,
		NodeID:     "node-a",
		Iteration:  1,
		Attempt:    1,
		Label:      fmt.Sprintf("snapshot %d", no),
		CreatedAt:  time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		StateJSON:  `{"messages":[]}`,
		SizeBytes:  1024,
	}
}

func fixtureSnapshotDiff(fromID, toID string) SnapshotDiff {
	return SnapshotDiff{
		FromID: fromID,
		ToID:   toID,
		FromNo: 1,
		ToNo:   2,
		Entries: []DiffEntry{
			{Path: "messages[0].content", Op: "replace", OldValue: "hello", NewValue: "world"},
		},
		AddedCount:   0,
		RemovedCount: 0,
		ChangedCount: 1,
	}
}

func fixtureRun(id string) ForkReplayRun {
	return ForkReplayRun{
		ID:           id,
		WorkflowPath: "workflows/my-flow.tsx",
		Status:       "paused",
		StartedAt:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}
}

// --- ListSnapshots ---

func TestListSnapshots_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/snapshot/list", r.URL.Path)
		assert.Equal(t, "run-1", r.URL.Query().Get("runId"))
		assert.Equal(t, "GET", r.Method)

		writeEnvelope(t, w, []Snapshot{
			fixtureSnapshot("snap-1", "run-1", 1),
			fixtureSnapshot("snap-2", "run-1", 2),
		})
	})

	snaps, err := c.ListSnapshots(context.Background(), "run-1")
	require.NoError(t, err)
	require.Len(t, snaps, 2)
	assert.Equal(t, "snap-1", snaps[0].ID)
	assert.Equal(t, "run-1", snaps[0].RunID)
	assert.Equal(t, 1, snaps[0].SnapshotNo)
	assert.Equal(t, "snap-2", snaps[1].ID)
	assert.Equal(t, 2, snaps[1].SnapshotNo)
}

func TestListSnapshots_HTTP_Empty(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/snapshot/list", r.URL.Path)
		writeEnvelope(t, w, []Snapshot{})
	})

	snaps, err := c.ListSnapshots(context.Background(), "run-no-snaps")
	require.NoError(t, err)
	assert.Empty(t, snaps)
}

func TestListSnapshots_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "snapshot", args[0])
		assert.Equal(t, "list", args[1])
		assert.Equal(t, "run-1", args[2])
		assert.Equal(t, "--format", args[3])
		assert.Equal(t, "json", args[4])
		return json.Marshal([]Snapshot{
			fixtureSnapshot("snap-1", "run-1", 1),
		})
	})

	snaps, err := c.ListSnapshots(context.Background(), "run-1")
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	assert.Equal(t, "snap-1", snaps[0].ID)
}

func TestListSnapshots_Exec_Empty(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal([]Snapshot{})
	})

	snaps, err := c.ListSnapshots(context.Background(), "run-empty")
	require.NoError(t, err)
	assert.Empty(t, snaps)
}

func TestListSnapshots_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("run not found")
	})

	_, err := c.ListSnapshots(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run not found")
}

func TestListSnapshots_Exec_InvalidJSON(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	})

	_, err := c.ListSnapshots(context.Background(), "run-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshots")
}

// --- GetSnapshot ---

func TestGetSnapshot_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/snapshot/snap-42", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		writeEnvelope(t, w, fixtureSnapshot("snap-42", "run-1", 5))
	})

	snap, err := c.GetSnapshot(context.Background(), "snap-42")
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, "snap-42", snap.ID)
	assert.Equal(t, "run-1", snap.RunID)
	assert.Equal(t, 5, snap.SnapshotNo)
	assert.Equal(t, "node-a", snap.NodeID)
}

func TestGetSnapshot_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "snapshot", args[0])
		assert.Equal(t, "get", args[1])
		assert.Equal(t, "snap-42", args[2])
		assert.Equal(t, "--format", args[3])
		assert.Equal(t, "json", args[4])
		return json.Marshal(fixtureSnapshot("snap-42", "run-1", 5))
	})

	snap, err := c.GetSnapshot(context.Background(), "snap-42")
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, "snap-42", snap.ID)
	assert.Equal(t, 5, snap.SnapshotNo)
}

func TestGetSnapshot_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("snapshot not found: snap-999")
	})

	_, err := c.GetSnapshot(context.Background(), "snap-999")
	require.Error(t, err)
}

func TestGetSnapshot_Exec_InvalidJSON(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("{bad json"), nil
	})

	_, err := c.GetSnapshot(context.Background(), "snap-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshot")
}

func TestGetSnapshot_HTTP_WithParentID(t *testing.T) {
	parentID := "snap-parent"
	snap := fixtureSnapshot("snap-child", "run-1", 3)
	snap.ParentID = &parentID

	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, snap)
	})

	result, err := c.GetSnapshot(context.Background(), "snap-child")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ParentID)
	assert.Equal(t, "snap-parent", *result.ParentID)
}

// --- DiffSnapshots ---

func TestDiffSnapshots_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/snapshot/diff", r.URL.Path)
		assert.Equal(t, "snap-1", r.URL.Query().Get("from"))
		assert.Equal(t, "snap-2", r.URL.Query().Get("to"))
		assert.Equal(t, "GET", r.Method)

		writeEnvelope(t, w, fixtureSnapshotDiff("snap-1", "snap-2"))
	})

	diff, err := c.DiffSnapshots(context.Background(), "snap-1", "snap-2")
	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Equal(t, "snap-1", diff.FromID)
	assert.Equal(t, "snap-2", diff.ToID)
	assert.Equal(t, 1, diff.FromNo)
	assert.Equal(t, 2, diff.ToNo)
	require.Len(t, diff.Entries, 1)
	assert.Equal(t, "messages[0].content", diff.Entries[0].Path)
	assert.Equal(t, "replace", diff.Entries[0].Op)
	assert.Equal(t, 1, diff.ChangedCount)
}

func TestDiffSnapshots_HTTP_EmptyDiff(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, SnapshotDiff{
			FromID:  "snap-1",
			ToID:    "snap-1-copy",
			FromNo:  1,
			ToNo:    1,
			Entries: nil,
		})
	})

	diff, err := c.DiffSnapshots(context.Background(), "snap-1", "snap-1-copy")
	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Empty(t, diff.Entries)
	assert.Equal(t, 0, diff.ChangedCount)
}

func TestDiffSnapshots_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "snapshot", args[0])
		assert.Equal(t, "diff", args[1])
		assert.Equal(t, "snap-1", args[2])
		assert.Equal(t, "snap-2", args[3])
		assert.Equal(t, "--format", args[4])
		assert.Equal(t, "json", args[5])
		return json.Marshal(fixtureSnapshotDiff("snap-1", "snap-2"))
	})

	diff, err := c.DiffSnapshots(context.Background(), "snap-1", "snap-2")
	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Equal(t, "snap-1", diff.FromID)
	assert.Equal(t, "snap-2", diff.ToID)
}

func TestDiffSnapshots_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("snapshot snap-99 not found")
	})

	_, err := c.DiffSnapshots(context.Background(), "snap-1", "snap-99")
	require.Error(t, err)
}

func TestDiffSnapshots_Exec_InvalidJSON(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("not-json"), nil
	})

	_, err := c.DiffSnapshots(context.Background(), "snap-1", "snap-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshot diff")
}

func TestDiffSnapshots_MultipleEntries(t *testing.T) {
	bigDiff := SnapshotDiff{
		FromID: "snap-a",
		ToID:   "snap-b",
		FromNo: 1,
		ToNo:   3,
		Entries: []DiffEntry{
			{Path: "messages[0].role", Op: "replace", OldValue: "user", NewValue: "assistant"},
			{Path: "messages[1]", Op: "add", NewValue: map[string]any{"role": "user", "content": "hi"}},
			{Path: "toolCalls[0]", Op: "remove", OldValue: "old-call"},
		},
		AddedCount:   1,
		RemovedCount: 1,
		ChangedCount: 1,
	}

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(bigDiff)
	})

	diff, err := c.DiffSnapshots(context.Background(), "snap-a", "snap-b")
	require.NoError(t, err)
	require.Len(t, diff.Entries, 3)
	assert.Equal(t, 1, diff.AddedCount)
	assert.Equal(t, 1, diff.RemovedCount)
	assert.Equal(t, 1, diff.ChangedCount)
}

// --- ForkRun ---

func TestForkRun_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/snapshot/fork", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "snap-5", body["snapshotId"])

		writeEnvelope(t, w, fixtureRun("run-fork-1"))
	})

	run, err := c.ForkRun(context.Background(), "snap-5", ForkOptions{})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-fork-1", run.ID)
	assert.Equal(t, "paused", run.Status)
}

func TestForkRun_HTTP_WithOptions(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "snap-5", body["snapshotId"])
		assert.Equal(t, "workflows/alt.tsx", body["workflowPath"])
		assert.Equal(t, "my fork", body["label"])

		writeEnvelope(t, w, fixtureRun("run-fork-opts"))
	})

	run, err := c.ForkRun(context.Background(), "snap-5", ForkOptions{
		WorkflowPath: "workflows/alt.tsx",
		Label:        "my fork",
	})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-fork-opts", run.ID)
}

func TestForkRun_HTTP_WithInputs(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		inputs, ok := body["inputs"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "bar", inputs["foo"])

		writeEnvelope(t, w, fixtureRun("run-fork-inputs"))
	})

	run, err := c.ForkRun(context.Background(), "snap-5", ForkOptions{
		Inputs: map[string]string{"foo": "bar"},
	})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestForkRun_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "fork", args[0])
		assert.Equal(t, "snap-5", args[1])
		assert.Equal(t, "--format", args[2])
		assert.Equal(t, "json", args[3])
		return json.Marshal(fixtureRun("run-fork-exec"))
	})

	run, err := c.ForkRun(context.Background(), "snap-5", ForkOptions{})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-fork-exec", run.ID)
}

func TestForkRun_Exec_WithWorkflowAndLabel(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--workflow")
		assert.Contains(t, args, "alt.tsx")
		assert.Contains(t, args, "--label")
		assert.Contains(t, args, "test fork")
		return json.Marshal(fixtureRun("run-fork-wf"))
	})

	run, err := c.ForkRun(context.Background(), "snap-5", ForkOptions{
		WorkflowPath: "alt.tsx",
		Label:        "test fork",
	})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestForkRun_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("snapshot snap-bad not found")
	})

	_, err := c.ForkRun(context.Background(), "snap-bad", ForkOptions{})
	require.Error(t, err)
}

func TestForkRun_Exec_InvalidJSON(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("{bad"), nil
	})

	_, err := c.ForkRun(context.Background(), "snap-5", ForkOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse run")
}

// --- ReplayRun ---

func TestReplayRun_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/snapshot/replay", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "snap-5", body["snapshotId"])

		writeEnvelope(t, w, fixtureRun("run-replay-1"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-replay-1", run.ID)
}

func TestReplayRun_HTTP_WithStopAt(t *testing.T) {
	stopAt := "snap-7"
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "snap-5", body["snapshotId"])
		assert.Equal(t, "snap-7", body["stopAt"])

		writeEnvelope(t, w, fixtureRun("run-replay-stop"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{StopAt: &stopAt})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-replay-stop", run.ID)
}

func TestReplayRun_HTTP_WithSpeed(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.InDelta(t, 2.5, body["speed"], 0.001)

		writeEnvelope(t, w, fixtureRun("run-replay-fast"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{Speed: 2.5})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestReplayRun_HTTP_WithLabel(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "regression test", body["label"])

		writeEnvelope(t, w, fixtureRun("run-replay-label"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{Label: "regression test"})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestReplayRun_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "replay", args[0])
		assert.Equal(t, "snap-5", args[1])
		assert.Equal(t, "--format", args[2])
		assert.Equal(t, "json", args[3])
		return json.Marshal(fixtureRun("run-replay-exec"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-replay-exec", run.ID)
}

func TestReplayRun_Exec_WithStopAt(t *testing.T) {
	stopAt := "snap-8"
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--stop-at")
		assert.Contains(t, args, "snap-8")
		return json.Marshal(fixtureRun("run-replay-stop-exec"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{StopAt: &stopAt})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestReplayRun_Exec_WithSpeed(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--speed")
		assert.Contains(t, args, "0.5")
		return json.Marshal(fixtureRun("run-replay-speed"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{Speed: 0.5})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestReplayRun_Exec_WithAllOptions(t *testing.T) {
	stopAt := "snap-10"
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--stop-at")
		assert.Contains(t, args, "snap-10")
		assert.Contains(t, args, "--speed")
		assert.Contains(t, args, "3")
		assert.Contains(t, args, "--label")
		assert.Contains(t, args, "full replay")
		return json.Marshal(fixtureRun("run-replay-all"))
	})

	run, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{
		StopAt: &stopAt,
		Speed:  3.0,
		Label:  "full replay",
	})
	require.NoError(t, err)
	require.NotNil(t, run)
}

func TestReplayRun_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("cannot replay: run is still active")
	})

	_, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{})
	require.Error(t, err)
}

func TestReplayRun_Exec_InvalidJSON(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("not-valid-json"), nil
	})

	_, err := c.ReplayRun(context.Background(), "snap-5", ReplayOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse run")
}

// --- msToTime ---

func TestMsToTime(t *testing.T) {
	// 0 ms should be Unix epoch
	assert.Equal(t, time.Unix(0, 0).UTC(), msToTime(0))

	// 1000ms = 1 second
	assert.Equal(t, time.Unix(1, 0).UTC(), msToTime(1000))

	// 1500ms = 1s + 500ms
	expected := time.Unix(1, 500*int64(time.Millisecond)).UTC()
	assert.Equal(t, expected, msToTime(1500))

	// Known timestamp
	known := int64(1743465600000) // 2025-04-01 00:00:00 UTC
	result := msToTime(known)
	assert.Equal(t, 2025, result.Year())
	assert.Equal(t, time.April, result.Month())
	assert.Equal(t, 1, result.Day())
}

// --- parseSnapshotsJSON ---

func TestParseSnapshotsJSON_Valid(t *testing.T) {
	data, err := json.Marshal([]Snapshot{
		fixtureSnapshot("s1", "r1", 1),
		fixtureSnapshot("s2", "r1", 2),
	})
	require.NoError(t, err)

	snaps, err := parseSnapshotsJSON(data)
	require.NoError(t, err)
	require.Len(t, snaps, 2)
	assert.Equal(t, "s1", snaps[0].ID)
	assert.Equal(t, "s2", snaps[1].ID)
}

func TestParseSnapshotsJSON_Invalid(t *testing.T) {
	_, err := parseSnapshotsJSON([]byte("garbage"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshots")
}

func TestParseSnapshotsJSON_EmptyArray(t *testing.T) {
	snaps, err := parseSnapshotsJSON([]byte("[]"))
	require.NoError(t, err)
	assert.Empty(t, snaps)
}

// --- parseSnapshotJSON ---

func TestParseSnapshotJSON_Valid(t *testing.T) {
	snap := fixtureSnapshot("s1", "r1", 1)
	data, err := json.Marshal(snap)
	require.NoError(t, err)

	result, err := parseSnapshotJSON(data)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "s1", result.ID)
}

func TestParseSnapshotJSON_Invalid(t *testing.T) {
	_, err := parseSnapshotJSON([]byte("{bad"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshot")
}

// --- parseSnapshotDiffJSON ---

func TestParseSnapshotDiffJSON_Valid(t *testing.T) {
	d := fixtureSnapshotDiff("s1", "s2")
	data, err := json.Marshal(d)
	require.NoError(t, err)

	result, err := parseSnapshotDiffJSON(data)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "s1", result.FromID)
	assert.Equal(t, "s2", result.ToID)
}

func TestParseSnapshotDiffJSON_Invalid(t *testing.T) {
	_, err := parseSnapshotDiffJSON([]byte("!!!"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshot diff")
}

// --- parseRunJSON ---

func TestParseForkReplayRunJSON_Valid(t *testing.T) {
	r := fixtureRun("run-1")
	data, err := json.Marshal(r)
	require.NoError(t, err)

	result, err := parseForkReplayRunJSON(data)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "run-1", result.ID)
	assert.Equal(t, "paused", result.Status)
}

func TestParseForkReplayRunJSON_Invalid(t *testing.T) {
	_, err := parseForkReplayRunJSON([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse run")
}

func TestParseForkReplayRunJSON_WithForkedFrom(t *testing.T) {
	snapID := "snap-origin"
	r := fixtureRun("run-forked")
	r.ForkedFrom = &snapID

	data, err := json.Marshal(r)
	require.NoError(t, err)

	result, err := parseForkReplayRunJSON(data)
	require.NoError(t, err)
	require.NotNil(t, result.ForkedFrom)
	assert.Equal(t, "snap-origin", *result.ForkedFrom)
}

// --- Type field coverage ---

func TestSnapshotType_Fields(t *testing.T) {
	parentID := "snap-p"
	s := Snapshot{
		ID:         "s1",
		RunID:      "r1",
		SnapshotNo: 3,
		NodeID:     "node-b",
		Iteration:  2,
		Attempt:    1,
		Label:      "after tool: edit",
		CreatedAt:  time.Now(),
		StateJSON:  `{"x":1}`,
		SizeBytes:  512,
		ParentID:   &parentID,
	}
	assert.Equal(t, "s1", s.ID)
	assert.Equal(t, 3, s.SnapshotNo)
	assert.NotNil(t, s.ParentID)
}

func TestDiffEntryType_OpsAreStrings(t *testing.T) {
	ops := []string{"add", "remove", "replace"}
	for _, op := range ops {
		e := DiffEntry{Op: op, Path: "x"}
		assert.Equal(t, op, e.Op)
	}
}

func TestSnapshotDiffType_Counts(t *testing.T) {
	d := SnapshotDiff{
		FromID:       "a",
		ToID:         "b",
		AddedCount:   3,
		RemovedCount: 1,
		ChangedCount: 2,
	}
	assert.Equal(t, 3, d.AddedCount)
	assert.Equal(t, 1, d.RemovedCount)
	assert.Equal(t, 2, d.ChangedCount)
}

func TestForkOptionsType_ZeroValue(t *testing.T) {
	var opts ForkOptions
	assert.Empty(t, opts.WorkflowPath)
	assert.Nil(t, opts.Inputs)
	assert.Empty(t, opts.Label)
}

func TestReplayOptionsType_ZeroValue(t *testing.T) {
	var opts ReplayOptions
	assert.Nil(t, opts.StopAt)
	assert.Equal(t, float64(0), opts.Speed)
	assert.Empty(t, opts.Label)
}

func TestForkReplayRunType_ZeroValue(t *testing.T) {
	var r ForkReplayRun
	assert.Empty(t, r.ID)
	assert.Nil(t, r.Label)
	assert.Nil(t, r.FinishedAt)
	assert.Nil(t, r.ForkedFrom)
}

func TestForkReplayRunType_FinishedAt(t *testing.T) {
	now := time.Now()
	r := fixtureRun("r1")
	r.FinishedAt = &now
	assert.NotNil(t, r.FinishedAt)
}

// --- JSON round-trip ---

func TestSnapshot_JSONRoundTrip(t *testing.T) {
	parentID := "snap-parent"
	original := Snapshot{
		ID:         "snap-rt",
		RunID:      "run-rt",
		SnapshotNo: 7,
		NodeID:     "node-rt",
		Iteration:  3,
		Attempt:    2,
		Label:      "rt label",
		CreatedAt:  time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		StateJSON:  `{"messages":[{"role":"user","content":"hello"}]}`,
		SizeBytes:  2048,
		ParentID:   &parentID,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored Snapshot
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.RunID, restored.RunID)
	assert.Equal(t, original.SnapshotNo, restored.SnapshotNo)
	assert.Equal(t, original.NodeID, restored.NodeID)
	assert.Equal(t, original.Label, restored.Label)
	assert.Equal(t, original.StateJSON, restored.StateJSON)
	assert.Equal(t, original.SizeBytes, restored.SizeBytes)
	require.NotNil(t, restored.ParentID)
	assert.Equal(t, *original.ParentID, *restored.ParentID)
}

func TestSnapshotDiff_JSONRoundTrip(t *testing.T) {
	original := SnapshotDiff{
		FromID: "from-snap",
		ToID:   "to-snap",
		FromNo: 1,
		ToNo:   5,
		Entries: []DiffEntry{
			{Path: "state.x", Op: "add", NewValue: 42},
			{Path: "state.y", Op: "remove", OldValue: "old"},
		},
		AddedCount:   1,
		RemovedCount: 1,
		ChangedCount: 0,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored SnapshotDiff
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, original.FromID, restored.FromID)
	assert.Equal(t, original.ToID, restored.ToID)
	assert.Len(t, restored.Entries, 2)
	assert.Equal(t, original.AddedCount, restored.AddedCount)
}

func TestForkReplayRun_JSONRoundTrip(t *testing.T) {
	label := "my run"
	forkedFrom := "snap-origin"
	finishedAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	original := ForkReplayRun{
		ID:           "run-rt",
		WorkflowPath: "wf/test.tsx",
		Status:       "completed",
		Label:        &label,
		StartedAt:    time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		FinishedAt:   &finishedAt,
		ForkedFrom:   &forkedFrom,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored ForkReplayRun
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.WorkflowPath, restored.WorkflowPath)
	assert.Equal(t, original.Status, restored.Status)
	require.NotNil(t, restored.Label)
	assert.Equal(t, *original.Label, *restored.Label)
	require.NotNil(t, restored.ForkedFrom)
	assert.Equal(t, *original.ForkedFrom, *restored.ForkedFrom)
}
