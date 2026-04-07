package views

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
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

var (
	jjhubTitleStyle     = lipgloss.NewStyle().Bold(true)
	jjhubSectionStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	jjhubMetaLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(12)
	jjhubMetaValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	jjhubMutedStyle     = lipgloss.NewStyle().Faint(true)
	jjhubErrorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

func jjhubAvailable() bool {
	_, err := exec.LookPath("jjhub")
	return err == nil
}

func jjhubHeader(title string, width int, right string) string {
	left := jjhubTitleStyle.Render(title)
	if width <= 0 || strings.TrimSpace(right) == "" {
		return left
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func jjhubMetaRow(label, value string) string {
	return jjhubMetaLabelStyle.Render(label) + jjhubMetaValueStyle.Render(value)
}

func jjhubJoinNonEmpty(sep string, parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, sep)
}

func jjhubRepoLabel(repo *jjhub.Repo) string {
	if repo == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(repo.FullName) != "":
		return jjhubMutedStyle.Render(repo.FullName)
	case strings.TrimSpace(repo.Owner) != "" && strings.TrimSpace(repo.Name) != "":
		return jjhubMutedStyle.Render(repo.Owner + "/" + repo.Name)
	case strings.TrimSpace(repo.Name) != "":
		return jjhubMutedStyle.Render(repo.Name)
	default:
		return ""
	}
}

func jjhubAtUser(login string) string {
	if strings.TrimSpace(login) == "" {
		return ""
	}
	return "@" + login
}

func jjhubFormatRelativeTime(raw string) string {
	if raw == "" {
		return "-"
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return raw
		}
	}

	delta := time.Since(parsed)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
	case delta < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(delta.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(delta.Hours()/(24*365)))
	}
}

func jjhubFormatTimestamp(raw string) string {
	if raw == "" {
		return "-"
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return raw
		}
	}

	return parsed.Local().Format("2006-01-02 15:04")
}

func jjhubClipLines(s string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return s, false
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s, false
	}
	return strings.Join(lines[:maxLines], "\n"), true
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
