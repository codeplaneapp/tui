# Smithers TUI — Engineering Document

**Status**: Draft
**Date**: 2026-04-02

---

## 1. Fork Strategy

### 1.1 Approach: Hard Fork

We hard-fork Crush (Go, Bubble Tea v2) and transform it into Smithers TUI.
This is preferred over a soft fork because:

- **Divergent product direction**: Smithers TUI is not a general-purpose AI
  chat; it's a workflow control plane.
- **Custom views**: Run dashboard, timeline, approval queue have no analog
  in Crush.
- **Branding**: Full rebrand required (logo, colors, config paths).
- **Tool system**: Different default tools (Smithers MCP primary, not
  general coding tools).

We maintain the ability to cherry-pick upstream Crush improvements
(Bubble Tea upgrades, rendering fixes, etc.) by keeping a clean initial
fork commit.

### 1.2 Repository Setup

```bash
# Fork
git clone https://github.com/charmbracelet/crush smithers-tui
cd smithers-tui

# Rename Go module
go mod edit -module github.com/anthropic/smithers-tui
find . -type f -name '*.go' -exec sed -i '' \
  's|github.com/charmbracelet/crush|github.com/anthropic/smithers-tui|g' {} +

# Keep upstream remote for cherry-picks
git remote add upstream https://github.com/charmbracelet/crush
```

---

## 2. Architecture Overview

### 2.1 Existing Crush Architecture (What We Inherit)

```
main.go
  └─ internal/cmd/root.go          (Cobra CLI entry)
      └─ internal/app/app.go       (Service orchestration)
          ├─ internal/config/       (Config loading: crush.json)
          ├─ internal/db/           (SQLite sessions via sqlc)
          ├─ internal/agent/        (Agent loop, tools, templates)
          │   ├─ coordinator.go     (Large/small model coordination)
          │   ├─ session.go         (Session agent loop)
          │   ├─ tools/             (57+ tool implementations)
          │   └─ templates/         (System prompt templates)
          ├─ internal/mcp/          (MCP client: stdio, http, sse)
          ├─ internal/lsp/          (Language server integration)
          ├─ internal/pubsub/       (Event broker)
          └─ internal/ui/           (Bubble Tea TUI)
              ├─ model/ui.go        (Root Bubble Tea model)
              ├─ chat/              (Chat rendering, tool renderers)
              ├─ styles/            (Lip Gloss styles)
              ├─ logo/              (ASCII art header)
              └─ ...                (Completions, dialogs, etc.)
```

### 2.2 Design Principle: Thin Frontend

The Go TUI contains **zero Smithers business logic**. It is a presentation
layer that talks to the Smithers CLI (TypeScript) via three channels:

1. **HTTP API** — `smithers up --serve` exposes REST + SSE. The TUI
   consumes the same endpoints the GUI used (`/ps`, `/sql`,
   `/workflow/run/{id}`, `/agent/chat/{id}`, `/ticket/list`, etc.).
2. **MCP Server** — `smithers mcp-serve` (stdio). Used by the chat
   agent for tool calls.
3. **Shell-out** — `exec.Command("smithers", ...)` as fallback for
   one-shot operations when no server is running.

This mirrors the GUI's transport layer (`gui/src/ui/api/transport.ts`)
which called the same Hono-based HTTP API from SolidJS.

### 2.3 New Smithers Architecture (What We Add)

```
internal/
  ├─ smithers/                      NEW — Thin HTTP/exec client (NO business logic)
  │   ├─ client.go                  HTTP client for Smithers server API
  │   ├─ types.go                   Run, Node, Attempt, Approval, Agent, etc.
  │   ├─ events.go                  SSE event stream consumer
  │   └─ exec.go                    Shell-out fallback (exec smithers CLI)
  │
  ├─ ui/
  │   ├─ views/                     NEW — View system (mirrors GUI's 8 tabs)
  │   │   ├─ router.go              View stack manager
  │   │   │
  │   │   │  ── Workspace (GUI parity) ──
  │   │   ├─ runs.go                Run Dashboard (→ RunsList.tsx)
  │   │   ├─ runinspect.go          Run Inspector + DAG (→ NodeInspector.tsx)
  │   │   ├─ tasktabs.go            Node detail tabs (→ TaskTabs.tsx)
  │   │   ├─ workflows.go           Workflow List + Executor (→ WorkflowsList.tsx)
  │   │   ├─ agents.go              Agent Browser (→ AgentsList.tsx)
  │   │   ├─ agentchat.go           Agent Chat (→ AgentChat.tsx)
  │   │   ├─ prompts.go             Prompt Editor + Preview (→ PromptsList.tsx)
  │   │   ├─ tickets.go             Ticket Manager (→ TicketsList.tsx)
  │   │   │
  │   │   │  ── Systems (GUI parity) ──
  │   │   ├─ sqlbrowser.go          SQL Browser (→ SqlBrowser.tsx)
  │   │   ├─ triggers.go            Trigger/Cron Manager (→ TriggersList.tsx)
  │   │   │
  │   │   │  ── TUI-only (beyond GUI) ──
  │   │   ├─ livechat.go            Live Chat Viewer (streaming)
  │   │   ├─ hijack.go              Hijack Mode
  │   │   ├─ approvals.go           Approval Queue
  │   │   ├─ timeline.go            Time-Travel Timeline
  │   │   ├─ scores.go              Scores / ROI
  │   │   └─ memory.go              Memory Browser
  │   │
  │   ├─ components/                NEW — Reusable UI components
  │   │   ├─ runtable.go            Run list table
  │   │   ├─ progressbar.go         Node progress bar
  │   │   ├─ dagview.go             DAG visualization
  │   │   ├─ approvalcard.go        Approval gate card
  │   │   ├─ notification.go        Toast notification overlay
  │   │   ├─ splitpane.go           Two-pane layout (list + detail)
  │   │   ├─ dynamicform.go         Dynamic input form (string/number/bool/obj/arr)
  │   │   ├─ jsontree.go            JSON viewer with folding
  │   │   └─ timeline.go            Timeline graph
  │   │
  │   ├─ chat/
  │   │   ├─ smithers_*.go          NEW — Smithers-specific tool renderers
  │   │   └─ (existing renderers)   KEEP — read, edit, bash, etc.
  │   │
  │   ├─ model/
  │   │   ├─ ui.go                  MODIFY — Add view routing
  │   │   └─ keys.go                MODIFY — Add Smithers keybindings
  │   │
  │   ├─ styles/
  │   │   └─ styles.go              MODIFY — Smithers color scheme
  │   │
  │   └─ logo/
  │       └─ logo.go                REPLACE — Smithers ASCII art
  │
  ├─ agent/
  │   └─ templates/
  │       └─ smithers.md.tpl        NEW — Smithers system prompt
  │
  └─ config/
      └─ config.go                  MODIFY — Add smithers config section
```

---

## 3. Implementation Plan

### Phase 0 — Foundation (Week 1-2)

#### 3.0.1 Fork and Rebrand

**Files to modify**:

| File | Change |
|------|--------|
| `go.mod` | Rename module |
| `main.go` | Update binary name |
| `internal/cmd/root.go` | Change app name, description, version string |
| `internal/ui/logo/logo.go` | Replace Crush ASCII art with Smithers logo |
| `internal/ui/styles/styles.go` | Apply Smithers color scheme |
| `internal/config/config.go` | Change config dir (`.smithers-tui/`), file names |
| `internal/config/defaults.go` | Change default config values |
| All `*.go` imports | `s/crush/smithers-tui/g` |

#### 3.0.2 Smithers MCP Server

The fastest path to full Smithers integration is an MCP server that wraps
the `smithers` CLI. This lives in the Smithers repo (not in the TUI fork)
and exposes all Smithers commands as MCP tools.

**MCP Server Design** (TypeScript, lives in `smithers/src/mcp-server/`):

```typescript
// Tool: smithers_ps
// Lists all runs with status
{
  name: "smithers_ps",
  description: "List active, paused, and completed Smithers runs",
  inputSchema: {
    type: "object",
    properties: {
      status: { type: "string", enum: ["active", "paused", "completed", "failed", "all"] },
      limit: { type: "number", default: 20 }
    }
  }
}

// Tool: smithers_chat
// Get agent chat output for a run
{
  name: "smithers_chat",
  description: "View agent chat output for a specific run",
  inputSchema: {
    type: "object",
    properties: {
      runId: { type: "string" },
      follow: { type: "boolean" },
      tail: { type: "number" }
    },
    required: ["runId"]
  }
}

// Tool: smithers_approve / smithers_deny
// Tool: smithers_inspect
// Tool: smithers_logs
// Tool: smithers_hijack
// Tool: smithers_cancel
// Tool: smithers_up
// Tool: smithers_diff
// Tool: smithers_fork
// Tool: smithers_replay
// Tool: smithers_timeline
// Tool: smithers_workflow_list
// Tool: smithers_workflow_run
// Tool: smithers_memory_list
// Tool: smithers_memory_recall
// Tool: smithers_scores
// Tool: smithers_cron_list
// Tool: smithers_sql
// ... (full CLI surface)
```

**TUI-side config** (default `smithers-tui.json`):

```jsonc
{
  "mcpServers": {
    "smithers": {
      "type": "stdio",
      "command": "smithers",
      "args": ["mcp-serve"],
      "env": {}
    }
  }
}
```

Crush's existing MCP client handles stdio transport, tool discovery, and
tool execution. No new client code needed.

#### 3.0.3 Smithers System Prompt

Create `internal/agent/templates/smithers.md.tpl`:

```markdown
You are the Smithers TUI assistant, a specialized agent for managing
Smithers AI workflows.

## Your Role
You help users monitor, control, and debug Smithers workflow runs from
within this terminal interface.

## Available Smithers Tools
You have access to the full Smithers CLI via MCP tools:
- smithers_ps: List runs
- smithers_inspect: Detailed run state
- smithers_chat: View agent conversations
- smithers_logs: Event logs
- smithers_approve / smithers_deny: Gate management
- smithers_hijack: Take over agent sessions
- smithers_cancel: Stop runs
- smithers_up: Start workflows
- smithers_diff / smithers_fork / smithers_replay: Time-travel
- smithers_workflow_list / smithers_workflow_run: Workflow management
- smithers_memory_list / smithers_memory_recall: Cross-run memory
- smithers_scores: Evaluation metrics
- smithers_cron_list: Schedule management
- smithers_sql: Direct database queries

## Behavior Guidelines
- When listing runs, format as tables with status indicators.
- Proactively mention pending approval gates.
- When a run fails, suggest inspection and common fixes.
- For hijacking, confirm with the user before taking over.
- Use tool results to provide context-aware answers.

## Context
{{- if .WorkflowDir }}
Workflow directory: {{ .WorkflowDir }}
{{- end }}
{{- if .ActiveRuns }}
Active runs: {{ .ActiveRuns }}
{{- end }}
```

#### 3.0.4 Default Tool Configuration

Modify the default tool set. Keep standard tools, add Smithers MCP as primary:

```go
// internal/config/defaults.go
var DefaultTools = ToolConfig{
    // Smithers tools (via MCP) — primary
    // These are auto-discovered from the smithers MCP server

    // Built-in tools — keep for general use
    Enabled: []string{
        "view",       // Read files
        "edit",       // Edit files
        "write",      // Write files
        "bash",       // Shell commands
        "glob",       // File search
        "grep",       // Content search
        "ls",         // Directory listing
        "fetch",      // HTTP fetch
        "diagnostics", // LSP diagnostics
    },
    // Disable tools not relevant for Smithers context
    Disabled: []string{
        "sourcegraph",  // Not needed
        "multiedit",    // Overkill for this context
    },
}
```

---

### Phase 1 — Chat + Run Dashboard (Week 3-4)

#### 3.1.1 View Router

The core architectural change: add a view stack system on top of Crush's
single-model TUI.

**`internal/ui/views/router.go`**:

```go
package views

import (
    tea "github.com/charmbracelet/bubbletea/v2"
)

// View represents a named TUI view
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Name() string
    // ShortHelp returns keybinding hints for the help bar
    ShortHelp() []string
}

// Router manages a stack of views
type Router struct {
    stack []View
    chat  View // always at bottom, never popped
}

func NewRouter(chat View) *Router {
    return &Router{
        stack: []View{chat},
        chat:  chat,
    }
}

func (r *Router) Push(v View) tea.Cmd {
    r.stack = append(r.stack, v)
    return v.Init()
}

func (r *Router) Pop() {
    if len(r.stack) > 1 {
        r.stack = r.stack[:len(r.stack)-1]
    }
}

func (r *Router) Current() View {
    return r.stack[len(r.stack)-1]
}

func (r *Router) IsChat() bool {
    return len(r.stack) == 1
}
```

**Integration with `internal/ui/model/ui.go`**:

The existing `UI` model wraps the chat as a sub-component. We modify it
to route through the `Router`:

```go
// In UI.Update():
case tea.KeyMsg:
    switch {
    case key.Matches(msg, keys.RunDashboard): // ctrl+r
        cmd := m.router.Push(runs.New(m.smithersClient))
        return m, cmd
    case key.Matches(msg, keys.Approvals): // ctrl+a
        cmd := m.router.Push(approvals.New(m.smithersClient))
        return m, cmd
    case key.Matches(msg, keys.Back): // esc (when not in chat)
        if !m.router.IsChat() {
            m.router.Pop()
            return m, nil
        }
    }

// In UI.View():
// Delegate to current view
return m.router.Current().View()
```

#### 3.1.2 Run Dashboard View

**`internal/ui/views/runs.go`**:

- Fetches run list via Smithers client (HTTP API or direct DB).
- Renders table with status indicators, progress bars, elapsed time.
- Subscribes to SSE events for real-time updates.
- Handles keybindings: Enter (inspect), c (chat), h (hijack), a (approve), x (cancel).

**Data flow**:

```
Smithers API/DB  →  SmithersClient.ListRuns()  →  RunsView  →  Table render
     │
     └─── SSE events  →  SmithersClient.StreamEvents()  →  Update run state
```

#### 3.1.3 Smithers Client

**`internal/smithers/client.go`**:

```go
package smithers

// Client provides access to Smithers data.
// Supports two modes:
// 1. HTTP API (when smithers server is running)
// 2. Direct SQLite (read-only, for local workflows)
type Client struct {
    apiURL   string
    apiToken string
    dbPath   string
    db       *sql.DB  // direct SQLite access (fallback)
}

func (c *Client) ListRuns(ctx context.Context, filter RunFilter) ([]Run, error)
func (c *Client) GetRun(ctx context.Context, id string) (*Run, error)
func (c *Client) InspectRun(ctx context.Context, id string) (*RunDetail, error)
func (c *Client) StreamEvents(ctx context.Context, afterSeq int) (<-chan Event, error)
func (c *Client) Approve(ctx context.Context, runID, nodeID string) error
func (c *Client) Deny(ctx context.Context, runID, nodeID string) error
func (c *Client) Cancel(ctx context.Context, runID string) error
func (c *Client) ListPendingApprovals(ctx context.Context) ([]Approval, error)
func (c *Client) GetChatOutput(ctx context.Context, runID string) ([]ChatBlock, error)
func (c *Client) StreamChat(ctx context.Context, runID string) (<-chan ChatBlock, error)
func (c *Client) ListSnapshots(ctx context.Context, runID string) ([]Snapshot, error)
func (c *Client) DiffSnapshots(ctx context.Context, runID string, from, to int) (*Diff, error)
func (c *Client) ForkRun(ctx context.Context, runID string, snapshotNo int) (string, error)
func (c *Client) ReplayRun(ctx context.Context, runID string, snapshotNo int) (string, error)
func (c *Client) ListWorkflows(ctx context.Context) ([]Workflow, error)
func (c *Client) RunWorkflow(ctx context.Context, id string, inputs map[string]string) (string, error)
func (c *Client) GetScores(ctx context.Context, runID string) ([]Score, error)
func (c *Client) ListMemoryFacts(ctx context.Context) ([]MemoryFact, error)
func (c *Client) RecallMemory(ctx context.Context, query string) ([]MemoryFact, error)
func (c *Client) HijackRun(ctx context.Context, runID string) (*HijackSession, error)
```

**Dual-mode access**:
- If `apiURL` is set and reachable → use HTTP API (preferred).
- If Smithers DB exists locally → read directly via SQLite (read-only ops only).
- Mutations (approve, cancel, hijack) always go through HTTP API.

#### 3.1.4 Status Bar Enhancement

Modify the header/status area to show Smithers run status:

```go
// internal/ui/model/header.go (or equivalent)
func (m *UI) renderSmithersStatus() string {
    runs := m.smithersClient.CachedRunSummary()
    parts := []string{}
    if runs.Active > 0 {
        parts = append(parts, fmt.Sprintf("%d active", runs.Active))
    }
    if runs.PendingApprovals > 0 {
        parts = append(parts, fmt.Sprintf("⚠ %d pending approval", runs.PendingApprovals))
    }
    if len(parts) == 0 {
        return "No active runs"
    }
    return strings.Join(parts, " · ")
}
```

---

### Phase 2 — Live Chat + Hijack (Week 5-6)

#### 3.2.1 Live Chat Viewer

**`internal/ui/views/livechat.go`**:

This view streams agent output for a specific run and renders it using
Crush's existing tool renderers. The key insight is that Smithers agent
output (tool calls, file reads, edits, bash commands) maps directly to
Crush's tool renderer types.

**Data mapping**:

```
Smithers ChatAttempt        →    Crush message model
  prompt                    →    User role message
  responseText              →    Assistant role message
  tool calls (from NDJSON)  →    ToolCall content parts
  tool results              →    ToolResult content parts
```

**Implementation**:

```go
type LiveChatView struct {
    runID      string
    client     *smithers.Client
    messages   []chat.Message   // Reuse Crush's message types
    following  bool             // Auto-scroll
    viewport   viewport.Model
    cancelFn   context.CancelFunc
}

func (v *LiveChatView) Init() tea.Cmd {
    // Start streaming chat events
    return v.streamChat()
}

func (v *LiveChatView) streamChat() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithCancel(context.Background())
        v.cancelFn = cancel
        ch, _ := v.client.StreamChat(ctx, v.runID)
        // Convert to Crush message format and send as tea.Msg
        // ...
    }
}
```

#### 3.2.2 Chat Hijacking

**`internal/smithers/hijack.go`**:

Hijacking requires coordinating between:
1. Smithers (pausing the automated agent)
2. The TUI (switching from viewer to interactive mode)
3. The underlying agent CLI (resuming the session)

**Flow**:

```
User presses 'h' in LiveChatView
    │
    ▼
TUI calls Client.HijackRun(runID)
    │
    ▼
Smithers HTTP API receives hijack request
    │
    ├─ Pauses running agent
    ├─ Captures session metadata (resume token, messages, cwd)
    └─ Returns HijackSession{engine, mode, resume, messages, cwd}
    │
    ▼
TUI receives HijackSession
    │
    ├─ If mode == "native-cli":
    │     Launch agent CLI with --resume flag
    │     Attach agent's stdin/stdout to TUI
    │     User types directly to agent
    │
    └─ If mode == "conversation":
          Switch to interactive chat mode
          Pre-populate context with message history
          User sends messages through Smithers agent
          Agent relays to underlying engine
```

**`internal/ui/views/hijack.go`**:

```go
type HijackView struct {
    runID    string
    session  *smithers.HijackSession
    // Embeds the chat component for interactive messaging
    chat     *chat.Chat
    // Banner state
    driving  bool
}

func (v *HijackView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Handle /resume command
        if v.isResumeCommand(msg) {
            return v, v.resume()
        }
    }
    // Delegate to embedded chat
    return v, v.chat.Update(msg)
}
```

**Native CLI hijack** (preferred for claude-code, codex):

When the agent supports `--resume`, we spawn the CLI as a subprocess
and attach it to a pseudo-terminal. The TUI renders the PTY output
and forwards input. This is similar to how Crush handles the bash tool
but with the agent CLI instead.

```go
// Spawn claude-code --resume <session-id>
cmd := exec.Command("claude-code", "--resume", session.Resume)
cmd.Dir = session.CWD
pty, _ := pty.Start(cmd)
// Wire PTY to TUI viewport
```

**Conversation hijack** (fallback):

When native resume isn't available, we replay the message history
into a new agent session and let the user continue the conversation.
The TUI's existing chat component handles this — we just pre-seed
the message history.

#### 3.2.3 Approval Management

**`internal/ui/views/approvals.go`**:

- Lists pending approvals with context.
- Inline approve/deny with confirmation.
- Shows recent decisions.

**Notification integration**:

```go
// internal/ui/components/notification.go
type Notification struct {
    Title   string
    Body    string
    Actions []Action  // e.g., [Approve, Deny, View]
    TTL     time.Duration
}

// Rendered as overlay in UI.View()
func (m *UI) renderNotifications() string {
    // Position at bottom-right of terminal
    // Show latest notification with action hints
}
```

Notifications are triggered by SSE events from Smithers:
- `ApprovalRequested` → Show approval notification
- `RunFailed` → Show failure notification
- `RunCompleted` → Show completion notification (brief)

---

### Phase 3 — Time-Travel (Week 7-8)

#### 3.3.1 Timeline View

**`internal/ui/views/timeline.go`**:

```go
type TimelineView struct {
    runID     string
    snapshots []smithers.Snapshot
    cursor    int  // Currently selected snapshot
    diff      *smithers.Diff  // Diff between cursor and cursor-1
}
```

**Rendering**: Uses Lip Gloss to draw the timeline graph:

```
①──②──③──④──⑤──⑥──⑦
               ▲
            selected
```

Selected snapshot shows details below. Left/right moves cursor.
`d` shows diff, `f` forks, `r` replays.

#### 3.3.2 DAG Visualization

**`internal/ui/components/dagview.go`**:

Renders the workflow DAG as ASCII art using a simple left-to-right layout:

```go
func RenderDAG(nodes []smithers.Node) string {
    // Group by depth (topological sort)
    // Render left-to-right with box-drawing characters
    // Color by status: green=done, yellow=running, red=failed, gray=pending
}
```

Output example:
```
✓ fetch-deps ──┐
✓ build ───────┤
✓ test ────────┼──▸ ⏸ deploy ──▸ ○ verify ──▸ ○ notify
✓ lint ────────┘
```

---

### Phase 4 — Polish (Week 9-10)

#### 3.4.1 Scores View

Simple table view backed by `Client.GetScores()`.

#### 3.4.2 Memory Browser

List/recall view backed by `Client.ListMemoryFacts()` and
`Client.RecallMemory()`.

#### 3.4.3 Cron Manager

CRUD view for cron schedules backed by Smithers cron commands.

#### 3.4.4 Workflow List

Discovery view showing workflows from `.smithers/workflows/` with
run action.

---

## 4. Data Architecture

### 4.1 Smithers Data Access

The TUI needs read access to Smithers data. Two complementary paths:

```
┌─────────────┐     HTTP      ┌─────────────────┐
│ Smithers TUI │ ◀──────────▶ │ Smithers Server  │
│   (Go)       │   REST/SSE   │ (smithers up     │
│              │              │  --serve)         │
└──────┬───────┘              └────────┬──────────┘
       │                               │
       │  Direct read                  │  Read/Write
       │  (fallback)                   │
       ▼                               ▼
┌──────────────────────────────────────────────┐
│            .smithers/smithers.db              │
│                  (SQLite)                     │
└──────────────────────────────────────────────┘
```

**HTTP path** (primary): Used when `smithers up --serve` is running.
Provides full CRUD + SSE streaming. Authenticated via bearer token.

**Direct DB path** (fallback): For read-only operations when no server
is running. Opens SQLite in read-only mode (`?mode=ro`). Good for
inspecting past runs, browsing scores, etc.

### 4.2 TUI Session Storage

The TUI maintains its own SQLite DB (inherited from Crush) for:
- Chat sessions with the Smithers agent
- User preferences
- Command history

This is separate from Smithers' workflow DB.

### 4.3 Event Streaming

Real-time updates flow via SSE from the Smithers HTTP API:

```go
// internal/smithers/events.go
func (c *Client) StreamEvents(ctx context.Context, afterSeq int) (<-chan Event, error) {
    url := fmt.Sprintf("%s/events?afterSeq=%d", c.apiURL, afterSeq)
    resp, err := http.Get(url)
    // Parse SSE stream
    // Emit typed events: RunStarted, NodeCompleted, ApprovalRequested, etc.
}
```

Events are consumed by the TUI's pub/sub system and routed to active views.

---

## 5. MCP Integration Details

### 5.1 Smithers MCP Server

A new `mcp-serve` subcommand in the Smithers CLI that exposes tools via
MCP stdio transport. This is the primary way the TUI's chat agent
interacts with Smithers.

**Why MCP (not direct Go calls)**:
1. Reuses Crush's proven MCP client infrastructure.
2. MCP server is independently useful (works with any MCP-compatible client).
3. Decouples TUI from Smithers internals.
4. TypeScript MCP server matches Smithers' existing codebase language.

**Tool categories**:

| Category | Tools | Returns |
|----------|-------|---------|
| Runs | `ps`, `up`, `cancel`, `down` | Run lists, status |
| Observe | `logs`, `chat`, `inspect` | Event logs, chat blocks, detail |
| Control | `approve`, `deny` | Confirmation |
| Navigate | `hijack` | Session metadata |
| Time-travel | `diff`, `fork`, `replay`, `timeline` | Diffs, new run IDs |
| Workflow | `workflow_list`, `workflow_run`, `workflow_doctor` | Workflow info |
| Memory | `memory_list`, `memory_recall` | Facts |
| Scoring | `scores` | Score data |
| Cron | `cron_list`, `cron_add`, `cron_rm`, `cron_toggle` | Schedule info |
| Query | `sql` | Raw query results |

### 5.2 Smithers-Specific Tool Renderers

When the agent calls Smithers MCP tools, we render the results with
custom UI instead of plain text.

**`internal/ui/chat/smithers_ps.go`**:
```go
// Renders smithers_ps results as a styled table
func renderSmithersPS(result mcp.ToolResult) string {
    var runs []smithers.Run
    json.Unmarshal(result.Content, &runs)
    return renderRunTable(runs)
}
```

**`internal/ui/chat/smithers_approve.go`**:
```go
// Renders approval confirmation with status indicator
func renderSmithersApprove(result mcp.ToolResult) string {
    return styles.Success.Render("✓ Approved: " + result.NodeID)
}
```

We register these renderers in the tool renderer registry (Crush already
has this pattern for its 57 built-in tools).

---

## 6. Testing Strategy

### 6.1 Unit Tests

- **Smithers client**: Mock HTTP server + test SQLite DB.
- **View components**: Bubble Tea test framework (`teatest`).
- **Tool renderers**: Snapshot tests comparing rendered output.
- **Router**: State machine transitions.

### 6.2 Integration Tests

- **MCP round-trip**: Start Smithers MCP server, verify tool discovery
  and execution from Go client.
- **SSE streaming**: Mock SSE server, verify event parsing.
- **DB access**: Real SQLite with test fixtures.

### 6.3 End-to-End Tests

- **TUI interaction**: Use `teatest` or VHS (Charm's terminal recorder)
  to verify full user flows.
- **Hijack flow**: Full hijack lifecycle with mock agent.

---

## 7. Build & Distribution

### 7.1 Binary

```bash
# Build
go build -o smithers-tui .

# Or install
go install github.com/anthropic/smithers-tui@latest
```

### 7.2 Integration with Smithers

Option A: **Standalone binary** — users install separately.

Option B: **Subcommand** — `smithers tui` spawns the Go binary.
Requires the Go binary to be bundled with Smithers (npm postinstall
or separate download).

**Recommendation**: Option B for discoverability, with Option A as
fallback. `smithers tui` checks for the binary in PATH, downloads
if missing.

### 7.3 Release

- Go binary: Cross-compiled for darwin/amd64, darwin/arm64, linux/amd64.
- Homebrew formula: `brew install anthropic/tap/smithers-tui`.
- npm wrapper: `npx smithers-tui` (downloads Go binary).

---

## 8. Migration Path from Crush

### 8.1 Files to Keep As-Is

| Directory/File | Reason |
|----------------|--------|
| `internal/ui/chat/` (most renderers) | Tool rendering reused |
| `internal/mcp/` | MCP client unchanged |
| `internal/lsp/` | LSP support useful for workflow authoring |
| `internal/db/` | Session storage |
| `internal/pubsub/` | Event system |
| `internal/agent/coordinator.go` | Agent loop |
| `internal/agent/session.go` | Session agent |
| `internal/agent/tools/` (most) | Keep read, edit, bash, grep, etc. |

### 8.2 Files to Modify

| File | Changes |
|------|---------|
| `internal/cmd/root.go` | Branding, defaults, Smithers client init |
| `internal/ui/model/ui.go` | View router, Smithers keybindings |
| `internal/ui/model/keys.go` | Add ctrl+r, ctrl+a, etc. |
| `internal/ui/styles/styles.go` | Color scheme |
| `internal/ui/logo/logo.go` | Smithers ASCII art |
| `internal/config/config.go` | Smithers config section |
| `internal/config/defaults.go` | Default MCP server, tools |
| `internal/app/app.go` | Initialize Smithers client |

### 8.3 Files to Add

| File | Purpose |
|------|---------|
| `internal/smithers/` (package) | Smithers domain logic |
| `internal/ui/views/` (package) | All new views |
| `internal/ui/components/` (package) | Reusable UI components |
| `internal/ui/chat/smithers_*.go` | Custom tool renderers |
| `internal/agent/templates/smithers.md.tpl` | System prompt |

### 8.4 Files to Remove

| File | Reason |
|------|---------|
| `internal/ui/logo/logo.go` (Crush logo) | Replaced |
| Crush-specific branding assets | Replaced |
| `CRUSH.md` context file handling | Replace with `SMITHERS-TUI.md` |

---

## 9. Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Crush upstream breaks fork | Medium | Clean initial commit, selective cherry-picks |
| Smithers MCP server not ready | High | Direct CLI execution fallback (`exec.Command("smithers", ...)`) |
| Hijack complexity across 7 agents | High | Start with claude-code only, add others iteratively |
| SSE reliability | Medium | Reconnection logic, direct DB fallback |
| Terminal compatibility | Low | Bubble Tea handles most terminals; test in common ones |
| Performance with many runs | Medium | Pagination, lazy loading, debounced updates |

---

## 10. Dependencies

### New Go Dependencies

| Package | Purpose |
|---------|---------|
| (none new required) | Crush already includes Bubble Tea, Lip Gloss, harmonica, etc. |

### External Dependencies

| Dependency | Required For |
|------------|-------------|
| `smithers` CLI | MCP server, workflow execution |
| Smithers SQLite DB | Direct data access |
| Agent CLIs (claude-code, codex, etc.) | Hijacking |

---

## 11. Open Technical Decisions

1. **View rendering**: Should views use Bubble Tea's standard Elm Update()
   or Crush's imperative sub-component pattern? **Recommendation**: Follow
   Crush's pattern for consistency, even though Elm is more idiomatic.

2. **Smithers client transport**: HTTP-first or DB-first?
   **Recommendation**: HTTP-first (more complete API), DB fallback for
   offline/read-only access.

3. **Hijack PTY handling**: Should we use a raw PTY or parse structured
   output? **Recommendation**: Start with structured (NDJSON from agent
   CLI), fall back to raw PTY for native-cli mode.

4. **Event handling**: Should SSE events go through Crush's pubsub or a
   separate channel? **Recommendation**: Through pubsub for consistency
   with Crush's architecture.

5. **Split panes**: Should Phase 1 include split pane support (chat +
   runs side by side)? **Recommendation**: Defer to Phase 3+. View stack
   is simpler and sufficient for v1.
