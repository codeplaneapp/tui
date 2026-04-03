## Goal
Deliver a regression-safe Tickets API client layer for Crush Smithers mode by
adding `ListTickets`, `CreateTicket`, and `UpdateTicket` in
`internal/smithers`, with transport behavior and payload handling locked to
real upstream references, then validating via unit tests, terminal E2E
coverage modeled on the upstream harness pattern, and a VHS happy-path test.

## Steps
1. Lock the ticket contract before coding to avoid rework.
   - Reconcile the mismatch that `../smithers/src` and `../smithers/src/server`
     in this checkout do not currently expose ticket endpoints, and
     `../smithers/gui/src` is missing.
   - Use available authoritative ticket behavior from the existing ticket
     inputs and the legacy Smithers transport shape (`ticket list/create/update`,
     `{ tickets: [...] }`, `{ id, content }`, `TICKET_EXISTS`,
     `TICKET_NOT_FOUND`) as the first-pass compatibility target.
2. Extend Smithers client domain types for tickets in
   `internal/smithers/types.go`.
   - Add a `Ticket` model and any small response/request helper types needed to
     keep client methods typed and stable.
3. Implement ticket client methods in `internal/smithers/client.go`.
   - Add `ListTickets`, `CreateTicket`, and `UpdateTicket`.
   - Use deterministic transport order to minimize regressions: HTTP first, then
     `exec smithers ticket ...` fallback.
   - Do not add a SQLite path for tickets (tickets are file-backed in
     `.smithers/tickets`, not DB-backed).
4. Make response/error parsing resilient to known Smithers transport variants.
   - Support envelope and direct JSON payloads where necessary.
   - Normalize ticket create/update/list payloads to the new Go ticket types.
   - Map expected ticket-domain failures into actionable errors for callers.
5. Add minimal UI entry wiring so terminal E2E can exercise the new client.
   - Add a lightweight tickets view scaffold and command-palette route so test
     flows can list, create, and update tickets through keyboard navigation.
   - Keep scope intentionally thin; full tickets UX parity remains in feature
     tickets.
6. Add focused unit tests for client behavior.
   - Cover HTTP success, exec fallback, optional create content, update required
     content, malformed payload handling, and domain error mapping.
7. Add view/router tests for the tickets entry path.
   - Verify open/close navigation and client method invocation in tickets
     interactions.
8. Add terminal E2E coverage in this repo modeled on upstream harness semantics.
   - Add a local harness with the same primitives as
     `../smithers/tests/tui-helpers.ts` (`launch`, `waitForText`,
     `waitForNoText`, `sendKeys`, `snapshot`, `terminate`).
   - Add a tickets client E2E test modeled on `../smithers/tests/tui.e2e.test.ts`
     that validates list/create/update through real TUI key flows.
9. Add one VHS happy-path recording test for the tickets flow.
   - Record a stable flow: open tickets view, create ticket, edit ticket, save,
     and return.
10. Wire repeatable validation commands.
   - Add/update task or documented commands so CI/local runs can execute unit,
     terminal E2E, and VHS checks consistently.

## File Plan
1. `internal/smithers/types.go`
2. `internal/smithers/client.go`
3. `internal/smithers/client_test.go`
4. `internal/ui/views/tickets.go` (new)
5. `internal/ui/views/tickets_test.go` (new)
6. `internal/ui/dialog/actions.go`
7. `internal/ui/dialog/commands.go`
8. `internal/ui/model/ui.go`
9. `tests/tui/helpers_test.go` (new, harness modeled on upstream
   `../smithers/tests/tui-helpers.ts`)
10. `tests/tui/tickets_client_e2e_test.go` (new, flow modeled on upstream
    `../smithers/tests/tui.e2e.test.ts`)
11. `tests/vhs/tickets-client-happy-path.tape` (new)
12. `Taskfile.yaml` (if adding explicit ticket E2E/VHS tasks)

## Validation
1. `gofumpt -w internal/smithers internal/ui/views internal/ui/dialog internal/ui/model tests/tui`
2. `go test ./internal/smithers -run Ticket -v`
3. `go test ./internal/ui/views -run Ticket -v`
4. `go test ./internal/ui/model -run Smithers -v`
5. `go test ./tests/tui -run TestTicketsClientE2E -count=1 -v -timeout 120s`
6. Terminal E2E parity check: confirm local harness and test semantics mirror
   upstream `../smithers/tests/tui.e2e.test.ts` and
   `../smithers/tests/tui-helpers.ts` (spawned process, ANSI-normalized text
   polling, keyboard injection, snapshot-on-failure, terminate cleanup).
7. `vhs tests/vhs/tickets-client-happy-path.tape`
8. `go test ./...`
9. Manual smoke check:
   - Run `go run .`
   - Open command palette, route to Tickets, create/edit a ticket, save, then
     return with `Esc`.

## Open Questions
1. `../smithers/src` currently lacks ticket HTTP/CLI endpoints and
   `../smithers/gui/src` is missing in this checkout. Should this ticket target
   the legacy ticket transport contract as a compatibility baseline, or wait for
   new upstream ticket endpoints?
2. Should the minimal tickets view scaffold live in this API-client ticket (to
   satisfy terminal E2E now), or should it be moved to a separate scaffolding
   ticket with a test-only API client exerciser here?
3. Should ticket-domain errors be exposed as typed sentinel errors in
   `internal/smithers` (e.g., exists/not-found), or left as wrapped transport
   errors for higher layers to parse?
4. For create semantics, should omitted content always delegate to upstream
   default template generation, or should Crush enforce an explicit content
   requirement?
