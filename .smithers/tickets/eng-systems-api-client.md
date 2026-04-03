# Implement Systems and Analytics API Client Methods

## Metadata
- ID: eng-systems-api-client
- Group: Systems And Analytics (systems-and-analytics)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Expand the Smithers HTTP/SQLite client to include methods required for the Systems and Analytics views. This includes API bindings for executing raw SQL queries, retrieving scores and metrics, memory recall/listing, and cron/trigger CRUD operations. Must support dual-mode access (HTTP when server running, SQLite fallback when direct db is present) for read operations where applicable.

## Acceptance Criteria

- Client struct includes ExecuteSQL, GetScores, ListMemoryFacts, RecallMemory, and cron management methods.
- Methods use HTTP API when available, falling back to direct SQLite access for read operations if possible.
- Unit tests confirm requests are routed to the correct transport layer.

## Source Context

- internal/app/
- ../smithers/src/server/

## Implementation Notes

- Refer to PRD Section 6.11-6.15 and Engineering Section 3.1.2.
- Cron management might need to shell out (`exec.Command`) to `smithers cron` if explicit HTTP endpoints don't exist.
