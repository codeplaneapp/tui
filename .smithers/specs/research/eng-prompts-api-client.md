# Research Summary: eng-prompts-api-client

## Existing Crush Surface
The Crush client in `internal/smithers/client.go` and `internal/smithers/types.go` establishes a robust Smithers API client using three transport tiers: HTTP API, direct SQLite (read-only), and an `exec` fallback. It currently supports methods like `ExecuteSQL` and `ListAgents` but completely lacks any endpoints for handling Prompts. The `internal/smithers/types.go` file contains data models for `Agent`, `SQLResult`, `ScoreRow`, etc., but has no model defined for Prompts.

## Upstream Smithers Reference
The upstream Prompts API is implemented in Smithers across the following primary files:
- **Server / CLI implementation** (`smithers_tmp/src/cli/prompts.ts`): Contains core logic like `discoverPrompts`, `updatePromptSource`, and `renderPromptFile`. It exposes the `DiscoveredPrompt` type containing `id`, `entryFile`, `source`, and `inputs` (which are extracted as `{ name, type, defaultValue }`).
- **GUI Transport** (`smithers_tmp/gui-src/ui/api/transport.ts`): Defines the expected HTTP surface:
  - `fetchPrompts()` uses `GET /prompt/list`
  - `updatePromptSource(id, source)` uses `POST /prompt/update/:id`
  - `renderPromptPreview(id, input)` uses `POST /prompt/render/:id`

## Gaps
- **Data Model Gap**: Crush's `types.go` is missing a `Prompt` struct and a corresponding `PromptInput` struct to correctly map to the `DiscoveredPrompt` interface returned by the server.
- **Transport Gap**: `client.go` lacks the client methods to interact with the prompts endpoints (`GET /prompt/list`, `POST /prompt/update/:id`, `POST /prompt/render/:id`).
- **Rendering Gap**: The client needs to handle the `RenderPromptPreview` response accurately, ensuring it supports passing a dynamic map of key-value properties to the server and cleanly extracting the resulting rendered string.
- **Testing Gap**: There are no corresponding terminal E2E test or VHS recordings for Crush testing the prompt management and preview UX flows.

## Recommended Direction
1. **Data Model**: Add `Prompt` and `PromptInput` structs to `internal/smithers/types.go` with JSON struct tags mirroring the upstream TypeScript definitions.
2. **API Methods**: Add `ListPrompts(ctx context.Context) ([]Prompt, error)`, `UpdatePromptSource(ctx context.Context, id, source string) (*Prompt, error)`, and `RenderPromptPreview(ctx context.Context, id string, input map[string]any) (string, error)` to `internal/smithers/client.go`. Implement these using the existing `c.httpGetJSON` and `c.httpPostJSON` helpers.
3. **Exec Fallbacks**: If the HTTP server is unavailable, fall back to executing `smithers prompt list`, `smithers prompt update`, and `smithers prompt render` commands (assuming the Smithers CLI supports these equivalents), following the `ExecuteSQL` pattern.
4. **Testing**: Write a terminal E2E test verifying API capabilities using the `@microsoft/tui-test` pattern. Create a new VHS `.tape` script to capture the happy-path recording of navigating to and editing a prompt in the Crush TUI.

## Files To Touch
- `internal/smithers/types.go`
- `internal/smithers/client.go`
- `internal/smithers/client_test.go`
- `test/e2e/tui_prompts_test.go` (or similar E2E test harness file based on upstream expectations)
- `test/vhs/prompts_preview.tape` (or equivalent VHS testing location)