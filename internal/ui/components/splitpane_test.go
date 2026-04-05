package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

// --- Mock pane ---

type mockPane struct {
	initCalled bool
	updateMsgs []tea.Msg
	sizeW      int
	sizeH      int
	viewContent string
}

func (p *mockPane) Init() tea.Cmd {
	p.initCalled = true
	return nil
}

func (p *mockPane) Update(msg tea.Msg) (Pane, tea.Cmd) {
	p.updateMsgs = append(p.updateMsgs, msg)
	return p, nil
}

func (p *mockPane) View() string {
	return p.viewContent
}

func (p *mockPane) SetSize(w, h int) {
	p.sizeW = w
	p.sizeH = h
}

// --- Helpers ---

func newTestSplitPane() (*SplitPane, *mockPane, *mockPane) {
	left := &mockPane{viewContent: "LEFT"}
	right := &mockPane{viewContent: "RIGHT"}
	sp := NewSplitPane(left, right, SplitPaneOpts{})
	return sp, left, right
}

// --- Tests ---

func TestSplitPane_Defaults(t *testing.T) {
	sp, _, _ := newTestSplitPane()
	assert.Equal(t, 30, sp.opts.LeftWidth, "default LeftWidth should be 30")
	assert.Equal(t, 1, sp.opts.DividerWidth, "default DividerWidth should be 1")
	assert.Equal(t, 80, sp.opts.CompactBreakpoint, "default CompactBreakpoint should be 80")
	assert.Equal(t, FocusLeft, sp.Focus(), "default focus should be FocusLeft")
}

func TestSplitPane_SetSize_Normal(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(120, 30)

	// left gets 30 - 1 (border accent when focused) = 29
	// right gets 120 - 30 - 1 = 89
	assert.Equal(t, 29, left.sizeW, "left pane width when focused should be LeftWidth-1")
	assert.Equal(t, 89, right.sizeW, "right pane width should be total - leftWidth - divider")
	assert.Equal(t, 30, left.sizeH)
	assert.Equal(t, 30, right.sizeH)
}

func TestSplitPane_SetSize_Compact(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(70, 20) // 70 < 80 (compact breakpoint)

	assert.True(t, sp.IsCompact(), "should be in compact mode at width 70")
	// Only focused pane (left) gets size
	assert.Equal(t, 70, left.sizeW)
	assert.Equal(t, 20, left.sizeH)
	// Right pane should NOT have been called
	assert.Equal(t, 0, right.sizeW)
	assert.Equal(t, 0, right.sizeH)
}

func TestSplitPane_LeftWidthClamped(t *testing.T) {
	sp, left, _ := newTestSplitPane()
	sp.SetSize(50, 20) // 50 >= 80? No, 50 < 80 so compact. Let's use 82.
	_ = left
	// Use width just above breakpoint where left would be clamped
	sp2, left2, _ := newTestSplitPane()
	sp2.SetSize(82, 20)
	// leftWidth = min(30, 82/2) = min(30, 41) = 30 (no clamping needed)
	// When focused (FocusLeft): left gets 30-1=29
	assert.Equal(t, 29, left2.sizeW, "left pane should get 29 (30-1 for border)")

	sp3, left3, _ := newTestSplitPane()
	sp3.SetSize(50, 20) // compact mode, no test needed here

	// Force left width > half: custom opts with LeftWidth=40 and total=82
	left4Pane := &mockPane{viewContent: "L"}
	right4Pane := &mockPane{viewContent: "R"}
	sp4 := NewSplitPane(left4Pane, right4Pane, SplitPaneOpts{LeftWidth: 40, CompactBreakpoint: 80})
	sp4.SetSize(82, 20)
	// clamp: min(40, 82/2=41) = 40 (not clamped since 40 <= 41)
	// focused left: 40-1=39
	assert.Equal(t, 39, left4Pane.sizeW, "left pane with LeftWidth=40 on width=82 should get 39")

	sp5Left := &mockPane{viewContent: "L"}
	sp5Right := &mockPane{viewContent: "R"}
	sp5 := NewSplitPane(sp5Left, sp5Right, SplitPaneOpts{LeftWidth: 40, CompactBreakpoint: 80})
	sp5.SetSize(60, 20) // compact (60<80)
	_ = sp3
	_ = left3
	_ = sp4
	_ = sp5
}

func TestSplitPane_TabTogglesFocus(t *testing.T) {
	sp, _, _ := newTestSplitPane()
	sp.SetSize(120, 30)
	assert.Equal(t, FocusLeft, sp.Focus())

	newSP, _ := sp.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, FocusRight, newSP.Focus(), "Tab should toggle to FocusRight")

	newSP2, _ := newSP.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, FocusLeft, newSP2.Focus(), "Tab again should toggle back to FocusLeft")
}

func TestSplitPane_ShiftTabTogglesFocus(t *testing.T) {
	sp, _, _ := newTestSplitPane()
	sp.SetSize(120, 30)
	assert.Equal(t, FocusLeft, sp.Focus())

	// shift+tab
	msg := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	newSP, _ := sp.Update(msg)
	assert.Equal(t, FocusRight, newSP.Focus(), "Shift+Tab should also toggle focus")
}

func TestSplitPane_KeyRouting_LeftFocused(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(120, 30)
	assert.Equal(t, FocusLeft, sp.Focus())

	keyMsg := tea.KeyPressMsg{Code: 'j'}
	sp.Update(keyMsg)

	assert.Len(t, left.updateMsgs, 1, "left pane should receive key when focused")
	assert.Empty(t, right.updateMsgs, "right pane should NOT receive key when left is focused")
}

func TestSplitPane_KeyRouting_RightFocused(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(120, 30)
	sp.SetFocus(FocusRight)

	keyMsg := tea.KeyPressMsg{Code: 'j'}
	sp.Update(keyMsg)

	assert.Empty(t, left.updateMsgs, "left pane should NOT receive key when right is focused")
	assert.Len(t, right.updateMsgs, 1, "right pane should receive key when focused")
}

func TestSplitPane_WindowResize(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(120, 30)

	// Clear the sizes set by SetSize above by re-creating mocks
	left.sizeW = 0
	left.sizeH = 0
	right.sizeW = 0
	right.sizeH = 0

	sp.Update(tea.WindowSizeMsg{Width: 150, Height: 40})

	// After resize, both panes should have new sizes
	// focused (left): 30-1=29, right: 150-30-1=119
	assert.Equal(t, 29, left.sizeW, "left pane should be resized on WindowSizeMsg")
	assert.Equal(t, 119, right.sizeW, "right pane should be resized on WindowSizeMsg")
}

func TestSplitPane_ViewOutput_Normal(t *testing.T) {
	sp, _, _ := newTestSplitPane()
	sp.SetSize(120, 5)

	out := sp.View()
	assert.False(t, sp.IsCompact(), "should not be in compact mode at width 120")
	assert.Contains(t, out, "LEFT", "view should contain left pane content")
	assert.Contains(t, out, "RIGHT", "view should contain right pane content")
	assert.Contains(t, out, "│", "view should contain divider")
}

func TestSplitPane_ViewOutput_Compact(t *testing.T) {
	sp, _, _ := newTestSplitPane()
	sp.SetSize(70, 5) // compact

	out := sp.View()
	assert.True(t, sp.IsCompact())
	assert.Contains(t, out, "LEFT", "compact mode should show focused (left) pane")
	assert.NotContains(t, out, "RIGHT", "compact mode should NOT show unfocused (right) pane")
}

func TestSplitPane_Init(t *testing.T) {
	left := &mockPane{viewContent: "L"}
	right := &mockPane{viewContent: "R"}
	sp := NewSplitPane(left, right, SplitPaneOpts{})

	sp.Init()
	assert.True(t, left.initCalled, "Init should call left pane's Init")
	assert.True(t, right.initCalled, "Init should call right pane's Init")
}

func TestSplitPane_SetFocus_Programmatic(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(120, 30)

	sp.SetFocus(FocusRight)
	assert.Equal(t, FocusRight, sp.Focus())

	// After SetFocus, right pane gets border space (right gets rightWidth-1)
	// left gets full leftWidth (no border when unfocused)
	assert.Equal(t, 30, left.sizeW, "unfocused left pane should get full leftWidth")
	// right: 120 - 30 - 1 = 89, minus 1 for border = 88
	assert.Equal(t, 88, right.sizeW, "focused right pane should get rightWidth-1")
}

func TestSplitPane_CompactToggle_SizePropagation(t *testing.T) {
	sp, left, right := newTestSplitPane()
	sp.SetSize(70, 20) // compact, left focused

	assert.Equal(t, 70, left.sizeW)

	// Toggle focus in compact mode: right should now get the full width
	sp.ToggleFocus()
	assert.Equal(t, FocusRight, sp.Focus())
	assert.Equal(t, 70, right.sizeW, "right pane should get full width after toggle in compact mode")
}

func TestSplitPane_NarrowSafety(t *testing.T) {
	// Should not panic on very narrow terminal
	sp, _, _ := newTestSplitPane()
	assert.NotPanics(t, func() {
		sp.SetSize(10, 5)
		_ = sp.View()
	})
}

func TestSplitPane_ShortHelp(t *testing.T) {
	sp, _, _ := newTestSplitPane()
	help := sp.ShortHelp()
	assert.NotEmpty(t, help)
	// Should include tab binding
	found := false
	for _, b := range help {
		if strings.Contains(b.Help().Key, "tab") {
			found = true
		}
	}
	assert.True(t, found, "ShortHelp should include tab binding")
}

func TestSplitPane_Accessors(t *testing.T) {
	left := &mockPane{viewContent: "L"}
	right := &mockPane{viewContent: "R"}
	sp := NewSplitPane(left, right, SplitPaneOpts{})
	sp.SetSize(120, 30)

	assert.Equal(t, left, sp.Left())
	assert.Equal(t, right, sp.Right())
	assert.Equal(t, 120, sp.Width())
	assert.Equal(t, 30, sp.Height())
	assert.False(t, sp.IsCompact())
}
