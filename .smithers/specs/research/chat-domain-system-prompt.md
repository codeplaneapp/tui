## Existing Crush Surface
- Prompt data is generic and coding-oriented. [`internal/agent/prompt/prompt.go:21`](/Users/williamcory/crush/internal/agent/prompt/prompt.go:21) defines `Prompt` without Smithers fields; [`internal/agent/prompt/prompt.go:29`](/Users/williamcory/crush/internal/agent/prompt/prompt.go:29) `PromptDat` has no workflow/approval/MCP-server context.
- Embedded templates only include coder/task/initialize. [`internal/agent/prompts.go:11`](/Users/williamcory/crush/internal/agent/prompts.go:11) and [`internal/agent/templates`](/Users/williamcory/crush/internal/agent/templates) show no `smithers.md.tpl`.
- The active system prompt path is hardwired to coder. [`internal/agent/coordinator.go:115`](/Users/williamcory/crush/internal/agent/coordinator.go:115) loads `config.AgentCoder` and [`internal/agent/coordinator.go:121`](/Users/williamcory/crush/internal/agent/coordinator.go:121) calls `coderPrompt(...)`; model/tool refresh is also coder-pinned at [`internal/agent/coordinator.go:893`](/Users/williamcory/crush/internal/agent/coordinator.go:893).
- App startup is coder-specific. [`internal/app/app.go:525`](/Users/williamcory/crush/internal/app/app.go:525) uses `InitCoderAgent` and errors if coder config is missing.
- Config has only coder/task agent identities and setup. [`internal/config/config.go:61`](/Users/williamcory/crush/internal/config/config.go:61) and [`internal/config/config.go:513`](/Users/williamcory/crush/internal/config/config.go:513).
- UI/rendering is generic for MCP tools. [`internal/ui/chat/tools.go:257`](/Users/williamcory/crush/internal/ui/chat/tools.go:257) falls back to generic `mcp_` handling; [`internal/ui/chat/mcp.go:34`](/Users/williamcory/crush/internal/ui/chat/mcp.go:34) pretty-prints JSON but has no Smithers-specific run/approval renderer.
- UX remains Crush-branded and coder-assumed. [`internal/ui/model/header.go:43`](/Users/williamcory/crush/internal/ui/model/header.go:43) renders `CRUSH`; [`internal/ui/logo/logo.go:37`](/Users/williamcory/crush/internal/ui/logo/logo.go:37) renders Crush logo; [`internal/ui/model/ui.go:2992`](/Users/williamcory/crush/internal/ui/model/ui.go:2992) reports `coder agent is not initialized`.
- Smithers client exists but is not wired into prompt selection or rich run context. [`internal/ui/model/ui.go:332`](/Users/williamcory/crush/internal/ui/model/ui.go:332) creates `smithers.NewClient()` with defaults; [`internal/smithers/client.go:108`](/Users/williamcory/crush/internal/smithers/client.go:108) `ListAgents` currently returns stub data.
- Spec file for this ticket is stale in places. [`/.smithers/specs/engineering/chat-domain-system-prompt.md:20`](/Users/williamcory/crush/.smithers/specs/engineering/chat-domain-system-prompt.md:20) claims template/agent surfaces are missing, but they exist.

## Upstream Smithers Reference
- System-prompt authority is in `ask.ts`. [`../smithers/src/cli/ask.ts:19`](/Users/williamcory/smithers/src/cli/ask.ts:19) defines `SYSTEM_PROMPT`; [`../smithers/src/cli/ask.ts:34`](/Users/williamcory/smithers/src/cli/ask.ts:34) defines fallback prompt; prompt includes Smithers MCP behavior and repo-clone fallback.
- Prompt injection path is explicit. [`../smithers/src/agents/BaseCliAgent.ts:1218`](/Users/williamcory/smithers/src/agents/BaseCliAgent.ts:1218) stores `systemPrompt`; [`../smithers/src/agents/BaseCliAgent.ts:1239`](/Users/williamcory/smithers/src/agents/BaseCliAgent.ts:1239) combines/forwards it on each generation.
- Run lifecycle and approval semantics are exposed server-side. [`../smithers/src/server/serve.ts:100`](/Users/williamcory/smithers/src/server/serve.ts:100) SSE events; [`../smithers/src/server/serve.ts:160`](/Users/williamcory/smithers/src/server/serve.ts:160) approve; [`../smithers/src/server/serve.ts:175`](/Users/williamcory/smithers/src/server/serve.ts:175) deny; [`../smithers/src/server/serve.ts:197`](/Users/williamcory/smithers/src/server/serve.ts:197) waiting-approval cancel path.
- TUI surfaces emphasize approval/run-first operations. [`../smithers/src/cli/tui/components/RunsList.tsx:118`](/Users/williamcory/smithers/src/cli/tui/components/RunsList.tsx:118) approve/deny keys; [`../smithers/src/cli/tui/app.tsx:119`](/Users/williamcory/smithers/src/cli/tui/app.tsx:119) run-centric shell; v2 top bar tracks approvals/running states at [`../smithers/src/cli/tui-v2/client/components/TopBar.tsx:13`](/Users/williamcory/smithers/src/cli/tui-v2/client/components/TopBar.tsx:13).
- MCP naming in upstream ecosystem is Smithers-prefixed tool naming in several consumers. [`../smithers/src/pi-plugin/extension.ts:802`](/Users/williamcory/smithers/src/pi-plugin/extension.ts:802) registers `smithers_${tool.name}`.
- Requested E2E harness reference exists and is concrete. [`../smithers/tests/tui.e2e.test.ts:18`](/Users/williamcory/smithers/tests/tui.e2e.test.ts:18) and [`../smithers/tests/tui-helpers.ts:96`](/Users/williamcory/smithers/tests/tui-helpers.ts:96).
- Handoff doc explicitly calls for Playwright/TDD terminal testing. [`../smithers/docs/guides/smithers-tui-v2-agent-handoff.md:29`](/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md:29).
- `../smithers/gui/src` and `../smithers/gui-ref` were not present in this checkout; `gui` currently contains only reports/deps: [`../smithers/gui`](/Users/williamcory/smithers/gui).

## Gaps
- Data-model gap: Crush prompt structs cannot carry Smithers-mode context (workflow dir, MCP server identity, mode flag), so template-level Smithers behavior cannot be conditionally rendered from first-class fields.
- Transport/wiring gap: agent selection is coder-hardcoded in coordinator/app/config, so even with a new template there is no domain switch path for primary chat.
- Rendering gap: MCP tool output is generic; Smithers-specific result affordances (run tables, approval summaries, status cards) are missing.
- UX gap: branding/help/status language remains Crush/coder-centric, not Smithers operator-centric.
- Naming gap: Crush tool names are `mcp_<server>_<tool>` ([`internal/agent/tools/mcp-tools.go:59`](/Users/williamcory/crush/internal/agent/tools/mcp-tools.go:59)); upstream prompt/examples often reference `smithers_*` commands, so prompt wording must match actual exposed tool names in Crush.
- Test gap: no current Crush terminal E2E harness modeled on Smithers `tui.e2e` pattern, and no VHS happy-path recording coverage yet.

## Recommended Direction
- Add a dedicated Smithers prompt template and constructor first, then wire a primary-agent resolver instead of hardcoded coder prompt selection.
- Extend prompt data minimally for domain gating: `SmithersMode`, `SmithersWorkflowDir`, `SmithersMCPServer`. Keep active-run/pending-approval snapshots for follow-up tickets.
- Add `AgentSmithers` and a config gate so `SetupAgents()` can register Smithers mode cleanly without breaking existing coder/task flows.
- Keep task sub-agent behavior unchanged for now (`taskPrompt`) to reduce risk.
- In prompt text, document both preferred MCP use and fallback CLI behavior, but align tool names with what Crush actually exposes to the model (`mcp_*` naming).
- Testing plan for this ticket set:
  - Unit: prompt rendering and coordinator selection.
  - Terminal E2E: add a harness patterned after [`../smithers/tests/tui.e2e.test.ts`](/Users/williamcory/smithers/tests/tui.e2e.test.ts) and [`../smithers/tests/tui-helpers.ts`](/Users/williamcory/smithers/tests/tui-helpers.ts).
  - Recording: add at least one VHS-style happy-path TUI flow as required by [`docs/smithers-tui/03-ENGINEERING.md:941`](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md:941).

## Files To Touch
- Primary ticket implementation:
- [`internal/agent/templates/smithers.md.tpl`](/Users/williamcory/crush/internal/agent/templates/smithers.md.tpl) (new).
- [`internal/agent/prompts.go`](/Users/williamcory/crush/internal/agent/prompts.go).
- [`internal/agent/prompt/prompt.go`](/Users/williamcory/crush/internal/agent/prompt/prompt.go).
- [`internal/agent/coordinator.go`](/Users/williamcory/crush/internal/agent/coordinator.go).
- [`internal/app/app.go`](/Users/williamcory/crush/internal/app/app.go).
- [`internal/config/config.go`](/Users/williamcory/crush/internal/config/config.go).
- Validation/tests to add or update:
- [`internal/config/agent_id_test.go`](/Users/williamcory/crush/internal/config/agent_id_test.go).
- [`internal/config/load_test.go`](/Users/williamcory/crush/internal/config/load_test.go).
- [`internal/agent/coordinator_test.go`](/Users/williamcory/crush/internal/agent/coordinator_test.go).
- [`internal/agent/prompt/prompt_test.go`](/Users/williamcory/crush/internal/agent/prompt/prompt_test.go) (new).
- Terminal E2E + recording follow-through expected by planning docs:
- `tests/tui.e2e.test.ts` (new, Smithers-harness pattern).
- `tests/tui-helpers.ts` (new harness utilities).
- `tests/vhs/smithers-happy-path.tape` (new VHS scenario).

First research pass complete. No code changes were made.