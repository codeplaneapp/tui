package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// Align controls horizontal alignment within a cell.
type Align int

const (
	AlignLeft Align = iota
	AlignRight
)

// Column defines one responsive table column.
type Column struct {
	Title    string
	Width    int
	Grow     bool
	MinWidth int
	Align    Align
}

// Row is one rendered table row.
type Row struct {
	Cells []string
}

var (
	jTableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	jTableAltRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	jTableRowStyle    = lipgloss.NewStyle().Background(lipgloss.Color("234"))
	jTableSelected    = lipgloss.NewStyle().Background(lipgloss.Color("238")).Bold(true)
	jTableInactive    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250"))
	jTableCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	jTableMutedStyle  = lipgloss.NewStyle().Faint(true)
)

type visibleColumn struct {
	column Column
	index  int
	width  int
}

// RenderTable draws a responsive table with a header, alternating rows, a
// cursor indicator, and a footer scroll indicator.
func RenderTable(
	columns []Column,
	rows []Row,
	cursor int,
	offset int,
	width int,
	height int,
	focused bool,
) (string, int) {
	if width <= 0 || height <= 0 {
		return "", 0
	}

	visibleColumns := filterVisibleColumns(columns, width)
	if len(visibleColumns) == 0 {
		return "", 0
	}

	const cursorWidth = 2
	availableWidth := width - cursorWidth - max(0, len(visibleColumns)-1)
	if availableWidth < len(visibleColumns) {
		availableWidth = len(visibleColumns)
	}

	assignColumnWidths(visibleColumns, availableWidth)

	headerHeight := 1
	footerHeight := 1
	bodyHeight := max(1, height-headerHeight-footerHeight)

	cursor = clamp(cursor, 0, max(0, len(rows)-1))
	offset = clampOffset(cursor, offset, len(rows), bodyHeight)

	var out strings.Builder
	out.WriteString(renderTableHeader(visibleColumns, cursorWidth))

	if len(rows) == 0 {
		out.WriteString("\n")
		out.WriteString(jTableMutedStyle.Render(padToWidth("No items found.", width)))
		out.WriteString("\n")
		out.WriteString(renderTableFooter(width, 0, 0))
		return out.String(), 0
	}

	end := min(len(rows), offset+bodyHeight)
	for rowIndex := offset; rowIndex < end; rowIndex++ {
		out.WriteString("\n")
		out.WriteString(renderTableRow(
			rows[rowIndex],
			visibleColumns,
			rowIndex,
			offset,
			width,
			cursor,
			focused,
		))
	}

	out.WriteString("\n")
	out.WriteString(renderTableFooter(width, cursor+1, len(rows)))

	return out.String(), offset
}

func filterVisibleColumns(columns []Column, width int) []visibleColumn {
	visible := make([]visibleColumn, 0, len(columns))
	for i, column := range columns {
		if column.MinWidth > 0 && width < column.MinWidth {
			continue
		}
		visible = append(visible, visibleColumn{
			column: column,
			index:  i,
		})
	}
	return visible
}

func assignColumnWidths(columns []visibleColumn, available int) {
	if len(columns) == 0 {
		return
	}

	fixedTotal := 0
	growCount := 0
	for i := range columns {
		if columns[i].column.Grow {
			growCount++
			continue
		}
		columns[i].width = columnBaseWidth(columns[i].column)
		fixedTotal += columns[i].width
	}

	remaining := max(0, available-fixedTotal)
	if growCount > 0 {
		share := 0
		extra := 0
		if remaining > 0 {
			share = remaining / growCount
			extra = remaining % growCount
		}
		for i := range columns {
			if !columns[i].column.Grow {
				continue
			}
			columns[i].width = share
			if extra > 0 {
				columns[i].width++
				extra--
			}
		}
	}

	total := 0
	for _, column := range columns {
		total += column.width
	}
	if total > available {
		shrinkWidths(columns, total-available)
	}

	for i := range columns {
		if columns[i].width <= 0 {
			columns[i].width = 1
		}
	}
}

func shrinkWidths(columns []visibleColumn, overflow int) {
	for overflow > 0 {
		shrunk := false
		for i := len(columns) - 1; i >= 0 && overflow > 0; i-- {
			if columns[i].width <= 1 {
				continue
			}
			columns[i].width--
			overflow--
			shrunk = true
		}
		if !shrunk {
			return
		}
	}
}

func columnBaseWidth(column Column) int {
	if column.Width > 0 {
		return column.Width
	}
	return max(1, ansi.StringWidth(column.Title))
}

func renderTableHeader(columns []visibleColumn, cursorWidth int) string {
	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		cells = append(cells, jTableHeaderStyle.Render(renderCell(column.column.Title, column.width, AlignLeft)))
	}
	return strings.Repeat(" ", cursorWidth) + strings.Join(cells, " ")
}

func renderTableRow(
	row Row,
	columns []visibleColumn,
	rowIndex int,
	offset int,
	width int,
	cursor int,
	focused bool,
) string {
	indicator := "  "
	if rowIndex == cursor {
		cursorGlyph := styles.BorderThin
		if focused {
			cursorGlyph = styles.BorderThick
		}
		indicator = jTableCursorStyle.Render(cursorGlyph + " ")
	}

	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		cell := ""
		if column.index < len(row.Cells) {
			cell = row.Cells[column.index]
		}
		cells = append(cells, renderCell(cell, column.width, column.column.Align))
	}

	line := indicator + strings.Join(cells, " ")
	line = padToWidth(line, width)

	switch {
	case rowIndex == cursor && focused:
		return jTableSelected.Render(line)
	case rowIndex == cursor:
		return jTableInactive.Render(line)
	case (rowIndex-offset)%2 == 1:
		return jTableAltRowStyle.Render(line)
	default:
		return jTableRowStyle.Render(line)
	}
}

func renderTableFooter(width int, current int, total int) string {
	label := fmt.Sprintf("%d/%d", current, total)
	return jTableMutedStyle.Render(padLeft(label, width))
}

func renderCell(value string, width int, align Align) string {
	if width <= 0 {
		return ""
	}

	truncated := value
	if ansi.StringWidth(truncated) > width {
		if width == 1 {
			truncated = ansi.Truncate(value, width, "")
		} else {
			truncated = ansi.Truncate(value, width, "…")
		}
	}

	padding := width - ansi.StringWidth(truncated)
	if padding <= 0 {
		return truncated
	}

	switch align {
	case AlignRight:
		return strings.Repeat(" ", padding) + truncated
	default:
		return truncated + strings.Repeat(" ", padding)
	}
}

func padToWidth(value string, width int) string {
	padding := width - ansi.StringWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func padLeft(value string, width int) string {
	padding := width - ansi.StringWidth(value)
	if padding <= 0 {
		return value
	}
	return strings.Repeat(" ", padding) + value
}

func clamp(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func clampOffset(cursor, offset, totalRows, bodyHeight int) int {
	if totalRows <= 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+bodyHeight {
		offset = cursor - bodyHeight + 1
	}
	maxOffset := max(0, totalRows-bodyHeight)
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}
