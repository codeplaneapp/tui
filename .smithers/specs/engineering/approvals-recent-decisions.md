# Implementation Plan: approvals-recent-decisions

## Ticket Summary
Display a history of recently approved or denied gates in the approvals view. Each entry shows the decision made and timestamps.

## Acceptance Criteria
- A section or toggleable view shows historical decisions
- Each entry shows the decision made and timestamps

## Implementation Plan

### 1. Extend Smithers Client Types (`internal/smithers/types.go`)
- Add an `ApprovalDecision` struct with fields: `ID`, `GateID`, `GateName`, `WorkflowName`, `RunID`, `Decision` (approved/denied), `DecidedAt`, `DecidedBy`
- Add a `ListApprovalDecisionsResponse` type

### 2. Add Client Method (`internal/smithers/client.go`)
- Add `ListRecentDecisions(ctx, limit int) ([]ApprovalDecision, error)` method that queries the Smithers API for historical approval decisions
- Include proper error handling and timeout support

### 3. Update Approvals View Model (`internal/ui/views/approvals.go`)
- Add a `recentDecisions []smithers.ApprovalDecision` field to the approvals view model
- Add a `showRecent bool` toggle to switch between pending queue and recent decisions
- Add a `decisionsCursor int` for navigating the decisions list
- Fetch recent decisions on view init and on refresh

### 4. Add Keybindings and Commands
- `tab` to toggle between pending approvals and recent decisions view
- `j/k` or arrow keys to navigate the decisions list
- `r` to refresh the decisions list
- Update the help bar to show available actions for the recent decisions view

### 5. Render Recent Decisions Section
- Render a table/list showing each decision with columns: Gate Name, Workflow, Decision (Approved/Denied with color coding), Timestamp (relative time)
- Use green for approved, red for denied decisions
- Show "No recent decisions" placeholder when the list is empty
- Style with Lip Gloss consistent with the existing approvals view

### 6. Add Tests (`internal/smithers/client_test.go`)
- Test `ListRecentDecisions` client method with mock HTTP responses
- Test view model state transitions between pending and recent views
- Test rendering output for various decision states

## Files to Create/Modify
- `internal/smithers/types.go` — Add ApprovalDecision type
- `internal/smithers/client.go` — Add ListRecentDecisions method
- `internal/ui/views/approvals.go` — Add recent decisions tab/section with rendering
- `internal/smithers/client_test.go` — Add tests for new client method

## Dependencies
- eng-approvals-view-scaffolding (must be completed first — provides the base approvals view)

## Feature Flag
- `APPROVALS_RECENT_DECISIONS`