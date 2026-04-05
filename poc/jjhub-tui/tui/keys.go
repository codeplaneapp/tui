package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Enter      key.Binding
	Quit       key.Binding
	Help       key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	Refresh    key.Binding
	Preview    key.Binding
	Escape     key.Binding
	GotoTop    key.Binding
	GotoBottom key.Binding
	PageDown   key.Binding
	PageUp     key.Binding
	Filter     key.Binding
	Search     key.Binding
	Open       key.Binding

	// Number shortcuts for tabs.
	Num1 key.Binding
	Num2 key.Binding
	Num3 key.Binding
	Num4 key.Binding
	Num5 key.Binding
	Num6 key.Binding
}

var defaultKeys = keyMap{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Left:       key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev tab")),
	Right:      key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view detail")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	ShiftTab:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev tab")),
	Refresh:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh all")),
	Preview:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "toggle preview")),
	Escape:     key.NewBinding(key.WithKeys("escape"), key.WithHelp("esc", "back/clear")),
	GotoTop:    key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "go to top")),
	GotoBottom: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "go to bottom")),
	PageDown:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("C-d", "page down")),
	PageUp:     key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("C-u", "page up")),
	Filter:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "cycle state filter")),
	Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),

	Num1: key.NewBinding(key.WithKeys("1")),
	Num2: key.NewBinding(key.WithKeys("2")),
	Num3: key.NewBinding(key.WithKeys("3")),
	Num4: key.NewBinding(key.WithKeys("4")),
	Num5: key.NewBinding(key.WithKeys("5")),
	Num6: key.NewBinding(key.WithKeys("6")),
}
