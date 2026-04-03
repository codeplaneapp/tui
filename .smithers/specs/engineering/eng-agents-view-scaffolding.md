# Engineering: Scaffolding for Agents View

## Metadata
- ID: eng-agents-view-scaffolding
- Group: Agents (agents)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Create the structural boilerplate for the Agents view and establish the internal client method to fetch agent data from the Smithers CLI.

## Acceptance Criteria

- Create internal/ui/views/agents.go implementing the base View interface.
- Add a ListAgents() stub to internal/smithers/client.go.
- Register the /agents route in the main view router so it can be navigated to.

## Source Context

- internal/ui/views/router.go
- internal/smithers/client.go
- internal/ui/model/ui.go

## Implementation Notes

- Use Crush's existing view pattern: each view implements Init(), Update(), View(), and ShortHelp() from the View interface defined in internal/ui/views/router.go.
- The AgentsView struct should hold a reference to the SmithersClient for data fetching.
- ListAgents() in the client should shell out to `smithers agents list --json` and parse the JSON output into []Agent structs.
- The Agent type should be defined in internal/smithers/types.go with fields: Name, Status, Role, BinaryPath.
- Register the route in the ViewRouter's RouteMap so navigation via command palette or keyboard shortcut works.
- The view should render a placeholder table layout that downstream feature tickets (agent status, role display, etc.) will populate.

## Existing State

- internal/ui/views/agents.go already exists with a basic AgentsView struct implementing the View interface. It includes:
  - AgentsView struct with fields: client, agents, table, loading, err, width, height
  - NewAgentsView constructor
  - Init() that dispatches a fetchAgentsMsg command
  - Update() handling fetchAgentsMsg, agentsResultMsg, and KeyMsg (with 'r' for refresh)
  - View() rendering a table with Name, Status, Role columns
  - ShortHelp() returning help bindings
- internal/smithers/client.go already has a ListAgents() method that calls `smithers agents list --json`
- internal/smithers/types.go already defines the Agent type with Name, Status, Role, BinaryPath fields
- internal/ui/views/router.go already has a ViewRouter with RouteMap registration pattern

## Remaining Work

- Verify the agents route is registered in the ViewRouter's RouteMap (connect AgentsView to the router).
- Add unit tests for AgentsView initialization, update cycle, and rendering.
- Add unit tests for ListAgents() client method (mock smithers CLI output).
- Ensure error states (smithers not found, parse failure) are handled gracefully in the view.

## E2E Test Outline

```typescript
import { TUIHarness } from "./harness";

describe("Agents View Scaffolding", () => {
  test("navigates to agents view and renders table", async () => {
    const tui = new TUIHarness();
    await tui.launch();
    try {
      // Navigate to agents view
      tui.sendKeys(":"); // Open command palette
      await tui.waitForText("command");
      tui.type("agents");
      tui.sendKeys("\r");
      
      // Verify table headers render
      await tui.waitForText("Name");
      await tui.waitForText("Status");
      await tui.waitForText("Role");
      
      // Verify refresh keybinding works
      tui.sendKeys("r");
      await tui.waitForText("Name"); // Table re-renders after refresh
    } catch (err) {
      require("fs").writeFileSync("tui-buffer.txt", tui.snapshot());
      throw err;
    } finally {
      await tui.terminate();
    }
  }, 15000);
});
```