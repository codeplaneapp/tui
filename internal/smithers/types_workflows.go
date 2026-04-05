package smithers

// WorkflowStatus mirrors the workflowStatusSchema from
// smithers/packages/shared/src/schemas/workflow.ts.
type WorkflowStatus string

const (
	WorkflowStatusDraft    WorkflowStatus = "draft"
	WorkflowStatusActive   WorkflowStatus = "active"
	WorkflowStatusHot      WorkflowStatus = "hot"
	WorkflowStatusArchived WorkflowStatus = "archived"
)

// Workflow is a discovered workflow record returned by the daemon API.
// Maps to the Workflow type in smithers/packages/shared/src/schemas/workflow.ts.
//
// Returned by GET /api/workspaces/{workspaceId}/workflows.
type Workflow struct {
	// ID is the workflow's unique identifier (the directory or file slug).
	ID string `json:"id"`
	// WorkspaceID is the parent workspace that owns this workflow.
	WorkspaceID string `json:"workspaceId"`
	// Name is the human-readable display name.
	Name string `json:"name"`
	// RelativePath is the path to the workflow entry file relative to the
	// workspace root, e.g. ".smithers/workflows/my-flow.tsx".
	RelativePath string `json:"relativePath"`
	// Status reflects the authoring lifecycle state.
	Status WorkflowStatus `json:"status"`
	// UpdatedAt is the ISO-8601 timestamp of the last modification (optional).
	UpdatedAt *string `json:"updatedAt,omitempty"`
}

// WorkflowDefinition is the full workflow document including source code.
// Maps to WorkflowDocument in smithers/packages/shared/src/schemas/workflow.ts.
//
// Returned by GET /api/workspaces/{workspaceId}/workflows/{workflowId}.
type WorkflowDefinition struct {
	Workflow
	// Source is the raw TypeScript/TSX workflow source code.
	Source string `json:"source"`
}

// WorkflowTask represents a single launchable field (input parameter) extracted
// from the workflow's first task or inferred from the workflow schema.
// Maps to WorkflowLaunchField in smithers/packages/shared/src/schemas/workflow.ts.
type WorkflowTask struct {
	// Key is the parameter name, e.g. "prompt" or "ticketId".
	Key string `json:"key"`
	// Label is the human-readable label shown in input forms.
	Label string `json:"label"`
	// Type is the input type; currently always "string".
	Type string `json:"type"`
}

// DAGDefinition describes the launchable input interface of a workflow — the
// fields the workflow expects when started. It maps to WorkflowLaunchFieldsResponse
// in smithers/packages/shared/src/schemas/workflow.ts.
//
// Returned by GET /api/workspaces/{workspaceId}/workflows/{workflowId}/launch-fields.
//
// Note: the daemon does static analysis of the TSX source to infer fields;
// when analysis fails it falls back to a single generic "prompt" field.
type DAGDefinition struct {
	// WorkflowID is the ID of the workflow these fields belong to.
	WorkflowID string `json:"workflowId"`
	// Mode is "inferred" when fields were extracted from the workflow source, or
	// "fallback" when static analysis failed and a generic prompt field was used.
	Mode string `json:"mode"`
	// EntryTaskID is the ID of the first task that receives the run input, or nil
	// when it could not be determined.
	EntryTaskID *string `json:"entryTaskId"`
	// Fields is the ordered list of launchable input fields.
	Fields []WorkflowTask `json:"fields"`
	// Message contains a human-readable note from the analyser (e.g. a warning
	// that analysis fell back to generic mode).
	Message *string `json:"message,omitempty"`
}

// DiscoveredWorkflow is returned by the legacy `smithers workflow list` CLI
// command (old smithers without the daemon).
// Maps to DiscoveredWorkflow in smithers/src/cli/workflows.ts.
type DiscoveredWorkflow struct {
	// ID is the workflow slug, derived from the filename without extension.
	ID string `json:"id"`
	// DisplayName is the human-readable name read from the file header comment
	// or derived from the ID.
	DisplayName string `json:"displayName"`
	// EntryFile is the absolute path to the workflow TSX file.
	EntryFile string `json:"entryFile"`
	// SourceType is one of "seeded", "user", or "generated".
	SourceType string `json:"sourceType"`
}

// daemonErrorResponse is the error shape returned by the daemon's toErrorResponse
// helper: { "error": "message", "details": ... }
type daemonErrorResponse struct {
	Error   string `json:"error"`
	Details any    `json:"details,omitempty"`
}
