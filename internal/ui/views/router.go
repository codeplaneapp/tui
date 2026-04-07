package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// View is the interface that all Smithers views implement.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	Name() string
	// SetSize informs the view of the current terminal dimensions.
	// The router calls this before Init when pushing a new view, and whenever
	// the terminal is resized.
	SetSize(width, height int)
	// ShortHelp returns key bindings shown in the contextual help bar.
	ShortHelp() []key.Binding
}

// Focusable is implemented by views that need focus/blur lifecycle callbacks.
// The router checks for this interface at push/pop time so that views which do
// not need it are not forced to implement no-op stubs.
type Focusable interface {
	OnFocus() tea.Cmd
	OnBlur() tea.Cmd
}

// PopViewMsg signals the router to pop the current view and return to chat.
type PopViewMsg struct{}

// Router manages a stack of views with chat as the guaranteed root.
// Chat is never popped from the stack; all non-root views push on top of it.
type Router struct {
	stack  []View
	width  int
	height int
}

// NewRouter creates a new Router with an empty stack.
// Chat must be pushed as the first view to establish the root.
func NewRouter() *Router {
	return &Router{
		stack: []View{},
	}
}

// Push pushes a view onto the stack, sizes it, and calls Init.
// It also calls OnBlur on the outgoing view and OnFocus on the new view if
// they implement Focusable.
func (r *Router) Push(v View, width, height int) tea.Cmd {
	var cmds []tea.Cmd

	// Blur the current top-of-stack view before pushing.
	if len(r.stack) > 0 {
		if f, ok := r.stack[len(r.stack)-1].(Focusable); ok {
			if cmd := f.OnBlur(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	r.width = width
	r.height = height
	v.SetSize(width, height)
	r.stack = append(r.stack, v)
	cmds = append(cmds, v.Init())

	if f, ok := v.(Focusable); ok {
		if cmd := f.OnFocus(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return tea.Batch(cmds...)
}

// Pop removes the top view from the stack, preserving the root view.
// If only the root view remains, Pop is a no-op and returns nil.
// Returns a tea.Cmd that includes any blur/focus lifecycle commands.
func (r *Router) Pop() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil
	}

	var cmds []tea.Cmd

	// Blur the outgoing view.
	outgoing := r.stack[len(r.stack)-1]
	if f, ok := outgoing.(Focusable); ok {
		if cmd := f.OnBlur(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	r.stack = r.stack[:len(r.stack)-1]

	// Focus the newly-exposed view.
	if len(r.stack) > 0 {
		if f, ok := r.stack[len(r.stack)-1].(Focusable); ok {
			if cmd := f.OnFocus(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// PopToRoot removes all views except the root (first) view.
// Returns a tea.Cmd from blur/focus lifecycle callbacks, or nil if already at
// root or stack is empty.
func (r *Router) PopToRoot() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil
	}

	var cmds []tea.Cmd

	// Blur the current top view.
	if f, ok := r.stack[len(r.stack)-1].(Focusable); ok {
		if cmd := f.OnBlur(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	r.stack = r.stack[:1]

	// Focus the root view.
	if f, ok := r.stack[0].(Focusable); ok {
		if cmd := f.OnFocus(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Reset clears the entire view stack.
func (r *Router) Reset() {
	r.stack = nil
}

// Update forwards msg to the current view and replaces it in the stack if it
// changed. This eliminates the awkward Pop()+Push(updated) pattern.
func (r *Router) Update(msg tea.Msg) tea.Cmd {
	current := r.Current()
	if current == nil {
		return nil
	}
	updated, cmd := current.Update(msg)
	if updated != current {
		r.stack[len(r.stack)-1] = updated
	}
	return cmd
}

// PushView pushes a view using the router's stored terminal dimensions.
// This is a convenience wrapper around Push for callers that don't track
// dimensions themselves.
func (r *Router) PushView(v View) tea.Cmd {
	return r.Push(v, r.width, r.height)
}

// SetSize propagates terminal dimensions to the currently-active view and
// stores them for use when new views are pushed.
func (r *Router) SetSize(width, height int) {
	r.width = width
	r.height = height
	if current := r.Current(); current != nil {
		current.SetSize(width, height)
	}
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
