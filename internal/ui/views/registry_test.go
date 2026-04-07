package views_test

import (
	"slices"
	"testing"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/views"
)

func TestNewRegistry_IsEmpty(t *testing.T) {
	r := views.NewRegistry()
	if names := r.Names(); len(names) != 0 {
		t.Errorf("expected empty registry, got names: %v", names)
	}
}

func TestRegistry_RegisterAndOpen(t *testing.T) {
	r := views.NewRegistry()
	client := smithers.NewClient()

	r.Register("test", func(c *smithers.Client) views.View {
		return &TestView{name: "test"}
	})

	v, ok := r.Open("test", client)
	if !ok {
		t.Fatal("expected Open to succeed for registered view")
	}
	if v == nil {
		t.Fatal("expected non-nil view")
	}
	if v.Name() != "test" {
		t.Errorf("expected view name 'test', got %q", v.Name())
	}
}

func TestRegistry_Open_NotFound(t *testing.T) {
	r := views.NewRegistry()
	v, ok := r.Open("nonexistent", smithers.NewClient())
	if ok {
		t.Error("expected Open to return false for unknown view")
	}
	if v != nil {
		t.Error("expected nil view for unknown name")
	}
}

func TestRegistry_Names_Sorted(t *testing.T) {
	r := views.NewRegistry()
	r.Register("zebra", func(c *smithers.Client) views.View { return &TestView{name: "zebra"} })
	r.Register("alpha", func(c *smithers.Client) views.View { return &TestView{name: "alpha"} })
	r.Register("mango", func(c *smithers.Client) views.View { return &TestView{name: "mango"} })

	names := r.Names()
	if !slices.IsSorted(names) {
		t.Errorf("expected sorted names, got %v", names)
	}
}

func TestDefaultRegistry_ContainsExpectedViews(t *testing.T) {
	r := views.DefaultRegistry()
	client := smithers.NewClient()

	for _, name := range []string{"agents", "approvals", "changes", "chat", "issues", "landings", "status", "tickets"} {
		v, ok := r.Open(name, client)
		if !ok {
			t.Errorf("expected default registry to contain %q", name)
			continue
		}
		if v.Name() != name {
			t.Errorf("expected view name %q, got %q", name, v.Name())
		}
	}
}

func TestDefaultRegistry_Names_HasThreeViews(t *testing.T) {
	r := views.DefaultRegistry()
	names := r.Names()
	if len(names) < 8 {
		t.Errorf("expected at least 8 views in default registry, got %d: %v", len(names), names)
	}
}
