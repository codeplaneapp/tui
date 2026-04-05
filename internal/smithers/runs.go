package smithers

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Sentinel errors for the runs v1 API.
var (
	// ErrRunNotFound is returned when the requested run does not exist.
	ErrRunNotFound = errors.New("run not found")
	// ErrRunNotActive is returned when an operation requires an active run
	// but the run has already reached a terminal state (e.g. CancelRun on a
	// finished run).
	ErrRunNotActive = errors.New("run is not active")
	// ErrUnauthorized is returned when the server responds with HTTP 401.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrDBNotConfigured is returned when the server was not started with a
	// database and the endpoint requires one.
	ErrDBNotConfigured = errors.New("smithers server has no database configured")
)

// --- ListRuns ---

// ListRuns returns a list of run summaries, optionally filtered by status and
// capped by the limit in filter.
//
// Routes (in priority order):
//  1. HTTP GET /v1/runs?limit=N&status=S
//  2. Direct SQLite read from _smithers_runs
//  3. exec `smithers ps --format json`
func (c *Client) ListRuns(ctx context.Context, filter RunFilter) ([]RunSummary, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	// 1. Try HTTP.
	if c.isServerAvailable() {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", limit))
		if filter.Status != "" {
			params.Set("status", filter.Status)
		}
		path := "/v1/runs?" + params.Encode()

		var runs []RunSummary
		err := c.v1GetJSON(ctx, path, &runs)
		switch {
		case err == nil:
			return runs, nil
		case errors.Is(err, ErrDBNotConfigured):
			// Server running but no DB configured — return empty list, not error.
			return nil, nil
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to lower tiers.
		default:
			return nil, err
		}
	}

	// 2. Try direct SQLite.
	if c.db != nil {
		return sqliteListRuns(ctx, c, filter, limit)
	}

	// 3. Fall back to exec.
	return execListRuns(ctx, c, filter)
}

// sqliteListRuns reads runs directly from the SQLite _smithers_runs table.
func sqliteListRuns(ctx context.Context, c *Client, filter RunFilter, limit int) ([]RunSummary, error) {
	query := `SELECT run_id, workflow_name, workflow_path, status,
		started_at_ms, finished_at_ms, error_json
		FROM _smithers_runs`
	var args []any
	if filter.Status != "" {
		query += " WHERE status = ?"
		args = append(args, filter.Status)
	}
	query += " ORDER BY started_at_ms DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := c.queryDB(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanRunSummaries(rows)
}

// execListRuns shells out to `smithers ps --format json`.
func execListRuns(ctx context.Context, c *Client, filter RunFilter) ([]RunSummary, error) {
	args := []string{"ps", "--format", "json"}
	if filter.Status != "" {
		args = append(args, "--status", filter.Status)
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		// smithers ps returns exit 1 when no DB exists.
		// The error JSON goes to stdout (captured by ExecError.Stderr or lost).
		// Treat any exec error from ps as "no runs" since the user hasn't started any yet.
		var execErr *ExecError
		if errors.As(err, &execErr) {
			// Check both stderr and stdout for known error codes
			combined := execErr.Stderr + string(out)
			if strings.Contains(combined, "PS_FAILED") ||
				strings.Contains(combined, "No smithers.db") ||
				strings.Contains(combined, "CLI_DB_NOT_FOUND") {
				return nil, nil
			}
			// Even for unknown exec errors from ps, return empty — ps failing just means no runs
			return nil, nil
		}
		// Binary not found is graceful empty
		if errors.Is(err, ErrBinaryNotFound) {
			return nil, nil
		}
		return nil, err
	}
	// Also check if the output itself is an error response
	if len(out) > 0 && bytes.Contains(out, []byte("PS_FAILED")) {
		return nil, nil
	}
	return parseRunSummariesJSON(out)
}

// --- GetRunSummary ---

// GetRunSummary returns the v1 run summary for the given runID.
//
// Routes (in priority order):
//  1. HTTP GET /v1/runs/:id
//  2. Direct SQLite read from _smithers_runs
//  3. exec `smithers inspect <runID> --format json`
//
// Use GetRun (from client.go) if you need the legacy Run shape used by the
// time-travel subsystem.
func (c *Client) GetRunSummary(ctx context.Context, runID string) (*RunSummary, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}

	// 1. Try HTTP.
	if c.isServerAvailable() {
		var run RunSummary
		err := c.v1GetJSON(ctx, "/v1/runs/"+url.PathEscape(runID), &run)
		switch {
		case err == nil:
			return &run, nil
		case errors.Is(err, ErrRunNotFound):
			return nil, ErrRunNotFound
		case errors.Is(err, ErrServerUnavailable):
			// Fall through.
		default:
			return nil, err
		}
	}

	// 2. Try direct SQLite.
	if c.db != nil {
		run, err := sqliteGetRunSummary(ctx, c, runID)
		if err != nil && !errors.Is(err, ErrRunNotFound) {
			return nil, err
		}
		if run != nil {
			return run, nil
		}
	}

	// 3. Fall back to exec.
	return execGetRunSummary(ctx, c, runID)
}

// sqliteGetRunSummary reads a single run summary from SQLite.
func sqliteGetRunSummary(ctx context.Context, c *Client, runID string) (*RunSummary, error) {
	rows, err := c.queryDB(ctx,
		`SELECT run_id, workflow_name, workflow_path, status,
		started_at_ms, finished_at_ms, error_json
		FROM _smithers_runs WHERE run_id = ? LIMIT 1`,
		runID)
	if err != nil {
		return nil, err
	}
	runs, err := scanRunSummaries(rows)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, ErrRunNotFound
	}
	return &runs[0], nil
}

// execGetRunSummary shells out to `smithers inspect <runID> --format json`.
func execGetRunSummary(ctx context.Context, c *Client, runID string) (*RunSummary, error) {
	out, err := c.execSmithers(ctx, "inspect", runID, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseRunSummaryJSON(out)
}

// --- InspectRun ---

// InspectRun returns enriched run details including per-node task state.
//
// Routes (in priority order):
//  1. GetRunSummary (HTTP/SQLite/exec) + SQLite for task nodes
//  2. GetRunSummary + exec `smithers inspect` for task nodes
func (c *Client) InspectRun(ctx context.Context, runID string) (*RunInspection, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}

	run, err := c.GetRunSummary(ctx, runID)
	if err != nil {
		return nil, err
	}

	inspection := &RunInspection{RunSummary: *run}

	// Enrich with node-level tasks (best-effort; errors are silently swallowed).
	if tasks, taskErr := getRunTasks(ctx, c, runID); taskErr == nil {
		inspection.Tasks = tasks
	}

	return inspection, nil
}

// getRunTasks fetches per-node task records for a run.
// SQLite is preferred; falls back to exec inspect.
func getRunTasks(ctx context.Context, c *Client, runID string) ([]RunTask, error) {
	if c.db != nil {
		return sqliteGetRunTasks(ctx, c, runID)
	}
	return execGetRunTasks(ctx, c, runID)
}

// sqliteGetRunTasks reads node rows from SQLite.
func sqliteGetRunTasks(ctx context.Context, c *Client, runID string) ([]RunTask, error) {
	rows, err := c.queryDB(ctx,
		`SELECT node_id, label, iteration, state, last_attempt, updated_at_ms
		FROM _smithers_nodes WHERE run_id = ?
		ORDER BY updated_at_ms ASC`,
		runID)
	if err != nil {
		return nil, err
	}
	return scanRunTasks(rows)
}

// execGetRunTasks shells out to `smithers inspect <runID> --nodes --format json`
// and parses the tasks array.
func execGetRunTasks(ctx context.Context, c *Client, runID string) ([]RunTask, error) {
	out, err := c.execSmithers(ctx, "inspect", runID, "--nodes", "--format", "json")
	if err != nil {
		return nil, err
	}
	// The inspect command may return either a { tasks: [...] } / { nodes: [...] }
	// wrapper or a bare array; try both forms.
	var wrapper struct {
		Tasks []RunTask `json:"tasks"`
		Nodes []RunTask `json:"nodes"`
	}
	if jsonErr := json.Unmarshal(out, &wrapper); jsonErr == nil {
		if len(wrapper.Tasks) > 0 {
			return wrapper.Tasks, nil
		}
		if len(wrapper.Nodes) > 0 {
			return wrapper.Nodes, nil
		}
	}
	var tasks []RunTask
	if jsonErr := json.Unmarshal(out, &tasks); jsonErr != nil {
		return nil, fmt.Errorf("parse run tasks: %w", jsonErr)
	}
	return tasks, nil
}

// --- CancelRun ---

// CancelRun cancels an active run.
//
// Routes (in priority order):
//  1. HTTP POST /v1/runs/:id/cancel
//  2. exec `smithers cancel <runID>`
func (c *Client) CancelRun(ctx context.Context, runID string) error {
	if runID == "" {
		return fmt.Errorf("runID is required")
	}

	// 1. Try HTTP.
	if c.isServerAvailable() {
		err := c.v1PostJSON(ctx, "/v1/runs/"+url.PathEscape(runID)+"/cancel", nil, nil)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrRunNotActive):
			return ErrRunNotActive
		case errors.Is(err, ErrRunNotFound):
			return ErrRunNotFound
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return err
		}
	}

	// 2. Fall back to exec.
	_, err := c.execSmithers(ctx, "cancel", runID)
	return err
}

// --- ApproveNode ---

// ApproveNode approves a pending approval gate on a specific node within a run.
//
// Routes (in priority order):
//  1. HTTP POST /v1/runs/:runID/approve/:nodeID
//  2. exec `smithers approve <runID> --node <nodeID>`
func (c *Client) ApproveNode(ctx context.Context, runID, nodeID string) error {
	if runID == "" {
		return fmt.Errorf("runID is required")
	}
	if nodeID == "" {
		return fmt.Errorf("nodeID is required")
	}

	// 1. Try HTTP.
	if c.isServerAvailable() {
		err := c.v1PostJSON(ctx, "/v1/runs/"+url.PathEscape(runID)+"/approve/"+url.PathEscape(nodeID), nil, nil)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrRunNotFound):
			return ErrRunNotFound
		case errors.Is(err, ErrUnauthorized):
			return ErrUnauthorized
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return err
		}
	}

	// 2. Fall back to exec.
	_, err := c.execSmithers(ctx, "approve", runID, "--node", nodeID)
	return err
}

// --- DenyNode ---

// DenyNode denies a pending approval gate on a specific node within a run.
//
// Routes (in priority order):
//  1. HTTP POST /v1/runs/:runID/deny/:nodeID
//  2. exec `smithers deny <runID> --node <nodeID>`
func (c *Client) DenyNode(ctx context.Context, runID, nodeID string) error {
	if runID == "" {
		return fmt.Errorf("runID is required")
	}
	if nodeID == "" {
		return fmt.Errorf("nodeID is required")
	}

	// 1. Try HTTP.
	if c.isServerAvailable() {
		err := c.v1PostJSON(ctx, "/v1/runs/"+url.PathEscape(runID)+"/deny/"+url.PathEscape(nodeID), nil, nil)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrRunNotFound):
			return ErrRunNotFound
		case errors.Is(err, ErrUnauthorized):
			return ErrUnauthorized
		case errors.Is(err, ErrServerUnavailable):
			// Fall through to exec.
		default:
			return err
		}
	}

	// 2. Fall back to exec.
	_, err := c.execSmithers(ctx, "deny", runID, "--node", nodeID)
	return err
}

// --- WaitForAllEvents ---

// WaitForAllEvents returns a tea.Cmd that blocks on the next message from the
// channel returned by Client.StreamAllEvents.
//
// StreamAllEvents sends pre-typed values into an interface{} channel:
//
//	RunEventMsg, RunEventErrorMsg, or RunEventDoneMsg.
//
// WaitForAllEvents returns the value directly so Bubble Tea routes it
// to the correct case in the calling view's Update method.
//
// Self-re-scheduling pattern:
//
//	case smithers.RunEventMsg:
//	    applyEvent(msg.Event)
//	    return v, smithers.WaitForAllEvents(v.allEventsCh)
//
// Important: if ch is nil, <-ch blocks forever — only dispatch this cmd after
// v.allEventsCh has been assigned from a runsStreamReadyMsg.
func WaitForAllEvents(ch <-chan interface{}) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return RunEventDoneMsg{}
		}
		return msg
	}
}

// --- StreamChat ---

// StreamChat opens an SSE connection to GET /v1/runs/:id/chat/stream and
// delivers ChatBlock events as they arrive on the returned channel.
// The caller must cancel ctx to close the stream.
// SSE streaming requires a live server — there is no SQLite or exec fallback.
func (c *Client) StreamChat(ctx context.Context, runID string) (<-chan ChatBlock, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}
	if c.apiURL == "" {
		return nil, ErrServerUnavailable
	}

	streamURL := c.apiURL + "/v1/runs/" + url.PathEscape(runID) + "/chat/stream"
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	// Use a streaming-safe HTTP client with no timeout.
	streamClient := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   0,
	}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, ErrServerUnavailable
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		resp.Body.Close()
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrRunNotFound
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream chat: unexpected status %d", resp.StatusCode)
	}

	ch := make(chan ChatBlock, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

		var dataBuf strings.Builder

		for scanner.Scan() {
			line := scanner.Text()

			switch {
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line, "data:")
				if len(data) > 0 && data[0] == ' ' {
					data = data[1:]
				}
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(data)

			case strings.HasPrefix(line, ":"):
				// Heartbeat/comment — ignored.

			case strings.HasPrefix(line, "event:"), strings.HasPrefix(line, "id:"), strings.HasPrefix(line, "retry:"):
				// Other SSE fields — ignored for chat stream.

			case line == "":
				// Blank line dispatches the accumulated event.
				if dataBuf.Len() == 0 {
					continue
				}
				raw := dataBuf.String()
				dataBuf.Reset()

				var block ChatBlock
				if jsonErr := json.Unmarshal([]byte(raw), &block); jsonErr == nil {
					select {
					case ch <- block:
					case <-ctx.Done():
						return
					}
				}
			}

			if ctx.Err() != nil {
				return
			}
		}
	}()

	return ch, nil
}

// WaitForChatBlock returns a tea.Cmd that blocks until the next ChatBlock
// arrives on ch.  When the channel closes it returns ChatStreamDoneMsg.
// Views use this in a self-re-scheduling pattern:
//
//	case smithers.ChatBlockMsg:
//	    // handle block...
//	    return v, smithers.WaitForChatBlock(v.runID, v.blockCh)
func WaitForChatBlock(runID string, ch <-chan ChatBlock) tea.Cmd {
	return func() tea.Msg {
		block, ok := <-ch
		if !ok {
			return ChatStreamDoneMsg{RunID: runID}
		}
		return ChatBlockMsg{RunID: runID, Block: block}
	}
}

// --- HijackRun ---

// HijackRun pauses the agent on the given run and returns session metadata
// for native TUI handoff via tea.ExecProcess.
// Routes: HTTP POST /v1/runs/:id/hijack
func (c *Client) HijackRun(ctx context.Context, runID string) (*HijackSession, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}
	if !c.isServerAvailable() {
		return nil, ErrServerUnavailable
	}
	var session HijackSession
	if err := c.v1PostJSON(ctx, "/v1/runs/"+url.PathEscape(runID)+"/hijack", nil, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// --- StreamRunEvents ---

// StreamRunEvents opens an SSE connection to GET /v1/runs/:id/events and
// returns a channel that receives values of three types:
//   - RunEventMsg — a decoded event from the stream
//   - RunEventErrorMsg — a non-fatal parse error (stream continues)
//   - RunEventDoneMsg — stream closed (run terminal or context cancelled)
//
// The channel is closed after RunEventDoneMsg is sent.
// SSE streaming has no SQLite or exec fallback — it requires a running server.
func (c *Client) StreamRunEvents(ctx context.Context, runID string) (<-chan interface{}, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}
	if c.apiURL == "" {
		return nil, ErrServerUnavailable
	}

	eventURL := c.apiURL + "/v1/runs/" + url.PathEscape(runID) + "/events?afterSeq=-1"

	req, err := http.NewRequestWithContext(ctx, "GET", eventURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	// Use a streaming-safe HTTP client with no timeout — the default
	// httpClient has a 10-second timeout which would kill long-running streams.
	streamClient := &http.Client{
		Transport: c.httpClient.Transport, // reuse transport (connection pooling, TLS)
		Timeout:   0,                      // no timeout on streaming body
	}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, ErrServerUnavailable
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		resp.Body.Close()
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrRunNotFound
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream events: unexpected status %d", resp.StatusCode)
	}

	ch := make(chan interface{}, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer to 1 MB to handle large NodeOutput/AgentEvent payloads.
		scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

		var (
			eventName string
			currentID string
			dataBuf   strings.Builder
		)

		send := func(msg interface{}) {
			select {
			case ch <- msg:
			case <-ctx.Done():
			}
		}

		for scanner.Scan() {
			line := scanner.Text()

			switch {
			case strings.HasPrefix(line, "event:"):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))

			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line, "data:")
				// SSE spec: strip exactly one leading space after the colon.
				if len(data) > 0 && data[0] == ' ' {
					data = data[1:]
				}
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(data)

			case strings.HasPrefix(line, "id:"):
				currentID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))

			case strings.HasPrefix(line, ":"):
				// Comment / heartbeat — silently ignored.

			case strings.HasPrefix(line, "retry:"):
				// Reconnect interval hint — noted but not acted on.

			case line == "":
				// Blank line — dispatch accumulated event.
				if dataBuf.Len() == 0 {
					eventName = ""
					currentID = ""
					continue
				}
				raw := []byte(dataBuf.String())
				dataBuf.Reset()

				if eventName == "smithers" || eventName == "" {
					var ev RunEvent
					if jsonErr := json.Unmarshal(raw, &ev); jsonErr != nil {
						send(RunEventErrorMsg{
							RunID: runID,
							Err:   fmt.Errorf("parse SSE data: %w", jsonErr),
						})
					} else {
						ev.Raw = raw
						// Populate Seq from SSE id: field if present.
						if currentID != "" {
							if n, parseErr := strconv.ParseInt(currentID, 10, 64); parseErr == nil {
								ev.Seq = int(n)
							}
						}
						send(RunEventMsg{RunID: runID, Event: ev})
					}
				}
				eventName = ""
				currentID = ""
			}

			if ctx.Err() != nil {
				return
			}
		}

		if scanErr := scanner.Err(); scanErr != nil && ctx.Err() == nil {
			send(RunEventErrorMsg{RunID: runID, Err: scanErr})
		}
		send(RunEventDoneMsg{RunID: runID})
	}()

	return ch, nil
}

// --- StreamAllEvents ---

// StreamAllEvents opens the global SSE stream at GET /v1/events and returns a
// channel that receives values of three types:
//   - RunEventMsg — a decoded event from the stream
//   - RunEventErrorMsg — a non-fatal parse error (stream continues)
//   - RunEventDoneMsg — stream closed (context cancelled or connection dropped)
//
// This is the primary feed for the notification overlay. Unlike StreamRunEvents,
// it receives events for all runs rather than a single run. The channel is
// closed after RunEventDoneMsg is sent.
//
// SSE streaming has no SQLite or exec fallback — it requires a running server.
// Returns ErrServerUnavailable when no API URL is configured.
func (c *Client) StreamAllEvents(ctx context.Context) (<-chan interface{}, error) {
	if c.apiURL == "" {
		return nil, ErrServerUnavailable
	}

	eventURL := c.apiURL + "/v1/events"

	req, err := http.NewRequestWithContext(ctx, "GET", eventURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	// Use a streaming-safe HTTP client with no timeout.
	streamClient := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   0,
	}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, ErrServerUnavailable
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		resp.Body.Close()
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		// Server running but no global event endpoint yet — treat as unavailable.
		resp.Body.Close()
		return nil, ErrServerUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream all events: unexpected status %d", resp.StatusCode)
	}

	ch := make(chan interface{}, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

		var (
			eventName string
			currentID string
			dataBuf   strings.Builder
		)

		send := func(msg interface{}) {
			select {
			case ch <- msg:
			case <-ctx.Done():
			}
		}

		for scanner.Scan() {
			line := scanner.Text()

			switch {
			case strings.HasPrefix(line, "event:"):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))

			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line, "data:")
				if len(data) > 0 && data[0] == ' ' {
					data = data[1:]
				}
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(data)

			case strings.HasPrefix(line, "id:"):
				currentID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))

			case strings.HasPrefix(line, ":"):
				// Comment / heartbeat — silently ignored.

			case strings.HasPrefix(line, "retry:"):
				// Reconnect interval hint — noted but not acted on.

			case line == "":
				if dataBuf.Len() == 0 {
					eventName = ""
					currentID = ""
					continue
				}
				raw := []byte(dataBuf.String())
				dataBuf.Reset()

				if eventName == "smithers" || eventName == "" {
					var ev RunEvent
					if jsonErr := json.Unmarshal(raw, &ev); jsonErr != nil {
						send(RunEventErrorMsg{
							RunID: ev.RunID,
							Err:   fmt.Errorf("parse SSE data: %w", jsonErr),
						})
					} else {
						ev.Raw = raw
						if currentID != "" {
							if n, parseErr := strconv.ParseInt(currentID, 10, 64); parseErr == nil {
								ev.Seq = int(n)
							}
						}
						send(RunEventMsg{RunID: ev.RunID, Event: ev})
					}
				}
				eventName = ""
				currentID = ""
			}

			if ctx.Err() != nil {
				return
			}
		}

		if scanErr := scanner.Err(); scanErr != nil && ctx.Err() == nil {
			send(RunEventErrorMsg{Err: scanErr})
		}
		send(RunEventDoneMsg{})
	}()

	return ch, nil
}

// --- v1 transport helpers ---

// v1GetJSON performs a GET against a /v1/* path that returns direct JSON
// (not the {ok,data,error} envelope used by legacy paths).
func (c *Client) v1GetJSON(ctx context.Context, path string, out any) error {
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

	return decodeV1Response(resp, out)
}

// v1PostJSON performs a POST against a /v1/* path with a direct JSON body and
// decodes the direct JSON response.
func (c *Client) v1PostJSON(ctx context.Context, path string, body any, out any) error {
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

	return decodeV1Response(resp, out)
}

// decodeV1Response maps HTTP status codes to typed errors and decodes the
// response body into out (when non-nil).
func decodeV1Response(resp *http.Response, out any) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrRunNotFound
	}

	// For non-2xx responses, try to decode the error envelope.
	if resp.StatusCode >= 300 {
		var errEnv v1ErrorEnvelope
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errEnv); jsonErr == nil && errEnv.Error != nil {
			switch errEnv.Error.Code {
			case "RUN_NOT_FOUND":
				return ErrRunNotFound
			case "RUN_NOT_ACTIVE":
				return ErrRunNotActive
			case "DB_NOT_CONFIGURED":
				return ErrDBNotConfigured
			case "UNAUTHORIZED":
				return ErrUnauthorized
			}
			return fmt.Errorf("smithers v1 API error %s: %s", errEnv.Error.Code, errEnv.Error.Message)
		}
		return fmt.Errorf("smithers v1 API: unexpected status %d", resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode v1 response: %w", err)
		}
	}
	return nil
}

// --- Scan / parse helpers for runs ---

// scanRunSummaries converts sql.Rows into a RunSummary slice.
func scanRunSummaries(rows *sql.Rows) ([]RunSummary, error) {
	defer rows.Close()
	var result []RunSummary
	for rows.Next() {
		var r RunSummary
		var workflowPath sql.NullString
		var startedAtMs, finishedAtMs sql.NullInt64
		var errorJSON sql.NullString
		if err := rows.Scan(
			&r.RunID, &r.WorkflowName, &workflowPath, &r.Status,
			&startedAtMs, &finishedAtMs, &errorJSON,
		); err != nil {
			return nil, err
		}
		if workflowPath.Valid {
			r.WorkflowPath = workflowPath.String
		}
		if startedAtMs.Valid {
			v := startedAtMs.Int64
			r.StartedAtMs = &v
		}
		if finishedAtMs.Valid {
			v := finishedAtMs.Int64
			r.FinishedAtMs = &v
		}
		if errorJSON.Valid {
			v := errorJSON.String
			r.ErrorJSON = &v
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// scanRunTasks converts sql.Rows into a RunTask slice.
func scanRunTasks(rows *sql.Rows) ([]RunTask, error) {
	defer rows.Close()
	var result []RunTask
	for rows.Next() {
		var t RunTask
		var label sql.NullString
		var lastAttempt sql.NullInt32
		var updatedAtMs sql.NullInt64
		if err := rows.Scan(
			&t.NodeID, &label, &t.Iteration, &t.State,
			&lastAttempt, &updatedAtMs,
		); err != nil {
			return nil, err
		}
		if label.Valid {
			v := label.String
			t.Label = &v
		}
		if lastAttempt.Valid {
			v := int(lastAttempt.Int32)
			t.LastAttempt = &v
		}
		if updatedAtMs.Valid {
			v := updatedAtMs.Int64
			t.UpdatedAtMs = &v
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// psRunEntry is the shape returned by `smithers ps --format json`.
// Fields differ from the v1 API RunSummary.
type psRunEntry struct {
	ID       string `json:"id"`
	Workflow string `json:"workflow"`
	Status   string `json:"status"`
	Step     string `json:"step"`
	Started  string `json:"started"`
}

// parseRunSummariesJSON parses exec JSON output into a RunSummary slice.
// Handles both a bare array and the `{"runs": [...]}` wrapper from `smithers ps`.
func parseRunSummariesJSON(data []byte) ([]RunSummary, error) {
	// Try bare array of RunSummary (v1 API shape).
	var runs []RunSummary
	if err := json.Unmarshal(data, &runs); err == nil && (len(runs) == 0 || runs[0].RunID != "") {
		return runs, nil
	}

	// Try {"runs": [...]} wrapper with ps-format entries.
	var wrapper struct {
		Runs []psRunEntry `json:"runs"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil {
		result := make([]RunSummary, len(wrapper.Runs))
		for i, r := range wrapper.Runs {
			result[i] = RunSummary{
				RunID:        r.ID,
				WorkflowName: r.Workflow,
				Status:       RunStatus(r.Status),
			}
		}
		return result, nil
	}

	// Try bare array of ps-format entries.
	var psRuns []psRunEntry
	if err := json.Unmarshal(data, &psRuns); err == nil {
		result := make([]RunSummary, len(psRuns))
		for i, r := range psRuns {
			result[i] = RunSummary{
				RunID:        r.ID,
				WorkflowName: r.Workflow,
				Status:       RunStatus(r.Status),
			}
		}
		return result, nil
	}

	return nil, fmt.Errorf("parse runs: unrecognized JSON format")
}

// parseRunSummaryJSON parses exec JSON output into a single RunSummary.
// Handles both a bare RunSummary object and a { run: {...} } wrapper.
func parseRunSummaryJSON(data []byte) (*RunSummary, error) {
	var run RunSummary
	if err := json.Unmarshal(data, &run); err == nil && run.RunID != "" {
		return &run, nil
	}
	var wrapper struct {
		Run RunSummary `json:"run"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse run summary: %w", err)
	}
	if wrapper.Run.RunID == "" {
		return nil, ErrRunNotFound
	}
	return &wrapper.Run, nil
}
