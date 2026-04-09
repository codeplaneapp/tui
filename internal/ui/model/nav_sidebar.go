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
const (
	navSidebarWidth        = 18
	navToggleWidth         = 4
	navSidebarTabsStartRow = 2
)

// activateTab switches to the workspace tab at idx. It swaps the viewRouter
// to the new tab's Router and sets the appropriate UI state.
func (m *UI) activateTab(idx int) tea.Cmd {
	return m.activateTabWithFocus(idx, uiFocusMain, false)
}

// activateTabWithFocus switches to the workspace tab at idx and optionally
// preserves a requested focus state.
func (m *UI) activateTabWithFocus(idx int, focus uiFocusState, keepFocus bool) tea.Cmd {
	if m.tabManager == nil {
		return nil
	}
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
	desiredFocus := uiFocusMain
	if keepFocus {
		desiredFocus = focus
	}
	switch tab.Kind {
	case TabKindLauncher:
		m.setState(uiSmithersDashboard, desiredFocus)
	case TabKindRunInspect, TabKindLiveChat, TabKindWorkspace, TabKindView:
		if tab.Router.HasViews() {
			m.setState(uiSmithersView, desiredFocus)
		} else {
			m.setState(uiSmithersDashboard, desiredFocus)
		}
	}

	focusCmd := m.setFocus(desiredFocus)
	return tea.Batch(initCmd, focusCmd)
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
		return tea.Batch(cmd, m.setFocus(uiFocusMain))
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
	keepFocus := m.focus == uiFocusNav
	m.tabManager.Close(m.tabManager.ActiveIndex())
	return m.activateTabWithFocus(m.tabManager.ActiveIndex(), uiFocusNav, keepFocus)
}

// toggleNavSidebar shows or hides the workspace tab sidebar.
func (m *UI) toggleNavSidebar() tea.Cmd {
	if !m.supportsNavSidebar() {
		return nil
	}
	m.navSidebarHidden = !m.navSidebarHidden
	var focusCmd tea.Cmd
	if m.navSidebarHidden && m.focus == uiFocusNav {
		focusCmd = m.setFocus(m.defaultFocusAfterNavHidden())
	}
	m.updateLayoutAndSize()
	return focusCmd
}

func (m *UI) supportsNavSidebar() bool {
	switch m.state {
	case uiLanding, uiChat, uiSmithersView, uiSmithersDashboard:
		return true
	default:
		return false
	}
}

func (m *UI) defaultFocusAfterNavHidden() uiFocusState {
	switch m.state {
	case uiLanding:
		return uiFocusEditor
	case uiChat, uiSmithersView, uiSmithersDashboard:
		return uiFocusMain
	default:
		return uiFocusNone
	}
}

func (m *UI) canFocusNavSidebar() bool {
	return m.supportsNavSidebar() && !m.navSidebarHidden && m.tabManager != nil && m.tabManager.Len() > 0
}

func (m *UI) setFocus(focus uiFocusState) tea.Cmd {
	m.focus = focus
	switch focus {
	case uiFocusEditor:
		if m.state == uiChat {
			m.chat.Blur()
		}
		return m.textarea.Focus()
	case uiFocusMain:
		m.textarea.Blur()
		if m.state == uiChat {
			m.chat.Focus()
			if m.chat.Len() > 0 {
				m.chat.SetSelected(m.chat.Len() - 1)
			}
		} else {
			m.chat.Blur()
		}
	case uiFocusNav:
		m.textarea.Blur()
		m.chat.Blur()
	default:
		m.textarea.Blur()
		m.chat.Blur()
	}
	return nil
}

func (m *UI) focusOrder() []uiFocusState {
	switch m.state {
	case uiLanding:
		order := []uiFocusState{uiFocusEditor}
		if m.canFocusNavSidebar() {
			order = append(order, uiFocusNav)
		}
		return order
	case uiChat:
		order := []uiFocusState{uiFocusEditor, uiFocusMain}
		if m.canFocusNavSidebar() {
			order = append(order, uiFocusNav)
		}
		return order
	case uiSmithersView, uiSmithersDashboard:
		order := []uiFocusState{uiFocusMain}
		if m.canFocusNavSidebar() {
			order = append(order, uiFocusNav)
		}
		return order
	default:
		return nil
	}
}

func (m *UI) cycleFocus(delta int) tea.Cmd {
	order := m.focusOrder()
	if len(order) <= 1 {
		return nil
	}

	current := 0
	for i, focus := range order {
		if focus == m.focus {
			current = i
			break
		}
	}

	next := (current + delta + len(order)) % len(order)
	return m.setFocus(order[next])
}

// renderNavSidebar draws the vertical navigation sidebar from the tab manager.
func (m *UI) renderNavSidebar(t *styles.Styles, height int, focused bool) string {
	if m.tabManager == nil {
		return ""
	}
	tabs := m.tabManager.Tabs()
	activeIdx := m.tabManager.ActiveIndex()

	var lines []string
	toggleStyle := lipgloss.NewStyle().Foreground(t.FgSubtle)
	if focused {
		toggleStyle = toggleStyle.Foreground(t.Primary).Bold(true)
	}
	lines = append(lines, toggleStyle.Render("[<] Hide"), "")

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

		itemStyle := lipgloss.NewStyle().Foreground(t.FgSubtle)
		if i == activeIdx {
			itemStyle = itemStyle.Foreground(t.Primary).Bold(true)
			if focused {
				itemStyle = itemStyle.Underline(true)
			}
		}
		lines = append(lines, num+" "+itemStyle.Render(icon+" "+label))
	}

	sidebar := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(navSidebarWidth).
		Height(height).
		Render(sidebar)
}

func renderNavToggle(t *styles.Styles, height int) string {
	return lipgloss.NewStyle().
		Width(navToggleWidth).
		Height(height).
		Render(lipgloss.NewStyle().Foreground(t.Primary).Bold(true).Render("[>]"))
}

// navigateToNavTab handles selecting a workspace tab by index (0-based).
func (m *UI) navigateToNavTab(idx int, keepFocus bool) tea.Cmd {
	if m.tabManager == nil || idx < 0 || idx >= m.tabManager.Len() {
		return nil
	}
	return m.activateTabWithFocus(idx, uiFocusNav, keepFocus)
}

func (m *UI) navigateRelativeNavTab(delta int, keepFocus bool) tea.Cmd {
	if m.tabManager == nil || m.tabManager.Len() == 0 {
		return nil
	}
	idx := (m.tabManager.ActiveIndex() + delta + m.tabManager.Len()) % m.tabManager.Len()
	return m.navigateToNavTab(idx, keepFocus)
}

func navSidebarTabIndexAt(row int, tabCount int) (int, bool) {
	row -= navSidebarTabsStartRow
	if row < 0 || row >= tabCount {
		return 0, false
	}
	return row, true
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
