package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/stringext"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

const smithersMCPPrefix = "mcp_smithers_"

// IsSmithersToolCall returns true if the tool name is a Smithers MCP tool.
// Matches the default server name "smithers" used in SmithersMCPServer template.
func IsSmithersToolCall(name string) bool {
	return strings.HasPrefix(name, smithersMCPPrefix)
}

// smithersToolName strips the MCP prefix and returns the bare tool name.
// e.g. "mcp_smithers_runs_list" → "runs_list"
func smithersToolName(name string) string {
	return strings.TrimPrefix(name, smithersMCPPrefix)
}

// SmithersToolMessageItem is a message item for Smithers MCP tool calls.
type SmithersToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SmithersToolMessageItem)(nil)

// NewSmithersToolMessageItem creates a new [SmithersToolMessageItem].
func NewSmithersToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &SmithersToolRenderContext{}, canceled)
}

// SmithersToolRenderContext renders Smithers MCP tool calls.
type SmithersToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (s *SmithersToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedMessageWidth(width)
	tool := smithersToolName(opts.ToolCall.Name)
	displayName := s.formatToolName(sty, tool)

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	mainParam := s.mainParam(opts, tool)
	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, mainParam)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := s.renderBody(sty, tool, opts, bodyWidth)
	if body == "" {
		return header
	}
	return joinToolParts(header, body)
}

// formatToolName returns "Smithers → <Action>" styled for the tool header.
func (s *SmithersToolRenderContext) formatToolName(sty *styles.Styles, tool string) string {
	action := s.humanizeTool(tool)
	serverPart := sty.Tool.Smithers.ServerName.Render("Smithers")
	arrow := sty.Tool.MCPArrow.String()
	actionPart := sty.Tool.MCPToolName.Render(action)
	return fmt.Sprintf("%s %s %s", serverPart, arrow, actionPart)
}

// smithersToolLabels maps known tool names to their human-readable form.
var smithersToolLabels = map[string]string{
	"runs_list":       "Runs List",
	"inspect":         "Inspect",
	"chat":            "Chat",
	"logs":            "Logs",
	"approve":         "Approve",
	"deny":            "Deny",
	"hijack":          "Hijack",
	"cancel":          "Cancel",
	"workflow_up":     "Start Workflow",
	"workflow_list":   "Workflow List",
	"workflow_run":    "Run Workflow",
	"workflow_doctor": "Workflow Doctor",
	"diff":            "Diff",
	"fork":            "Fork",
	"replay":          "Replay",
	"revert":          "Revert",
	"timeline":        "Timeline",
	"memory_list":     "Memory List",
	"memory_recall":   "Memory Recall",
	"scores":          "Scores",
	"cron_list":       "Cron List",
	"cron_add":        "Add Cron",
	"cron_rm":         "Remove Cron",
	"cron_toggle":     "Toggle Cron",
	"sql":             "SQL",
	"agent_list":      "Agent List",
	"agent_chat":      "Agent Chat",
	"ticket_list":     "Ticket List",
	"ticket_get":      "Get Ticket",
	"ticket_create":   "Create Ticket",
	"ticket_update":   "Update Ticket",
	"ticket_delete":   "Delete Ticket",
	"ticket_search":   "Search Tickets",
	"prompt_list":     "Prompt List",
	"prompt_get":      "Get Prompt",
	"prompt_render":   "Render Prompt",
	"prompt_update":   "Update Prompt",
}

// humanizeTool converts a snake_case tool name to a human-readable form.
// Uses a lookup table for known tools; falls back to Title Case conversion.
func (s *SmithersToolRenderContext) humanizeTool(tool string) string {
	if label, ok := smithersToolLabels[tool]; ok {
		return label
	}
	// Generic fallback: snake_case → Title Case
	parts := strings.Split(tool, "_")
	for i, p := range parts {
		parts[i] = stringext.Capitalize(p)
	}
	return strings.Join(parts, " ")
}

// smithersPrimaryKeys maps tool names to the most informative input parameter key.
var smithersPrimaryKeys = map[string]string{
	"inspect":        "runId",
	"chat":           "runId",
	"logs":           "runId",
	"approve":        "runId",
	"deny":           "runId",
	"hijack":         "runId",
	"cancel":         "runId",
	"workflow_up":    "workflow",
	"workflow_run":   "workflow",
	"diff":           "runId",
	"fork":           "runId",
	"replay":         "runId",
	"revert":         "runId",
	"timeline":       "runId",
	"memory_recall":  "query",
	"sql":            "query",
	"scores":         "runId",
	"cron_add":       "workflow",
	"cron_rm":        "cronId",
	"cron_toggle":    "cronId",
	"agent_chat":     "agentId",
	"ticket_get":     "ticketId",
	"ticket_create":  "id",
	"ticket_update":  "ticketId",
	"ticket_delete":  "ticketId",
	"ticket_search":  "query",
	"prompt_get":     "promptId",
	"prompt_render":  "promptId",
	"prompt_update":  "promptId",
}

// mainParam extracts the most informative single parameter for the header display.
func (s *SmithersToolRenderContext) mainParam(opts *ToolRenderOpts, tool string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return ""
	}
	if key, ok := smithersPrimaryKeys[tool]; ok {
		if val, ok := params[key]; ok {
			if str, ok := val.(string); ok {
				return str
			}
		}
	}
	return ""
}

// renderBody dispatches to a per-tool body renderer.
func (s *SmithersToolRenderContext) renderBody(
	sty *styles.Styles, tool string, opts *ToolRenderOpts, bodyWidth int,
) string {
	switch tool {
	// Tables
	case "runs_list":
		return s.renderRunsTable(sty, opts, bodyWidth)
	case "workflow_list":
		return s.renderWorkflowTable(sty, opts, bodyWidth)
	case "cron_list":
		return s.renderCronTable(sty, opts, bodyWidth)
	case "scores":
		return s.renderScoresTable(sty, opts, bodyWidth)
	case "memory_list", "memory_recall":
		return s.renderMemoryTable(sty, opts, bodyWidth)
	case "sql":
		return s.renderSQLTable(sty, opts, bodyWidth)
	case "agent_list":
		return s.renderAgentTable(sty, opts, bodyWidth)
	case "ticket_list", "ticket_search":
		return s.renderTicketTable(sty, opts, bodyWidth)
	case "prompt_list":
		return s.renderPromptTable(sty, opts, bodyWidth)
	case "timeline":
		return s.renderTimelineTable(sty, opts, bodyWidth)

	// Cards
	case "approve":
		return s.renderActionCard(sty, opts, bodyWidth, "APPROVED", sty.Tool.Smithers.CardApproved)
	case "deny":
		return s.renderActionCard(sty, opts, bodyWidth, "DENIED", sty.Tool.Smithers.CardDenied)
	case "cancel":
		return s.renderActionCard(sty, opts, bodyWidth, "CANCELED", sty.Tool.Smithers.CardCanceled)
	case "hijack":
		return s.renderHijackCard(sty, opts, bodyWidth)
	case "workflow_up", "workflow_run":
		return s.renderActionCard(sty, opts, bodyWidth, "STARTED", sty.Tool.Smithers.CardStarted)
	case "fork", "replay", "revert":
		return s.renderActionCard(sty, opts, bodyWidth, "DONE", sty.Tool.Smithers.CardDone)
	case "cron_add":
		return s.renderActionCard(sty, opts, bodyWidth, "SCHEDULED", sty.Tool.Smithers.CardStarted)
	case "cron_rm":
		return s.renderActionCard(sty, opts, bodyWidth, "REMOVED", sty.Tool.Smithers.CardCanceled)
	case "cron_toggle":
		return s.renderActionCard(sty, opts, bodyWidth, "TOGGLED", sty.Tool.Smithers.CardDone)
	case "ticket_create", "ticket_update", "ticket_delete":
		return s.renderActionCard(sty, opts, bodyWidth, "DONE", sty.Tool.Smithers.CardDone)
	case "prompt_update":
		return s.renderActionCard(sty, opts, bodyWidth, "UPDATED", sty.Tool.Smithers.CardDone)

	// Tree
	case "inspect":
		return s.renderInspectTree(sty, opts, bodyWidth)
	case "diff":
		return s.renderDiff(sty, opts, bodyWidth)
	case "workflow_doctor":
		return s.renderWorkflowDoctorOutput(sty, opts, bodyWidth)

	// Plain text (prose content)
	case "chat", "logs", "agent_chat", "prompt_render":
		return sty.Tool.Body.Render(
			toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))

	// Fallback: pretty JSON → markdown → plain text
	case "ticket_get", "prompt_get":
		return s.renderFallback(sty, opts, bodyWidth)

	default:
		return s.renderFallback(sty, opts, bodyWidth)
	}
}

// ─── Data Types ──────────────────────────────────────────────────────────────

// RunEntry is the expected shape of a single run in the runs_list result.
type RunEntry struct {
	ID       string `json:"id"`
	Workflow string `json:"workflow"`
	Status   string `json:"status"`
	Step     string `json:"step"`    // e.g. "3/5"
	Elapsed  string `json:"elapsed"` // e.g. "2m14s"
}

// WorkflowEntry is the expected shape of a workflow in workflow_list.
type WorkflowEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Nodes int    `json:"nodes"`
}

// CronEntry is the expected shape of a cron trigger in cron_list.
type CronEntry struct {
	ID       string `json:"id"`
	Workflow string `json:"workflow"`
	Schedule string `json:"schedule"`
	Enabled  bool   `json:"enabled"`
}

// ScoreEntry is the expected shape of a single score metric.
type ScoreEntry struct {
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
}

// MemoryEntry is the expected shape of a memory record.
type MemoryEntry struct {
	Key       string  `json:"key"`
	Value     string  `json:"value"`
	RunID     string  `json:"runId,omitempty"`
	Relevance float64 `json:"relevance,omitempty"`
}

// ActionConfirmation is the expected shape of an action tool result.
type ActionConfirmation struct {
	Success bool   `json:"success"`
	RunID   string `json:"runId"`
	GateID  string `json:"gateId,omitempty"`
	Message string `json:"message,omitempty"`
}

// HijackConfirmation is the shape for hijack tool result (may include agent info).
type HijackConfirmation struct {
	Success      bool   `json:"success"`
	RunID        string `json:"runId"`
	Agent        string `json:"agent,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// NodeEntry is the expected shape of a node in the inspect result.
type NodeEntry struct {
	Name     string      `json:"name"`
	Status   string      `json:"status"`
	Output   string      `json:"output,omitempty"`
	Children []NodeEntry `json:"children,omitempty"`
}

// InspectResult is the expected shape of the inspect tool result.
type InspectResult struct {
	RunID    string      `json:"runId"`
	Workflow string      `json:"workflow"`
	Status   string      `json:"status"`
	Nodes    []NodeEntry `json:"nodes"`
}

// SQLResult is the expected shape of a sql tool result.
type SQLResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

// AgentEntry is the expected shape of a single agent in agent_list.
type AgentEntry struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Available bool     `json:"available"`
	Roles     []string `json:"roles,omitempty"`
}

// TicketEntry is the expected shape of a single ticket in ticket_list / ticket_search.
type TicketEntry struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status,omitempty"`
	Content   string `json:"content,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// PromptEntry is the expected shape of a single prompt in prompt_list.
type PromptEntry struct {
	ID        string `json:"id"`
	EntryFile string `json:"entryFile,omitempty"`
}

// WorkflowDiagnostic is the expected shape of a single diagnostic from workflow_doctor.
type WorkflowDiagnostic struct {
	Level   string `json:"level"` // "error", "warn", "info"
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// SnapshotSummary is the expected shape of a single snapshot entry in timeline.
type SnapshotSummary struct {
	ID         string `json:"id"`
	SnapshotNo int    `json:"snapshotNo"`
	Label      string `json:"label,omitempty"`
	NodeID     string `json:"nodeId,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
}

// DiffEntry is a single change in a SnapshotDiff result.
type DiffEntry struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
	Op     string `json:"op"` // "add", "remove", "change"
}

// SnapshotDiff is the expected shape of a diff tool result.
type SnapshotDiff struct {
	FromID  string      `json:"fromId"`
	ToID    string      `json:"toId"`
	Changes []DiffEntry `json:"changes"`
}

// ─── Table Renderers ─────────────────────────────────────────────────────────

// maxTableRows limits the number of rows shown without expansion.
const maxTableRows = 15

// renderRunsTable renders a runs_list result as a table.
func (s *SmithersToolRenderContext) renderRunsTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var runs []RunEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &runs); err != nil {
		var envelope struct {
			Data []RunEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		runs = envelope.Data
	}
	if len(runs) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No runs found."))
	}

	shown := runs
	extra := ""
	if !opts.ExpandedContent && len(runs) > maxTableRows {
		shown = runs[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(runs)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, r := range shown {
		rows = append(rows, []string{
			sty.Base.Render(r.ID),
			sty.Base.Render(r.Workflow),
			s.styleStatus(sty, r.Status, r.Status),
			sty.Subtle.Render(r.Step),
			sty.Subtle.Render(r.Elapsed),
		})
	}

	t := smithersTable(sty, []string{"ID", "Workflow", "Status", "Step", "Time"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderWorkflowTable renders a workflow_list result as a table.
func (s *SmithersToolRenderContext) renderWorkflowTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var workflows []WorkflowEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &workflows); err != nil {
		var envelope struct {
			Data []WorkflowEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		workflows = envelope.Data
	}
	if len(workflows) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No workflows found."))
	}

	shown := workflows
	extra := ""
	if !opts.ExpandedContent && len(workflows) > maxTableRows {
		shown = workflows[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(workflows)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, w := range shown {
		rows = append(rows, []string{
			sty.Base.Render(w.Name),
			sty.Subtle.Render(w.Path),
			sty.Subtle.Render(fmt.Sprintf("%d", w.Nodes)),
		})
	}

	t := smithersTable(sty, []string{"Name", "Path", "Nodes"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderCronTable renders a cron_list result as a table.
func (s *SmithersToolRenderContext) renderCronTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var crons []CronEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &crons); err != nil {
		var envelope struct {
			Data []CronEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		crons = envelope.Data
	}
	if len(crons) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No cron triggers found."))
	}

	shown := crons
	extra := ""
	if !opts.ExpandedContent && len(crons) > maxTableRows {
		shown = crons[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(crons)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, c := range shown {
		enabled := sty.Subtle.Render("no")
		if c.Enabled {
			enabled = sty.Tool.Smithers.StatusRunning.Render("yes")
		}
		rows = append(rows, []string{
			sty.Base.Render(c.ID),
			sty.Base.Render(c.Workflow),
			sty.Subtle.Render(c.Schedule),
			enabled,
		})
	}

	t := smithersTable(sty, []string{"ID", "Workflow", "Schedule", "Enabled"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderScoresTable renders a scores result as a table.
func (s *SmithersToolRenderContext) renderScoresTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var scores []ScoreEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &scores); err != nil {
		var envelope struct {
			Data []ScoreEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		scores = envelope.Data
	}
	if len(scores) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No scores found."))
	}

	rows := make([][]string, 0, len(scores))
	for _, sc := range scores {
		rows = append(rows, []string{
			sty.Base.Render(sc.Metric),
			sty.Base.Render(fmt.Sprintf("%.4g", sc.Value)),
		})
	}

	t := smithersTable(sty, []string{"Metric", "Value"}, rows, width)
	return sty.Tool.Body.Render(t.Render())
}

// renderMemoryTable renders a memory_list or memory_recall result as a table.
func (s *SmithersToolRenderContext) renderMemoryTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var entries []MemoryEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &entries); err != nil {
		var envelope struct {
			Data []MemoryEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		entries = envelope.Data
	}
	if len(entries) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No memory entries found."))
	}

	shown := entries
	extra := ""
	if !opts.ExpandedContent && len(entries) > maxTableRows {
		shown = entries[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(entries)-maxTableRows))
	}

	// Determine if this is a recall result (has Relevance field).
	hasRelevance := false
	for _, e := range shown {
		if e.Relevance != 0 {
			hasRelevance = true
			break
		}
	}

	var headers []string
	var rows [][]string
	if hasRelevance {
		headers = []string{"Relevance", "Key", "Value"}
		for _, e := range shown {
			rows = append(rows, []string{
				sty.Subtle.Render(fmt.Sprintf("%.2f", e.Relevance)),
				sty.Base.Render(e.Key),
				sty.Subtle.Render(e.Value),
			})
		}
	} else {
		headers = []string{"Key", "Value", "RunID"}
		for _, e := range shown {
			rows = append(rows, []string{
				sty.Base.Render(e.Key),
				sty.Subtle.Render(e.Value),
				sty.Subtle.Render(e.RunID),
			})
		}
	}

	t := smithersTable(sty, headers, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderSQLTable renders a sql tool result as a dynamic table.
// Supports both {"columns":[...],"rows":[[...],...]} and [{...},...] shapes.
func (s *SmithersToolRenderContext) renderSQLTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	content := opts.Result.Content
	if content == "" {
		return sty.Tool.Body.Render(sty.Subtle.Render("No results."))
	}

	// Try structured {"columns": [...], "rows": [[...],...]} shape first.
	var structured SQLResult
	if err := json.Unmarshal([]byte(content), &structured); err == nil && len(structured.Columns) > 0 {
		if len(structured.Rows) == 0 {
			return sty.Tool.Body.Render(sty.Subtle.Render("No rows returned."))
		}

		shown := structured.Rows
		extra := ""
		if !opts.ExpandedContent && len(structured.Rows) > maxTableRows {
			shown = structured.Rows[:maxTableRows]
			extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(structured.Rows)-maxTableRows))
		}

		rows := make([][]string, 0, len(shown))
		for _, row := range shown {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = sty.Base.Render(fmt.Sprintf("%v", cell))
			}
			rows = append(rows, cells)
		}

		t := smithersTable(sty, structured.Columns, rows, width)
		out := t.Render()
		if extra != "" {
			out += "\n" + extra
		}
		return sty.Tool.Body.Render(out)
	}

	// Try array-of-objects shape: [{col1:val1, col2:val2},...].
	var objRows []map[string]interface{}
	if err := json.Unmarshal([]byte(content), &objRows); err == nil && len(objRows) > 0 {
		// Collect ordered columns from the first row.
		colSet := make(map[string]bool)
		var columns []string
		for k := range objRows[0] {
			if !colSet[k] {
				colSet[k] = true
				columns = append(columns, k)
			}
		}

		shown := objRows
		extra := ""
		if !opts.ExpandedContent && len(objRows) > maxTableRows {
			shown = objRows[:maxTableRows]
			extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(objRows)-maxTableRows))
		}

		rows := make([][]string, 0, len(shown))
		for _, rowObj := range shown {
			cells := make([]string, len(columns))
			for i, col := range columns {
				cells[i] = sty.Base.Render(fmt.Sprintf("%v", rowObj[col]))
			}
			rows = append(rows, cells)
		}

		t := smithersTable(sty, columns, rows, width)
		out := t.Render()
		if extra != "" {
			out += "\n" + extra
		}
		return sty.Tool.Body.Render(out)
	}

	return s.renderFallback(sty, opts, width)
}

// renderAgentTable renders an agent_list result as a table.
func (s *SmithersToolRenderContext) renderAgentTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var agents []AgentEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &agents); err != nil {
		var envelope struct {
			Data []AgentEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		agents = envelope.Data
	}
	if len(agents) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No agents found."))
	}

	shown := agents
	extra := ""
	if !opts.ExpandedContent && len(agents) > maxTableRows {
		shown = agents[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(agents)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, a := range shown {
		available := sty.Subtle.Render("no")
		if a.Available {
			available = sty.Tool.Smithers.StatusRunning.Render("yes")
		}
		roles := sty.Subtle.Render("—")
		if len(a.Roles) > 0 {
			roles = sty.Subtle.Render(strings.Join(a.Roles, ", "))
		}
		rows = append(rows, []string{
			sty.Base.Render(a.Name),
			available,
			roles,
		})
	}

	t := smithersTable(sty, []string{"Name", "Available", "Roles"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderTicketTable renders a ticket_list or ticket_search result as a table.
func (s *SmithersToolRenderContext) renderTicketTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var tickets []TicketEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &tickets); err != nil {
		var envelope struct {
			Data []TicketEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		tickets = envelope.Data
	}
	if len(tickets) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No tickets found."))
	}

	shown := tickets
	extra := ""
	if !opts.ExpandedContent && len(tickets) > maxTableRows {
		shown = tickets[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(tickets)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, tk := range shown {
		rows = append(rows, []string{
			sty.Base.Render(tk.ID),
			sty.Base.Render(tk.Title),
			s.styleStatus(sty, tk.Status, tk.Status),
		})
	}

	t := smithersTable(sty, []string{"ID", "Title", "Status"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderPromptTable renders a prompt_list result as a table.
func (s *SmithersToolRenderContext) renderPromptTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var prompts []PromptEntry
	if err := json.Unmarshal([]byte(opts.Result.Content), &prompts); err != nil {
		var envelope struct {
			Data []PromptEntry `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		prompts = envelope.Data
	}
	if len(prompts) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No prompts found."))
	}

	shown := prompts
	extra := ""
	if !opts.ExpandedContent && len(prompts) > maxTableRows {
		shown = prompts[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(prompts)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, p := range shown {
		rows = append(rows, []string{
			sty.Base.Render(p.ID),
			sty.Subtle.Render(p.EntryFile),
		})
	}

	t := smithersTable(sty, []string{"ID", "Entry File"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// renderTimelineTable renders a timeline result as a table of snapshots.
func (s *SmithersToolRenderContext) renderTimelineTable(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var snapshots []SnapshotSummary
	if err := json.Unmarshal([]byte(opts.Result.Content), &snapshots); err != nil {
		var envelope struct {
			Data []SnapshotSummary `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		snapshots = envelope.Data
	}
	if len(snapshots) == 0 {
		return sty.Tool.Body.Render(sty.Subtle.Render("No snapshots found."))
	}

	shown := snapshots
	extra := ""
	if !opts.ExpandedContent && len(snapshots) > maxTableRows {
		shown = snapshots[:maxTableRows]
		extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(snapshots)-maxTableRows))
	}

	rows := make([][]string, 0, len(shown))
	for _, snap := range shown {
		label := snap.Label
		if label == "" {
			label = "—"
		}
		rows = append(rows, []string{
			sty.Subtle.Render(fmt.Sprintf("%d", snap.SnapshotNo)),
			sty.Base.Render(snap.NodeID),
			sty.Base.Render(label),
			sty.Subtle.Render(snap.CreatedAt),
		})
	}

	t := smithersTable(sty, []string{"No.", "Node", "Label", "Created At"}, rows, width)
	out := t.Render()
	if extra != "" {
		out += "\n" + extra
	}
	return sty.Tool.Body.Render(out)
}

// ─── Card Renderers ───────────────────────────────────────────────────────────

// renderActionCard renders a generic action confirmation card.
func (s *SmithersToolRenderContext) renderActionCard(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
	badge string, badgeStyle lipgloss.Style,
) string {
	var conf ActionConfirmation
	if err := json.Unmarshal([]byte(opts.Result.Content), &conf); err != nil || !conf.Success {
		return s.renderFallback(sty, opts, width)
	}

	badgeRendered := badgeStyle.Render(badge)
	runLine := fmt.Sprintf("%s  %s  %s",
		badgeRendered,
		sty.Tool.Smithers.CardLabel.Render("run"),
		sty.Tool.Smithers.CardValue.Render(conf.RunID),
	)

	var lines []string
	lines = append(lines, runLine)
	if conf.GateID != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s",
			sty.Tool.Smithers.CardLabel.Render("gate"),
			sty.Tool.Smithers.CardValue.Render(conf.GateID),
		))
	}
	if conf.Message != "" {
		lines = append(lines, "  "+sty.Subtle.Render(conf.Message))
	}

	return sty.Tool.Body.Render(strings.Join(lines, "\n"))
}

// renderHijackCard renders the hijack tool card, which includes handoff context.
func (s *SmithersToolRenderContext) renderHijackCard(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var conf HijackConfirmation
	if err := json.Unmarshal([]byte(opts.Result.Content), &conf); err != nil || !conf.Success {
		return s.renderFallback(sty, opts, width)
	}

	badge := sty.Tool.Smithers.CardStarted.Render("HIJACKED")
	runLine := fmt.Sprintf("%s  %s  %s",
		badge,
		sty.Tool.Smithers.CardLabel.Render("run"),
		sty.Tool.Smithers.CardValue.Render(conf.RunID),
	)

	var lines []string
	lines = append(lines, runLine)
	if conf.Agent != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s",
			sty.Tool.Smithers.CardLabel.Render("agent"),
			sty.Tool.Smithers.CardValue.Render(conf.Agent),
		))
	}
	if conf.Instructions != "" {
		lines = append(lines, "  "+sty.Subtle.Render(conf.Instructions))
	}

	return sty.Tool.Body.Render(strings.Join(lines, "\n"))
}

// ─── Tree Renderers ───────────────────────────────────────────────────────────

// renderInspectTree renders an inspect tool result as a lipgloss tree.
func (s *SmithersToolRenderContext) renderInspectTree(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var result InspectResult
	if err := json.Unmarshal([]byte(opts.Result.Content), &result); err != nil {
		return s.renderFallback(sty, opts, width)
	}

	rootLabel := fmt.Sprintf("%s  %s",
		s.styleStatus(sty, result.Status, "●"),
		sty.Base.Render(result.RunID+" / "+result.Workflow),
	)
	root := tree.Root(rootLabel)

	for _, node := range result.Nodes {
		root.Child(s.buildNodeTree(sty, node))
	}

	return sty.Tool.Body.Render(root.String())
}

// buildNodeTree recursively builds a lipgloss tree node.
func (s *SmithersToolRenderContext) buildNodeTree(sty *styles.Styles, node NodeEntry) *tree.Tree {
	label := fmt.Sprintf("%s  %s",
		s.nodeStatusIcon(sty, node.Status),
		sty.Base.Render(node.Name),
	)
	t := tree.Root(label)
	for _, child := range node.Children {
		t.Child(s.buildNodeTree(sty, child))
	}
	return t
}

// nodeStatusIcon returns the styled icon for a node's status.
func (s *SmithersToolRenderContext) nodeStatusIcon(sty *styles.Styles, status string) string {
	switch strings.ToLower(status) {
	case "running":
		return sty.Tool.Smithers.TreeNodeRunning.Render("●")
	case "completed", "done":
		return sty.Tool.Smithers.TreeNodeComplete.Render("✓")
	case "failed", "error":
		return sty.Tool.Smithers.TreeNodeFailed.Render("×")
	default:
		return sty.Tool.Smithers.TreeNodePending.Render("○")
	}
}

// renderDiff renders a diff tool result as a list of changes.
// Each change is shown as "op  path  before → after". Falls back to renderFallback
// if the content does not match the expected SnapshotDiff shape.
func (s *SmithersToolRenderContext) renderDiff(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var diff SnapshotDiff
	if err := json.Unmarshal([]byte(opts.Result.Content), &diff); err != nil || len(diff.Changes) == 0 {
		return s.renderFallback(sty, opts, width)
	}

	var lines []string
	for _, ch := range diff.Changes {
		op := s.styleDiffOp(sty, ch.Op)
		before := fmt.Sprintf("%v", ch.Before)
		after := fmt.Sprintf("%v", ch.After)
		lines = append(lines, fmt.Sprintf("%s  %s  %s → %s",
			op,
			sty.Base.Render(ch.Path),
			sty.Subtle.Render(before),
			sty.Base.Render(after),
		))
	}
	return sty.Tool.Body.Render(strings.Join(lines, "\n"))
}

// styleDiffOp applies styling to a diff operation label ("add", "remove", "change").
func (s *SmithersToolRenderContext) styleDiffOp(sty *styles.Styles, op string) string {
	switch strings.ToLower(op) {
	case "add":
		return sty.Tool.Smithers.StatusRunning.Render("+ add   ")
	case "remove":
		return sty.Tool.Smithers.StatusFailed.Render("- remove")
	default:
		return sty.Tool.Smithers.StatusApproval.Render("~ change")
	}
}

// renderWorkflowDoctorOutput renders workflow_doctor diagnostics as styled lines.
// Each diagnostic is prefixed with a level badge (error/warn/info).
// Falls back to renderFallback if the content cannot be parsed.
func (s *SmithersToolRenderContext) renderWorkflowDoctorOutput(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	var diagnostics []WorkflowDiagnostic
	if err := json.Unmarshal([]byte(opts.Result.Content), &diagnostics); err != nil {
		var envelope struct {
			Data []WorkflowDiagnostic `json:"data"`
		}
		if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
			return s.renderFallback(sty, opts, width)
		}
		diagnostics = envelope.Data
	}
	if len(diagnostics) == 0 {
		return sty.Tool.Body.Render(sty.Tool.Smithers.StatusComplete.Render("No issues found."))
	}

	var lines []string
	for _, d := range diagnostics {
		badge := s.styleDiagnosticLevel(sty, d.Level)
		loc := ""
		if d.File != "" {
			loc = sty.Subtle.Render(fmt.Sprintf(" %s", d.File))
			if d.Line > 0 {
				loc = sty.Subtle.Render(fmt.Sprintf(" %s:%d", d.File, d.Line))
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s%s", badge, sty.Base.Render(d.Message), loc))
	}
	return sty.Tool.Body.Render(strings.Join(lines, "\n"))
}

// styleDiagnosticLevel returns a styled badge for a diagnostic severity level.
func (s *SmithersToolRenderContext) styleDiagnosticLevel(sty *styles.Styles, level string) string {
	switch strings.ToLower(level) {
	case "error":
		return sty.Tool.Smithers.CardDenied.Render("ERROR")
	case "warn", "warning":
		return sty.Tool.Smithers.StatusApproval.Render("WARN ")
	default:
		return sty.Subtle.Render("INFO ")
	}
}

// ─── Status Styling ───────────────────────────────────────────────────────────

// styleStatus applies the appropriate Smithers status style to a label.
func (s *SmithersToolRenderContext) styleStatus(sty *styles.Styles, status, label string) string {
	switch strings.ToLower(status) {
	case "running":
		return sty.Tool.Smithers.StatusRunning.Render(label)
	case "approval", "waiting", "paused":
		return sty.Tool.Smithers.StatusApproval.Render(label)
	case "completed", "done":
		return sty.Tool.Smithers.StatusComplete.Render(label)
	case "failed", "error":
		return sty.Tool.Smithers.StatusFailed.Render(label)
	case "canceled", "cancelled":
		return sty.Tool.Smithers.StatusCanceled.Render(label)
	default:
		return sty.Subtle.Render(label)
	}
}

// ─── Fallback Renderer ────────────────────────────────────────────────────────

// renderFallback renders the result as pretty JSON, markdown, or plain text.
// This is the same approach as MCPToolRenderContext and is the last resort for
// unknown or malformed Smithers responses.
func (s *SmithersToolRenderContext) renderFallback(
	sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
	if opts.Result == nil || opts.Result.Content == "" {
		return ""
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(opts.Result.Content), &raw); err == nil {
		prettyResult, err := json.MarshalIndent(raw, "", "  ")
		if err == nil {
			return sty.Tool.Body.Render(
				toolOutputCodeContent(sty, "result.json", string(prettyResult), 0, width, opts.ExpandedContent))
		}
	}
	if looksLikeMarkdown(opts.Result.Content) {
		return sty.Tool.Body.Render(
			toolOutputCodeContent(sty, "result.md", opts.Result.Content, 0, width, opts.ExpandedContent))
	}
	return sty.Tool.Body.Render(
		toolOutputPlainContent(sty, opts.Result.Content, width, opts.ExpandedContent))
}

// ─── Table Helper ─────────────────────────────────────────────────────────────

// smithersTable builds a borderless lipgloss table consistent with Docker MCP tables.
func smithersTable(sty *styles.Styles, headers []string, rows [][]string, width int) *table.Table {
	return table.New().
		Wrap(false).
		BorderTop(false).
		BorderBottom(false).
		BorderRight(false).
		BorderLeft(false).
		BorderColumn(false).
		BorderRow(false).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return sty.Tool.Smithers.TableHeader
			}
			return lipgloss.NewStyle().PaddingRight(2)
		}).
		Rows(rows...).
		Width(width)
}
