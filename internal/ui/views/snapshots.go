package views

// SnapshotsOpenSource identifies where the user opened the snapshots view from.
type SnapshotsOpenSource string

const (
	SnapshotsOpenSourceRuns       SnapshotsOpenSource = "runs"
	SnapshotsOpenSourceRunInspect SnapshotsOpenSource = "run_inspect"
	SnapshotsOpenSourceUnknown    SnapshotsOpenSource = "unknown"
)

// OpenSnapshotsMsg signals ui.go to push the snapshots view for the given run.
type OpenSnapshotsMsg struct {
	RunID  string
	Source SnapshotsOpenSource
}
