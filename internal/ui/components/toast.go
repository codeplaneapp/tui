// Package components provides reusable Bubble Tea v2 UI components for the
// Smithers TUI.
package components

import (
	"fmt"
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
)

// DefaultToastTTL is the default duration before a toast is automatically dismissed.
const DefaultToastTTL = 5 * time.Second

// MaxToastWidth is the maximum render width (in terminal columns) for a single toast.
const MaxToastWidth = 48

// MaxVisibleToasts is the maximum number of toasts rendered in the stack at once.
const MaxVisibleToasts = 3

// ToastLevel controls the visual severity of a toast notification.
type ToastLevel uint8

const (
	// ToastLevelInfo is a neutral informational message (default).
	ToastLevelInfo ToastLevel = iota
	// ToastLevelSuccess indicates a successful operation.
	ToastLevelSuccess
	// ToastLevelWarning indicates a warning that does not block the user.
	ToastLevelWarning
	// ToastLevelError indicates a failure or error condition.
	ToastLevelError
)

// ActionHint pairs a keyboard key label with a short action description shown
// at the bottom of a toast, e.g. {Key: "esc", Label: "dismiss"}.
type ActionHint struct {
	Key   string
	Label string
}

// ShowToastMsg requests the ToastManager to add and display a toast.
type ShowToastMsg struct {
	// Title is the bold heading displayed at the top of the toast.
	Title string
	// Body is the optional descriptive text below the title.
	Body string
	// ActionHints is an optional list of key/label pairs rendered at the bottom.
	ActionHints []ActionHint
	// Level controls the border colour and visual severity.
	Level ToastLevel
	// TTL overrides the auto-dismiss timeout. Zero uses [DefaultToastTTL].
	TTL time.Duration
}

// DismissToastMsg requests removal of the toast identified by id.
type DismissToastMsg struct {
	ID uint64
}

// toastTimedOutMsg is an internal message fired when a toast's TTL expires.
type toastTimedOutMsg struct {
	id uint64
}

// toast is the state for a single toast notification.
type toast struct {
	id          uint64
	title       string
	body        string
	actionHints []ActionHint
	level       ToastLevel
	ttl         time.Duration
}

// ToastManager manages a bounded stack of in-terminal toast notifications and
// handles their lifecycle (add, auto-dismiss, manual dismiss).
//
// Usage in a root model:
//
//	type Model struct {
//	    toasts *components.ToastManager
//	    ...
//	}
//
//	func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
//	    cmd := m.toasts.Update(msg)
//	    return m, cmd
//	}
//
//	func (m Model) Draw(scr uv.Screen, area uv.Rectangle) {
//	    m.toasts.Draw(scr, area)
//	}
type ToastManager struct {
	toasts  []toast
	nextID  uint64
	st      *styles.Styles
	now     func() time.Time // injectable for tests; nil uses time.Now
}

// NewToastManager creates a new [ToastManager].
func NewToastManager(st *styles.Styles) *ToastManager {
	return &ToastManager{st: st}
}

// Update handles [ShowToastMsg], [DismissToastMsg], and internal TTL messages.
// It returns a [tea.Cmd] that schedules auto-dismiss timers as needed.
func (m *ToastManager) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ShowToastMsg:
		return m.add(msg)
	case DismissToastMsg:
		m.remove(msg.ID)
	case toastTimedOutMsg:
		m.remove(msg.id)
	}
	return nil
}

// add appends a new toast and returns a command that will fire [toastTimedOutMsg]
// after the configured TTL.
func (m *ToastManager) add(msg ShowToastMsg) tea.Cmd {
	ttl := msg.TTL
	if ttl <= 0 {
		ttl = DefaultToastTTL
	}

	m.nextID++
	id := m.nextID

	t := toast{
		id:          id,
		title:       msg.Title,
		body:        msg.Body,
		actionHints: msg.ActionHints,
		level:       msg.Level,
		ttl:         ttl,
	}

	// Maintain bounded stack: evict oldest if at capacity.
	if len(m.toasts) >= MaxVisibleToasts {
		m.toasts = m.toasts[1:]
	}
	m.toasts = append(m.toasts, t)

	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return toastTimedOutMsg{id: id}
	})
}

// remove deletes the toast with the given id (no-op if not found).
func (m *ToastManager) remove(id uint64) {
	for i, t := range m.toasts {
		if t.id == id {
			m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
			return
		}
	}
}

// Clear removes all active toasts.
func (m *ToastManager) Clear() {
	m.toasts = m.toasts[:0]
}

// Len returns the number of currently active toasts.
func (m *ToastManager) Len() int {
	return len(m.toasts)
}

// FrontID returns the ID of the newest (bottom-most in visual stack) toast,
// or 0 if the stack is empty.
func (m *ToastManager) FrontID() uint64 {
	if len(m.toasts) == 0 {
		return 0
	}
	return m.toasts[len(m.toasts)-1].id
}

// Draw renders the toast stack at the bottom-right of area.
// Toasts are stacked upward (newest at bottom, oldest at top).
// It never returns a cursor — toasts are purely decorative overlays.
func (m *ToastManager) Draw(scr uv.Screen, area uv.Rectangle) {
	if len(m.toasts) == 0 {
		return
	}

	// Clamp render width to available space.
	maxW := min(MaxToastWidth, area.Dx()-2)
	if maxW <= 0 {
		return
	}

	// Render each toast into a string and collect heights, bottom-to-top.
	type rendered struct {
		view string
		h    int
	}
	items := make([]rendered, 0, len(m.toasts))
	for _, t := range m.toasts {
		v := m.renderToast(t, maxW)
		_, h := lipgloss.Size(v)
		items = append(items, rendered{view: v, h: h})
	}

	// Draw from the bottom of area upward. Newest toast is last in the slice
	// (bottom of visual stack).
	curY := area.Max.Y
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if curY-item.h < area.Min.Y {
			break // no more vertical room
		}
		curY -= item.h
		w, _ := lipgloss.Size(item.view)
		rowArea := image.Rect(area.Min.X, curY, area.Max.X, curY+item.h)
		rect := common.BottomRightRect(rowArea, w, item.h)
		uv.NewStyledString(item.view).Draw(scr, rect)
	}
}

// renderToast renders a single toast entry into an ANSI-styled string.
func (m *ToastManager) renderToast(t toast, maxW int) string {
	st := m.st

	// Pick container style based on level.
	var container lipgloss.Style
	switch t.level {
	case ToastLevelSuccess:
		container = st.Toast.ContainerSuccess
	case ToastLevelWarning:
		container = st.Toast.ContainerWarning
	case ToastLevelError:
		container = st.Toast.ContainerError
	default:
		container = st.Toast.ContainerInfo
	}

	// Inner width accounts for the container's horizontal frame (border + padding).
	innerW := maxW - container.GetHorizontalFrameSize()
	if innerW <= 0 {
		innerW = 1
	}

	var lines []string

	// Title
	if t.title != "" {
		title := st.Toast.Title.Width(innerW).Render(truncate(t.title, innerW))
		lines = append(lines, title)
	}

	// Body — wrap long lines naively at innerW.
	if t.body != "" {
		for _, line := range wordWrap(t.body, innerW) {
			lines = append(lines, st.Toast.Body.Width(innerW).Render(line))
		}
	}

	// Action hints — try to fit all on one line; fall back to one per line.
	if len(t.actionHints) > 0 {
		parts := make([]string, 0, len(t.actionHints))
		for _, hint := range t.actionHints {
			key := st.Toast.ActionHintKey.Render(fmt.Sprintf("[%s]", hint.Key))
			label := st.Toast.ActionHint.Render(hint.Label)
			parts = append(parts, key+" "+label)
		}
		if t.title != "" || t.body != "" {
			lines = append(lines, st.Toast.Body.Width(innerW).Render("")) // blank separator
		}
		combined := strings.Join(parts, "  ")
		// Check if combined fits on one line; if not, put each hint on its own line.
		if len([]rune(combined)) <= innerW {
			lines = append(lines, st.Toast.ActionHint.Width(innerW).Render(combined))
		} else {
			for _, part := range parts {
				lines = append(lines, st.Toast.ActionHint.Width(innerW).Render(part))
			}
		}
	}

	content := strings.Join(lines, "\n")
	return container.Width(maxW).Render(content)
}

// truncate cuts s to at most n visible characters (no ellipsis — the caller
// controls max width via lipgloss.Width).
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// wordWrap splits text into lines of at most width visible characters,
// breaking on spaces. It is intentionally simple: no hyphenation, no
// bidi/complex-script awareness.
func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		candidate := current + " " + w
		if len([]rune(candidate)) <= width {
			current = candidate
		} else {
			lines = append(lines, current)
			current = w
		}
	}
	lines = append(lines, current)
	return lines
}
