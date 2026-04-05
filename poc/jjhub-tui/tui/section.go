package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/poc/jjhub-tui/jjhub"
)

// TabKind identifies a tab type.
type TabKind int

const (
	TabLandings TabKind = iota
	TabIssues
	TabWorkspaces
	TabWorkflows
	TabRepos
	TabNotifications
)

var allTabs = []TabKind{
	TabLandings, TabIssues, TabWorkspaces,
	TabWorkflows, TabRepos, TabNotifications,
}

func (t TabKind) String() string {
	switch t {
	case TabLandings:
		return "Landings"
	case TabIssues:
		return "Issues"
	case TabWorkspaces:
		return "Workspaces"
	case TabWorkflows:
		return "Workflows"
	case TabRepos:
		return "Repos"
	case TabNotifications:
		return "Notifications"
	default:
		return "?"
	}
}

func (t TabKind) Icon() string {
	switch t {
	case TabLandings:
		return "⬆"
	case TabIssues:
		return "◉"
	case TabWorkspaces:
		return "▣"
	case TabWorkflows:
		return "⟳"
	case TabRepos:
		return "◆"
	case TabNotifications:
		return "●"
	default:
		return " "
	}
}

// StateFilter tracks the current filter for list views.
type StateFilter int

const (
	FilterOpen StateFilter = iota
	FilterClosed
	FilterAll
)

var landingFilters = []struct {
	label string
	value string
}{
	{"Open", "open"},
	{"Merged", "merged"},
	{"Closed", "closed"},
	{"Draft", "draft"},
	{"All", "all"},
}

var issueFilters = []struct {
	label string
	value string
}{
	{"Open", "open"},
	{"Closed", "closed"},
	{"All", "all"},
}

// Section holds the state for one tab's content.
type Section struct {
	Kind    TabKind
	Columns []Column
	Rows    []TableRow
	Cursor  int
	Offset  int // scroll offset
	Loading bool
	Error   string
	Search  string // active search filter

	// Filter state (for tabs that support state filtering).
	FilterIndex int
	FilterLabel string

	// Raw data for sidebar preview.
	Landings      []jjhub.Landing
	Issues        []jjhub.Issue
	Repos         []jjhub.Repo
	Notifications []jjhub.Notification
	Workspaces    []jjhub.Workspace
	Workflows     []jjhub.Workflow
}

// ---- Constructors ----

func NewLandingsSection() *Section {
	return &Section{
		Kind:        TabLandings,
		Loading:     true,
		FilterLabel: "Open",
		Columns: []Column{
			{Title: "", Width: 2},
			{Title: "#", Width: 5},
			{Title: "Title", Grow: true},
			{Title: "Author", Width: 14, MinWidth: 80},
			{Title: "Changes", Width: 9, MinWidth: 100},
			{Title: "Target", Width: 10, MinWidth: 90},
			{Title: "Updated", Width: 10},
		},
	}
}

func NewIssuesSection() *Section {
	return &Section{
		Kind:        TabIssues,
		Loading:     true,
		FilterLabel: "Open",
		Columns: []Column{
			{Title: "", Width: 2},
			{Title: "#", Width: 5},
			{Title: "Title", Grow: true},
			{Title: "Author", Width: 14, MinWidth: 80},
			{Title: "Comments", Width: 10, MinWidth: 90},
			{Title: "Updated", Width: 10},
		},
	}
}

func NewWorkspacesSection() *Section {
	return &Section{
		Kind:    TabWorkspaces,
		Loading: true,
		Columns: []Column{
			{Title: "", Width: 2},
			{Title: "Name", Width: 18},
			{Title: "Status", Width: 10},
			{Title: "Persistence", Width: 14, MinWidth: 90},
			{Title: "SSH", Grow: true, MinWidth: 100},
			{Title: "Idle", Width: 8, MinWidth: 80},
			{Title: "Updated", Width: 10},
		},
	}
}

func NewWorkflowsSection() *Section {
	return &Section{
		Kind:    TabWorkflows,
		Loading: true,
		Columns: []Column{
			{Title: "", Width: 2},
			{Title: "Name", Width: 18},
			{Title: "Path", Grow: true},
			{Title: "Active", Width: 8},
			{Title: "Updated", Width: 10},
		},
	}
}

func NewReposSection() *Section {
	return &Section{
		Kind:    TabRepos,
		Loading: true,
		Columns: []Column{
			{Title: "Name", Width: 20},
			{Title: "Description", Grow: true},
			{Title: "Issues", Width: 8, MinWidth: 80},
			{Title: "Visibility", Width: 10, MinWidth: 90},
			{Title: "Updated", Width: 10},
		},
	}
}

func NewNotificationsSection() *Section {
	return &Section{
		Kind:    TabNotifications,
		Loading: true,
		Columns: []Column{
			{Title: "", Width: 2},
			{Title: "Type", Width: 12},
			{Title: "Title", Grow: true},
			{Title: "Repo", Width: 16, MinWidth: 80},
			{Title: "Updated", Width: 10},
		},
	}
}

// ---- Build rows from data ----

func (s *Section) BuildLandingRows(landings []jjhub.Landing) {
	s.Landings = landings
	s.Rows = make([]TableRow, 0, len(landings))
	for _, l := range landings {
		if s.Search != "" && !matchesSearch(l.Title, s.Search) {
			continue
		}
		s.Rows = append(s.Rows, TableRow{
			Cells: []string{
				landingStateIcon(l.State),
				fmt.Sprintf("#%d", l.Number),
				l.Title,
				l.Author.Login,
				fmt.Sprintf("%d", len(l.ChangeIDs)),
				l.TargetBookmark,
				relativeTime(l.UpdatedAt),
			},
		})
	}
	s.Loading = false
	s.Error = ""
	s.clampCursor()
}

func (s *Section) BuildIssueRows(issues []jjhub.Issue) {
	s.Issues = issues
	s.Rows = make([]TableRow, 0, len(issues))
	for _, iss := range issues {
		if s.Search != "" && !matchesSearch(iss.Title, s.Search) {
			continue
		}
		s.Rows = append(s.Rows, TableRow{
			Cells: []string{
				issueStateIcon(iss.State),
				fmt.Sprintf("#%d", iss.Number),
				iss.Title,
				iss.Author.Login,
				fmt.Sprintf("%d", iss.CommentCount),
				relativeTime(iss.UpdatedAt),
			},
		})
	}
	s.Loading = false
	s.Error = ""
	s.clampCursor()
}

func (s *Section) BuildWorkspaceRows(workspaces []jjhub.Workspace) {
	s.Workspaces = workspaces
	s.Rows = make([]TableRow, 0, len(workspaces))
	for _, w := range workspaces {
		name := w.Name
		if name == "" {
			name = dimStyle.Render("(unnamed)")
		}
		if s.Search != "" && !matchesSearch(name, s.Search) {
			continue
		}
		ssh := "-"
		if w.SSHHost != nil && *w.SSHHost != "" {
			ssh = *w.SSHHost
		}
		idle := "-"
		if w.IdleTimeoutSeconds > 0 {
			idle = fmt.Sprintf("%dm", w.IdleTimeoutSeconds/60)
		}
		s.Rows = append(s.Rows, TableRow{
			Cells: []string{
				workspaceStatusIcon(w.Status),
				name,
				w.Status,
				w.Persistence,
				ssh,
				idle,
				relativeTime(w.UpdatedAt),
			},
		})
	}
	s.Loading = false
	s.Error = ""
	s.clampCursor()
}

func (s *Section) BuildWorkflowRows(workflows []jjhub.Workflow) {
	s.Workflows = workflows
	s.Rows = make([]TableRow, 0, len(workflows))
	for _, wf := range workflows {
		if s.Search != "" && !matchesSearch(wf.Name, s.Search) {
			continue
		}
		active := closedStyle.Render("✗")
		if wf.IsActive {
			active = openStyle.Render("✓")
		}
		s.Rows = append(s.Rows, TableRow{
			Cells: []string{
				workflowIcon(wf.IsActive),
				wf.Name,
				wf.Path,
				active,
				relativeTime(wf.UpdatedAt),
			},
		})
	}
	s.Loading = false
	s.Error = ""
	s.clampCursor()
}

func (s *Section) BuildRepoRows(repos []jjhub.Repo) {
	s.Repos = repos
	s.Rows = make([]TableRow, 0, len(repos))
	for _, r := range repos {
		desc := r.Description
		if desc == "" {
			desc = dimStyle.Render("(no description)")
		}
		if s.Search != "" && !matchesSearch(r.Name, s.Search) {
			continue
		}
		vis := openStyle.Render("public")
		if !r.IsPublic {
			vis = closedStyle.Render("private")
		}
		s.Rows = append(s.Rows, TableRow{
			Cells: []string{
				r.Name,
				desc,
				fmt.Sprintf("%d", r.NumIssues),
				vis,
				relativeTime(r.UpdatedAt.Format(time.RFC3339)),
			},
		})
	}
	s.Loading = false
	s.Error = ""
	s.clampCursor()
}

func (s *Section) BuildNotificationRows(notifs []jjhub.Notification) {
	s.Notifications = notifs
	s.Rows = make([]TableRow, 0, len(notifs))
	for _, n := range notifs {
		if s.Search != "" && !matchesSearch(n.Title, s.Search) {
			continue
		}
		indicator := dimStyle.Render("○")
		if n.Unread {
			indicator = openStyle.Render("●")
		}
		s.Rows = append(s.Rows, TableRow{
			Cells: []string{
				indicator,
				n.Type,
				n.Title,
				n.RepoName,
				relativeTime(n.UpdatedAt),
			},
		})
	}
	s.Loading = false
	s.Error = ""
	s.clampCursor()
}

func (s *Section) SetError(err error) {
	s.Loading = false
	s.Error = err.Error()
}

// ---- Navigation ----

func (s *Section) CursorUp()   { if s.Cursor > 0 { s.Cursor-- } }
func (s *Section) CursorDown() { if s.Cursor < len(s.Rows)-1 { s.Cursor++ } }
func (s *Section) GotoTop()    { s.Cursor = 0 }
func (s *Section) GotoBottom() { if len(s.Rows) > 0 { s.Cursor = len(s.Rows) - 1 } }
func (s *Section) PageDown(pageSize int) {
	s.Cursor += pageSize
	if s.Cursor >= len(s.Rows) {
		s.Cursor = len(s.Rows) - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
}
func (s *Section) PageUp(pageSize int) {
	s.Cursor -= pageSize
	if s.Cursor < 0 {
		s.Cursor = 0
	}
}

func (s *Section) clampCursor() {
	if s.Cursor >= len(s.Rows) {
		s.Cursor = len(s.Rows) - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
}

// ---- Rendering ----

func (s *Section) ViewTable(width, height int) string {
	if s.Loading {
		return spinnerStyle.Render("  ⟳ Loading...")
	}
	if s.Error != "" {
		return errorStyle.Render("  ✗ " + s.Error)
	}
	rendered, newOffset := RenderTable(s.Columns, s.Rows, s.Cursor, s.Offset, width, height)
	s.Offset = newOffset
	return rendered
}

// ---- Sidebar preview content ----

func (s *Section) PreviewContent(width int) string {
	if len(s.Rows) == 0 || s.Cursor < 0 {
		return emptyStyle.Render("Nothing selected")
	}

	wrapWidth := width - 6 // account for sidebar padding
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	switch s.Kind {
	case TabLandings:
		if s.Cursor < len(s.Landings) {
			return renderLandingPreview(s.Landings[s.Cursor], wrapWidth)
		}
	case TabIssues:
		if s.Cursor < len(s.Issues) {
			return renderIssuePreview(s.Issues[s.Cursor], wrapWidth)
		}
	case TabWorkspaces:
		if s.Cursor < len(s.Workspaces) {
			return renderWorkspacePreview(s.Workspaces[s.Cursor])
		}
	case TabWorkflows:
		if s.Cursor < len(s.Workflows) {
			return renderWorkflowPreview(s.Workflows[s.Cursor])
		}
	case TabRepos:
		if s.Cursor < len(s.Repos) {
			return renderRepoPreview(s.Repos[s.Cursor])
		}
	case TabNotifications:
		if s.Cursor < len(s.Notifications) {
			return renderNotificationPreview(s.Notifications[s.Cursor])
		}
	}
	return emptyStyle.Render("Nothing selected")
}

// ---- Preview renderers ----

func renderLandingPreview(l jjhub.Landing, wrap int) string {
	var b strings.Builder
	b.WriteString(sidebarTitleStyle.Render(l.Title))
	b.WriteString("\n")
	b.WriteString(sidebarSubtitleStyle.Render(fmt.Sprintf("Landing #%d", l.Number)))
	b.WriteString("\n\n")

	b.WriteString(fieldRow("State", landingStateIcon(l.State)+" "+l.State))
	b.WriteString(fieldRow("Author", l.Author.Login))
	b.WriteString(fieldRow("Target", sidebarTagStyle.Render(l.TargetBookmark)))
	b.WriteString(fieldRow("Stack", fmt.Sprintf("%d change(s)", len(l.ChangeIDs))))
	if l.ConflictStatus != "" && l.ConflictStatus != "unknown" {
		b.WriteString(fieldRow("Conflicts", l.ConflictStatus))
	}
	b.WriteString(fieldRow("Created", relativeTime(l.CreatedAt)))
	b.WriteString(fieldRow("Updated", relativeTime(l.UpdatedAt)))

	if len(l.ChangeIDs) > 0 {
		b.WriteString("\n")
		b.WriteString(sidebarSectionStyle.Render("Changes"))
		b.WriteString("\n")
		for _, cid := range l.ChangeIDs {
			short := cid
			if len(short) > 12 {
				short = short[:12]
			}
			b.WriteString("  " + dimStyle.Render(short) + "\n")
		}
	}

	if l.Body != "" {
		b.WriteString("\n")
		b.WriteString(sidebarSectionStyle.Render("Description"))
		b.WriteString("\n")
		b.WriteString(sidebarBodyStyle.Render(wordWrap(l.Body, wrap)))
		b.WriteString("\n")
	}
	return b.String()
}

func renderIssuePreview(iss jjhub.Issue, wrap int) string {
	var b strings.Builder
	b.WriteString(sidebarTitleStyle.Render(iss.Title))
	b.WriteString("\n")
	b.WriteString(sidebarSubtitleStyle.Render(fmt.Sprintf("Issue #%d", iss.Number)))
	b.WriteString("\n\n")

	b.WriteString(fieldRow("State", issueStateIcon(iss.State)+" "+iss.State))
	b.WriteString(fieldRow("Author", iss.Author.Login))
	b.WriteString(fieldRow("Comments", fmt.Sprintf("%d", iss.CommentCount)))
	if len(iss.Assignees) > 0 {
		names := make([]string, len(iss.Assignees))
		for i, a := range iss.Assignees {
			names[i] = a.Login
		}
		b.WriteString(fieldRow("Assignees", strings.Join(names, ", ")))
	}
	if len(iss.Labels) > 0 {
		var labels []string
		for _, l := range iss.Labels {
			labels = append(labels, sidebarTagStyle.Render(l.Name))
		}
		b.WriteString(fieldRow("Labels", strings.Join(labels, " ")))
	}
	b.WriteString(fieldRow("Created", relativeTime(iss.CreatedAt)))
	b.WriteString(fieldRow("Updated", relativeTime(iss.UpdatedAt)))

	if iss.Body != "" {
		b.WriteString("\n")
		b.WriteString(sidebarSectionStyle.Render("Description"))
		b.WriteString("\n")
		b.WriteString(sidebarBodyStyle.Render(wordWrap(iss.Body, wrap)))
		b.WriteString("\n")
	}
	return b.String()
}

func renderWorkspacePreview(w jjhub.Workspace) string {
	var b strings.Builder
	name := w.Name
	if name == "" {
		name = "(unnamed)"
	}
	b.WriteString(sidebarTitleStyle.Render(name))
	b.WriteString("\n")
	b.WriteString(sidebarSubtitleStyle.Render("Workspace"))
	b.WriteString("\n\n")

	b.WriteString(fieldRow("Status", workspaceStatusIcon(w.Status)+" "+w.Status))
	b.WriteString(fieldRow("Persistence", w.Persistence))
	if w.SSHHost != nil && *w.SSHHost != "" {
		b.WriteString(fieldRow("SSH", *w.SSHHost))
	}
	if w.IdleTimeoutSeconds > 0 {
		b.WriteString(fieldRow("Idle timeout", fmt.Sprintf("%d min", w.IdleTimeoutSeconds/60)))
	}
	if w.IsFork {
		b.WriteString(fieldRow("Fork", "yes"))
	}
	if w.SuspendedAt != nil {
		b.WriteString(fieldRow("Suspended", relativeTime(*w.SuspendedAt)))
	}
	b.WriteString(fieldRow("VM ID", truncateID(w.FreestyleVMID)))
	b.WriteString(fieldRow("Created", relativeTime(w.CreatedAt)))
	b.WriteString(fieldRow("Updated", relativeTime(w.UpdatedAt)))
	return b.String()
}

func renderWorkflowPreview(wf jjhub.Workflow) string {
	var b strings.Builder
	b.WriteString(sidebarTitleStyle.Render(wf.Name))
	b.WriteString("\n")
	b.WriteString(sidebarSubtitleStyle.Render("Workflow"))
	b.WriteString("\n\n")

	active := closedStyle.Render("inactive")
	if wf.IsActive {
		active = openStyle.Render("active")
	}
	b.WriteString(fieldRow("Status", active))
	b.WriteString(fieldRow("Path", wf.Path))
	b.WriteString(fieldRow("Created", relativeTime(wf.CreatedAt)))
	b.WriteString(fieldRow("Updated", relativeTime(wf.UpdatedAt)))
	return b.String()
}

func renderRepoPreview(r jjhub.Repo) string {
	var b strings.Builder
	b.WriteString(sidebarTitleStyle.Render(r.Name))
	b.WriteString("\n")
	if r.FullName != "" {
		b.WriteString(sidebarSubtitleStyle.Render(r.FullName))
		b.WriteString("\n")
	}
	if r.Description != "" {
		b.WriteString("\n")
		b.WriteString(sidebarBodyStyle.Render(r.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	visibility := openStyle.Render("public")
	if !r.IsPublic {
		visibility = closedStyle.Render("private")
	}
	b.WriteString(fieldRow("Visibility", visibility))
	b.WriteString(fieldRow("Default", sidebarTagStyle.Render(r.DefaultBookmark)))
	b.WriteString(fieldRow("Issues", fmt.Sprintf("%d", r.NumIssues)))
	b.WriteString(fieldRow("Stars", fmt.Sprintf("%d", r.NumStars)))
	b.WriteString(fieldRow("Updated", relativeTime(r.UpdatedAt.Format(time.RFC3339))))
	return b.String()
}

func renderNotificationPreview(n jjhub.Notification) string {
	var b strings.Builder
	b.WriteString(sidebarTitleStyle.Render(n.Title))
	b.WriteString("\n\n")
	b.WriteString(fieldRow("Type", n.Type))
	b.WriteString(fieldRow("Repo", n.RepoName))
	status := dimStyle.Render("read")
	if n.Unread {
		status = openStyle.Render("unread")
	}
	b.WriteString(fieldRow("Status", status))
	b.WriteString(fieldRow("Updated", relativeTime(n.UpdatedAt)))
	return b.String()
}

// ---- Helpers ----

func fieldRow(label, value string) string {
	return sidebarLabelStyle.Width(14).Render(label) + " " + sidebarValueStyle.Render(value) + "\n"
}

func landingStateIcon(state string) string {
	switch state {
	case "open":
		return openStyle.Render("⬆")
	case "merged":
		return mergedStyle.Render("✓")
	case "closed":
		return closedStyle.Render("✗")
	case "draft":
		return draftStyle.Render("◌")
	default:
		return dimStyle.Render("?")
	}
}

func issueStateIcon(state string) string {
	switch state {
	case "open":
		return openStyle.Render("◉")
	case "closed":
		return closedStyle.Render("◎")
	default:
		return dimStyle.Render("?")
	}
}

func workspaceStatusIcon(status string) string {
	switch status {
	case "running":
		return runningStyle.Render("●")
	case "pending":
		return pendingStyle.Render("◌")
	case "stopped":
		return stoppedStyle.Render("○")
	case "failed":
		return failedStyle.Render("✗")
	default:
		return dimStyle.Render("?")
	}
}

func workflowIcon(active bool) string {
	if active {
		return openStyle.Render("⟳")
	}
	return dimStyle.Render("○")
}

func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12] + "…"
	}
	return id
}

func relativeTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return ts
		}
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	// Respect existing newlines.
	paragraphs := strings.Split(s, "\n")
	var result []string
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(p)
		var lines []string
		var current []string
		lineLen := 0
		for _, w := range words {
			wl := len(w)
			if lineLen+wl+len(current) > width && len(current) > 0 {
				lines = append(lines, strings.Join(current, " "))
				current = nil
				lineLen = 0
			}
			current = append(current, w)
			lineLen += wl
		}
		if len(current) > 0 {
			lines = append(lines, strings.Join(current, " "))
		}
		result = append(result, strings.Join(lines, "\n"))
	}
	return strings.Join(result, "\n")
}

func matchesSearch(text, query string) bool {
	return strings.Contains(
		strings.ToLower(text),
		strings.ToLower(query),
	)
}
