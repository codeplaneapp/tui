package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestRenderTable_HidesColumnsBelowBreakpoint(t *testing.T) {
	t.Parallel()

	columns := []Column{
		{Title: "#", Width: 4},
		{Title: "Title", Grow: true},
		{Title: "Author", Width: 10, MinWidth: 80},
	}
	rows := []Row{{Cells: []string{"#1", "Landing title", "will"}}}

	rendered, _ := RenderTable(columns, rows, 0, 0, 60, 6, true)

	assert.Contains(t, rendered, "Title")
	assert.NotContains(t, rendered, "Author")
}

func TestRenderTable_ShowsFocusedCursor(t *testing.T) {
	t.Parallel()

	rendered, _ := RenderTable(
		[]Column{{Title: "Title", Grow: true}},
		[]Row{{Cells: []string{"Row one"}}},
		0,
		0,
		40,
		5,
		true,
	)

	assert.Contains(t, rendered, styles.BorderThick)
}

func TestRenderTable_AdjustsOffsetForCursor(t *testing.T) {
	t.Parallel()

	rows := []Row{
		{Cells: []string{"one"}},
		{Cells: []string{"two"}},
		{Cells: []string{"three"}},
		{Cells: []string{"four"}},
		{Cells: []string{"five"}},
	}

	rendered, offset := RenderTable(
		[]Column{{Title: "Title", Grow: true}},
		rows,
		4,
		0,
		40,
		4,
		true,
	)

	assert.Equal(t, 3, offset)
	assert.Contains(t, rendered, "5/5")
}

func TestRenderTable_TruncatesANSIContentByVisibleWidth(t *testing.T) {
	t.Parallel()

	colored := "\x1b[31mvery-long-colored-title\x1b[0m"
	rendered, _ := RenderTable(
		[]Column{{Title: "Title", Width: 8}},
		[]Row{{Cells: []string{colored}}},
		0,
		0,
		20,
		5,
		false,
	)

	lines := strings.Split(rendered, "\n")
	assert.Len(t, lines, 3)
	assert.Contains(t, lines[1], "…")
	assert.LessOrEqual(t, ansi.StringWidth(lines[1]), 20)
}
