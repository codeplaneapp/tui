package views

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleTicket returns a ticket with multi-section markdown content.
func sampleTicket() smithers.Ticket {
	return smithers.Ticket{
		ID: "ticket-001",
		Content: "# Ticket 001\n\n## Summary\n\nThis is the summary.\n\n## Details\n\nSome details here.\n",
	}
}

// TestTicketDetailView_Init verifies Init returns nil (no async fetch needed).
func TestTicketDetailView_Init(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	cmd := v.Init()
	assert.Nil(t, cmd)
}

// TestTicketDetailView_Name verifies the view name.
func TestTicketDetailView_Name(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	assert.Equal(t, "ticket-detail", v.Name())
}

// TestTicketDetailView_SetSize verifies SetSize invalidates the render cache.
func TestTicketDetailView_SetSize(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	assert.Equal(t, 80, v.width)
	assert.Equal(t, 40, v.height)
	// Force a render to populate the cache.
	_ = v.View()
	assert.Equal(t, 80, v.renderedWidth)
	// Resize should invalidate.
	v.SetSize(120, 40)
	assert.Equal(t, 0, v.renderedWidth)
}

// TestTicketDetailView_ViewContainsTicketID verifies the header shows the ticket ID.
func TestTicketDetailView_ViewContainsTicketID(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	out := v.View()
	assert.Contains(t, out, "ticket-001")
}

// TestTicketDetailView_ViewContainsContent verifies content appears in the rendered output.
func TestTicketDetailView_ViewContainsContent(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	out := v.View()
	assert.Contains(t, out, "summary")
}

// TestTicketDetailView_ScrollDown verifies j/down increments scrollOffset.
func TestTicketDetailView_ScrollDown(t *testing.T) {
	ticket := smithers.Ticket{
		ID:      "ticket-002",
		Content: strings.Repeat("line\n", 60),
	}
	v := NewTicketDetailView(nil, nil, ticket)
	v.SetSize(80, 20)
	// Force a render so rendered lines are populated.
	_ = v.View()

	initial := v.scrollOffset
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	dv := updated.(*TicketDetailView)
	assert.Greater(t, dv.scrollOffset, initial)
}

// TestTicketDetailView_ScrollUp verifies k/up decrements scrollOffset.
func TestTicketDetailView_ScrollUp(t *testing.T) {
	ticket := smithers.Ticket{
		ID:      "ticket-002",
		Content: strings.Repeat("line\n", 60),
	}
	v := NewTicketDetailView(nil, nil, ticket)
	v.SetSize(80, 20)
	_ = v.View()

	// Move down first.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	v = updated.(*TicketDetailView)
	assert.Greater(t, v.scrollOffset, 0)

	// Now move up.
	updated, _ = v.Update(tea.KeyPressMsg{Code: 'k'})
	v = updated.(*TicketDetailView)
	assert.Equal(t, 0, v.scrollOffset)
}

// TestTicketDetailView_ScrollClampAtZero verifies scrollOffset doesn't go below 0.
func TestTicketDetailView_ScrollClampAtZero(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	_ = v.View()
	assert.Equal(t, 0, v.scrollOffset)

	// Up when already at top should stay at 0.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	dv := updated.(*TicketDetailView)
	assert.Equal(t, 0, dv.scrollOffset)
}

// TestTicketDetailView_GoTopBottom verifies g/G jump to top and bottom.
func TestTicketDetailView_GoTopBottom(t *testing.T) {
	ticket := smithers.Ticket{
		ID:      "ticket-003",
		Content: strings.Repeat("line\n", 60),
	}
	v := NewTicketDetailView(nil, nil, ticket)
	v.SetSize(80, 20)
	_ = v.View()

	// G goes to bottom.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'G'})
	v = updated.(*TicketDetailView)
	assert.Equal(t, v.maxScrollOffset(), v.scrollOffset)

	// g goes back to top.
	updated, _ = v.Update(tea.KeyPressMsg{Code: 'g'})
	v = updated.(*TicketDetailView)
	assert.Equal(t, 0, v.scrollOffset)
}

// TestTicketDetailView_EscEmitsPopViewMsg verifies Esc emits PopViewMsg.
func TestTicketDetailView_EscEmitsPopViewMsg(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "expected PopViewMsg")
}

// TestTicketDetailView_EditorHandoff_NoChange verifies no save cmd when content is unchanged.
func TestTicketDetailView_EditorHandoff_NoChange(t *testing.T) {
	ticket := smithers.Ticket{ID: "ticket-001", Content: "hello"}
	v := NewTicketDetailView(nil, nil, ticket)
	v.SetSize(80, 40)

	// Pre-populate temp file with identical content.
	tmpFile, err := os.CreateTemp("", "ticket-*.md")
	require.NoError(t, err)
	_, err = tmpFile.WriteString(ticket.Content)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	v.tmpPath = tmpFile.Name()
	defer os.Remove(tmpFile.Name())

	_, cmd := v.Update(handoff.HandoffMsg{Tag: "ticket-edit", Result: handoff.HandoffResult{}})
	assert.Nil(t, cmd, "no change → no save cmd")
}

// TestTicketDetailView_EditorHandoff_Changed verifies a save cmd is returned when content changes.
func TestTicketDetailView_EditorHandoff_Changed(t *testing.T) {
	ticket := smithers.Ticket{ID: "ticket-001", Content: "original content"}
	v := NewTicketDetailView(nil, nil, ticket)
	v.SetSize(80, 40)

	// Pre-populate temp file with different content.
	tmpFile, err := os.CreateTemp("", "ticket-*.md")
	require.NoError(t, err)
	_, err = tmpFile.WriteString("updated content")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	v.tmpPath = tmpFile.Name()
	// tmpFile is cleaned up by the Update handler.

	updated, cmd := v.Update(handoff.HandoffMsg{Tag: "ticket-edit", Result: handoff.HandoffResult{}})
	dv := updated.(*TicketDetailView)
	assert.NotNil(t, cmd, "content changed → save cmd must be non-nil")
	assert.True(t, dv.loading, "loading should be set while save is in flight")
}

// TestTicketDetailView_EditorHandoff_WrongTag verifies non-matching tags are ignored.
func TestTicketDetailView_EditorHandoff_WrongTag(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)

	_, cmd := v.Update(handoff.HandoffMsg{Tag: "other-tag", Result: handoff.HandoffResult{}})
	assert.Nil(t, cmd)
}

// TestTicketDetailView_ReloadedMsg verifies ticketDetailReloadedMsg updates the ticket.
func TestTicketDetailView_ReloadedMsg(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	v.loading = true

	newTicket := smithers.Ticket{ID: "ticket-001", Content: "# Updated\n\nNew content."}
	updated, cmd := v.Update(ticketDetailReloadedMsg{ticket: newTicket})
	dv := updated.(*TicketDetailView)

	assert.Nil(t, cmd)
	assert.False(t, dv.loading)
	assert.Equal(t, "# Updated\n\nNew content.", dv.ticket.Content)
	assert.Equal(t, 0, dv.scrollOffset, "scroll should reset on reload")
	assert.Equal(t, 0, dv.renderedWidth, "render cache should be invalidated on reload")
}

// TestTicketDetailView_ErrorMsg verifies ticketDetailErrorMsg sets the error.
func TestTicketDetailView_ErrorMsg(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	v.loading = true

	updated, cmd := v.Update(ticketDetailErrorMsg{err: fmt.Errorf("network error")})
	dv := updated.(*TicketDetailView)

	assert.Nil(t, cmd)
	assert.False(t, dv.loading)
	require.Error(t, dv.err)
	assert.Contains(t, dv.err.Error(), "network error")

	// Error should appear in View output.
	out := dv.View()
	assert.Contains(t, out, "Error:")
	assert.Contains(t, out, "network error")
}

// TestTicketDetailView_ShortHelp verifies the expected help bindings are present.
func TestTicketDetailView_ShortHelp(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	bindings := v.ShortHelp()
	require.NotEmpty(t, bindings)

	var keys []string
	for _, b := range bindings {
		keys = append(keys, b.Help().Key)
	}
	assert.Contains(t, keys, "↑↓/jk")
	assert.Contains(t, keys, "g/G")
	assert.Contains(t, keys, "e")
	assert.Contains(t, keys, "esc")
}

// TestTicketDetailView_LoadingState verifies the loading state renders a spinner/message.
func TestTicketDetailView_LoadingState(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	v.SetSize(80, 40)
	v.loading = true

	out := v.View()
	assert.Contains(t, out, "Saving...")
}

// TestTicketDetailView_WindowSizeMsg verifies window resize is handled.
func TestTicketDetailView_WindowSizeMsg(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	dv := updated.(*TicketDetailView)
	assert.Nil(t, cmd)
	assert.Equal(t, 120, dv.width)
	assert.Equal(t, 50, dv.height)
}

// --- feat-tickets-edit-inline: TicketDetailView edit mode ---

// TestTicketDetailViewEditMode_InitFiresEditor verifies that Init() returns a non-nil command
// when the view is constructed in edit mode.
func TestTicketDetailViewEditMode_InitFiresEditor(t *testing.T) {
	// We need $EDITOR to resolve to something; set it to a known binary.
	t.Setenv("EDITOR", "true")

	v := NewTicketDetailViewEditMode(nil, nil, sampleTicket())
	cmd := v.Init()
	// startEditor writes a temp file and returns a handoff command; it must be non-nil.
	assert.NotNil(t, cmd, "Init in edit mode must return a non-nil editor command")
	// autoEdit flag must be cleared after Init so subsequent Init calls are no-ops.
	assert.False(t, v.autoEdit, "autoEdit must be cleared after Init")
}

// TestTicketDetailViewEditMode_InitOnlyOnce verifies that Init() is a no-op on the second call.
func TestTicketDetailViewEditMode_InitOnlyOnce(t *testing.T) {
	t.Setenv("EDITOR", "true")

	v := NewTicketDetailViewEditMode(nil, nil, sampleTicket())
	// First Init fires the editor.
	first := v.Init()
	assert.NotNil(t, first)

	// Second Init must be a no-op (autoEdit was cleared).
	second := v.Init()
	assert.Nil(t, second, "second Init must not fire the editor again")
}

// TestNewTicketDetailViewEditMode_Name verifies the view name is unchanged.
func TestNewTicketDetailViewEditMode_Name(t *testing.T) {
	v := NewTicketDetailViewEditMode(nil, nil, sampleTicket())
	assert.Equal(t, "ticket-detail", v.Name())
}

// TestNewTicketDetailView_InitIsNoop verifies the regular constructor does not fire the editor.
func TestNewTicketDetailView_InitIsNoop(t *testing.T) {
	v := NewTicketDetailView(nil, nil, sampleTicket())
	cmd := v.Init()
	assert.Nil(t, cmd, "regular constructor must not fire editor on Init")
}
