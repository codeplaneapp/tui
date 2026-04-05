package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

func smithersStyles() *styles.Styles {
	s := styles.DefaultStyles()
	return &s
}

// makeSmithersOpts creates a ToolRenderOpts for a finished Smithers tool call.
func makeSmithersOpts(tool, input, resultContent string, status ToolStatus) *ToolRenderOpts {
	finished := status == ToolStatusSuccess || status == ToolStatusError
	return &ToolRenderOpts{
		ToolCall: message.ToolCall{
			Name:     smithersMCPPrefix + tool,
			Input:    input,
			Finished: finished,
		},
		Result: &message.ToolResult{
			Content: resultContent,
			IsError: status == ToolStatusError,
		},
		Status: status,
	}
}

// makePendingSmithersOpts creates opts for a pending (not-yet-finished) tool call.
func makePendingSmithersOpts(tool string) *ToolRenderOpts {
	return &ToolRenderOpts{
		ToolCall: message.ToolCall{
			Name:     smithersMCPPrefix + tool,
			Input:    "{}",
			Finished: false,
		},
		Status: ToolStatusRunning,
	}
}

// ─── IsSmithersToolCall ───────────────────────────────────────────────────────

func TestIsSmithersToolCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{"mcp_smithers_runs_list", true},
		{"mcp_smithers_inspect", true},
		{"mcp_smithers_approve", true},
		{"mcp_smithers_sql", true},
		{"mcp_docker-desktop_mcp-find", false},
		{"mcp_anthropic_tool", false},
		{"bash", false},
		{"", false},
		{"smithers_runs_list", false}, // missing mcp_ prefix
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsSmithersToolCall(tt.name))
		})
	}
}

// ─── humanizeTool ────────────────────────────────────────────────────────────

func TestHumanizeTool_KnownTools(t *testing.T) {
	t.Parallel()

	s := &SmithersToolRenderContext{}
	for tool, expected := range smithersToolLabels {
		tool, expected := tool, expected
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, expected, s.humanizeTool(tool))
		})
	}
}

func TestHumanizeTool_UnknownFallback(t *testing.T) {
	t.Parallel()

	s := &SmithersToolRenderContext{}
	// snake_case should become Title Case words
	got := s.humanizeTool("some_unknown_tool")
	assert.Equal(t, "Some Unknown Tool", got)
}

// ─── styleStatus ─────────────────────────────────────────────────────────────

func TestStyleStatus_AllValues(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	statuses := []string{"running", "approval", "waiting", "paused", "completed", "done", "failed", "error", "canceled", "cancelled", "unknown"}
	for _, status := range statuses {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			rendered := s.styleStatus(sty, status, status)
			require.NotEmpty(t, rendered, "styleStatus should not return empty string for status=%q", status)
		})
	}
}

// ─── Pending state ────────────────────────────────────────────────────────────

func TestRenderTool_PendingDoesNotPanic(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	tools := []string{"runs_list", "inspect", "approve", "deny", "cancel", "sql", "workflow_list"}
	for _, tool := range tools {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			opts := makePendingSmithersOpts(tool)
			rendered := s.RenderTool(sty, 120, opts)
			require.NotEmpty(t, rendered)
		})
	}
}

// ─── Compact mode ─────────────────────────────────────────────────────────────

func TestRenderTool_CompactReturnsOneLiner(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("runs_list", "{}", `[{"id":"r1","workflow":"wf","status":"running","step":"1/3","elapsed":"1m"}]`, ToolStatusSuccess)
	opts.Compact = true

	rendered := s.RenderTool(sty, 120, opts)
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	assert.Equal(t, 1, len(lines), "compact mode should render exactly one line")
}

// ─── Error / early-state ──────────────────────────────────────────────────────

func TestRenderTool_ErrorStateRendersWithoutPanic(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("runs_list", "{}", "something went wrong", ToolStatusError)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── runs_list table ──────────────────────────────────────────────────────────

func TestRenderRunsTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"r1","workflow":"wf-a","status":"running","step":"2/5","elapsed":"3m"},{"id":"r2","workflow":"wf-b","status":"completed","step":"5/5","elapsed":"10m"}]`
	opts := makeSmithersOpts("runs_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "r1")
	assert.Contains(t, rendered, "wf-a")
}

func TestRenderRunsTable_EnvelopeShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"data":[{"id":"r3","workflow":"wf-c","status":"failed","step":"1/1","elapsed":"5s"}]}`
	opts := makeSmithersOpts("runs_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "r3")
}

func TestRenderRunsTable_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("runs_list", "{}", "not json at all", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered) // fallback renders something
}

func TestRenderRunsTable_EmptyList(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("runs_list", "{}", `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No runs found")
}

// ─── workflow_list table ──────────────────────────────────────────────────────

func TestRenderWorkflowTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"name":"my-wf","path":"workflows/my-wf.yaml","nodes":4}]`
	opts := makeSmithersOpts("workflow_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "my-wf")
}

func TestRenderWorkflowTable_Empty(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("workflow_list", "{}", `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No workflows found")
}

// ─── scores table ────────────────────────────────────────────────────────────

func TestRenderScoresTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"metric":"accuracy","value":0.95},{"metric":"latency","value":1.23}]`
	opts := makeSmithersOpts("scores", `{"runId":"r1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "accuracy")
	assert.Contains(t, rendered, "0.95")
}

// ─── sql table ────────────────────────────────────────────────────────────────

func TestRenderSQLTable_StructuredShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"columns":["a","b"],"rows":[[1,2],[3,4]]}`
	opts := makeSmithersOpts("sql", `{"query":"SELECT a,b FROM t"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "a")
	assert.Contains(t, rendered, "b")
}

func TestRenderSQLTable_ObjectArrayShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"name":"foo","val":1},{"name":"bar","val":2}]`
	opts := makeSmithersOpts("sql", `{"query":"SELECT name,val FROM t"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "foo")
}

func TestRenderSQLTable_Empty(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("sql", `{"query":"SELECT 1"}`, "", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No results")
}

// ─── approve / deny / cancel cards ───────────────────────────────────────────

func TestRenderApproveCard(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"r-abc","gateId":"g-xyz"}`
	opts := makeSmithersOpts("approve", `{"runId":"r-abc","gateId":"g-xyz"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "APPROVED")
	assert.Contains(t, rendered, "r-abc")
	assert.Contains(t, rendered, "g-xyz")
}

func TestRenderDenyCard(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"r-abc"}`
	opts := makeSmithersOpts("deny", `{"runId":"r-abc"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "DENIED")
}

func TestRenderCancelCard(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"r-abc"}`
	opts := makeSmithersOpts("cancel", `{"runId":"r-abc"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "CANCELED")
}

func TestRenderActionCard_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("approve", `{}`, "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered) // falls back gracefully
}

func TestRenderActionCard_SuccessFalseFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":false,"runId":"r-abc"}`
	opts := makeSmithersOpts("approve", `{"runId":"r-abc"}`, content, ToolStatusSuccess)

	// success=false triggers fallback
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── workflow cards ───────────────────────────────────────────────────────────

func TestRenderWorkflowUp_StartsCard(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"r-new","message":"started"}`
	opts := makeSmithersOpts("workflow_up", `{"workflow":"my-wf"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "STARTED")
	assert.Contains(t, rendered, "r-new")
}

// ─── fork/replay/revert cards ─────────────────────────────────────────────────

func TestRenderForkCard(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"r-forked"}`
	opts := makeSmithersOpts("fork", `{"runId":"r-orig"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "DONE")
}

// ─── inspect tree ─────────────────────────────────────────────────────────────

func TestRenderInspectTree_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"runId":"r1","workflow":"wf","status":"running","nodes":[{"name":"fetch","status":"completed"},{"name":"process","status":"running","children":[{"name":"sub-a","status":"running"}]}]}`
	opts := makeSmithersOpts("inspect", `{"runId":"r1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "fetch")
	assert.Contains(t, rendered, "process")
	assert.Contains(t, rendered, "sub-a")
}

func TestRenderInspectTree_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("inspect", `{"runId":"r1"}`, "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── chat / logs (plain text) ─────────────────────────────────────────────────

func TestRenderChat_PlainText(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("chat", `{"runId":"r1"}`, "Hello from the run!", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "Hello from the run!")
}

func TestRenderLogs_PlainText(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("logs", `{"runId":"r1"}`, "log line 1\nlog line 2", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "log line 1")
}

// ─── memory tables ────────────────────────────────────────────────────────────

func TestRenderMemoryList(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"key":"fact-1","value":"the sky is blue","runId":"r1"}]`
	opts := makeSmithersOpts("memory_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "fact-1")
}

func TestRenderMemoryRecall_HasRelevance(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"key":"fact-1","value":"sky is blue","relevance":0.92}]`
	opts := makeSmithersOpts("memory_recall", `{"query":"sky"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "Relevance")
}

// ─── cron table ───────────────────────────────────────────────────────────────

func TestRenderCronTable(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"c1","workflow":"daily-job","schedule":"0 9 * * *","enabled":true}]`
	opts := makeSmithersOpts("cron_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "daily-job")
}

// ─── unknown Smithers tool fallback ───────────────────────────────────────────

func TestRenderUnknownSmithersTool_JSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"foo":"bar","baz":42}`
	opts := makeSmithersOpts("some_future_tool", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "foo")
}

// ─── diff renderer ────────────────────────────────────────────────────────────

func TestRenderDiff_ValidChanges(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"fromId":"snap-1","toId":"snap-2","changes":[{"path":"state.x","before":1,"after":2,"op":"change"},{"path":"state.y","after":"new","op":"add"}]}`
	opts := makeSmithersOpts("diff", `{"runId":"r1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "state.x")
	assert.Contains(t, rendered, "state.y")
}

func TestRenderDiff_EmptyChangesFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	// Has valid SnapshotDiff shape but no changes — should fall back
	content := `{"fromId":"snap-1","toId":"snap-2","changes":[]}`
	opts := makeSmithersOpts("diff", `{"runId":"r1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered) // fallback still renders
}

func TestRenderDiff_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("diff", `{"runId":"r1"}`, "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── NewToolMessageItem dispatch ─────────────────────────────────────────────

// TestNewToolMessageItem_SmithersDispatched verifies that an mcp_smithers_* tool
// call is dispatched to the Smithers renderer (confirmed by rendered output
// containing the "Smithers" server label).
func TestNewToolMessageItem_SmithersDispatched(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	tc := message.ToolCall{
		ID:       "tc1",
		Name:     "mcp_smithers_runs_list",
		Input:    "{}",
		Finished: true,
	}
	result := &message.ToolResult{Content: `[]`}
	item := NewToolMessageItem(sty, "msg1", tc, result, false)
	// Render and confirm Smithers-specific header is present
	rendered := item.RawRender(120)
	assert.Contains(t, rendered, "Smithers", "Smithers renderer should produce 'Smithers' in header")
}

// TestNewToolMessageItem_GenericMCPNotDispatched verifies that a non-Smithers
// mcp_ tool does NOT use the Smithers renderer (output won't have "Smithers →").
func TestNewToolMessageItem_GenericMCPNotDispatched(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	tc := message.ToolCall{
		ID:       "tc2",
		Name:     "mcp_other_server_tool",
		Input:    "{}",
		Finished: true,
	}
	result := &message.ToolResult{Content: `{}`}
	item := NewToolMessageItem(sty, "msg2", tc, result, false)
	rendered := item.RawRender(120)
	// "Other" (from the generic MCP renderer's prettyName) should appear; "Smithers" should not
	assert.NotContains(t, rendered, "Smithers →", "generic MCP tool should NOT use Smithers renderer")
}

// ─── agent_list table ─────────────────────────────────────────────────────────

func TestRenderAgentTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"a1","name":"planner","available":true,"roles":["planning","routing"]},{"id":"a2","name":"coder","available":false}]`
	opts := makeSmithersOpts("agent_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "planner")
	assert.Contains(t, rendered, "coder")
}

func TestRenderAgentTable_EnvelopeShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"data":[{"id":"a3","name":"reviewer","available":true}]}`
	opts := makeSmithersOpts("agent_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "reviewer")
}

func TestRenderAgentTable_Empty(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("agent_list", "{}", `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No agents found")
}

func TestRenderAgentTable_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("agent_list", "{}", "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── agent_chat plain text ────────────────────────────────────────────────────

func TestRenderAgentChat_PlainText(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("agent_chat", `{"agentId":"a1"}`, "Handoff to planner agent initiated.", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "Handoff to planner agent initiated.")
}

// ─── ticket_list / ticket_search tables ──────────────────────────────────────

func TestRenderTicketTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"t-1","title":"Fix login bug","status":"open"},{"id":"t-2","title":"Add dark mode","status":"closed"}]`
	opts := makeSmithersOpts("ticket_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "t-1")
	assert.Contains(t, rendered, "Fix login bug")
}

func TestRenderTicketSearch_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"t-3","title":"Dark mode request","status":"open"}]`
	opts := makeSmithersOpts("ticket_search", `{"query":"dark mode"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "t-3")
	assert.Contains(t, rendered, "Dark mode request")
}

func TestRenderTicketTable_EnvelopeShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"data":[{"id":"t-4","title":"Refactor auth","status":"in-progress"}]}`
	opts := makeSmithersOpts("ticket_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "t-4")
}

func TestRenderTicketTable_Empty(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("ticket_list", "{}", `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No tickets found")
}

func TestRenderTicketTable_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("ticket_list", "{}", "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── ticket mutation cards ─────────────────────────────────────────────────────

func TestRenderTicketCreate_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"","id":"t-new"}`
	opts := makeSmithersOpts("ticket_create", `{"id":"t-new"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "DONE")
}

func TestRenderTicketUpdate_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":"","message":"updated"}`
	opts := makeSmithersOpts("ticket_update", `{"ticketId":"t-1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "DONE")
}

func TestRenderTicketDelete_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":""}`
	opts := makeSmithersOpts("ticket_delete", `{"ticketId":"t-1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "DONE")
}

// ─── ticket_get fallback ──────────────────────────────────────────────────────

func TestRenderTicketGet_Fallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"id":"t-1","title":"Fix login bug","status":"open","content":"## Description\nLogin fails on Safari."}`
	opts := makeSmithersOpts("ticket_get", `{"ticketId":"t-1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── prompt_list table ────────────────────────────────────────────────────────

func TestRenderPromptTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"system-prompt","entryFile":"prompts/system.md"},{"id":"user-prompt","entryFile":"prompts/user.md"}]`
	opts := makeSmithersOpts("prompt_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "system-prompt")
	assert.Contains(t, rendered, "prompts/system.md")
}

func TestRenderPromptTable_EnvelopeShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"data":[{"id":"base","entryFile":"prompts/base.md"}]}`
	opts := makeSmithersOpts("prompt_list", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "base")
}

func TestRenderPromptTable_Empty(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("prompt_list", "{}", `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No prompts found")
}

func TestRenderPromptTable_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("prompt_list", "{}", "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── prompt_render plain text ─────────────────────────────────────────────────

func TestRenderPromptRender_PlainText(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("prompt_render", `{"promptId":"system-prompt"}`, "You are a helpful assistant.", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "You are a helpful assistant.")
}

// ─── prompt_update card ───────────────────────────────────────────────────────

func TestRenderPromptUpdate_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":""}`
	opts := makeSmithersOpts("prompt_update", `{"promptId":"system-prompt"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "UPDATED")
}

// ─── cron mutation cards ──────────────────────────────────────────────────────

func TestRenderCronAdd_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":""}`
	opts := makeSmithersOpts("cron_add", `{"workflow":"daily-job"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "SCHEDULED")
}

func TestRenderCronRm_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":""}`
	opts := makeSmithersOpts("cron_rm", `{"cronId":"c1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "REMOVED")
}

func TestRenderCronToggle_Card(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"success":true,"runId":""}`
	opts := makeSmithersOpts("cron_toggle", `{"cronId":"c1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "TOGGLED")
}

func TestRenderCronAdd_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("cron_add", `{"workflow":"daily-job"}`, "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── workflow_doctor ──────────────────────────────────────────────────────────

func TestRenderWorkflowDoctor_ValidDiagnostics(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"level":"error","message":"Missing required field","file":"workflows/daily.yaml","line":12},{"level":"warn","message":"Deprecated option used"},{"level":"info","message":"2 nodes found"}]`
	opts := makeSmithersOpts("workflow_doctor", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "Missing required field")
	assert.Contains(t, rendered, "Deprecated option used")
	assert.Contains(t, rendered, "2 nodes found")
}

func TestRenderWorkflowDoctor_EnvelopeShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"data":[{"level":"info","message":"All good"}]}`
	opts := makeSmithersOpts("workflow_doctor", "{}", content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "All good")
}

func TestRenderWorkflowDoctor_NoDiagnostics(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("workflow_doctor", "{}", `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No issues found")
}

func TestRenderWorkflowDoctor_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("workflow_doctor", "{}", "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── timeline table ───────────────────────────────────────────────────────────

func TestRenderTimelineTable_ValidJSON(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `[{"id":"s1","snapshotNo":1,"nodeId":"fetch","label":"before-fetch","createdAt":"2026-04-05T10:00:00Z"},{"id":"s2","snapshotNo":2,"nodeId":"process","createdAt":"2026-04-05T10:01:00Z"}]`
	opts := makeSmithersOpts("timeline", `{"runId":"r1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "fetch")
	assert.Contains(t, rendered, "before-fetch")
}

func TestRenderTimelineTable_EnvelopeShape(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	content := `{"data":[{"id":"s3","snapshotNo":3,"nodeId":"emit","createdAt":"2026-04-05T10:02:00Z"}]}`
	opts := makeSmithersOpts("timeline", `{"runId":"r1"}`, content, ToolStatusSuccess)

	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "emit")
}

func TestRenderTimelineTable_Empty(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("timeline", `{"runId":"r1"}`, `[]`, ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	assert.Contains(t, rendered, "No snapshots found")
}

func TestRenderTimelineTable_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	sty := smithersStyles()
	s := &SmithersToolRenderContext{}

	opts := makeSmithersOpts("timeline", `{"runId":"r1"}`, "not json", ToolStatusSuccess)
	rendered := s.RenderTool(sty, 120, opts)
	require.NotEmpty(t, rendered)
}

// ─── revert primary key ───────────────────────────────────────────────────────

func TestRevertPrimaryKey_InMap(t *testing.T) {
	t.Parallel()

	key, ok := smithersPrimaryKeys["revert"]
	assert.True(t, ok, "revert should be in smithersPrimaryKeys")
	assert.Equal(t, "runId", key)
}
