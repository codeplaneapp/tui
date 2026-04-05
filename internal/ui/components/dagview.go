package components

// dagview.go — ASCII/UTF-8 DAG visualization for workflow field pipelines.
//
// RenderDAGFields renders a linear chain of WorkflowTask fields as a
// left-to-right ASCII box-drawing pipeline.  Because the Smithers
// DAGDefinition exposes only an ordered list of launch-fields (not the full
// node graph), the visualization represents each field as a labelled box and
// connects them with arrows.
//
// Example output (for two fields "Prompt" and "Ticket ID"):
//
//	┌───────────┐     ┌───────────────┐
//	│  Prompt   │ ──▶ │   Ticket ID   │
//	└───────────┘     └───────────────┘
//
// When the full node list is available (e.g. from RunInspection.Tasks), use
// RenderDAGTasks to colour-code nodes by their TaskState.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// RenderDAGFields renders an ordered list of workflow launch-fields as a
// linear left-to-right pipeline using box-drawing characters.
//
// maxWidth constrains the total output width; if maxWidth <= 0 it defaults to
// 120.  When there are no fields an empty string is returned.
func RenderDAGFields(fields []smithers.WorkflowTask, maxWidth int) string {
	if len(fields) == 0 {
		return ""
	}
	if maxWidth <= 0 {
		maxWidth = 120
	}

	// Compute inner widths for each box (field label + type annotation).
	boxes := make([]dagBox, len(fields))
	for i, f := range fields {
		kind := f.Type
		if kind == "" {
			kind = "string"
		}
		boxes[i] = dagBox{label: f.Label, kind: kind}
	}

	// Determine inner widths (minimum 8) so that all boxes look aligned.
	minInner := 8
	maxInner := 0
	for _, b := range boxes {
		w := lipgloss.Width(b.label)
		if w > maxInner {
			maxInner = w
		}
	}
	if maxInner < minInner {
		maxInner = minInner
	}

	// Arrow connector: " ──▶ "
	const arrow = " ──▶ "
	arrowW := lipgloss.Width(arrow)

	// Total line width estimate to check fit.
	// box_width = inner + 4 (│ space … space │) → actually inner + 2 padding + 2 borders
	boxW := maxInner + 4
	totalW := len(boxes)*boxW + (len(boxes)-1)*arrowW
	// If total exceeds maxWidth, render vertically.
	if totalW > maxWidth {
		return renderDAGVertical(boxes, maxWidth)
	}

	// Render horizontal layout.
	faintStyle := lipgloss.NewStyle().Faint(true)
	labelStyle := lipgloss.NewStyle().Bold(true)

	top := "  "
	mid := "  "
	bot := "  "

	for i, b := range boxes {
		inner := padLabelRight(b.label, maxInner)
		top += "┌" + strings.Repeat("─", maxInner+2) + "┐"
		mid += "│ " + labelStyle.Render(inner) + " │"
		bot += "└" + strings.Repeat("─", maxInner+2) + "┘"
		if i < len(boxes)-1 {
			// Arrow fills matching height in the middle row.
			top += strings.Repeat(" ", arrowW)
			mid += arrow
			bot += strings.Repeat(" ", arrowW)
		}
	}

	var sb strings.Builder
	sb.WriteString(top + "\n")
	sb.WriteString(mid + "\n")
	sb.WriteString(bot + "\n")

	// Type annotations below each box.
	if len(boxes) > 1 || boxes[0].kind != "" {
		typeLine := "  "
		for i, b := range boxes {
			annotation := faintStyle.Render(fmt.Sprintf("(%s)", b.kind))
			// Centre the annotation under the box.
			annotW := lipgloss.Width(annotation)
			pad := (maxInner + 4 - annotW) / 2
			if pad < 0 {
				pad = 0
			}
			typeLine += strings.Repeat(" ", pad) + annotation
			remaining := maxInner + 4 - pad - annotW
			if remaining < 0 {
				remaining = 0
			}
			typeLine += strings.Repeat(" ", remaining)
			if i < len(boxes)-1 {
				typeLine += strings.Repeat(" ", arrowW)
			}
		}
		sb.WriteString(typeLine + "\n")
	}

	return sb.String()
}

// RenderDAGTasks renders workflow tasks from a run inspection as a
// colour-coded linear chain.  Nodes are colour-coded by TaskState:
//
//   - green  — finished
//   - yellow — running / waiting
//   - red    — failed / cancelled
//   - grey   — pending / skipped / blocked
func RenderDAGTasks(tasks []smithers.RunTask, maxWidth int) string {
	if len(tasks) == 0 {
		return ""
	}
	if maxWidth <= 0 {
		maxWidth = 120
	}

	boxes := make([]dagTaskBox, len(tasks))
	for i, t := range tasks {
		lbl := t.NodeID
		if t.Label != nil && *t.Label != "" {
			lbl = *t.Label
		}
		boxes[i] = dagTaskBox{label: lbl, state: t.State}
	}

	maxInner := 8
	for _, b := range boxes {
		if w := lipgloss.Width(b.label); w > maxInner {
			maxInner = w
		}
	}

	const arrow = " ──▶ "
	arrowW := lipgloss.Width(arrow)
	boxW := maxInner + 4
	totalW := len(boxes)*boxW + (len(boxes)-1)*arrowW
	if totalW > maxWidth {
		return renderDAGTasksVertical(boxes, maxWidth)
	}

	top := "  "
	mid := "  "
	bot := "  "

	for i, b := range boxes {
		style := taskStateStyle(b.state)
		inner := padLabelRight(b.label, maxInner)
		top += "┌" + strings.Repeat("─", maxInner+2) + "┐"
		mid += "│ " + style.Render(inner) + " │"
		bot += "└" + strings.Repeat("─", maxInner+2) + "┘"
		if i < len(boxes)-1 {
			top += strings.Repeat(" ", arrowW)
			mid += arrow
			bot += strings.Repeat(" ", arrowW)
		}
	}

	var sb strings.Builder
	sb.WriteString(top + "\n")
	sb.WriteString(mid + "\n")
	sb.WriteString(bot + "\n")
	return sb.String()
}

// --- Vertical fallback layouts (narrow terminals) ---

type dagBox struct {
	label string
	kind  string
}

func renderDAGVertical(boxes []dagBox, maxWidth int) string {
	labelStyle := lipgloss.NewStyle().Bold(true)
	faintStyle := lipgloss.NewStyle().Faint(true)

	inner := 0
	for _, b := range boxes {
		if w := lipgloss.Width(b.label); w > inner {
			inner = w
		}
	}
	if inner < 8 {
		inner = 8
	}
	if inner > maxWidth-6 {
		inner = maxWidth - 6
	}

	var sb strings.Builder
	for i, b := range boxes {
		sb.WriteString("  ┌" + strings.Repeat("─", inner+2) + "┐\n")
		sb.WriteString("  │ " + labelStyle.Render(padLabelRight(b.label, inner)) + " │\n")
		if b.kind != "" {
			sb.WriteString("  │ " + faintStyle.Render(padLabelRight("("+b.kind+")", inner)) + " │\n")
		}
		sb.WriteString("  └" + strings.Repeat("─", inner+2) + "┘\n")
		if i < len(boxes)-1 {
			sb.WriteString("        │\n")
			sb.WriteString("        ▼\n")
		}
	}
	return sb.String()
}

type dagTaskBox struct {
	label string
	state smithers.TaskState
}

func renderDAGTasksVertical(boxes []dagTaskBox, maxWidth int) string {
	inner := 8
	for _, b := range boxes {
		if w := lipgloss.Width(b.label); w > inner {
			inner = w
		}
	}
	if inner > maxWidth-6 {
		inner = maxWidth - 6
	}
	var sb strings.Builder
	for i, b := range boxes {
		style := taskStateStyle(b.state)
		sb.WriteString("  ┌" + strings.Repeat("─", inner+2) + "┐\n")
		sb.WriteString("  │ " + style.Render(padLabelRight(b.label, inner)) + " │\n")
		sb.WriteString("  └" + strings.Repeat("─", inner+2) + "┘\n")
		if i < len(boxes)-1 {
			sb.WriteString("        │\n")
			sb.WriteString("        ▼\n")
		}
	}
	return sb.String()
}

// taskStateStyle returns the lipgloss style for a given TaskState.
func taskStateStyle(state smithers.TaskState) lipgloss.Style {
	switch state {
	case smithers.TaskStateFinished:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case smithers.TaskStateRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	case smithers.TaskStateBlocked, smithers.TaskStateSkipped:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	case smithers.TaskStateFailed, smithers.TaskStateCancelled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	default:
		return lipgloss.NewStyle().Faint(true) // grey for pending
	}
}

// padLabelRight pads a label to width using rune-aware measurement.
func padLabelRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
