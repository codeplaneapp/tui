package views_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/views"
)

// TestView is a simple test implementation of View interface.
type TestView struct {
	name string
}

func (v *TestView) Init() tea.Cmd                                 { return nil }
func (v *TestView) Update(msg tea.Msg) (views.View, tea.Cmd)     { return v, nil }
func (v *TestView) View() string                                  { return v.name }
func (v *TestView) Name() string                                  { return v.name }
func (v *TestView) ShortHelp() []string                           { return []string{} }

// TestRouterPush verifies that views can be pushed onto the router stack.
func TestRouterPush(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1)
	if router.Current() != view1 {
		t.Errorf("expected current view to be view1")
	}

	router.Push(view2)
	if router.Current() != view2 {
		t.Errorf("expected current view to be view2")
	}

	if router.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", router.Depth())
	}
}

// TestRouterPop verifies that Pop() only removes top view when depth > 1.
func TestRouterPop(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1)
	router.Push(view2)

	// Pop should remove view2
	if ok := router.Pop(); !ok {
		t.Errorf("expected Pop to succeed with 2 views")
	}

	if router.Current() != view1 {
		t.Errorf("expected current view to be view1 after pop")
	}

	// Pop should fail with 1 view (protection against popping root)
	if ok := router.Pop(); ok {
		t.Errorf("expected Pop to fail with 1 view (root protection)")
	}

	if router.Current() != view1 {
		t.Errorf("expected current view to still be view1 after failed pop")
	}
}

// TestRouterPopToRoot verifies PopToRoot clears all but first view.
func TestRouterPopToRoot(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}
	view3 := &TestView{name: "view3"}

	router.Push(view1)
	router.Push(view2)
	router.Push(view3)

	if router.Depth() != 3 {
		t.Errorf("expected depth 3, got %d", router.Depth())
	}

	// PopToRoot should leave only view1
	if ok := router.PopToRoot(); !ok {
		t.Errorf("expected PopToRoot to succeed with 3 views")
	}

	if router.Depth() != 1 {
		t.Errorf("expected depth 1 after PopToRoot, got %d", router.Depth())
	}

	if router.Current() != view1 {
		t.Errorf("expected current view to be view1 after PopToRoot")
	}
}

// TestRouterRoot verifies that Root() returns the first view.
func TestRouterRoot(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1)
	router.Push(view2)

	if router.Root() != view1 {
		t.Errorf("expected Root to return view1")
	}

	if router.Current() != view2 {
		t.Errorf("expected Current to return view2")
	}
}

// TestRouterEmptyStack verifies behavior with empty stack.
func TestRouterEmptyStack(t *testing.T) {
	router := views.NewRouter()

	if router.HasViews() {
		t.Errorf("expected HasViews to be false for empty stack")
	}

	if router.Current() != nil {
		t.Errorf("expected Current to be nil for empty stack")
	}

	if router.Root() != nil {
		t.Errorf("expected Root to be nil for empty stack")
	}

	if ok := router.Pop(); ok {
		t.Errorf("expected Pop to fail on empty stack")
	}

	if ok := router.PopToRoot(); ok {
		t.Errorf("expected PopToRoot to fail on empty stack")
	}
}
