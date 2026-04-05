package views

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// padRight pads a string to the given width with spaces.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// truncate shortens s to maxLen runes, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// truncateStr shortens s to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// formatStatus returns a styled status string.
func formatStatus(status string) string {
	switch status {
	case "pending":
		return lipgloss.NewStyle().Bold(true).Render("● pending")
	case "approved":
		return lipgloss.NewStyle().Render("✓ approved")
	case "denied":
		return lipgloss.NewStyle().Render("✗ denied")
	default:
		return status
	}
}

// formatPayload attempts to pretty-print JSON payload, falling back to raw text.
func formatPayload(payload string, width int) string {
	var parsed interface{}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		// Not JSON; wrap text.
		return wrapText(payload, width)
	}

	pretty, err := json.MarshalIndent(parsed, "  ", "  ")
	if err != nil {
		return wrapText(payload, width)
	}
	return "  " + string(pretty)
}

// wrapText wraps text to fit within the given width.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			lines = append(lines, "  "+line[:width-2])
			line = line[width-2:]
		}
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n")
}

// wrapLineToWidth splits s into lines each at most width runes wide.
func wrapLineToWidth(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var result []string
	for _, line := range strings.Split(s, "\n") {
		runes := []rune(line)
		for len(runes) > width {
			result = append(result, string(runes[:width]))
			runes = runes[width:]
		}
		result = append(result, string(runes))
	}
	return result
}

// fmtRelativeAge returns a human-readable relative age string for a Unix
// millisecond timestamp, e.g. "30s ago", "5m ago", "3h ago", "2d ago".
// Returns "" if updatedAtMs <= 0.
func fmtRelativeAge(updatedAtMs int64) string {
	if updatedAtMs <= 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(updatedAtMs))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
