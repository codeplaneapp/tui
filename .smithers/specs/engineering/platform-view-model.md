# Platform View Stack Architecture - Research Summary

## Ticket: platform-view-model

Introduce the `View` interface representing distinct TUI screens (Runs, SQL, Timeline, Chat) adhering to the Workspace/Systems separation.

## Acceptance Criteria
- View interface defined with Init, Update, View, and Name methods
- ShortHelp() method added to View to power contextual help bars

## Current State of Codebase

### Existing Router (`internal/ui/views/router.go`)
A `ViewRouter` struct already exists with:
- `activeView` field (type `View`)
- `views` map (`map[string]View`)
- `Register(view View)` method
- `Switch(name string)` method
- `ActiveView() View` accessor
- `Update(msg tea.Msg)` that delegates to active view
- `View() string` that delegates to active view

The existing `View` interface has: `Init() tea.Cmd`, `Update(msg tea.Msg) (tea.Model, tea.Cmd)`, `View() string`, `Name() string`

### Existing Agents View (`internal/ui/views/agents.go`)
An `AgentsView` struct exists implementing the View interface with placeholder content showing "Agents View - Coming Soon".

### Main UI Model (`internal/ui/model/ui.go`)
The main `Model` struct in `internal/ui/model/ui.go` is a large Bubble Tea model (~220 lines for struct definition alone) that manages:
- Session state, dialog handling, permissions
- Terminal capabilities, textarea editor, attachments
- Completions, chat component, onboarding state
- Progress bar, header component

This model does NOT yet integrate the ViewRouter. The current architecture has the chat/conversation as the primary (and only) view baked directly into the Model struct.

### Dialog System (`internal/ui/dialog/`)
- `actions.go` and `commands.go` handle dialog-based UI interactions
- These are separate from the view system

## Implementation Plan

### Step 1: Enhance the View Interface
Add `ShortHelp() []key.Binding` method to the existing View interface in `router.go` to power contextual help bars per the acceptance criteria.

### Step 2: No Other Changes Needed
The View interface already has Init, Update, View, and Name methods. The ViewRouter already exists with Register, Switch, and delegation logic. The only missing piece from the acceptance criteria is `ShortHelp()`.

### Key Files to Modify
1. `internal/ui/views/router.go` - Add ShortHelp to View interface
2. `internal/ui/views/agents.go` - Add ShortHelp implementation to AgentsView

### Dependencies
None listed. This is a foundational ticket.

### Notes
- The ViewRouter is not yet wired into the main Model - that integration is likely a separate ticket
- The existing View interface closely mirrors tea.Model but adds Name() for routing
- ShortHelp() follows the pattern from Bubble Tea's help.KeyMap interface