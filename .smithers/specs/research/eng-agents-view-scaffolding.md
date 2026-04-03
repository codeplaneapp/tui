# Research: Agents View Scaffolding

## Existing Crush Surface
- **Data Model:** `internal/smithers/types.go` defines the `Agent` struct, which successfully merges fields from upstream detection (`Status`, `HasAuth`, `HasAPIKey`, `Usable`) and UI metadata (`Name`, `Command`, `Roles`).
- **Transport:** `internal/smithers/client.go` provides a `Client` with a `ListAgents()` method. The codebase currently returns a hardcoded placeholder list of 6 agents (Claude Code, Codex, Gemini, Kimi, Amp, Forge) with `Status: "unavailable"`.
- **Rendering & UX:** `internal/ui/views/agents.go` implements the `View` interface for an `AgentsView` struct. It currently renders a basic flat list showing the Name and Status with a `▸` cursor and "○ unavailable" status icons. It supports arrow navigation, `r` for refresh, and `esc` to return via a `PopViewMsg`.
- **Routing:** `internal/ui/views/router.go` manages the view stack with `Push`, `Pop`, and `Current` methods. `internal/ui/model/ui.go` introduces the `uiSmithersView` state, which bridges Crush's main state machine with the view router and forwards messages (e.g. `tea.KeyMsg`) and render passes (`m.viewRouter.Current().View()`) to the active view.

## Upstream Smithers Reference
- **Detection & Transport:** `../smithers/src/cli/agent-detection.ts` contains the logic for detecting agents dynamically on the host system. It uses `AgentAvailabilityStatus` (e.g., `likely-subscription`, `api-key`) and checks for specific binary presence (e.g., `command -v claude`) and auth signals (e.g., `~/.claude`).
- **Data Model (UI):** `../smithers/gui-ref/packages/shared/src/schemas/agent.ts` defines static CLI metadata in `agentCliSchema`, including fields like `logoProvider`.
- **Rendering & UX:** `../smithers/gui/src/ui/AgentsList.tsx` renders a very detailed, multi-group view. It splits agents into "Available" (usable agents) and "Not Detected" sections. It features robust visual styling, such as colored `StatusBadge` components (emerald for subscribed, amber for binary-only) and inline checks ("Binary found", "Authenticated").
- **Testing:** `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts` model a highly specific E2E terminal testing harness using a spawned sub-process with `TERM=xterm-256color` and ANSI-stripped text assertions (`waitForText`, `waitForNoText`, `sendKeys`).

## Gaps
1. **Data Model:** Crush's `Agent` struct (`types.go`) simplifies the data by merging `AgentAvailability` and `AgentCli` into a single type. This is structurally complete but lacks the live detection logic that the upstream uses (`agent-detection.ts`).
2. **Transport:** Crush's `ListAgents()` currently serves hardcoded data. It does not yet shell out to `smithers agents list --json` to acquire actual system state (as noted in the engineering spec).
3. **Rendering & UX:** Crush's `AgentsView` is a single, flat list that does not group agents by availability ("Available" vs. "Not Detected"). Furthermore, Crush lacks the rich UI badges for `Status`, `HasAuth`, `HasApiKey`, and `Roles` present in the upstream React component (`AgentsList.tsx`).
4. **Testing:** Crush currently lacks the unit tests, the custom Go-based E2E terminal testing harness modeled after the upstream TypeScript version, and the required VHS tape recording for visual verification.

## Recommended Direction
- Keep the `Agent` struct design as implemented in `types.go`, since merging the detection and display metadata serves the TUI well.
- **Transport:** Address the remaining work in the spec by updating `ListAgents()` in `internal/smithers/client.go` to optionally shell out to `smithers agents list --json` (via the existing `execSmithers` utility) and parse the JSON output into the `[]Agent` slice.
- **Routing:** Verify that the `/agents` route is fully integrated with the command palette (`internal/ui/dialog/commands.go`) so it correctly triggers the `ActionOpenAgentsView` action and pushes the view.
- **Testing (High Priority):** Implement unit tests for `AgentsView` logic and `ListAgents()` parsing. Crucially, build a Go E2E testing harness (e.g., `tests/tui/helpers_test.go`) mirroring `../smithers/tests/tui-helpers.ts` using `exec.Command` and a standard ANSI-stripped buffer poller. Finally, write a VHS tape (`tests/vhs/agents_view.tape`) that records a happy-path scenario (open command palette → `/agents` → enter → navigate up/down → escape).

## Files To Touch
- `internal/smithers/client.go` (Update `ListAgents` to handle `smithers agents list --json`)
- `internal/smithers/client_test.go` (Add tests for CLI shelling and JSON parsing)
- `internal/ui/views/agents.go` (Minor fixes to ensure it correctly presents the fetched dynamic data)
- `internal/ui/views/agents_test.go` (Add unit tests for initialization, update cycles, and rendering)
- `internal/ui/dialog/commands.go` (Verify or add `/agents` route mapping)
- `tests/tui/helpers_test.go` (Create the terminal E2E testing harness)
- `tests/tui/agents_e2e_test.go` (Create the specific `/agents` view E2E assertions)
- `tests/vhs/agents_view.tape` (Add the happy-path VHS recording)