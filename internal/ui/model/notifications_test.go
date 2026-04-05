package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- notificationTracker dedup tests ---

func TestNotificationTracker_ShouldToastRunStatus_FirstCall(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	got := tracker.shouldToastRunStatus("run-1", smithers.RunStatusRunning)
	require.True(t, got, "first call for a new (runID, status) should return true")
}

func TestNotificationTracker_ShouldToastRunStatus_DuplicateReturnsFalse(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	tracker.shouldToastRunStatus("run-1", smithers.RunStatusRunning)
	got := tracker.shouldToastRunStatus("run-1", smithers.RunStatusRunning)
	require.False(t, got, "duplicate (runID, status) pair should return false")
}

func TestNotificationTracker_ShouldToastRunStatus_DifferentStatusAllowed(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	tracker.shouldToastRunStatus("run-1", smithers.RunStatusRunning)
	got := tracker.shouldToastRunStatus("run-1", smithers.RunStatusFailed)
	require.True(t, got, "different status for same runID should return true")
}

func TestNotificationTracker_ForgetRun_AllowsReToast(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	tracker.shouldToastRunStatus("run-1", smithers.RunStatusFailed)
	tracker.forgetRun("run-1")
	got := tracker.shouldToastRunStatus("run-1", smithers.RunStatusFailed)
	require.True(t, got, "after forgetRun the same status should be toastable again")
}

func TestNotificationTracker_ShouldToastApproval_FirstCall(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	got := tracker.shouldToastApproval("approval-abc")
	require.True(t, got, "first call for a new approvalID should return true")
}

func TestNotificationTracker_ShouldToastApproval_DuplicateReturnsFalse(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	tracker.shouldToastApproval("approval-abc")
	got := tracker.shouldToastApproval("approval-abc")
	require.False(t, got, "duplicate approvalID should return false")
}

func TestNotificationTracker_ShouldToastApproval_DifferentIDsAllowed(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	tracker.shouldToastApproval("approval-abc")
	got := tracker.shouldToastApproval("approval-xyz")
	require.True(t, got, "different approvalID should return true")
}

// --- runEventToToast translation tests ---

func TestRunEventToToast_NonStatusChangedReturnsNil(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	ev := smithers.RunEvent{Type: "node_started", RunID: "run-1", Status: "running"}
	got := runEventToToast(ev, tracker)
	require.Nil(t, got, "non-status_changed events should not produce a toast")
}

func TestRunEventToToast_WaitingApprovalReturnsNil(t *testing.T) {
	t.Parallel()

	// waiting-approval is handled asynchronously via fetchApprovalAndToastCmd;
	// runEventToToast must return nil so the caller emits the fetch Cmd instead.
	tracker := newNotificationTracker()
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-abc123",
		Status: string(smithers.RunStatusWaitingApproval),
	}
	got := runEventToToast(ev, tracker)
	require.Nil(t, got, "waiting-approval should not produce a synchronous toast")
}

func TestRunEventToToast_FailedProducesErrorToast(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-abc123",
		Status: string(smithers.RunStatusFailed),
	}
	got := runEventToToast(ev, tracker)
	require.NotNil(t, got)
	assert.Equal(t, components.ToastLevelError, got.Level)
	assert.Equal(t, "Run failed", got.Title)
	// Body should contain the short run ID (first 8 chars).
	assert.Contains(t, got.Body, "run-abc1", "body should include truncated run ID")
}

func TestRunEventToToast_FinishedProducesSuccessToast(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-abc123",
		Status: string(smithers.RunStatusFinished),
	}
	got := runEventToToast(ev, tracker)
	require.NotNil(t, got)
	assert.Equal(t, components.ToastLevelSuccess, got.Level)
	assert.Equal(t, "Run finished", got.Title)
	// Body should contain the short run ID (first 8 chars).
	assert.Contains(t, got.Body, "run-abc1", "body should include truncated run ID")
}

func TestRunEventToToast_CancelledProducesInfoToast(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-abc123",
		Status: string(smithers.RunStatusCancelled),
	}
	got := runEventToToast(ev, tracker)
	require.NotNil(t, got)
	assert.Equal(t, components.ToastLevelInfo, got.Level)
	assert.Equal(t, "Run cancelled", got.Title)
	// Body should contain the short run ID (first 8 chars).
	assert.Contains(t, got.Body, "run-abc1", "body should include truncated run ID")
}

func TestRunEventToToast_DuplicateWaitingApprovalReturnsBothNil(t *testing.T) {
	t.Parallel()

	// waiting-approval is handled asynchronously; both calls must return nil.
	tracker := newNotificationTracker()
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-abc123",
		Status: string(smithers.RunStatusWaitingApproval),
	}
	got1 := runEventToToast(ev, tracker)
	got2 := runEventToToast(ev, tracker)
	require.Nil(t, got1, "waiting-approval first call should be nil (async path)")
	require.Nil(t, got2, "waiting-approval second call should also be nil (async path)")
}

func TestRunEventToToast_DifferentStatusesProduceSeparateToasts(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()

	ev1 := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-1",
		Status: string(smithers.RunStatusRunning),
	}
	ev2 := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-1",
		Status: string(smithers.RunStatusFinished),
	}

	got1 := runEventToToast(ev1, tracker)
	got2 := runEventToToast(ev2, tracker)

	require.Nil(t, got1, "RunStatusRunning is not a toastable status")
	require.NotNil(t, got2, "Finished status should produce a toast")
}

func TestRunEventToToast_TerminalStateAllowsReToastAfterForget(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()

	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  "run-1",
		Status: string(smithers.RunStatusFailed),
	}

	// First failure — produces toast and calls forgetRun internally.
	got1 := runEventToToast(ev, tracker)
	require.NotNil(t, got1)

	// Same failure again — forgetRun was called, so it should toast again.
	got2 := runEventToToast(ev, tracker)
	require.NotNil(t, got2, "after terminal state forgetRun should allow re-toast")
}

func TestRunEventToToast_ShortIDTruncation(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	longID := "abcdef1234567890"
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  longID,
		Status: string(smithers.RunStatusFinished),
	}
	got := runEventToToast(ev, tracker)
	require.NotNil(t, got)
	assert.Contains(t, got.Body, "abcdef12", "body should contain the first 8 chars of the runID")
	assert.NotContains(t, got.Body, longID, "body should not contain the full long ID")
}

func TestRunEventToToast_ShortIDNotTruncatedWhenAlreadyShort(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	shortID := "abc"
	ev := smithers.RunEvent{
		Type:   "status_changed",
		RunID:  shortID,
		Status: string(smithers.RunStatusFinished),
	}
	got := runEventToToast(ev, tracker)
	require.NotNil(t, got)
	assert.Contains(t, got.Body, shortID)
}

// --- approvalEventToToast tests ---

func TestApprovalEventToToast_WithGate(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	approval := &smithers.Approval{
		ID:           "appr-1",
		RunID:        "run-abc12345",
		Gate:         "Deploy to staging?",
		WorkflowPath: "deploy.tsx",
		Status:       "pending",
	}
	got := approvalEventToToast("run-abc12345", approval, tracker)
	require.NotNil(t, got)
	assert.Contains(t, got.Body, "Deploy to staging?")
	assert.Contains(t, got.Body, "deploy")
	assert.Equal(t, components.ToastLevelWarning, got.Level)
	assert.Equal(t, "Approval needed", got.Title)
	assert.Equal(t, 15*time.Second, got.TTL)
	require.Len(t, got.ActionHints, 2)
	assert.Equal(t, components.ActionHint{Key: "a", Label: "approve"}, got.ActionHints[0])
	assert.Equal(t, components.ActionHint{Key: "ctrl+a", Label: "view approvals"}, got.ActionHints[1])
}

func TestApprovalEventToToast_FallbackOnNilApproval(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	got := approvalEventToToast("run-abc12345", nil, tracker)
	require.NotNil(t, got)
	assert.Equal(t, "run-abc1 is waiting for approval", got.Body)
}

func TestApprovalEventToToast_DedupByApprovalID(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	approval := &smithers.Approval{
		ID:    "appr-1",
		RunID: "run-1",
		Gate:  "Gate question",
	}
	got1 := approvalEventToToast("run-1", approval, tracker)
	require.NotNil(t, got1, "first call should return a toast")

	got2 := approvalEventToToast("run-1", approval, tracker)
	require.Nil(t, got2, "second call with same approval ID should be deduped")
}

func TestApprovalEventToToast_DifferentIDsSameRun(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	a1 := &smithers.Approval{ID: "appr-1", RunID: "run-1", Gate: "Gate 1"}
	a2 := &smithers.Approval{ID: "appr-2", RunID: "run-1", Gate: "Gate 2"}

	got1 := approvalEventToToast("run-1", a1, tracker)
	require.NotNil(t, got1, "first approval should produce a toast")

	got2 := approvalEventToToast("run-1", a2, tracker)
	require.NotNil(t, got2, "second approval with different ID on same run should also produce a toast")
}

func TestApprovalEventToToast_EmptyGateFallback(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	approval := &smithers.Approval{
		ID:    "appr-1",
		RunID: "run-abc12345",
		Gate:  "", // empty gate
	}
	got := approvalEventToToast("run-abc12345", approval, tracker)
	require.NotNil(t, got)
	assert.Contains(t, got.Body, "run-abc1")
	assert.Contains(t, got.Body, "waiting for approval")
}

func TestApprovalEventToToast_WorkflowBaseNameExtraction(t *testing.T) {
	t.Parallel()

	tracker := newNotificationTracker()
	approval := &smithers.Approval{
		ID:           "appr-1",
		RunID:        "run-abc12345",
		Gate:         "Deploy?",
		WorkflowPath: ".smithers/workflows/deploy-staging.tsx",
	}
	got := approvalEventToToast("run-abc12345", approval, tracker)
	require.NotNil(t, got)
	assert.Contains(t, got.Body, "deploy-staging")
	assert.NotContains(t, got.Body, ".smithers/workflows/")
}

// --- fetchApprovalAndToastCmd tests ---

// mockSmithersClient is a test double for the smithersClient interface.
type mockSmithersClient struct {
	approvals []smithers.Approval
	err       error
}

func (m *mockSmithersClient) ListPendingApprovals(_ context.Context) ([]smithers.Approval, error) {
	return m.approvals, m.err
}

func TestFetchApprovalAndToastCmd_MatchesRunID(t *testing.T) {
	t.Parallel()

	a1 := smithers.Approval{ID: "appr-1", RunID: "run-1", Status: "pending"}
	a2 := smithers.Approval{ID: "appr-2", RunID: "run-2", Status: "pending"}
	client := &mockSmithersClient{approvals: []smithers.Approval{a1, a2}}

	cmd := fetchApprovalAndToastCmd(context.Background(), "run-1", client)
	msg := cmd().(approvalFetchedMsg)

	require.NoError(t, msg.Err)
	require.NotNil(t, msg.Approval)
	assert.Equal(t, "run-1", msg.Approval.RunID)
	assert.Equal(t, "appr-1", msg.Approval.ID)
}

func TestFetchApprovalAndToastCmd_NoMatchReturnsNilApproval(t *testing.T) {
	t.Parallel()

	client := &mockSmithersClient{approvals: []smithers.Approval{}}

	cmd := fetchApprovalAndToastCmd(context.Background(), "run-1", client)
	msg := cmd().(approvalFetchedMsg)

	require.NoError(t, msg.Err)
	assert.Nil(t, msg.Approval, "no pending approval found should return nil Approval")
	assert.Equal(t, "run-1", msg.RunID)
}

func TestFetchApprovalAndToastCmd_ErrorReturnsErr(t *testing.T) {
	t.Parallel()

	fetchErr := errors.New("network error")
	client := &mockSmithersClient{err: fetchErr}

	cmd := fetchApprovalAndToastCmd(context.Background(), "run-1", client)
	msg := cmd().(approvalFetchedMsg)

	require.Error(t, msg.Err)
	assert.Nil(t, msg.Approval)
	assert.Equal(t, "run-1", msg.RunID)
}

// --- workflowBaseName tests ---

func TestWorkflowBaseName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{".smithers/workflows/deploy.tsx", "deploy"},
		{"deploy-staging.tsx", "deploy-staging"},
		{"/abs/path/to/ci-checks.workflow", "ci-checks"},
		{"", ""},
		{"noext", "noext"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, workflowBaseName(tc.input))
		})
	}
}
