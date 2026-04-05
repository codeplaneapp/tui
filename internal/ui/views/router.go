package views

import tea "charm.land/bubbletea/v2"

// View is the interface that all Smithers views implement.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	Name() string
	ShortHelp() []string
}

// PopViewMsg signals the router to pop the current view and return to chat.
type PopViewMsg struct{}

// Router manages a stack of views with chat as the guaranteed root.
// Chat is never popped from the stack; all non-root views push on top of it.
type Router struct {
	stack []View
}

// NewRouter creates a new Router with an empty stack.
// Chat must be pushed as the first view to establish the root.
func NewRouter() *Router {
	return &Router{
		stack: []View{},
	}
}

// Push pushes a view onto the stack and calls Init.
func (r *Router) Push(v View) tea.Cmd {
	r.stack = append(r.stack, v)
	return v.Init()
}

// Pop removes the top view from the stack, preserving the root view.
// If only the root view remains, Pop is a no-op and returns false.
// Returns true if a view was actually popped.
func (r *Router) Pop() bool {
	// Only allow popping if we have more than one view (root + at least one pushed view)
	// or if stack is empty (defensive check).
	if len(r.stack) <= 1 {
		return false
	}
	r.stack = r.stack[:len(r.stack)-1]
	return true
}

// PopToRoot removes all views except the root (first) view.
// Returns true if any views were popped, false if stack is empty or already at root.
func (r *Router) PopToRoot() bool {
	if len(r.stack) <= 1 {
		return false
	}
	r.stack = r.stack[:1]
	return true
}

// Current returns the top view, or nil if the stack is empty.
func (r *Router) Current() View {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1]
}

// Root returns the root (base) view, or nil if the stack is empty.
func (r *Router) Root() View {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[0]
}

// HasViews returns true if there are views on the stack.
func (r *Router) HasViews() bool {
	return len(r.stack) > 0
}

// Depth returns the number of views in the stack.
func (r *Router) Depth() int {
	return len(r.stack)
}
