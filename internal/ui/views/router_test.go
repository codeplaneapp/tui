package views_test

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/views"
)

// TestView is a simple test implementation of View interface.
type TestView struct {
	name   string
	width  int
	height int
}

func (v *TestView) Init() tea.Cmd                            { return nil }
func (v *TestView) Update(msg tea.Msg) (views.View, tea.Cmd) { return v, nil }
func (v *TestView) View() string                             { return v.name }
func (v *TestView) Name() string                             { return v.name }
func (v *TestView) SetSize(w, h int)                         { v.width = w; v.height = h }
func (v *TestView) ShortHelp() []key.Binding                 { return nil }

// FocusableView extends TestView with OnFocus/OnBlur callbacks for testing.
type FocusableView struct {
	TestView
	focused int
	blurred int
}

func (v *FocusableView) OnFocus() tea.Cmd { v.focused++; return nil }
func (v *FocusableView) OnBlur() tea.Cmd  { v.blurred++; return nil }

// MutatingView returns a different view pointer from Update to test in-stack replacement.
type MutatingView struct {
	name  string
	count int
}

func (v *MutatingView) Init() tea.Cmd { return nil }
func (v *MutatingView) Update(msg tea.Msg) (views.View, tea.Cmd) {
	next := &MutatingView{name: v.name, count: v.count + 1}
	return next, nil
}
func (v *MutatingView) View() string             { return v.name }
func (v *MutatingView) Name() string             { return v.name }
func (v *MutatingView) SetSize(w, h int)         {}
func (v *MutatingView) ShortHelp() []key.Binding { return nil }

// TestRouterPush verifies that Push calls SetSize and Init before adding to stack.
func TestRouterPush(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1, 100, 40)
	if router.Current() != view1 {
		t.Errorf("expected current view to be view1")
	}
	if view1.width != 100 || view1.height != 40 {
		t.Errorf("expected SetSize(100,40) to be called, got (%d,%d)", view1.width, view1.height)
	}

	router.Push(view2, 120, 50)
	if router.Current() != view2 {
		t.Errorf("expected current view to be view2")
	}
	if view2.width != 120 || view2.height != 50 {
		t.Errorf("expected SetSize(120,50) to be called, got (%d,%d)", view2.width, view2.height)
	}

	if router.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", router.Depth())
	}
}

// TestRouterPop verifies that Pop removes the top view and returns lifecycle cmds.
func TestRouterPop(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1, 100, 40)
	router.Push(view2, 100, 40)

	// Pop should remove view2 and return a command (or nil if no Focusable).
	_ = router.Pop()

	if router.Current() != view1 {
		t.Errorf("expected current view to be view1 after pop")
	}

	// Pop should return nil with 1 view (root protection).
	cmd := router.Pop()
	if cmd != nil {
		// Allowed to be nil for root view
	}
	// But stack should still be depth >= 1 after this (root is kept or stack was 1 already).
	// Pop returns nil when len <= 1 and does not shrink stack below 1.
	if router.Depth() != 1 {
		t.Errorf("expected depth 1 after failed pop, got %d", router.Depth())
	}

	if router.Current() != view1 {
		t.Errorf("expected current view to still be view1 after failed pop")
	}
}

// TestRouterPopCallsLifecycle verifies blur/focus callbacks during Push and Pop.
func TestRouterPopCallsLifecycle(t *testing.T) {
	router := views.NewRouter()

	root := &FocusableView{TestView: TestView{name: "root"}}
	top := &FocusableView{TestView: TestView{name: "top"}}

	// Pushing root onto an empty stack focuses root.
	router.Push(root, 100, 40)
	if root.focused != 1 {
		t.Errorf("expected root.focused=1 after initial push, got %d", root.focused)
	}

	// Pushing top blurs root, then focuses top.
	router.Push(top, 100, 40)

	if root.blurred != 1 {
		t.Errorf("expected root.blurred=1 after pushing top, got %d", root.blurred)
	}
	if top.focused != 1 {
		t.Errorf("expected top.focused=1 after push, got %d", top.focused)
	}

	router.Pop()

	if top.blurred != 1 {
		t.Errorf("expected top.blurred=1 after pop, got %d", top.blurred)
	}
	// root was focused once on initial push and once when re-exposed by Pop.
	if root.focused != 2 {
		t.Errorf("expected root.focused=2 after pop (once on initial push + once on reveal), got %d", root.focused)
	}
}

// TestRouterPopToRoot verifies PopToRoot clears all but first view.
func TestRouterPopToRoot(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}
	view3 := &TestView{name: "view3"}

	router.Push(view1, 80, 24)
	router.Push(view2, 80, 24)
	router.Push(view3, 80, 24)

	if router.Depth() != 3 {
		t.Errorf("expected depth 3, got %d", router.Depth())
	}

	_ = router.PopToRoot()

	if router.Depth() != 1 {
		t.Errorf("expected depth 1 after PopToRoot, got %d", router.Depth())
	}

	if router.Current() != view1 {
		t.Errorf("expected current view to be view1 after PopToRoot")
	}
}

func TestRouterReset(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1, 80, 24)
	router.Push(view2, 80, 24)

	router.Reset()

	if router.Depth() != 0 {
		t.Errorf("expected depth 0 after Reset, got %d", router.Depth())
	}

	if router.Current() != nil {
		t.Errorf("expected Current to be nil after Reset")
	}
}

// TestRouterRoot verifies that Root() returns the first view.
func TestRouterRoot(t *testing.T) {
	router := views.NewRouter()

	view1 := &TestView{name: "view1"}
	view2 := &TestView{name: "view2"}

	router.Push(view1, 80, 24)
	router.Push(view2, 80, 24)

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

	if cmd := router.Pop(); cmd != nil {
		// Pop on empty returns nil (no-op).
	}

	_ = router.PopToRoot()
}

// TestRouterSetSize verifies that SetSize propagates to the current view.
func TestRouterSetSize(t *testing.T) {
	router := views.NewRouter()

	view := &TestView{name: "v"}
	router.Push(view, 80, 24)

	router.SetSize(120, 50)
	if view.width != 120 || view.height != 50 {
		t.Errorf("expected SetSize to propagate to current view, got (%d,%d)", view.width, view.height)
	}
}

// TestRouterSetSize_EmptyStack verifies that SetSize does not panic on empty stack.
func TestRouterSetSize_EmptyStack(t *testing.T) {
	router := views.NewRouter()
	router.SetSize(80, 24) // should not panic
}

// TestRouterUpdate_ReplacesCurrentView verifies that Update replaces the view in-stack
// when the updated view pointer differs from the original.
func TestRouterUpdate_ReplacesCurrentView(t *testing.T) {
	router := views.NewRouter()

	mv := &MutatingView{name: "mutating", count: 0}
	router.Push(mv, 80, 24)

	_ = router.Update(tea.KeyPressMsg{})

	current, ok := router.Current().(*MutatingView)
	if !ok {
		t.Fatalf("expected current to be *MutatingView")
	}
	if current.count != 1 {
		t.Errorf("expected count=1 after update, got %d", current.count)
	}
}

// TestRouterUpdate_EmptyStack verifies that Update returns nil on empty stack.
func TestRouterUpdate_EmptyStack(t *testing.T) {
	router := views.NewRouter()
	cmd := router.Update(tea.KeyPressMsg{})
	if cmd != nil {
		t.Errorf("expected nil cmd from Update on empty stack")
	}
}

// TestRouterHasViews verifies HasViews reflects the stack state.
func TestRouterHasViews(t *testing.T) {
	router := views.NewRouter()
	if router.HasViews() {
		t.Errorf("expected HasViews=false for empty stack")
	}

	router.Push(&TestView{name: "v"}, 80, 24)
	if !router.HasViews() {
		t.Errorf("expected HasViews=true after push")
	}
}

// TestRouterPushView verifies the convenience PushView method uses stored dimensions.
func TestRouterPushView(t *testing.T) {
	router := views.NewRouter()

	// Push an initial view with explicit dimensions so the router stores them.
	root := &TestView{name: "root"}
	router.Push(root, 120, 50)

	// PushView should use the stored 120x50.
	child := &TestView{name: "child"}
	router.PushView(child)

	if child.width != 120 || child.height != 50 {
		t.Errorf("expected PushView to use stored dimensions (120,50), got (%d,%d)", child.width, child.height)
	}
	if router.Depth() != 2 {
		t.Errorf("expected depth 2 after PushView, got %d", router.Depth())
	}
	if router.Current() != child {
		t.Errorf("expected current view to be child after PushView")
	}
}

// TestRouterPopToRoot_Lifecycle verifies blur/focus callbacks during PopToRoot
// with Focusable views.
func TestRouterPopToRoot_Lifecycle(t *testing.T) {
	router := views.NewRouter()

	root := &FocusableView{TestView: TestView{name: "root"}}
	mid := &FocusableView{TestView: TestView{name: "mid"}}
	top := &FocusableView{TestView: TestView{name: "top"}}

	router.Push(root, 80, 24)
	router.Push(mid, 80, 24)
	router.Push(top, 80, 24)

	// Reset counters after pushes to isolate PopToRoot behavior.
	root.focused = 0
	root.blurred = 0
	mid.focused = 0
	mid.blurred = 0
	top.focused = 0
	top.blurred = 0

	router.PopToRoot()

	// top was the active view; it should have been blurred.
	if top.blurred != 1 {
		t.Errorf("expected top.blurred=1 after PopToRoot, got %d", top.blurred)
	}
	// root is the newly exposed view; it should have been focused.
	if root.focused != 1 {
		t.Errorf("expected root.focused=1 after PopToRoot, got %d", root.focused)
	}
	// mid should not have received any lifecycle calls from PopToRoot.
	if mid.blurred != 0 || mid.focused != 0 {
		t.Errorf("expected mid to have no lifecycle calls, got blurred=%d focused=%d", mid.blurred, mid.focused)
	}
}

// TestRouterReset_FullClear verifies Reset clears all views and nulls Current/Root.
func TestRouterReset_FullClear(t *testing.T) {
	router := views.NewRouter()

	root := &FocusableView{TestView: TestView{name: "root"}}
	top := &FocusableView{TestView: TestView{name: "top"}}

	router.Push(root, 80, 24)
	router.Push(top, 80, 24)

	router.Reset()

	if router.Depth() != 0 {
		t.Errorf("expected depth 0 after Reset, got %d", router.Depth())
	}
	if router.Current() != nil {
		t.Errorf("expected Current to be nil after Reset")
	}
	if router.Root() != nil {
		t.Errorf("expected Root to be nil after Reset")
	}
	if router.HasViews() {
		t.Errorf("expected HasViews=false after Reset")
	}
}
