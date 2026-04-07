package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/views"
)

// navSidebarWidth is the fixed width of the persistent vertical nav sidebar.
const navSidebarWidth = 18

// activateTab switches to the workspace tab at idx. It swaps the viewRouter
// to the new tab's Router and sets the appropriate UI state.
func (m *UI) activateTab(idx int) tea.Cmd {
	if m.tabManager == nil {
		return nil
	}
	prev := m.tabManager.ActiveIndex()
	m.tabManager.Activate(idx)
	tab := m.tabManager.Active()

	// Swap viewRouter to the active tab's router.
	m.viewRouter = tab.Router

	var initCmd tea.Cmd

	// Lazily initialize the tab's root view on first activation.
	if !tab.initialized {
		tab.initialized = true
		switch tab.Kind {
		case TabKindLauncher:
			// Launcher uses the dashboard — it's initialized separately.
		case TabKindRunInspect:
			if tab.RunID != "" {
				v := views.NewRunInspectView(m.smithersClient, tab.RunID)
				initCmd = tab.Router.PushView(v)
			}
		case TabKindWorkspace:
			// Workspace detail views can be initialized here when available.
		}
	}

	// Set the UI state based on tab kind.
	switch tab.Kind {
	case TabKindLauncher:
		m.setState(uiSmithersDashboard, uiFocusMain)
	case TabKindRunInspect, TabKindLiveChat, TabKindWorkspace, TabKindView:
		if tab.Router.HasViews() {
			m.setState(uiSmithersView, uiFocusMain)
		} else {
			m.setState(uiSmithersDashboard, uiFocusMain)
		}
	}

	_ = prev
	return initCmd
}

// openTabForView creates or activates a tab for a given view with dedup ID.
func (m *UI) openTabForView(id string, kind TabKind, label string, v views.View) tea.Cmd {
	if m.tabManager == nil {
		return nil
	}
	tab := &WorkspaceTab{
		ID:       id,
		Kind:     kind,
		Label:    label,
		Closable: true,
		Router:   views.NewRouter(),
	}
	if kind == TabKindRunInspect {
		tab.RunID = strings.TrimPrefix(id, "run:")
	}
	idx := m.tabManager.Add(tab)
	existingTab := m.tabManager.Tabs()[idx]
	if !existingTab.initialized && v != nil {
		existingTab.initialized = true
		cmd := existingTab.Router.PushView(v)
		m.viewRouter = existingTab.Router
		m.tabManager.Activate(idx)
		m.setState(uiSmithersView, uiFocusMain)
		return cmd
	}
	return m.activateTab(idx)
}

// openViewAsTab opens a named view (runs, approvals, etc.) as a new workspace tab.
func (m *UI) openViewAsTab(name string) tea.Cmd {
	if m.tabManager == nil {
		return nil
	}

	// Create the view using handleNavigateToView's logic, but in a new tab.
	var v views.View
	label := name
	switch name {
	case "runs":
		v = views.NewRunsView(m.smithersClient)
		label = "Runs"
	case "approvals":
		v = views.NewApprovalsView(m.com, m.smithersClient)
		label = "Approvals"
	default:
		if rv, ok := m.viewRegistry.Open(name, m.smithersClient); ok {
			v = rv
			label = name
		} else {
			return nil
		}
	}

	return m.openTabForView("view:"+name, TabKindView, label, v)
}

// closeActiveTab closes the current tab and activates the previous one.
func (m *UI) closeActiveTab() tea.Cmd {
	if m.tabManager == nil || m.tabManager.ActiveIndex() == 0 {
		return nil
	}
	m.tabManager.Close(m.tabManager.ActiveIndex())
	return m.activateTab(m.tabManager.ActiveIndex())
}

// toggleNavSidebar shows or hides the workspace tab sidebar.
func (m *UI) toggleNavSidebar() {
	m.navSidebarHidden = !m.navSidebarHidden
	m.updateLayoutAndSize()
}

// renderNavSidebar draws the vertical navigation sidebar from the tab manager.
func (m *UI) renderNavSidebar(t *styles.Styles, height int) string {
	if m.tabManager == nil {
		return ""
	}
	tabs := m.tabManager.Tabs()
	activeIdx := m.tabManager.ActiveIndex()

	var lines []string
	for i, tab := range tabs {
		// Number label (1-9, then blank).
		var num string
		if i < 9 {
			num = lipgloss.NewStyle().Foreground(t.FgSubtle).Render(fmt.Sprintf("%d", i+1))
		} else {
			num = lipgloss.NewStyle().Foreground(t.FgSubtle).Render(" ")
		}

		icon := tab.Kind.Icon()
		label := tab.Label
		// Truncate label to fit sidebar width (leaving room for number + icon + padding).
		maxLabel := navSidebarWidth - 6
		if len([]rune(label)) > maxLabel {
			label = string([]rune(label)[:maxLabel-1]) + "…"
		}

		if i == activeIdx {
			styled := num + " " + lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(icon+" "+label)
			lines = append(lines, styled)
		} else {
			styled := num + " " + lipgloss.NewStyle().Foreground(t.FgSubtle).Render(icon+" "+label)
			lines = append(lines, styled)
		}
	}

	sidebar := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(navSidebarWidth).
		Height(height).
		PaddingTop(1).
		Render(sidebar)
}

// navigateToNavTab handles selecting a workspace tab by index (0-based).
func (m *UI) navigateToNavTab(idx int) tea.Cmd {
	if m.tabManager == nil || idx < 0 || idx >= m.tabManager.Len() {
		return nil
	}
	return m.activateTab(idx)
}

// renderNavDivider draws the vertical divider line between nav sidebar and content.
func renderNavDivider(t *styles.Styles, height int) string {
	if height <= 0 {
		return ""
	}
	var b strings.Builder
	line := lipgloss.NewStyle().Foreground(t.FgSubtle).Faint(true).Render("│")
	for i := 0; i < height; i++ {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	return b.String()
}
