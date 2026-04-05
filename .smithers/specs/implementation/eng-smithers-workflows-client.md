# Implementation Summary: eng-smithers-workflows-client

**Date**: 2026-04-05
**Status**: Complete
**Ticket**: eng-smithers-workflows-client

---

## What Was Built

Three new files were created in `internal/smithers/`, and one existing file was modified:

### `types_workflows.go`

New types for the workflows domain:

| Type | Description |
|------|-------------|
| `WorkflowStatus` | String enum (`draft`, `active`, `hot`, `archived`) |
| `Workflow` | Workflow record returned by `GET /api/workspaces/{id}/workflows`. Maps to `Workflow` in `@burns/shared` |
| `WorkflowDefinition` | Extends `Workflow` with `Source string` — the full TSX source. Maps to `WorkflowDocument` |
| `WorkflowTask` | Single launchable input field (`key`, `label`, `type`). Maps to `WorkflowLaunchField` |
| `DAGDefinition` | Launch-fields response: `workflowId`, `mode`, `entryTaskId?`, `fields[]`, `message?`. Maps to `WorkflowLaunchFieldsResponse` |
| `DiscoveredWorkflow` | Legacy CLI discovery record (`id`, `displayName`, `entryFile`, `sourceType`). Maps to `DiscoveredWorkflow` in `smithers/src/cli/workflows.ts` |
| `daemonErrorResponse` | Daemon plain-JSON error shape `{error, details?}` used by `toErrorResponse` |

### `workflows.go`

New methods on `*Client` and their three-tier transports:

| Method | Transport |
|--------|-----------|
| `ListWorkflows(ctx) ([]Workflow, error)` | HTTP `GET /api/workspaces/{id}/workflows` → exec `smithers workflow list --format json` |
| `GetWorkflowDefinition(ctx, workflowID) (*WorkflowDefinition, error)` | HTTP `GET /api/workspaces/{id}/workflows/{wid}` → exec `smithers workflow path {wid} --format json` |
| `RunWorkflow(ctx, workflowID, inputs) (*RunSummary, error)` | HTTP `POST /api/workspaces/{id}/runs` → exec `smithers workflow run {wid} --format json [--input ...]` |
| `GetWorkflowDAG(ctx, workflowID) (*DAGDefinition, error)` | HTTP `GET /api/workspaces/{id}/workflows/{wid}/launch-fields` → exec `smithers workflow path {wid}` + stub fallback |

New sentinel error:
- `ErrWorkflowNotFound` — HTTP 404 or empty workflow ID returned from exec

New transport helpers (distinct from legacy `httpGetJSON`/`v1GetJSON`):
- `apiGetJSON` / `apiPostJSON` — direct JSON transport for daemon `/api/*` routes
- `decodeDaemonResponse` — maps HTTP status codes to typed errors, decodes `{error,details?}` body

New parse/adapt helpers:
- `execListWorkflows` / `parseDiscoveredWorkflowsJSON` / `adaptDiscoveredWorkflows` — bridges legacy CLI output to the canonical `Workflow` type
- `execGetWorkflowDefinition` — resolves the workflow path and builds a `WorkflowDefinition` stub
- `execRunWorkflow` / `parseWorkflowRunResultJSON` — runs via CLI, tolerates both bare and wrapped run responses
- `execGetWorkflowDAG` — verifies the workflow exists, returns a single-field generic fallback `DAGDefinition`

### `workflows_test.go`

45 unit tests covering:

- **ListWorkflows**: HTTP success (2 items), HTTP empty list, HTTP server error, exec with wrapped JSON, exec with bare array, exec error, no-workspaceID falls to exec, server-down falls to exec
- **GetWorkflowDefinition**: HTTP success, HTTP 404 (ErrWorkflowNotFound), HTTP malformed JSON, empty workflowID guard, exec success, exec error, exec returns empty ID
- **RunWorkflow**: HTTP success with inputs, HTTP no inputs (key omitted), HTTP 404, HTTP server error, empty workflowID guard, exec with inputs, exec no inputs (flag omitted), exec wrapped result, exec error
- **GetWorkflowDAG**: HTTP success with inferred mode, HTTP 404, HTTP fallback mode with message, empty workflowID guard, exec fallback DAG (generic prompt field), exec workflow-not-found error, exec empty ID
- **Bearer token propagation**: Token forwarded on all HTTP calls
- **Parse helpers**: `parseDiscoveredWorkflowsJSON` with wrapped JSON, bare array, invalid JSON; `adaptDiscoveredWorkflows` with empty and non-empty input
- **Transport**: `decodeDaemonResponse` for 401, 404, error body, unknown status, nil-out success, JSON-decode success

### `client.go` (modified)

Added `WithWorkspaceID(id string) ClientOption` and a `workspaceID string` field to `Client`.
The daemon API routes are workspace-scoped (`/api/workspaces/{workspaceId}/...`); when `workspaceID` is empty, workspace-scoped methods skip HTTP and fall through directly to exec.

---

## Key Design Decisions

1. **No SQLite fallback for workflows**: Workflows are filesystem artefacts (`.tsx` files), not stored in the Smithers SQLite database. Only HTTP and exec tiers apply.

2. **`workspaceID` gates daemon HTTP**: Without a workspace ID configured, the client cannot construct valid daemon routes and skips HTTP automatically. This preserves backward compatibility — callers using only `WithAPIURL` (e.g., the old v1 server) are unaffected.

3. **Separate transport helpers**: `apiGetJSON`/`apiPostJSON`/`decodeDaemonResponse` are a third transport layer, distinct from:
   - `httpGetJSON`/`httpPostJSON` (legacy `{ok,data,error}` envelope)
   - `v1GetJSON`/`v1PostJSON`/`decodeV1Response` (direct JSON but v1-error-code aware)

   The daemon format is direct JSON with `{error, details?}` on failure — no envelope, no error code enum.

4. **`ListWorkflows` adapts DiscoveredWorkflow → Workflow**: The CLI returns the legacy `DiscoveredWorkflow` shape. The adapter maps `displayName → Name`, `entryFile → RelativePath`, sets `Status = active`. `WorkspaceID` is empty in the exec path (no daemon context available).

5. **`GetWorkflowDAG` exec fallback returns a stub**: Rather than returning an error when the daemon is unavailable, `execGetWorkflowDAG` verifies the workflow exists (via `workflow path`) and returns a single generic `prompt` field with `mode = "fallback"`. This lets the TUI always render an input form even without a running daemon.

6. **`RunWorkflow` returns `*RunSummary`**: Consistent with `ListRuns`/`GetRunSummary` in `runs.go`. The daemon POST `/api/workspaces/{id}/runs` body is `{workflowId, input?}` — the `input` key is omitted entirely when `inputs` is nil or empty.

7. **`parseDiscoveredWorkflowsJSON` key-detection**: Uses a `map[string]json.RawMessage` probe to detect the `workflows` key before attempting struct decode. This correctly handles an empty array in the wrapped format (where the naïve `len > 0` check would fail and fall through to the wrong branch).

---

## Coverage

Workflow-specific function coverage (from `go test -coverprofile`):

| Function | Coverage |
|----------|----------|
| `ListWorkflows` | 100% |
| `execListWorkflows` | 100% |
| `parseDiscoveredWorkflowsJSON` | 90.9% |
| `adaptDiscoveredWorkflows` | 100% |
| `GetWorkflowDefinition` | 100% |
| `execGetWorkflowDefinition` | 88.9% |
| `RunWorkflow` | 100% |
| `execRunWorkflow` | 90.0% |
| `parseWorkflowRunResultJSON` | 85.7% |
| `GetWorkflowDAG` | 90.9% |
| `execGetWorkflowDAG` | 100% |
| `apiGetJSON` | 81.8% |
| `apiPostJSON` | 75.0% |
| `decodeDaemonResponse` | 100% |

Overall package coverage: **70.6%** (held back by SQLite scan helpers that require a real DB and pre-existing uncovered code inherited from `client.go`).

---

## Files Created / Modified

- `/Users/williamcory/crush/internal/smithers/types_workflows.go` (new)
- `/Users/williamcory/crush/internal/smithers/workflows.go` (new)
- `/Users/williamcory/crush/internal/smithers/workflows_test.go` (new, 45 tests)
- `/Users/williamcory/crush/internal/smithers/client.go` (modified: `WithWorkspaceID`, `workspaceID` field)
- `/Users/williamcory/crush/.smithers/specs/implementation/eng-smithers-workflows-client.md` (this file)

## Files NOT Modified

- `internal/smithers/types.go` — untouched; workflow types live in `types_workflows.go`
- `internal/smithers/runs.go`, `types_runs.go` — untouched
- All other existing files — untouched
