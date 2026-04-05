package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Column defines a table column with optional responsive breakpoint.
type Column struct {
	Title    string
	Width    int  // fixed width, 0 = auto
	Grow     bool // take remaining space
	MinWidth int  // hide column below this terminal width (0 = always show)
}

// TableRow is one row in the table.
type TableRow struct {
	Cells []string
}

// RenderTable draws a table with header, rows, cursor, and scroll.
// Returns the rendered string.
func RenderTable(
	columns []Column,
	rows []TableRow,
	cursor int,
	offset int,
	width int,
	height int,
) (rendered string, newOffset int) {
	if len(rows) == 0 {
		return emptyStyle.Render("  No items found."), offset
	}

	// Filter visible columns based on terminal width.
	type visCol struct {
		col   Column
		index int
	}
	var visCols []visCol
	for i, c := range columns {
		if c.MinWidth > 0 && width < c.MinWidth {
			continue
		}
		visCols = append(visCols, visCol{col: c, index: i})
	}
	if len(visCols) == 0 {
		return "", offset
	}

	// Compute column widths.
	colWidths := make(map[int]int)
	fixedTotal := 0
	growCount := 0
	for _, vc := range visCols {
		if vc.col.Grow {
			growCount++
		} else {
			w := vc.col.Width
			if w == 0 {
				w = len(vc.col.Title) + 2
			}
			colWidths[vc.index] = w
			fixedTotal += w
		}
	}
	separators := len(visCols) - 1
	remaining := width - fixedTotal - separators - 2 // -2 for cursor column
	if remaining < 0 {
		remaining = 0
	}
	if growCount > 0 {
		per := remaining / growCount
		if per < 10 {
			per = 10
		}
		for _, vc := range visCols {
			if vc.col.Grow {
				colWidths[vc.index] = per
			}
		}
	}

	var b strings.Builder

	// Header row.
	b.WriteString("  ") // cursor column placeholder
	var hcells []string
	for _, vc := range visCols {
		hcells = append(hcells, headerColStyle.Render(padRight(vc.col.Title, colWidths[vc.index])))
	}
	b.WriteString(strings.Join(hcells, " "))
	b.WriteString("\n")

	// Viewport rows.
	visibleRows := height - 2 // header + scroll info
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Adjust offset so cursor is visible.
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visibleRows {
		offset = cursor - visibleRows + 1
	}
	newOffset = offset

	for i := offset; i < len(rows) && i < offset+visibleRows; i++ {
		row := rows[i]

		// Cursor indicator.
		indicator := "  "
		if i == cursor {
			indicator = cursorStyle.Render("> ")
		}

		var cells []string
		for _, vc := range visCols {
			cell := ""
			if vc.index < len(row.Cells) {
				cell = row.Cells[vc.index]
			}
			cells = append(cells, padOrTruncate(cell, colWidths[vc.index]))
		}
		line := indicator + strings.Join(cells, " ")

		if i == cursor {
			line = selectedRowStyle.Width(width).Render(line)
		} else if (i-offset)%2 == 1 {
			line = altRowStyle.Width(width).Render(line)
		}
		b.WriteString(line)
		if i < offset+visibleRows-1 && i < len(rows)-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator.
	if len(rows) > visibleRows {
		pos := fmt.Sprintf(" %d/%d", cursor+1, len(rows))
		scrollLine := "\n" + strings.Repeat(" ", width-lipgloss.Width(pos)-1) + scrollInfoStyle.Render(pos)
		b.WriteString(scrollLine)
	}

	return b.String(), newOffset
}

// padRight pads a string with spaces to the given width.
func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// padOrTruncate pads or truncates a string to exactly width visible characters.
func padOrTruncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := lipgloss.Width(s)
	if visible > width {
		// Truncate: strip ANSI, take runes, add ellipsis.
		plain := stripAnsi(s)
		runes := []rune(plain)
		if len(runes) > width-1 && width > 1 {
			return string(runes[:width-1]) + "…"
		}
		if len(runes) > width {
			return string(runes[:width])
		}
		return plain
	}
	return s + strings.Repeat(" ", width-visible)
}

func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
