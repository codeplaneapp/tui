package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestLogViewer_ImplementsPane(t *testing.T) {
	t.Parallel()

	var _ Pane = (*LogViewer)(nil)
}

func TestLogViewer_SetLinesAndView(t *testing.T) {
	t.Parallel()

	lv := NewLogViewer()
	lv.SetSize(60, 8)
	lv.SetTitle("build")
	lv.SetLines([]LogLine{
		{Text: "line one"},
		{Text: "line two", Error: true},
	})

	view := lv.View()
	assert.Contains(t, view, "build")
	assert.Contains(t, view, "line one")
	assert.Contains(t, view, "line two")
	assert.Contains(t, view, "1")
	assert.True(t, lv.errorLines[1])
}

func TestLogViewer_SearchLifecycle(t *testing.T) {
	t.Parallel()

	lv := NewLogViewer()
	lv.SetSize(60, 8)
	lv.SetLines([]LogLine{
		{Text: "error: first"},
		{Text: "info"},
		{Text: "error: second"},
	})

	_, cmd := lv.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	assert.NotNil(t, cmd)

	for _, r := range []rune("error") {
		updated, _ := lv.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		lv = updated.(*LogViewer)
	}

	assert.True(t, lv.searchActive)
	assert.Equal(t, 2, lv.MatchCount())

	updated, _ := lv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	lv = updated.(*LogViewer)

	assert.False(t, lv.searchActive)
	assert.Equal(t, "error", lv.SearchValue())
	assert.Equal(t, 2, lv.MatchCount())
}

func TestLogViewer_InvalidRegex(t *testing.T) {
	t.Parallel()

	lv := NewLogViewer()
	lv.SetSize(60, 8)
	lv.SetLines([]LogLine{{Text: "hello"}})

	_, _ = lv.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	updated, _ := lv.Update(tea.KeyPressMsg{Text: "[", Code: '['})
	lv = updated.(*LogViewer)

	assert.Error(t, lv.searchErr)
	assert.Equal(t, 0, lv.MatchCount())
	assert.True(t, lv.searchActive)
}

func TestLogViewer_SearchNextMatchScrolls(t *testing.T) {
	t.Parallel()

	lv := NewLogViewer()
	lv.SetSize(40, 4)

	lines := make([]LogLine, 0, 20)
	for i := range 20 {
		text := "line " + strings.Repeat("x", i%3)
		if i == 12 || i == 17 {
			text = "target"
		}
		lines = append(lines, LogLine{Text: text})
	}
	lv.SetLines(lines)
	lv.applySearch("target")
	initialOffset := lv.viewport.YOffset()

	updated, _ := lv.Update(tea.KeyPressMsg{Text: "n", Code: 'n'})
	lv = updated.(*LogViewer)

	assert.NotEqual(t, initialOffset, lv.viewport.YOffset())
	assert.Equal(t, 2, lv.MatchCount())
}
