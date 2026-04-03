# Platform: Rebrand TUI to Smithers

## Existing Crush Surface

- **`go.mod`**: Currently configured with `module github.com/charmbracelet/crush`. This is the core module name that needs updating.
- **`main.go` & `internal/cmd/root.go`**: Contains references to the `crush` command, `CRUSH_PROFILE`, `CRUSH_DISABLE_METRICS`, and the hardcoded `.crush` and `crush.json` directory structures. Additionally, `root.go` contains a hardcoded ASCII `heartbit` string.
- **`internal/ui/logo/logo.go`**: Contains complex logic specifically designed to stretch and render the letters "C", "R", "U", "S", "H". It includes `Render` and `SmallRender` methods which will need a complete rewrite to render the word "SMITHERS".
- **`internal/ui/styles/styles.go`**: Defines the overall application styling. It currently sets `primary = charmtone.Charple` (purple) and `secondary = charmtone.Dolly` (yellow). The base background is `charmtone.Pepper`.

## Upstream Smithers Reference

- **`docs/smithers-tui/01-PRD.md` & `02-DESIGN.md`**: Dictate that the TUI should be a hard-fork rebranded to "SMITHERS", using a cyan/green/magenta color scheme, and configured to use `.smithers-tui/smithers-tui.json`.
- **`docs/smithers-tui/03-ENGINEERING.md` & `docs/smithers-tui/features.ts`**: The engineering plan outlines renaming the module to `github.com/anthropic/smithers-tui` and ensuring zero Smithers business logic exists in the TUI, pointing to the need for a complete replacement of branding before adding the specialized logic. Contains features like `PLATFORM_SMITHERS_REBRAND` and `PLATFORM_SMITHERS_CONFIG_NAMESPACE`.
- **`smithers_tmp/tests/tui.e2e.test.ts` & `smithers_tmp/tests/tui-helpers.ts`**: These files show an expectation for E2E terminal testing using standard input (`\r`, `\x1b`, text typing) and parsing the raw or un-ANSI-fied terminal buffer output (e.g. `waitForText("Smithers Runs")`). The new TUI will need an equivalent E2E harness in Go, supplemented with a VHS happy-path recording.

## Gaps

- **Data-Model / Config Namespace**: The app still defaults to `crush.json` in the `.crush` directory. Upstream requires `.smithers-tui/smithers-tui.json`. Environment variables still use the `CRUSH_` prefix instead of `SMITHERS_`.
- **Rendering (Logo)**: The logic in `logo.go` specifically structures the 5 letters of Crush. There is no existing logic for "SMITHERS", nor is there an upstream asset ready to drop in yet—meaning a new ASCII generator for "SMITHERS" must be constructed.
- **UX (Colors & Copy)**: The terminal color scheme is entirely dependent on `charmtone` purple and yellow palettes instead of the cyan/green/magenta defined in the Smithers PRD. Copy throughout the app (like `root.go`'s description "A terminal-first AI assistant...") is still Crush-oriented.
- **Testing**: Crush lacks an overarching end-to-end integration test suite modeled on terminal inputs and assertions equivalent to `@microsoft/tui-test` or the provided `tui-helpers.ts`.

## Recommended Direction

1. **Module & Imports**: Perform a global replace of `github.com/charmbracelet/crush` to `github.com/anthropic/smithers-tui` across the codebase, and update `go.mod` via `go mod edit -module github.com/anthropic/smithers-tui`.
2. **Rebranding Configuration**: Update `internal/cmd/root.go` and `internal/config/config.go` (and wherever directories are parsed) to use `.smithers-tui` as the data directory, `smithers-tui` as the binary name, and update environment variables to use `SMITHERS_` prefixes.
3. **Logo Replacement**: Rewrite `internal/ui/logo/logo.go` to render the word "SMITHERS". This can involve either simplifying the rendering to a static ASCII string or writing new stretching letter functions for S-M-I-T-H-E-R-S. Also replace the `heartbit` in `internal/cmd/root.go`.
4. **Color Scheme**: Modify `internal/ui/styles/styles.go` to use the new Smithers palette. E.g., setting primary to a cyan tone, secondary to magenta, and keeping success/info tags green/cyan.
5. **Testing Harness**: Implement an E2E test file in Go (e.g. `tui_e2e_test.go`) utilizing a pseudo-terminal (PTY) or Bubble Tea's `teatest` to write keys and read screen output, mirroring the approach in `smithers_tmp/tests/tui-helpers.ts`. Additionally, add a `.vhs` file for the happy path recording.

## Files To Touch

- `go.mod`
- `main.go`
- `internal/cmd/root.go`
- `internal/ui/logo/logo.go`
- `internal/ui/styles/styles.go`
- `internal/config/config.go` (and any related default initializers)
- All `*.go` files containing `github.com/charmbracelet/crush` imports.
- `tests/tui_e2e_test.go` (new)
- `tests/happy_path.vhs` (new)