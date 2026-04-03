# Smithers TUI — Product Requirements Document

**Status**: Draft
**Date**: 2026-04-02
**Author**: William Cory

---

## 1. Overview

**Smithers TUI** is a terminal-based dashboard and control plane for Smithers,
built by forking [Crush](https://github.com/charmbracelet/crush) (Charm's
Go/Bubble Tea AI chat TUI). The default screen remains a conversational chat
interface, but the agent is specialized for Smithers operations and the tool
palette is wired to Smithers' CLI via MCP.

The product replaces the three incomplete TUI attempts (`tui`, `tui-v2`,
`tui-v3`) with a shipping terminal experience that achieves feature parity
with the Electrobun GUI (our best prior attempt) while adding chat-first
AI interaction. The Go TUI is a **thin frontend** — it does not implement
Smithers business logic. Instead it talks to the Smithers CLI (TypeScript)
running as an HTTP server, shells out to it, or connects to it as an MCP
server, mirroring the same transport layer architecture the GUI used.

---

## 2. Problem Statement

Smithers has 40+ CLI commands spread across `up`, `ps`, `logs`, `chat`,
`hijack`, `inspect`, `approve`, `deny`, `cancel`, `replay`, `fork`, `diff`,
`timeline`, `workflow`, `memory`, `rag`, `cron`, `scores`, and more. Users
currently must:

1. Memorize commands and flags to operate workflows.
2. Open multiple terminal tabs to simultaneously monitor runs, view chats,
   and approve gates.
3. Context-switch between raw CLI output and separate dashboards.
4. Manually piece together run status, agent output, and time-travel state.

Three prior TUI attempts (built in TypeScript with OpenTUI) were never
completed. The Electrobun desktop GUI was our most complete attempt and
serves as the **feature reference** for this project — it implemented runs,
workflows, agents, prompts, tickets, console chat, SQL browser, and
triggers across 8 tabs. Crush already ships a production-quality,
extensible TUI with chat, MCP, tool rendering, sessions, and a command
palette — exactly the foundation we need to surpass the GUI.

---

## 3. Goals

| # | Goal | Success Metric |
|---|------|----------------|
| G1 | Single pane of glass for Smithers operations | Users can monitor, control, and chat with workflows without leaving the TUI |
| G2 | Chat-first UX | Default screen is a conversational interface; agent understands Smithers domain |
| G3 | Live observability | Real-time run status, event feed, agent chat streaming |
| G4 | Session hijacking | User can view a running agent's chat, then take over ("hijack") with zero handoff latency |
| G5 | Approval gate management | Approve/deny gates inline without switching to CLI |
| G6 | Time-travel debugging | Inspect snapshots, diff states, fork/replay from any checkpoint |
| G7 | Workflow management | List, run, inspect, and create workflows from the TUI |
| G8 | GUI feature parity | Every feature in the Electrobun GUI is available in the TUI |
| G9 | Thin frontend | Go TUI contains zero Smithers business logic; all ops go through Smithers CLI server / MCP / shell-out |
| G10 | Built on Crush | Leverage Crush's mature Bubble Tea infrastructure, sessions, MCP, tool rendering |
| G11 | Native TUI handoff | Seamlessly suspend the TUI to launch external programs (agent CLIs, editors) and resume on exit |

---

## 4. Non-Goals (v1)

- **Voice interaction** — Smithers supports `<Voice>` nodes but TUI v1 is
  text-only.
- **RAG ingestion** — Users can query RAG via the agent, but document
  ingestion is CLI-only.
- **Workflow authoring** — The TUI does not provide a JSX/TOON editor.
  Users author workflows in their normal editor.
- **Multi-user collaboration** — Single-user terminal session.
- **Replacing the HTTP API** — The TUI consumes the API; it does not replace
  it.

---

## 5. User Personas

### 5.1 Workflow Operator
Runs workflows daily, monitors status, handles approval gates, investigates
failures. Needs: run list, event feed, approval queue, logs.

### 5.2 AI Developer
Builds and debugs Smithers workflows. Needs: live chat viewing, session
hijacking, time-travel debugging, workflow doctor.

### 5.3 Team Lead
Checks aggregate status and scores. Needs: run overview, ROI/scores,
cron schedule visibility.

---

## 6. Features

### 6.1 Chat Interface (Default Screen)

The primary screen is a chat with a Smithers-specialized agent. The agent has:

- **System prompt**: Smithers domain knowledge — workflows, components,
  agents, time-travel, approval patterns.
- **MCP tools**: Full Smithers CLI exposed as MCP tools (see §6.8).
- **Built-in tools**: File read/write/edit, bash, grep, glob (inherited from
  Crush).
- **Context awareness**: Reads `.smithers/` directory, active runs, workflow
  definitions.

**Example interactions**:
```
> What workflows are available?
> Run the code-review workflow on ticket PROJ-123
> Show me the status of all active runs
> Why did run abc123 fail?
> Approve the pending gate on the deploy workflow
> Hijack the agent session on run abc123
> Show me the diff between snapshots 3 and 7
> What's the ROI score for yesterday's runs?
```

### 6.2 Run Dashboard

A dedicated view showing all Smithers runs:

- **Run list**: Active, paused, completed, failed runs with status indicators.
- **Real-time updates**: Status changes stream in via SSE/polling.
- **Inline details**: Expand a run to see nodes, current step, elapsed time.
- **Quick actions**: Approve, deny, cancel, hijack from the run list.
- **Filtering**: By status, workflow name, date range.

### 6.3 Live Chat Viewer

View a running agent's chat output in real-time:

- **Streaming output**: Prompt, stdout, stderr, response rendered live.
- **Attempt tracking**: See current attempt number and retry history.
- **Tool call rendering**: Visualize tool calls and results (leveraging
  Crush's 57 tool renderers).
- **Follow mode**: Auto-scroll as new output arrives.
- **Multi-pane**: View chat alongside run status.

### 6.4 Chat Hijacking (Native TUI Handoff)

Take over a running agent session by handing off to the agent's own TUI:

- **Hijack command**: `/hijack <run_id>` from chat or command palette.
- **Native TUI handoff**: Smithers TUI suspends itself and launches the
  agent's native CLI/TUI (e.g., `claude-code --resume`, `codex`, `amp`).
  The user gets the full native experience of that agent's own interface.
  When the user exits the agent TUI, Smithers TUI resumes automatically.
- **Seamless transition**: Bubble Tea's `tea.ExecProcess` handles terminal
  handoff — no PTY management, no I/O proxying. The agent TUI gets full
  control of stdin/stdout/stderr.
- **On return**: Smithers TUI refreshes run state, shows updated chat
  history, and resumes normal operation.
- **Engine support**: claude-code, codex, gemini, pi, kimi, forge, amp.
- **Fallback**: For agents without a TUI or `--resume` support, fall back
  to conversation replay through the Smithers chat interface.

### 6.5 Approval Gate Management

- **Notification**: Badge/indicator when approvals are pending.
- **Approval queue**: Dedicated view listing all pending gates.
- **Inline approve/deny**: Act on gates without leaving current view.
- **Context display**: Show the task that needs approval, its inputs, and
  the workflow context.

### 6.6 Time-Travel Debugging

- **Timeline view**: Visual timeline of run execution with snapshot markers.
- **Snapshot inspector**: View full state at any checkpoint.
- **Diff**: Compare two snapshots side-by-side (nodes, outputs, state).
- **Fork**: Create a branched run from any snapshot.
- **Replay**: Fork + immediately run with optional input override.

### 6.7 Workflow Management

- **List workflows**: Show discovered workflows from `.smithers/workflows/`.
- **Run workflow**: Start a workflow with dynamic input forms (string,
  number, boolean, object, array types — matching the GUI's form generation).
- **Inspect workflow**: View DAG structure, agents, schemas.
- **Doctor**: Run diagnostics on workflow configuration.

### 6.8 Agent Browser & Chat (Native TUI Handoff)

Mirrors the GUI's **Agents** tab (`AgentsList.tsx` + `AgentChat.tsx`).

- **Agent detection**: List all CLI agents found on the system (claude,
  codex, gemini, kimi, amp, forge, pi).
- **Status display**: For each agent show binary path, availability,
  auth status (`likely-subscription`, `api-key`, `binary-only`,
  `unavailable`), and roles.
- **Agent chat via native TUI**: Pressing Enter on an agent suspends
  Smithers TUI and launches the agent's own CLI/TUI (e.g., `claude-code`,
  `codex`). The user gets the full native agent experience. When the user
  exits the agent TUI, Smithers TUI resumes.
- **Use case**: Quick ad-hoc conversations with a specific agent outside
  of a workflow context, using that agent's own interface.

### 6.9 Ticket Manager

Mirrors the GUI's **Tickets** tab (`TicketsList.tsx`).

- **List tickets**: Show all tickets from `.smithers/tickets/`.
- **View ticket**: Display ticket markdown content.
- **Create ticket**: Create a new ticket with ID + content.
- **Edit ticket**: Edit ticket content inline in a textarea/editor.
- **Split-pane layout**: List on the left, detail/editor on the right.

### 6.10 Prompt Editor & Preview

Mirrors the GUI's **Prompts** tab (`PromptsList.tsx`).

- **List prompts**: Show all prompts from `.smithers/prompts/`.
- **View/Edit source**: Edit `.mdx` prompt source in a textarea or
  external editor (`$EDITOR`).
- **Props discovery**: Automatically detect `{props.variableName}`
  patterns and present input fields.
- **Live preview**: Render the prompt with test props and show the
  output.
- **Save**: Persist changes back to the prompt file.

### 6.11 SQL Browser

Mirrors the GUI's **Database** tab (`SqlBrowser.tsx`).

- **Query editor**: Text input for raw SQL queries against the Smithers
  SQLite database.
- **Table sidebar**: Clickable list of available tables
  (`_smithers_runs`, `_smithers_nodes`, `_smithers_events`,
  `_smithers_chat_attempts`, `_smithers_memory`).
- **Results table**: Dynamic column detection, horizontal scroll for
  wide results.
- **Use case**: Power-user debugging and ad-hoc analysis.

### 6.12 Triggers / Cron Manager

Mirrors the GUI's **Triggers** tab (`TriggersList.tsx`).

- **List triggers**: Show all scheduled cron triggers with workflow path,
  cron pattern, and enabled status.
- **Toggle**: Enable/disable triggers with a single keypress.
- **Create/Edit/Delete**: Full CRUD for cron schedules.

### 6.13 MCP Tool Integration

Smithers CLI is exposed as an MCP server providing these tool groups:

| Tool Group | Tools |
|------------|-------|
| **Runs** | `ps`, `up`, `cancel`, `down` |
| **Observability** | `logs`, `chat`, `inspect`, `timeline` |
| **Control** | `approve`, `deny`, `hijack` |
| **Time-Travel** | `diff`, `fork`, `replay`, `revert` |
| **Workflows** | `workflow list`, `workflow run`, `workflow doctor` |
| **Agents** | `agent list`, `agent chat` |
| **Tickets** | `ticket list`, `ticket create`, `ticket update` |
| **Prompts** | `prompt list`, `prompt update`, `prompt render` |
| **Memory** | `memory list`, `memory recall` |
| **Scoring** | `scores` |
| **Cron** | `cron list`, `cron add`, `cron rm`, `cron toggle` |
| **SQL** | `sql` (direct SQLite query) |

### 6.14 Scores / ROI Dashboard

- **Run scores**: View evaluation scores for completed runs.
- **Metrics**: Token usage, tool call counts, latency, cache efficiency.
- **Aggregation**: Daily/weekly summaries.
- **Cost tracking**: Estimated cost per run/workflow.

### 6.15 Memory Browser

- **Fact list**: View cross-run memory facts.
- **Semantic recall**: Query memory with natural language.
- **Message history**: Browse conversation threads across runs.

### 6.16 Native TUI Handoff

A cross-cutting capability that allows Smithers TUI to seamlessly suspend
itself and hand terminal control to an external program, resuming when that
program exits. This uses Bubble Tea's `tea.ExecProcess` which Crush already
uses for its `Ctrl+O` editor integration.

**Handoff scenarios**:

| Trigger | External Program | On Return |
|---------|-----------------|-----------|
| Hijack a run (`h` key) | Agent's native TUI (`claude-code --resume`, `codex`, etc.) | Refresh run state, show updated chat |
| Chat with agent (Enter in agent list) | Agent's native TUI (`claude-code`, `codex`, `amp`, etc.) | Return to agent browser |
| Edit file (from chat tool result) | `$EDITOR` (nvim, vim, helix, etc.) | Refresh file state in chat context |
| Edit ticket (`Ctrl+O` in ticket editor) | `$EDITOR` on ticket file | Reload ticket content |
| Edit prompt (`Ctrl+O` in prompt editor) | `$EDITOR` on prompt `.mdx` file | Reload prompt, re-render preview |

**Why native handoff over embedded UI**:
- Users get the **full native experience** of each tool (syntax highlighting,
  plugins, keybindings, completion, etc.).
- **Zero implementation cost** for replicating agent UIs — we don't need to
  build a claude-code clone or a codex clone inside Smithers.
- **Automatically supports new agents** — any agent that ships a CLI/TUI
  works with zero Smithers-side changes.
- **Proven pattern** — Crush already does this for `$EDITOR` via `Ctrl+O`.

---

## 7. Navigation Model

The TUI uses a **chat-first, view-switching** navigation model:

- **Default**: Chat view (always accessible via `Esc` or keybinding).
- **Views**: Switched via command palette (`/` or `Ctrl+P`) or keybindings.
  Organized to mirror the GUI's two-section sidebar (Workspace + Systems):

  **Workspace**:
  - `/runs` → Run Dashboard
  - `/workflows` → Workflow List & Executor
  - `/agents` → Agent Browser & Chat
  - `/prompts` → Prompt Editor & Preview
  - `/tickets` → Ticket Manager

  **Systems**:
  - `/console` → Chat (default/home view)
  - `/sql` → SQL Browser
  - `/triggers` → Triggers / Cron Manager

  **Detail views** (reached from parent views):
  - `/chat <run_id>` → Live Chat Viewer
  - `/approvals` → Approval Queue
  - `/timeline <run_id>` → Time-Travel Timeline
  - `/scores` → ROI Dashboard
  - `/memory` → Memory Browser
- **Split panes**: Optional side-by-side layout (chat + run dashboard).
- **Back stack**: Views push onto a stack; `Esc` pops back.

---

## 8. Branding

| Element | Crush | Smithers TUI |
|---------|-------|--------------|
| Name | CRUSH | SMITHERS |
| Header | `Charm™ CRUSH` | `SMITHERS` |
| Color scheme | Purple/magenta | TBD (Smithers brand colors) |
| Logo | Crush ASCII art | Smithers ASCII art |
| Config dir | `.crush/` | `.smithers-tui/` (separate from `.smithers/`) |
| Config file | `crush.json` | `smithers-tui.json` |
| Binary name | `crush` | `smithers-tui` (or `stui`) |

---

## 9. Configuration

Smithers TUI reads configuration from (in priority order):

1. `.smithers-tui.json` (project-level)
2. `smithers-tui.json` (project-level)
3. `~/.config/smithers-tui/smithers-tui.json` (user-level)
4. Embedded defaults

**Key config**:
```jsonc
{
  "defaultModel": "claude-opus-4-6",
  "smithers": {
    "dbPath": ".smithers/smithers.db",        // Smithers SQLite DB
    "apiUrl": "http://localhost:7331",         // Smithers HTTP API (if running)
    "apiToken": "${SMITHERS_API_KEY}",
    "workflowDir": ".smithers/workflows"
  },
  "mcpServers": {
    "smithers": {
      "type": "stdio",
      "command": "smithers",
      "args": ["mcp-serve"]
    }
  }
}
```

---

## 10. Success Criteria

| Criterion | Target |
|-----------|--------|
| Users can monitor all active runs without leaving TUI | 100% of `ps` functionality |
| Users can approve/deny gates from TUI | < 3 keystrokes from notification |
| Users can hijack a running agent session | Native TUI handoff, instant suspend/resume |
| Users can time-travel debug from TUI | Diff, fork, replay all functional |
| Chat agent can answer Smithers questions | Handles 90% of common queries |
| Startup time | < 500ms to interactive |

---

## 11. Architecture Principle: Thin Frontend

The Go TUI is a **presentation layer only**. It does NOT contain Smithers
business logic. All data and mutations flow through one of four channels:

```
┌──────────────────┐
│  Smithers TUI    │  (Go / Bubble Tea)
│  Thin Frontend   │
└─┬──────┬───┬───┬─┘
  │      │   │   │
  │      │   │   └── 4. TUI Handoff: tea.ExecProcess(cmd)
  │      │   │           Suspend TUI, launch agent CLI/editor,
  │      │   │           resume on exit. For hijack, agent chat,
  │      │   │           file editing.
  │      │   │
  │      │   └────── 3. Shell-out: exec("smithers", args...)
  │      │               For one-shot commands, fallback
  │      │
  │      └────────── 2. MCP Server: smithers mcp-serve (stdio)
  │                      For AI agent tool calls
  │
  └───────────────── 1. HTTP API: smithers up --serve
                         For views, SSE streaming, mutations
```

This mirrors the GUI's architecture: the SolidJS frontend called the same
Smithers CLI server via HTTP (`/ps`, `/sql`, `/workflow/run/{id}`, etc.)
through a transport layer (`gui/src/ui/api/transport.ts`). The TUI does
the same thing in Go — plus adds channel 4 (TUI handoff), which has no
GUI equivalent since desktop apps can't suspend themselves to launch a
terminal program.

**Consequences**:
- No Drizzle ORM, no SQLite access, no workflow engine code in Go.
- The `internal/smithers/` package is purely an HTTP/exec client.
- All 17 GUI API endpoints are consumed by the TUI.
- MCP server provides the same data to the chat agent.
- Hijacking and agent chat use native TUI handoff — no need to replicate
  agent UIs inside Smithers.

## 12. Open Questions

1. **MCP server**: Does Smithers already expose `mcp-serve`, or do we need
   to build it? (Current: `createSmithersObservabilityLayer` exists but
   full MCP server not yet shipped.)
2. **Agent model**: Should the default agent be Claude (via Anthropic API)
   or should it support the same multi-provider system as Crush?
3. **Binary distribution**: Ship as `smithers-tui` standalone or as a
   subcommand `smithers tui`?
4. **Split pane**: Should v1 support split panes or defer to later?

---

## 12. Milestones

| Phase | Scope |
|-------|-------|
| **P0 — Foundation** | Fork Crush, rebrand, wire Smithers CLI server + MCP, specialized system prompt |
| **P1 — Workspace Core** | Runs dashboard, workflow list & executor, agent browser & chat (GUI parity: 3/5 workspace tabs) |
| **P2 — Workspace Complete** | Tickets manager, prompt editor & preview, node inspector + task tabs (GUI parity: 5/5 workspace tabs) |
| **P3 — Systems** | SQL browser, triggers/cron manager (GUI parity: all systems tabs) |
| **P4 — Live Chat + Hijack** | Live chat viewer, session hijacking, approval management, notifications |
| **P5 — Time-Travel** | Timeline view, snapshot inspector, diff, fork, replay |
| **P6 — Polish** | Scores/ROI, memory browser, workflow doctor |

### 12.1 GUI Feature Parity Checklist

| GUI Tab | TUI View | Phase |
|---------|----------|-------|
| Runs (RunsList + NodeInspector + TaskTabs) | `/runs` + inspect + task tabs | P1, P2 |
| Workflows (WorkflowsList) | `/workflows` | P1 |
| Agents (AgentsList + AgentChat) | `/agents` | P1 |
| Prompts (PromptsList) | `/prompts` | P2 |
| Tickets (TicketsList) | `/tickets` | P2 |
| Console (ChatConsole) | Default chat view | P0 |
| Database (SqlBrowser) | `/sql` | P3 |
| Triggers (TriggersList) | `/triggers` | P3 |
