# Research Summary: chat-ui-branding-status

## Ticket Overview
The `chat-ui-branding-status` ticket requires updating the default Crush UI to reflect the Smithers brand. This includes replacing the ASCII art logo and restructuring the status/header bar to support Smithers client connection state and run metrics.

## Key Files Identified
- `internal/ui/logo/logo.go` — Contains the current Crush ASCII art logo built with lipgloss letterforms. The logo system uses a sophisticated stretching/randomization mechanism for dynamic rendering. This file needs a new Smithers ASCII art logo added or the existing one replaced.
- `internal/ui/model/header.go` — Header model that will need to display Smithers branding and connection state.
- `internal/ui/model/status.go` — Status bar model that will need to show Smithers run metrics.
- `internal/ui/styles/styles.go` — Shared styles used across the UI.
- `internal/smithers/client.go` — The Smithers API client (new file) that provides connection state and run data.
- `internal/smithers/types.go` — Smithers type definitions.

## Architecture Notes
- The logo system in `logo.go` uses lipgloss for horizontal/vertical joins of styled letter parts. Each letter is defined as a function returning a styled string. Letters support configurable stretching via `letterformProps` (width, minStretch, maxStretch).
- The `stretchLetterformPart` helper uses cached random numbers for reproducible randomized stretching.
- The header and status components follow the Bubble Tea (bubbletea) model-update-view pattern.

## Acceptance Criteria
1. The application header displays Smithers ASCII art instead of Crush.
2. The header/status components are prepared to receive and display dynamic Smithers client state (connection status, run metrics).

## Dependencies
None — this ticket has no blocking dependencies.

## Implementation Approach
1. Design a new Smithers ASCII art logo using the existing letterform infrastructure in `logo.go`.
2. Update header model to source branding from Smithers config.
3. Extend status bar model with fields for Smithers connection state and run metrics.
4. Wire Smithers client state into the header/status view layer.