## Goal

Enrich the `ApprovalsView` detail pane so it shows the full task context — wait time with SLA color, run status, step progress, node/workflow metadata, pretty-printed payload with height truncation, and resolution info — updating live as the operator moves the cursor through the queue.

The `ApprovalsView` already exists at `internal/ui/views/approvals.go` with a split-pane layout and reads `Gate`, `Status`, `WorkflowPath`, `RunID`, `NodeID`, `Payload` from the `Approval` struct. This plan adds:

1. A `RunSummary` type + `GetRunSummary` client method with a 30-second TTL cache.
2. Cursor-driven async fetch: moving the cursor fires a `tea.Cmd` that fetches `RunSummary` for the new selection's `RunID`.
3. Enriched `renderDetail()`: gate header, SLA-colored wait time, run context block (workflow name, step progress, run status), static node/workflow metadata, height-aware payload, resolution info.
4. Enriched `renderListItem()`: wait time right-aligned with SLA colors.
5. Unit tests (client + view), a terminal E2E test, and a VHS recording.

This directly satisfies PRD §6.5 ("Show the task that needs approval, its inputs, and the workflow context") and the wireframe in `02-DESIGN.md §3.5`.

---

## Steps

### Step 1: Add `RunSummary` type

**File**: `internal/smithers/types.go`

Add after the `Approval` type (around line 96):

```go
// RunSummary holds run-level metadata used by the approval detail pane
// and other views that need run context without full node trees.
// Maps to the shape returned by GET /v1/runs/{id} in the Smithers server
// and _smithers_runs + _smithers_nodes in the SQLite database.
type RunSummary struct {
    ID           string `json:"id"`
    WorkflowPath string `json:"workflowPath"`
    WorkflowName string `json:"workflowName"` // Derived from path.Base(WorkflowPath) without extension
    Status       string `json:"status"`       // "running" | "paused" | "completed" | "failed"
    NodeTotal    int    `json:"nodeTotal"`    // Total nodes in the DAG
    NodesDone    int    `json:"nodesDone"`    // Nodes with status "completed" or "failed"
    StartedAtMs  int64  `json:"startedAtMs"`  // Unix ms
    ElapsedMs    int64  `json:"elapsedMs"`    // Computed: time.Now().UnixMilli() - StartedAtMs
}
```

**Verification**: `go build ./internal/smithers/...` passes.

Note on naming: the `runs-dashboard` plan also defines a `RunSummary` type (for run progress counters). If that ticket has landed, check for a naming conflict and resolve by either unifying the types or renaming this one to `RunContext`. The engineering spec intends this type to carry per-run metadata for the approvals detail pane; it is intentionally lightweight.

---

### Step 2: Add cache fields to `Client`

**File**: `internal/smithers/client.go`

Add fields to the `Client` struct (after the `serverMu`/`serverUp`/`serverChecked` block, around line 72):

```go
// runSummaryCache stores fetched RunSummary values keyed by runID.
// Each entry expires after 30 seconds.
runSummaryCache sync.Map // map[string]runSummaryCacheEntry
```

Add a private type below the struct definition (before `NewClient`):

```go
type runSummaryCacheEntry struct {
    summary   *RunSummary
    fetchedAt time.Time
}
```

Add the cache accessor methods immediately before `GetRunSummary`:

```go
// getRunSummaryCache returns a cached RunSummary if one exists and has not expired.
func (c *Client) getRunSummaryCache(runID string) (*RunSummary, bool) {
    val, ok := c.runSummaryCache.Load(runID)
    if !ok {
        return nil, false
    }
    entry := val.(runSummaryCacheEntry)
    if time.Since(entry.fetchedAt) > 30*time.Second {
        c.runSummaryCache.Delete(runID)
        return nil, false
    }
    return entry.summary, true
}

// setRunSummaryCache stores a RunSummary in the cache.
func (c *Client) setRunSummaryCache(runID string, summary *RunSummary) {
    c.runSummaryCache.Store(runID, runSummaryCacheEntry{
        summary:   summary,
        fetchedAt: time.Now(),
    })
}

// ClearRunSummaryCache evicts the cached RunSummary for a run.
// Called by approve/deny mutations (future tickets) to force a fresh fetch.
func (c *Client) ClearRunSummaryCache(runID string) {
    c.runSummaryCache.Delete(runID)
}
```

**Why `sync.Map`**: The existing `serverUp` cache uses `sync.RWMutex` with a single value. `sync.Map` is cleaner for a per-key TTL cache where keys are dynamic run IDs. It avoids a mutex protecting a full `map[string]...` and matches the pattern from the engineering spec.

**Verification**: `go build ./internal/smithers/...` passes.

---

### Step 3: Add `GetRunSummary` to client

**File**: `internal/smithers/client.go`

Add after the `--- Approvals ---` section (around line 296), as its own `--- Run Summary ---` section:

```go
// --- Run Summary ---

// GetRunSummary returns lightweight metadata for a single run.
// Routes: cache → HTTP GET /v1/runs/{runID} → SQLite → exec smithers inspect.
func (c *Client) GetRunSummary(ctx context.Context, runID string) (*RunSummary, error) {
    // Check cache first.
    if cached, ok := c.getRunSummaryCache(runID); ok {
        return cached, nil
    }

    var summary *RunSummary

    // 1. Try HTTP.
    if c.isServerAvailable() {
        var s RunSummary
        err := c.httpGetJSON(ctx, "/v1/runs/"+runID, &s)
        if err == nil {
            s.WorkflowName = workflowNameFromPath(s.WorkflowPath)
            s.ElapsedMs = time.Now().UnixMilli() - s.StartedAtMs
            summary = &s
        }
    }

    // 2. Try direct SQLite.
    if summary == nil && c.db != nil {
        s, err := c.getRunSummaryDB(ctx, runID)
        if err == nil {
            summary = s
        }
    }

    // 3. Fall back to exec.
    if summary == nil {
        s, err := c.getRunSummaryExec(ctx, runID)
        if err != nil {
            return nil, err
        }
        summary = s
    }

    c.setRunSummaryCache(runID, summary)
    return summary, nil
}

// getRunSummaryDB fetches a RunSummary from the direct SQLite connection.
func (c *Client) getRunSummaryDB(ctx context.Context, runID string) (*RunSummary, error) {
    rows, err := c.queryDB(ctx, `
        SELECT r.id, r.workflow_path, r.status, r.started_at,
               (SELECT COUNT(*) FROM _smithers_nodes n WHERE n.run_id = r.id) AS node_total,
               (SELECT COUNT(*) FROM _smithers_nodes n WHERE n.run_id = r.id
                AND n.status IN ('completed', 'failed')) AS nodes_done
        FROM _smithers_runs r WHERE r.id = ?`, runID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    if !rows.Next() {
        return nil, fmt.Errorf("run %s not found", runID)
    }
    var s RunSummary
    var startedAt int64
    if err := rows.Scan(&s.ID, &s.WorkflowPath, &s.Status, &startedAt,
        &s.NodeTotal, &s.NodesDone); err != nil {
        return nil, err
    }
    s.StartedAtMs = startedAt
    s.WorkflowName = workflowNameFromPath(s.WorkflowPath)
    s.ElapsedMs = time.Now().UnixMilli() - s.StartedAtMs
    return &s, rows.Err()
}

// getRunSummaryExec fetches a RunSummary via exec smithers inspect.
func (c *Client) getRunSummaryExec(ctx context.Context, runID string) (*RunSummary, error) {
    out, err := c.execSmithers(ctx, "inspect", runID, "--format", "json")
    if err != nil {
        return nil, err
    }
    return parseRunSummaryJSON(out)
}

// parseRunSummaryJSON parses exec output into a RunSummary.
// The smithers inspect command may return a nested object; we extract the top-level fields.
func parseRunSummaryJSON(data []byte) (*RunSummary, error) {
    var s RunSummary
    if err := json.Unmarshal(data, &s); err != nil {
        return nil, fmt.Errorf("parse run summary: %w", err)
    }
    s.WorkflowName = workflowNameFromPath(s.WorkflowPath)
    s.ElapsedMs = time.Now().UnixMilli() - s.StartedAtMs
    return &s, nil
}

// workflowNameFromPath derives a human-readable workflow name from its file path.
// ".smithers/workflows/deploy-staging.ts" → "deploy-staging"
func workflowNameFromPath(p string) string {
    base := path.Base(p)
    // Strip extension.
    if idx := strings.LastIndex(base, "."); idx > 0 {
        base = base[:idx]
    }
    return base
}
```

Add `"path"` to the import block if not already present.

**Verification**: `go build ./internal/smithers/...` passes.

---

### Step 4: Add `GetRunSummary` client unit tests

**File**: `internal/smithers/client_test.go`

Add after the existing approvals tests. Follow the `newTestServer` / `writeEnvelope` / `newExecClient` helpers already in the file:

```go
// --- GetRunSummary ---

func TestGetRunSummary_HTTP(t *testing.T) {
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/v1/runs/run-abc", r.URL.Path)
        assert.Equal(t, "GET", r.Method)
        writeEnvelope(t, w, RunSummary{
            ID:           "run-abc",
            WorkflowPath: ".smithers/workflows/deploy-staging.ts",
            Status:       "running",
            NodeTotal:    6,
            NodesDone:    4,
            StartedAtMs:  time.Now().Add(-10 * time.Minute).UnixMilli(),
        })
    })

    s, err := c.GetRunSummary(context.Background(), "run-abc")
    require.NoError(t, err)
    require.NotNil(t, s)
    assert.Equal(t, "deploy-staging", s.WorkflowName)
    assert.Equal(t, 6, s.NodeTotal)
    assert.Equal(t, 4, s.NodesDone)
    assert.Greater(t, s.ElapsedMs, int64(0))
}

func TestGetRunSummary_Exec(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        assert.Equal(t, []string{"inspect", "run-abc", "--format", "json"}, args)
        data, _ := json.Marshal(RunSummary{
            ID:           "run-abc",
            WorkflowPath: ".smithers/workflows/gdpr-cleanup.ts",
            Status:       "running",
            NodeTotal:    4,
            NodesDone:    3,
            StartedAtMs:  time.Now().Add(-5 * time.Minute).UnixMilli(),
        })
        return data, nil
    })

    s, err := c.GetRunSummary(context.Background(), "run-abc")
    require.NoError(t, err)
    assert.Equal(t, "gdpr-cleanup", s.WorkflowName)
    assert.Equal(t, 4, s.NodeTotal)
}

func TestGetRunSummary_NotFound(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        return nil, fmt.Errorf("smithers inspect run-missing: exit status 1")
    })
    _, err := c.GetRunSummary(context.Background(), "run-missing")
    require.Error(t, err)
}

func TestGetRunSummary_CacheHit(t *testing.T) {
    requestCount := 0
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        if strings.Contains(r.URL.Path, "run-cached") {
            requestCount++
            writeEnvelope(t, w, RunSummary{ID: "run-cached", WorkflowPath: "wf.ts", Status: "running"})
        }
    })

    _, err := c.GetRunSummary(context.Background(), "run-cached")
    require.NoError(t, err)
    _, err = c.GetRunSummary(context.Background(), "run-cached")
    require.NoError(t, err)

    assert.Equal(t, 1, requestCount, "HTTP should be called only once; second call should hit cache")
}

func TestGetRunSummary_CacheExpiry(t *testing.T) {
    requestCount := 0
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        if strings.Contains(r.URL.Path, "run-expire") {
            requestCount++
            writeEnvelope(t, w, RunSummary{ID: "run-expire", WorkflowPath: "wf.ts", Status: "running"})
        }
    })

    _, err := c.GetRunSummary(context.Background(), "run-expire")
    require.NoError(t, err)

    // Manually expire the cache entry.
    c.runSummaryCache.Store("run-expire", runSummaryCacheEntry{
        summary:   &RunSummary{ID: "run-expire"},
        fetchedAt: time.Now().Add(-31 * time.Second),
    })

    _, err = c.GetRunSummary(context.Background(), "run-expire")
    require.NoError(t, err)

    assert.Equal(t, 2, requestCount, "HTTP should be called twice after cache expiry")
}

func TestGetRunSummary_WorkflowNameFromPath(t *testing.T) {
    cases := []struct{ path, want string }{
        {".smithers/workflows/deploy-staging.ts", "deploy-staging"},
        {"./workflows/gdpr-cleanup.tsx", "gdpr-cleanup"},
        {"simple", "simple"},
        {"no/ext", "ext"},
    }
    for _, tc := range cases {
        assert.Equal(t, tc.want, workflowNameFromPath(tc.path))
    }
}
```

**Verification**: `go test ./internal/smithers/ -run TestGetRunSummary -v` passes.

---

### Step 5: Add message types and view state fields

**File**: `internal/ui/views/approvals.go`

Add two new message types after `approvalsErrorMsg` (around line 24):

```go
type runSummaryLoadedMsg struct {
    runID   string
    summary *smithers.RunSummary
}

type runSummaryErrorMsg struct {
    runID string
    err   error
}
```

Extend `ApprovalsView` struct (add after `err error` field, around line 35):

```go
// Enriched context for the selected approval (fetched async).
selectedRun    *smithers.RunSummary // nil until fetched
contextLoading bool                 // true while RunSummary is being fetched
contextErr     error                // non-nil if most recent fetch failed
lastFetchRun   string               // RunID of the last-triggered fetch (dedup)
```

**Verification**: `go build ./...` passes (fields alone don't break anything).

---

### Step 6: Wire async context fetch

**File**: `internal/ui/views/approvals.go`

Add the `fetchRunContext` helper method after `Init()`:

```go
// fetchRunContext fires an async command to fetch RunSummary for the currently
// selected approval. Returns nil if the same RunID was already fetched.
func (v *ApprovalsView) fetchRunContext() tea.Cmd {
    if v.cursor < 0 || v.cursor >= len(v.approvals) {
        return nil
    }
    a := v.approvals[v.cursor]
    if a.RunID == v.lastFetchRun && v.selectedRun != nil {
        return nil // Already have fresh data for this RunID.
    }
    v.contextLoading = true
    v.contextErr = nil
    v.lastFetchRun = a.RunID
    runID := a.RunID
    return func() tea.Msg {
        summary, err := v.client.GetRunSummary(context.Background(), runID)
        if err != nil {
            return runSummaryErrorMsg{runID: runID, err: err}
        }
        return runSummaryLoadedMsg{runID: runID, summary: summary}
    }
}
```

Update the `Update` handler:

**In `approvalsLoadedMsg` case** — trigger initial context fetch:
```go
case approvalsLoadedMsg:
    v.approvals = msg.approvals
    v.loading = false
    if len(v.approvals) > 0 {
        return v, v.fetchRunContext()
    }
    return v, nil
```

**In `tea.KeyPressMsg` case** — trigger fetch on cursor movement:
```go
case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
    if v.cursor > 0 {
        v.cursor--
        return v, v.fetchRunContext()
    }

case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
    if v.cursor < len(v.approvals)-1 {
        v.cursor++
        return v, v.fetchRunContext()
    }
```

**Add new message handlers** in `Update` (before the final `return v, nil`):
```go
case runSummaryLoadedMsg:
    // Only apply if this result is still for the current selection.
    if msg.runID == v.lastFetchRun {
        v.selectedRun = msg.summary
        v.contextLoading = false
        v.contextErr = nil
    }
    return v, nil

case runSummaryErrorMsg:
    if msg.runID == v.lastFetchRun {
        v.contextErr = msg.err
        v.contextLoading = false
    }
    return v, nil
```

**In the `r` (refresh) key handler** — clear stale context so it re-fetches after list reloads:
```go
case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
    v.loading = true
    v.selectedRun = nil
    v.lastFetchRun = ""
    v.contextErr = nil
    return v, v.Init()
```

**Verification**: `go build ./...` passes.

---

### Step 7: Add SLA helper functions

**File**: `internal/ui/views/approvals.go`

Add to the `--- Helpers ---` section at the bottom of the file:

```go
// formatWait formats a duration as a human-readable wait time.
// < 1m → "<1m"; < 1h → "Xm"; ≥ 1h → "Xh Ym".
func formatWait(d time.Duration) string {
    if d < time.Minute {
        return "<1m"
    }
    if d < time.Hour {
        return fmt.Sprintf("%dm", int(d.Minutes()))
    }
    return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// slaStyle returns a lipgloss.Style colored by SLA urgency.
// Green: < 10m, Yellow: 10–30m, Red: ≥ 30m.
// Matches approval-ui.ts thresholds from the upstream GUI.
func slaStyle(d time.Duration) lipgloss.Style {
    switch {
    case d < 10*time.Minute:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
    case d < 30*time.Minute:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
    default:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
    }
}

// relativeTime formats a Unix-millisecond timestamp as a relative string.
// e.g., "2m ago", "1h 15m ago", "just now".
func relativeTime(ms int64) string {
    d := time.Since(time.UnixMilli(ms))
    if d < time.Minute {
        return "just now"
    }
    if d < time.Hour {
        return fmt.Sprintf("%dm ago", int(d.Minutes()))
    }
    return fmt.Sprintf("%dh %dm ago", int(d.Hours()), int(d.Minutes())%60)
}
```

Add `"time"` to the import block if not already present.

**Verification**: `go build ./...` passes.

---

### Step 8: Rewrite `renderListItem` with wait time

**File**: `internal/ui/views/approvals.go`

Replace the current `renderListItem` implementation (lines 215–241):

```go
// renderListItem renders a single approval in the list with wait-time SLA indicator.
func (v *ApprovalsView) renderListItem(idx, width int) string {
    a := v.approvals[idx]
    cursor := "  "
    nameStyle := lipgloss.NewStyle()
    if idx == v.cursor {
        cursor = "▸ "
        nameStyle = nameStyle.Bold(true)
    }

    label := a.Gate
    if label == "" {
        label = a.NodeID
    }

    statusIcon := "○"
    switch a.Status {
    case "approved":
        statusIcon = "✓"
    case "denied":
        statusIcon = "✗"
    }

    // Build wait/age string for pending items.
    waitStr := ""
    if a.Status == "pending" && a.RequestedAt > 0 {
        d := time.Since(time.UnixMilli(a.RequestedAt))
        waitStr = slaStyle(d).Render(formatWait(d))
    }

    // Lay out: [cursor 2][icon 1][ 1][label][right-pad][waitStr]
    // Total prefix = 4 chars; leave room for waitStr on right.
    waitWidth := lipgloss.Width(waitStr)
    labelMax := width - 4 - waitWidth - 1
    if labelMax < 4 {
        labelMax = 4
    }
    if len(label) > labelMax {
        label = label[:labelMax-3] + "..."
    }

    line := cursor + statusIcon + " " + nameStyle.Render(label)
    if waitStr != "" {
        pad := width - lipgloss.Width(line) - waitWidth
        if pad > 0 {
            line += strings.Repeat(" ", pad)
        }
        line += waitStr
    }

    return line + "\n"
}
```

**Verification**: `go build ./...` passes. Visually: pending items show wait time right-aligned in the list pane.

---

### Step 9: Rewrite `renderDetail` with enriched context

**File**: `internal/ui/views/approvals.go`

Replace the current `renderDetail` implementation (lines 289–320) with:

```go
// renderDetail renders the enriched context detail pane for the selected approval.
// Layout (top to bottom):
//   1. Gate header + status badge + wait time
//   2. Run context block (from RunSummary, or loading/error indicator)
//   3. Static metadata: workflow path, node ID
//   4. Payload: height-aware pretty-printed JSON or wrapped text
//   5. Resolution info (for decided approvals only)
func (v *ApprovalsView) renderDetail(width int) string {
    if v.cursor < 0 || v.cursor >= len(v.approvals) {
        return lipgloss.NewStyle().Faint(true).Render("Select an approval to view details.")
    }

    a := v.approvals[v.cursor]
    var b strings.Builder

    titleStyle  := lipgloss.NewStyle().Bold(true)
    labelStyle  := lipgloss.NewStyle().Faint(true)
    faintStyle  := lipgloss.NewStyle().Faint(true)
    errorStyle  := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("1"))

    linesUsed := 0

    // 1. Gate title.
    gate := a.Gate
    if gate == "" {
        gate = a.NodeID
    }
    b.WriteString(titleStyle.Render(gate) + "\n")
    linesUsed++

    // Status + wait time on same line.
    statusStr := formatStatus(a.Status)
    if a.Status == "pending" && a.RequestedAt > 0 {
        d := time.Since(time.UnixMilli(a.RequestedAt))
        statusStr += "  " + slaStyle(d).Render("⏱ "+formatWait(d))
    }
    b.WriteString(statusStr + "\n\n")
    linesUsed += 2

    // 2. Run context block.
    if v.contextLoading {
        b.WriteString(faintStyle.Render("Loading run details...") + "\n\n")
        linesUsed += 2
    } else if v.contextErr != nil {
        b.WriteString(errorStyle.Render("Could not load run details") + "\n\n")
        linesUsed += 2
    } else if v.selectedRun != nil && v.selectedRun.ID == a.RunID {
        rs := v.selectedRun
        // Run ID + workflow name.
        runLine := labelStyle.Render("Run: ") + rs.ID
        if rs.WorkflowName != "" {
            runLine += " (" + rs.WorkflowName + ")"
        }
        b.WriteString(runLine + "\n")
        linesUsed++

        // Step progress + run status + elapsed.
        var progressParts []string
        if rs.NodeTotal > 0 {
            progressParts = append(progressParts,
                fmt.Sprintf("Step %d of %d", rs.NodesDone, rs.NodeTotal))
        }
        if rs.Status != "" {
            progressParts = append(progressParts, rs.Status)
        }
        if rs.StartedAtMs > 0 {
            progressParts = append(progressParts,
                "started "+relativeTime(rs.StartedAtMs))
        }
        if len(progressParts) > 0 {
            b.WriteString(faintStyle.Render(strings.Join(progressParts, " · ")) + "\n")
            linesUsed++
        }
        b.WriteString("\n")
        linesUsed++
    }

    // 3. Static metadata.
    b.WriteString(labelStyle.Render("Workflow: ") + a.WorkflowPath + "\n")
    b.WriteString(labelStyle.Render("Node:     ") + a.NodeID + "\n")
    linesUsed += 2

    // 4. Payload.
    if a.Payload != "" {
        b.WriteString("\n" + labelStyle.Render("Payload:") + "\n")
        linesUsed += 2

        // Reserve lines for header + resolution section below (estimate 3 lines).
        availLines := v.height - linesUsed - 3
        if availLines < 4 {
            availLines = 4
        }
        payloadStr := formatPayload(a.Payload, width)
        payloadLines := strings.Split(payloadStr, "\n")
        if len(payloadLines) > availLines {
            truncated := payloadLines[:availLines]
            remaining := len(payloadLines) - availLines
            truncated = append(truncated,
                faintStyle.Render(fmt.Sprintf("  ... (%d more lines)", remaining)))
            payloadStr = strings.Join(truncated, "\n")
        }
        b.WriteString(payloadStr + "\n")
    }

    // 5. Resolution info (decided approvals only).
    if a.Status != "pending" && a.ResolvedAt != nil {
        b.WriteString("\n")
        if a.ResolvedBy != nil && *a.ResolvedBy != "" {
            b.WriteString(labelStyle.Render("Resolved by: ") + *a.ResolvedBy + "\n")
        }
        b.WriteString(labelStyle.Render("Resolved:    ") + relativeTime(*a.ResolvedAt) + "\n")
    }

    return b.String()
}
```

**Verification**: `go build ./...` passes.

---

### Step 10: Update `renderListCompact` for enriched inline context

**File**: `internal/ui/views/approvals.go`

Replace the inline context block inside `renderListCompact` (the `if i == v.cursor` block, lines 271–279) with:

```go
if i == v.cursor {
    faint := lipgloss.NewStyle().Faint(true)
    b.WriteString(faint.Render("    Workflow: "+a.WorkflowPath) + "\n")
    b.WriteString(faint.Render("    Run: "+a.RunID) + "\n")

    // Step progress from RunSummary if available and still matching.
    if v.selectedRun != nil && v.selectedRun.ID == a.RunID && v.selectedRun.NodeTotal > 0 {
        b.WriteString(faint.Render(fmt.Sprintf("    Step %d of %d · %s",
            v.selectedRun.NodesDone, v.selectedRun.NodeTotal,
            v.selectedRun.Status)) + "\n")
    } else if v.contextLoading {
        b.WriteString(faint.Render("    Loading...") + "\n")
    }

    // Payload preview: parse JSON first, then truncate.
    if a.Payload != "" {
        var parsed interface{}
        preview := a.Payload
        if err := json.Unmarshal([]byte(a.Payload), &parsed); err == nil {
            if compact, err := json.Marshal(parsed); err == nil {
                preview = string(compact)
            }
        }
        b.WriteString(faint.Render("    "+truncate(preview, 60)) + "\n")
    }

    // Wait time for pending items.
    if a.Status == "pending" && a.RequestedAt > 0 {
        d := time.Since(time.UnixMilli(a.RequestedAt))
        b.WriteString(faint.Render("    Waiting: ") + slaStyle(d).Render(formatWait(d)) + "\n")
    }
}
```

**Verification**: `go build ./...` passes.

---

### Step 11: Write view unit tests

**File**: `internal/ui/views/approvals_test.go` (new file)

Create with these 12 test cases. Use `tea.Batch` + `Update` to exercise message handling, `View()` output for content assertions:

```go
package views

import (
    "testing"
    "time"

    tea "charm.land/bubbletea/v2"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func makeApproval(id, runID, gate, status string, requestedMinsAgo int) smithers.Approval {
    return smithers.Approval{
        ID:           id,
        RunID:        runID,
        NodeID:       "node-" + id,
        WorkflowPath: ".smithers/workflows/test.ts",
        Gate:         gate,
        Status:       status,
        Payload:      `{"env":"staging"}`,
        RequestedAt:  time.Now().Add(-time.Duration(requestedMinsAgo) * time.Minute).UnixMilli(),
    }
}

func TestApprovalsView_DetailShowsRunContext(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{
        approvals: []smithers.Approval{makeApproval("a1", "run-abc", "Deploy?", "pending", 5)},
    })
    av := v2.(*ApprovalsView)
    rs := &smithers.RunSummary{ID: "run-abc", WorkflowName: "deploy-staging",
        NodeTotal: 6, NodesDone: 4, Status: "running"}
    v3, _ := av.Update(runSummaryLoadedMsg{runID: "run-abc", summary: rs})
    out := v3.(View).View()

    assert.Contains(t, out, "deploy-staging")
    assert.Contains(t, out, "Step 4 of 6")
}

func TestApprovalsView_DetailWaitTimeSLA(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    // Under 10m → should not render red.
    a5 := makeApproval("a1", "run-abc", "Gate", "pending", 5)
    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{a5}})
    out := v2.(View).View()
    assert.Contains(t, out, "5m")

    // Over 30m → should render in context.
    a45 := makeApproval("a2", "run-xyz", "Gate", "pending", 45)
    v3, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{a45}})
    out2 := v3.(View).View()
    assert.Contains(t, out2, "45m")
}

func TestApprovalsView_CursorChangeTriggersContextFetch(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate A", "pending", 3),
        makeApproval("a2", "run-xyz", "Gate B", "pending", 7),
    }})

    _, cmd := v2.(View).Update(tea.KeyPressMsg{Code: tea.KeyCodeDown})
    require.NotNil(t, cmd, "cursor move to item with different RunID should return a fetch cmd")
    av := v2.(*ApprovalsView)
    assert.Equal(t, "run-xyz", av.lastFetchRun)
}

func TestApprovalsView_SameRunIDSkipsFetch(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    // Two approvals referencing the same run.
    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate A", "pending", 3),
        makeApproval("a2", "run-abc", "Gate B", "pending", 7),
    }})
    // Pre-populate selectedRun so dedup check triggers.
    av := v2.(*ApprovalsView)
    av.selectedRun = &smithers.RunSummary{ID: "run-abc"}
    av.lastFetchRun = "run-abc"

    _, cmd := av.Update(tea.KeyPressMsg{Code: tea.KeyCodeDown})
    assert.Nil(t, cmd, "same RunID should not trigger a second fetch")
}

func TestApprovalsView_DetailLoadingState(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate", "pending", 5),
    }})
    av := v2.(*ApprovalsView)
    av.contextLoading = true
    av.selectedRun = nil

    assert.Contains(t, av.View(), "Loading run details...")
}

func TestApprovalsView_DetailErrorState(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate", "pending", 5),
    }})
    av := v2.(*ApprovalsView)
    av.lastFetchRun = "run-abc"
    v3, _ := av.Update(runSummaryErrorMsg{runID: "run-abc", err: fmt.Errorf("timeout")})

    assert.Contains(t, v3.(View).View(), "Could not load run details")
}

func TestApprovalsView_DetailNoPayload(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    a := makeApproval("a1", "run-abc", "Gate", "pending", 5)
    a.Payload = ""
    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{a}})
    out := v2.(View).View()

    assert.NotContains(t, out, "Payload:", "empty payload should not render the Payload header")
}

func TestApprovalsView_ResolvedApprovalShowsDecision(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    resolvedAt := time.Now().Add(-30 * time.Minute).UnixMilli()
    resolvedBy := "alice"
    a := makeApproval("a1", "run-abc", "Gate", "approved", 35)
    a.ResolvedAt = &resolvedAt
    a.ResolvedBy = &resolvedBy

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{a}})
    out := v2.(View).View()

    assert.Contains(t, out, "Resolved by:")
    assert.Contains(t, out, "alice")
}

func TestApprovalsView_ListItemShowsWaitTime(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Deploy?", "pending", 12),
    }})
    out := v2.(View).View()

    assert.Contains(t, out, "12m")
}

func TestApprovalsView_CompactModeShowsRunContext(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 60 // triggers compact mode
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate", "pending", 5),
    }})
    av := v2.(*ApprovalsView)
    av.selectedRun = &smithers.RunSummary{
        ID: "run-abc", NodeTotal: 6, NodesDone: 4, Status: "running",
    }
    out := av.View()

    assert.Contains(t, out, "Step 4 of 6")
    assert.NotContains(t, out, " │ ", "compact mode must not show split-pane divider")
}

func TestApprovalsView_InitialLoadTriggersContextFetch(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    _, cmd := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate", "pending", 5),
    }})
    require.NotNil(t, cmd, "initial load with approvals should trigger a context fetch cmd")
}

func TestApprovalsView_SplitPaneDividerPresent(t *testing.T) {
    v := NewApprovalsView(smithers.NewClient())
    v.width = 120
    v.height = 40

    v2, _ := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
        makeApproval("a1", "run-abc", "Gate", "pending", 5),
    }})
    out := v2.(View).View()
    assert.Contains(t, out, "│", "wide terminal should render split-pane divider")
}
```

**Verification**: `go test ./internal/ui/views/ -run TestApprovalsView -v` passes.

---

### Step 12: Write terminal E2E test

**File**: `internal/e2e/approvals_context_display_test.go` (new)

Model on `chat_domain_system_prompt_test.go`. The mock server must serve:
- `GET /health` → 200 OK (required by `isServerAvailable`)
- `GET /approval/list` → two `Approval` records with distinct `RunID`s
- `GET /v1/runs/run-abc` → `RunSummary` for first approval
- `GET /v1/runs/run-xyz` → `RunSummary` for second approval

The response format must use the `{ok: true, data: ...}` envelope (as required by `httpGetJSON` which decodes `apiEnvelope`).

```go
func TestApprovalsContextDisplayE2E(t *testing.T) {
    if os.Getenv("SMITHERS_TUI_E2E") != "1" {
        t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
    }

    mockServer := startMockSmithersContextServer(t)

    configDir := t.TempDir()
    dataDir := t.TempDir()
    writeGlobalConfig(t, configDir, fmt.Sprintf(`{
        "smithers": { "apiUrl": %q }
    }`, mockServer.URL))
    t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
    t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

    tui := launchTUI(t)
    defer tui.Terminate()

    require.NoError(t, tui.WaitForText("CRUSH", 15*time.Second))

    tui.SendKeys("\x01") // Ctrl+A → approvals view

    require.NoError(t, tui.WaitForText("Pending", 5*time.Second),
        "approvals view not reached; buffer: %s", tui.Snapshot())

    // First approval context should appear after RunSummary fetch.
    require.NoError(t, tui.WaitForText("deploy-staging", 5*time.Second),
        "first approval workflow name missing; buffer: %s", tui.Snapshot())
    require.NoError(t, tui.WaitForText("Step 4 of 6", 5*time.Second),
        "first approval step progress missing; buffer: %s", tui.Snapshot())

    // Navigate to second approval.
    tui.SendKeys("j")
    require.NoError(t, tui.WaitForText("gdpr-cleanup", 5*time.Second),
        "second approval workflow name missing; buffer: %s", tui.Snapshot())
    require.NoError(t, tui.WaitForText("Step 3 of 4", 5*time.Second),
        "second approval step progress missing; buffer: %s", tui.Snapshot())

    // Navigate back to first.
    tui.SendKeys("k")
    require.NoError(t, tui.WaitForText("deploy-staging", 5*time.Second),
        "should revert to first approval context on k; buffer: %s", tui.Snapshot())

    // Esc returns to chat.
    tui.SendKeys("\x1b")
    require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second),
        "esc should return to chat; buffer: %s", tui.Snapshot())
}

func startMockSmithersContextServer(t *testing.T) *httptest.Server {
    t.Helper()
    requestedAt1 := time.Now().Add(-8 * time.Minute).UnixMilli()
    requestedAt2 := time.Now().Add(-2 * time.Minute).UnixMilli()
    approvals := []smithers.Approval{
        {ID: "appr-1", RunID: "run-abc", NodeID: "deploy", WorkflowPath: ".smithers/workflows/deploy-staging.ts",
         Gate: "Deploy to staging?", Status: "pending", Payload: `{"commit":"a1b2c3d","env":"staging"}`,
         RequestedAt: requestedAt1},
        {ID: "appr-2", RunID: "run-xyz", NodeID: "delete-records", WorkflowPath: ".smithers/workflows/gdpr-cleanup.ts",
         Gate: "Delete user data?", Status: "pending", Payload: `{"userId":98765,"records":142}`,
         RequestedAt: requestedAt2},
    }
    runs := map[string]smithers.RunSummary{
        "run-abc": {ID: "run-abc", WorkflowPath: ".smithers/workflows/deploy-staging.ts",
            WorkflowName: "deploy-staging", Status: "running", NodeTotal: 6, NodesDone: 4,
            StartedAtMs: time.Now().Add(-10 * time.Minute).UnixMilli()},
        "run-xyz": {ID: "run-xyz", WorkflowPath: ".smithers/workflows/gdpr-cleanup.ts",
            WorkflowName: "gdpr-cleanup", Status: "running", NodeTotal: 4, NodesDone: 3,
            StartedAtMs: time.Now().Add(-4 * time.Minute).UnixMilli()},
    }

    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        writeJSONEnvelope := func(data any) {
            dataBytes, _ := json.Marshal(data)
            env := map[string]interface{}{"ok": true, "data": json.RawMessage(dataBytes)}
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(env)
        }
        switch {
        case r.URL.Path == "/health":
            w.WriteHeader(http.StatusOK)
        case r.URL.Path == "/approval/list":
            writeJSONEnvelope(approvals)
        case strings.HasPrefix(r.URL.Path, "/v1/runs/"):
            runID := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
            if rs, ok := runs[runID]; ok {
                writeJSONEnvelope(rs)
            } else {
                w.WriteHeader(http.StatusNotFound)
            }
        default:
            w.WriteHeader(http.StatusNotFound)
        }
    }))
    t.Cleanup(srv.Close)
    return srv
}
```

**Verification**: `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsContextDisplayE2E -timeout 30s -v` passes.

---

### Step 13: Write VHS tape

**File**: `tests/vhs/approvals-context-display.tape` (new)

```tape
# Approvals context display — enriched detail pane with cursor-driven updates
Output tests/vhs/output/approvals-context-display.gif
Set FontSize 14
Set Width 120
Set Height 35
Set Shell zsh

# Start TUI with mock server fixture config
Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Navigate to approvals
Ctrl+a
Sleep 2s

# Capture split-pane with enriched context for first approval
Screenshot tests/vhs/output/approvals-context-first.png

# Navigate to second approval — context should update
Down
Sleep 1s
Screenshot tests/vhs/output/approvals-context-second.png

# Navigate back to first
Up
Sleep 500ms

# Refresh the list
Type "r"
Sleep 1s

# Return to chat
Escape
Sleep 1s
Screenshot tests/vhs/output/approvals-context-back.png
```

For the VHS tape to work, `tests/vhs/fixtures/` must contain a `smithers-tui.json` (or equivalent global config file name) pointing to a running mock server or stub. See `tests/vhs/fixtures/` for the existing fixture format from `smithers-domain-system-prompt.tape`.

**Verification**: `vhs tests/vhs/approvals-context-display.tape` exits 0 and produces `approvals-context-display.gif`.

---

## Validation Checklist

| Check | Command |
|---|---|
| New types compile | `go build ./internal/smithers/...` |
| Client tests pass | `go test ./internal/smithers/ -run TestGetRunSummary -v` |
| View tests pass | `go test ./internal/ui/views/ -run TestApprovalsView -v` |
| Full build | `go build ./...` |
| No regressions | `go test ./...` |
| E2E test (opt-in) | `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsContextDisplayE2E -timeout 30s -v` |
| VHS recording | `vhs tests/vhs/approvals-context-display.tape` |

---

## Risks and Mitigations

**Risk: `/v1/runs/{runID}` HTTP endpoint may not exist on the Smithers server.**
Mitigation: Three-tier transport automatically falls back to SQLite then exec. At implementation, probe `../smithers/src/server/index.ts` for the actual run route path. Adjust the HTTP path in `GetRunSummary` accordingly (e.g., `/runs/{id}`, `/ps/{id}`, or `/inspect/{id}`).

**Risk: `_smithers_nodes` lazy init may produce inaccurate `NodeTotal`.**
Mitigation: Skip the "Step N of M" line when `NodeTotal == 0`. The exec path via `smithers inspect` provides accurate DAG counts including unexecuted nodes and is the canonical fallback.

**Risk: `RunSummary` type name may conflict with the `runs-dashboard` plan.**
Mitigation: The `runs-dashboard` plan defined `RunSummary` as a node completion counter (different shape). If that ticket has landed, rename this one to `RunContext` and update all references. Add a `// TODO: reconcile with runs-dashboard RunSummary` comment to flag the overlap.

**Risk: Cache staleness after approve/deny (future tickets).**
Mitigation: `ClearRunSummaryCache(runID)` is exposed on the client so future approve/deny implementations can invalidate the cache. The 30-second TTL provides a natural safety net.

**Risk: Large payloads overflow the terminal height.**
Mitigation: Step 9 adds height-aware truncation with a `... (N more lines)` trailer. The exact line budget depends on `v.height` (available from `tea.WindowSizeMsg`). Ensure `v.height` is initialized to a reasonable default (e.g., `24`) in `NewApprovalsView` so tests that don't send `WindowSizeMsg` still get sensible truncation.

---

## Files To Touch

| File | Change |
|---|---|
| `internal/smithers/types.go` | Add `RunSummary` struct |
| `internal/smithers/client.go` | Add cache fields, `getRunSummaryCache`, `setRunSummaryCache`, `ClearRunSummaryCache`, `GetRunSummary`, `getRunSummaryDB`, `getRunSummaryExec`, `parseRunSummaryJSON`, `workflowNameFromPath` |
| `internal/smithers/client_test.go` | Add 6 `TestGetRunSummary_*` tests |
| `internal/ui/views/approvals.go` | New message types, view fields, `fetchRunContext`, updated `Update` handler, `renderDetail` rewrite, `renderListItem` rewrite, `renderListCompact` update, `formatWait`, `slaStyle`, `relativeTime` helpers |
| `internal/ui/views/approvals_test.go` | New file — 12 test cases |
| `internal/e2e/approvals_context_display_test.go` | New file — E2E test + mock server helper |
| `tests/vhs/approvals-context-display.tape` | New VHS tape |
