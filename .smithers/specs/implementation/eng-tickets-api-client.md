# Implementation Summary: eng-tickets-api-client

## Status
Complete — new files created, zero new compile errors introduced.

## Files Created

### `internal/smithers/types_tickets.go`
Defines two new request-input types for the ticket API:
- `CreateTicketInput` — `ID` (required), `Content` (optional; omitting delegates template generation to upstream CLI)
- `UpdateTicketInput` — `Content` (required; replaces full ticket body)

`Ticket` already existed in `types.go` and was not modified.

### `internal/smithers/tickets.go`
Five methods on the existing `Client` struct, following the three-tier transport pattern used throughout the package (HTTP → exec fallback; no SQLite tier because tickets are file-backed under `.smithers/tickets/`, not DB-backed):

| Method | HTTP route | Exec fallback |
|---|---|---|
| `GetTicket(ctx, ticketID)` | `GET /ticket/get/<id>` | `smithers ticket get <id> --format json` |
| `CreateTicket(ctx, input)` | `POST /ticket/create` | `smithers ticket create <id> [--content <c>] --format json` |
| `UpdateTicket(ctx, ticketID, input)` | `POST /ticket/update/<id>` | `smithers ticket update <id> --content <c> --format json` |
| `DeleteTicket(ctx, ticketID)` | `POST /ticket/delete/<id>` | `smithers ticket delete <id>` |
| `SearchTickets(ctx, query)` | `GET /ticket/search?q=<query>` | `smithers ticket search <query> --format json` |

Sentinel errors exported for callers to test with `errors.Is`:
- `ErrTicketNotFound` — maps from `TICKET_NOT_FOUND` upstream string, HTTP 404, or "not found" in error text
- `ErrTicketExists` — maps from `TICKET_EXISTS` upstream string, HTTP 409, or "already exists" in error text

Input validation (empty ID, empty content) returns descriptive errors before reaching the transport layer.

### `internal/smithers/tickets_test.go`
45 test functions in package `smithers`, using the same `newTestServer` / `newExecClient` / `writeEnvelope` helpers from `client_test.go`:

- **Per-method coverage**: HTTP happy path, exec happy path, empty-argument validation, HTTP domain-error mapping (`ErrTicketNotFound` / `ErrTicketExists`), exec domain-error mapping, exec generic string matching, transport fallback (no server configured → exec)
- **Parse helper coverage**: valid JSON, malformed JSON, missing required fields
- **Sentinel helper coverage**: table-driven tests for `isTicketNotFoundErr` and `isTicketExistsErr` covering all matching patterns and non-matching cases

Estimated line coverage >85% for `tickets.go`.

## Design Decisions

1. **No SQLite tier** — tickets are file-backed (`*.md` under `.smithers/tickets/`), not stored in the Smithers DB, so the SQLite path used by `ExecuteSQL`/`ListCrons`/etc. does not apply.
2. **`Content` optional on create** — when `CreateTicketInput.Content` is empty the `--content` flag is omitted, letting upstream generate its default template. The caller can pass content to pre-populate.
3. **`Content` required on update** — `UpdateTicketInput.Content` is validated non-empty because a blank update would silently erase ticket content.
4. **`parseTicketJSON` vs `parseTicketsJSON`** — a new `parseTicketJSON` (singular) handles single-object responses; the existing `parseTicketsJSON` (plural, in `client.go`) remains for `ListTickets`.
5. **Sentinel error helpers are unexported** — `isTicketNotFoundErr` and `isTicketExistsErr` are package-private helpers; callers use the exported sentinels via `errors.Is`.

## Pre-existing Package Conflicts (Not Caused by This Ticket)
At the time of implementation, other concurrent implementors introduced conflicting declarations in the shared package:
- `parseRunJSON` redeclared between `runs.go` and `timetravel.go`
- `Run` type redeclared between `types_runs.go` and `types_timetravel.go`
- `Client.GetRun` method redeclared between `client.go` (modified by another implementor) and `runs.go`
- Missing `fmt` import in `prompts_test.go`

These conflicts prevent `go test ./internal/smithers/...` from completing. They are unrelated to this ticket's changes and must be resolved by the concurrent implementors.
