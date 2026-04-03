# Implementation Summary: chat-domain-system-prompt

- Ticket: Smithers Domain System Prompt
- Group: Chat And Console (chat-and-console)

## Summary

Implemented `chat-domain-system-prompt` and committed as `19caede3` (branch pointer `impl/chat-domain-system-prompt` updated to this commit).

What was implemented:
- Added Smithers system prompt template and embed wiring.
- Added Smithers prompt options/data fields (`WithSmithersMode`, `SmithersMode`, `SmithersWorkflowDir`, `SmithersMCPServer`).
- Added Smithers config/agent support (`AgentSmithers`, `SmithersConfig`, conditional agent registration, Smithers defaults in config loading).
- Updated coordinator to resolve primary agent (Smithers-first when configured) and stop hardcoding coder in model/tool refresh.
- Added unit tests for prompt rendering/plumbing, config defaults/agent setup, and coordinator agent resolution.
- Added Smithers prompt golden snapshot test data.
- Added terminal E2E harness scaffolding (upstream-style wait/send/snapshot/terminate) and gated E2E tests.
- Added VHS happy-path tape + fixture + README.

Validation run:
- `go test ./internal/agent/... ./internal/config/... -count=1` passed
- `go test ./internal/e2e -run TestSmithersDomainSystemPrompt -count=1` passed (tests are env-gated)
- `go build ./...` passed
- `vhs tests/vhs/smithers-domain-system-prompt.tape` passed

## Files Changed

- internal/agent/coordinator.go
- internal/agent/coordinator_test.go
- internal/agent/prompt/prompt.go
- internal/agent/prompt/prompt_test.go
- internal/agent/prompts.go
- internal/agent/prompts_test.go
- internal/agent/templates/smithers.md.tpl
- internal/agent/testdata/smithers_prompt.golden
- internal/config/agent_id_test.go
- internal/config/config.go
- internal/config/load.go
- internal/config/load_test.go
- internal/e2e/chat_domain_system_prompt_test.go
- internal/e2e/tui_helpers_test.go
- tests/vhs/README.md
- tests/vhs/fixtures/crush.json
- tests/vhs/smithers-domain-system-prompt.tape

## Validation

- CRUSH_UPDATE_GOLDEN=1 go test ./internal/agent -run TestSmithersPromptSnapshot -count=1
- go test ./internal/agent/... ./internal/config/... -count=1
- go test ./internal/e2e -run TestSmithersDomainSystemPrompt -count=1
- go build ./...
- vhs tests/vhs/smithers-domain-system-prompt.tape

## Follow Up

- Run `CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestSmithersDomainSystemPrompt -count=1` in an interactive-capable environment to execute the gated terminal scenarios.
- Decide whether `tests/vhs/output/*` artifacts should be committed or ignored in this repo state.
