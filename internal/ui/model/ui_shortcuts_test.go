package model

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/attachments"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/charmbracelet/crush/internal/ui/util"
	"github.com/stretchr/testify/require"
)

func TestHandleKeyPressMsg_NavigateShortcuts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		keyCode   rune
		wantView  string
		keystroke string
	}{
		{name: "runs", keyCode: 'r', wantView: "runs", keystroke: "ctrl+r"},
		{name: "approvals", keyCode: 'a', wantView: "approvals", keystroke: "ctrl+a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ui := newShortcutTestUI()
			cmd := ui.handleKeyPressMsg(tea.KeyPressMsg{
				Code: tt.keyCode,
				Mod:  tea.ModCtrl,
			})
			require.NotNil(t, cmd)

			msg := cmd()
			navigateMsg, ok := msg.(NavigateToViewMsg)
			require.Truef(t, ok, "expected NavigateToViewMsg for %s, got %T", tt.keystroke, msg)
			require.Equal(t, tt.wantView, navigateMsg.View)
		})
	}
}

func TestShortHelp_IncludesSmithersShortcutBindings(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.focus = uiFocusMain

	bindings := ui.ShortHelp()
	assertHasHelpBinding(t, bindings, "ctrl+r", "runs")
	assertHasHelpBinding(t, bindings, "ctrl+a", "approvals")
}

func TestFullHelp_IncludesSmithersShortcutBindings(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.focus = uiFocusMain

	var bindings []keyHelp
	for _, row := range ui.FullHelp() {
		for _, binding := range row {
			help := binding.Help()
			bindings = append(bindings, keyHelp{key: help.Key, desc: help.Desc})
		}
	}

	require.Contains(t, bindings, keyHelp{key: "ctrl+r", desc: "runs"})
	require.Contains(t, bindings, keyHelp{key: "ctrl+a", desc: "approvals"})
}

func TestHandleNavigateToView_UsesComingSoonFallback(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	cmd := ui.handleNavigateToView(NavigateToViewMsg{View: "runs"})
	require.NotNil(t, cmd)

	msg := cmd()
	infoMsg, ok := msg.(util.InfoMsg)
	require.True(t, ok)
	require.Equal(t, "runs view coming soon", infoMsg.Msg)
}

type keyHelp struct {
	key  string
	desc string
}

func assertHasHelpBinding(t *testing.T, bindings []key.Binding, key, desc string) {
	t.Helper()

	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == key && help.Desc == desc {
			return
		}
	}

	t.Fatalf("missing help binding: %q %q", key, desc)
}

func newShortcutTestUI() *UI {
	keyMap := DefaultKeyMap()
	return &UI{
		com: &common.Common{},
		attachments: attachments.New(
			attachments.NewRenderer(
				lipgloss.NewStyle(),
				lipgloss.NewStyle(),
				lipgloss.NewStyle(),
				lipgloss.NewStyle(),
			),
			attachments.Keymap{
				DeleteMode: keyMap.Editor.AttachmentDeleteMode,
				DeleteAll:  keyMap.Editor.DeleteAllAttachments,
				Escape:     keyMap.Editor.Escape,
			},
		),
		dialog: dialog.NewOverlay(),
		keyMap: keyMap,
		state:  uiChat,
		focus:  uiFocusEditor,
	}
}
