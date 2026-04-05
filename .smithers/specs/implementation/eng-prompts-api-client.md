# Implementation: eng-prompts-api-client

**Status**: Complete
**Date**: 2026-04-05

---

## Summary

Implemented the Prompts API Client for the Smithers TUI project. Three new files were added to `internal/smithers/` following the established patterns from `client.go` and `types.go`.

---

## Files Created

### `internal/smithers/types_prompts.go`

Defines two new types:

- **`Prompt`** — mirrors `DiscoveredPrompt` from `smithers/src/cli/prompts.ts`. Fields: `ID`, `EntryFile`, `Source` (omitempty), `Props` (as `[]PromptProp`, JSON key `inputs`).
- **`PromptProp`** — mirrors the `inputs[]` entries. Fields: `Name`, `Type` (defaults to `"string"`), `DefaultValue *string` (omitempty).

### `internal/smithers/prompts.go`

Five methods on `*Client`, each following the three-tier transport pattern (HTTP → filesystem → exec):

| Method | HTTP route | Filesystem | Exec fallback |
|---|---|---|---|
| `ListPrompts(ctx)` | `GET /prompt/list` | scan `.smithers/prompts/*.mdx` | `smithers prompt list --format json` |
| `GetPrompt(ctx, id)` | `GET /prompt/get/{id}` | read `{id}.mdx` + parse props | `smithers prompt get {id} --format json` |
| `UpdatePrompt(ctx, id, content)` | `POST /prompt/update/{id}` | overwrite `{id}.mdx` | `smithers prompt update {id} --source {content}` |
| `DiscoverPromptProps(ctx, id)` | `GET /prompt/props/{id}` | local parse of MDX source | exec get → local parse |
| `PreviewPrompt(ctx, id, props)` | `POST /prompt/render/{id}` | local `{props.X}` substitution | `smithers prompt render {id} --input {json} --format json` |

Key implementation details:
- `discoverPropsFromSource` uses a regex (`\{props\.([A-Za-z_][A-Za-z0-9_]*)\}`) to extract variables in first-appearance order with deduplication.
- `renderTemplate` performs deterministic substitution; unresolved placeholders are left intact.
- `parsePromptsJSON` tolerates both direct `[]Prompt` arrays and `{"prompts": [...]}` wrapped shapes.
- `parseRenderResultJSON` tolerates plain strings, `{"result": "..."}`, and `{"rendered": "..."}` shapes.
- `updatePromptOnFS` stat-checks the file before writing to avoid creating stray files.
- No SQLite fallback — prompts are file-backed with no database representation.

### `internal/smithers/prompts_test.go`

36 test functions covering:

- All three transport tiers (HTTP, filesystem, exec) for each method
- HTTP request/response shape assertions (path, method, body fields)
- Filesystem round-trips using `withTempPromptsDir` helper (temp dir + chdir + cleanup)
- Exec fallback with JSON body assertions
- `discoverPropsFromSource`: multiline, deduplication, no props, malformed input ignored
- `renderTemplate`: full resolution, partial resolution, empty props, numeric values
- `parsePromptsJSON`: direct array and wrapped shape
- `parseRenderResultJSON`: plain string, `result` field, `rendered` field, malformed

---

## Constraints Respected

- **No existing files modified** — all code is in new files only.
- **No SQLite path** — prompts are file-backed; filesystem is the second tier.
- **No git commit** — as instructed.

---

## Pre-existing Build Issues (not caused by this ticket)

The `internal/smithers` package has pre-existing redeclaration errors in files added by other in-flight tickets (`timetravel.go`, `runs.go`, `types_runs.go`, `types_timetravel.go`). These prevent `go test ./internal/smithers/...` from running for the whole package. The new prompts files produce zero errors when checked with `go build -gcflags="-e"`.

---

## Open Questions Resolved

1. **Exec flag format**: Used `--format json` on `list` and `get`; `--source` for update; `--input` (JSON string) for render — consistent with existing `--format json` conventions in `client.go`.
2. **Prompts view scaffold**: Deferred — kept implementation strictly to `internal/smithers/` as specified in the ticket scope.
3. **UI client initialization from config**: Deferred to `platform-config-namespace` ticket.
4. **Exec output tolerance**: `parsePromptsJSON` tolerates both wrapped and direct array shapes.
