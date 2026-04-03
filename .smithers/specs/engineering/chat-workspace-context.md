# chat-workspace-context Research

## Ticket Summary
Inject local workspace context (workflow directory, active runs) into the agent's system prompt during template execution.

## Acceptance Criteria
- Agent system prompt receives dynamic context such as `.WorkflowDir` and `.ActiveRuns`
- Agent is aware of currently active workflow runs without needing to execute a tool first

## Key Files
- `internal/agent/agent.go` — Contains the Agent struct and prompt template execution logic
- `internal/smithers/client.go` — Smithers HTTP client with methods like `ListRuns()` that return run data
- `internal/smithers/types.go` — Type definitions including `RunSummary`, `RunEvent`, etc.
- `internal/ui/model/ui.go` — UI model that holds `smithersClient` and `workflowDir`

## Current Architecture

### Agent System (agent.go)
- `Agent` struct has `systemPrompt string` and `domainPrompt string` fields
- `buildSystemPrompt()` method executes a Go template using a `promptData` struct
- Current `promptData` only has `Domain string` field
- Template is executed via `text/template` and the result becomes the system prompt
- `NewAgent()` calls `buildSystemPrompt()` during construction

### Smithers Client (client.go)
- `Client` struct wraps an HTTP client talking to a local smithers server
- Has `ListRuns()` method returning `[]RunSummary`
- `RunSummary` includes: `RunID`, `WorkflowPath`, `Status`, `CreatedAtMs`, `UpdatedAtMs`
- Also has methods for workflows, memory, cron schedules, etc.

### UI Model (model/ui.go)
- `Model` struct has `smithersClient *smithers.Client` and `workflowDir string`
- The agent is constructed in `initAgent()` method
- Currently passes domain prompt string to `NewAgent()`

## Implementation Plan

### Step 1: Extend promptData in agent.go
Add `WorkflowDir` and `ActiveRuns` fields to the `promptData` struct:
```go
type promptData struct {
    Domain     string
    WorkflowDir string
    ActiveRuns  []RunContext
}

type RunContext struct {
    RunID        string
    WorkflowPath string
    Status       string
}
```

### Step 2: Update NewAgent to accept workspace context
Modify `NewAgent()` signature to accept a `WorkspaceContext` parameter:
```go
type WorkspaceContext struct {
    WorkflowDir string
    ActiveRuns  []RunContext
}

func NewAgent(model, systemPrompt, domainPrompt string, ctx WorkspaceContext) *Agent
```

### Step 3: Update buildSystemPrompt
Pass the full context into template execution so `.WorkflowDir` and `.ActiveRuns` are available in the template.

### Step 4: Update UI model's initAgent
In `model/ui.go`, before constructing the agent:
1. Call `smithersClient.ListRuns()` to get active runs
2. Filter for active statuses (running, pending)
3. Build `WorkspaceContext` with `workflowDir` and filtered runs
4. Pass context to `NewAgent()`

### Step 5: Update system prompt template
Add conditional sections to the system prompt template that render workspace context when available:
```
{{if .WorkflowDir}}Workflow directory: {{.WorkflowDir}}{{end}}
{{if .ActiveRuns}}Active runs:
{{range .ActiveRuns}}- {{.RunID}}: {{.WorkflowPath}} ({{.Status}})
{{end}}{{end}}
```

## Dependencies
- `chat-domain-system-prompt` (already implemented — domain prompt template system exists)

## Risks / Considerations
- ListRuns() makes an HTTP call; need to handle errors gracefully (don't block agent creation if smithers is unavailable)
- Active runs are a point-in-time snapshot; they may become stale during a long conversation
- The workspace context is injected at agent construction time, not dynamically refreshed