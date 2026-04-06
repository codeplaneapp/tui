package views

import (
	"sort"

	"github.com/charmbracelet/crush/internal/smithers"
)

// ViewFactory constructs a View given a Smithers client.
type ViewFactory func(client *smithers.Client) View

// Registry maps route names to view factories, decoupling view construction
// from the root model.
type Registry struct {
	factories map[string]ViewFactory
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]ViewFactory)}
}

// Register associates name with the given factory.
func (r *Registry) Register(name string, f ViewFactory) {
	r.factories[name] = f
}

// Open constructs the named view using the given client.
// Returns (view, true) if found, (nil, false) if the name is not registered.
func (r *Registry) Open(name string, client *smithers.Client) (View, bool) {
	f, ok := r.factories[name]
	if !ok {
		return nil, false
	}
	return f(client), true
}

// Names returns all registered view names in alphabetical order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// DefaultRegistry returns a Registry pre-loaded with all built-in Smithers views.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register("agents", func(c *smithers.Client) View { return NewAgentsView(c) })
	r.Register("approvals", func(c *smithers.Client) View { return NewApprovalsView(c) })
	r.Register("changes", func(c *smithers.Client) View { return NewChangesView() })
	r.Register("issues", func(c *smithers.Client) View { return NewIssuesView(c) })
	r.Register("landings", func(c *smithers.Client) View { return NewLandingsView(c) })
	r.Register("runs", func(c *smithers.Client) View { return NewRunsView(c) })
	r.Register("sessions", func(c *smithers.Client) View { return NewSessionsView(c) })
	r.Register("sql", func(c *smithers.Client) View { return NewSQLBrowserView(c) })
	r.Register("tickets", func(c *smithers.Client) View { return NewTicketsView(c) })
	r.Register("triggers", func(c *smithers.Client) View { return NewTriggersView(c) })
	r.Register("workspaces", func(c *smithers.Client) View { return NewWorkspacesView(c) })
	r.Register("workflows", func(c *smithers.Client) View { return NewWorkflowsView(c) })
	r.Register("workflow-runs", func(c *smithers.Client) View { return NewWorkflowRunView(c) })
	return r
}
