## Goal
Deliver a regression-safe Prompts API client slice for Smithers mode in Crush by adding `ListPrompts`, `UpdatePromptSource`, and `RenderPromptPreview` to `internal/smithers`, then proving those paths through unit tests, a terminal E2E flow modeled on the upstream `tui-test` harness semantics, and a VHS happy-path recording.

## Steps
1. Lock the prompts contract from current upstream sources before coding.
   - Use `smithers_tmp/gui-src/ui/api/transport.ts` as the HTTP contract source for `GET /prompt/list`, `POST /prompt/update/:id`, and `POST /prompt/render/:id`.
   - Confirm payload shape from `smithers_tmp/src/cli/prompts.ts` and `smithers_tmp/src/cli/index.ts` (`DiscoveredPrompt`, `inputs[]`, and render result shape).
   - Treat prompts as file-backed operations (`.smithers/prompts`) with **no SQLite fallback**.
2. Add prompt domain types in `internal/smithers/types.go`.
   - Add `Prompt` and `PromptInput` to mirror upstream prompt discovery fields (`id`, `entryFile`, `source`, `inputs[{name,type,defaultValue}]`).
   - Add any small response helper structs needed to keep parsing explicit and stable.
3. Implement prompt client methods in `internal/smithers/client.go` with deterministic transport order.
   - Add `ListPrompts(ctx context.Context) ([]Prompt, error)`.
   - Add `UpdatePromptSource(ctx context.Context, id, source string) (*Prompt, error)`.
   - Add `RenderPromptPreview(ctx context.Context, id string, input map[string]any) (string, error)`.
   - Use HTTP first when `apiURL` is configured and server is reachable.
   - Fall back to `exec smithers prompt ...` for prompt list/update/render when HTTP is unavailable.
4. Make prompt parsing resilient to known transport variants.
   - Support expected envelope-unwrapped HTTP data (`prompts`, prompt object, render result) and normalize into Go return types.
   - For exec fallback, parse direct JSON shapes and preserve actionable errors when decode or command execution fails.
5. Add focused client unit tests before UI wiring.
   - Extend `internal/smithers/client_test.go` with HTTP success tests, exec fallback tests, request-body assertions (`source`, JSON-stringified render input), and malformed payload/error coverage.
6. Add a minimal prompts view scaffold only to exercise the new client methods in the TUI.
   - Add a lightweight `prompts` view that can: load prompt list, select a prompt, edit source, save, and trigger render preview.
   - Wire command-palette navigation so `/prompts` is reachable via keyboard flow.
   - Keep this intentionally thin to avoid overlap with full feature tickets (`feat-prompts-*`).
7. Wire Smithers client options from config for deterministic E2E.
   - Initialize `smithers.NewClient(...)` in UI with configured `smithers.apiUrl`, `smithers.apiToken`, and `smithers.dbPath` when present.
   - This allows E2E to target a mock HTTP server instead of relying on external CLI/runtime availability.
8. Add terminal E2E coverage using the existing Crush harness, explicitly aligned to upstream semantics.
   - Extend `internal/e2e` with a prompts API client scenario that uses `launchTUI`, `WaitForText`, `WaitForNoText`, `SendKeys`, `Snapshot`, and `Terminate` semantics matching `smithers_tmp/tests/tui-helpers.ts` + `tui.e2e.test.ts`.
   - Validate keyboard-driven flow: open prompts view, list prompts, save edited source, render preview.
9. Add one VHS happy-path recording for prompts.
   - Add a prompts-focused tape that demonstrates open prompts view, edit/save, render preview, and exit cleanly.
   - Keep fixture setup deterministic so the recording is stable across reruns.
10. Run full validation and document parity checks.
   - Verify unit tests, terminal E2E flow, and VHS recording all pass together to reduce regressions.

## File Plan
1. `/Users/williamcory/crush/internal/smithers/types.go`
2. `/Users/williamcory/crush/internal/smithers/client.go`
3. `/Users/williamcory/crush/internal/smithers/client_test.go`
4. `/Users/williamcory/crush/internal/ui/views/prompts.go` (new)
5. `/Users/williamcory/crush/internal/ui/views/prompts_test.go` (new)
6. `/Users/williamcory/crush/internal/ui/dialog/actions.go`
7. `/Users/williamcory/crush/internal/ui/dialog/commands.go`
8. `/Users/williamcory/crush/internal/ui/model/ui.go`
9. `/Users/williamcory/crush/internal/e2e/prompts_api_client_test.go` (new)
10. `/Users/williamcory/crush/internal/e2e/tui_helpers_test.go` (extend only if needed for parity)
11. `/Users/williamcory/crush/tests/vhs/prompts-api-client-happy-path.tape` (new)
12. `/Users/williamcory/crush/tests/vhs/README.md`
13. `/Users/williamcory/crush/tests/vhs/fixtures/` (new prompt/API fixture files as needed)

## Validation
1. `gofumpt -w internal/smithers internal/ui/views internal/ui/dialog internal/ui/model internal/e2e`
2. `go test ./internal/smithers -run 'Prompt|ListPrompts|UpdatePromptSource|RenderPromptPreview' -v`
3. `go test ./internal/ui/views -run Prompt -v`
4. `CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestPromptsAPIClient_TUI -count=1 -v -timeout 120s`
5. Terminal E2E parity check (required): confirm the Crush harness/test flow mirrors upstream `smithers_tmp/tests/tui-helpers.ts` and `smithers_tmp/tests/tui.e2e.test.ts` behavior for process launch, ANSI-normalized polling, key injection, failure snapshot capture, and termination cleanup.
6. `vhs tests/vhs/prompts-api-client-happy-path.tape`
7. `go test ./...`
8. Manual smoke check:
   - Run `go run .`.
   - Open command palette and navigate to Prompts.
   - Confirm prompt list loads, source edit/save works, render preview updates, and `Esc` returns to the previous view.

## Open Questions
1. For exec fallback, should prompts commands be invoked with explicit `--format json` on all subcommands, or should we rely on current default CLI JSON behavior for `prompt list/update/render`?
2. Should this API-client ticket include the minimal prompts view scaffold for E2E reachability, or should we add a temporary test-only entrypoint and keep all view work in `feat-prompts-*` tickets?
3. Should UI Smithers client initialization from config (`apiUrl/apiToken/dbPath`) be included here for deterministic prompt E2E, or deferred to `platform-config-namespace`?
4. Should prompt update/render parse tolerate both wrapped (`{ data: ... }`) and direct JSON in exec output to guard against CLI output drift across Smithers versions?
