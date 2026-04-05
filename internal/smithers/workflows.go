package smithers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Sentinel errors for workflow operations.
var (
	// ErrWorkflowNotFound is returned when the requested workflow does not exist.
	ErrWorkflowNotFound = errors.New("workflow not found")
)

// --- ListWorkflows ---

// ListWorkflows returns all workflows discoverable from the project.
//
// Routes (in priority order):
//  1. HTTP GET /api/workspaces/{workspaceId}/workflows  (daemon API)
//  2. exec `smithers workflow list --format json`       (CLI fallback)
//
// There is no SQLite fallback because workflows are filesystem-based artefacts,
// not stored in the Smithers SQLite database.
//
// When a workspaceID has been configured via WithWorkspaceID and the server is
// available, the daemon API is used and returns full Workflow records.
// Otherwise the exec path returns DiscoveredWorkflow records adapted into
// the Workflow type.
func (c *Client) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	// 1. Try HTTP (requires workspaceID + server).
	if c.workspaceID != "" && c.isServerAvailable() {
		var workflows []Workflow
		path := "/api/workspaces/" + url.PathEscape(c.workspaceID) + "/workflows"
		err := c.apiGetJSON(ctx, path, &workflows)
		switch {
		case err == nil:
			return workflows, nil
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return nil, err
		}
	}

	// 2. Fall back to exec.
	return execListWorkflows(ctx, c)
}

// execListWorkflows shells out to `smithers workflow list --format json`.
// The CLI returns { "workflows": [DiscoveredWorkflow, ...] }; each entry is
// adapted into a Workflow record for API uniformity.
func execListWorkflows(ctx context.Context, c *Client) ([]Workflow, error) {
	out, err := c.execSmithers(ctx, "workflow", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseDiscoveredWorkflowsJSON(out)
}

// parseDiscoveredWorkflowsJSON parses the CLI JSON output and maps
// DiscoveredWorkflow entries into Workflow values.
func parseDiscoveredWorkflowsJSON(data []byte) ([]Workflow, error) {
	// The CLI wraps the array in a { "workflows": [...] } envelope.
	// Check if the top-level value is an object with a "workflows" key.
	var wrapper struct {
		Workflows []DiscoveredWorkflow `json:"workflows"`
	}
	// Use a discriminator: if it parses as an object with "workflows" key, use it.
	// We detect this by checking that the outer parse succeeds as an object
	// (bare arrays would fail to parse into this struct without error only if
	// Workflows ends up nil — so we check the field explicitly via a map first).
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err == nil {
		if _, hasKey := probe["workflows"]; hasKey {
			if err := json.Unmarshal(data, &wrapper); err != nil {
				return nil, fmt.Errorf("parse workflow list: %w", err)
			}
			return adaptDiscoveredWorkflows(wrapper.Workflows), nil
		}
	}
	// Try a bare array as a fallback.
	var discovered []DiscoveredWorkflow
	if err := json.Unmarshal(data, &discovered); err != nil {
		return nil, fmt.Errorf("parse workflow list: %w", err)
	}
	return adaptDiscoveredWorkflows(discovered), nil
}

// adaptDiscoveredWorkflows converts DiscoveredWorkflow CLI records into the
// canonical Workflow type used by the daemon API.
func adaptDiscoveredWorkflows(discovered []DiscoveredWorkflow) []Workflow {
	workflows := make([]Workflow, len(discovered))
	for i, d := range discovered {
		workflows[i] = Workflow{
			ID:           d.ID,
			Name:         d.DisplayName,
			RelativePath: d.EntryFile,
			Status:       WorkflowStatusActive,
		}
	}
	return workflows
}

// --- GetWorkflowDefinition ---

// GetWorkflowDefinition returns the full workflow definition including source
// code for the given workflowID.
//
// Routes (in priority order):
//  1. HTTP GET /api/workspaces/{workspaceId}/workflows/{workflowId}  (daemon API)
//  2. exec `smithers workflow path {workflowId} --format json`       (CLI fallback)
//
// There is no SQLite fallback for the same reason as ListWorkflows.
func (c *Client) GetWorkflowDefinition(ctx context.Context, workflowID string) (*WorkflowDefinition, error) {
	if workflowID == "" {
		return nil, fmt.Errorf("workflowID is required")
	}

	// 1. Try HTTP (requires workspaceID + server).
	if c.workspaceID != "" && c.isServerAvailable() {
		path := "/api/workspaces/" + url.PathEscape(c.workspaceID) +
			"/workflows/" + url.PathEscape(workflowID)
		var def WorkflowDefinition
		err := c.apiGetJSON(ctx, path, &def)
		switch {
		case err == nil:
			return &def, nil
		case errors.Is(err, ErrWorkflowNotFound):
			return nil, ErrWorkflowNotFound
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return nil, err
		}
	}

	// 2. Fall back to exec.
	return execGetWorkflowDefinition(ctx, c, workflowID)
}

// execGetWorkflowDefinition resolves the workflow entry file via the CLI and
// returns a WorkflowDefinition populated from exec output.
func execGetWorkflowDefinition(ctx context.Context, c *Client, workflowID string) (*WorkflowDefinition, error) {
	out, err := c.execSmithers(ctx, "workflow", "path", workflowID, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", workflowID, err)
	}
	var pathResult struct {
		ID         string `json:"id"`
		Path       string `json:"path"`
		SourceType string `json:"sourceType"`
	}
	if jsonErr := json.Unmarshal(out, &pathResult); jsonErr != nil {
		return nil, fmt.Errorf("parse workflow path: %w", jsonErr)
	}
	if pathResult.ID == "" {
		return nil, ErrWorkflowNotFound
	}
	return &WorkflowDefinition{
		Workflow: Workflow{
			ID:           pathResult.ID,
			Name:         pathResult.ID,
			RelativePath: pathResult.Path,
			Status:       WorkflowStatusActive,
		},
	}, nil
}

// --- RunWorkflow ---

// RunWorkflow starts a new workflow execution and returns the initial run record.
//
// Routes (in priority order):
//  1. HTTP POST /api/workspaces/{workspaceId}/runs  (daemon API)
//     body: { "workflowId": "...", "input": { ... } }
//  2. exec `smithers workflow run {workflowID}` with JSON input via stdin or
//     exec `smithers up {entryFile}` when the daemon is not available.
//
// The returned RunSummary reflects the initial status ("running") of the newly
// started run. There is no SQLite fallback because run creation is a mutation.
func (c *Client) RunWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (*RunSummary, error) {
	if workflowID == "" {
		return nil, fmt.Errorf("workflowID is required")
	}

	// 1. Try HTTP (requires workspaceID + server).
	if c.workspaceID != "" && c.isServerAvailable() {
		path := "/api/workspaces/" + url.PathEscape(c.workspaceID) + "/runs"
		reqBody := map[string]any{"workflowId": workflowID}
		if len(inputs) > 0 {
			reqBody["input"] = inputs
		}
		var run RunSummary
		err := c.apiPostJSON(ctx, path, reqBody, &run)
		switch {
		case err == nil:
			return &run, nil
		case errors.Is(err, ErrWorkflowNotFound):
			return nil, ErrWorkflowNotFound
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return nil, err
		}
	}

	// 2. Fall back to exec.
	return execRunWorkflow(ctx, c, workflowID, inputs)
}

// execRunWorkflow shells out to `smithers workflow run {workflowID} --format json`.
// Input values are passed as --input key=value pairs when present.
func execRunWorkflow(ctx context.Context, c *Client, workflowID string, inputs map[string]any) (*RunSummary, error) {
	args := []string{"workflow", "run", workflowID, "--format", "json"}
	if len(inputs) > 0 {
		inputJSON, err := json.Marshal(inputs)
		if err != nil {
			return nil, fmt.Errorf("marshal workflow inputs: %w", err)
		}
		args = append(args, "--input", string(inputJSON))
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseWorkflowRunResultJSON(out)
}

// parseWorkflowRunResultJSON parses the exec output of `smithers workflow run`
// into a RunSummary. The CLI may return a bare RunSummary or a wrapper.
func parseWorkflowRunResultJSON(data []byte) (*RunSummary, error) {
	var run RunSummary
	if err := json.Unmarshal(data, &run); err == nil && run.RunID != "" {
		return &run, nil
	}
	// Try { "run": {...} } wrapper.
	var wrapper struct {
		Run RunSummary `json:"run"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Run.RunID != "" {
		return &wrapper.Run, nil
	}
	return nil, fmt.Errorf("parse workflow run result: unexpected shape: %s", data)
}

// --- GetWorkflowDAG ---

// GetWorkflowDAG returns the DAG definition for the given workflow — the
// ordered list of input fields the workflow expects at launch time.
//
// Routes (in priority order):
//  1. HTTP GET /api/workspaces/{workspaceId}/workflows/{workflowId}/launch-fields
//  2. exec `smithers workflow path {workflowID} --format json` (returns a stub
//     DAGDefinition with a single generic "prompt" field, mode "fallback").
//
// There is no SQLite fallback because this data is derived from static workflow
// source analysis, not from the database.
func (c *Client) GetWorkflowDAG(ctx context.Context, workflowID string) (*DAGDefinition, error) {
	if workflowID == "" {
		return nil, fmt.Errorf("workflowID is required")
	}

	// 1. Try HTTP (requires workspaceID + server).
	if c.workspaceID != "" && c.isServerAvailable() {
		path := "/api/workspaces/" + url.PathEscape(c.workspaceID) +
			"/workflows/" + url.PathEscape(workflowID) + "/launch-fields"
		var dag DAGDefinition
		err := c.apiGetJSON(ctx, path, &dag)
		switch {
		case err == nil:
			return &dag, nil
		case errors.Is(err, ErrWorkflowNotFound):
			return nil, ErrWorkflowNotFound
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return nil, err
		}
	}

	// 2. Fall back to exec — return a minimal stub so the caller can
	// always render an input form even without a running daemon.
	return execGetWorkflowDAG(ctx, c, workflowID)
}

// execGetWorkflowDAG shells out to resolve the workflow and returns a
// fallback DAGDefinition with a single generic "prompt" field.
func execGetWorkflowDAG(ctx context.Context, c *Client, workflowID string) (*DAGDefinition, error) {
	// Use workflow path to verify the workflow exists before returning a stub.
	out, err := c.execSmithers(ctx, "workflow", "path", workflowID, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", workflowID, err)
	}
	var pathResult struct {
		ID string `json:"id"`
	}
	if jsonErr := json.Unmarshal(out, &pathResult); jsonErr != nil || pathResult.ID == "" {
		return nil, ErrWorkflowNotFound
	}

	label := "Prompt"
	fallbackMsg := "Launch fields inferred via CLI fallback; daemon API unavailable."
	return &DAGDefinition{
		WorkflowID: workflowID,
		Mode:       "fallback",
		Fields: []WorkflowTask{
			{Key: "prompt", Label: label, Type: "string"},
		},
		Message: &fallbackMsg,
	}, nil
}

// --- daemon API transport helpers ---

// apiGetJSON performs a GET against a daemon /api/* path that returns direct
// JSON (no envelope). Maps HTTP status codes to typed errors.
func (c *Client) apiGetJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+path, nil)
	if err != nil {
		return err
	}
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrServerUnavailable
	}
	defer resp.Body.Close()

	return decodeDaemonResponse(resp, out)
}

// apiPostJSON performs a POST against a daemon /api/* path with a direct JSON
// body and decodes the direct JSON response.
func (c *Client) apiPostJSON(ctx context.Context, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrServerUnavailable
	}
	defer resp.Body.Close()

	return decodeDaemonResponse(resp, out)
}

// decodeDaemonResponse maps HTTP status codes to typed errors and decodes the
// response body. The daemon returns plain JSON errors: { "error": "msg" }.
func decodeDaemonResponse(resp *http.Response, out any) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrWorkflowNotFound
	}

	if resp.StatusCode >= 300 {
		var errBody daemonErrorResponse
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errBody); jsonErr == nil && errBody.Error != "" {
			return fmt.Errorf("smithers daemon error: %s", errBody.Error)
		}
		return fmt.Errorf("smithers daemon: unexpected status %d", resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode daemon response: %w", err)
		}
	}
	return nil
}
