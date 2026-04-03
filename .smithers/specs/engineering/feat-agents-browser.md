# Implementation Plan: feat-agents-browser

## Ticket Summary
- **ID**: feat-agents-browser
- **Group**: Agents
- **Dependencies**: eng-agents-view-scaffolding
- **Goal**: Implement the main Bubble Tea view for the Agent Browser, rendering the layout frame and handling standard navigation.

## Acceptance Criteria
1. Navigating to /agents or using the command palette opens the Agents view.
2. The view displays a 'SMITHERS › Agents' header and a placeholder list.
3. Pressing Esc returns the user to the previous view (chat/console).

## Existing Code Analysis

### Current agents.go (internal/ui/views/agents.go)
Already has a basic scaffold with:
- `AgentsView` struct with `width`, `height`, `focused` fields
- `NewAgentsView()` constructor
- Empty `Init()`, `Update()`, `View()` methods (View returns placeholder string)
- `SetSize(w, h int)` method

### Router (internal/ui/views/router.go)
Already has:
- `ViewType` enum with `ViewAgents` constant
- `Router` struct managing view switching
- `Navigate(vt ViewType)` method that handles `ViewAgents` by calling `NewAgentsView()`
- `Back()` method using a history stack for Esc navigation
- `ActiveView()` returns current view as `tea.Model`

### Dialog layer (internal/ui/dialog/)
- `actions.go`: Has `NavigateAction` with a `View` field, and action constants including `ActionNavigateAgents`
- `commands.go`: Has `NavigateTo(view string)` command helper

### UI model (internal/ui/model/ui.go)
- Main UI model handles dialog actions
- Has a `router` field of type `*views.Router`
- Processes `NavigateAction` in Update, calling `m.router.Navigate()`

### Smithers Client (internal/smithers/client.go & types.go)
- `Client` struct with `baseURL` and `http.Client`
- `ListAgents(ctx) ([]AgentInfo, error)` method exists
- `AgentInfo` type has: `AgentID`, `Name`, `Role`, `Binary`, `Status`, `Available`, `AuthStatus`, `CreatedAtMs`

## Implementation Plan

### Step 1: Enhance AgentsView struct (internal/ui/views/agents.go)
- Add fields: `agents []smithers.AgentInfo`, `loading bool`, `err error`, `cursor int`
- Import the smithers client package
- Add a `smithersClient *smithers.Client` field or accept agents as data

### Step 2: Implement Init() to fetch agents
- Return a `tea.Cmd` that calls the smithers client `ListAgents()`
- Define a `agentsLoadedMsg` and `agentsErrorMsg` message types
- Set `loading = true` initially

### Step 3: Implement Update() for keyboard navigation
- Handle `agentsLoadedMsg`: store agents, set loading=false
- Handle `agentsErrorMsg`: store error, set loading=false
- Handle `key.Matches` for:
  - `esc`: Return a command that triggers `Back()` navigation (return to previous view)
  - `up/k`: Move cursor up in the agent list
  - `down/j`: Move cursor down in the agent list

### Step 4: Implement View() for rendering
- Render header: `SMITHERS › Agents` using lipgloss styling
- If loading: show a loading indicator
- If error: show error message
- If agents loaded: render a list of agents with name, role, status
- Highlight the currently selected agent (cursor position)
- Respect `width` and `height` constraints from `SetSize()`

### Step 5: Wire up navigation in the router
- Ensure `Navigate(ViewAgents)` passes the smithers client to `NewAgentsView()`
- Or: have the router hold a reference to the client and pass it through

### Step 6: Ensure command palette integration
- Verify that the dialog action `ActionNavigateAgents` properly triggers navigation to the agents view
- The existing `NavigateAction` and router code should handle this, but verify the command palette lists "Agents" as an option

## Key Files to Modify
1. **internal/ui/views/agents.go** — Main implementation (bulk of the work)
2. **internal/ui/views/router.go** — May need to pass client dependency to AgentsView
3. **internal/ui/model/ui.go** — May need minor updates for wiring

## Design References
- Design doc section 3.7 for layout specifications
- Follow existing patterns in the codebase for Bubble Tea views
- Use lipgloss for styling consistent with other views

## Testing Approach
- Unit test the View() output for header text
- Unit test Update() for Esc key producing back-navigation command
- Unit test cursor movement with up/down keys
- Integration: verify route /agents activates the correct view