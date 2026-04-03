# Research Summary: eng-smithers-workflows-client

## Ticket Overview
Build the Smithers Workflow API Client subsystem in `internal/smithers/client.go` with `ListWorkflows`, `GetWorkflow`, and `RunWorkflow` methods. The client must correctly deserialize workflow schemas and parameters from the Smithers HTTP API, and unit tests should be added.

## Current State of the Codebase

### Existing Client (`internal/smithers/client.go`)
- Already has a `Client` struct with `BaseURL` and `http.Client`
- Has a `NewClient(baseURL string)` constructor
- Implements `ListRuns()` and `GetRun(id string)` methods
- Uses standard Go HTTP patterns with JSON deserialization
- Returns typed responses from `internal/smithers/types.go`

### Existing Types (`internal/smithers/types.go`)
- Defines `Run` struct with fields: ID, WorkflowID, Status, Input, Output, CreatedAt, UpdatedAt
- Defines `ListRunsResponse` with a `Runs []Run` field
- No workflow-specific types exist yet

### Server-Side Workflow Endpoints (from `../smithers/src/server/index.ts`)
The Smithers server exposes these workflow-related endpoints:
1. **GET `/api/workflows`** - Lists all workflows, returns array of workflow objects with: id, name, description, nodes (array of {id, type, agent}), edges (array of {from, to}), triggers (array of {type, schedule})
2. **GET `/api/workflows/:id`** - Gets a single workflow by ID, returns same shape as above with 404 on not found
3. **POST `/api/workflows/:id/run`** - Runs a workflow, accepts JSON body with `input` field, returns a Run object with: id, workflowId, status ("running"), input, output (null), createdAt, updatedAt

### Workflow Data Shape (from server code)
```
Workflow {
  id: string
  name: string
  description: string
  nodes: [{ id: string, type: string, agent: string }]
  edges: [{ from: string, to: string }]
  triggers: [{ type: string, schedule: string }]
}
```

### Test Infrastructure (`tests/tui/`)
- Uses Bun test runner with `bun:test`
- Has a `TUITestInstance` interface with `snapshot()`, `write(text)`, and `terminate()` methods
- `launchTUI()` helper spawns the TUI process and returns a backend for interaction
- Tests use `expect(snapshot).toContain()` pattern for assertions
- Entry point is at `cmd/tui/main.ts` (referenced as `TUI_ENTRY`)

### Design Documents
- PRD (`docs/smithers-tui/01-PRD.md`): Describes Smithers TUI as a terminal-based control plane for managing AI agent workflows
- Engineering doc (`docs/smithers-tui/03-ENGINEERING.md`): Specifies Go-based TUI using Bubble Tea framework, with `internal/smithers/` package for API client
- Architecture uses view-model pattern with router, views are in `internal/ui/views/`

## Implementation Plan

### 1. Add Workflow Types to `internal/smithers/types.go`
- `WorkflowNode` struct: ID, Type, Agent (all string)
- `WorkflowEdge` struct: From, To (all string)
- `WorkflowTrigger` struct: Type, Schedule (all string)
- `Workflow` struct: ID, Name, Description, Nodes, Edges, Triggers
- `ListWorkflowsResponse` struct wrapping `[]Workflow`
- `RunWorkflowRequest` struct with Input field
- `RunWorkflowResponse` reusing the existing `Run` type

### 2. Add Client Methods to `internal/smithers/client.go`
- `ListWorkflows() ([]Workflow, error)` - GET /api/workflows
- `GetWorkflow(id string) (*Workflow, error)` - GET /api/workflows/:id
- `RunWorkflow(id string, input map[string]any) (*Run, error)` - POST /api/workflows/:id/run

### 3. Add Unit Tests
- Create `internal/smithers/client_test.go` (or add to existing test file)
- Use `net/http/httptest` to create mock server
- Test each method: ListWorkflows, GetWorkflow, RunWorkflow
- Test error cases: 404, malformed JSON, network errors

### Key Patterns to Follow
- Match existing code style from ListRuns/GetRun methods
- Use `json.NewDecoder(resp.Body).Decode()` pattern
- Return errors with `fmt.Errorf` wrapping
- JSON tags on struct fields matching the API response keys