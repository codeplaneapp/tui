# Implementation Plan: eng-mcp-renderer-scaffolding

## Goal

Create the scaffolding for Smithers-specific MCP tool renderers in the chat UI. After this ticket, every `mcp_smithers_*` tool call produces a rich, semantically appropriate terminal rendering instead of the generic pretty-JSON fallback. Twelve MCP tool categories are covered. The scaffolding establishes patterns that all subsequent Smithers chat features depend on.

---

## Architecture: Renderer Registry

### Decision: Prefix Intercept, Not a Struct Registry

There is no renderer registry in Crush today. Dispatch is a single `switch`/prefix chain in `NewToolMessageItem`. A formal `map[string]RendererFactory` registry would be cleaner long-term but is not necessary to achieve the ticket's goals and would require changes across more files than warranted.

The chosen approach mirrors `docker_mcp.go` exactly:

1. Add a `IsSmithersToolCall(name string) bool` helper (analogous to `IsDockerMCPTool`).
2. Insert a check **before** the generic `mcp_` case in `NewToolMessageItem`'s default branch.
3. Implement `NewSmithersToolMessageItem` + `SmithersToolRenderContext` in a new file.
4. Inside `SmithersToolRenderContext.RenderTool`, dispatch on the suffix via a `switch tool` (strip prefix, route to per-tool render helper).

This intercept pattern is:
- Non-breaking: falls through to `MCPToolRenderContext` if misconfigured
- Self-contained: all Smithers rendering lives in one new file
- Consistent: identical to the Docker MCP precedent already in the codebase

### Dispatch Chain After This Ticket

```
NewToolMessageItem(toolCall.Name)
  │
  ├─ case BashToolName, ViewToolName, ... (16 named cases)  → specific renderer
  │
  default:
  ├─ IsDockerMCPTool(name)?      → DockerMCPToolRenderContext   (unchanged)
  ├─ IsSmithersToolCall(name)?   → SmithersToolRenderContext     (NEW)
  ├─ HasPrefix "mcp_"?           → MCPToolRenderContext          (fallback for other MCP servers)
  └─ else                        → GenericToolRenderContext
```

---

## Steps

### Step 1: Smithers Style Additions

**File**: `internal/ui/styles/smithers_styles.go` (new)

Add Smithers-specific style constants and a `SmithersStyles` struct that is embedded in the existing `Tool` struct. Because `styles.go` is large (400+ lines), the Smithers additions go in a separate file that extends the package.

Define Smithers status badge colors as named constants, then add a `SmithersStyles` sub-struct:

```go
package styles

import "charm.land/lipgloss/v2"

// SmithersStyles holds tool-rendering styles specific to Smithers MCP tools.
// It is embedded in the Tool sub-struct of Styles.
type SmithersStyles struct {
    // Server label — the "Smithers" part of "Smithers → runs list"
    ServerName lipgloss.Style

    // Run status badges
    StatusRunning  lipgloss.Style // green
    StatusApproval lipgloss.Style // yellow
    StatusComplete lipgloss.Style // muted green
    StatusFailed   lipgloss.Style // red
    StatusCanceled lipgloss.Style // subtle/grey
    StatusPaused   lipgloss.Style // yellow-ish

    // Action card styles
    CardBorder     lipgloss.Style // card border box
    CardTitle      lipgloss.Style // card header text
    CardValue      lipgloss.Style // card value text
    CardLabel      lipgloss.Style // card field label (muted)
    CardApproved   lipgloss.Style // "APPROVED" badge (green bg)
    CardDenied     lipgloss.Style // "DENIED" badge (red bg)
    CardCanceled   lipgloss.Style // "CANCELED" badge (subtle)
    CardStarted    lipgloss.Style // "STARTED" badge (blue)

    // Table header
    TableHeader lipgloss.Style // column header row

    // Tree node indicator styles
    TreeNodeRunning  lipgloss.Style // ● green
    TreeNodeComplete lipgloss.Style // ✓ green
    TreeNodeFailed   lipgloss.Style // × red
    TreeNodePending  lipgloss.Style // ○ subtle
}
```

In `styles.go`, add `Smithers SmithersStyles` to the `Tool` struct:

```go
Tool struct {
    // ... existing fields ...

    // Smithers-specific tool rendering styles.
    Smithers SmithersStyles
}
```

In the `New()` / theme initialization function of `styles.go`, populate `sty.Tool.Smithers`:

```go
// Smithers styles
sty.Tool.Smithers = SmithersStyles{
    ServerName:     lipgloss.NewStyle().Foreground(sty.Primary).Bold(true),
    StatusRunning:  lipgloss.NewStyle().Foreground(sty.Green),
    StatusApproval: lipgloss.NewStyle().Foreground(sty.Yellow),
    StatusComplete: lipgloss.NewStyle().Foreground(sty.GreenLight),
    StatusFailed:   lipgloss.NewStyle().Foreground(sty.Red),
    StatusCanceled: lipgloss.NewStyle().Foreground(sty.FgSubtle),
    StatusPaused:   lipgloss.NewStyle().Foreground(sty.Yellow),

    CardBorder:   lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).
                    BorderForeground(sty.Border).Padding(0, 1),
    CardTitle:    lipgloss.NewStyle().Bold(true).Foreground(sty.FgBase),
    CardValue:    lipgloss.NewStyle().Foreground(sty.FgBase),
    CardLabel:    lipgloss.NewStyle().Foreground(sty.FgMuted),
    CardApproved: lipgloss.NewStyle().Background(sty.Green).
                    Foreground(sty.White).Bold(true).Padding(0, 1),
    CardDenied:   lipgloss.NewStyle().Background(sty.Red).
                    Foreground(sty.White).Bold(true).Padding(0, 1),
    CardCanceled: lipgloss.NewStyle().Background(sty.BgSubtle).
                    Foreground(sty.FgMuted).Bold(true).Padding(0, 1),
    CardStarted:  lipgloss.NewStyle().Background(sty.Blue).
                    Foreground(sty.White).Bold(true).Padding(0, 1),

    TableHeader: lipgloss.NewStyle().Foreground(sty.FgMuted).Bold(true),

    TreeNodeRunning:  lipgloss.NewStyle().Foreground(sty.Green),
    TreeNodeComplete: lipgloss.NewStyle().Foreground(sty.GreenLight),
    TreeNodeFailed:   lipgloss.NewStyle().Foreground(sty.Red),
    TreeNodePending:  lipgloss.NewStyle().Foreground(sty.FgSubtle),
}
```

### Step 2: Smithers Renderer File

**File**: `internal/ui/chat/smithers_mcp.go` (new)

This is the primary deliverable. It contains:

#### 2a. Constructor and Interface

```go
package chat

import (
    "strings"
    "github.com/charmbracelet/crush/internal/message"
    "github.com/charmbracelet/crush/internal/ui/styles"
)

const smithersMCPPrefix = "mcp_smithers_"

// IsSmithersToolCall returns true if the tool name is a Smithers MCP tool.
// The server name "smithers" matches the default SmithersMCPServer template var.
// This also catches aliased server names that start with "smithers" (e.g. "smithers_dev").
func IsSmithersToolCall(name string) bool {
    return strings.HasPrefix(name, smithersMCPPrefix)
}

// smithersToolName strips the MCP prefix and returns the bare tool name.
// e.g. "mcp_smithers_runs_list" → "runs_list"
func smithersToolName(name string) string {
    return strings.TrimPrefix(name, smithersMCPPrefix)
}

// SmithersToolMessageItem wraps baseToolMessageItem for Smithers MCP tools.
type SmithersToolMessageItem struct {
    *baseToolMessageItem
}

var _ ToolMessageItem = (*SmithersToolMessageItem)(nil)

// NewSmithersToolMessageItem creates a renderer for a Smithers MCP tool call.
func NewSmithersToolMessageItem(
    sty *styles.Styles,
    toolCall message.ToolCall,
    result *message.ToolResult,
    canceled bool,
) ToolMessageItem {
    return newBaseToolMessageItem(sty, toolCall, result, &SmithersToolRenderContext{}, canceled)
}
```

#### 2b. RenderTool Dispatch

```go
// SmithersToolRenderContext renders Smithers MCP tool calls.
type SmithersToolRenderContext struct{}

// RenderTool implements ToolRenderer.
func (s *SmithersToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
    cappedWidth := cappedMessageWidth(width)
    tool := smithersToolName(opts.ToolCall.Name)
    displayName := s.formatToolName(sty, tool)

    if opts.IsPending() {
        return pendingTool(sty, displayName, opts.Anim, opts.Compact)
    }

    header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, s.mainParam(opts, tool))
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
```

#### 2c. Tool Name Formatting

```go
// formatToolName returns "Smithers → <Action>" styled for the tool header.
func (s *SmithersToolRenderContext) formatToolName(sty *styles.Styles, tool string) string {
    action := s.humanizeTool(tool)
    serverPart := sty.Tool.Smithers.ServerName.Render("Smithers")
    arrow := sty.Tool.MCPArrow.String()
    actionPart := sty.Tool.MCPToolName.Render(action)
    return fmt.Sprintf("%s %s %s", serverPart, arrow, actionPart)
}

// humanizeTool converts a snake_case tool name to human-readable form.
// Uses a lookup table for well-known tools; falls back to generic conversion.
var smithersToolLabels = map[string]string{
    "runs_list":    "Runs List",
    "inspect":      "Inspect",
    "chat":         "Chat",
    "logs":         "Logs",
    "approve":      "Approve",
    "deny":         "Deny",
    "hijack":       "Hijack",
    "cancel":       "Cancel",
    "workflow_up":  "Start Workflow",
    "workflow_list":"Workflow List",
    "workflow_run": "Run Workflow",
    "diff":         "Diff",
    "fork":         "Fork",
    "replay":       "Replay",
    "revert":       "Revert",
    "memory_list":  "Memory List",
    "memory_recall":"Memory Recall",
    "scores":       "Scores",
    "cron_list":    "Cron List",
    "sql":          "SQL",
}

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
```

#### 2d. Main Parameter Extraction

```go
// mainParam extracts the most informative single parameter to show in the header.
func (s *SmithersToolRenderContext) mainParam(opts *ToolRenderOpts, tool string) string {
    var params map[string]any
    if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
        return ""
    }
    // Per-tool primary parameter key
    primaryKeys := map[string]string{
        "inspect":      "runId",
        "chat":         "runId",
        "logs":         "runId",
        "approve":      "runId",
        "deny":         "runId",
        "hijack":       "runId",
        "cancel":       "runId",
        "workflow_up":  "workflow",
        "workflow_run": "workflow",
        "diff":         "runId",
        "fork":         "runId",
        "replay":       "runId",
        "revert":       "runId",
        "memory_recall":"query",
        "sql":          "query",
        "scores":       "runId",
    }
    if key, ok := primaryKeys[tool]; ok {
        if val, ok := params[key]; ok {
            if s, ok := val.(string); ok {
                return s
            }
        }
    }
    return ""
}
```

#### 2e. Body Renderer Dispatch

```go
// renderBody dispatches to a per-tool body renderer.
func (s *SmithersToolRenderContext) renderBody(
    sty *styles.Styles, tool string, opts *ToolRenderOpts, bodyWidth int,
) string {
    switch tool {
    // --- Tables ---
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

    // --- Cards ---
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
        return s.renderActionCard(sty, opts, bodyWidth, "DONE", sty.Tool.Smithers.CardStarted)

    // --- Tree ---
    case "inspect":
        return s.renderInspectTree(sty, opts, bodyWidth)
    case "diff":
        return s.renderDiffTree(sty, opts, bodyWidth)

    // --- Plain text (logs and chat are prose) ---
    case "chat", "logs":
        return sty.Tool.Body.Render(
            toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))

    // --- Fallback: pretty JSON or plain text ---
    default:
        return s.renderFallback(sty, opts, bodyWidth)
    }
}
```

### Step 3: Per-Tool Renderer Implementations

All per-tool renderers follow one of three patterns. The scaffolding provides a working implementation for each pattern and a stub + fallback for tools whose JSON shapes are not yet confirmed.

#### 3a. Table Renderer Pattern (runs_list, workflow_list, sql, etc.)

Uses `lipgloss/v2/table` (already imported via `docker_mcp.go`). Reference implementation for `runs_list`:

```go
// RunEntry is the expected shape of a single run in the runs_list result.
type RunEntry struct {
    ID       string `json:"id"`
    Workflow string `json:"workflow"`
    Status   string `json:"status"`
    Step     string `json:"step"`     // e.g. "3/5"
    Elapsed  string `json:"elapsed"`  // e.g. "2m14s"
}

func (s *SmithersToolRenderContext) renderRunsTable(
    sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
    var runs []RunEntry
    // Try array at top level first, then look for a "data" envelope.
    if err := json.Unmarshal([]byte(opts.Result.Content), &runs); err != nil {
        var envelope struct{ Data []RunEntry `json:"data"` }
        if err2 := json.Unmarshal([]byte(opts.Result.Content), &envelope); err2 != nil {
            return s.renderFallback(sty, opts, width)
        }
        runs = envelope.Data
    }
    if len(runs) == 0 {
        return sty.Tool.Body.Render(sty.Subtle.Render("No runs found."))
    }

    headers := []string{"ID", "Workflow", "Status", "Step", "Time"}
    rows := make([][]string, 0, len(runs))
    shown := runs
    extra := ""
    if len(runs) > 15 {
        shown = runs[:15]
        extra = sty.Subtle.Render(fmt.Sprintf("… and %d more", len(runs)-15))
    }
    for _, r := range shown {
        statusStyled := s.styleStatus(sty, r.Status, r.Status)
        rows = append(rows, []string{
            sty.Base.Render(r.ID),
            sty.Base.Render(r.Workflow),
            statusStyled,
            sty.Subtle.Render(r.Step),
            sty.Subtle.Render(r.Elapsed),
        })
    }

    t := table.New().
        Wrap(false).
        BorderTop(false).BorderBottom(false).
        BorderRight(false).BorderLeft(false).
        BorderColumn(false).BorderRow(false).
        Headers(headers...).
        StyleFunc(func(row, col int) lipgloss.Style {
            if row == table.HeaderRow {
                return sty.Tool.Smithers.TableHeader
            }
            return lipgloss.NewStyle().PaddingRight(2)
        }).
        Rows(rows...).
        Width(width)

    out := t.Render()
    if extra != "" {
        out += "\n" + extra
    }
    return sty.Tool.Body.Render(out)
}

// styleStatus returns a status string styled with the appropriate color.
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
```

The `renderSQLTable` implementation follows the same pattern but parses `{ "columns": [...], "rows": [[...]] }` or an array of row-objects with dynamic column detection.

#### 3b. Card Renderer Pattern (approve, deny, cancel, workflow_up, fork, replay, revert)

Action cards show a compact confirmation with a colored badge. Generic implementation handles all action tools:

```go
// ActionConfirmation is the expected shape of an action tool result.
type ActionConfirmation struct {
    Success  bool   `json:"success"`
    RunID    string `json:"runId"`
    GateID   string `json:"gateId,omitempty"`
    Message  string `json:"message,omitempty"`
}

func (s *SmithersToolRenderContext) renderActionCard(
    sty *styles.Styles, opts *ToolRenderOpts, width int,
    badge string, badgeStyle lipgloss.Style,
) string {
    var conf ActionConfirmation
    if err := json.Unmarshal([]byte(opts.Result.Content), &conf); err != nil || !conf.Success {
        // Graceful fallback: show plain content
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
```

The `renderHijackCard` variant adds a note about the native TUI handoff that will follow.

#### 3c. Tree Renderer Pattern (inspect, diff)

Uses `lipgloss/v2/tree` (already imported in `tools.go`) to render the run's DAG node hierarchy:

```go
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

func (s *SmithersToolRenderContext) renderInspectTree(
    sty *styles.Styles, opts *ToolRenderOpts, width int,
) string {
    var result InspectResult
    if err := json.Unmarshal([]byte(opts.Result.Content), &result); err != nil {
        return s.renderFallback(sty, opts, width)
    }

    root := tree.Root(
        fmt.Sprintf("%s  %s",
            s.styleStatus(sty, result.Status, "●"),
            sty.Base.Render(result.RunID+" / "+result.Workflow),
        ),
    )

    for _, node := range result.Nodes {
        root.Child(s.buildNodeTree(sty, node))
    }

    return sty.Tool.Body.Render(root.String())
}

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
```

#### 3d. Fallback Renderer

All renderers call `s.renderFallback` on parse failure. This mirrors the existing `MCPToolRenderContext` behavior:

```go
// renderFallback renders the result as pretty JSON, markdown, or plain text.
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
```

### Step 4: Wire Into NewToolMessageItem

**File**: `internal/ui/chat/tools.go`

In the `default` branch of `NewToolMessageItem`, insert `IsSmithersToolCall` before the generic `mcp_` check:

```go
default:
    if IsDockerMCPTool(toolCall.Name) {
        item = NewDockerMCPToolMessageItem(sty, toolCall, result, canceled)
    } else if IsSmithersToolCall(toolCall.Name) {          // NEW
        item = NewSmithersToolMessageItem(sty, toolCall, result, canceled)  // NEW
    } else if strings.HasPrefix(toolCall.Name, "mcp_") {
        item = NewMCPToolMessageItem(sty, toolCall, result, canceled)
    } else {
        item = NewGenericToolMessageItem(sty, toolCall, result, canceled)
    }
```

This is the only change to `tools.go`.

---

## File Plan

| File | Change |
|------|--------|
| `internal/ui/chat/smithers_mcp.go` | **New** — `IsSmithersToolCall`, `SmithersToolMessageItem`, `SmithersToolRenderContext`, all per-tool renderers |
| `internal/ui/styles/smithers_styles.go` | **New** — `SmithersStyles` struct definition |
| `internal/ui/styles/styles.go` | **Modify** — add `Smithers SmithersStyles` field to `Tool` struct; populate in `New()` |
| `internal/ui/chat/tools.go` | **Modify** — add `IsSmithersToolCall` check in `NewToolMessageItem` default branch (3-line change) |

---

## Per-Renderer Implementation Scope

This table defines what is built vs. stubbed. "Full" means working parser + styled output. "Stub" means the fallback renders, with a comment marking where the real parser goes when the API shape is confirmed by `feat-mcp-tool-discovery`.

| Tool(s) | Format | Implementation | Notes |
|---------|--------|---------------|-------|
| `runs_list` | Table | Full | Primary use case; full implementation per design mockup |
| `workflow_list` | Table | Full | Simple 3-column table |
| `cron_list` | Table | Full | 4-column table |
| `scores` | Table | Full | Metric/value table |
| `memory_list` | Table | Full | Key/value/runID table |
| `memory_recall` | Table | Full | Relevance/key/value table |
| `sql` | Table | Full | Dynamic columns from JSON; handles both `{columns, rows}` and `[{...}]` shapes |
| `approve` | Card | Full | Green "APPROVED" badge + runID + gateID |
| `deny` | Card | Full | Red "DENIED" badge + runID + gateID |
| `cancel` | Card | Full | Subtle "CANCELED" badge + runID |
| `hijack` | Card | Full | Blue badge + handoff instructions line |
| `workflow_up` / `workflow_run` | Card | Full | "STARTED" badge + workflow name + new runID |
| `fork` / `replay` / `revert` | Card | Full | "DONE" badge + new runID |
| `inspect` | Tree | Full | Lipgloss tree with per-node status icons |
| `diff` | Tree | Stub | Placeholder pending confirmed diff JSON shape |
| `chat` | Plain | Full | Pass-through to `toolOutputPlainContent` |
| `logs` | Plain | Full | Pass-through to `toolOutputPlainContent` |
| Unknown `mcp_smithers_*` | Fallback | Full | Pretty JSON → markdown → plain text chain |

---

## Testing Strategy

### Unit Tests

**File**: `internal/ui/chat/smithers_mcp_test.go` (new)

Test coverage required:

**`IsSmithersToolCall`**
- `"mcp_smithers_runs_list"` → `true`
- `"mcp_smithers_inspect"` → `true`
- `"mcp_docker-desktop_mcp-find"` → `false`
- `"mcp_anthropic_tool"` → `false`
- `"bash"` → `false`
- `""` → `false`

**`SmithersToolRenderContext.RenderTool` — all states**

For each tool category, test three states:
1. **Pending** (no result yet): verify the spinner/pending line renders without panicking.
2. **Success with valid JSON**: verify the expected render format is used (table has expected column count, card has badge, tree has root).
3. **Success with invalid JSON**: verify `renderFallback` is called and no panic.
4. **Error result** (`result.IsError = true`): verify `toolEarlyStateContent` renders an error.
5. **Compact mode**: verify only the header line is returned (no body).

Test helpers to create `ToolRenderOpts` for each scenario:

```go
func makeOpts(name, input, result string, status ToolStatus) *ToolRenderOpts {
    return &ToolRenderOpts{
        ToolCall: message.ToolCall{Name: name, Input: input, Finished: true},
        Result:   &message.ToolResult{Content: result},
        Status:   status,
    }
}
```

**`styleStatus`**
- All six status strings map to the correct style (verify non-empty render).
- Unknown status maps to `sty.Subtle`.

**`humanizeTool`**
- All 19 known tool names return their human-readable label.
- Unknown tool name falls back to title-cased conversion.

**`renderSQLTable` — dynamic columns**
- Parse `{"columns": ["a","b"], "rows": [[1,2],[3,4]]}` → 2-column table.
- Parse `[{"name":"foo","val":1}]` → table with detected columns.
- Empty result → "No results." subtle text.

### Integration Test

**File**: `internal/e2e/smithers_mcp_renderer_test.go` (new)

Test scenario:
1. Launch TUI with a mock MCP server that returns canned Smithers tool responses.
2. Send a chat message that triggers `mcp_smithers_runs_list`.
3. Wait for the tool result to appear.
4. Strip ANSI, assert the output contains column headers "ID", "Workflow", "Status".
5. Repeat for `mcp_smithers_approve` — assert "APPROVED" appears in output.
6. Repeat for `mcp_smithers_inspect` — assert tree indentation appears.

This test depends on the mock MCP server infrastructure. If that infrastructure is not yet available, mark the test `t.Skip("requires mock MCP server")` and file a follow-up.

### VHS Recording

**File**: `tests/vhs/smithers-mcp-renderers.tape` (new)

Record a session that:
1. Opens the TUI.
2. Types "list all runs" and sends.
3. Shows the `mcp_smithers_runs_list` tool call rendering (spinner → table).
4. Types "approve the pending gate on run abc123" and sends.
5. Shows the `mcp_smithers_approve` card rendering.

The tape is for visual review during PR, not CI.

### Manual Verification Checklist

1. `go build ./...` — no compilation errors.
2. With Smithers MCP server connected:
   - Ask "what runs are active?" — verify table renders with colored status column.
   - Ask "approve the gate on run X" — verify green APPROVED card.
   - Ask "deny the gate on run X" — verify red DENIED card.
   - Ask "inspect run X" — verify tree renders with node names and status icons.
   - Ask a SQL query — verify dynamic table renders.
3. With Smithers MCP server disconnected (tools error out):
   - Verify error state renders via `toolEarlyStateContent` (not a panic).
4. In compact mode (many tool calls collapsed):
   - Verify Smithers tools show one-line header `● Smithers → Runs List  <runId>`.
5. Resize terminal to < 80 columns — verify table truncates, no layout overflow.

---

## Open Questions

1. **Server name configurability**: The system prompt uses `{{.SmithersMCPServer}}` to allow a non-default server name. The renderer hardcodes `mcp_smithers_`. Should `IsSmithersToolCall` be config-driven (accepting the server name as a parameter passed at construction time)? For now: hardcode `smithers`; add config support as a follow-up when the template override is actually used.

2. **`runs_list` response envelope vs. bare array**: Does the MCP tool return `[{...}, ...]` directly or `{ "data": [...] }`? The implementation tries both. Confirm with `feat-mcp-tool-discovery`.

3. **`diff` result shape**: The `diff` renderer is stubbed. What does the Smithers diff response look like? Is it a JSON patch, a map of `{before, after}`, or something else? File a follow-up to implement the diff tree once shape is confirmed.

4. **`lipgloss/v2/tree` for inspect**: The `tree` package is already imported in `tools.go`. Confirm it renders correctly at narrow widths (< 60 cols) without overflow before merging.

5. **`sql` empty result handling**: Should an empty SQL result set render "No rows returned." or simply nothing? Prefer explicit "No rows returned." for clarity.

6. **Expand/collapse for tables**: `ToolRenderOpts.ExpandedContent` is used by `toolOutputPlainContent` and `toolOutputCodeContent` to show all lines. Should tables respect this flag (show all rows when expanded, cap at 15 when not)? Recommended: yes, consistent with other renderers.
