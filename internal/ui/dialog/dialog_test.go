package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandType_String(t *testing.T) {
	tests := []struct {
		ct       CommandType
		expected string
	}{
		{SystemCommands, "System"},
		{UserCommands, "User"},
		{MCPPrompts, "MCP"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.ct.String())
		})
	}
}

func TestOverlay_OpenAndClose(t *testing.T) {
	st := styles.DefaultStyles()
	overlay := NewOverlay(&st)

	assert.False(t, overlay.HasDialogs(), "overlay should start empty")
	assert.Nil(t, overlay.DialogLast(), "no front dialog when empty")

	d1 := &stubDialog{id: "d1"}
	d2 := &stubDialog{id: "d2"}

	overlay.OpenDialog(d1)
	assert.True(t, overlay.HasDialogs())
	assert.True(t, overlay.ContainsDialog("d1"))
	assert.False(t, overlay.ContainsDialog("d2"))

	overlay.OpenDialog(d2)
	assert.True(t, overlay.ContainsDialog("d2"))
	assert.Equal(t, d2, overlay.DialogLast(), "last dialog should be d2")

	// Close d1 by ID
	overlay.CloseDialog("d1")
	assert.False(t, overlay.ContainsDialog("d1"))
	assert.True(t, overlay.ContainsDialog("d2"))

	// Close front dialog (d2)
	overlay.CloseFrontDialog()
	assert.False(t, overlay.HasDialogs())
}

func TestOverlay_BringToFront(t *testing.T) {
	st := styles.DefaultStyles()
	overlay := NewOverlay(&st)

	d1 := &stubDialog{id: "d1"}
	d2 := &stubDialog{id: "d2"}
	d3 := &stubDialog{id: "d3"}

	overlay.OpenDialog(d1)
	overlay.OpenDialog(d2)
	overlay.OpenDialog(d3)

	// d3 is currently the front dialog
	assert.Equal(t, d3, overlay.DialogLast())

	// Bring d1 to front
	overlay.BringToFront("d1")
	assert.Equal(t, d1, overlay.DialogLast(), "d1 should now be in front")

	// d2 and d3 should still be present
	assert.True(t, overlay.ContainsDialog("d2"))
	assert.True(t, overlay.ContainsDialog("d3"))
}

func TestOverlay_Dialog_ByID(t *testing.T) {
	st := styles.DefaultStyles()
	overlay := NewOverlay(&st)

	d1 := &stubDialog{id: "d1"}
	overlay.OpenDialog(d1)

	found := overlay.Dialog("d1")
	assert.Equal(t, d1, found)

	notFound := overlay.Dialog("nonexistent")
	assert.Nil(t, notFound)
}

func TestOverlay_CloseFrontDialog_Empty(t *testing.T) {
	st := styles.DefaultStyles()
	overlay := NewOverlay(&st)

	// Should not panic on empty overlay
	overlay.CloseFrontDialog()
	assert.False(t, overlay.HasDialogs())
}

func TestNewCommandItem(t *testing.T) {
	st := styles.DefaultStyles()
	action := ActionNewSession{}

	item := NewCommandItem(&st, "test_id", "Test Title", "ctrl+t", action)
	require.NotNil(t, item)

	assert.Equal(t, "test_id", item.ID())
	assert.Equal(t, "Test Title", item.Filter())
	assert.Equal(t, "ctrl+t", item.Shortcut())

	// Verify action is stored correctly
	a, ok := item.Action().(ActionNewSession)
	assert.True(t, ok, "action should be ActionNewSession")
	assert.Equal(t, ActionNewSession{}, a)
}

func TestNewCommandItem_NoShortcut(t *testing.T) {
	st := styles.DefaultStyles()
	item := NewCommandItem(&st, "no_shortcut", "No Shortcut", "", ActionToggleHelp{})

	assert.Equal(t, "", item.Shortcut())
	assert.Equal(t, "No Shortcut", item.Filter())
}

// stubDialog is a minimal Dialog implementation for testing the Overlay.
type stubDialog struct {
	id string
}

func (d *stubDialog) ID() string                                       { return d.id }
func (d *stubDialog) HandleMsg(msg tea.Msg) Action                     { return nil }
func (d *stubDialog) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor { return nil }
