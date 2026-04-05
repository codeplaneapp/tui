package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// runRowKind distinguishes header rows from selectable run rows in the
// virtual row list used for section-aware rendering and cursor navigation.
type runRowKind int

const (
	runRowKindHeader runRowKind = iota
	runRowKindRun
)

// runVirtualRow is one entry in the virtual row list built by RunTable.View().
// Header rows have a non-empty sectionLabel and a zero runIdx.
// A header row with an empty sectionLabel is a divider sentinel.
// Run rows have runIdx set to the index into RunTable.Runs.
type runVirtualRow struct {
	kind         runRowKind
	sectionLabel string // non-empty for section header rows; empty string = divider sentinel
	runIdx       int    // index into RunTable.Runs; only meaningful for runRowKindRun
}

// partitionRuns builds the virtual row list for sectioned rendering.
// Section order: Active (running | waiting-approval | waiting-event),
// Completed (finished | cancelled), Failed (failed).
// Sections with zero runs are omitted. A divider sentinel row (empty
// sectionLabel) is inserted before each non-first section.
func partitionRuns(runs []smithers.RunSummary) []runVirtualRow {
	type sectionDef struct {
		label string
		idxs  []int
	}
	sections := []sectionDef{
		{label: "ACTIVE"},
		{label: "COMPLETED"},
		{label: "FAILED"},
	}
	for i, r := range runs {
		switch r.Status {
		case smithers.RunStatusRunning,
			smithers.RunStatusWaitingApproval,
			smithers.RunStatusWaitingEvent:
			sections[0].idxs = append(sections[0].idxs, i)
		case smithers.RunStatusFinished,
			smithers.RunStatusCancelled:
			sections[1].idxs = append(sections[1].idxs, i)
		case smithers.RunStatusFailed:
			sections[2].idxs = append(sections[2].idxs, i)
		}
	}
	var rows []runVirtualRow
	first := true
	for _, sec := range sections {
		if len(sec.idxs) == 0 {
			continue
		}
		if !first {
			// divider sentinel before non-first sections
			rows = append(rows, runVirtualRow{kind: runRowKindHeader, sectionLabel: ""})
		}
		label := fmt.Sprintf("● %s (%d)", sec.label, len(sec.idxs))
		rows = append(rows, runVirtualRow{kind: runRowKindHeader, sectionLabel: label})
		for _, idx := range sec.idxs {
			rows = append(rows, runVirtualRow{kind: runRowKindRun, runIdx: idx})
		}
		first = false
	}
	return rows
}

// RunTable renders a tabular list of runs as a string.
// Stateless: call View() any time data or cursor changes.
// Cursor is a navigable-row index (counts only run rows, not section headers).
type RunTable struct {
	Runs        []smithers.RunSummary
	Cursor      int
	Width       int // available terminal columns
	Expanded    map[string]bool                     // runID → detail row visible
	Inspections map[string]*smithers.RunInspection  // runID → fetched tasks
}

// RunAtCursor returns the RunSummary at the given navigable cursor index and
// true if found, or a zero value and false when the index is out of range.
// This shared helper is used by both View() and RunsView.selectedRun().
func RunAtCursor(runs []smithers.RunSummary, cursor int) (smithers.RunSummary, bool) {
	rows := partitionRuns(runs)
	navigableIdx := -1
	for _, row := range rows {
		if row.kind != runRowKindRun {
			continue
		}
		navigableIdx++
		if navigableIdx == cursor {
			return runs[row.runIdx], true
		}
	}
	return smithers.RunSummary{}, false
}

// fmtDetailLine renders the context-sensitive secondary line shown below an
// expanded run row.  insp may be nil (inspection not yet loaded).
// width is the terminal width; it is accepted for future truncation but not
// currently used for padding.
func fmtDetailLine(run smithers.RunSummary, insp *smithers.RunInspection, _ int) string {
	const indent = "    " // 4-space indent aligns under the workflow column

	faint := lipgloss.NewStyle().Faint(true)

	switch run.Status {
	case smithers.RunStatusRunning:
		if insp != nil {
			for _, task := range insp.Tasks {
				if task.State == smithers.TaskStateRunning {
					if task.Label != nil && *task.Label != "" {
						return indent + faint.Render(fmt.Sprintf(`└─ Running: "%s"`, *task.Label))
					}
					break
				}
			}
		}
		return indent + faint.Render("└─ Running…")

	case smithers.RunStatusWaitingApproval:
		approvalStyle := statusStyle(smithers.RunStatusWaitingApproval)
		prefix := approvalStyle.Render("⏸ APPROVAL PENDING")
		reason := run.ErrorReason()
		var detail string
		if reason != "" {
			detail = fmt.Sprintf(`%s: "%s"   [a]pprove / [d]eny`, prefix, reason)
		} else {
			detail = prefix + "   [a]pprove / [d]eny"
		}
		return indent + detail

	case smithers.RunStatusWaitingEvent:
		return indent + faint.Render("└─ ⏳ Waiting for external event")

	case smithers.RunStatusFailed:
		reason := run.ErrorReason()
		if reason != "" {
			return indent + faint.Render(fmt.Sprintf("└─ ✗ Error: %s", reason))
		}
		return indent + faint.Render("└─ ✗ Failed")

	case smithers.RunStatusFinished, smithers.RunStatusCancelled:
		elapsed := fmtElapsed(run)
		if elapsed != "" {
			return indent + faint.Render(fmt.Sprintf("└─ Completed in %s", elapsed))
		}
		return indent + faint.Render("└─ Completed")

	default:
		return ""
	}
}

// statusStyle returns the lipgloss style for a run status.
func statusStyle(status smithers.RunStatus) lipgloss.Style {
	switch status {
	case smithers.RunStatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case smithers.RunStatusWaitingApproval:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow bold
	case smithers.RunStatusWaitingEvent:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	case smithers.RunStatusFinished:
		return lipgloss.NewStyle().Faint(true)
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	case smithers.RunStatusCancelled:
		return lipgloss.NewStyle().Faint(true).Strikethrough(true)
	default:
		return lipgloss.NewStyle()
	}
}

// fmtElapsed formats a run's elapsed time as a human-readable string.
// If the run has finished, it returns the total duration.
// If the run is still active, it returns time since start.
func fmtElapsed(run smithers.RunSummary) string {
	if run.StartedAtMs == nil {
		return ""
	}
	start := time.UnixMilli(*run.StartedAtMs)
	var elapsed time.Duration
	if run.FinishedAtMs != nil {
		elapsed = time.UnixMilli(*run.FinishedAtMs).Sub(start)
	} else {
		elapsed = time.Since(start)
	}
	elapsed = elapsed.Round(time.Second)

	h := int(elapsed.Hours())
	m := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// fmtProgress returns the node progress as "completed/total" or "" if no summary.
func fmtProgress(run smithers.RunSummary) string {
	if len(run.Summary) == 0 {
		return ""
	}
	completed := run.Summary["finished"] + run.Summary["failed"] + run.Summary["cancelled"]
	total := run.Summary["total"]
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", completed, total)
}

// progressBarWidth is the number of block characters in the visual bar.
const progressBarWidth = 8

// progressBar renders a visual block-character progress bar for the given run.
// The bar is barWidth characters wide and is colored based on the run status:
//   - green   (color "2") for running / on-track
//   - yellow  (color "3") for stalled states (waiting-approval, waiting-event)
//   - faint              for terminal states (finished, cancelled, failed)
//
// When no summary data is present an empty string is returned so the caller
// can fall back gracefully.
//
// The returned string has the form "[████░░░░] 50%" and is always the same
// printed width so column alignment is preserved.
func progressBar(run smithers.RunSummary, barWidth int) string {
	if len(run.Summary) == 0 {
		return ""
	}
	total := run.Summary["total"]
	if total <= 0 {
		return ""
	}
	completed := run.Summary["finished"] + run.Summary["failed"] + run.Summary["cancelled"]
	if completed > total {
		completed = total
	}

	ratio := float64(completed) / float64(total)
	filled := int(ratio * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	pct := int(ratio * 100)

	var style lipgloss.Style
	switch run.Status {
	case smithers.RunStatusRunning:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case smithers.RunStatusWaitingApproval, smithers.RunStatusWaitingEvent:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	default:
		// terminal states: finished, failed, cancelled
		style = lipgloss.NewStyle().Faint(true)
	}

	return style.Render(fmt.Sprintf("[%s] %3d%%", bar, pct))
}

// View renders the run table as a string suitable for embedding in a Bubble Tea view.
func (t RunTable) View() string {
	var b strings.Builder

	showProgress := t.Width >= 80
	showTime := t.Width >= 80

	// Column widths.
	const (
		cursorW   = 2  // "▸ " or "  "
		idW       = 8  // truncated run ID
		statusW   = 18 // status column
		progressW = 15 // "[████████] 100%"
		timeW     = 9  // "1h 30m" etc.
		gapW      = 2  // spacing between columns
	)

	// Compute workflow column width: fill remaining space.
	fixed := cursorW + idW + gapW + statusW + gapW
	if showProgress {
		fixed += progressW + gapW
	}
	if showTime {
		fixed += timeW + gapW
	}
	workflowW := t.Width - fixed
	if workflowW < 8 {
		workflowW = 8
	}

	faint := lipgloss.NewStyle().Faint(true)

	// Header row.
	header := faint.Render(
		fmt.Sprintf("  %-*s  %-*s  %-*s",
			idW, "ID",
			workflowW, "Workflow",
			statusW, "Status",
		),
	)
	if showProgress {
		header += faint.Render(fmt.Sprintf("  %-*s", progressW, "Progress"))
	}
	if showTime {
		header += faint.Render(fmt.Sprintf("  %-*s", timeW, "Time"))
	}
	b.WriteString(header)
	b.WriteString("\n")

	// Sectioned data rows.
	sectionStyle := lipgloss.NewStyle().Bold(true)
	dividerStyle := lipgloss.NewStyle().Faint(true)

	rows := partitionRuns(t.Runs)
	navigableIdx := -1

	for _, row := range rows {
		switch row.kind {
		case runRowKindHeader:
			if row.sectionLabel == "" {
				// divider sentinel between sections
				divWidth := t.Width
				if divWidth < 1 {
					divWidth = 20
				}
				b.WriteString(dividerStyle.Render(strings.Repeat("─", divWidth)) + "\n")
			} else {
				b.WriteString("\n" + sectionStyle.Render(row.sectionLabel) + "\n\n")
			}

		case runRowKindRun:
			navigableIdx++
			run := t.Runs[row.runIdx]

			cursor := "  "
			idStyle := lipgloss.NewStyle()
			if navigableIdx == t.Cursor {
				cursor = "▸ "
				idStyle = idStyle.Bold(true)
			}

			// Truncate/pad run ID.
			runID := run.RunID
			if len(runID) > idW {
				runID = runID[:idW]
			}

			// Truncate/pad workflow name.
			workflow := run.WorkflowName
			if workflow == "" {
				workflow = run.WorkflowPath
			}
			if len(workflow) > workflowW {
				if workflowW > 3 {
					workflow = workflow[:workflowW-3] + "..."
				} else {
					workflow = workflow[:workflowW]
				}
			}

			// Status with color.
			statusStr := string(run.Status)
			styledStatus := statusStyle(run.Status).Render(fmt.Sprintf("%-*s", statusW, statusStr))

			line := fmt.Sprintf("%s%-*s  %-*s  %s",
				cursor,
				idW, idStyle.Render(runID),
				workflowW, workflow,
				styledStatus,
			)

			if showProgress {
				bar := progressBar(run, progressBarWidth)
				if bar == "" {
					// No summary data — emit blank space to preserve alignment.
					line += "  " + strings.Repeat(" ", progressW)
				} else {
					// bar contains ANSI escapes; pad to progressW using visible width.
					barVis := lipgloss.Width(bar)
					pad := progressW - barVis
					if pad < 0 {
						pad = 0
					}
					line += "  " + bar + strings.Repeat(" ", pad)
				}
			}

			if showTime {
				line += fmt.Sprintf("  %-*s", timeW, fmtElapsed(run))
			}

			b.WriteString(line)
			b.WriteString("\n")

			// Render optional inline detail row when this run is expanded.
			if t.Expanded[run.RunID] {
				var insp *smithers.RunInspection
				if t.Inspections != nil {
					insp = t.Inspections[run.RunID]
				}
				detail := fmtDetailLine(run, insp, t.Width)
				if detail != "" {
					b.WriteString(detail)
					b.WriteString("\n")
				}
			}
		}
	}

	return b.String()
}
