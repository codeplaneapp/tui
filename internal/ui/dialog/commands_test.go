package dialog

import (
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commandTestWorkspace struct {
	workspace.Workspace
	cfg *config.Config
}

func (w *commandTestWorkspace) Config() *config.Config {
	return w.cfg
}

func TestCommands_DefaultCommands_HideTimeline(t *testing.T) {
	st := styles.DefaultStyles()
	com := &common.Common{
		Styles: &st,
		Workspace: &commandTestWorkspace{
			cfg: &config.Config{},
		},
	}

	cmds, err := NewCommands(com, "", false, false, false, nil, nil)
	require.NoError(t, err)

	var titles []string
	for _, item := range cmds.defaultCommands() {
		titles = append(titles, item.title)
	}

	assert.Contains(t, titles, "Work Items")
	assert.NotContains(t, titles, "Timeline")
}

func TestCommands_DefaultCommands_IncludeJJHubViews(t *testing.T) {
	st := styles.DefaultStyles()
	com := &common.Common{
		Styles: &st,
		Workspace: &commandTestWorkspace{
			cfg: &config.Config{},
		},
	}

	cmds, err := NewCommands(com, "", false, false, false, nil, nil)
	require.NoError(t, err)

	var titles []string
	for _, item := range cmds.defaultCommands() {
		titles = append(titles, item.title)
	}

	assert.Contains(t, titles, "JJHub Changes")
	assert.Contains(t, titles, "JJHub Status")
	assert.Contains(t, titles, "JJHub Issues")
	assert.Contains(t, titles, "JJHub Landings")
	assert.Contains(t, titles, "GitHub Pull Requests")
	assert.Contains(t, titles, "JJHub Workflows")
	assert.Contains(t, titles, "JJHub Workspaces")
	assert.Contains(t, titles, "JJHub Search")
}
