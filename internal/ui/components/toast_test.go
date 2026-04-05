package components_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newManager creates a ToastManager backed by the default style set.
func newManager() *components.ToastManager {
	s := styles.DefaultStyles()
	return components.NewToastManager(&s)
}

// drawToString draws the toast manager onto a fresh virtual screen and returns
// the plain-text content of the buffer.
func drawToString(m *components.ToastManager, w, h int) string {
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	scr := uv.NewScreen(w, h)
	m.Draw(scr, scr.Bounds())
	return scr.Buffer.String()
}

// ---------------------------------------------------------------------------
// Update / lifecycle
// ---------------------------------------------------------------------------

func TestToastManager_InitiallyEmpty(t *testing.T) {
	t.Parallel()
	m := newManager()
	assert.Equal(t, 0, m.Len(), "new manager should have no toasts")
}

func TestToastManager_ShowToastAddsEntry(t *testing.T) {
	t.Parallel()
	m := newManager()
	cmd := m.Update(components.ShowToastMsg{Title: "hello"})
	require.NotNil(t, cmd, "ShowToastMsg should return a TTL command")
	assert.Equal(t, 1, m.Len())
}

func TestToastManager_ShowToastMultiple(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "first"})
	m.Update(components.ShowToastMsg{Title: "second"})
	assert.Equal(t, 2, m.Len())
}

func TestToastManager_DismissRemovesToast(t *testing.T) {
	t.Parallel()
	m := newManager()

	// ID assignment begins at 1 for the first toast.
	m.Update(components.ShowToastMsg{Title: "bye"})
	require.Equal(t, 1, m.Len())

	m.Update(components.DismissToastMsg{ID: 1})
	assert.Equal(t, 0, m.Len())
}

func TestToastManager_DismissUnknownIDIsNoop(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "keep me"})
	m.Update(components.DismissToastMsg{ID: 9999})
	assert.Equal(t, 1, m.Len(), "dismiss of unknown ID should not remove other toasts")
}

func TestToastManager_BoundedStackEvictsOldest(t *testing.T) {
	t.Parallel()
	m := newManager()
	for i := range components.MaxVisibleToasts + 2 {
		m.Update(components.ShowToastMsg{Title: strings.Repeat("x", i+1)})
	}
	assert.Equal(t, components.MaxVisibleToasts, m.Len(),
		"stack must be capped at MaxVisibleToasts")
}

func TestToastManager_ClearRemovesAll(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "a"})
	m.Update(components.ShowToastMsg{Title: "b"})
	m.Clear()
	assert.Equal(t, 0, m.Len())
}

func TestToastManager_TimedOutMsgDismissesToast(t *testing.T) {
	t.Parallel()
	m := newManager()
	cmd := m.Update(components.ShowToastMsg{Title: "fast", TTL: 1 * time.Millisecond})
	require.NotNil(t, cmd)

	// Wait briefly so the tick fires, then execute the command synchronously.
	time.Sleep(10 * time.Millisecond)
	msg := cmd()
	require.NotNil(t, msg)

	// Feed the timed-out message back to the manager.
	m.Update(msg.(tea.Msg))
	assert.Equal(t, 0, m.Len(), "toast should be gone after TTL fires")
}

func TestToastManager_CustomTTLOverridesDefault(t *testing.T) {
	t.Parallel()
	m := newManager()
	cmd := m.Update(components.ShowToastMsg{Title: "custom", TTL: 42 * time.Second})
	require.NotNil(t, cmd)
	assert.Equal(t, 1, m.Len())
}

// ---------------------------------------------------------------------------
// Rendering / Draw
// ---------------------------------------------------------------------------

func TestToastManager_DrawEmptyIsNoop(t *testing.T) {
	t.Parallel()
	m := newManager()
	// Should not panic when no toasts are present.
	assert.NotPanics(t, func() {
		_ = drawToString(m, 80, 24)
	})
}

func TestToastManager_DrawRendersTitle(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "visible"})
	output := drawToString(m, 80, 24)
	assert.Contains(t, output, "visible",
		"rendered screen should contain toast title")
}

func TestToastManager_DrawRendersBody(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{
		Title: "T",
		Body:  "this is the body text",
	})
	output := drawToString(m, 80, 24)
	assert.Contains(t, output, "body text")
}

func TestToastManager_DrawRendersActionHints(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{
		Title: "approve?",
		ActionHints: []components.ActionHint{
			{Key: "enter", Label: "approve"},
			{Key: "esc", Label: "dismiss"},
		},
	})
	output := drawToString(m, 80, 24)
	assert.Contains(t, output, "enter")
	assert.Contains(t, output, "approve")
	assert.Contains(t, output, "esc")
}

func TestToastManager_DrawMultipleToastsAllVisible(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "alpha"})
	m.Update(components.ShowToastMsg{Title: "beta"})
	output := drawToString(m, 80, 24)
	assert.Contains(t, output, "alpha")
	assert.Contains(t, output, "beta")
}

func TestToastManager_DrawAfterDismissRemovesFromScreen(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "gone"})
	m.Update(components.DismissToastMsg{ID: 1})
	output := drawToString(m, 80, 24)
	assert.NotContains(t, output, "gone")
}

func TestToastManager_DrawPositionedBottomRight(t *testing.T) {
	t.Parallel()
	// Render at a modest terminal size and check that the toast lands in the
	// lower half of the screen buffer.
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "corner"})

	const W, H = 60, 12
	scr := uv.NewScreen(W, H)
	m.Draw(scr, scr.Bounds())

	// Find the line that contains the title text.
	foundLine := -1
	for y := range H {
		line := scr.Buffer.Line(y).String()
		if strings.Contains(line, "corner") {
			foundLine = y
			break
		}
	}
	require.GreaterOrEqual(t, foundLine, 0, "title 'corner' not found in any line")

	// It should appear in the bottom half.
	assert.GreaterOrEqual(t, foundLine, H/2,
		"toast should render in the bottom half of the screen")
}

// ---------------------------------------------------------------------------
// Level / severity styling
// ---------------------------------------------------------------------------

func TestToastLevel_AllLevelsRenderWithoutError(t *testing.T) {
	t.Parallel()
	levels := []components.ToastLevel{
		components.ToastLevelInfo,
		components.ToastLevelSuccess,
		components.ToastLevelWarning,
		components.ToastLevelError,
	}
	for _, lvl := range levels {
		lvl := lvl
		t.Run(levelName(lvl), func(t *testing.T) {
			t.Parallel()
			m := newManager()
			m.Update(components.ShowToastMsg{Title: "msg", Level: lvl})
			output := drawToString(m, 80, 24)
			assert.Contains(t, output, "msg",
				"level %v toast should render title", lvl)
		})
	}
}

func levelName(l components.ToastLevel) string {
	switch l {
	case components.ToastLevelInfo:
		return "info"
	case components.ToastLevelSuccess:
		return "success"
	case components.ToastLevelWarning:
		return "warning"
	case components.ToastLevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Width handling
// ---------------------------------------------------------------------------

func TestToastManager_DrawNarrowTerminalNoPanic(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "narrow"})
	// Very narrow terminal — should not panic.
	assert.NotPanics(t, func() {
		_ = drawToString(m, 10, 5)
	})
}

func TestToastManager_DrawZeroSizeNoPanic(t *testing.T) {
	t.Parallel()
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "zero"})
	// Zero-size — should not panic (drawToString clamps to 1×1 minimum).
	assert.NotPanics(t, func() {
		_ = drawToString(m, 0, 0)
	})
}

// ---------------------------------------------------------------------------
// Word wrap (indirectly via rendering)
// ---------------------------------------------------------------------------

func TestToastManager_LongBodyWraps(t *testing.T) {
	t.Parallel()
	longBody := strings.Repeat("word ", 20) // clearly longer than MaxToastWidth
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "wrap", Body: longBody})

	const W, H = 80, 30
	scr := uv.NewScreen(W, H)
	m.Draw(scr, scr.Bounds())

	// Count lines that contain body words.
	bodyLineCount := 0
	for y := range H {
		if strings.Contains(scr.Buffer.Line(y).String(), "word") {
			bodyLineCount++
		}
	}
	assert.Greater(t, bodyLineCount, 1, "long body should wrap onto multiple lines")
}

// ---------------------------------------------------------------------------
// BottomRightRect placement
// ---------------------------------------------------------------------------

func TestToastManager_PositionedInBottomQuarter(t *testing.T) {
	t.Parallel()
	// Verify the toast lands in the bottom quarter of a tall screen.
	m := newManager()
	m.Update(components.ShowToastMsg{Title: "pos"})

	const W, H = 80, 40
	scr := uv.NewScreen(W, H)
	m.Draw(scr, scr.Bounds())

	found := -1
	for y := range H {
		if strings.Contains(scr.Buffer.Line(y).String(), "pos") {
			found = y
		}
	}
	require.GreaterOrEqual(t, found, 0, "title not found")
	assert.GreaterOrEqual(t, found, 3*H/4,
		"toast should render in the bottom quarter of a tall screen")
}
