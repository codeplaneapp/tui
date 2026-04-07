package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTabManager(t *testing.T) {
	tm := NewTabManager()
	require.Equal(t, 1, tm.Len())
	assert.Equal(t, "launcher", tm.Active().ID)
	assert.Equal(t, TabKindLauncher, tm.Active().Kind)
	assert.False(t, tm.Active().Closable)
}

func TestTabManagerAdd(t *testing.T) {
	tm := NewTabManager()
	idx := tm.Add(&WorkspaceTab{ID: "run:abc", Kind: TabKindRunInspect, Label: "Run abc", Closable: true})
	assert.Equal(t, 1, idx)
	assert.Equal(t, 2, tm.Len())
	assert.Equal(t, "Run abc", tm.Tabs()[1].Label)
}

func TestTabManagerAddDedup(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&WorkspaceTab{ID: "run:abc", Kind: TabKindRunInspect, Label: "Run abc", Closable: true})
	// Adding same ID again should not create a duplicate.
	idx := tm.Add(&WorkspaceTab{ID: "run:abc", Kind: TabKindRunInspect, Label: "Run abc v2", Closable: true})
	assert.Equal(t, 1, idx)
	assert.Equal(t, 2, tm.Len())
	// Should have activated the existing tab.
	assert.Equal(t, 1, tm.ActiveIndex())
}

func TestTabManagerClose(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&WorkspaceTab{ID: "run:1", Kind: TabKindRunInspect, Label: "Run 1", Closable: true})
	tm.Add(&WorkspaceTab{ID: "run:2", Kind: TabKindRunInspect, Label: "Run 2", Closable: true})
	assert.Equal(t, 3, tm.Len())

	// Close middle tab.
	tm.Activate(1)
	tm.Close(1)
	assert.Equal(t, 2, tm.Len())
	assert.Equal(t, "launcher", tm.Active().ID) // should fall back to previous
}

func TestTabManagerCloseCannotCloseLauncher(t *testing.T) {
	tm := NewTabManager()
	tm.Close(0)
	assert.Equal(t, 1, tm.Len())
	assert.Equal(t, "launcher", tm.Active().ID)
}

func TestTabManagerCloseActiveShifts(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&WorkspaceTab{ID: "a", Kind: TabKindView, Label: "A", Closable: true})
	tm.Add(&WorkspaceTab{ID: "b", Kind: TabKindView, Label: "B", Closable: true})
	tm.Add(&WorkspaceTab{ID: "c", Kind: TabKindView, Label: "C", Closable: true})
	tm.Activate(3) // Active = "c" at index 3
	tm.Close(1)    // Close "a" — indices shift, "c" moves from 3 to 2
	assert.Equal(t, 2, tm.ActiveIndex())
	assert.Equal(t, "c", tm.Active().ID)
}

func TestTabManagerActivate(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&WorkspaceTab{ID: "run:1", Kind: TabKindRunInspect, Label: "Run 1", Closable: true})
	tm.Activate(1)
	assert.Equal(t, "run:1", tm.Active().ID)
	// Out of range is a no-op.
	tm.Activate(99)
	assert.Equal(t, "run:1", tm.Active().ID)
}

func TestTabManagerFindByID(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&WorkspaceTab{ID: "run:abc", Kind: TabKindRunInspect, Label: "Run abc", Closable: true})
	idx, ok := tm.FindByID("run:abc")
	assert.True(t, ok)
	assert.Equal(t, 1, idx)

	_, ok = tm.FindByID("nonexistent")
	assert.False(t, ok)
}

func TestTabKindIcon(t *testing.T) {
	assert.Equal(t, "◆", TabKindLauncher.Icon())
	assert.Equal(t, "●", TabKindRunInspect.Icon())
	assert.Equal(t, "⬡", TabKindWorkspace.Icon())
}
