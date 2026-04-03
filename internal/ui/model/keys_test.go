package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultKeyMap_SmithersHelpbarShortcuts(t *testing.T) {
	t.Parallel()

	km := DefaultKeyMap()

	require.Equal(t, []string{"ctrl+r"}, km.RunDashboard.Keys())
	require.Equal(t, "ctrl+r", km.RunDashboard.Help().Key)
	require.Equal(t, "runs", km.RunDashboard.Help().Desc)

	require.Equal(t, []string{"ctrl+a"}, km.Approvals.Keys())
	require.Equal(t, "ctrl+a", km.Approvals.Help().Key)
	require.Equal(t, "approvals", km.Approvals.Help().Desc)
}

func TestDefaultKeyMap_AttachmentDeleteModeMovedOffCtrlR(t *testing.T) {
	t.Parallel()

	km := DefaultKeyMap()

	require.Equal(t, []string{"ctrl+shift+r"}, km.Editor.AttachmentDeleteMode.Keys())
	require.Equal(t, "ctrl+shift+r+{i}", km.Editor.AttachmentDeleteMode.Help().Key)
	require.Equal(t, "ctrl+shift+r+r", km.Editor.DeleteAllAttachments.Help().Key)
}
