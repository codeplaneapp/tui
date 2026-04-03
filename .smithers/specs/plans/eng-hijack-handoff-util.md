## Goal

Implement a reusable `HandoffToProgram` utility that wraps `tea.ExecProcess`. This function will cleanly suspend the Smithers TUI, hand full terminal control to an external CLI program (such as `claude-code`, `codex`, or an `$EDITOR`), and seamlessly resume the TUI when the external process exits. It provides the foundation for native TUI handoff features (like hijacking agent sessions and agent native chat) without requiring the TUI to reimplement agent-specific interfaces.

## Steps

1. **Implement Core Handoff Logic**: Create `internal/ui/util/handoff.go`. Add the exported `HandoffToProgram` and `HandoffWithCallback` functions. Create the `HandoffReturnMsg` struct and `buildCmd` helper. Include binary validation via `exec.LookPath` and environment merging.
2. **Add Unit Tests**: Create `internal/ui/util/handoff_test.go` and add unit tests validating `buildCmd` behavior (e.g., path resolution, environment merges, working directory propagation).
3. **Establish E2E Harness Base**: Setup the base terminal test harness modeled after upstream `@microsoft/tui-test` (from `../smithers/tests/tui-helpers.ts`). Create `tests/handoff.e2e.test.ts` to simulate sending keys and waiting for terminal screen updates.
4. **Implement VHS Tape**: Create `tests/vhs/handoff-happy-path.tape` to visually record the TUI suspension, external process run (e.g., `EDITOR=cat`), and resumption.
5. **Code Quality and Compilation**: Ensure the codebase complies with linters and tests pass successfully before declaring completion.

## File Plan

- `internal/ui/util/handoff.go`: (New File) Will contain the `HandoffToProgram` function, `HandoffWithCallback` fallback, `buildCmd` internal helper, and `HandoffReturnMsg` struct.
- `internal/ui/util/handoff_test.go`: (New File) Will contain unit tests (`TestBuildCmd_ValidBinary`, `TestBuildCmd_InvalidBinary`, `TestBuildCmd_EnvMerge`, etc.)
- `tests/handoff.e2e.test.ts`: (New File) Will contain the Playwright/TypeScript E2E tests validating terminal suspension and resumption modeled on the upstream harness.
- `tests/vhs/handoff-happy-path.tape`: (New File) VHS tape script recording the happy path of the handoff mechanism.

## Validation

- **Unit Tests**:
  ```bash
  go test ./internal/ui/util/ -run TestHandoff -v
  go test ./internal/ui/util/ -run TestBuildCmd -v
  ```
- **Lint & Build**:
  ```bash
  go build ./...
  go vet ./internal/ui/util/...
  golangci-lint run ./internal/ui/util/...
  ```
- **Terminal E2E Test**:
  Run the TypeScript test harness modeled on `@microsoft/tui-test` (from `../smithers/tests/tui-helpers.ts`):
  ```bash
  bun test tests/handoff.e2e.test.ts
  ```
  *(Must ensure TUI suspends, resumes, and properly shows an error if the binary is missing.)*
- **VHS Visual Test**:
  Execute the VHS tape to visually assert the handoff:
  ```bash
  vhs tests/vhs/handoff-happy-path.tape
  ```
- **Smoke Testing**:
  Manually run a minimal Bubble Tea program pointing to `HandoffToProgram` with a dummy process (`echo 'hello from child'; sleep 1`) to ensure stdout outputs correctly, TUI suspends, and TUI resumes cleanly.

## Open Questions

- **E2E Infrastructure Alignment**: Should we fully copy the `tui-helpers.ts` file from `../smithers/tests/` into this project's `tests/` directory, or will it be consumed as a shared npm package?
- **Environment Injection Scope**: Do we strictly need to merge parent `os.Environ()` across all handoff scenarios, or should some agents have tightly scoped/whitelisted environments to prevent bleeding unnecessary environment variables into the sub-process?
- **Test-Only Command Hook**: What is the preferred approach for injecting the test-only `/test-handoff` command for the E2E suite without exposing it in production builds? Should we use Go build tags?