## Goal
Fork the existing Crush application and completely rebrand it to the Smithers TUI. This involves renaming the Go module, updating the binary name to `smithers-tui`, replacing the Crush ASCII art with "SMITHERS" branding, modifying the terminal color scheme to the Smithers brand palette (cyan/green/magenta), and moving the configuration namespace to `.smithers-tui`. Comprehensive testing, including a terminal E2E test modeled on upstream standards and a VHS happy-path recording, will ensure stability and visual fidelity.

## Steps
1. **Module Renaming & Imports:**
   - Execute `go mod edit -module github.com/anthropic/smithers-tui` in the root directory.
   - Perform a global find-and-replace to change all `github.com/charmbracelet/crush` imports to `github.com/anthropic/smithers-tui` across the codebase.
   - Run `go mod tidy` to clean up dependencies.

2. **Config Namespace & Environment Updates:**
   - Modify `internal/config/config.go` to set `appName = "smithers-tui"` and `defaultDataDirectory = ".smithers-tui"`.
   - Update config file generation to produce `smithers-tui.json` instead of `crush.json`.
   - In `internal/cmd/root.go` and `internal/config/config.go`, replace environment variable prefixes from `CRUSH_` to `SMITHERS_` (e.g., `SMITHERS_PROFILE`).

3. **Logo Swap & Copy Updates:**
   - Rewrite `internal/ui/logo/logo.go`. Remove the Crush-specific letterform stretching logic and implement a simplified, static ASCII representation of "SMITHERS".
   - Replace the hardcoded `heartbit` ASCII string in `internal/cmd/root.go` to match the Smithers brand.
   - Update CLI descriptions, such as changing "Charm CRUSH" to "Smithers TUI" in `internal/cmd/root.go`.

4. **Color Palette Alignment:**
   - Update `internal/ui/styles/styles.go` to align with the Smithers design system. Replace the existing `charmtone.Charple` (purple) and `charmtone.Dolly` (yellow) palettes with cyan and magenta variants, while retaining green/cyan for success and info states.

5. **Testing Implementation:**
   - Create `tests/tui_e2e_test.go` to introduce a Go-based E2E harness (using `teatest` or similar PTY testing framework) to replicate the terminal buffer-reading and input mechanics established in `../smithers/tests/tui-helpers.ts`.
   - Create `tests/happy_path.vhs` for visual CI validation of the happy-path flow.

## File Plan
- `go.mod`
- `main.go`
- `internal/cmd/root.go`
- `internal/config/config.go`
- `internal/ui/logo/logo.go`
- `internal/ui/styles/styles.go`
- `tests/tui_e2e_test.go` (new)
- `tests/happy_path.vhs` (new)
- All `*.go` files within `internal/` that contain the `github.com/charmbracelet/crush` import path.

## Validation
- **Build & Unit Tests:** Run `go build -o smithers-tui main.go` to ensure compilation succeeds. Run `go test ./...` to verify all unit tests pass after the module rename.
- **Terminal E2E Coverage:** Run `go test ./tests/tui_e2e_test.go`. This test suite must be modeled on the upstream `@microsoft/tui-test` harness (referencing `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`), verifying that the TUI spins up, types characters into the PTY, and reads raw screen buffers to assert expected output (e.g., the presence of "SMITHERS").
- **Visual Integration:** Run `vhs tests/happy_path.vhs` to record the TUI's execution. Manually review the resulting `.gif` or `.mp4` to confirm the cyan/green/magenta color scheme and the new ASCII logo.
- **Manual Checks:** Execute `./smithers-tui`. Verify that it creates the `~/.smithers-tui` directory instead of `~/.crush` and respects variables like `SMITHERS_PROFILE`.

## Open Questions
- Is there a specific, pre-designed ASCII art asset for the "SMITHERS" logo, or should we generate a placeholder for the initial implementation?
- What are the precise hex codes or terminal ANSI equivalents for the Smithers brand cyan, magenta, and green?
- Should the new E2E tests strictly use Bubble Tea's `teatest`, or is there a preferred PTY wrapper library for parity with the Bun-based upstream tests?