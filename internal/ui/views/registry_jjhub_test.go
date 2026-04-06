package views_test

import (
	"testing"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/views"
)

func TestDefaultRegistry_ContainsJJHubChangeRoutes(t *testing.T) {
	t.Parallel()

	r := views.DefaultRegistry()
	client := smithers.NewClient()

	for _, name := range []string{"changes", "status", "issues", "landings", "workspaces"} {
		v, ok := r.Open(name, client)
		if !ok {
			t.Fatalf("expected default registry to contain %q", name)
		}
		if v.Name() != name {
			t.Fatalf("expected view name %q, got %q", name, v.Name())
		}
	}
}
