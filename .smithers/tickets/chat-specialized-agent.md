# Specialized Agent Configuration

## Metadata
- ID: chat-specialized-agent
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_SPECIALIZED_AGENT
- Dependencies: chat-workspace-context

## Summary

Configure Smithers mode so the default agent uses Smithers-specific prompts and MCP tooling out of the box, while excluding irrelevant built-in tools.

## Acceptance Criteria

- Smithers agent defaults exclude irrelevant tools (`sourcegraph`, `multiedit`).
- Smithers agent is wired to Smithers MCP tools without requiring manual MCP config bootstrap.

## Source Context

- `internal/config/load.go`
- `internal/config/config.go`
- `internal/agent/coordinator.go`
- `internal/e2e/tui_helpers_test.go`
- `../smithers/tests/tui-helpers.ts`
- `../smithers/tests/tui.e2e.test.ts`

## Implementation Notes

- `internal/config/defaults.go` from the Smithers design doc does not exist in this fork; defaulting is implemented in `internal/config/load.go`.
- `SetupAgents` already creates a Smithers agent and excludes `sourcegraph`/`multiedit`; this ticket focuses on default MCP bootstrap and drift-proof prompt wiring.
- Upstream Smithers CLI currently serves MCP via `smithers --mcp` (`../smithers/src/cli/index.ts`), so default MCP args should follow that implementation unless compatibility requirements say otherwise.

## Goal

Deliver `CHAT_SMITHERS_SPECIALIZED_AGENT` so Smithers mode reliably boots into a specialized chat agent profile: Smithers prompt selected, irrelevant built-in tools excluded, and Smithers MCP tools auto-available without manual MCP bootstrap.

## Steps

1. Add regression-first config coverage in `internal/config/load_test.go` for Smithers MCP defaults:
   - `Smithers` config present + missing `mcp["smithers"]` seeds a default stdio MCP entry (`command: "smithers"`, `args: ["--mcp"]`).
   - Existing `mcp["smithers"]` entries are preserved exactly (append-only behavior, no user override loss).
2. Implement Smithers MCP auto-seeding in `internal/config/load.go` inside `setDefaults`:
   - Only run when `c.Smithers != nil`.
   - Only seed when `c.MCP["smithers"]` is absent.
   - Keep current MCP timeout semantics (15s effective default via MCP runtime fallback).
3. Keep Smithers agent specialization locked in config tests:
   - Extend `internal/config/load_test.go` and `internal/config/agent_id_test.go` assertions so Smithers agent always excludes `sourcegraph` and `multiedit` and keeps `AllowedMCP["smithers"]`.
4. Remove prompt wiring drift by updating coordinator Smithers prompt setup:
   - In `internal/agent/coordinator.go`, derive the Smithers MCP server name from Smithers agent config (`AllowedMCP`) with deterministic fallback to `"smithers"`.
   - Add focused tests in `internal/agent/coordinator_test.go` for that helper and prompt-option input.
5. Upgrade terminal E2E harness in `internal/e2e/tui_helpers_test.go` to PTY-backed execution while preserving the upstream `@microsoft/tui-test`-style API surface used by `../smithers/tests/tui-helpers.ts` and `../smithers/tests/tui.e2e.test.ts`:
   - `waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, `terminate`.
   - Snapshot capture on failure.
6. Add `internal/e2e/chat_specialized_agent_test.go` for specialized-agent navigation flow:
   - Use `SMITHERS_TUI_E2E=1`, `OPENAI_API_KEY=dummy`, `SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE=1`.
   - Write temp config/data roots via `SMITHERS_TUI_GLOBAL_CONFIG` and `SMITHERS_TUI_GLOBAL_DATA`.
   - Launch TUI, open command palette (`Ctrl+P`), navigate into `Agents`/`Approvals`/`Tickets`, verify view headers render, then `Esc` back.
7. Add a VHS happy-path recording for startup + Smithers view navigation:
   - Create `tests/vhs/smithers-specialized-agent.tape`.
   - Add `tests/vhs/fixtures/smithers-tui.json`.
   - Update `tests/vhs/README.md` with run instructions.
8. Run validation in dependency order to minimize rework: config tests -> coordinator tests -> terminal E2E -> VHS -> full regression.

## File Plan

- `/Users/williamcory/crush/internal/config/load.go`
- `/Users/williamcory/crush/internal/config/load_test.go`
- `/Users/williamcory/crush/internal/config/agent_id_test.go`
- `/Users/williamcory/crush/internal/agent/coordinator.go`
- `/Users/williamcory/crush/internal/agent/coordinator_test.go`
- `/Users/williamcory/crush/internal/e2e/tui_helpers_test.go`
- `/Users/williamcory/crush/internal/e2e/chat_specialized_agent_test.go` (new)
- `/Users/williamcory/crush/tests/vhs/smithers-specialized-agent.tape` (new)
- `/Users/williamcory/crush/tests/vhs/fixtures/smithers-tui.json` (new)
- `/Users/williamcory/crush/tests/vhs/README.md`
- `/Users/williamcory/crush/go.mod` (if PTY test dependency is added)
- `/Users/williamcory/crush/go.sum` (if PTY test dependency is added)
- `/Users/williamcory/crush/.smithers/tickets/chat-specialized-agent.md`

## Validation

- Config + Smithers agent defaults:
  - `go test ./internal/config -run 'TestConfig_setDefaultsWithSmithers|TestConfig_setDefaults.*Smithers.*MCP|TestConfig_setupAgentsWithSmithers|TestConfig_AgentIDsWithSmithers' -count=1`
- Coordinator Smithers prompt wiring:
  - `go test ./internal/agent -run 'TestCoordinatorResolveAgent|Test.*Smithers.*MCP.*Server' -count=1`
- Terminal E2E coverage (modeled on upstream harness behavior in `../smithers/tests/tui-helpers.ts` + `../smithers/tests/tui.e2e.test.ts`):
  - `OPENAI_API_KEY=dummy SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE=1 SMITHERS_TUI_E2E=1 go test ./internal/e2e -run TestChatSpecializedAgent_TUI -count=1`
  - Required checks: `waitForText` + `sendKeys` command-palette flow, Smithers view entry, `Esc` return, snapshot persisted on failure.
- VHS happy-path recording:
  - `vhs tests/vhs/smithers-specialized-agent.tape`
  - Verify tape uses `SMITHERS_TUI_GLOBAL_CONFIG` + `SMITHERS_TUI_GLOBAL_DATA` and shows startup -> command palette -> Smithers view -> return flow.
- Full regression check:
  - `go test ./...`
- Manual smoke:
  - `OPENAI_API_KEY=dummy SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE=1 SMITHERS_TUI_GLOBAL_CONFIG=/tmp/smithers-cfg SMITHERS_TUI_GLOBAL_DATA=/tmp/smithers-data go run .`
  - In TUI: `Ctrl+P` -> `Agents`/`Approvals`/`Tickets` -> `Esc`, and verify startup succeeds with auto-seeded `mcp.smithers`.

## Open Questions

1. Should we support an explicit fallback MCP launch arg set (for older CLIs) if `smithers --mcp` is unavailable, or keep this ticket pinned to current upstream behavior only?
2. For CI, should specialized-agent E2E require a real `smithers` binary on `PATH`, or should tests stub/mask MCP startup and only validate prompt/agent/view wiring?
3. `../smithers/gui/src`, `../smithers/gui-ref`, and `../smithers/docs/guides/smithers-tui-v2-agent-handoff.md` are absent in this checkout; confirm `../smithers/src` plus `../smithers/tests` are the authoritative references for this ticket.
