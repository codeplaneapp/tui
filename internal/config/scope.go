package config

// Scope determines which config file is targeted for read/write operations.
type Scope int

const (
	// ScopeGlobal targets the global data config (~/.local/share/smithers-tui/smithers-tui.json).
	ScopeGlobal Scope = iota
	// ScopeWorkspace targets the workspace config (.smithers-tui/smithers-tui.json).
	ScopeWorkspace
)
