package views

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

// newTimelineView creates a TimelineView with a stub smithers.Client that will
// never reach a real server.  Tests drive the model by calling Update directly.
func newTimelineView(runID string) *TimelineView {
	c := smithers.NewClient() // no-op client; no server
	return NewTimelineView(c, runID)
}

// makeSnapshot is a convenience constructor for a Snapshot fixture.
func makeSnapshot(id, runID, nodeID, label string, no int, createdAt time.Time) smithers.Snapshot {
	return smithers.Snapshot{
		ID:         id,
		RunID:      runID,
		SnapshotNo: no,
		NodeID:     nodeID,
		Label:      label,
		CreatedAt:  createdAt,
		SizeBytes:  1024,
	}
}

// makeSnapshots creates a slice of n test snapshots for the given runID.
func makeSnapshots(runID string, n int) []smithers.Snapshot {
	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	snaps := make([]smithers.Snapshot, n)
	for i := 0; i < n; i++ {
		snaps[i] = makeSnapshot(
			fmt.Sprintf("snap-%03d", i+1),
			runID,
			fmt.Sprintf("node-%d", i+1),
			fmt.Sprintf("Step %d complete", i+1),
			i+1,
			base.Add(time.Duration(i)*10*time.Second),
		)
	}
	return snaps
}

// makeDiff returns a minimal SnapshotDiff for two consecutive snapshots.
func makeDiff(fromID, toID string, fromNo, toNo, added, removed, changed int) *smithers.SnapshotDiff {
	return &smithers.SnapshotDiff{
		FromID:       fromID,
		ToID:         toID,
		FromNo:       fromNo,
		ToNo:         toNo,
		AddedCount:   added,
		RemovedCount: removed,
		ChangedCount: changed,
		Entries: []smithers.DiffEntry{
			{Path: "messages[0].content", Op: "replace",
				OldValue: "old value", NewValue: "new value"},
		},
	}
}

func configureTimelineObservability(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))
}

func newTimelineHTTPClient(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *smithers.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	client := smithers.NewClient(
		smithers.WithAPIURL(server.URL),
		smithers.WithHTTPClient(server.Client()),
	)
	client.SetServerUp(true)
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func writeTimelineEnvelope(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"data": payload,
	}))
}

func requireRecentSpanAttrs(t *testing.T, name string) map[string]any {
	t.Helper()

	spans := observability.RecentSpans(20)
	for i := len(spans) - 1; i >= 0; i-- {
		if spans[i].Name == name {
			return spans[i].Attributes
		}
	}

	t.Fatalf("span %q not found in %+v", name, spans)
	return nil
}

// pressKey simulates a key press on the view and returns the updated view.
func pressKey(v View, key string) (View, tea.Cmd) {
	return v.Update(tea.KeyPressMsg{Code: rune(key[0])})
}

// pressSpecialKey simulates a special key press (e.g. arrow keys) on the view.
func pressSpecialKey(v View, code rune) (View, tea.Cmd) {
	return v.Update(tea.KeyPressMsg{Code: code})
}

// --- Interface compliance ---

func TestTimelineView_ImplementsView(t *testing.T) {
	var _ View = (*TimelineView)(nil)
}

// --- Constructor defaults ---

func TestNewTimelineView_Defaults(t *testing.T) {
	v := newTimelineView("run-001")
	assert.Equal(t, "run-001", v.runID)
	assert.True(t, v.loading, "should start loading")
	assert.True(t, v.follow, "follow mode should default to on")
	assert.Equal(t, 0, v.cursor)
	assert.Equal(t, 0, v.focusPane)
	assert.NotNil(t, v.diffs)
	assert.NotNil(t, v.diffErrs)
	assert.True(t, v.detailDirty)
}

// --- Init ---

func TestTimelineView_Init_ReturnsCmd(t *testing.T) {
	v := newTimelineView("run-init")
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil batch cmd")
}

// --- Update: snapshot loading ---

func TestTimelineView_Update_SnapshotsLoaded(t *testing.T) {
	v := newTimelineView("run-abc")
	snaps := makeSnapshots("run-abc", 3)

	updated, cmd := v.Update(timelineLoadedMsg{snapshots: snaps})
	require.NotNil(t, updated)

	tv := updated.(*TimelineView)
	assert.False(t, tv.loading)
	assert.Len(t, tv.snapshots, 3)
	// follow=true → cursor should be at last snapshot
	assert.Equal(t, 2, tv.cursor)
	// cmd may be non-nil (prefetchAdjacentDiff if cursor > 0)
	_ = cmd
}

func TestTimelineView_Update_SnapshotsLoaded_FollowOff(t *testing.T) {
	v := newTimelineView("run-flw")
	v.follow = false
	snaps := makeSnapshots("run-flw", 5)

	updated, _ := v.Update(timelineLoadedMsg{snapshots: snaps})
	tv := updated.(*TimelineView)
	// follow is off → cursor stays at 0
	assert.Equal(t, 0, tv.cursor)
}

func TestTimelineView_Update_SnapshotsError(t *testing.T) {
	v := newTimelineView("run-err")
	loadErr := errors.New("connection refused")

	updated, cmd := v.Update(timelineErrorMsg{err: loadErr})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	tv := updated.(*TimelineView)
	assert.False(t, tv.loading)
	assert.Equal(t, loadErr, tv.loadingErr)
	assert.Nil(t, tv.snapshots)
}

func TestTimelineView_Update_SnapshotsEmpty(t *testing.T) {
	v := newTimelineView("run-empty")

	updated, _ := v.Update(timelineLoadedMsg{snapshots: []smithers.Snapshot{}})
	tv := updated.(*TimelineView)
	assert.False(t, tv.loading)
	assert.Empty(t, tv.snapshots)
	// cursor stays at 0
	assert.Equal(t, 0, tv.cursor)
}

// --- Update: diff loading ---

func TestTimelineView_Update_DiffLoaded_CachedByKey(t *testing.T) {
	v := newTimelineView("run-diff")
	snaps := makeSnapshots("run-diff", 3)
	v.snapshots = snaps
	v.cursor = 1

	diff := makeDiff(snaps[0].ID, snaps[1].ID, 1, 2, 1, 0, 0)
	key := snaps[0].ID + ":" + snaps[1].ID

	updated, cmd := v.Update(timelineDiffLoadedMsg{key: key, diff: diff})
	assert.Nil(t, cmd)

	tv := updated.(*TimelineView)
	assert.False(t, tv.loadingDiff)
	assert.Equal(t, diff, tv.diffs[key])
}

func TestTimelineView_Update_DiffError_CachedByKey(t *testing.T) {
	v := newTimelineView("run-differr")
	snaps := makeSnapshots("run-differr", 2)
	v.snapshots = snaps
	v.cursor = 1

	diffErr := errors.New("diff computation failed")
	key := snaps[0].ID + ":" + snaps[1].ID

	updated, _ := v.Update(timelineDiffErrorMsg{key: key, err: diffErr})
	tv := updated.(*TimelineView)
	assert.False(t, tv.loadingDiff)
	assert.Equal(t, diffErr, tv.diffErrs[key])
}

func TestTimelineView_Update_DiffNotRefetched_IfCached(t *testing.T) {
	v := newTimelineView("run-diffcache")
	snaps := makeSnapshots("run-diffcache", 3)
	v.snapshots = snaps
	v.cursor = 1

	diff := makeDiff(snaps[0].ID, snaps[1].ID, 1, 2, 1, 0, 0)
	key := snaps[0].ID + ":" + snaps[1].ID
	v.diffs[key] = diff // pre-cache the diff

	// prefetchAdjacentDiff should return nil since diff is already cached.
	cmd := v.prefetchAdjacentDiff()
	assert.Nil(t, cmd, "prefetchAdjacentDiff should return nil when diff is already cached")
}

func TestTimelineView_Update_DiffNotRefetched_IfErrorCached(t *testing.T) {
	v := newTimelineView("run-diffcacherr")
	snaps := makeSnapshots("run-diffcacherr", 2)
	v.snapshots = snaps
	v.cursor = 1

	key := snaps[0].ID + ":" + snaps[1].ID
	v.diffErrs[key] = errors.New("cached error")

	cmd := v.prefetchAdjacentDiff()
	assert.Nil(t, cmd, "prefetchAdjacentDiff should return nil when error is cached")
}

// --- Update: fork/replay confirmation flow ---

func TestTimelineView_Update_FSetsPendingFork(t *testing.T) {
	v := newTimelineView("run-fork")
	v.snapshots = makeSnapshots("run-fork", 3)
	v.cursor = 1

	updated, cmd := pressKey(v, "f")
	assert.Nil(t, cmd)

	tv := updated.(*TimelineView)
	assert.Equal(t, pendingFork, tv.pendingAction)
	assert.Nil(t, tv.pendingResult)
	assert.Nil(t, tv.pendingErr)
}

func TestTimelineView_Update_FNoOpWhenNoSnapshots(t *testing.T) {
	v := newTimelineView("run-fnosnap")

	updated, _ := pressKey(v, "f")
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction, "f should be no-op with empty snapshots")
}

func TestTimelineView_Update_RSetsPendingReplay(t *testing.T) {
	v := newTimelineView("run-replay")
	v.snapshots = makeSnapshots("run-replay", 3)
	v.cursor = 1

	updated, _ := pressKey(v, "r")
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingReplay, tv.pendingAction)
}

func TestTimelineView_Update_YConfirmsFork_DispatchesCmd(t *testing.T) {
	v := newTimelineView("run-yconfirm")
	v.snapshots = makeSnapshots("run-yconfirm", 2)
	v.cursor = 0
	v.pendingAction = pendingFork

	updated, cmd := pressKey(v, "y")
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction, "y should clear pendingAction")
	assert.NotNil(t, cmd, "y should return a dispatch command")
}

func TestTimelineView_Update_OtherKeyCancelsConfirmation(t *testing.T) {
	v := newTimelineView("run-cancel")
	v.snapshots = makeSnapshots("run-cancel", 2)
	v.cursor = 0
	v.pendingAction = pendingFork

	updated, cmd := pressKey(v, "n")
	assert.Nil(t, cmd, "n should return nil cmd (cancel)")

	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction, "n should cancel the pending action")
}

func TestTimelineView_Update_EscCancelsConfirmation(t *testing.T) {
	v := newTimelineView("run-esccancel")
	v.snapshots = makeSnapshots("run-esccancel", 2)
	v.cursor = 0
	v.pendingAction = pendingReplay

	updated, cmd := pressSpecialKey(v, tea.KeyEscape)
	assert.Nil(t, cmd)
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
}

func TestTimelineView_Update_ForkDone_ClearsState(t *testing.T) {
	v := newTimelineView("run-forkdone")
	v.pendingAction = pendingFork

	forkRun := &smithers.ForkReplayRun{
		ID:        "new-fork-run-id",
		Status:    "active",
		StartedAt: time.Now(),
	}

	updated, cmd := v.Update(timelineForkDoneMsg{run: forkRun})
	// cmd is now non-nil: it carries the success toast.
	assert.NotNil(t, cmd, "ForkDone should return a toast cmd")

	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
	assert.Equal(t, forkRun, tv.pendingResult)
	assert.Nil(t, tv.pendingErr)
}

func TestTimelineView_Update_ReplayDone_ClearsState(t *testing.T) {
	v := newTimelineView("run-replaydone")
	v.pendingAction = pendingReplay

	replayRun := &smithers.ForkReplayRun{
		ID:        "replay-run-id",
		Status:    "paused",
		StartedAt: time.Now(),
	}

	updated, cmd := v.Update(timelineReplayDoneMsg{run: replayRun})
	// cmd is now non-nil: it carries the success toast.
	assert.NotNil(t, cmd, "ReplayDone should return a toast cmd")

	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
	assert.Equal(t, replayRun, tv.pendingResult)
}

func TestTimelineView_Update_ActionError_ClearsState(t *testing.T) {
	v := newTimelineView("run-actionerr")
	v.pendingAction = pendingFork

	actionErr := errors.New("fork failed: server busy")
	updated, cmd := v.Update(timelineActionErrorMsg{err: actionErr})
	// cmd is now non-nil: it carries the error toast.
	assert.NotNil(t, cmd, "ActionError should return a toast cmd")

	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
	assert.Nil(t, tv.pendingResult)
	assert.Equal(t, actionErr, tv.pendingErr)
}

// --- Update: keyboard navigation ---

func TestTimelineView_Update_DownMoveCursor(t *testing.T) {
	v := newTimelineView("run-down")
	v.snapshots = makeSnapshots("run-down", 5)
	v.cursor = 1
	v.follow = false

	updated, _ := pressSpecialKey(v, tea.KeyDown)
	tv := updated.(*TimelineView)
	assert.Equal(t, 2, tv.cursor)
	assert.False(t, tv.follow, "manual nav should turn follow off")
}

func TestTimelineView_Update_UpMoveCursor(t *testing.T) {
	v := newTimelineView("run-up")
	v.snapshots = makeSnapshots("run-up", 5)
	v.cursor = 3
	v.follow = false

	updated, _ := pressSpecialKey(v, tea.KeyUp)
	tv := updated.(*TimelineView)
	assert.Equal(t, 2, tv.cursor)
}

func TestTimelineView_Update_UpMoveCursor_NoBelowZero(t *testing.T) {
	v := newTimelineView("run-upzero")
	v.snapshots = makeSnapshots("run-upzero", 3)
	v.cursor = 0
	v.follow = false

	updated, _ := pressSpecialKey(v, tea.KeyUp)
	tv := updated.(*TimelineView)
	assert.Equal(t, 0, tv.cursor, "cursor should not go below zero")
}

func TestTimelineView_Update_DownNoopAtEnd(t *testing.T) {
	v := newTimelineView("run-downend")
	v.snapshots = makeSnapshots("run-downend", 3)
	v.cursor = 2 // last item
	v.follow = false

	updated, _ := pressSpecialKey(v, tea.KeyDown)
	tv := updated.(*TimelineView)
	assert.Equal(t, 2, tv.cursor, "cursor should not exceed last index")
}

func TestTimelineView_Update_GGoToFirst(t *testing.T) {
	v := newTimelineView("run-g")
	v.snapshots = makeSnapshots("run-g", 5)
	v.cursor = 3
	v.follow = false

	updated, _ := pressKey(v, "g")
	tv := updated.(*TimelineView)
	assert.Equal(t, 0, tv.cursor)
	assert.False(t, tv.follow)
}

func TestTimelineView_Update_ShiftGGoToLast(t *testing.T) {
	v := newTimelineView("run-G")
	v.snapshots = makeSnapshots("run-G", 5)
	v.cursor = 1

	updated, _ := pressKey(v, "G")
	tv := updated.(*TimelineView)
	assert.Equal(t, 4, tv.cursor)
}

func TestTimelineView_Update_EscPopsView(t *testing.T) {
	v := newTimelineView("run-esc")
	v.width = 80
	v.height = 24

	_, cmd := pressSpecialKey(v, tea.KeyEscape)
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

func TestTimelineView_Update_QPopsView(t *testing.T) {
	v := newTimelineView("run-q")
	v.width = 80
	v.height = 24

	_, cmd := pressKey(v, "q")
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "'q' should emit PopViewMsg")
}

func TestTimelineView_Update_FollowTurnedOffOnManualNav(t *testing.T) {
	v := newTimelineView("run-followoff")
	v.snapshots = makeSnapshots("run-followoff", 5)
	v.cursor = 4
	v.follow = true

	updated, _ := pressSpecialKey(v, tea.KeyUp)
	tv := updated.(*TimelineView)
	assert.False(t, tv.follow, "moving cursor manually should turn follow off")
}

func TestTimelineView_Update_RRefreshesSnapshots(t *testing.T) {
	v := newTimelineView("run-refresh")
	v.snapshots = makeSnapshots("run-refresh", 3)

	updated, cmd := pressKey(v, "R")
	tv := updated.(*TimelineView)
	assert.True(t, tv.loading, "'R' should set loading=true")
	assert.NotNil(t, cmd, "'R' should return a fetch command")
}

func TestTimelineView_Update_FocusPaneLeft(t *testing.T) {
	v := newTimelineView("run-focusleft")
	v.snapshots = makeSnapshots("run-focusleft", 3)
	v.focusPane = 1
	v.width = 100

	updated, _ := pressSpecialKey(v, tea.KeyLeft)
	tv := updated.(*TimelineView)
	assert.Equal(t, 0, tv.focusPane)
}

func TestTimelineView_Update_FocusPaneRight_WideTerminal(t *testing.T) {
	v := newTimelineView("run-focusright")
	v.snapshots = makeSnapshots("run-focusright", 3)
	v.focusPane = 0
	v.width = 100 // >= 80 → allows right focus

	updated, _ := pressSpecialKey(v, tea.KeyRight)
	tv := updated.(*TimelineView)
	assert.Equal(t, 1, tv.focusPane)
}

func TestTimelineView_Update_FocusPaneRight_NarrowTerminal(t *testing.T) {
	v := newTimelineView("run-focusnarrow")
	v.snapshots = makeSnapshots("run-focusnarrow", 3)
	v.focusPane = 0
	v.width = 60 // < 80 → right focus not allowed

	updated, _ := pressSpecialKey(v, tea.KeyRight)
	tv := updated.(*TimelineView)
	assert.Equal(t, 0, tv.focusPane, "right focus should not activate on narrow terminal")
}

func TestTimelineView_Update_DetailPaneScrollWhenFocused(t *testing.T) {
	v := newTimelineView("run-detailscroll")
	v.snapshots = makeSnapshots("run-detailscroll", 3)
	v.focusPane = 1 // right pane focused
	v.detailScroll = 3

	updated, _ := pressSpecialKey(v, tea.KeyDown)
	tv := updated.(*TimelineView)
	assert.Equal(t, 4, tv.detailScroll, "down in focused right pane should scroll detail")

	updated2, _ := pressSpecialKey(tv, tea.KeyUp)
	tv2 := updated2.(*TimelineView)
	assert.Equal(t, 3, tv2.detailScroll, "up in focused right pane should scroll detail up")
}

func TestTimelineView_Update_DetailScrollNotBelowZero(t *testing.T) {
	v := newTimelineView("run-noscrollneg")
	v.snapshots = makeSnapshots("run-noscrollneg", 3)
	v.focusPane = 1
	v.detailScroll = 0

	updated, _ := pressSpecialKey(v, tea.KeyUp)
	tv := updated.(*TimelineView)
	assert.Equal(t, 0, tv.detailScroll, "detail scroll should not go below 0")
}

// --- Update: window resize via SetSize ---

func TestTimelineView_SetSize(t *testing.T) {
	v := newTimelineView("run-setsize")
	v.SetSize(120, 40)
	assert.Equal(t, 120, v.width)
	assert.Equal(t, 40, v.height)
	assert.True(t, v.detailDirty)
}

func TestTimelineView_Update_WindowSizeMsg(t *testing.T) {
	v := newTimelineView("run-wsize")
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	tv := updated.(*TimelineView)
	assert.Equal(t, 100, tv.width)
	assert.Equal(t, 30, tv.height)
}

// --- Update: refresh tick ---

func TestTimelineView_Update_RefreshTick_ReturnsFetchCmd(t *testing.T) {
	v := newTimelineView("run-tick")
	v.snapshots = makeSnapshots("run-tick", 2)

	updated, cmd := v.Update(timelineRefreshTickMsg{})
	require.NotNil(t, updated)
	// cmd should be a batch of fetchSnapshots + refreshTick
	assert.NotNil(t, cmd)
}

// --- View() rendering ---

func TestTimelineView_View_LoadingState(t *testing.T) {
	v := newTimelineView("run-loading")
	v.SetSize(80, 24)
	out := v.View()
	assert.Contains(t, out, "Loading")
}

func TestTimelineView_View_ErrorState(t *testing.T) {
	v := newTimelineView("run-errview")
	v.SetSize(80, 24)
	v.loading = false
	v.loadingErr = errors.New("server unavailable")
	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "server unavailable")
}

func TestTimelineView_View_EmptyState(t *testing.T) {
	v := newTimelineView("run-empty-view")
	v.SetSize(80, 24)
	v.loading = false
	v.snapshots = []smithers.Snapshot{}
	out := v.View()
	assert.Contains(t, out, "No snapshots")
}

func TestTimelineView_View_ContainsRunID(t *testing.T) {
	v := newTimelineView("run-viewtest")
	v.SetSize(80, 24)
	v.loading = false
	v.snapshots = []smithers.Snapshot{}
	out := v.View()
	// runID is truncated to 8 chars in the header
	assert.Contains(t, out, "run-view")
}

func TestTimelineView_View_RendersSnapshots(t *testing.T) {
	v := newTimelineView("run-render")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-render", 3)
	v.cursor = 0
	out := v.View()
	// Header should be present
	assert.Contains(t, out, "Snapshots")
	// Snapshot list section heading
	assert.Contains(t, out, "Snapshots")
	// At least one marker (encircled 1)
	assert.Contains(t, out, "①")
}

func TestTimelineView_View_ConfirmationPrompt_Fork(t *testing.T) {
	v := newTimelineView("run-forkprompt")
	v.SetSize(80, 24)
	v.loading = false
	v.snapshots = makeSnapshots("run-forkprompt", 3)
	v.cursor = 1
	v.pendingAction = pendingFork
	out := v.View()
	assert.Contains(t, out, "Fork")
	assert.Contains(t, out, "[y/N]")
}

func TestTimelineView_View_ConfirmationPrompt_Replay(t *testing.T) {
	v := newTimelineView("run-replayprompt")
	v.SetSize(80, 24)
	v.loading = false
	v.snapshots = makeSnapshots("run-replayprompt", 3)
	v.cursor = 2
	v.pendingAction = pendingReplay
	out := v.View()
	assert.Contains(t, out, "Replay")
	assert.Contains(t, out, "[y/N]")
}

func TestTimelineView_View_RailMarkers_CircledNumbers(t *testing.T) {
	v := newTimelineView("run-rail")
	v.SetSize(200, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-rail", 5)
	v.cursor = 0
	out := v.View()
	// All 5 encircled numbers should appear
	for i := 1; i <= 5; i++ {
		marker := snapshotMarker(i)
		assert.Contains(t, out, marker,
			"marker %d (%s) should appear in view", i, marker)
	}
}

func TestTimelineView_View_RailMarkers_BeyondTwenty(t *testing.T) {
	v := newTimelineView("run-railbig")
	v.SetSize(600, 40)
	v.loading = false
	// Create 25 snapshots to trigger bracketed numbers
	snaps := makeSnapshots("run-railbig", 25)
	v.snapshots = snaps
	v.cursor = 0
	out := v.View()
	// Snapshot 21 should use bracketed form [21]
	assert.Contains(t, out, "[21]")
}

func TestTimelineView_RenderRail_FollowsSelectedSnapshotWindow(t *testing.T) {
	v := newTimelineView("run-rail-window")
	v.SetSize(32, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-rail-window", 8)
	v.cursor = 7

	rail := v.renderRail()

	assert.Contains(t, rail, snapshotMarker(8), "selected latest snapshot should stay visible in the rail window")
	assert.Contains(t, rail, "...+4", "rail should indicate omitted snapshots on the left")
}

func TestTimelineView_View_SplitPane_WideTerminal(t *testing.T) {
	v := newTimelineView("run-wide")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-wide", 3)
	v.cursor = 1
	out := v.View()
	// The split divider character should appear in wide mode
	assert.Contains(t, out, "│")
	// Both list header and detail header should be present
	assert.Contains(t, out, "Snapshots")
	assert.Contains(t, out, "Snapshot")
}

func TestTimelineView_View_CompactLayout_NarrowTerminal(t *testing.T) {
	v := newTimelineView("run-narrow")
	v.SetSize(60, 30)
	v.loading = false
	v.snapshots = makeSnapshots("run-narrow", 3)
	v.cursor = 0
	out := v.View()
	// In compact mode there should be no divider column
	// (the │ might still appear in the divider lines, but the split pane shouldn't)
	assert.Contains(t, out, "①")
}

func TestTimelineView_View_DiffSectionFirstSnapshot(t *testing.T) {
	v := newTimelineView("run-diffirst")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-diffirst", 3)
	v.cursor = 0 // first snapshot — no diff
	out := v.View()
	assert.Contains(t, out, "first snapshot")
}

func TestTimelineView_View_DiffSectionLoadingState(t *testing.T) {
	v := newTimelineView("run-diffloading")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-diffloading", 3)
	v.cursor = 1
	v.loadingDiff = true
	out := v.View()
	assert.Contains(t, out, "computing diff")
}

func TestTimelineView_View_DiffSectionShowsDiff(t *testing.T) {
	v := newTimelineView("run-diffshow")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-diffshow", 3)
	v.snapshots = snaps
	v.cursor = 1
	// Pre-load a diff
	key := snaps[0].ID + ":" + snaps[1].ID
	v.diffs[key] = makeDiff(snaps[0].ID, snaps[1].ID, 1, 2, 1, 0, 1)
	out := v.View()
	// Diff summary should be shown
	assert.Contains(t, out, "Diff")
	assert.Contains(t, out, "→")
}

func TestTimelineView_View_ActionResult_Fork(t *testing.T) {
	v := newTimelineView("run-forkresult")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-forkresult", 2)
	v.cursor = 0
	v.pendingResult = &smithers.ForkReplayRun{
		ID:        "new-run-abcdef12",
		Status:    "active",
		StartedAt: time.Now(),
	}
	out := v.View()
	assert.Contains(t, out, "New run")
}

func TestTimelineView_View_ActionError(t *testing.T) {
	v := newTimelineView("run-acerr")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-acerr", 2)
	v.cursor = 0
	v.pendingErr = errors.New("fork failed: quota exceeded")
	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "quota exceeded")
}

func TestTimelineView_View_ForkOriginMarker(t *testing.T) {
	v := newTimelineView("run-forked")
	v.SetSize(120, 40)
	v.loading = false
	parentID := "parent-snap-id"
	snaps := makeSnapshots("run-forked", 3)
	snaps[1].ParentID = &parentID // mark snapshot 2 as a fork
	v.snapshots = snaps
	v.cursor = 0
	out := v.View()
	// Fork origin marker ⎇ should appear for snapshot with a parent
	assert.Contains(t, out, "⎇")
}

// --- Name / ShortHelp ---

func TestTimelineView_Name(t *testing.T) {
	v := newTimelineView("run-name")
	assert.Equal(t, "timeline", v.Name())
}

func TestTimelineView_ShortHelp_Normal(t *testing.T) {
	v := newTimelineView("run-help")
	binds := v.ShortHelp()
	assert.NotEmpty(t, binds)

	var keys []string
	for _, b := range binds {
		h := b.Help()
		keys = append(keys, h.Desc)
	}
	joined := strings.Join(keys, " ")
	assert.Contains(t, joined, "navigate")
	assert.Contains(t, joined, "fork")
	assert.Contains(t, joined, "replay")
	assert.Contains(t, joined, "back")
}

func TestTimelineView_ShortHelp_DuringConfirmation(t *testing.T) {
	v := newTimelineView("run-help-confirm")
	v.pendingAction = pendingFork
	binds := v.ShortHelp()
	assert.Len(t, binds, 2)

	var descs []string
	for _, b := range binds {
		descs = append(descs, b.Help().Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "confirm")
	assert.Contains(t, joined, "cancel")
}

// --- Helper functions ---

func TestSnapshotMarker_OneToTwenty(t *testing.T) {
	encircled := []string{
		"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩",
		"⑪", "⑫", "⑬", "⑭", "⑮", "⑯", "⑰", "⑱", "⑲", "⑳",
	}
	for i, want := range encircled {
		got := snapshotMarker(i + 1)
		assert.Equal(t, want, got, "snapshotMarker(%d)", i+1)
	}
}

func TestSnapshotMarker_BeyondTwenty(t *testing.T) {
	assert.Equal(t, "[21]", snapshotMarker(21))
	assert.Equal(t, "[100]", snapshotMarker(100))
	assert.Equal(t, "[0]", snapshotMarker(0), "zero should use bracket form")
	assert.Equal(t, "[-1]", snapshotMarker(-1), "negative should use bracket form")
}

func TestRenderSnapshotDiff_Nil(t *testing.T) {
	out := renderSnapshotDiff(nil, 1, 2, 80)
	assert.Empty(t, out)
}

func TestRenderSnapshotDiff_EmptyEntries(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 1, ToNo: 2,
	}
	out := renderSnapshotDiff(diff, 1, 2, 80)
	assert.Contains(t, out, "no changes")
}

func TestRenderSnapshotDiff_AddEntry(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 1, ToNo: 2,
		AddedCount: 1,
		Entries: []smithers.DiffEntry{
			{Path: "toolCalls[0]", Op: "add", NewValue: `{"name":"bash"}`},
		},
	}
	out := renderSnapshotDiff(diff, 1, 2, 80)
	assert.Contains(t, out, "+")
	assert.Contains(t, out, "toolCalls[0]")
}

func TestRenderSnapshotDiff_RemoveEntry(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 1, ToNo: 2,
		RemovedCount: 1,
		Entries: []smithers.DiffEntry{
			{Path: "messages[5]", Op: "remove", OldValue: "old content"},
		},
	}
	out := renderSnapshotDiff(diff, 1, 2, 80)
	assert.Contains(t, out, "-")
	assert.Contains(t, out, "messages[5]")
}

func TestRenderSnapshotDiff_ReplaceEntry(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 3, ToNo: 4,
		ChangedCount: 1,
		Entries: []smithers.DiffEntry{
			{Path: "nodeState.status", Op: "replace",
				OldValue: "pending", NewValue: "running"},
		},
	}
	out := renderSnapshotDiff(diff, 3, 4, 80)
	assert.Contains(t, out, "~")
	assert.Contains(t, out, "nodeState.status")
	assert.Contains(t, out, "pending")
	assert.Contains(t, out, "running")
}

func TestRenderSnapshotDiff_TruncatesAtTwentyEntries(t *testing.T) {
	entries := make([]smithers.DiffEntry, 25)
	for i := range entries {
		entries[i] = smithers.DiffEntry{
			Path:     fmt.Sprintf("field[%d]", i),
			Op:       "add",
			NewValue: "val",
		}
	}
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 1, ToNo: 2,
		AddedCount: 25,
		Entries:    entries,
	}
	out := renderSnapshotDiff(diff, 1, 2, 80)
	assert.Contains(t, out, "more entries")
	// Should not contain field[20] or beyond as separate items
	assert.Contains(t, out, "+5 more entries")
}

func TestFmtBytes_Sizes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{2048, "2.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}
	for _, tt := range tests {
		got := fmtBytes(tt.input)
		assert.Equal(t, tt.want, got, "fmtBytes(%d)", tt.input)
	}
}

func TestTruncateMiddle_Short(t *testing.T) {
	s := "hello"
	got := truncateMiddle(s, 10)
	assert.Equal(t, s, got, "short string should not be truncated")
}

func TestTruncateMiddle_Long(t *testing.T) {
	s := "messages[0].content.very.long.path.name"
	got := truncateMiddle(s, 20)
	assert.LessOrEqual(t, len(got), len(s), "truncated string should be shorter")
	assert.Contains(t, got, "…", "truncated string should contain ellipsis")
}

func TestTruncateMiddle_ExactLength(t *testing.T) {
	s := "exactly20charslong!!"
	got := truncateMiddle(s, 20)
	assert.Equal(t, s, got)
}

func TestTruncateMiddle_TooSmallMaxLen(t *testing.T) {
	s := "hello world"
	got := truncateMiddle(s, 3)
	// maxLen < 5, so no truncation
	assert.Equal(t, s, got, "maxLen < 5 should not truncate")
}

// --- Integration-style: client exec wiring ---

func TestTimelineView_FetchSnapshots_DoesNotPanic(t *testing.T) {
	// Ensures the fetchSnapshots command can be invoked without panicking even
	// when no server is available (will return an error message, not panic).
	v := newTimelineView("run-ctx")
	cmd := v.fetchSnapshots()
	require.NotNil(t, cmd)
	msg := cmd()
	switch msg.(type) {
	case timelineLoadedMsg, timelineErrorMsg:
		// OK — either result is acceptable
	default:
		t.Errorf("unexpected message type %T from fetchSnapshots", msg)
	}
}

func TestTimelineView_FetchDiff_DoesNotPanic(t *testing.T) {
	v := newTimelineView("run-ctx2")
	snaps := makeSnapshots("run-ctx2", 2)
	cmd := v.fetchDiff(snaps[0], snaps[1])
	require.NotNil(t, cmd)
	msg := cmd()
	switch msg.(type) {
	case timelineDiffLoadedMsg, timelineDiffErrorMsg:
		// OK
	default:
		t.Errorf("unexpected message type %T from fetchDiff", msg)
	}
}

func TestTimelineView_FetchSnapshots_RecordsObservability(t *testing.T) {
	configureTimelineObservability(t)

	client := newTimelineHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/snapshot/list", r.URL.Path)
		writeTimelineEnvelope(t, w, makeSnapshots("run-obs", 3))
	})
	v := NewTimelineView(client, "run-obs")

	msg := v.fetchSnapshots()()
	loadedMsg, ok := msg.(timelineLoadedMsg)
	require.True(t, ok, "expected timelineLoadedMsg, got %T", msg)
	require.Len(t, loadedMsg.snapshots, 3)

	attrs := requireRecentSpanAttrs(t, "ui.snapshots.load")
	require.Equal(t, "load", attrs["crush.snapshot.operation"])
	require.Equal(t, "ok", attrs["crush.snapshot.result"])
	require.Equal(t, "run-obs", attrs["crush.run_id"])
	require.EqualValues(t, 3, attrs["crush.snapshot.count"])
}

func TestTimelineView_FetchDiff_RecordsObservability(t *testing.T) {
	configureTimelineObservability(t)

	snaps := makeSnapshots("run-diff-obs", 2)
	client := newTimelineHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/snapshot/diff", r.URL.Path)
		writeTimelineEnvelope(t, w, makeDiff(snaps[0].ID, snaps[1].ID, 1, 2, 1, 0, 1))
	})
	v := NewTimelineView(client, "run-diff-obs")

	msg := v.fetchDiff(snaps[0], snaps[1])()
	diffMsg, ok := msg.(timelineDiffLoadedMsg)
	require.True(t, ok, "expected timelineDiffLoadedMsg, got %T", msg)
	require.NotNil(t, diffMsg.diff)

	attrs := requireRecentSpanAttrs(t, "ui.snapshots.diff")
	require.Equal(t, "diff", attrs["crush.snapshot.operation"])
	require.Equal(t, "ok", attrs["crush.snapshot.result"])
	require.Equal(t, "run-diff-obs", attrs["crush.run_id"])
	require.Equal(t, snaps[0].ID, attrs["crush.snapshot.from_id"])
	require.Equal(t, snaps[1].ID, attrs["crush.snapshot.to_id"])
}

func TestTimelineView_DispatchAction_ForkRecordsObservability(t *testing.T) {
	configureTimelineObservability(t)

	client := newTimelineHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/snapshot/fork", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		writeTimelineEnvelope(t, w, smithers.ForkReplayRun{
			ID: "forked-run",
		})
	})

	v := NewTimelineView(client, "run-fork-obs")
	v.snapshots = makeSnapshots("run-fork-obs", 2)
	v.cursor = 1

	msg := v.dispatchAction(pendingFork)()
	_, ok := msg.(timelineForkDoneMsg)
	require.True(t, ok, "expected timelineForkDoneMsg, got %T", msg)

	attrs := requireRecentSpanAttrs(t, "ui.snapshots.fork")
	require.Equal(t, "fork", attrs["crush.snapshot.operation"])
	require.Equal(t, "ok", attrs["crush.snapshot.result"])
	require.Equal(t, "run-fork-obs", attrs["crush.run_id"])
	require.Equal(t, v.snapshots[1].ID, attrs["crush.snapshot.id"])
	require.Equal(t, "forked-run", attrs["crush.snapshot.result_run_id"])
}

func TestTimelineView_DispatchAction_ReplayRecordsObservability(t *testing.T) {
	configureTimelineObservability(t)

	client := newTimelineHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/snapshot/replay", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		writeTimelineEnvelope(t, w, smithers.ForkReplayRun{
			ID: "replayed-run",
		})
	})

	v := NewTimelineView(client, "run-replay-obs")
	v.snapshots = makeSnapshots("run-replay-obs", 2)
	v.cursor = 1

	msg := v.dispatchAction(pendingReplay)()
	_, ok := msg.(timelineReplayDoneMsg)
	require.True(t, ok, "expected timelineReplayDoneMsg, got %T", msg)

	attrs := requireRecentSpanAttrs(t, "ui.snapshots.replay")
	require.Equal(t, "replay", attrs["crush.snapshot.operation"])
	require.Equal(t, "ok", attrs["crush.snapshot.result"])
	require.Equal(t, "run-replay-obs", attrs["crush.run_id"])
	require.Equal(t, v.snapshots[1].ID, attrs["crush.snapshot.id"])
	require.Equal(t, "replayed-run", attrs["crush.snapshot.result_run_id"])
}

// ============================================================================
// feat-time-travel-snapshot-markers
// ============================================================================

// --- classifySnapshot ---

func TestClassifySnapshot_Auto(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "node-run", "Step complete", 1, time.Now())
	assert.Equal(t, snapshotKindAuto, classifySnapshot(snap))
}

func TestClassifySnapshot_Error_Label(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "node-run", "Error: tool failed", 1, time.Now())
	assert.Equal(t, snapshotKindError, classifySnapshot(snap))
}

func TestClassifySnapshot_Error_NodeID(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "error-handler", "step done", 1, time.Now())
	assert.Equal(t, snapshotKindError, classifySnapshot(snap))
}

func TestClassifySnapshot_Error_Fail(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "node", "tool-fail detected", 1, time.Now())
	assert.Equal(t, snapshotKindError, classifySnapshot(snap))
}

func TestClassifySnapshot_Manual_Label(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "node", "manual checkpoint", 1, time.Now())
	assert.Equal(t, snapshotKindManual, classifySnapshot(snap))
}

func TestClassifySnapshot_Manual_Save(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "node", "save state", 1, time.Now())
	assert.Equal(t, snapshotKindManual, classifySnapshot(snap))
}

func TestClassifySnapshot_Fork_ParentID(t *testing.T) {
	pid := "parent-snap-id"
	snap := makeSnapshot("s1", "r1", "node", "step", 1, time.Now())
	snap.ParentID = &pid
	// fork takes priority over label-based classification
	assert.Equal(t, snapshotKindFork, classifySnapshot(snap))
}

func TestClassifySnapshot_Fork_TakesPriorityOverError(t *testing.T) {
	pid := "parent-snap-id"
	snap := makeSnapshot("s1", "r1", "error-node", "Error: some error", 1, time.Now())
	snap.ParentID = &pid
	assert.Equal(t, snapshotKindFork, classifySnapshot(snap))
}

// --- snapshotKindLabel ---

func TestSnapshotKindLabel_All(t *testing.T) {
	assert.Equal(t, "auto", snapshotKindLabel(snapshotKindAuto))
	assert.Equal(t, "error", snapshotKindLabel(snapshotKindError))
	assert.Equal(t, "manual", snapshotKindLabel(snapshotKindManual))
	assert.Equal(t, "fork", snapshotKindLabel(snapshotKindFork))
}

// --- snapshotKindStyle returns a non-zero style for each kind ---

func TestSnapshotKindStyle_ReturnsDistinctColors(t *testing.T) {
	// We just verify each kind returns a non-zero foreground colour (not the
	// default empty style).  We cannot compare lipgloss.Color directly so we
	// check that the rendered outputs of a test string differ between kinds.
	render := func(kind snapshotKind) string {
		return snapshotKindStyle(kind).Render("X")
	}
	auto := render(snapshotKindAuto)
	errK := render(snapshotKindError)
	man := render(snapshotKindManual)
	fork := render(snapshotKindFork)

	assert.NotEqual(t, auto, errK)
	assert.NotEqual(t, auto, man)
	assert.NotEqual(t, auto, fork)
	assert.NotEqual(t, errK, man)
	assert.NotEqual(t, errK, fork)
	assert.NotEqual(t, man, fork)
}

// --- Rail renders kind-coloured markers ---

func TestTimelineView_View_RailKindColors_ErrorSnap(t *testing.T) {
	v := newTimelineView("run-rail-kind")
	v.SetSize(200, 40)
	v.loading = false
	snaps := makeSnapshots("run-rail-kind", 3)
	snaps[1].Label = "Error: tool call failed" // mark snap 2 as error
	v.snapshots = snaps
	v.cursor = 0
	out := v.View()
	// The view should render without panicking and contain the markers.
	assert.Contains(t, out, "①")
	assert.Contains(t, out, "②")
	assert.Contains(t, out, "③")
}

func TestTimelineView_View_RailArrow_SelectedSnapshot(t *testing.T) {
	v := newTimelineView("run-rail-arrow")
	v.SetSize(200, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-rail-arrow", 4)
	v.cursor = 1
	out := v.View()
	// Arrow indicator ▲ should appear below the selected snapshot.
	assert.Contains(t, out, "▲", "rail should contain ▲ under selected snapshot")
}

func TestTimelineView_View_ListKindBadge_AutoSnapshot(t *testing.T) {
	v := newTimelineView("run-list-auto")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-list-auto", 2)
	v.snapshots = snaps
	v.cursor = 0
	out := v.View()
	// Detail pane should show the [auto] kind badge.
	assert.Contains(t, out, "[auto]")
}

func TestTimelineView_View_ListKindBadge_ErrorSnapshot(t *testing.T) {
	v := newTimelineView("run-list-error")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-list-error", 2)
	snaps[0].Label = "Error: something went wrong"
	v.snapshots = snaps
	v.cursor = 0
	out := v.View()
	assert.Contains(t, out, "[error]")
}

func TestTimelineView_View_CompactListKind(t *testing.T) {
	v := newTimelineView("run-compact-kind")
	v.SetSize(60, 30)
	v.loading = false
	snaps := makeSnapshots("run-compact-kind", 3)
	snaps[1].Label = "manual checkpoint"
	v.snapshots = snaps
	v.cursor = 1
	out := v.View()
	// Compact layout should show kind label for selected snapshot.
	assert.Contains(t, out, "kind:")
	assert.Contains(t, out, "manual")
}

// ============================================================================
// feat-time-travel-snapshot-inspector
// ============================================================================

// --- Inspector open/close via Enter ---

func TestTimelineView_Enter_OpensInspector(t *testing.T) {
	v := newTimelineView("run-insp-open")
	v.snapshots = makeSnapshots("run-insp-open", 3)
	v.cursor = 1

	updated, cmd := pressSpecialKey(v, tea.KeyEnter)
	assert.Nil(t, cmd)

	tv := updated.(*TimelineView)
	assert.True(t, tv.inspecting, "Enter should open inspector")
	assert.Equal(t, 0, tv.inspectorScroll, "inspector scroll should reset to 0")
}

func TestTimelineView_Enter_NoopWhenNoSnapshots(t *testing.T) {
	v := newTimelineView("run-insp-nosnap")
	// no snapshots
	updated, cmd := pressSpecialKey(v, tea.KeyEnter)
	assert.Nil(t, cmd)
	tv := updated.(*TimelineView)
	assert.False(t, tv.inspecting, "Enter should be no-op with no snapshots")
}

func TestTimelineView_Inspector_EnterClosesInspector(t *testing.T) {
	v := newTimelineView("run-insp-close-enter")
	v.snapshots = makeSnapshots("run-insp-close-enter", 2)
	v.inspecting = true
	v.inspectorScroll = 3

	updated, cmd := pressSpecialKey(v, tea.KeyEnter)
	assert.Nil(t, cmd)
	tv := updated.(*TimelineView)
	assert.False(t, tv.inspecting, "Enter in inspector should close it")
	assert.Equal(t, 0, tv.inspectorScroll, "scroll should reset on close")
}

func TestTimelineView_Inspector_QClosesInspector(t *testing.T) {
	v := newTimelineView("run-insp-close-q")
	v.snapshots = makeSnapshots("run-insp-close-q", 2)
	v.inspecting = true
	v.inspectorScroll = 2

	updated, cmd := pressKey(v, "q")
	assert.Nil(t, cmd)
	tv := updated.(*TimelineView)
	assert.False(t, tv.inspecting)
	assert.Equal(t, 0, tv.inspectorScroll)
}

func TestTimelineView_Inspector_EscClosesInspector(t *testing.T) {
	v := newTimelineView("run-insp-close-esc")
	v.snapshots = makeSnapshots("run-insp-close-esc", 2)
	v.inspecting = true

	updated, cmd := pressSpecialKey(v, tea.KeyEscape)
	assert.Nil(t, cmd)
	tv := updated.(*TimelineView)
	assert.False(t, tv.inspecting)
}

// --- Inspector scroll ---

func TestTimelineView_Inspector_DownScrolls(t *testing.T) {
	v := newTimelineView("run-insp-scroll-down")
	v.snapshots = makeSnapshots("run-insp-scroll-down", 2)
	v.inspecting = true
	v.inspectorScroll = 2

	updated, _ := pressSpecialKey(v, tea.KeyDown)
	tv := updated.(*TimelineView)
	assert.Equal(t, 3, tv.inspectorScroll)
	assert.True(t, tv.inspecting, "inspecting should remain true while scrolling")
}

func TestTimelineView_Inspector_UpScrolls(t *testing.T) {
	v := newTimelineView("run-insp-scroll-up")
	v.snapshots = makeSnapshots("run-insp-scroll-up", 2)
	v.inspecting = true
	v.inspectorScroll = 5

	updated, _ := pressSpecialKey(v, tea.KeyUp)
	tv := updated.(*TimelineView)
	assert.Equal(t, 4, tv.inspectorScroll)
}

func TestTimelineView_Inspector_UpNotBelowZero(t *testing.T) {
	v := newTimelineView("run-insp-scroll-min")
	v.snapshots = makeSnapshots("run-insp-scroll-min", 2)
	v.inspecting = true
	v.inspectorScroll = 0

	updated, _ := pressSpecialKey(v, tea.KeyUp)
	tv := updated.(*TimelineView)
	assert.Equal(t, 0, tv.inspectorScroll, "scroll should not go below 0")
}

func TestTimelineView_Inspector_NavKeysDoNotMoveCursor(t *testing.T) {
	v := newTimelineView("run-insp-nav-block")
	v.snapshots = makeSnapshots("run-insp-nav-block", 5)
	v.cursor = 2
	v.inspecting = true

	// Down in inspector mode should not move the snapshot cursor.
	updated, _ := pressSpecialKey(v, tea.KeyDown)
	tv := updated.(*TimelineView)
	assert.Equal(t, 2, tv.cursor, "cursor should not change while inspector is open")
}

// --- Inspector rendering ---

func TestTimelineView_View_InspectorOverlay(t *testing.T) {
	v := newTimelineView("run-insp-view")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-insp-view", 3)
	snaps[1].StateJSON = `{"messages":["hello"],"status":"running"}`
	v.snapshots = snaps
	v.cursor = 1
	v.inspecting = true
	out := v.View()

	// Inspector header.
	assert.Contains(t, out, "Inspector")
	assert.Contains(t, out, "②")
	// Metadata fields.
	assert.Contains(t, out, "ID:")
	assert.Contains(t, out, "Run:")
	assert.Contains(t, out, "Node:")
	assert.Contains(t, out, "Time:")
	// StateJSON section.
	assert.Contains(t, out, "State JSON:")
}

func TestTimelineView_View_InspectorStateJSON_Pretty(t *testing.T) {
	v := newTimelineView("run-insp-json")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-insp-json", 2)
	snaps[0].StateJSON = `{"key":"value","count":42,"active":true,"nothing":null}`
	v.snapshots = snaps
	v.cursor = 0
	v.inspecting = true
	out := v.View()

	// Pretty-printed JSON keys should appear.
	assert.Contains(t, out, "key")
	assert.Contains(t, out, "value")
	assert.Contains(t, out, "count")
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "active")
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "nothing")
	assert.Contains(t, out, "null")
}

func TestTimelineView_View_InspectorEmptyStateJSON(t *testing.T) {
	v := newTimelineView("run-insp-empty-json")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-insp-empty-json", 2)
	snaps[0].StateJSON = ""
	v.snapshots = snaps
	v.cursor = 0
	v.inspecting = true
	out := v.View()
	assert.Contains(t, out, "empty")
}

func TestTimelineView_View_InspectorInvalidJSON(t *testing.T) {
	v := newTimelineView("run-insp-invalid-json")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-insp-invalid-json", 2)
	snaps[0].StateJSON = "not-valid-json-at-all"
	v.snapshots = snaps
	v.cursor = 0
	v.inspecting = true
	out := v.View()
	// Should render without panicking; raw text should appear.
	assert.Contains(t, out, "not-valid-json-at-all")
}

func TestTimelineView_View_InspectorScrollHint_LongJSON(t *testing.T) {
	v := newTimelineView("run-insp-scroll-hint")
	v.SetSize(120, 10) // very small height to force scroll
	v.loading = false
	// Build a large JSON object so stateLines > maxStateLines.
	var pairs []string
	for i := 0; i < 50; i++ {
		pairs = append(pairs, fmt.Sprintf(`"field%d": "value%d"`, i, i))
	}
	jsonStr := "{" + strings.Join(pairs, ", ") + "}"
	snaps := makeSnapshots("run-insp-scroll-hint", 2)
	snaps[0].StateJSON = jsonStr
	v.snapshots = snaps
	v.cursor = 0
	v.inspecting = true
	out := v.View()
	// Scroll hint should appear.
	assert.Contains(t, out, "scroll")
}

func TestTimelineView_View_InspectorFooter(t *testing.T) {
	v := newTimelineView("run-insp-footer")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-insp-footer", 2)
	v.cursor = 0
	v.inspecting = true
	out := v.View()
	assert.Contains(t, out, "Close inspector")
}

func TestTimelineView_View_InspectorNotShownWhenClosed(t *testing.T) {
	v := newTimelineView("run-insp-closed")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-insp-closed", 3)
	v.cursor = 1
	v.inspecting = false
	out := v.View()
	// "Inspector:" prefix should not appear when not inspecting.
	assert.NotContains(t, out, "Inspector:")
}

func TestTimelineView_ShortHelp_InspectorMode(t *testing.T) {
	v := newTimelineView("run-help-insp")
	v.inspecting = true
	binds := v.ShortHelp()
	var descs []string
	for _, b := range binds {
		descs = append(descs, b.Help().Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "scroll")
	assert.Contains(t, joined, "close inspector")
}

// --- renderPrettyJSON standalone tests ---

func TestRenderPrettyJSON_Empty(t *testing.T) {
	out := renderPrettyJSON("", 80)
	assert.Contains(t, out, "empty")
}

func TestRenderPrettyJSON_ValidObject(t *testing.T) {
	out := renderPrettyJSON(`{"a":1,"b":"hello"}`, 80)
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "b")
	assert.Contains(t, out, "hello")
}

func TestRenderPrettyJSON_InvalidJSON(t *testing.T) {
	out := renderPrettyJSON("not-json", 80)
	assert.Contains(t, out, "not-json")
}

func TestRenderPrettyJSON_NestedObject(t *testing.T) {
	out := renderPrettyJSON(`{"outer":{"inner":"val"}}`, 80)
	assert.Contains(t, out, "outer")
	assert.Contains(t, out, "inner")
	assert.Contains(t, out, "val")
}

func TestRenderPrettyJSON_Array(t *testing.T) {
	out := renderPrettyJSON(`[1,2,3]`, 80)
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "3")
}

func TestRenderPrettyJSON_Null(t *testing.T) {
	out := renderPrettyJSON(`{"key":null}`, 80)
	assert.Contains(t, out, "null")
}

func TestRenderPrettyJSON_Boolean(t *testing.T) {
	out := renderPrettyJSON(`{"ok":true,"bad":false}`, 80)
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "false")
}

// --- colorizeJSONValue standalone tests ---

func TestColorizeJSONValue_Null(t *testing.T) {
	faint := lipgloss.NewStyle().Faint(true)
	strS := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	numS := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	nullS := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	out := colorizeJSONValue("null", strS, numS, nullS, faint)
	assert.NotEmpty(t, out)
}

func TestColorizeJSONValue_True(t *testing.T) {
	faint := lipgloss.NewStyle().Faint(true)
	strS := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	numS := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	nullS := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	out := colorizeJSONValue("true", strS, numS, nullS, faint)
	assert.Contains(t, out, "true")
}

func TestColorizeJSONValue_String(t *testing.T) {
	faint := lipgloss.NewStyle().Faint(true)
	strS := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	numS := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	nullS := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	out := colorizeJSONValue(`"hello"`, strS, numS, nullS, faint)
	assert.Contains(t, out, "hello")
}

func TestColorizeJSONValue_Number(t *testing.T) {
	faint := lipgloss.NewStyle().Faint(true)
	strS := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	numS := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	nullS := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	out := colorizeJSONValue("42", strS, numS, nullS, faint)
	assert.Contains(t, out, "42")
}

func TestColorizeJSONValue_TrailingComma(t *testing.T) {
	faint := lipgloss.NewStyle().Faint(true)
	strS := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	numS := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	nullS := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	out := colorizeJSONValue(`"hello",`, strS, numS, nullS, faint)
	assert.Contains(t, out, "hello")
}

func TestColorizeJSONValue_Punctuation(t *testing.T) {
	faint := lipgloss.NewStyle().Faint(true)
	strS := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	numS := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	nullS := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	for _, p := range []string{"{", "}", "[", "]", "{}", "[]"} {
		out := colorizeJSONValue(p, strS, numS, nullS, faint)
		assert.Contains(t, out, p, "punctuation %q should be preserved", p)
	}
}

// ============================================================================
// feat-time-travel-fork-from-snapshot
// ============================================================================

// TestForkSuccessToast_EmitsShowToastMsg verifies that a successful fork
// dispatches a ShowToastMsg with success level and the new run ID.
func TestForkSuccessToast_EmitsShowToastMsg(t *testing.T) {
	run := &smithers.ForkReplayRun{
		ID:        "fork-run-abcdef12",
		Status:    "paused",
		StartedAt: time.Now(),
	}
	cmd := forkSuccessToast(run)
	require.NotNil(t, cmd, "forkSuccessToast should return a non-nil cmd")

	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "forkSuccessToast must emit ShowToastMsg, got %T", msg)
	assert.Equal(t, components.ToastLevelSuccess, toast.Level)
	assert.Contains(t, toast.Title, "Fork")
	// run ID should be truncated to 8 chars in the body
	assert.Contains(t, toast.Body, "fork-run")
	assert.Contains(t, toast.Body, "paused")
}

// TestForkSuccessToast_NilRun verifies that a nil run returns no cmd.
func TestForkSuccessToast_NilRun(t *testing.T) {
	cmd := forkSuccessToast(nil)
	assert.Nil(t, cmd, "forkSuccessToast(nil) should return nil")
}

// TestForkSuccessToast_LongRunID_Truncated ensures the run ID is limited to 8
// chars in the toast body.
func TestForkSuccessToast_LongRunID_Truncated(t *testing.T) {
	run := &smithers.ForkReplayRun{
		ID:        "very-long-run-id-that-exceeds-eight-chars",
		Status:    "active",
		StartedAt: time.Now(),
	}
	cmd := forkSuccessToast(run)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(components.ShowToastMsg)
	assert.NotContains(t, toast.Body, "very-long-run-id-that-exceeds-eight-chars",
		"full run ID should not appear in toast body")
	assert.Contains(t, toast.Body, "very-lon", "truncated 8-char prefix should appear")
}

// TestTimelineView_ForkDone_EmitsToastCmd verifies that when the view receives
// a timelineForkDoneMsg it returns a command that emits a ShowToastMsg.
func TestTimelineView_ForkDone_EmitsToastCmd(t *testing.T) {
	v := newTimelineView("run-fork-toast")
	v.pendingAction = pendingFork

	run := &smithers.ForkReplayRun{
		ID:        "fork-toast-run",
		Status:    "active",
		StartedAt: time.Now(),
	}

	updated, cmd := v.Update(timelineForkDoneMsg{run: run})
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
	assert.Equal(t, run, tv.pendingResult)

	require.NotNil(t, cmd, "ForkDone should return a toast cmd")
	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "ForkDone cmd must emit ShowToastMsg, got %T", msg)
	assert.Equal(t, components.ToastLevelSuccess, toast.Level)
	assert.Contains(t, toast.Body, "active")
}

// TestTimelineView_View_ForkResultInDetailPane verifies the inline result is
// rendered in the detail pane after a successful fork.
func TestTimelineView_View_ForkResultInDetailPane(t *testing.T) {
	v := newTimelineView("run-fork-detail")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-fork-detail", 2)
	v.cursor = 0
	v.pendingResult = &smithers.ForkReplayRun{
		ID:        "forked-run-xyz",
		Status:    "paused",
		StartedAt: time.Now(),
	}
	out := v.View()
	assert.Contains(t, out, "New run", "fork result should appear in detail pane")
	assert.Contains(t, out, "paused", "run status should appear in detail pane")
}

// ============================================================================
// feat-time-travel-replay-from-snapshot
// ============================================================================

// TestReplaySuccessToast_EmitsShowToastMsg verifies that a successful replay
// dispatches a ShowToastMsg with success level and the new run ID.
func TestReplaySuccessToast_EmitsShowToastMsg(t *testing.T) {
	run := &smithers.ForkReplayRun{
		ID:        "replay-run-abcd",
		Status:    "active",
		StartedAt: time.Now(),
	}
	cmd := replaySuccessToast(run)
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "replaySuccessToast must emit ShowToastMsg, got %T", msg)
	assert.Equal(t, components.ToastLevelSuccess, toast.Level)
	assert.Contains(t, toast.Title, "Replay")
	assert.Contains(t, toast.Body, "replay-r") // 8-char truncation
	assert.Contains(t, toast.Body, "active")
}

// TestReplaySuccessToast_NilRun verifies that a nil run returns no cmd.
func TestReplaySuccessToast_NilRun(t *testing.T) {
	cmd := replaySuccessToast(nil)
	assert.Nil(t, cmd)
}

// TestTimelineView_ReplayDone_EmitsToastCmd verifies that timelineReplayDoneMsg
// causes the view to return a ShowToastMsg command.
func TestTimelineView_ReplayDone_EmitsToastCmd(t *testing.T) {
	v := newTimelineView("run-replay-toast")
	v.pendingAction = pendingReplay

	run := &smithers.ForkReplayRun{
		ID:        "replay-toast-run",
		Status:    "active",
		StartedAt: time.Now(),
	}

	updated, cmd := v.Update(timelineReplayDoneMsg{run: run})
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
	assert.Equal(t, run, tv.pendingResult)

	require.NotNil(t, cmd, "ReplayDone should return a toast cmd")
	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "ReplayDone cmd must emit ShowToastMsg, got %T", msg)
	assert.Equal(t, components.ToastLevelSuccess, toast.Level)
	assert.Contains(t, toast.Title, "Replay")
}

// TestTimelineView_View_ReplayResultInDetailPane verifies the inline result is
// rendered in the detail pane after a successful replay.
func TestTimelineView_View_ReplayResultInDetailPane(t *testing.T) {
	v := newTimelineView("run-replay-detail")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-replay-detail", 2)
	v.cursor = 0
	v.pendingResult = &smithers.ForkReplayRun{
		ID:        "replayed-run-abc",
		Status:    "active",
		StartedAt: time.Now(),
	}
	out := v.View()
	assert.Contains(t, out, "New run")
	assert.Contains(t, out, "active")
}

// ============================================================================
// feat-time-travel-snapshot-diff
// ============================================================================

// TestRenderSnapshotDiff_AddedOpSymbol verifies that "add" entries render
// with the "+" symbol and green colouring (tested by symbol presence).
func TestRenderSnapshotDiff_AddedOpSymbol(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 1, ToNo: 2,
		AddedCount: 1,
		Entries: []smithers.DiffEntry{
			{Path: "toolResults[0]", Op: "add", NewValue: "result text"},
		},
	}
	out := renderSnapshotDiff(diff, 1, 2, 80)
	assert.Contains(t, out, "+", "added entry must render with + symbol")
	assert.Contains(t, out, "toolResults[0]", "path must appear")
	assert.Contains(t, out, "result text", "new value must appear for add")
}

// TestRenderSnapshotDiff_RemovedOpSymbol verifies "remove" renders with "-"
// and shows the old value.
func TestRenderSnapshotDiff_RemovedOpSymbol(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 2, ToNo: 3,
		RemovedCount: 1,
		Entries: []smithers.DiffEntry{
			{Path: "context.vars.x", Op: "remove", OldValue: "old-val"},
		},
	}
	out := renderSnapshotDiff(diff, 2, 3, 80)
	assert.Contains(t, out, "-", "removed entry must render with - symbol")
	assert.Contains(t, out, "context.vars.x")
	assert.Contains(t, out, "old-val", "old value must appear for remove")
}

// TestRenderSnapshotDiff_ModifiedOpSymbol verifies "replace" renders with "~"
// and shows both old and new values.
func TestRenderSnapshotDiff_ModifiedOpSymbol(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 3, ToNo: 4,
		ChangedCount: 1,
		Entries: []smithers.DiffEntry{
			{Path: "state.phase", Op: "replace", OldValue: "planning", NewValue: "executing"},
		},
	}
	out := renderSnapshotDiff(diff, 3, 4, 80)
	assert.Contains(t, out, "~", "modified entry must render with ~ symbol")
	assert.Contains(t, out, "state.phase")
	assert.Contains(t, out, "planning", "old value must appear for replace")
	assert.Contains(t, out, "executing", "new value must appear for replace")
}

// TestRenderSnapshotDiff_SummaryLine verifies the header line shows counts in
// the expected "+added -removed ~changed" format.
func TestRenderSnapshotDiff_SummaryLine(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 1, ToNo: 5,
		AddedCount: 3, RemovedCount: 1, ChangedCount: 2,
	}
	out := renderSnapshotDiff(diff, 1, 5, 80)
	assert.Contains(t, out, "+3", "added count should appear in summary")
	assert.Contains(t, out, "-1", "removed count should appear in summary")
	assert.Contains(t, out, "~2", "changed count should appear in summary")
}

// TestRenderSnapshotDiff_MarkerRange verifies the from/to snapshot markers
// appear in the summary header.
func TestRenderSnapshotDiff_MarkerRange(t *testing.T) {
	diff := &smithers.SnapshotDiff{
		FromID: "a", ToID: "b", FromNo: 2, ToNo: 7,
	}
	out := renderSnapshotDiff(diff, 2, 7, 80)
	// Markers for 2 and 7 (encircled numbers)
	assert.Contains(t, out, snapshotMarker(2))
	assert.Contains(t, out, snapshotMarker(7))
	assert.Contains(t, out, "→", "arrow separator must appear between markers")
}

// TestTimelineView_DiffSection_PressD_TriggersFetch verifies pressing "d"
// when a diff is not cached returns a non-nil fetch command.
func TestTimelineView_DiffSection_PressD_TriggersFetch(t *testing.T) {
	v := newTimelineView("run-d-fetch")
	v.snapshots = makeSnapshots("run-d-fetch", 3)
	v.cursor = 2 // cursor > 0 so prev exists
	// Ensure diff is not cached.
	v.diffs = make(map[string]*smithers.SnapshotDiff)
	v.diffErrs = make(map[string]error)

	_, cmd := pressKey(v, "d")
	assert.NotNil(t, cmd, "'d' with uncached diff should return fetch command")
}

// TestTimelineView_DiffSection_PressD_NilWhenCached verifies pressing "d"
// when the diff is already cached returns nil (no redundant fetch).
func TestTimelineView_DiffSection_PressD_NilWhenCached(t *testing.T) {
	v := newTimelineView("run-d-cached")
	snaps := makeSnapshots("run-d-cached", 3)
	v.snapshots = snaps
	v.cursor = 1
	key := snaps[0].ID + ":" + snaps[1].ID
	v.diffs[key] = makeDiff(snaps[0].ID, snaps[1].ID, 1, 2, 0, 0, 1)

	_, cmd := pressKey(v, "d")
	assert.Nil(t, cmd, "'d' with cached diff should return nil")
}

// TestTimelineView_View_DiffSection_NoPrevious verifies the "first snapshot"
// message appears when cursor is at position 0.
func TestTimelineView_View_DiffSection_NoPrevious(t *testing.T) {
	v := newTimelineView("run-diff-noprev")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-diff-noprev", 4)
	v.cursor = 0
	out := v.View()
	assert.Contains(t, out, "first snapshot")
}

// TestTimelineView_View_DiffSection_ErrorMsg verifies that a cached diff error
// is shown in the detail pane.
func TestTimelineView_View_DiffSection_ErrorMsg(t *testing.T) {
	v := newTimelineView("run-diff-err")
	v.SetSize(120, 40)
	v.loading = false
	snaps := makeSnapshots("run-diff-err", 3)
	v.snapshots = snaps
	v.cursor = 1
	diffKey := snaps[0].ID + ":" + snaps[1].ID
	v.diffErrs[diffKey] = errors.New("diff service timeout")
	out := v.View()
	assert.Contains(t, out, "diff unavailable")
	assert.Contains(t, out, "diff service timeout")
}

// TestTimelineView_View_DiffSection_HintWhenNotLoaded verifies the "[press d
// to load diff]" hint when no diff or error is cached.
func TestTimelineView_View_DiffSection_HintWhenNotLoaded(t *testing.T) {
	v := newTimelineView("run-diff-hint")
	v.SetSize(120, 40)
	v.loading = false
	v.snapshots = makeSnapshots("run-diff-hint", 3)
	v.cursor = 2
	// No diff cached, no error, not loading.
	out := v.View()
	assert.Contains(t, out, "press d")
}

// TestActionErrorToast_EmitsErrorToast verifies that actionErrorToast returns
// a cmd that emits a ShowToastMsg with error level.
func TestActionErrorToast_EmitsErrorToast(t *testing.T) {
	err := errors.New("fork failed: quota exceeded")
	cmd := actionErrorToast(err)
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "actionErrorToast must emit ShowToastMsg, got %T", msg)
	assert.Equal(t, components.ToastLevelError, toast.Level)
	assert.Contains(t, toast.Body, "quota exceeded")
}

// TestActionErrorToast_NilErr returns nil cmd when error is nil.
func TestActionErrorToast_NilErr(t *testing.T) {
	cmd := actionErrorToast(nil)
	assert.Nil(t, cmd)
}

// TestTimelineView_ActionError_EmitsToastCmd verifies that a timelineActionErrorMsg
// causes the view to return a ShowToastMsg error command.
func TestTimelineView_ActionError_EmitsToastCmd(t *testing.T) {
	v := newTimelineView("run-aerr-toast")
	v.pendingAction = pendingFork

	actionErr := errors.New("network error")
	updated, cmd := v.Update(timelineActionErrorMsg{err: actionErr})
	tv := updated.(*TimelineView)
	assert.Equal(t, pendingNone, tv.pendingAction)
	assert.Equal(t, actionErr, tv.pendingErr)

	require.NotNil(t, cmd, "ActionError should return a toast cmd")
	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "ActionError cmd must emit ShowToastMsg, got %T", msg)
	assert.Equal(t, components.ToastLevelError, toast.Level)
	assert.Contains(t, toast.Body, "network error")
}

// --- classifySnapshot: exception keyword ---

func TestClassifySnapshot_Error_Exception(t *testing.T) {
	snap := makeSnapshot("s1", "r1", "node", "Caught exception in handler", 1, time.Now())
	assert.Equal(t, snapshotKindError, classifySnapshot(snap))
}

// --- classifySnapshot: case-insensitivity ---

func TestClassifySnapshot_CaseInsensitive(t *testing.T) {
	// The function lowercases both label and nodeID before matching.
	snap := makeSnapshot("s1", "r1", "NODE-X", "MANUAL Save", 1, time.Now())
	assert.Equal(t, snapshotKindManual, classifySnapshot(snap))

	snapErr := makeSnapshot("s2", "r1", "node", "FATAL ERROR OCCURRED", 2, time.Now())
	assert.Equal(t, snapshotKindError, classifySnapshot(snapErr))
}

// --- snapshotKindLabel: unknown kind falls to default ---

func TestSnapshotKindLabel_UnknownKind(t *testing.T) {
	// An out-of-range snapshotKind should fall through to the default "auto"
	// branch, since the switch only handles the four known constants.
	unknown := snapshotKind(999)
	assert.Equal(t, "auto", snapshotKindLabel(unknown))
}

// --- snapshotKindStyle: unknown kind returns green (auto) default ---

func TestSnapshotKindStyle_UnknownKind(t *testing.T) {
	unknown := snapshotKind(999)
	// The default case should return the auto (green) style.
	rendered := snapshotKindStyle(unknown).Render("X")
	autoRendered := snapshotKindStyle(snapshotKindAuto).Render("X")
	assert.Equal(t, autoRendered, rendered, "unknown kind should use the auto/default style")
}
