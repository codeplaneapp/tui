## Goal
Implement a reusable in-terminal toast component for Smithers notifications in Crush, with bottom-right overlay rendering primitives, TTL lifecycle, and test coverage. This first pass keeps production behavior stable by building component and validation scaffolding first, then adding only minimal opt-in UI hooks needed for automated terminal verification.

## Steps
1. Lock the component contract from PRD/DESIGN/ENGINEERING/features and current Crush architecture (`UI` as sole model, imperative subcomponents). Define toast fields (`title`, `body`, `actions`, `level`, `ttl`, `id`), default TTL, and max visible count.
2. Add geometry support in `internal/ui/common` by introducing `BottomRightRect` and tests before component work, so layout math is validated independently.
3. Extend `internal/ui/styles/styles.go` with a `Toast` style group (container/title/body/actions and level colors mapped to semantic palette) to avoid hardcoded styles in component logic.
4. Create `internal/ui/components/notification.go` with `Toast`, `ToastLevel`, and `ToastManager` (`Add`, `Dismiss`, `Clear`, `Update`, `Draw`). Use `tea.Tick` for TTL expiry and deterministic time injection for tests.
5. Write focused unit tests in `internal/ui/components/notification_test.go` for stack limits, replacement behavior, TTL expiry, dismiss behavior, wrapping, and bottom-right draw placement.
6. Add a minimal, opt-in toast trigger path in `internal/ui/model` strictly for automated tests (for example env-gated debug toast command), keeping normal user flows unchanged.
7. Implement terminal E2E coverage in this repo using a harness modeled on upstream `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`: spawn TUI, `waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, terminate.
8. Add at least one VHS happy-path recording test that shows toast appearance and dismissal in Crush TUI with fixed terminal size/theme for deterministic output.
9. Run formatting/tests, then update ticket/spec notes with what shipped now versus deferred integration work (SSE/event-driven triggering and full overlay orchestration).

## File Plan
- `internal/ui/components/notification.go` (new): toast model, manager, TTL/update logic, draw logic.
- `internal/ui/components/notification_test.go` (new): unit tests for lifecycle, rendering, and bounds.
- `internal/ui/common/common.go`: add `BottomRightRect`.
- `internal/ui/common/common_test.go` (new): rectangle helper tests.
- `internal/ui/styles/styles.go`: add `Styles.Toast` definitions and defaults.
- `internal/ui/model/ui.go`: minimal opt-in test hook to surface toast in a real TUI process.
- `internal/ui/model/keys.go`: route toast dismiss key handling if needed by the E2E scenario.
- `tests/e2e/tui_helpers_test.go` (new): terminal harness helpers mirroring upstream test helper semantics.
- `tests/e2e/toast_notification_e2e_test.go` (new): end-to-end toast lifecycle test.
- `tests/vhs/toast_notification_happy_path.tape` (new): VHS happy-path capture for visual regression.
- `Taskfile.yaml`: add explicit tasks to run toast E2E and VHS checks.

## Validation
- `task fmt`
- `go test ./internal/ui/common ./internal/ui/components -count=1`
- `go test ./internal/ui/model -run Toast -count=1`
- `go test ./tests/e2e -run TestToastNotificationLifecycle -count=1`
- `vhs tests/vhs/toast_notification_happy_path.tape`
- Manual check: `CRUSH_TEST_TOAST_ON_START=1 go run .`, then verify (1) toast appears bottom-right without blocking typing, (2) action hints render, (3) dismiss key removes it, (4) auto-dismiss occurs at configured TTL.
- E2E requirement gate: the terminal test must explicitly use helper operations equivalent to upstream `@microsoft/tui-test` flow (`waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, `terminate`), and assert both appearance and dismissal.

## Open Questions
1. Confirm default TTL for this component: design doc says 30s; should component default be 30s or shorter with per-event overrides?
2. Confirm dismissal semantics: any keypress versus explicit keys only (for example Esc/action keys), given non-interrupting typing requirement.
3. Confirm toast stacking behavior for this ticket: single latest toast or bounded stack (for example max 3) before overlay integration ticket.
4. `../smithers/gui/src` and `../smithers/gui-ref` were not present in the local Smithers checkout used for reference; should implementation proceed with `../smithers/src` plus `../smithers/tests` as authoritative for behavior and harness patterns?
5. For follow-on notification event wiring, should completion events map to `RunFinished` (observed upstream server code) or `RunCompleted` (named in current engineering doc)?