package styles

import "charm.land/lipgloss/v2"

// SmithersStyles holds tool-rendering styles specific to Smithers MCP tools.
// It is embedded in the Tool sub-struct of Styles.
type SmithersStyles struct {
	// ServerName is the "Smithers" label in "Smithers → runs list"
	ServerName lipgloss.Style

	// Run status badges
	StatusRunning  lipgloss.Style // green — actively executing
	StatusApproval lipgloss.Style // yellow — waiting for approval gate
	StatusComplete lipgloss.Style // muted green — finished successfully
	StatusFailed   lipgloss.Style // red — terminal failure
	StatusCanceled lipgloss.Style // subtle/grey — canceled by user or system
	StatusPaused   lipgloss.Style // yellow-ish — paused/waiting

	// Action card styles
	CardTitle    lipgloss.Style // card header text (bold)
	CardValue    lipgloss.Style // card value text
	CardLabel    lipgloss.Style // card field label (muted)
	CardApproved lipgloss.Style // "APPROVED" badge (green bg)
	CardDenied   lipgloss.Style // "DENIED" badge (red bg)
	CardCanceled lipgloss.Style // "CANCELED" badge (subtle bg)
	CardStarted  lipgloss.Style // "STARTED" badge (blue bg)
	CardDone     lipgloss.Style // "DONE" badge (blue bg)

	// Table header style
	TableHeader lipgloss.Style // column header row (muted, bold)

	// Tree node indicator styles
	TreeNodeRunning  lipgloss.Style // ● green — node actively running
	TreeNodeComplete lipgloss.Style // ✓ muted green — node done
	TreeNodeFailed   lipgloss.Style // × red — node failed
	TreeNodePending  lipgloss.Style // ○ subtle — node not yet started
}
