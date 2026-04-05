package smithers

import (
	"context"
	"log/slog"
)

// WorkspaceContext holds pre-fetched workspace state injected into the agent
// system prompt at session start. It is a point-in-time snapshot; it does not
// refresh automatically during a session.
type WorkspaceContext struct {
	// ActiveRuns are non-terminal runs at the time of session creation.
	ActiveRuns []RunSummary
	// PendingApprovals is the count of runs currently in waiting-approval state.
	PendingApprovals int
}

// FetchWorkspaceContext fetches the current workspace state from the Smithers
// client. Errors are logged at debug level and result in a zero-value context
// so that the caller is never blocked by an unavailable Smithers server.
func FetchWorkspaceContext(ctx context.Context, c *Client) WorkspaceContext {
	if c == nil {
		return WorkspaceContext{}
	}

	// Active statuses we care about for the prompt.
	activeStatuses := []RunStatus{
		RunStatusRunning,
		RunStatusWaitingApproval,
		RunStatusWaitingEvent,
	}

	var allActive []RunSummary
	var pendingApprovals int

	for _, status := range activeStatuses {
		runs, err := c.ListRuns(ctx, RunFilter{Status: string(status), Limit: 20})
		if err != nil {
			slog.Debug("smithers: FetchWorkspaceContext: ListRuns failed",
				"status", status, "err", err)
			// Non-blocking: return what we have so far.
			return WorkspaceContext{
				ActiveRuns:       allActive,
				PendingApprovals: pendingApprovals,
			}
		}
		allActive = append(allActive, runs...)
		if status == RunStatusWaitingApproval {
			pendingApprovals = len(runs)
		}
	}

	return WorkspaceContext{
		ActiveRuns:       allActive,
		PendingApprovals: pendingApprovals,
	}
}
