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

// PopViewMsg signals the router to pop the current view.
type PopViewMsg struct{}

// Router manages a stack of views.
type Router struct {
	stack []View
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{}
}

// Push pushes a view onto the stack and calls Init.
func (r *Router) Push(v View) tea.Cmd {
	r.stack = append(r.stack, v)
	return v.Init()
}

// Pop removes the top view from the stack.
// Returns false if the stack is empty.
func (r *Router) Pop() bool {
	if len(r.stack) == 0 {
		return false
	}
	r.stack = r.stack[:len(r.stack)-1]
	return true
}

// Current returns the top view, or nil if the stack is empty.
func (r *Router) Current() View {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1]
}

// HasViews returns true if there are views on the stack.
func (r *Router) HasViews() bool {
	return len(r.stack) > 0
}
