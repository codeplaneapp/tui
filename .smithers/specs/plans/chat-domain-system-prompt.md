## Goal
Implement `CHAT_SMITHERS_DOMAIN_SYSTEM_PROMPT` by loading a Smithers-specific system prompt for the primary chat agent in Smithers mode, while preserving current coder/task behavior as fallback. The prompt must explicitly cover run-table formatting, proactive pending-approval mentions, and MCP-first Smithers operations aligned to Crush tool naming (`mcp_<server>_<tool>`).

## Steps
1. Lock the activation contract and backward-compatibility path: use config-gated Smithers mode (`config.smithers`) plus `AgentSmithers`, but keep `AgentCoder` and `AgentTask` registration so current UI/model lookups do not regress.
2. Add `internal/agent/templates/smithers.md.tpl` with Smithers domain instructions, env/context blocks, and conditional workflow metadata; keep it intentionally narrow (orchestrator behavior, not coder editing rules).
3. Extend prompt data plumbing in `internal/agent/prompt/prompt.go` with additive Smithers fields (`SmithersMode`, `SmithersWorkflowDir`, `SmithersMCPServer`) and a `WithSmithersMode(...)` option; do not change existing coder fields.
4. Register the new prompt constructor in `internal/agent/prompts.go` (`smithersPromptTmpl` + `smithersPrompt(...)`), matching the existing embed/new-prompt pattern.
5. Add Smithers config and agent registration in `internal/config/config.go` (new `AgentSmithers`, `SmithersConfig`, and `SetupAgents` conditional injection), and set safe defaults in `internal/config/load.go` for workflow/db paths only when Smithers config is present.
6. Update coordinator selection in `internal/agent/coordinator.go` so primary agent resolution prefers Smithers when configured, and update model/tool refresh logic to use the resolved current agent instead of hardcoded `AgentCoder`.
7. Add unit coverage before/with refactor: prompt rendering tests, config agent-setup tests, and coordinator primary-agent resolution tests; include a golden/snapshot for rendered Smithers prompt.
8. Add terminal E2E and VHS coverage last: create a harness modeled on upstream `tests/tui.e2e.test.ts` + `tests/tui-helpers.ts` (`launchTUI`, wait/send/snapshot/terminate semantics), then add one Smithers happy-path VHS recording test and wire both into repeatable commands.

## File Plan
- `internal/agent/templates/smithers.md.tpl` (new)
- `internal/agent/prompts.go`
- `internal/agent/prompt/prompt.go`
- `internal/agent/coordinator.go`
- `internal/config/config.go`
- `internal/config/load.go`
- `internal/agent/prompt/prompt_test.go` (new)
- `internal/agent/coordinator_test.go`
- `internal/config/agent_id_test.go`
- `internal/config/load_test.go`
- `internal/e2e/tui_helpers_test.go` (new; harness patterned after upstream helpers)
- `internal/e2e/chat_domain_system_prompt_test.go` (new; terminal E2E)
- `tests/vhs/smithers-domain-system-prompt.tape` (new)
- `tests/vhs/README.md` or `Taskfile.yaml` entry for reproducible VHS invocation

## Validation
- `go test ./internal/agent/... ./internal/config/... -count=1`
- `go test ./internal/agent/prompt -run TestSmithersPrompt -v -count=1`
- `go test ./internal/agent -run TestCoordinatorPrimaryAgent -v -count=1`
- `go test ./internal/e2e -run TestSmithersDomainSystemPrompt_TUI -v -count=1`
- `go build ./...`
- `task test`
- `vhs tests/vhs/smithers-domain-system-prompt.tape`

Terminal E2E coverage (modeled on upstream `@microsoft/tui-test` pattern in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`):
- Harness methods mirror upstream flow: launch process, poll for text, send key bytes, snapshot buffer, terminate process.
- Scenario 1: launch with Smithers-enabled config, assert TUI boots, send chat input, and assert captured request/behavior proves Smithers prompt path was used.
- Scenario 2: launch without Smithers config, assert coder fallback path remains active.
- Scenario 3: capture snapshot on failure for debugging parity with upstream harness.

VHS happy-path coverage in this repo:
- Record one tape that launches the TUI with Smithers config, sends a single run-status style prompt, and captures startup + response frames.
- Verify the tape exits successfully and produces output artifacts under `tests/vhs/output`.

Manual checks:
- Run `CRUSH_GLOBAL_CONFIG=<tmp-config-dir> CRUSH_GLOBAL_DATA=<tmp-data-dir> go run .` with Smithers config and confirm the first assistant turn is Smithers-domain (runs/approvals/MCP), not coding-rule boilerplate.
- Remove the Smithers config block and re-run to confirm coder fallback still initializes.

## Open Questions
- Should Smithers-mode activation be strictly `config.smithers` driven, or should presence of an MCP server named `smithers` also auto-enable the Smithers prompt?
- Which tool names should be canonical in the prompt text for this repo: upstream `smithers_*` labels or exact Crush-exposed `mcp_smithers_*` names (recommended)?
- Where should long-running terminal E2E live for CI stability (`internal/e2e` vs a new top-level `tests/e2e`) given current repo layout and Go `internal` import rules?
- In the current local upstream checkout, `../smithers/gui/src` and `../smithers/gui-ref` are not both present; should plan validation standardize on `~/smithers` plus `crush/smithers_tmp/gui-ref` for historical-reference-only checks?