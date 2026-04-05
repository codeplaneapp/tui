package tui

import (
	"charm.land/lipgloss/v2"
)

// Palette — dark theme inspired by gh-dash / Tailwind slate.
var (
	purple  = lipgloss.Color("#7C3AED")
	green   = lipgloss.Color("#10B981")
	red     = lipgloss.Color("#EF4444")
	yellow  = lipgloss.Color("#F59E0B")
	blue    = lipgloss.Color("#3B82F6")
	violet  = lipgloss.Color("#8B5CF6")
	slate50 = lipgloss.Color("#F8FAFC")
	slate300 = lipgloss.Color("#CBD5E1")
	slate400 = lipgloss.Color("#94A3B8")
	slate500 = lipgloss.Color("#64748B")
	slate600 = lipgloss.Color("#475569")
	slate700 = lipgloss.Color("#334155")
	slate800 = lipgloss.Color("#1E293B")
	slate900 = lipgloss.Color("#0F172A")
)

// ---- Header / Brand ----

var (
	logoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(purple)

	repoNameStyle = lipgloss.NewStyle().
			Foreground(slate300).
			Bold(true)

	headerBarStyle = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottomForeground(slate700).
			Padding(0, 1)
)

// ---- Tab bar ----

var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(purple).
			BorderBottom(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderBottomForeground(purple).
			Padding(0, 1)

	activeTabNumStyle = lipgloss.NewStyle().
				Foreground(purple).
				Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(slate500).
				Padding(0, 1)

	inactiveTabNumStyle = lipgloss.NewStyle().
				Foreground(slate600)

	tabCountStyle = lipgloss.NewStyle().
			Foreground(slate500)

	tabBarStyle = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottomForeground(slate700)
)

// ---- Table ----

var (
	cursorStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	selectedRowStyle = lipgloss.NewStyle().
				Background(slate800)

	normalRowStyle = lipgloss.NewStyle()

	altRowStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#141B2D"))

	headerColStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(slate500).
			Underline(true)

	scrollInfoStyle = lipgloss.NewStyle().
			Foreground(slate600).
			Italic(true)
)

// ---- Status badges ----

var (
	openStyle   = lipgloss.NewStyle().Foreground(green)
	closedStyle = lipgloss.NewStyle().Foreground(red)
	mergedStyle = lipgloss.NewStyle().Foreground(violet)
	draftStyle  = lipgloss.NewStyle().Foreground(slate500)

	runningStyle  = lipgloss.NewStyle().Foreground(green)
	stoppedStyle  = lipgloss.NewStyle().Foreground(slate500)
	pendingStyle  = lipgloss.NewStyle().Foreground(yellow)
	failedStyle   = lipgloss.NewStyle().Foreground(red)
)

// ---- Sidebar / preview ----

var (
	sidebarStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeftForeground(slate700).
			PaddingLeft(2).
			PaddingRight(1).
			PaddingTop(1)

	sidebarTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(slate50)

	sidebarSubtitleStyle = lipgloss.NewStyle().
				Foreground(slate400)

	sidebarLabelStyle = lipgloss.NewStyle().
				Foreground(slate500)

	sidebarValueStyle = lipgloss.NewStyle().
				Foreground(slate300)

	sidebarSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(slate400).
				MarginTop(1)

	sidebarBodyStyle = lipgloss.NewStyle().
				Foreground(slate400)

	sidebarTagStyle = lipgloss.NewStyle().
			Foreground(violet).
			Bold(true)
)

// ---- Footer ----

var (
	footerStyle = lipgloss.NewStyle().
			Foreground(slate500).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTopForeground(slate700).
			Padding(0, 1)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(slate400).
			Bold(true)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(slate600)

	footerFilterStyle = lipgloss.NewStyle().
				Foreground(yellow).
				Bold(true)

	footerSepStyle = lipgloss.NewStyle().
			Foreground(slate700)
)

// ---- Search bar ----

var (
	searchBarStyle = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottomForeground(purple).
			Padding(0, 1)

	searchPromptStyle = lipgloss.NewStyle().
				Foreground(purple).
				Bold(true)

	searchInputStyle = lipgloss.NewStyle().
				Foreground(slate300)
)

// ---- General ----

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(slate50)

	dimStyle = lipgloss.NewStyle().
			Foreground(slate600)

	errorStyle = lipgloss.NewStyle().
			Foreground(red)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(purple)

	emptyStyle = lipgloss.NewStyle().
			Foreground(slate500).
			Italic(true)

	// Help overlay
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(slate300).
			Bold(true).
			Width(18)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(slate500)

	helpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(purple).
				MarginTop(1)
)
