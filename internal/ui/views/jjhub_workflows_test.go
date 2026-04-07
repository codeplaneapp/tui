package views

import (
	"testing"

	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/stretchr/testify/require"
)

func TestJJHubWorkflowsView_OpenRunPromptUsesRepoDefaultBookmark(t *testing.T) {
	v := newJJHubWorkflowsViewWithClient(nil)
	v.repo = &jjhub.Repo{DefaultBookmark: "trunk"}
	v.workflows = []jjhub.Workflow{{ID: 1, Name: "Deploy"}}

	cmd := v.openRunPrompt()

	require.NotNil(t, cmd)
	require.True(t, v.prompt.active)
	require.Equal(t, "trunk", v.prompt.ref)
	require.Equal(t, "trunk", v.prompt.input.Value())
}

func TestJJHubWorkflowsView_OpenRunPromptFallsBackToMain(t *testing.T) {
	v := newJJHubWorkflowsViewWithClient(nil)
	v.workflows = []jjhub.Workflow{{ID: 1, Name: "Deploy"}}

	cmd := v.openRunPrompt()

	require.NotNil(t, cmd)
	require.Equal(t, "main", v.prompt.ref)
	require.Equal(t, "main", v.prompt.input.Value())
}
