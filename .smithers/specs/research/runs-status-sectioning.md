## Existing Crush Surface

### Section grouping pattern in ApprovalsView

The canonical reference for section grouping already exists at
`internal/ui/views/approvals.go:177-211`. `ApprovalsView.renderList` partitions
the flat `[]smithers.Approval` slice into two index-slices (`pending`, `resolved`)
and writes a bold-faint section header before each group:

```go
sectionHeader := lipgloss.NewStyle().Bold(true).Faint(true)
if len(pending) > 0 {
    b.WriteString(sectionHeader.Render("Pending") + "\n")
    for _, idx := range pending { b.WriteString(v.renderListItem(idx, width)) }
}
```

The cursor index is a flat integer into `v.approvals`. The cursor navigates
freely across sections because items are stored in one slice and the section
headers are not selectable rows. This pattern is the direct model for the runs
sectioning work.

### RunsView and RunTable state (runs-dashboard baseline)

`internal/ui/views/runs.go` holds:
- `runs []smithers.RunSummary` — a single flat slice loaded once from
  `client.ListRuns` at Init time.
- `cursor int` — flat index into `runs`.

`internal/ui/components/runtable.go` iterates `t.Runs` linearly, rendering one
row per run. It has no section concept.

The `cursor` bounds are enforced in `Update`:
```go
case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
    if v.cursor > 0 { v.cursor-- }
case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
    if v.cursor < len(v.runs)-1 { v.cursor++ }
```

### RunStatus constants and IsTerminal helper

`internal/smithers/types_runs.go` defines:

```go
const (
    RunStatusRunning         RunStatus = "running"
    RunStatusWaitingApproval RunStatus = "waiting-approval"
    RunStatusWaitingEvent    RunStatus = "waiting-event"
    RunStatusFinished        RunStatus = "finished"
    RunStatusFailed          RunStatus = "failed"
    RunStatusCancelled       RunStatus = "cancelled"
)

func (s RunStatus) IsTerminal() bool { ... }  // finished | failed | cancelled
```

There is no `IsWaiting()` helper but waiting statuses are
`WaitingApproval | WaitingEvent`. The design doc groups `running + waiting-*`
under a single "Active" section label.

### Design doc section layout

`docs/smithers-tui/02-DESIGN.md:142-172` shows the intended three-section wireframe:

```
● ACTIVE (3)
  ▸ abc123  code-review       ████████░░  3/5 nodes  2m 14s
  ▸ def456  deploy-staging    ██████░░░░  4/6 nodes  8m 02s  ⚠ 1
  ▸ ghi789  test-suite        ██░░░░░░░░  1/3 nodes  30s

─────────────────────────────
● COMPLETED TODAY (12)
  jkl012  code-review  ...  ✓
  ...

─────────────────────────────
● FAILED (1)
  stu901  dependency-update  ...  ✗
```

Section order: Active → Completed → Failed. The ticket acceptance criteria also
mention a "Waiting" section. The design shows "Active" as the union of running +
waiting; splitting into Running / Waiting / Completed / Failed is a minor variant
that can be resolved during implementation — the structural code is the same.

### Cursor navigation across sections

The design doc (`02-DESIGN.md:175`) says `↑`/`↓` navigate runs. Section headers
are non-selectable. The accepted pattern (from ApprovalsView and generic list
UIs) is to keep the cursor as a flat index over selectable rows only, with
header rows rendered as presentation elements that are not counted in cursor
arithmetic.

Two approaches exist:

**Option A — index map**: Build a `[]int` for each section containing indices
into the flat `runs` slice. The cursor addresses positions within a logical
sequence that skips headers. A `flatIndex(cursor) int` helper converts the
navigable position to the actual runs-slice index.

**Option B — virtual row list**: Append all items to a `[]virtualRow{kind,
idx}` where kind is `rowHeader | rowRun`. Navigation increments/decrements the
cursor, skipping header rows automatically. This matches how bubbles/list handles
section headers internally.

Option B is simpler to render (one loop over virtual rows), and simpler to
navigate (one loop that skips headers on increment/decrement). It is the
recommended approach.

### Ticket implementation note from tickets.json

```
"implementationNotes": [
  "Update the list component to support section headers that cannot be selected."
]
```

This confirms the virtual-row / non-selectable-header approach. The change is
localized to `runtable.go` (or a new `RunSectionedTable` component) and
`runs.go` (cursor navigation logic).

### lipgloss styling for section headers

The design doc uses colored bullets (`● ACTIVE`, `● COMPLETED TODAY`, `● FAILED`)
with horizontal dividers between sections. lipgloss equivalents:

- Section header with bullet: `lipgloss.NewStyle().Bold(true).Render("● ACTIVE (3)")`
- Status-colored bullet: Use the same `statusStyle()` already in `runtable.go`
  applied only to the `●` character, with the label in bold plain text.
- Divider: `strings.Repeat("─", width)` rendered with `lipgloss.NewStyle().Faint(true)`
- Count badge: rendered as part of the header string, e.g. `fmt.Sprintf("● ACTIVE (%d)", n)`

### VHS test infrastructure

Existing VHS tapes live in `tests/vhs/`. Fixtures are injected via
`CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures`. The new tape for this ticket should
use a multi-status fixture JSON loaded via the mock server pattern established
in `runs-dashboard` E2E tests. A minimal tape should scroll through sections and
verify the cursor skips headers.

---

## Upstream Smithers Reference

### API contract (no changes needed)

`GET /v1/runs?limit=50` already returns all statuses in one response. The
sectioning is a pure client-side partition — no new API calls, no new query
parameters for status filtering (that is `RUNS_FILTER_BY_STATUS`, a separate
ticket). The existing `client.ListRuns(ctx, RunFilter{Limit: 50})` call in
`RunsView.Init()` is sufficient.

### Status grouping logic

The three sections map to statuses as follows:

| Section label | RunStatus values |
|---|---|
| ACTIVE | `running`, `waiting-approval`, `waiting-event` |
| COMPLETED | `finished`, `cancelled` |
| FAILED | `failed` |

`cancelled` is grouped with Completed because it is a terminal non-error state.
Alternatively, cancelled runs could form their own section — this is a design
decision for the implementer to validate against the design doc wireframe.

---

## Data Flow: No New I/O

The sectioning enhancement introduces zero new I/O paths. The data flow is
unchanged from `runs-dashboard`:

```
RunsView.Init() → client.ListRuns() → runsLoadedMsg{runs}
  → RunsView.Update() stores runs
  → RunsView.View() partitions runs into sections at render time
  → RunTable renders sections with headers and row items
```

Partitioning is stateless and performed in `View()` (or in `RunTable.View()`)
on every render call. This is consistent with the ApprovalsView pattern where
`renderList` partitions items on every call.

---

## Key Files

- `/Users/williamcory/crush/internal/ui/views/runs.go` — cursor navigation update
- `/Users/williamcory/crush/internal/ui/components/runtable.go` — section rendering
- `/Users/williamcory/crush/internal/smithers/types_runs.go` — status constants used for partition logic
- `/Users/williamcory/crush/internal/ui/views/approvals.go` — section grouping reference implementation
- `/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md:132-172` — wireframe for section layout
