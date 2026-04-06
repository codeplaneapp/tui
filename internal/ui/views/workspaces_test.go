package views

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleWorkspace(name, status string) jjhub.Workspace {
	sshHost := name + ".jjhub.tech"
	return jjhub.Workspace{
		ID:                 name + "-id",
		Name:               name,
		Status:             status,
		SSHHost:            &sshHost,
		IsFork:             true,
		IdleTimeoutSeconds: 1800,
		FreestyleVMID:      "vm-123",
		CreatedAt:          time.Now().Add(-3 * time.Hour).Format(time.RFC3339),
		UpdatedAt:          time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
	}
}

func newTestWorkspacesView() *WorkspacesView {
	return NewWorkspacesView(smithers.NewClient())
}

func seedWorkspacesView(v *WorkspacesView, workspaces []jjhub.Workspace) *WorkspacesView {
	updated, _ := v.Update(workspacesLoadedMsg{workspaces: workspaces})
	return updated.(*WorkspacesView)
}

func TestWorkspacesView_ImplementsView(t *testing.T) {
	t.Parallel()
	var _ View = (*WorkspacesView)(nil)
}

func TestWorkspacesView_SearchApply(t *testing.T) {
	t.Parallel()

	v := seedWorkspacesView(newTestWorkspacesView(), []jjhub.Workspace{
		sampleWorkspace("alpha", "running"),
		sampleWorkspace("beta", "stopped"),
	})
	v.search.active = true
	v.search.input.SetValue("beta")

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Equal(t, "beta", wv.searchQuery)
	assert.Len(t, wv.workspaces, 1)
	assert.Equal(t, "beta", wv.workspaces[0].Name)
}

func TestWorkspacesView_EnterReturnsSSHCmd(t *testing.T) {
	t.Parallel()

	v := seedWorkspacesView(newTestWorkspacesView(), []jjhub.Workspace{sampleWorkspace("alpha", "running")})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Same(t, v, updated)
	require.NotNil(t, cmd)
}

func TestWorkspacesView_SPendingReturnsWarningToast(t *testing.T) {
	t.Parallel()

	v := seedWorkspacesView(newTestWorkspacesView(), []jjhub.Workspace{sampleWorkspace("alpha", "pending")})

	_, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, components.ToastLevelWarning, toast.Level)
	assert.Contains(t, toast.Title, "pending")
}

func TestWorkspacesView_RenderPreviewIncludesSSHHost(t *testing.T) {
	t.Parallel()

	v := seedWorkspacesView(newTestWorkspacesView(), []jjhub.Workspace{sampleWorkspace("alpha", "running")})

	content := v.renderPreview(v.workspaces[0])
	assert.Contains(t, content, "alpha.jjhub.tech")
}
