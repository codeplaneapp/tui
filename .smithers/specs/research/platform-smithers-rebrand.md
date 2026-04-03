# Platform: Rebrand TUI to Smithers

## Existing Crush Surface
The current Crush codebase (`internal/`) heavily references the "Crush" brand in multiple layers:
- **Module and Imports:** The `go.mod` and all internal imports across the codebase use `github.com/charmbracelet/crush`.
- **Config & Data Model:** `internal/config/config.go` sets `appName = "crush"` and `defaultDataDirectory = ".crush"`. It specifically looks for files like `crush.md` and `crush.json`.
- **CLI Bootstrapping:** `internal/cmd/root.go` defines the `crush` command, loads the `CRUSH_PROFILE` environment variable, and contains a hardcoded `heartbit` ASCII string for CLI branding.
- **Rendering & UX (Logo):** `internal/ui/logo/logo.go` contains specialized logic (`Render` and `SmallRender`) uniquely designed to construct and horizontally stretch the five letters "C", "R", "U", "S", "H" via `letterform` arrays.
- **Rendering & UX (Colors):** `internal/ui/styles/styles.go` defines a theme using the `charmtone` library, specifically relying on `charmtone.Charple` (purple) for primary and `charmtone.Dolly` (yellow) for secondary colors. 

## Upstream Smithers Reference
The upstream references in the `docs/smithers-tui` and `../smithers` directories establish the target state:
- **Specifications (`docs/smithers-tui/01-PRD.md`, `02-DESIGN.md`, `03-ENGINEERING.md`):** Mandate renaming the module to `github.com/anthropic/smithers-tui` and establishing a new aesthetic featuring a cyan/green/magenta palette. The configuration namespace must move to `.smithers-tui`.
- **Features Manifest (`docs/smithers-tui/features.ts`):** Defines the exact feature inventory needed, including `PLATFORM_SMITHERS_REBRAND` and `PLATFORM_SMITHERS_CONFIG_NAMESPACE`.
- **Testing (`../smithers/tests/tui.e2e.test.ts`, `../smithers/tests/tui-helpers.ts`):** Provide a reference E2E framework written in Bun that spins up a PTY, types characters, and reads raw screen buffers (e.g., `tui.waitForText("Smithers Runs")`). The new TUI requires equivalent E2E test coverage asserting real output, supplemented with a VHS happy-path recording.
- **Backend Model (`../smithers/src/server/index.ts`):** Represents the eventual broker backend the TUI will integrate with, highlighting the long-term gap between Crush's local file-based data model and Smithers's client-server transport model.

## Gaps
Comparing Crush and Smithers reveals significant gaps:
- **Data-Model / Transport:** Crush is locally configured via `.crush` directories and `CRUSH_` prefixed environment variables. Upstream requires `.smithers-tui/smithers-tui.json` and `SMITHERS_` prefixes.
- **Rendering (Logo):** Crush's ASCII generator is hardcoded specifically for the 5-letter word "CRUSH" with complex stretching mechanics. There is no equivalent logic ready to render the 8-letter word "SMITHERS".
- **UX (Colors & Copy):** Crush uses a purple/yellow `charmtone` palette and copy referring to "Charm CRUSH". Smithers requires a cyan/magenta/green brand identity and accurate "Smithers TUI" text throughout the CLI.
- **Testing Expectation:** Crush lacks an overarching E2E testing framework equivalent to the terminal buffer scraping seen in `smithers/tests/tui.e2e.test.ts`.

## Recommended Direction
1. **Module Renaming:** Use `go mod edit` to rename the module to `github.com/anthropic/smithers-tui` and perform a global find-and-replace for `github.com/charmbracelet/crush`.
2. **Config Namespace:** Update `internal/config/config.go` and `internal/cmd/root.go` to use the `.smithers-tui` data directory, rename the binary to `smithers-tui`, and switch environment variables to `SMITHERS_` prefixes.
3. **Logo Swap:** Rewrite or replace `internal/ui/logo/logo.go` to render a static or simplified ASCII representation of "SMITHERS", removing the Crush-specific stretching logic.
4. **Color Palette:** Modify `internal/ui/styles/styles.go` to replace `charmtone.Charple` and `charmtone.Dolly` with cyan and magenta variants aligned with the Smithers design system.
5. **Testing Path:** Add a Go-based E2E harness (using a PTY testing library like `teatest`) to replicate the buffer-reading assertions in `smithers/tests/tui-helpers.ts`, and create a `tests/happy_path.vhs` tape for visual CI validation.

## Files To Touch
- `go.mod`
- `main.go`
- `internal/cmd/root.go`
- `internal/config/config.go`
- `internal/ui/logo/logo.go`
- `internal/ui/styles/styles.go`
- `tests/tui_e2e_test.go` (new)
- `tests/happy_path.vhs` (new)
- All `*.go` files containing the legacy `github.com/charmbracelet/crush` import path.