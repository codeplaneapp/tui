package views

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleTickets returns n synthetic Ticket values with metadata + summary sections.
func sampleTickets(n int) []smithers.Ticket {
	t := make([]smithers.Ticket, n)
	for i := range n {
		t[i] = smithers.Ticket{
			ID: fmt.Sprintf("ticket-%03d", i+1),
			Content: fmt.Sprintf(
				"# Ticket %d\n\n## Metadata\n- ID: ticket-%03d\n- Group: test\n\n## Summary\n\nSummary for ticket %d.",
				i+1, i+1, i+1,
			),
		}
	}
	return t
}

// loadedView is a helper that creates a TicketsView and fires a ticketsLoadedMsg.
func loadedView(tickets []smithers.Ticket, width, height int) *TicketsView {
	v := NewTicketsView(nil)
	v.width = width
	v.height = height
	updated, _ := v.Update(ticketsLoadedMsg{tickets: tickets})
	return updated.(*TicketsView)
}

// --- Tests ---

func TestTicketsView_Init(t *testing.T) {
	v := NewTicketsView(nil)
	// loading must be true before Init fires.
	assert.True(t, v.loading)
	// Init returns a non-nil command (the fetch closure).
	cmd := v.Init()
	assert.NotNil(t, cmd)
}

func TestTicketsView_LoadedMsg(t *testing.T) {
	v := loadedView(sampleTickets(3), 80, 40)
	assert.False(t, v.loading)
	assert.Len(t, v.tickets, 3)
	output := v.View()
	assert.Contains(t, output, "ticket-001")
	assert.Contains(t, output, "ticket-002")
	assert.Contains(t, output, "ticket-003")
}

func TestTicketsView_ErrorMsg(t *testing.T) {
	v := NewTicketsView(nil)
	v.width = 80
	v.height = 40
	updated, _ := v.Update(ticketsErrorMsg{err: errors.New("connection refused")})
	tv := updated.(*TicketsView)
	assert.False(t, tv.loading)
	out := tv.View()
	assert.Contains(t, out, "Error:")
	assert.Contains(t, out, "connection refused")
}

func TestTicketsView_EmptyList(t *testing.T) {
	v := loadedView([]smithers.Ticket{}, 80, 40)
	assert.Contains(t, v.View(), "No tickets found")
}

func TestTicketsView_CursorNavigation(t *testing.T) {
	v := loadedView(sampleTickets(5), 80, 40)
	assert.Equal(t, 0, v.listPane.cursor)

	// Move down 3 times.
	for i := 0; i < 3; i++ {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 3, v.listPane.cursor)

	// Move up once.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	v = updated.(*TicketsView)
	assert.Equal(t, 2, v.listPane.cursor)

	// Move up 5 times — clamped at 0.
	for i := 0; i < 5; i++ {
		updated, _ = v.Update(tea.KeyPressMsg{Code: 'k'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 0, v.listPane.cursor)

	// Move down past end — clamped at len-1.
	for i := 0; i < 10; i++ {
		updated, _ = v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 4, v.listPane.cursor)
}

func TestTicketsView_PageNavigation(t *testing.T) {
	// Terminal with height=20 gives pageSize = (20-4)/3 = 5.
	v := loadedView(sampleTickets(20), 80, 20)
	assert.Equal(t, 0, v.listPane.cursor)

	// PgDn should jump by pageSize.
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	v = updated.(*TicketsView)
	ps := v.listPane.pageSize()
	assert.Equal(t, ps, v.listPane.cursor)

	// PgUp should return to 0.
	updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	v = updated.(*TicketsView)
	assert.Equal(t, 0, v.listPane.cursor)

	// PgDn past end should clamp to last.
	for i := 0; i < 10; i++ {
		updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 19, v.listPane.cursor)

	// PgUp past beginning should clamp to 0.
	for i := 0; i < 10; i++ {
		updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 0, v.listPane.cursor)
}

func TestTicketsView_HomeEnd(t *testing.T) {
	v := loadedView(sampleTickets(10), 80, 40)

	// Move to middle.
	for i := 0; i < 5; i++ {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 5, v.listPane.cursor)

	// End (G) jumps to last.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'G'})
	v = updated.(*TicketsView)
	assert.Equal(t, 9, v.listPane.cursor)

	// Home (g) jumps to first.
	updated, _ = v.Update(tea.KeyPressMsg{Code: 'g'})
	v = updated.(*TicketsView)
	assert.Equal(t, 0, v.listPane.cursor)
	assert.Equal(t, 0, v.listPane.scrollOffset)
}

func TestTicketsView_Refresh(t *testing.T) {
	v := loadedView(sampleTickets(2), 80, 40)
	assert.False(t, v.loading)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	tv := updated.(*TicketsView)
	assert.True(t, tv.loading)
	assert.NotNil(t, cmd)
}

func TestTicketsView_Escape(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok)
}

func TestTicketsView_CursorIndicator(t *testing.T) {
	v := loadedView(sampleTickets(3), 80, 40)
	output := v.View()
	assert.Contains(t, output, "▸ ")
	// First item should have the cursor on the same line.
	lines := strings.Split(output, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "▸") && strings.Contains(line, "ticket-001") {
			found = true
			break
		}
	}
	assert.True(t, found, "cursor indicator should be on the first ticket line")
}

func TestTicketsView_HeaderCount(t *testing.T) {
	// Header should not show count while loading.
	v := NewTicketsView(nil)
	v.width = 80
	v.height = 40
	out := v.View()
	assert.NotContains(t, out, "(")

	// Header should show count after load.
	v2 := loadedView(sampleTickets(7), 80, 40)
	out2 := v2.View()
	assert.Contains(t, out2, "(7)")
}

func TestTicketsView_ScrollOffset(t *testing.T) {
	// Height=10: pageSize = (10-4)/3 = 2.
	v := loadedView(sampleTickets(10), 80, 10)
	ps := v.listPane.pageSize()

	// Advance cursor past the first page.
	for i := 0; i < ps+2; i++ {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}

	// Render to trigger scroll offset update.
	v.View()

	// scrollOffset should have advanced so cursor stays visible.
	assert.LessOrEqual(t, v.listPane.scrollOffset, v.listPane.cursor)
	assert.Greater(t, v.listPane.scrollOffset+ps, v.listPane.cursor)
}

// --- ticketSnippet ---

func TestTicketSnippet(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "summary heading preferred",
			content: "# Title\n\n## Metadata\n- ID: foo\n- Group: bar\n\n## Summary\n\nActual summary here.",
			want:    "Actual summary here.",
		},
		{
			name:    "plain paragraph fallback",
			content: "# Title\n\nThis is the first paragraph.",
			want:    "This is the first paragraph.",
		},
		{
			name:    "metadata only skips all",
			content: "# Title\n\n## Metadata\n- ID: foo\n- Group: bar\n",
			want:    "",
		},
		{
			name:    "long line truncated",
			content: "# T\n\n## Summary\n\n" + strings.Repeat("x", 100),
			want:    strings.Repeat("x", 77) + "...",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "description heading also works",
			content: "# T\n\n## Description\n\nSome description text.",
			want:    "Some description text.",
		},
		{
			name:    "separator lines skipped",
			content: "# T\n---\n\n## Summary\n\nContent after separator.",
			want:    "Content after separator.",
		},
		{
			name:    "zero maxLen defaults to 80",
			content: "# T\n\n" + strings.Repeat("y", 100),
			want:    strings.Repeat("y", 77) + "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ticketSnippet(tt.content, 80)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTicketSnippet_DefaultMaxLen(t *testing.T) {
	// maxLen <= 0 should behave the same as maxLen=80.
	content := "# T\n\n## Summary\n\n" + strings.Repeat("z", 100)
	assert.Equal(t, ticketSnippet(content, 80), ticketSnippet(content, 0))
}

// TestTicketsView_EnterEmitsOpenDetail verifies Enter on the list emits OpenTicketDetailMsg.
func TestTicketsView_EnterEmitsOpenDetail(t *testing.T) {
	v := loadedView(sampleTickets(3), 100, 30)
	// Default focus is left pane; send Enter.
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = updated
	require.NotNil(t, cmd)
	msg := cmd()
	detail, ok := msg.(OpenTicketDetailMsg)
	require.True(t, ok, "expected OpenTicketDetailMsg, got %T", msg)
	assert.Equal(t, "ticket-001", detail.Ticket.ID)
}

// TestTicketsView_EnterNoTickets verifies Enter on an empty list emits no cmd.
func TestTicketsView_EnterNoTickets(t *testing.T) {
	v := loadedView([]smithers.Ticket{}, 100, 30)
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
}

// --- feat-tickets-create ('n' key) ---

// TestTicketsView_NKeyOpensCreatePrompt verifies 'n' activates the inline create prompt.
func TestTicketsView_NKeyOpensCreatePrompt(t *testing.T) {
	v := loadedView(sampleTickets(2), 80, 40)
	assert.False(t, v.createPrompt.active)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	tv := updated.(*TicketsView)
	assert.True(t, tv.createPrompt.active)

	// View should show the prompt, not the ticket list.
	out := tv.View()
	assert.Contains(t, out, "New ticket ID:")
	assert.Contains(t, out, "[Enter] create")
	assert.Contains(t, out, "[Esc] cancel")
}

// TestTicketsView_NKeyNoopWhileLoading verifies 'n' is ignored while loading.
func TestTicketsView_NKeyNoopWhileLoading(t *testing.T) {
	v := NewTicketsView(nil)
	v.width = 80
	v.height = 40

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	tv := updated.(*TicketsView)
	assert.False(t, tv.createPrompt.active)
}

// TestTicketsView_CreatePromptEscDismisses verifies Esc dismisses the create prompt.
func TestTicketsView_CreatePromptEscDismisses(t *testing.T) {
	v := loadedView(sampleTickets(2), 80, 40)

	// Open prompt.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	v = updated.(*TicketsView)
	require.True(t, v.createPrompt.active)

	// Dismiss with Esc.
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	v = updated.(*TicketsView)
	assert.False(t, v.createPrompt.active)
	assert.Nil(t, cmd)
}

// TestTicketsView_CreatePromptEnterEmptyNoOp verifies Enter with empty input is a no-op.
func TestTicketsView_CreatePromptEnterEmptyNoOp(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	v = updated.(*TicketsView)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter with empty ID must not emit a create command")
	assert.True(t, v.createPrompt.active, "prompt should still be active")
}

// TestTicketsView_CreatePromptSubmit verifies Enter with a non-empty ID returns a create command.
func TestTicketsView_CreatePromptSubmit(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)

	// Open prompt.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	v = updated.(*TicketsView)

	// Directly set the input value (mirrors how runs_test.go drives the textinput).
	v.createPrompt.input.SetValue("my-ticket")
	assert.Equal(t, "my-ticket", v.createPrompt.input.Value())

	// Submit.
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.NotNil(t, cmd, "Enter with non-empty ID must emit a create command")
}

// TestTicketsView_TicketCreatedMsgRefreshes verifies ticketCreatedMsg dismisses the prompt and refreshes.
func TestTicketsView_TicketCreatedMsgRefreshes(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)

	// Simulate the prompt being open.
	v.createPrompt.active = true

	newTicket := smithers.Ticket{ID: "new-ticket", Content: "# New"}
	updated, cmd := v.Update(ticketCreatedMsg{ticket: newTicket})
	tv := updated.(*TicketsView)

	assert.False(t, tv.createPrompt.active, "prompt must be dismissed after creation")
	assert.True(t, tv.loading, "view must be refreshing after creation")
	assert.NotNil(t, cmd, "a refresh command must be returned")
	assert.Equal(t, "", tv.createPrompt.input.Value(), "input must be reset")
}

// TestTicketsView_TicketCreateErrorMsgSurfacesError verifies ticketCreateErrorMsg shows error in prompt.
func TestTicketsView_TicketCreateErrorMsgSurfacesError(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)
	v.createPrompt.active = true

	updated, cmd := v.Update(ticketCreateErrorMsg{err: errors.New("ticket already exists")})
	tv := updated.(*TicketsView)

	assert.Nil(t, cmd)
	assert.True(t, tv.createPrompt.active, "prompt must stay active on error")
	require.Error(t, tv.createPrompt.err)
	assert.Contains(t, tv.createPrompt.err.Error(), "ticket already exists")

	// Error must appear in the View output.
	out := tv.View()
	assert.Contains(t, out, "ticket already exists")
}

// TestTicketsView_CreatePromptShortHelp verifies ShortHelp returns prompt-specific bindings.
func TestTicketsView_CreatePromptShortHelp(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)
	v.createPrompt.active = true

	bindings := v.ShortHelp()
	var keys []string
	for _, b := range bindings {
		keys = append(keys, b.Help().Key)
	}
	assert.Contains(t, keys, "enter")
	assert.Contains(t, keys, "esc")
}

// --- feat-tickets-edit-inline ('e' key) ---

// TestTicketsView_EKeyEmitsOpenDetailEditMode verifies 'e' on a selected ticket emits
// OpenTicketDetailMsg with EditMode=true.
func TestTicketsView_EKeyEmitsOpenDetailEditMode(t *testing.T) {
	v := loadedView(sampleTickets(3), 100, 30)
	// Default focus is the left pane (list).

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'e'})
	_ = updated
	require.NotNil(t, cmd)

	msg := cmd()
	detail, ok := msg.(OpenTicketDetailMsg)
	require.True(t, ok, "expected OpenTicketDetailMsg, got %T", msg)
	assert.Equal(t, "ticket-001", detail.Ticket.ID)
	assert.True(t, detail.EditMode, "EditMode must be true when 'e' is pressed")
}

// TestTicketsView_EKeyNoTicketsNoCmd verifies 'e' on an empty list emits no command.
func TestTicketsView_EKeyNoTicketsNoCmd(t *testing.T) {
	v := loadedView([]smithers.Ticket{}, 100, 30)
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'e'})
	assert.Nil(t, cmd)
}

// TestTicketsView_EKeyNotOnPrompt verifies 'e' is a no-op when the create prompt is active.
func TestTicketsView_EKeyNotOnPrompt(t *testing.T) {
	v := loadedView(sampleTickets(2), 80, 40)
	v.createPrompt.active = true

	// 'e' while prompt is active should route to the textinput, not open detail.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'e'})
	tv := updated.(*TicketsView)
	// Prompt must still be active — the key was consumed by the textinput.
	assert.True(t, tv.createPrompt.active)
}

// TestTicketsView_ShortHelpHasEditAndNew verifies ShortHelp includes 'e' and 'n' in list mode.
func TestTicketsView_ShortHelpHasEditAndNew(t *testing.T) {
	v := loadedView(sampleTickets(1), 80, 40)
	bindings := v.ShortHelp()
	var keys []string
	for _, b := range bindings {
		keys = append(keys, b.Help().Key)
	}
	assert.Contains(t, keys, "e")
	assert.Contains(t, keys, "n")
}

func TestMetadataLine(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"- ID: foo", true},
		{"- Group: bar", true},
		{"- Type: feature", true},
		{"- Some bullet point", false},      // no colon
		{"- multi word key: val", false},    // key has space
		{"Normal paragraph text", false},    // doesn't start with "- "
		{"- : missing key", false},          // empty key (colon at position 0)
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, metadataLine(tt.input))
		})
	}
}
