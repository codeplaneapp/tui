package model

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/components"
	dn "github.com/charmbracelet/crush/internal/ui/diffnav"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/util"
	"github.com/charmbracelet/crush/internal/ui/views"
	"github.com/stretchr/testify/require"
)

type popOnEscView struct{}

func (v *popOnEscView) Init() tea.Cmd { return nil }

func (v *popOnEscView) Update(msg tea.Msg) (views.View, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return v, nil
	}
	if keyMsg.Code == tea.KeyEscape {
		return v, func() tea.Msg { return views.PopViewMsg{} }
	}
	return v, nil
}

func (v *popOnEscView) View() string { return "" }

func (v *popOnEscView) Name() string { return "pop-on-esc" }

func (v *popOnEscView) SetSize(width, height int) {}

func (v *popOnEscView) ShortHelp() []key.Binding { return nil }

func TestUI_ShowToastMsgFallsBackToStatusWhenToastsDisabled(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.focus = uiFocusNone
	ui.toasts = nil

	_, cmd := ui.Update(components.ShowToastMsg{
		Title: "diffnav not installed",
		Body:  "Install diffnav to view diffs? (y to install)",
		Level: components.ToastLevelWarning,
	})
	require.NotNil(t, cmd)

	msg := cmd()
	infoMsg, ok := msg.(util.InfoMsg)
	require.True(t, ok, "expected util.InfoMsg, got %T", msg)
	require.Equal(t, util.InfoTypeWarn, infoMsg.Type)
	require.Equal(t, "diffnav not installed: Install diffnav to view diffs? (y to install)", infoMsg.Msg)
}

func TestHandleKeyPressMsg_PendingDiffInstallDismissesAndForwardsEscape(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.state = uiSmithersView
	ui.focus = uiFocusMain
	ui.pendingDiffInstall = &dn.InstallPromptMsg{PendingCommand: "jjhub change diff abc123"}
	ui.viewRouter = views.NewRouter()
	ui.viewRouter.Push(&popOnEscView{}, 80, 24)

	cmd := ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Nil(t, ui.pendingDiffInstall)
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(views.PopViewMsg)
	require.True(t, ok, "expected views.PopViewMsg, got %T", msg)
}

func TestUI_PopViewMsgFromSingleSmithersViewReturnsToDashboard(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.state = uiSmithersView
	ui.focus = uiFocusMain
	ui.width = 80
	ui.height = 24
	st := styles.DefaultStyles()
	ui.com.Styles = &st
	ui.chat = NewChat(ui.com)
	ui.status = NewStatus(ui.com, ui)
	ui.textarea = textarea.New()
	ui.dashboard = &views.DashboardView{}
	ui.viewRouter = views.NewRouter()
	ui.viewRouter.Push(&popOnEscView{}, ui.width, ui.height)

	model, cmd := ui.Update(views.PopViewMsg{})
	require.Nil(t, cmd)

	updated := model.(*UI)
	require.Equal(t, uiSmithersDashboard, updated.state)
	require.False(t, updated.viewRouter.HasViews())
}

func TestPagerFallbackTitleDetectsCrash(t *testing.T) {
	t.Parallel()

	require.Equal(t, "diffnav crashed", pagerFallbackTitle("Caught panic: divide by zero"))
	require.Equal(t, "diffnav failed", pagerFallbackTitle("exit status 1"))
}

func TestPagerFallbackBodyIncludesSummary(t *testing.T) {
	t.Parallel()

	body := pagerFallbackBody("FATA program was killed: program experienced a panic\nstack trace")
	require.Contains(t, body, "Opening raw diff pager instead.")
	require.Contains(t, body, "program was killed: program experienced a panic")
}
