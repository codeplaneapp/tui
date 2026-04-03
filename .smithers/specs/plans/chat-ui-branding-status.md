## Goal
Deliver the `chat-ui-branding-status` ticket by replacing Crush header branding with Smithers branding and adding a nil-safe Smithers status data surface (active runs, pending approvals, MCP connection) in header/status UI components, while keeping behavior unchanged when no Smithers data is provided.

## Steps
1. Lock scope and reference mapping to `PLATFORM_SMITHERS_REBRAND` plus status slots (`CHAT_SMITHERS_ACTIVE_RUN_SUMMARY`, `CHAT_SMITHERS_PENDING_APPROVAL_SUMMARY`, `CHAT_SMITHERS_MCP_CONNECTION_STATUS`), and use [TopBar.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/TopBar.tsx), [TuiAppV2.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/app/TuiAppV2.tsx), and [store.ts](/Users/williamcory/smithers/src/cli/tui-v2/client/state/store.ts) as current implementation references.
2. Rebrand static header surfaces first by updating [logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go) to render `SMITHERS` in the existing half-block pipeline, while preserving stretch/randomization and truncation behavior.
3. Update compact and small branding strings in [logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go) and [header.go](/Users/williamcory/crush/internal/ui/model/header.go) so `Charm™ CRUSH` and `Crush` are removed and replaced with Smithers naming.
4. Apply the Smithers palette through semantic style tokens in [styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go) by changing primary/secondary/focus/logo token assignments rather than hardcoding per-component colors.
5. Introduce and plumb a shared optional `SmithersStatus` model across [header.go](/Users/williamcory/crush/internal/ui/model/header.go), [status.go](/Users/williamcory/crush/internal/ui/model/status.go), and [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go), with a default `nil` path for backward-compatible rendering.
6. Extend header/status rendering to conditionally append Smithers runtime segments only when `SmithersStatus` is present, keeping existing help and info-message precedence unchanged.
7. Add focused unit coverage for branding text, Smithers status rendering (nil and populated paths), and style token regressions before adding end-to-end tests.
8. Add terminal E2E coverage modeled on [tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts) and [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts), then add one VHS happy-path tape and a Task target for reproducible local/CI execution.

## File Plan
- Update [logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go).
- Update [header.go](/Users/williamcory/crush/internal/ui/model/header.go).
- Update [status.go](/Users/williamcory/crush/internal/ui/model/status.go).
- Update [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go).
- Update [styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go).
- Add [logo_test.go](/Users/williamcory/crush/internal/ui/logo/logo_test.go).
- Add [header_test.go](/Users/williamcory/crush/internal/ui/model/header_test.go).
- Add [status_test.go](/Users/williamcory/crush/internal/ui/model/status_test.go).
- Add [styles_test.go](/Users/williamcory/crush/internal/ui/styles/styles_test.go).
- Add terminal E2E helper/test files under `/Users/williamcory/crush/internal/ui/model` (for example `tui_e2e_helpers_test.go` and `tui_branding_e2e_test.go`).
- Add VHS tape under `/Users/williamcory/crush/internal/ui/testdata/vhs` (for example `branding.tape`).
- Update [Taskfile.yaml](/Users/williamcory/crush/Taskfile.yaml) with a VHS execution task.

## Validation
1. Build and unit suites: `go build .` and `go test ./internal/ui/logo ./internal/ui/model ./internal/ui/styles`.
2. Terminal E2E harness modeled on upstream `tui-test` flow: `go test ./internal/ui/model -run TestTUIBrandingE2E -count=1`; verify `SMITHERS` appears, `CRUSH` and `Charm™` are absent, and `Ctrl+C` exits cleanly.
3. VHS happy-path recording: `command -v vhs` and `vhs internal/ui/testdata/vhs/branding.tape`; verify the generated artifact shows Smithers-branded startup without crash.
4. Manual regression checks: run `go run .`, confirm wide-mode ASCII Smithers branding at `>=120` columns, confirm compact branding below `120` columns, and confirm Smithers status segments truncate safely when test-injected.

## Open Questions
- The requested upstream paths `../smithers/gui/src` and `../smithers/gui-ref` are not present in the current checkout. Should implementation references use `/Users/williamcory/crush/smithers_tmp/gui-src` and `/Users/williamcory/crush/smithers_tmp/gui-ref` as fallback historical sources?
- Should this ticket derive MCP connectivity immediately from existing `mcpStates` in [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go), or keep `SmithersStatus` strictly write-only for downstream tickets to populate?
- Should terminal E2E and VHS assets stay co-located under `internal/ui/...` (recommended for Go conventions), or should a new top-level `tests/` tree be created for all future Smithers TUI E2E work?
- Do we want fixed hex palette values now, or tokenized Smithers theme constants in `styles` to reduce churn across upcoming branding and navigation tickets?
