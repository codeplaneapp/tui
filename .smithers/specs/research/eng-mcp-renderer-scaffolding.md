# Research: eng-mcp-renderer-scaffolding

**Ticket**: eng-mcp-renderer-scaffolding
**Date**: 2026-04-05
**Author**: Smithers Agent

---

## 1. Audit: Existing Tool Rendering Pipeline

### 1.1 File Inventory

The chat UI rendering pipeline lives entirely in `internal/ui/chat/`:

| File | Role |
|------|------|
| `messages.go` | Core interfaces: `MessageItem`, `ToolMessageItem`, `ToolRenderer`; `cappedMessageWidth()` |
| `tools.go` | `baseToolMessageItem`, `NewToolMessageItem` (factory), `ToolRenderOpts`, all layout helpers |
| `bash.go` | `BashToolRenderContext`, `joinToolParts`, `renderJobTool` |
| `file.go` | `ViewToolRenderContext`, `WriteToolRenderContext`, `EditToolRenderContext`, `MultiEditToolRenderContext` |
| `search.go` | `GlobToolRenderContext`, `GrepToolRenderContext`, `LSToolRenderContext` |
| `fetch.go` | `FetchToolRenderContext`, `WebFetchToolRenderContext`, `WebSearchToolRenderContext`, `DownloadToolRenderContext` |
| `agent.go` | `AgentToolRenderContext`, `AgenticFetchToolRenderContext` |
| `todos.go` | `TodosToolRenderContext` |
| `references.go` | `ReferencesToolRenderContext` |
| `diagnostics.go` | `DiagnosticsToolRenderContext` |
| `lsp_restart.go` | `LSPRestartToolRenderContext` |
| `mcp.go` | `MCPToolRenderContext` — generic MCP renderer (parses `mcp_<server>_<tool>` name format) |
| `docker_mcp.go` | `DockerMCPToolRenderContext` — specialized renderer for Docker MCP tools; includes table rendering via `lipgloss/v2/table` |
| `generic.go` | `GenericToolRenderContext` — final fallback for completely unknown tools |
| `assistant.go` | `AssistantMessageItem` — renders non-tool assistant messages |
| `user.go` | `UserMessageItem` |

### 1.2 Core Interfaces

```go
// ToolRenderer — the one interface every renderer must implement
type ToolRenderer interface {
    RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string
}

// ToolRenderOpts — all context a renderer needs
type ToolRenderOpts struct {
    ToolCall        message.ToolCall
    Result          *message.ToolResult
    Anim            *anim.Anim
    ExpandedContent bool
    Compact         bool
    IsSpinning      bool
    Status          ToolStatus
}
```

`message.ToolCall` carries:
- `Name string` — the full tool name, e.g. `mcp_smithers_runs_list`
- `Input string` — JSON-encoded input params
- `Finished bool`
- `ID string`

`message.ToolResult` carries:
- `Content string` — the result payload (often JSON-encoded)
- `IsError bool`
- `Data string` — for binary content (images)
- `MIMEType string`
- `Metadata string` — JSON-encoded metadata blob (used by bash, view, etc.)

### 1.3 baseToolMessageItem

Every rendered tool is a `*baseToolMessageItem` embedding:
- `*highlightableMessageItem` — text selection support
- `*cachedMessageItem` — render caching (keyed on `width`)
- `*focusableMessageItem` — keyboard focus

The `baseToolMessageItem` stores:
- `toolRenderer ToolRenderer` — injected at construction
- `anim *anim.Anim` — gradient spinner animation (shared across all tools)
- `isCompact bool` — compact/expanded mode (toggled by parent list)
- `expandedContent bool` — expanded output (toggled by user)
- `hasCappedWidth bool` — `true` for most tools; `false` only for Edit/MultiEdit (full-width diffs)

`RawRender(width)` calls `toolRenderer.RenderTool(sty, width, opts)`. The result is cached by width unless the tool is spinning.

---

## 2. Tool Result Flow: Agent → Chat UI → Render

```
Smithers MCP Server (smithers mcp-serve)
    │
    │  stdio / SSE
    ▼
internal/mcp/client.go
    │  publishes tool call + result to internal/pubsub
    ▼
internal/agent/session.go
    │  builds message.ToolCall + message.ToolResult
    ▼
internal/ui/model/ui.go  (Update loop)
    │  receives agent messages via pubsub
    │  calls chat.NewToolMessageItem(sty, msgID, toolCall, result, canceled)
    ▼
internal/ui/chat/tools.go: NewToolMessageItem
    │  switch toolCall.Name → select ToolRenderer
    ▼
ToolMessageItem  (stored in chat list)
    │
    │  on Render(width) call from list
    ▼
baseToolMessageItem.RawRender(width)
    │  checks cache → calls toolRenderer.RenderTool(sty, width, opts)
    ▼
Specific RenderContext.RenderTool(...)
    │  builds header + body string using lipgloss styles
    ▼
Displayed in terminal
```

Key notes on the flow:
1. `NewToolMessageItem` is called **twice** in practice: once when the tool call starts (no result yet, `Finished=false`) and again when the result arrives. Each call returns a new item; the calling code replaces the item in the list.
2. The `Anim` object on `baseToolMessageItem` drives the gradient spinner shown while `!toolCall.Finished && !canceled`.
3. The cache in `cachedMessageItem` is width-keyed. The cache is cleared on `SetResult`, `SetToolCall`, `SetStatus`, `SetCompact`, `ToggleExpanded`.
4. `cappedMessageWidth(availableWidth)` caps at 120 columns for readability. Edit/MultiEdit bypass this cap because diffs benefit from full width.

---

## 3. Registration and Dispatch Mechanism

### 3.1 The Factory Switch

The **only** dispatch point is `NewToolMessageItem` in `internal/ui/chat/tools.go` (lines 201–268):

```go
func NewToolMessageItem(sty, messageID, toolCall, result, canceled) ToolMessageItem {
    var item ToolMessageItem
    switch toolCall.Name {
    case tools.BashToolName:       item = NewBashToolMessageItem(...)
    case tools.ViewToolName:       item = NewViewToolMessageItem(...)
    // ... 16 named cases ...
    default:
        if IsDockerMCPTool(toolCall.Name) {
            item = NewDockerMCPToolMessageItem(...)
        } else if strings.HasPrefix(toolCall.Name, "mcp_") {
            item = NewMCPToolMessageItem(...)
        } else {
            item = NewGenericToolMessageItem(...)
        }
    }
    item.SetMessageID(messageID)
    return item
}
```

There is **no registry struct**. Dispatch is entirely switch/prefix logic. New renderers are added by:
1. Adding a constant for the tool name (in `internal/agent/tools/` or locally)
2. Adding a case to the switch, or inserting a prefix check in the `default` branch

### 3.2 Precedence of the Default Branch

The `default` branch currently applies three checks in order:
1. `IsDockerMCPTool(name)` — matches `mcp_docker-desktop_*` (reads from `config.DockerMCPName`)
2. `strings.HasPrefix(name, "mcp_")` — matches all other MCP tools including Smithers
3. Final fallback: `NewGenericToolMessageItem`

**Smithers tools currently fall into case 2** and are rendered by `MCPToolRenderContext`, which:
- Parses the name as `mcp_<server>_<tool>`
- Displays `Server Name → Tool Name` as the header
- Renders result as pretty-printed JSON if parseable, markdown if it looks like markdown, otherwise plain text

This is the baseline Smithers tools have today.

### 3.3 How Docker MCP Shows the Pattern for Smithers

`docker_mcp.go` is the most instructive reference because it:
1. Intercepts a specific MCP server (`mcp_docker-desktop_*`) before the generic `mcp_` path
2. Uses a per-tool `switch tool` (after stripping the prefix) to extract the right main parameter from input JSON
3. Renders a **table** (`lipgloss/v2/table`) for the `mcp-find` result — demonstrating that structured rendering works in this pipeline
4. Uses a formatted tool name (`Docker MCP → Find`) rather than the generic split approach

Smithers should follow the exact same pattern:
- Intercept `mcp_smithers_*` (or `mcp_<SmithersMCPServer>_*`) before the generic `mcp_` path
- Strip the prefix and dispatch on the remaining tool name
- Render structured output per tool

---

## 4. Smithers MCP Tool Names and Desired Render Formats

### 4.1 Tool Name Scheme

Crush exposes MCP tools as `mcp_<server>_<tool>`. The primary Smithers server name is `smithers` (configurable via the `SmithersMCPServer` template variable in `smithers.md.tpl`). So the prefix to intercept is `mcp_smithers_`.

The full tool list from `smithers.md.tpl` and the PRD §6.13:

| MCP Name (after prefix strip) | Full Tool Name | Category |
|-------------------------------|----------------|----------|
| `runs_list` | `mcp_smithers_runs_list` | Runs |
| `inspect` | `mcp_smithers_inspect` | Observability |
| `chat` | `mcp_smithers_chat` | Observability |
| `logs` | `mcp_smithers_logs` | Observability |
| `approve` | `mcp_smithers_approve` | Control |
| `deny` | `mcp_smithers_deny` | Control |
| `hijack` | `mcp_smithers_hijack` | Control |
| `cancel` | `mcp_smithers_cancel` | Runs |
| `workflow_up` | `mcp_smithers_workflow_up` | Runs |
| `workflow_list` | `mcp_smithers_workflow_list` | Workflows |
| `workflow_run` | `mcp_smithers_workflow_run` | Workflows |
| `diff` | `mcp_smithers_diff` | Time-Travel |
| `fork` | `mcp_smithers_fork` | Time-Travel |
| `replay` | `mcp_smithers_replay` | Time-Travel |
| `memory_list` | `mcp_smithers_memory_list` | Memory |
| `memory_recall` | `mcp_smithers_memory_recall` | Memory |
| `scores` | `mcp_smithers_scores` | Scoring |
| `cron_list` | `mcp_smithers_cron_list` | Cron |
| `sql` | `mcp_smithers_sql` | SQL |

### 4.2 Desired Render Format per Tool

Render format is driven by what the result data looks like and what the user needs at a glance:

| Tool(s) | Input Display | Output Format | Rationale |
|---------|--------------|---------------|-----------|
| `runs_list` | `status=<filter>` | **Table**: ID, Workflow, Status, Step, Time | Mirrors design mockup in DESIGN.md §3.1; dense, scannable |
| `workflow_list` | — | **Table**: Name, Path, Nodes | Dense list |
| `cron_list` | — | **Table**: ID, Workflow, Schedule, Enabled | Mirrors TriggersList GUI tab |
| `scores` | `runID=<id>` or `period` | **Table**: Metric, Value | Numeric output, table is clearest |
| `sql` | `query=<sql>` (truncated) | **Table**: dynamic columns from JSON array | Dynamic, width-adaptive |
| `approve` | `runID=<id>`, `gateID=<id>` | **Card**: run ID, gate task, decision (APPROVED in green) | Action confirmation, prominent |
| `deny` | `runID=<id>`, `gateID=<id>` | **Card**: run ID, gate task, decision (DENIED in red) | Action confirmation, prominent |
| `cancel` | `runID=<id>` | **Card**: run ID, status (CANCELED) | Compact action confirmation |
| `hijack` | `runID=<id>` | **Card**: run ID, agent, instructions for handoff | Signals TUI handoff is about to occur |
| `workflow_up` / `workflow_run` | `workflow=<name>`, `inputs=...` | **Card**: workflow name, run ID, initial status | Confirms run started |
| `inspect` | `runID=<id>` | **Tree**: DAG nodes with status indicators per node | Mirrors NodeInspector.tsx; tree view shows structure |
| `diff` | `runID`, `snapshotA`, `snapshotB` | **Tree/diff**: changed keys per snapshot | Hierarchical JSON diff |
| `fork` / `replay` | `runID`, `snapshot` | **Card**: new run ID, source snapshot | Simple confirmation |
| `chat` | `runID=<id>` | **Plain text** (markdown-friendly): streamed chat history | Chat is prose; code fence already handles it |
| `logs` | `runID=<id>` | **Plain text**: event log lines, truncated at 10 | Log lines are variable-width text |
| `memory_list` | — | **Table**: Key, Value, RunID | Fact list |
| `memory_recall` | `query=<text>` | **Table**: Relevance, Key, Value | Ranked results table |
| `revert` | `runID`, `snapshot` | **Card**: revert confirmation | |

### 4.3 JSON Response Shape Assumptions

Until the `feat-mcp-tool-discovery` ticket lands, exact JSON shapes are inferred from the PRD and GUI source. Safe assumptions:

- **List tools** return a JSON array of objects, or an object with a `data` array field.
- **Action tools** (`approve`, `deny`, `cancel`, `hijack`, `fork`, `replay`, `revert`) return a confirmation object: `{ "success": true, "runId": "...", ... }`.
- **`inspect`** returns a run object with a `nodes` array, each node having `name`, `status`, `output`.
- **`sql`** returns `{ "columns": [...], "rows": [[...], ...] }` or a JSON array of row objects.
- **`diff`** returns an object with `before` and `after` keys, each containing snapshot state.
- **`chat`** and **`logs`** return text content (possibly markdown) or a `{ "messages": [...] }` array.

The renderer **must fail gracefully**: if JSON parsing fails or the shape doesn't match expectations, fall back to the generic JSON pretty-print or plain text path (same as `MCPToolRenderContext` today).

---

## 5. Existing Style Infrastructure Relevant to Smithers

From `internal/ui/styles/styles.go`:

The `Tool` sub-struct already provides:
- `IconSuccess`, `IconError`, `IconPending`, `IconCancelled` — status icons
- `NameNormal`, `NameNested` — tool name label styles
- `ParamMain`, `ParamKey` — parameter display styles
- `ContentLine`, `ContentTruncation`, `ContentCodeLine`, `ContentCodeBg` — body content
- `Body` — padding wrapper (`PaddingLeft(2)`)
- `ErrorTag`, `ErrorMessage` — error display
- `StateWaiting`, `StateCancelled` — in-flight states
- `MCPName`, `MCPToolName`, `MCPArrow` — already used by `MCPToolRenderContext` for `Server → Tool` header formatting
- `DockerMCPActionAdd`, `DockerMCPActionDel` — Docker MCP special action colors (green/red)

Semantic colors available via `sty.*` (direct fields):
- `sty.Green`, `sty.GreenLight`, `sty.GreenDark` — success/running
- `sty.Red`, `sty.RedDark` — error/deny
- `sty.Yellow` — warning/pending
- `sty.Blue`, `sty.BlueLight`, `sty.BlueDark` — info/neutral
- `sty.Subtle` — muted text

The `lipgloss/v2/table` package is already imported in `docker_mcp.go` and is the correct primitive for table rendering in this pipeline.

Styles **needed but not yet defined** for Smithers:
- Status badge styles: `running` (green), `approval` (yellow), `completed` (muted), `failed` (red), `canceled` (subtle)
- Card border style for action confirmations (approve/deny/cancel/hijack/fork)
- Smithers server name style (analogous to `DockerMCPActionAdd` in Docker MCP)

These should be added to the `Tool` sub-struct or to a separate `Tool.Smithers` nested struct.

---

## 6. Key Findings and Constraints

1. **No renderer registry exists.** Dispatch is a single switch + prefix checks in `NewToolMessageItem`. Adding Smithers renderers means adding a prefix check before the generic `mcp_` case. This is low-risk and non-breaking.

2. **The `mcp_<server>_<tool>` naming is a Crush convention**, not an MCP spec. The server name `smithers` is configurable in the template but `smithers` is the hardcoded default. The renderer must handle both `mcp_smithers_*` and potentially `mcp_<customServerName>_*` if the server name is overridden.

3. **`MCPToolRenderContext` is the current Smithers renderer.** It already produces acceptable output for unknown Smithers tools (pretty JSON + markdown detection). The Smithers-specific renderers are improvements on top, not replacements of a broken system.

4. **`docker_mcp.go` is the canonical pattern** for a specialized MCP renderer — it handles prefix detection, per-tool dispatch, table rendering, and action-specific formatting. Smithers follows this exact pattern.

5. **Table rendering is already proven** in `docker_mcp.go` via `lipgloss/v2/table`. The Smithers table renderer can reuse the same approach (no-border table, `StyleFunc` for per-column styles, `Rows(rows...)`, `Width(bodyWidth)`).

6. **`toolHeader` and `joinToolParts` are shared layout helpers** in `tools.go` and `bash.go` respectively. All renderers call them. Smithers renderers must call them too to be visually consistent.

7. **The `ToolRenderOpts.Compact` flag** suppresses the body — renders header only. All renderers must honor it. Most renderers return early when `opts.Compact` is true.

8. **Render caching** is handled automatically by `baseToolMessageItem.RawRender`. Renderers do not need to implement caching themselves; they just need to be pure functions of their inputs.

9. **The `hasCappedWidth` flag** is set to `false` only for Edit/MultiEdit in `newBaseToolMessageItem`. All Smithers tools should use the capped width (default `true`), which limits rendering to 120 columns.

10. **12 MCP tool categories** are described in PRD §6.13. The renderer scaffolding must accommodate all categories via a sub-dispatch switch on the tool name suffix (same pattern as Docker MCP's per-tool `switch tool`).
