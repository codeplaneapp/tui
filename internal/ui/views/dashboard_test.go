package views

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDashboardView_HidesTimelineMenuItem(t *testing.T) {
	t.Parallel()

	view := NewDashboardView(smithers.NewClient(), true)

	var labels []string
	for _, item := range view.menuItems {
		labels = append(labels, item.label)
	}

	assert.NotContains(t, labels, "Timeline")
	assert.Contains(t, labels, "Tickets")
}

func TestDashboardView_FetchErrorStates(t *testing.T) {
	t.Parallel()

	t.Run("runs error sets runsErr and clears loading", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = true

		msg := dashRunsFetchedMsg{err: errors.New("runs fetch failed")}
		updated, cmd := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.runsLoading)
		assert.NotNil(t, dv.runsErr)
		assert.Contains(t, dv.runsErr.Error(), "runs fetch failed")
		assert.Nil(t, dv.runs)
		assert.Nil(t, cmd)
	})

	t.Run("workflows error sets wfErr and clears loading", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.wfLoading = true

		msg := dashWorkflowsFetchedMsg{err: errors.New("wf fetch failed")}
		updated, cmd := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.wfLoading)
		assert.NotNil(t, dv.wfErr)
		assert.Contains(t, dv.wfErr.Error(), "wf fetch failed")
		assert.Nil(t, dv.workflows)
		assert.Nil(t, cmd)
	})

	t.Run("approvals error sets approvalsErr and clears loading", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.approvalsLoading = true

		msg := dashApprovalsFetchedMsg{err: errors.New("approvals down")}
		updated, cmd := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.approvalsLoading)
		assert.NotNil(t, dv.approvalsErr)
		assert.Nil(t, dv.approvals)
		assert.Nil(t, cmd)
	})

	t.Run("error view renders fallback text for runs", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = false
		d.runsErr = errors.New("cannot reach smithers")
		d.wfLoading = false
		d.approvalsLoading = false

		output := d.View()
		assert.Contains(t, output, "No runs data")
	})

	t.Run("error view renders fallback text for workflows", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.wfLoading = false
		d.wfErr = errors.New("workflow error")
		d.runsLoading = false
		d.approvalsLoading = false

		output := d.View()
		assert.Contains(t, output, "No workflow data")
	})

	t.Run("landings error msg sets error and clears loading", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.landingsLoading = true

		msg := dashLandingsFetchedMsg{err: errors.New("landings broken")}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.landingsLoading)
		assert.NotNil(t, dv.landingsErr)
		assert.Nil(t, dv.landings)
	})

	t.Run("issues error msg sets error and clears loading", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.issuesLoading = true

		msg := dashIssuesFetchedMsg{err: errors.New("issues broken")}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.issuesLoading)
		assert.NotNil(t, dv.issuesErr)
		assert.Nil(t, dv.issues)
	})

	t.Run("workspaces error msg sets error and clears loading", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.workspacesLoading = true

		msg := dashWorkspacesFetchedMsg{err: errors.New("workspaces broken")}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.workspacesLoading)
		assert.NotNil(t, dv.workspacesErr)
		assert.Nil(t, dv.workspaces)
	})
}

func TestDashboardView_DataLoadingStates(t *testing.T) {
	t.Parallel()

	t.Run("loading state renders spinner text for runs", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = true
		d.wfLoading = true
		d.approvalsLoading = true

		output := d.View()
		assert.Contains(t, output, "Loading runs")
		assert.Contains(t, output, "Loading workflows")
		assert.Contains(t, output, "Loading approvals")
	})

	t.Run("successful runs fetch populates data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = true

		runs := []smithers.RunSummary{
			{RunID: "run-1", WorkflowName: "ci", Status: smithers.RunStatusRunning},
			{RunID: "run-2", WorkflowName: "deploy", Status: smithers.RunStatusFinished},
		}
		msg := dashRunsFetchedMsg{runs: runs}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.runsLoading)
		assert.Nil(t, dv.runsErr)
		require.Len(t, dv.runs, 2)
		assert.Equal(t, "run-1", dv.runs[0].RunID)
	})

	t.Run("successful workflows fetch populates data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.wfLoading = true

		wfs := []smithers.Workflow{
			{ID: "wf-1", Name: "Code Review"},
		}
		msg := dashWorkflowsFetchedMsg{workflows: wfs}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.wfLoading)
		assert.Nil(t, dv.wfErr)
		require.Len(t, dv.workflows, 1)
		assert.Equal(t, "Code Review", dv.workflows[0].Name)
	})

	t.Run("successful approvals fetch populates data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.approvalsLoading = true

		approvals := []smithers.Approval{
			{ID: "a-1", RunID: "run-1", Gate: "deploy-gate"},
		}
		msg := dashApprovalsFetchedMsg{approvals: approvals}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.approvalsLoading)
		assert.Nil(t, dv.approvalsErr)
		require.Len(t, dv.approvals, 1)
		assert.Equal(t, "deploy-gate", dv.approvals[0].Gate)
	})

	t.Run("successful landings fetch populates data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.landingsLoading = true

		landings := []jjhub.Landing{
			{Number: 1, Title: "Add feature", State: "open"},
		}
		msg := dashLandingsFetchedMsg{landings: landings}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.landingsLoading)
		assert.Nil(t, dv.landingsErr)
		require.Len(t, dv.landings, 1)
		assert.Equal(t, "Add feature", dv.landings[0].Title)
	})

	t.Run("successful issues fetch populates data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.issuesLoading = true

		issues := []jjhub.Issue{
			{Number: 42, Title: "Fix bug", State: "open"},
		}
		msg := dashIssuesFetchedMsg{issues: issues}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.issuesLoading)
		assert.Nil(t, dv.issuesErr)
		require.Len(t, dv.issues, 1)
		assert.Equal(t, 42, dv.issues[0].Number)
	})

	t.Run("successful workspaces fetch populates data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.workspacesLoading = true

		workspaces := []jjhub.Workspace{
			{ID: "ws-1", Name: "dev", Status: "running"},
		}
		msg := dashWorkspacesFetchedMsg{workspaces: workspaces}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.False(t, dv.workspacesLoading)
		assert.Nil(t, dv.workspacesErr)
		require.Len(t, dv.workspaces, 1)
		assert.Equal(t, "dev", dv.workspaces[0].Name)
	})

	t.Run("runs tab renders loaded data", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = false
		d.wfLoading = false
		d.approvalsLoading = false
		d.runs = []smithers.RunSummary{
			{RunID: "run-abc12345", WorkflowName: "code-review", Status: smithers.RunStatusRunning},
			{RunID: "run-def67890", WorkflowName: "deploy", Status: smithers.RunStatusFailed},
		}

		// Switch to the Runs tab
		d.Update(tea.KeyPressMsg{Code: '2'})
		output := d.View()
		assert.Contains(t, output, "Recent Runs")
		assert.Contains(t, output, "code-review")
	})

	t.Run("overview renders at-a-glance stats", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = false
		d.wfLoading = false
		d.approvalsLoading = false
		d.runs = []smithers.RunSummary{
			{RunID: "r1", Status: smithers.RunStatusRunning},
			{RunID: "r2", Status: smithers.RunStatusFinished},
		}
		d.workflows = []smithers.Workflow{
			{ID: "w1", Name: "CI"},
		}

		output := d.View()
		assert.Contains(t, output, "Runs: 2 total")
		assert.Contains(t, output, "Workflows: 1 available")
	})

	t.Run("repo name msg populates header", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40

		msg := dashRepoNameFetchedMsg{name: "acme/monorepo"}
		updated, _ := d.Update(msg)
		dv := updated.(*DashboardView)

		assert.Equal(t, "acme/monorepo", dv.repoName)

		output := dv.View()
		assert.Contains(t, output, "acme/monorepo")
	})

	t.Run("refresh key resets loading flags", func(t *testing.T) {
		t.Parallel()

		d := NewDashboardView(smithers.NewClient(), true)
		d.width = 120
		d.height = 40
		d.runsLoading = false
		d.wfLoading = false

		updated, cmd := d.Update(tea.KeyPressMsg{Code: 'r'})
		dv := updated.(*DashboardView)

		assert.True(t, dv.runsLoading)
		assert.True(t, dv.wfLoading)
		require.NotNil(t, cmd, "refresh should return batch command")
	})
}
