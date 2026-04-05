package model

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// approvalFetchedMsg is an internal tea.Msg returned by fetchApprovalAndToastCmd.
// Approval is nil if no matching pending approval was found or the fetch failed.
type approvalFetchedMsg struct {
	RunID    string
	Approval *smithers.Approval
	Err      error
}

// smithersClient is the subset of smithers.Client used by notification helpers.
// Scoped to keep notification logic testable without a live HTTP server.
type smithersClient interface {
	ListPendingApprovals(ctx context.Context) ([]smithers.Approval, error)
}

// fetchApprovalAndToastCmd returns a tea.Cmd that fetches the pending
// approval for runID from the Smithers client and returns an approvalFetchedMsg.
// Used when a waiting-approval status event arrives, to enrich the toast with
// the gate question and a per-approval dedup key.
func fetchApprovalAndToastCmd(ctx context.Context, runID string, client smithersClient) tea.Cmd {
	return func() tea.Msg {
		approvals, err := client.ListPendingApprovals(ctx)
		if err != nil {
			return approvalFetchedMsg{RunID: runID, Err: err}
		}
		for _, a := range approvals {
			if a.RunID == runID && a.Status == "pending" {
				aa := a
				return approvalFetchedMsg{RunID: runID, Approval: &aa}
			}
		}
		return approvalFetchedMsg{RunID: runID} // no match; Approval stays nil
	}
}

// approvalEventToToast builds a ShowToastMsg from a fetched Approval.
// If approval is nil (fetch failed or no pending approval found for the run),
// returns a fallback toast using the short run ID.
// Returns nil if the approval has already been toasted (dedup).
func approvalEventToToast(runID string, approval *smithers.Approval, tracker *notificationTracker) *components.ShowToastMsg {
	shortID := runID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	// Dedup: prefer per-approval-ID; fall back to per-(runID,status).
	if approval != nil {
		if !tracker.shouldToastApproval(approval.ID) {
			return nil
		}
	} else {
		if !tracker.shouldToastRunStatus(runID, smithers.RunStatusWaitingApproval) {
			return nil
		}
	}

	var body string
	if approval != nil && approval.Gate != "" {
		body = approval.Gate
		if approval.WorkflowPath != "" {
			body += "\nrun: " + shortID + " · " + workflowBaseName(approval.WorkflowPath)
		}
	} else {
		body = shortID + " is waiting for approval"
	}

	return &components.ShowToastMsg{
		Title: "Approval needed",
		Body:  body,
		Level: components.ToastLevelWarning,
		TTL:   15 * time.Second,
		ActionHints: []components.ActionHint{
			{Key: "a", Label: "approve"},
			{Key: "ctrl+a", Label: "view approvals"},
		},
	}
}

// workflowBaseName returns a short display name from a workflow file path.
// ".smithers/workflows/deploy-staging.tsx" → "deploy-staging"
func workflowBaseName(path string) string {
	base := filepath.Base(path)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	return base
}

// notificationTracker deduplicates Smithers event → toast mappings.
// The zero value is NOT usable; construct via newNotificationTracker.
type notificationTracker struct {
	mu            sync.Mutex
	seenRunStates map[string]smithers.RunStatus // runID → last toasted status
	seenApprovals map[string]struct{}            // approvalID → seen
}

// newNotificationTracker creates an initialized notificationTracker.
func newNotificationTracker() *notificationTracker {
	return &notificationTracker{
		seenRunStates: make(map[string]smithers.RunStatus),
		seenApprovals: make(map[string]struct{}),
	}
}

// shouldToastRunStatus returns true if this (runID, status) pair has not
// previously produced a toast. Records the pair on first call so subsequent
// duplicate calls for the same pair return false.
func (t *notificationTracker) shouldToastRunStatus(runID string, status smithers.RunStatus) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seenRunStates[runID] == status {
		return false
	}
	t.seenRunStates[runID] = status
	return true
}

// forgetRun removes a run from the dedup set. Should be called when the run
// reaches a terminal state so that future failures on a re-run can produce a
// fresh toast.
func (t *notificationTracker) forgetRun(runID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.seenRunStates, runID)
}

// shouldToastApproval returns true if this approvalID has not previously
// produced a toast. Records the ID on first call.
func (t *notificationTracker) shouldToastApproval(approvalID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, seen := t.seenApprovals[approvalID]; seen {
		return false
	}
	t.seenApprovals[approvalID] = struct{}{}
	return true
}

// runEventToToast translates a RunEvent into a ShowToastMsg.
// Returns nil if the event should not produce a toast (wrong type, duplicate,
// or uninteresting status).
func runEventToToast(ev smithers.RunEvent, tracker *notificationTracker) *components.ShowToastMsg {
	if ev.Type != "status_changed" {
		return nil
	}
	status := smithers.RunStatus(ev.Status)
	if !tracker.shouldToastRunStatus(ev.RunID, status) {
		return nil
	}

	shortID := ev.RunID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	switch status {
	case smithers.RunStatusWaitingApproval:
		// Approval toasts are handled asynchronously via fetchApprovalAndToastCmd
		// to include gate context. Return nil here; the caller emits the Cmd.
		return nil
	case smithers.RunStatusFailed:
		tracker.forgetRun(ev.RunID) // allow re-toast on a future failure
		return &components.ShowToastMsg{
			Title: "Run failed",
			Body:  shortID + " encountered an error",
			Level: components.ToastLevelError,
		}
	case smithers.RunStatusFinished:
		tracker.forgetRun(ev.RunID)
		return &components.ShowToastMsg{
			Title: "Run finished",
			Body:  shortID + " completed successfully",
			Level: components.ToastLevelSuccess,
		}
	case smithers.RunStatusCancelled:
		tracker.forgetRun(ev.RunID)
		return &components.ShowToastMsg{
			Title: "Run cancelled",
			Body:  shortID,
			Level: components.ToastLevelInfo,
		}
	}
	return nil
}
