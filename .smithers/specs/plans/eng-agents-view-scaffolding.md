# Implementation Plan: eng-agents-view-scaffolding

## Goal
Implement the structural boilerplate for the Agents view and establish the internal client method to fetch real agent data from the Smithers CLI, integrating it fully into the Crush-based Smithers TUI. This ensures users can navigate to the `/agents` view via the command palette, see a dynamically fetched list of available agents, and have robust E2E testing infrastructure validating the workflow.

## Steps
1. **Transport Layer Implementation**:
   - Update the existing `ListAgents()` method in `internal/smithers/client.go` to shell out to the `smithers agents list --json` command.
   - Parse the resulting JSON into the existing `[]Agent` struct slice.
   - Implement graceful fallback and error handling (e.g., if the CLI is not found or JSON parsing fails, either return an error or a fallback hardcoded list).

2. **View Refinement**:
   - Verify and adjust `internal/ui/views/agents.go` so it properly handles the newly dynamic `ListAgents()` output.
   - Ensure the UI correctly processes the `agentsLoadedMsg` and `agentsErrorMsg`, accurately rendering loading and error states for the CLI command execution.

3. **Routing Integration**:
   - Confirm that `internal/ui/dialog/commands.go` has the `/agents` command registered.
   - Verify that selecting the command dispatches `ActionOpenAgentsView` and the router properly pushes the view onto the stack in `internal/ui/model/ui.go`.

4. **Testing Infrastructure (Go E2E)**:
   - Create a Go-based E2E terminal testing harness in `tests/tui/helpers_test.go` mirroring the upstream TypeScript approach in `../smithers/tests/tui-helpers.ts`. This involves spawning the TUI via `exec.Command` with `TERM=xterm-256color`, buffering stdout, stripping ANSI codes, polling for text presence, and sending keys via stdin.
   - Create `tests/tui/agents_e2e_test.go` utilizing this harness to validate the full navigation round trip (launch -> open command palette -> type "/agents" -> check rendered text -> escape).

5. **VHS Testing**:
   - Create a `tests/vhs/agents_view.tape` file to record a happy-path scenario visually, providing reproducible visual regression coverage.

6. **Unit Tests**:
   - Write unit tests for `client_test.go` specifically covering the new `ListAgents()` shell-out parsing logic (mocking the CLI execution output).
   - Add view-level tests in `agents_test.go` for the Bubble Tea initialization, command generation, and update cycles.

## File Plan
- `internal/smithers/client.go` (Modify: update `ListAgents` to execute `smithers agents list --json`)
- `internal/smithers/client_test.go` (Modify: add JSON parsing and shell-out unit tests for `ListAgents`)
- `internal/ui/views/agents.go` (Modify: minor adjustments to ensure dynamic data consumption and error/loading states match expected behavior)
- `internal/ui/views/agents_test.go` (Create: add unit tests for the Bubble Tea model `Init`, `Update`, `View` loops)
- `internal/ui/dialog/commands.go` (Modify: ensure `/agents` is properly registered in the command palette list)
- `tests/tui/helpers_test.go` (Create: Terminal E2E test harness mimicking `../smithers/tests/tui-helpers.ts`)
- `tests/tui/agents_e2e_test.go` (Create: Specific E2E test for the agents view navigation, data rendering, and routing return)
- `tests/vhs/agents_view.tape` (Create: VHS tape for visual happy-path validation)

## Validation
- **Compilation**: Run `go build ./...` to ensure all structural changes compile cleanly.
- **Unit Testing**: Run `go test ./internal/smithers/... ./internal/ui/views/... -v` to validate the client JSON parsing and Bubble Tea view state transitions.
- **E2E Terminal Harness**: Run `go test ./tests/tui/... -run TestAgentsViewNavigation -timeout 30s -v`. This explicitly covers the terminal E2E requirement modeled on the upstream `@microsoft/tui-test` harness. It must verify that the TUI launches, `/agents` navigates to the view, agent status is rendered, arrow keys move the cursor, and Esc pops the view back to chat.
- **VHS Recording**: Run `vhs tests/vhs/agents_view.tape` and verify that the generated `tests/vhs/agents_view.gif` exists and is non-empty.
- **Manual Check**: 
  1. `go run .`
  2. Press `/` to open the command palette.
  3. Type `agents` and press Enter.
  4. Verify that the Agents view appears, displays the live list from `smithers agents list --json`.
  5. Press `r` to trigger a refresh.
  6. Press `Esc` to return to the chat view.

## Open Questions
- Should `ListAgents()` implement a strict timeout (e.g., 2-3 seconds) to prevent the TUI view initialization from hanging indefinitely if the host's `smithers` CLI is slow to respond?
- If the host system does not have `smithers` installed in its path, should the `ListAgents()` client fall back to returning the hardcoded mock list of 6 unavailable agents, or should it explicitly render a "Smithers CLI not found" error state in the view?