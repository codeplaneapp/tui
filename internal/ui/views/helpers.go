package views

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

var packageCom = common.DefaultCommon(nil)

var (
	jjhubTitleStyle     = packageCom.Styles.JJHub.Title
	jjhubSectionStyle   = packageCom.Styles.JJHub.Section
	jjhubMetaLabelStyle = packageCom.Styles.JJHub.MetaLabel
	jjhubMetaValueStyle = packageCom.Styles.JJHub.MetaValue
	jjhubMutedStyle     = packageCom.Styles.JJHub.Muted
	jjhubErrorStyle     = packageCom.Styles.JJHub.Error
)

var v = struct {
	com *common.Common
}{
	com: packageCom,
}

func viewCommon(com *common.Common) *common.Common {
	if com != nil && com.Styles != nil {
		return com
	}
	if com == nil {
		return common.DefaultCommon(nil)
	}
	defaultStyles := styles.DefaultStyles()
	com.Styles = &defaultStyles
	return com
}

func parseCommonAndClient(args []any) (*common.Common, *smithers.Client, int) {
	com := viewCommon(nil)
	offset := 0

	if len(args) > 0 {
		if provided, ok := args[0].(*common.Common); ok || args[0] == nil {
			if ok && provided != nil {
				com = viewCommon(provided)
			}
			offset++
		}
	}

	var client *smithers.Client
	if len(args) > offset {
		client, _ = args[offset].(*smithers.Client)
		if client != nil || args[offset] == nil {
			offset++
		}
	}

	return com, client, offset
}

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
		maxLen = 80
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

func jjhubAvailable() bool {
	_, err := exec.LookPath("jjhub")
	return err == nil
}

func jjhubHeader(args ...any) string {
	t := packageCom.Styles
	var title string
	var width int
	var right string
	switch len(args) {
	case 3:
		title, _ = args[0].(string)
		width, _ = args[1].(int)
		right, _ = args[2].(string)
	case 4:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		title, _ = args[1].(string)
		width, _ = args[2].(int)
		right, _ = args[3].(string)
	default:
		return ""
	}
	left := t.JJHub.Title.Render(title)
	if width <= 0 || strings.TrimSpace(right) == "" {
		return left
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func jjhubMetaRow(args ...any) string {
	t := packageCom.Styles
	var label string
	var value string
	switch len(args) {
	case 2:
		label, _ = args[0].(string)
		value, _ = args[1].(string)
	case 3:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		label, _ = args[1].(string)
		value, _ = args[2].(string)
	default:
		return ""
	}
	return t.JJHub.MetaLabel.Render(label) + t.JJHub.MetaValue.Render(value)
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

func jjhubRepoLabel(args ...any) string {
	t := packageCom.Styles
	var repo *jjhub.Repo
	switch len(args) {
	case 1:
		repo, _ = args[0].(*jjhub.Repo)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		repo, _ = args[1].(*jjhub.Repo)
	default:
		return ""
	}
	if repo == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(repo.FullName) != "":
		return t.JJHub.Muted.Render(repo.FullName)
	case strings.TrimSpace(repo.Owner) != "" && strings.TrimSpace(repo.Name) != "":
		return t.JJHub.Muted.Render(repo.Owner + "/" + repo.Name)
	case strings.TrimSpace(repo.Name) != "":
		return t.JJHub.Muted.Render(repo.Name)
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

func jjRelativeTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return ts
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func statusGlyph(args ...any) string {
	t := packageCom.Styles
	var s smithers.RunStatus
	switch len(args) {
	case 1:
		s, _ = args[0].(smithers.RunStatus)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		s, _ = args[1].(smithers.RunStatus)
	default:
		return t.Subtle.Render("○")
	}
	switch s {
	case smithers.RunStatusRunning:
		return lipgloss.NewStyle().Foreground(t.Green).Render("●")
	case smithers.RunStatusWaitingApproval:
		return lipgloss.NewStyle().Foreground(t.Warning).Render("⚠")
	case smithers.RunStatusFinished:
		return t.Subtle.Render("✓")
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(t.Error).Render("✗")
	case smithers.RunStatusCancelled:
		return t.Subtle.Render("–")
	default:
		return t.Subtle.Render("○")
	}
}

func fmtDurationMs(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func jjLandingStateIcon(args ...any) string {
	t := packageCom.Styles
	var state string
	switch len(args) {
	case 1:
		state, _ = args[0].(string)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		state, _ = args[1].(string)
	default:
		return t.Subtle.Render("?")
	}
	switch state {
	case "open":
		return lipgloss.NewStyle().Foreground(t.Green).Render("⬆")
	case "merged":
		return lipgloss.NewStyle().Foreground(t.Primary).Render("✓")
	case "closed":
		return lipgloss.NewStyle().Foreground(t.Error).Render("✗")
	case "draft":
		return t.Subtle.Render("◌")
	default:
		return t.Subtle.Render("?")
	}
}

func jjLandingStateStyle(args ...any) lipgloss.Style {
	t := packageCom.Styles
	var state string
	switch len(args) {
	case 1:
		state, _ = args[0].(string)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		state, _ = args[1].(string)
	default:
		return t.Subtle
	}
	switch state {
	case "open":
		return lipgloss.NewStyle().Foreground(t.Green)
	case "merged":
		return lipgloss.NewStyle().Foreground(t.Primary)
	case "closed":
		return lipgloss.NewStyle().Foreground(t.Error)
	case "draft":
		return t.Subtle
	default:
		return t.Subtle
	}
}

func jjIssueStateIcon(args ...any) string {
	t := packageCom.Styles
	var state string
	switch len(args) {
	case 1:
		state, _ = args[0].(string)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		state, _ = args[1].(string)
	default:
		return t.Subtle.Render("?")
	}
	switch state {
	case "open":
		return lipgloss.NewStyle().Foreground(t.Green).Render("◉")
	case "closed":
		return lipgloss.NewStyle().Foreground(t.Error).Render("◎")
	default:
		return t.Subtle.Render("?")
	}
}

func jjWorkspaceStatusIcon(args ...any) string {
	t := packageCom.Styles
	var status string
	switch len(args) {
	case 1:
		status, _ = args[0].(string)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		status, _ = args[1].(string)
	default:
		return t.Subtle.Render("?")
	}
	switch status {
	case "running":
		return lipgloss.NewStyle().Foreground(t.Green).Render("●")
	case "pending":
		return lipgloss.NewStyle().Foreground(t.Warning).Render("◌")
	case "stopped":
		return t.Subtle.Render("○")
	case "failed":
		return lipgloss.NewStyle().Foreground(t.Error).Render("✗")
	default:
		return t.Subtle.Render("?")
	}
}

func agentStatusIcon(status string) string {
	switch status {
	case "likely-subscription", "api-key":
		return "●"
	case "binary-only":
		return "◐"
	default:
		return "○"
	}
}

func agentStatusStyle(args ...any) lipgloss.Style {
	t := packageCom.Styles
	var status string
	switch len(args) {
	case 1:
		status, _ = args[0].(string)
	case 2:
		if provided, ok := args[0].(*styles.Styles); ok && provided != nil {
			t = provided
		}
		status, _ = args[1].(string)
	default:
		return lipgloss.NewStyle().Faint(true)
	}
	switch status {
	case "likely-subscription":
		return lipgloss.NewStyle().Foreground(t.Green)
	case "api-key":
		return lipgloss.NewStyle().Foreground(t.Yellow)
	case "binary-only":
		return lipgloss.NewStyle().Foreground(t.Gray)
	default:
		return lipgloss.NewStyle().Faint(true)
	}
}

func styledCheck(args ...any) string {
	t := packageCom.Styles
	var ok bool
	switch len(args) {
	case 1:
		ok, _ = args[0].(bool)
	case 2:
		if provided, styleOK := args[0].(*styles.Styles); styleOK && provided != nil {
			t = provided
		}
		ok, _ = args[1].(bool)
	default:
		return lipgloss.NewStyle().Foreground(t.Red).Render("✗")
	}
	if ok {
		return lipgloss.NewStyle().Foreground(t.Green).Render("✓")
	}
	return lipgloss.NewStyle().Foreground(t.Red).Render("✗")
}

func boolLabel(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func ViewHeader(t *styles.Styles, app, view string, width int, helpHint string) string {
	appPart := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(app)
	sepPart := lipgloss.NewStyle().Bold(true).Faint(true).Render(" \u203a ")
	viewPart := lipgloss.NewStyle().Bold(true).Render(view)

	header := appPart + sepPart + viewPart
	if helpHint == "" {
		helpHint = "[Esc] Back"
	}
	helpStyle := lipgloss.NewStyle().Faint(true).Render(helpHint)

	if width <= 0 {
		return header
	}

	headerWidth := lipgloss.Width(header)
	helpWidth := lipgloss.Width(helpStyle)

	gap := width - headerWidth - helpWidth - 2
	if gap <= 0 {
		return header + "  " + helpStyle
	}
	return header + strings.Repeat(" ", gap) + helpStyle
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
