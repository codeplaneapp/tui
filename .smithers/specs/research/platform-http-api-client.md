Research based on current code in `internal/smithers/client.go`, `internal/smithers/types.go`, `internal/smithers/client_test.go`, upstream Smithers server contracts, and the PRD/engineering docs.

## What Is Already Implemented

### Client struct and constructor
- `Client` struct with `apiURL`, `apiToken`, `dbPath`, `db` (read-only SQLite), `httpClient`, `execFunc` (injectable for testing), and server availability cache (`serverMu`, `serverUp`, `serverChecked`).
- `NewClient(opts ...ClientOption)` — functional options pattern (`WithAPIURL`, `WithAPIToken`, `WithDBPath`, `WithHTTPClient`, `withExecFunc`). Zero-options call is valid (backward-compatible stub).
- `Close()` releases the SQLite connection.

### Three-tier transport hierarchy
The client always tries in this order:
1. **HTTP** — `c.isServerAvailable()` gate → `httpGetJSON` / `httpPostJSON`
2. **SQLite** — `c.db` direct read-only access via `queryDB`
3. **exec** — `execSmithers(ctx, args...)` shells out to the `smithers` binary

This mirrors the PRD's four-channel architecture (channels 1 + 3 in Go, with SQLite as a bonus read-only fast path).

### Server availability probe
- `isServerAvailable()` sends `GET /health` with a 1-second context timeout.
- Result is cached 30 seconds under a `sync.RWMutex` with a double-check after acquiring the write lock (correct double-checked locking pattern).
- Probe result stored as `serverUp bool`; a failed probe marks the server down and skips HTTP for subsequent calls within the cache window.

### HTTP helpers
- `httpGetJSON(ctx, path, out)` — sets `Authorization: Bearer` if token present, decodes `apiEnvelope{ok, data, error}`, unmarshals `data` into `out`.
- `httpPostJSON(ctx, path, body, out)` — JSON-encodes body, sets `Content-Type: application/json` + auth, same envelope decode.
- Both wrap transport errors as `ErrServerUnavailable` (sentinel sentinel, not wrapping the underlying network error).

### Implemented domain methods

| Method | HTTP route | SQLite fallback | exec fallback |
|---|---|---|---|
| `ListPendingApprovals` | `GET /approval/list` | `_smithers_approvals` | `smithers approval list --format json` |
| `ExecuteSQL` | `POST /sql` | direct query (SELECT/PRAGMA/EXPLAIN only) | `smithers sql --query Q --format json` |
| `GetScores` | — (no HTTP endpoint) | `_smithers_scorer_results` | `smithers scores <runID> --format json` |
| `GetAggregateScores` | delegates to `GetScores` | same | same |
| `ListMemoryFacts` | — (no HTTP endpoint) | `_smithers_memory_facts` | `smithers memory list <ns> --format json` |
| `RecallMemory` | — (always exec) | — | `smithers memory recall <query>` |
| `ListCrons` | `GET /cron/list` | `_smithers_crons` | `smithers cron list --format json` |
| `CreateCron` | `POST /cron/add` | — | `smithers cron add <pattern> <path>` |
| `ToggleCron` | `POST /cron/toggle/{id}` | — | `smithers cron toggle <id> --enabled <bool>` |
| `DeleteCron` | — (no HTTP endpoint) | — | `smithers cron rm <id>` |
| `ListTickets` | `GET /ticket/list` | — | `smithers ticket list --format json` |
| `ListAgents` | — (stub only) | — | — |

### Types defined in types.go
`Agent`, `SQLResult`, `ScoreRow`, `AggregateScore`, `MemoryFact`, `MemoryRecallResult`, `Ticket`, `Approval`, `CronSchedule`.

### Test coverage
`client_test.go` covers HTTP and exec paths for: `ExecuteSQL`, `GetScores`, `GetAggregateScores`, `ListMemoryFacts`, `RecallMemory`, `ListCrons`, `CreateCron`, `ToggleCron`, `DeleteCron`, transport fallback (server down), `convertResultMaps`, `ListAgents`. Uses `httptest.NewServer` with a health-check shortcut and an injectable `execFunc`.

---

## What Is Missing (Gap Analysis)

### 1. Core run/node/attempt types
`types.go` has no `Run`, `RunStatus`, `RunNode`, `Attempt`, `RunEvent`, `Workflow`, `HijackSession`, `Snapshot`, `Diff`, or `ChatBlock`. These are required by the engineering doc's full client interface (`ListRuns`, `GetRun`, `StreamEvents`, `StreamChat`, `HijackRun`, `ListSnapshots`, `DiffSnapshots`, etc.) and by every Smithers view (runs dashboard, live chat, time-travel).

### 2. Run-scoped HTTP methods
None of the run-centric operations are implemented:
- `ListRuns` / `GetRun` — `GET /v1/runs`, `GET /v1/runs/:id`
- `InspectRun` — `GET /v1/runs/:id` (with node/attempt detail)
- `CancelRun` — `POST /v1/runs/:id/cancel` (or per serve.ts: `/cancel`)
- `ApproveNode` / `DenyNode` — the existing `ListPendingApprovals` exists but there is no `ApproveNode(runID, nodeID)` or `DenyNode(runID, nodeID)` method yet
- `RunWorkflow` — `POST /v1/runs` (start a new run)
- `ListWorkflows` — no HTTP endpoint yet; would need exec fallback

### 3. SSE streaming
No SSE consumer exists. The engineering doc specifies `internal/smithers/events.go` as a separate file for:
- `StreamRunEvents(ctx, runID, afterSeq)` → `<-chan RunEvent` — consumes `GET /v1/runs/:id/events?afterSeq=N`
- `StreamWorkspaceEvents(ctx, afterSeq)` → `<-chan RunEvent` — global feed (`GET /v1/runs/events`)
- Parsing `text/event-stream` lines: `event: smithers\ndata: {...}\n\n`
- Keep-alive comment lines (`:\n`) must be skipped
- Graceful shutdown on context cancel
- Reconnect on connection drop

### 4. Schema/name drift in existing SQLite fallbacks
- `GetScores` queries `_smithers_scorer_results`, but upstream schema (`smithers/src/scorers/schema.ts`) uses `_smithers_scorers`.
- `ListCrons` SQLite query targets `_smithers_crons`; upstream schema uses `_smithers_cron` (no trailing `s`).
- `GetScores` SQLite path skips HTTP tier entirely (there is a comment saying "no HTTP endpoint" but upstream `GET /v1/runs/:id` includes score data in the run detail — this should be verified).

### 5. CLI fallback drift
- `ExecuteSQL` exec falls back to `smithers sql --query Q`; the upstream CLI (`smithers/src/cli/index.ts`) does not expose a `sql` subcommand — this fallback will always fail when the server is not available.
- `ToggleCron` exec uses `smithers cron toggle <id> --enabled <bool>`, but the upstream CLI uses `cron enable <id>` / `cron disable <id>` — the flag-based form does not exist.
- `DeleteCron` exec uses `smithers cron rm`; upstream uses `smithers cron remove` (or `rm` — needs verification).
- `ListMemoryFacts` always appends `--format json`; upstream `memory list` may not support `--format` (check CLI surface).

### 6. Error handling gaps
- `httpGetJSON` and `httpPostJSON` wrap all transport errors as `ErrServerUnavailable`, losing the underlying `net/http` error (connection refused vs. DNS failure vs. timeout). This prevents callers from distinguishing transient network errors from permanent misconfiguration.
- Non-OK HTTP status codes outside the envelope format (e.g., `401 Unauthorized`, `429 Too Many Requests`, `503 Service Unavailable`) are not handled — they fall through to JSON decode of the body which will likely fail with a confusing parse error.
- There is no retry logic. A single transient network error immediately marks the server down for 30 seconds.
- `isServerAvailable()` uses `context.Background()` (not caller's context), so it cannot be cancelled by the calling operation's deadline.

### 7. Timeout configuration
- `httpClient` is created with `Timeout: 10 * time.Second` (global). This single timeout applies to all operations equally. Long-running streaming responses (SSE, large SQL result sets) will be killed by this timeout.
- The health probe uses a separate 1-second `context.WithTimeout` correctly, but this does not protect against slow response bodies on normal API calls.

### 8. Connection pooling
- The default `http.Client` uses Go's default `http.Transport`, which provides connection pooling. No explicit configuration of `MaxIdleConns`, `MaxIdleConnsPerHost`, or `IdleConnTimeout`. For a TUI polling multiple endpoints every few seconds, default settings are probably fine, but they should be documented.
- There is no explicit `DisableKeepAlives` toggle, meaning all connections are pooled by default (correct behavior for a persistent client).

### 9. Authentication
- Bearer token auth is supported via `WithAPIToken` / `apiToken` field.
- No token refresh, no token expiry handling, no 401-triggered re-auth flow.
- The `SMITHERS_API_KEY` env var is referenced in PRD config but is not automatically read in `NewClient` — the caller must wire it via `WithAPIToken`.

### 10. Config-to-client wiring
- `internal/ui/model/ui.go` constructs `smithers.NewClient()` with no options (line 332 per research doc), so `apiURL`, `apiToken`, and `dbPath` from `config.Smithers` are never passed in. All HTTP and SQLite paths are dead in runtime despite being fully implemented.

### 11. ListAgents is a stub
`ListAgents` returns hardcoded placeholder data regardless of options. It should detect binaries on `$PATH` via `exec.LookPath` and check auth signals (env vars, keychain), but this is a separate ticket concern.

---

## What Is Working Well

1. **Transport hierarchy architecture** — the three-tier fallback pattern (HTTP → SQLite → exec) is the right design for Smithers TUI's "server may or may not be running" reality.
2. **Functional options pattern** — `ClientOption` functions make the client easily configurable and testable without exported fields.
3. **Injectable exec function** — `withExecFunc` enables complete unit testing of the exec tier without spawning real processes.
4. **Double-checked locking** on availability cache — correct use of `sync.RWMutex` with `RLock` fast path and `Lock` upgrade with double-check.
5. **`apiEnvelope` decoding** — the `{ok, data, error}` envelope is correct for the Smithers API shape used by cron/ticket/approval endpoints.
6. **`isSelectQuery` guard** — prevents mutation queries from hitting the read-only SQLite path.
7. **Test infrastructure** — `newTestServer` + `writeEnvelope` + `newExecClient` helpers cover both HTTP and exec tiers cleanly with `httptest`.

---

## Authentication Requirements

The Smithers HTTP server supports bearer token auth. For local development, the server typically runs unauthenticated on `localhost:7331`. In production/CI, `SMITHERS_API_KEY` should be read at startup and passed via `WithAPIToken`. The `httpGetJSON` / `httpPostJSON` helpers already set the `Authorization: Bearer` header when a token is present. No changes to auth mechanics are needed — only wiring the config value at construction time.

---

## Server Availability Probing

The 30-second availability cache is appropriate for a TUI that polls every 1-5 seconds. The probe hitting `GET /health` is correct. Two improvements are needed:

1. **Invalidate cache on transport error**: when `httpGetJSON` or `httpPostJSON` returns `ErrServerUnavailable` (meaning a real HTTP call failed mid-operation), the availability cache should be immediately invalidated so the next call re-probes instead of waiting up to 30 seconds.

2. **Background keepalive**: once the server is known-up, a lightweight background goroutine can periodically touch `/health` and update `serverUp` without blocking API calls. This decouples the probe latency from the call latency. The goroutine must be stopped on `Close()`.

---

## Connection Pooling Assessment

Default `http.Transport` pools connections per host. For a local loopback connection (`localhost:7331`), this is negligible overhead. The client does not need custom transport configuration unless SSE streaming is added — SSE responses are long-lived and the global `Timeout: 10s` on `httpClient` will terminate them. A separate `http.Client` with no timeout (or a much longer timeout) is needed for streaming endpoints.

---

## Summary of What This Ticket Must Deliver

The `platform-http-api-client` ticket is specifically about the HTTP transport layer. Based on the gap analysis:

1. Add missing run-domain types to `types.go`.
2. Implement HTTP methods for the run lifecycle (`ListRuns`, `GetRun`, `CancelRun`, `ApproveNode`, `DenyNode`).
3. Fix error handling so non-OK HTTP status codes (401, 429, 503) produce typed errors, not decode failures.
4. Fix transport error wrapping to preserve the underlying error for diagnostics.
5. Invalidate availability cache on transport error.
6. Add a separate streaming `http.Client` (no timeout) for SSE.
7. Implement `StreamRunEvents` / `StreamWorkspaceEvents` in a new `events.go`.
8. Fix SQLite table name drift (`_smithers_crons` → `_smithers_cron`, `_smithers_scorer_results` → verify against current schema).
9. Fix CLI fallback drift for `cron toggle`, `sql` command.
10. Wire `config.Smithers` into client construction in `ui.go`.
11. Tests for all new methods using existing `newTestServer` + `newExecClient` patterns.
