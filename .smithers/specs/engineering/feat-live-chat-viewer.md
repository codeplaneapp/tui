I've completed an extensive exploration of the crush codebase to understand its architecture, patterns, and implementation details. Here's a comprehensive summary:

## Project Overview

Crush is a Go-based TUI (Terminal User Interface) application built with the Bubble Tea framework. It serves as a developer operations dashboard for managing AI agent workflows, runs, approvals, and live chat interactions. The project uses a Smithers backend integration for workflow orchestration.

## Architecture

### Core Stack
- **Language**: Go
- **TUI Framework**: Bubble Tea (charmbracelet/bubbletea)
- **Styling**: Lip Gloss (charmbracelet/lipgloss)
- **Backend**: Smithers HTTP API + SQLite database
- **Build**: Standard Go modules

### Directory Structure
- `cmd/crush/` - Main entry point
- `internal/ui/model/` - Core UI model (Bubble Tea model)
- `internal/ui/views/` - View implementations (runs, agents, approvals, livechat, router)
- `internal/ui/dialog/` - Dialog system (actions, commands, rendering)
- `internal/ui/styles/` - Shared Lip Gloss styles and theme
- `internal/ui/components/` - Reusable UI components (toast, splitpane, helpbar)
- `internal/smithers/` - Smithers API client and types
- `internal/config/` - Configuration management

### Key Patterns

1. **View System**: Views implement a common pattern with `Init()`, `Update()`, `View()`, and `Focused()` methods. The router (`internal/ui/views/router.go`) manages view switching via `ViewMsg`.

2. **Dialog System**: A command palette/dialog system (`internal/ui/dialog/`) provides keyboard-driven actions. Actions are registered per-view and globally.

3. **Smithers Client**: The `internal/smithers/client.go` provides HTTP API client methods for runs, workflows, agents, approvals, cron schedules, and system information. It communicates with a local Smithers server.

4. **Message-Passing**: Standard Bubble Tea message passing with custom message types defined in each view and the dialog system.

5. **Feature Flags**: Views and features are gated behind feature flags defined in `docs/smithers-tui/features.ts` and checked at runtime.

## Existing Views

### Runs View (`internal/ui/views/runs.go`)
- Displays workflow runs in a table format
- Shows status, workflow name, run ID, started time, and duration
- Supports filtering by status (running, completed, failed, pending)
- Auto-refreshes via tick messages
- Actions: view details, open live chat, cancel run, approve/deny

### Agents View (`internal/ui/views/agents.go`)
- Lists available AI agents
- Shows agent name, role, engine, and status
- Color-coded status indicators

### Approvals View (`internal/ui/views/approvals.go`)
- Manages pending approval requests
- Inline approve/deny actions
- Shows approval context and metadata

### Live Chat View (`internal/ui/views/livechat.go`)
- Skeleton implementation exists
- Header with run ID, agent name, node, elapsed time
- Placeholder for streaming chat content

### Router (`internal/ui/views/router.go`)
- Manages active view state
- Handles view switching via ViewMsg
- Delegates Update/View calls to active view

## Smithers Client API

The client (`internal/smithers/client.go`) provides:
- **Runs**: List, Get, Cancel, GetEvents
- **Workflows**: List, Get, Run, GetNodes
- **Agents**: List, Get
- **Approvals**: List pending, Approve, Deny
- **Cron Schedules**: List, Get, Create, Update, Delete
- **System**: Health check, Info

## Types System

`internal/smithers/types.go` defines all data types:
- Run, RunEvent, RunStatus
- Workflow, WorkflowNode
- Agent
- Approval, ApprovalDecision
- CronSchedule
- SystemInfo, HealthStatus

## Styling

`internal/ui/styles/styles.go` provides:
- Color constants (StatusRunning, StatusCompleted, StatusFailed, etc.)
- Shared style functions (HeaderStyle, TableHeaderStyle, etc.)
- Consistent theming across views

## Component Library

- **Toast** (`internal/ui/components/toast.go`): In-terminal notification overlays
- **Split Pane** (`internal/ui/components/splitpane.go`): Resizable split pane layout
- **Help Bar** (`internal/ui/components/helpbar.go`): Context-sensitive keyboard shortcut hints

## Configuration

`internal/config/config.go` manages:
- Smithers server URL
- API keys
- Feature flag overrides
- Theme preferences

## Ticket/Spec System

The `.smithers/` directory contains an extensive ticket tracking system:
- `tickets/` - Individual feature tickets in markdown
- `specs/` - Engineering specs, research docs, plans, and reviews
- Organized by feature groups (agents, approvals, chat, runs, workflows, etc.)

## Documentation

`docs/smithers-tui/` contains:
- `01-PRD.md` - Product requirements
- `02-DESIGN.md` - Design specifications
- `03-ENGINEERING.md` - Engineering guidelines
- `features.ts` - Feature flag definitions

## Reference GUI

`smithers_tmp/gui-ref/` contains a TypeScript/React reference implementation of a web-based Smithers GUI, providing patterns for:
- Daemon architecture
- API routes
- Web UI components
- Feature organization