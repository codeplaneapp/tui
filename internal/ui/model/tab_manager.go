package model

import (
	"github.com/charmbracelet/crush/internal/ui/views"
)

// TabKind identifies the content type of a workspace tab.
type TabKind uint8

const (
	TabKindLauncher   TabKind = iota // tab 0, unclosable dashboard/launcher
	TabKindChat                      // embedded chat home/composer
	TabKindRunInspect                // a specific run inspection
	TabKindLiveChat                  // live chat for a running agent
	TabKindWorkspace                 // jjhub workspace detail
	TabKindView                      // generic view from the registry
)

// TabKindIcon returns a short icon string for each tab kind.
func (k TabKind) Icon() string {
	switch k {
	case TabKindLauncher:
		return "◆"
	case TabKindChat:
		return "💬"
	case TabKindRunInspect:
		return "●"
	case TabKindLiveChat:
		return "💬"
	case TabKindWorkspace:
		return "⬡"
	case TabKindView:
		return "◇"
	default:
		return "·"
	}
}

// WorkspaceTab represents a single tab in the workspace sidebar.
type WorkspaceTab struct {
	ID       string // unique dedup key: "launcher", "run:<id>", "ws:<id>"
	Kind     TabKind
	Label    string // display label in sidebar
	Closable bool   // false only for the launcher

	// Per-tab navigation stack. Each tab owns its own Router so
	// push/pop is scoped to the tab.
	Router *views.Router

	// Optional associated resource IDs for dedup.
	RunID       string
	WorkspaceID string
	SessionID   string

	// initialized tracks whether the tab's root view has been Init'd.
	// Tabs are lazily initialized on first activation.
	initialized bool
}

// TabManager manages the ordered list of workspace tabs.
type TabManager struct {
	tabs      []*WorkspaceTab
	activeIdx int
}

// NewTabManager creates a manager with a single launcher tab.
func NewTabManager() *TabManager {
	launcher := &WorkspaceTab{
		ID:       "launcher",
		Kind:     TabKindLauncher,
		Label:    "Launcher",
		Closable: false,
		Router:   views.NewRouter(),
	}
	return &TabManager{
		tabs:      []*WorkspaceTab{launcher},
		activeIdx: 0,
	}
}

// Add appends a tab. If a tab with the same ID already exists, it activates
// that tab instead and returns its index. Returns the index of the tab.
func (tm *TabManager) Add(tab *WorkspaceTab) int {
	if idx, ok := tm.FindByID(tab.ID); ok {
		tm.activeIdx = idx
		return idx
	}
	if tab.Router == nil {
		tab.Router = views.NewRouter()
	}
	tm.tabs = append(tm.tabs, tab)
	return len(tm.tabs) - 1
}

// Close removes the tab at idx. No-op if idx == 0 (launcher) or out of range.
// If the closed tab was active, activates the previous tab.
func (tm *TabManager) Close(idx int) {
	if idx <= 0 || idx >= len(tm.tabs) {
		return
	}
	tm.tabs = append(tm.tabs[:idx], tm.tabs[idx+1:]...)
	if tm.activeIdx == idx {
		tm.activeIdx = idx - 1
		if tm.activeIdx < 0 {
			tm.activeIdx = 0
		}
	} else if tm.activeIdx > idx {
		tm.activeIdx--
	}
}

// Activate sets the active tab by index. No-op if out of range.
func (tm *TabManager) Activate(idx int) {
	if idx >= 0 && idx < len(tm.tabs) {
		tm.activeIdx = idx
	}
}

// Active returns the currently active tab.
func (tm *TabManager) Active() *WorkspaceTab {
	if tm.activeIdx < len(tm.tabs) {
		return tm.tabs[tm.activeIdx]
	}
	return tm.tabs[0]
}

// ActiveIndex returns the index of the active tab.
func (tm *TabManager) ActiveIndex() int {
	return tm.activeIdx
}

// Tabs returns the tab list.
func (tm *TabManager) Tabs() []*WorkspaceTab {
	return tm.tabs
}

// Len returns the number of tabs.
func (tm *TabManager) Len() int {
	return len(tm.tabs)
}

// FindByID returns the index and true if a tab with the given ID exists.
func (tm *TabManager) FindByID(id string) (int, bool) {
	for i, t := range tm.tabs {
		if t.ID == id {
			return i, true
		}
	}
	return -1, false
}
