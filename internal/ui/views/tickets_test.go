package views

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleTickets(n int) []smithers.Ticket {
	tickets := make([]smithers.Ticket, n)
	for i := range n {
		tickets[i] = smithers.Ticket{
			ID: fmt.Sprintf("ticket-%03d", i+1),
			Content: fmt.Sprintf(
				"# Ticket %d\n\n## Metadata\n- ID: ticket-%03d\n- Group: test\n\n## Summary\n\nSummary for ticket %d.",
				i+1, i+1, i+1,
			),
		}
	}
	return tickets
}

func localOnlyTicketsView() *TicketsView {
	return NewTicketsViewWithSources(nil, nil, nil)
}

func loadedLocalView(tickets []smithers.Ticket, width, height int) *TicketsView {
	v := localOnlyTicketsView()
	v.SetSize(width, height)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: tickets})
	return updated.(*TicketsView)
}

func helpKeys(bindings []key.Binding) []string {
	keys := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		keys = append(keys, binding.Help().Key)
	}
	return keys
}

func TestTicketsView_Init(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	assert.True(t, v.loading)
	assert.NotNil(t, v.Init())
}

func TestTicketsView_LoadedMsg(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(3), 80, 40)
	assert.False(t, v.loading)
	assert.Len(t, v.activeItems(), 3)
	output := v.View()
	assert.Contains(t, output, "ticket-001")
	assert.Contains(t, output, "ticket-002")
	assert.Contains(t, output, "ticket-003")
}

func TestTicketsView_ErrorMsg(t *testing.T) {
	t.Parallel()

	v := localOnlyTicketsView()
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsErrorMsg{err: errors.New("connection refused")})
	tv := updated.(*TicketsView)
	assert.False(t, tv.loading)
	out := tv.View()
	assert.Contains(t, out, "Error:")
	assert.Contains(t, out, "connection refused")
}

func TestTicketsView_EmptyList(t *testing.T) {
	t.Parallel()

	v := loadedLocalView([]smithers.Ticket{}, 80, 40)
	assert.Contains(t, v.View(), "No tickets found.")
}

func TestTicketsView_CursorNavigation(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(5), 80, 40)
	assert.Equal(t, 0, v.listPane.cursor)

	for range 3 {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 3, v.listPane.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	v = updated.(*TicketsView)
	assert.Equal(t, 2, v.listPane.cursor)

	for range 5 {
		updated, _ = v.Update(tea.KeyPressMsg{Code: 'k'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 0, v.listPane.cursor)

	for range 10 {
		updated, _ = v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 4, v.listPane.cursor)
}

func TestTicketsView_PageNavigation(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(20), 80, 20)
	assert.Equal(t, 0, v.listPane.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	v = updated.(*TicketsView)
	ps := v.listPane.pageSize()
	assert.Equal(t, ps, v.listPane.cursor)

	updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	v = updated.(*TicketsView)
	assert.Equal(t, 0, v.listPane.cursor)

	for range 10 {
		updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 19, v.listPane.cursor)

	for range 10 {
		updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 0, v.listPane.cursor)
}

func TestTicketsView_HomeEnd(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(10), 80, 40)
	for range 5 {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}
	assert.Equal(t, 5, v.listPane.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'G'})
	v = updated.(*TicketsView)
	assert.Equal(t, 9, v.listPane.cursor)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'g'})
	v = updated.(*TicketsView)
	assert.Equal(t, 0, v.listPane.cursor)
	assert.Equal(t, 0, v.listPane.scrollOffset)
}

func TestTicketsView_Refresh(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: sampleTickets(2)})
	v = updated.(*TicketsView)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	tv := updated.(*TicketsView)
	assert.True(t, tv.loading)
	assert.NotNil(t, cmd)
}

func TestTicketsView_Escape(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(1), 80, 40)
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, ok := cmd().(PopViewMsg)
	assert.True(t, ok)
}

func TestTicketsView_CursorIndicator(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(3), 80, 40)
	output := v.View()
	assert.Contains(t, output, "▸ ")

	lines := strings.Split(output, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "▸") && strings.Contains(line, "ticket-001") {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestTicketsView_HeaderCount(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	assert.NotContains(t, v.View(), "(7)")

	v2 := loadedLocalView(sampleTickets(7), 80, 40)
	assert.Contains(t, v2.View(), "Tickets (7)")
}

func TestTicketsView_ScrollOffset(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(10), 80, 10)
	ps := v.listPane.pageSize()

	for range ps + 2 {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TicketsView)
	}

	v.View()
	assert.LessOrEqual(t, v.listPane.scrollOffset, v.listPane.cursor)
	assert.Greater(t, v.listPane.scrollOffset+ps, v.listPane.cursor)
}

func TestTicketsView_EnterEmitsOpenDetail(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(3), 100, 30)
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	detail, ok := msg.(OpenTicketDetailMsg)
	require.True(t, ok)
	assert.Equal(t, "ticket-001", detail.Ticket.ID)
}

func TestTicketsView_EnterNoTickets(t *testing.T) {
	t.Parallel()

	v := loadedLocalView([]smithers.Ticket{}, 100, 30)
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
}

func TestTicketsView_NKeyOpensCreatePrompt(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: sampleTickets(2)})
	v = updated.(*TicketsView)
	assert.False(t, v.createPrompt.active)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'n'})
	tv := updated.(*TicketsView)
	assert.True(t, tv.createPrompt.active)

	out := tv.View()
	assert.Contains(t, out, "New ticket ID:")
	assert.Contains(t, out, "[Enter] create")
	assert.Contains(t, out, "[Esc] cancel")
}

func TestTicketsView_NKeyNoopWhileLoading(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	tv := updated.(*TicketsView)
	assert.False(t, tv.createPrompt.active)
}

func TestTicketsView_CreatePromptEscDismisses(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: sampleTickets(2)})
	v = updated.(*TicketsView)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'n'})
	v = updated.(*TicketsView)
	require.True(t, v.createPrompt.active)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	v = updated.(*TicketsView)
	assert.False(t, v.createPrompt.active)
	assert.Nil(t, cmd)
}

func TestTicketsView_CreatePromptEnterEmptyNoOp(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: sampleTickets(1)})
	v = updated.(*TicketsView)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'n'})
	v = updated.(*TicketsView)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
	assert.True(t, v.createPrompt.active)
}

func TestTicketsView_CreatePromptSubmit(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: sampleTickets(1)})
	v = updated.(*TicketsView)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'n'})
	v = updated.(*TicketsView)
	v.createPrompt.input.SetValue("my-ticket")

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.NotNil(t, cmd)
}

func TestTicketsView_TicketCreatedMsgRefreshes(t *testing.T) {
	t.Parallel()

	v := NewTicketsViewWithSources(smithers.NewClient(), nil, nil)
	v.SetSize(80, 40)
	updated, _ := v.Update(ticketsLoadedMsg{tickets: sampleTickets(1)})
	v = updated.(*TicketsView)
	v.createPrompt.active = true

	newTicket := smithers.Ticket{ID: "new-ticket", Content: "# New"}
	updated, cmd := v.Update(ticketCreatedMsg{ticket: newTicket})
	tv := updated.(*TicketsView)

	assert.False(t, tv.createPrompt.active)
	assert.True(t, tv.loading)
	assert.NotNil(t, cmd)
	assert.Equal(t, "", tv.createPrompt.input.Value())
}

func TestTicketsView_TicketCreateErrorMsgSurfacesError(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(1), 80, 40)
	v.createPrompt.active = true

	updated, cmd := v.Update(ticketCreateErrorMsg{err: errors.New("ticket already exists")})
	tv := updated.(*TicketsView)

	assert.Nil(t, cmd)
	assert.True(t, tv.createPrompt.active)
	require.Error(t, tv.createPrompt.err)
	assert.Contains(t, tv.createPrompt.err.Error(), "ticket already exists")
	assert.Contains(t, tv.View(), "ticket already exists")
}

func TestTicketsView_CreatePromptShortHelp(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(1), 80, 40)
	v.createPrompt.active = true
	keys := helpKeys(v.ShortHelp())
	assert.Contains(t, keys, "enter")
	assert.Contains(t, keys, "esc")
}

func TestTicketsView_EKeyEmitsOpenDetailEditMode(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(3), 100, 30)
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'e'})
	require.NotNil(t, cmd)

	msg := cmd()
	detail, ok := msg.(OpenTicketDetailMsg)
	require.True(t, ok)
	assert.Equal(t, "ticket-001", detail.Ticket.ID)
	assert.True(t, detail.EditMode)
}

func TestTicketsView_EKeyNoTicketsNoCmd(t *testing.T) {
	t.Parallel()

	v := loadedLocalView([]smithers.Ticket{}, 100, 30)
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'e'})
	assert.Nil(t, cmd)
}

func TestTicketsView_EKeyNotOnPrompt(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(2), 80, 40)
	v.createPrompt.active = true

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'e'})
	tv := updated.(*TicketsView)
	assert.True(t, tv.createPrompt.active)
}

func TestTicketsView_ShortHelpHasEditAndNew(t *testing.T) {
	t.Parallel()

	v := loadedLocalView(sampleTickets(1), 80, 40)
	keys := helpKeys(v.ShortHelp())
	assert.Contains(t, keys, "e")
	assert.Contains(t, keys, "n")
}

func TestTicketSnippet(t *testing.T) {
	t.Parallel()

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
			assert.Equal(t, tt.want, ticketSnippet(tt.content, 80))
		})
	}
}

func TestTicketSnippet_DefaultMaxLen(t *testing.T) {
	t.Parallel()

	content := "# T\n\n## Summary\n\n" + strings.Repeat("z", 100)
	assert.Equal(t, ticketSnippet(content, 80), ticketSnippet(content, 0))
}

func TestMetadataLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"- ID: foo", true},
		{"- Group: bar", true},
		{"- Type: feature", true},
		{"- Some bullet point", false},
		{"- multi word key: val", false},
		{"Normal paragraph text", false},
		{"- : missing key", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, metadataLine(tt.input))
		})
	}
}

