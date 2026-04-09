package model

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/attachments"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/views"
	"github.com/charmbracelet/crush/internal/workspace"
	uv "github.com/charmbracelet/ultraviolet"
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

func TestHandleKeyPressMsg_ViewApprovalsShort_DoesNotStealSmithersViewInput(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.viewRouter = views.NewRouter()
	view := &shortcutCaptureView{}
	ui.viewRouter.Push(view, ui.width, ui.height)
	ui.state = uiSmithersView
	ui.focus = uiFocusMain

	cmd := ui.handleKeyPressMsg(tea.KeyPressMsg{Code: 'a'})

	require.Nil(t, cmd)
	require.True(t, view.receivedKey, "expected smithers view to receive bare 'a'")
}

func TestShortHelp_IncludesSmithersShortcutBindingsInChat(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.focus = uiFocusMain

	bindings := ui.ShortHelp()
	assertHasHelpBinding(t, bindings, "ctrl+r", "runs")
	assertHasHelpBinding(t, bindings, "ctrl+a", "approvals")
	assertHasHelpBinding(t, bindings, "ctrl+b", "sidebar")
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
	require.Contains(t, bindings, keyHelp{key: "ctrl+b", desc: "sidebar"})
	require.Contains(t, bindings, keyHelp{key: "alt+1-9", desc: "tabs"})
	require.Contains(t, bindings, keyHelp{key: "alt+h", desc: "prev tab"})
	require.Contains(t, bindings, keyHelp{key: "alt+l", desc: "next tab"})
}

func TestHandleKeyPressMsg_AltTabNumberNavigatesWhileEditorFocused(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUIWithTabs()
	ui.focus = uiFocusEditor

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: '2', Mod: tea.ModAlt})
	require.Equal(t, 1, ui.tabManager.ActiveIndex())
	require.Equal(t, uiSmithersView, ui.state)
	require.Equal(t, uiFocusMain, ui.focus)
}

func TestHandleKeyPressMsg_TabCyclesChatFocusThroughNavSidebar(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUIWithTabs()
	ui.focus = uiFocusEditor

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, uiFocusMain, ui.focus)

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, uiFocusNav, ui.focus)

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	require.Equal(t, uiFocusMain, ui.focus)
}

func TestHandleKeyPressMsg_NavFocusSwitchesTabs(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUIWithTabs()
	ui.focus = uiFocusNav
	ui.state = uiChat

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	require.Equal(t, 1, ui.tabManager.ActiveIndex())
	require.Equal(t, uiSmithersView, ui.state)
	require.Equal(t, uiFocusNav, ui.focus)

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, uiFocusMain, ui.focus)
}

func TestHandleNavMouseClick_ActivatesClickedTab(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUIWithTabs()
	ui.layout.navSidebar = uv.Rect(0, 0, navSidebarWidth, 12)

	handled, _ := ui.handleNavMouseClick(tea.MouseClickMsg{X: 1, Y: navSidebarTabsStartRow + 1})
	require.True(t, handled)
	require.Equal(t, 1, ui.tabManager.ActiveIndex())
	require.Equal(t, uiSmithersView, ui.state)
}

func TestHandleNavMouseClick_ShowHandleFocusesNav(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUIWithTabs()
	ui.navSidebarHidden = true
	ui.layout.navToggle = uv.Rect(0, 0, navToggleWidth, 6)

	handled, _ := ui.handleNavMouseClick(tea.MouseClickMsg{X: 0, Y: 0})
	require.True(t, handled)
	require.False(t, ui.navSidebarHidden)
	require.Equal(t, uiFocusNav, ui.focus)
}

func TestHandleNavigateToView_EmptyViewIsNoop(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	cmd := ui.handleNavigateToView(NavigateToViewMsg{View: ""})
	require.Nil(t, cmd, "empty view name should return nil cmd")
}

func TestHandleNavigateToView_RunsView_IsHandled(t *testing.T) {
	t.Parallel()

	// Verify that the "runs" view name is handled by the runs-specific case
	// (not falling through to the registry/coming-soon branch).
	// We can't call handleNavigateToView directly here because it requires a
	// fully wired UI (viewRouter, viewRegistry). Instead, verify the key-press
	// path sends a NavigateToViewMsg with View="runs".
	ui := newShortcutTestUI()
	cmd := ui.handleKeyPressMsg(tea.KeyPressMsg{
		Code: 'r',
		Mod:  tea.ModCtrl,
	})
	require.NotNil(t, cmd)
	msg := cmd()
	navigateMsg, ok := msg.(NavigateToViewMsg)
	require.True(t, ok, "Ctrl+R should produce a NavigateToViewMsg")
	require.Equal(t, "runs", navigateMsg.View)
}

type keyHelp struct {
	key  string
	desc string
}

type mockWorkspace struct {
	workspace.Workspace
}

func (m *mockWorkspace) AgentIsReady() bool { return false }
func (m *mockWorkspace) AgentIsBusy() bool  { return false }

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

func assertLacksHelpBinding(t *testing.T, bindings []key.Binding, key, desc string) {
	t.Helper()

	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == key && help.Desc == desc {
			t.Fatalf("unexpected help binding: %q %q", key, desc)
		}
	}
}

func newShortcutTestUI() *UI {
	keyMap := DefaultKeyMap()
	st := styles.DefaultStyles()
	ta := textarea.New()
	ta.SetStyles(st.TextArea)
	ta.ShowLineNumbers = false
	ta.CharLimit = -1
	ta.SetVirtualCursor(false)
	ta.DynamicHeight = true
	ta.MinHeight = TextareaMinHeight
	ta.MaxHeight = TextareaMaxHeight
	ta.Focus()

	com := &common.Common{
		Styles:    &st,
		Workspace: &mockWorkspace{},
	}

	return &UI{
		com: com,
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
		dialog:       dialog.NewOverlay(&st),
		keyMap:       keyMap,
		state:        uiChat,
		focus:        uiFocusEditor,
		dashboard:    views.NewDashboardView(com, nil, false),
		tabManager:   NewTabManager(),
		viewRegistry: views.DefaultRegistry(),
		textarea:     ta,
		status:       NewStatus(com, nil),
		chat:         NewChat(com),
		width:        140,
		height:       45,
	}
}

func newShortcutTestUIWithTabs() *UI {
	ui := newShortcutTestUI()
	tab := &WorkspaceTab{
		ID:       "view:capture",
		Kind:     TabKindView,
		Label:    "Capture",
		Closable: true,
		Router:   views.NewRouter(),
	}
	tab.Router.Push(&shortcutCaptureView{}, ui.width, ui.height)
	tab.initialized = true
	ui.tabManager.Add(tab)
	return ui
}

type shortcutCaptureView struct {
	receivedKey bool
}

func (v *shortcutCaptureView) Init() tea.Cmd { return nil }

func (v *shortcutCaptureView) Update(msg tea.Msg) (views.View, tea.Cmd) {
	if _, ok := msg.(tea.KeyPressMsg); ok {
		v.receivedKey = true
	}
	return v, nil
}

func (v *shortcutCaptureView) View() string { return "" }

func (v *shortcutCaptureView) Name() string { return "capture" }

func (v *shortcutCaptureView) SetSize(_, _ int) {}

func (v *shortcutCaptureView) ShortHelp() []key.Binding { return nil }
