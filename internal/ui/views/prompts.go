package views

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/handoff"
)

// Compile-time interface check.
var _ View = (*PromptsView)(nil)

// promptsFocus represents the focus state of the prompts view.
type promptsFocus int

const (
	focusList    promptsFocus = iota
	focusEditor  promptsFocus = iota
	focusPreview promptsFocus = iota
)

// promptSaveMsg carries the result of a prompt save operation.
type promptSaveMsg struct {
	err error // nil on success
}

// promptPropsDiscoveredMsg carries the result of a DiscoverPromptProps call.
type promptPropsDiscoveredMsg struct {
	promptID string
	props    []smithers.PromptProp
	err      error
}

// promptPreviewMsg carries the result of a PreviewPrompt call.
type promptPreviewMsg struct {
	promptID string
	rendered string
	err      error
}

// promptEditorTag is the handoff tag used for Ctrl+O external editor sessions.
const promptEditorTag = "prompt-edit"

type promptsLoadedMsg struct {
	prompts []smithers.Prompt
}

type promptsErrorMsg struct {
	err error
}

type promptSourceLoadedMsg struct {
	prompt *smithers.Prompt
}

type promptSourceErrorMsg struct {
	id  string
	err error
}

// PromptsView displays a split-pane list of Smithers MDX prompt templates.
// Left pane: navigable list of prompts. Right pane: raw MDX source + discovered inputs.
type PromptsView struct {
	client        *smithers.Client
	prompts       []smithers.Prompt
	loadedSources map[string]*smithers.Prompt
	cursor        int
	width         int
	height        int
	loading       bool  // list is loading
	loadingSource bool  // source is loading for the selected prompt
	err           error // list load error
	sourceErr     error // source load error for the selected prompt

	// Edit mode fields.
	focus   promptsFocus
	editor  textarea.Model
	dirty   bool
	tmpPath string // temp file path for Ctrl+O external editor handoff

	// Props discovery: most-recently discovered props (may differ from cached source).
	discoveredProps  []smithers.PromptProp
	discoveringProps bool
	discoverPropsErr error

	// Live preview: rendered output from PreviewPrompt.
	// propValues holds the user-supplied values for each discovered prop.
	previewText    string
	previewErr     error
	previewLoading bool
	propValues     map[string]string // propName → value
}

// NewPromptsView creates a new PromptsView.
func NewPromptsView(client *smithers.Client) *PromptsView {
	ed := textarea.New()
	ed.ShowLineNumbers = false
	ed.CharLimit = 0 // no limit
	ed.Prompt = ""
	ed.SetHeight(10)

	return &PromptsView{
		client:        client,
		loading:       true,
		loadedSources: make(map[string]*smithers.Prompt),
		focus:         focusList,
		editor:        ed,
		propValues:    make(map[string]string),
	}
}

// Init loads the prompt list from the client.
func (v *PromptsView) Init() tea.Cmd {
	return func() tea.Msg {
		prompts, err := v.client.ListPrompts(context.Background())
		if err != nil {
			return promptsErrorMsg{err: err}
		}
		return promptsLoadedMsg{prompts: prompts}
	}
}

// Update handles messages for the prompts view.
func (v *PromptsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case promptsLoadedMsg:
		v.prompts = msg.prompts
		v.loading = false
		// Immediately kick off lazy source load for the first prompt.
		return v, v.loadSelectedSource()

	case promptsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case promptSourceLoadedMsg:
		v.loadedSources[msg.prompt.ID] = msg.prompt
		v.loadingSource = false
		v.sourceErr = nil
		return v, nil

	case promptSourceErrorMsg:
		v.sourceErr = msg.err
		v.loadingSource = false
		return v, nil

	case promptPropsDiscoveredMsg:
		v.discoveringProps = false
		if msg.err != nil {
			v.discoverPropsErr = msg.err
		} else {
			v.discoverPropsErr = nil
			v.discoveredProps = msg.props
			// Ensure propValues has an entry for every discovered prop.
			for _, p := range msg.props {
				if _, exists := v.propValues[p.Name]; !exists {
					v.propValues[p.Name] = ""
				}
			}
		}
		return v, nil

	case promptPreviewMsg:
		v.previewLoading = false
		if msg.err != nil {
			v.previewErr = msg.err
			v.previewText = ""
		} else {
			v.previewErr = nil
			v.previewText = msg.rendered
		}
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.resizeEditor()
		return v, nil

	case promptSaveMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return components.ShowToastMsg{
					Title: "Save failed",
					Body:  msg.err.Error(),
					Level: components.ToastLevelError,
				}
			}
		}
		// On success: clear dirty flag and update the source cache so Esc restores to saved state.
		v.dirty = false
		if v.cursor >= 0 && v.cursor < len(v.prompts) {
			id := v.prompts[v.cursor].ID
			if cached, ok := v.loadedSources[id]; ok {
				saved := v.editor.Value()
				updated := *cached
				updated.Source = saved
				v.loadedSources[id] = &updated
			}
		}
		return v, func() tea.Msg {
			return components.ShowToastMsg{
				Title: "Saved",
				Level: components.ToastLevelSuccess,
			}
		}

	case handoff.HandoffMsg:
		if msg.Tag != promptEditorTag {
			return v, nil
		}
		tmpPath := v.tmpPath
		v.tmpPath = ""
		if msg.Result.Err != nil {
			_ = os.Remove(tmpPath)
			return v, func() tea.Msg {
				return components.ShowToastMsg{
					Title: "Editor failed",
					Body:  msg.Result.Err.Error(),
					Level: components.ToastLevelError,
				}
			}
		}
		newContentBytes, err := os.ReadFile(tmpPath)
		_ = os.Remove(tmpPath)
		if err != nil {
			return v, func() tea.Msg {
				return components.ShowToastMsg{
					Title: "Editor failed",
					Body:  fmt.Sprintf("read temp file: %v", err),
					Level: components.ToastLevelError,
				}
			}
		}
		newContent := string(newContentBytes)
		// Update textarea and dirty flag.
		v.editor.SetValue(newContent)
		if v.cursor >= 0 && v.cursor < len(v.prompts) {
			id := v.prompts[v.cursor].ID
			if loaded, ok := v.loadedSources[id]; ok {
				v.dirty = newContent != loaded.Source
			}
		}
		return v, nil

	case tea.KeyPressMsg:
		if v.focus == focusEditor {
			return v.updateEditor(msg)
		}
		if v.focus == focusPreview {
			return v.updatePreview(msg)
		}
		return v.updateList(msg)
	}
	return v, nil
}

// updateList handles key events when focus is on the list pane.
func (v *PromptsView) updateList(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
		return v, func() tea.Msg { return PopViewMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if v.cursor > 0 {
			v.cursor--
			return v, v.loadSelectedSource()
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if v.cursor < len(v.prompts)-1 {
			v.cursor++
			return v, v.loadSelectedSource()
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		v.loading = true
		v.loadedSources = make(map[string]*smithers.Prompt)
		v.sourceErr = nil
		return v, v.Init()

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		v.enterEditMode()

	case key.Matches(msg, key.NewBinding(key.WithKeys("p"))):
		// Discover / refresh props for the selected prompt.
		return v, v.discoverPropsCmd()

	case key.Matches(msg, key.NewBinding(key.WithKeys("v"))):
		// Toggle live preview panel for the selected prompt.
		if v.focus == focusPreview {
			v.focus = focusList
		} else {
			v.focus = focusPreview
			return v, v.previewCmd()
		}
	}
	return v, nil
}

// updateEditor handles key events when focus is on the editor pane.
func (v *PromptsView) updateEditor(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		v.exitEditMode()
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+s"))):
		return v, v.savePromptCmd()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+o"))):
		return v, v.openExternalEditorCmd()

	default:
		// Forward all other keys to the textarea.
		var cmd tea.Cmd
		v.editor, cmd = v.editor.Update(msg)

		// Dirty tracking: compare current editor value against cached source.
		if v.cursor >= 0 && v.cursor < len(v.prompts) {
			id := v.prompts[v.cursor].ID
			if loaded, ok := v.loadedSources[id]; ok {
				v.dirty = v.editor.Value() != loaded.Source
			}
		}
		return v, cmd
	}
}

// enterEditMode switches to editor focus for the selected prompt.
// Guards against empty loadedSources and narrow terminals.
func (v *PromptsView) enterEditMode() {
	if v.width < 80 {
		return // guard: narrow terminal
	}
	if v.cursor < 0 || v.cursor >= len(v.prompts) {
		return // guard: no selection
	}
	id := v.prompts[v.cursor].ID
	loaded, ok := v.loadedSources[id]
	if !ok {
		return // guard: source not yet loaded
	}

	v.editor.SetValue(loaded.Source)
	v.editor.MoveToBegin()
	v.resizeEditor()
	_ = v.editor.Focus()
	v.focus = focusEditor
	v.dirty = false
}

// exitEditMode returns to list focus, discarding any unsaved edits.
func (v *PromptsView) exitEditMode() {
	v.editor.Blur()

	// Restore textarea to the cached (unmodified) source.
	if v.cursor >= 0 && v.cursor < len(v.prompts) {
		id := v.prompts[v.cursor].ID
		if loaded, ok := v.loadedSources[id]; ok {
			v.editor.SetValue(loaded.Source)
		}
	}

	v.dirty = false
	v.focus = focusList
}

// resizeEditor applies the current terminal dimensions to the textarea.
func (v *PromptsView) resizeEditor() {
	listWidth := 30
	dividerWidth := 3
	detailWidth := v.width - listWidth - dividerWidth
	if detailWidth < 20 {
		detailWidth = 20
	}

	// Reserve space for: header (1) + entry file label (1) + blank line (1) +
	// source header (1) + inputs section + bottom padding (1).
	reservedForProps := 0
	if v.cursor >= 0 && v.cursor < len(v.prompts) {
		id := v.prompts[v.cursor].ID
		if loaded, ok := v.loadedSources[id]; ok && len(loaded.Props) > 0 {
			reservedForProps = 3 + len(loaded.Props)
		}
	}
	editorHeight := v.height - 5 - reservedForProps
	if editorHeight < 3 {
		editorHeight = 3
	}

	v.editor.SetWidth(detailWidth)
	v.editor.MaxHeight = editorHeight
	v.editor.SetHeight(editorHeight)
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *PromptsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.resizeEditor()
}

// loadSelectedSource issues a GetPrompt command for the currently selected
// prompt if its source has not yet been cached. It is a no-op when the source
// is already in loadedSources.
func (v *PromptsView) loadSelectedSource() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.prompts) {
		return nil
	}
	id := v.prompts[v.cursor].ID
	if _, ok := v.loadedSources[id]; ok {
		return nil // already cached
	}
	v.loadingSource = true
	return func() tea.Msg {
		p, err := v.client.GetPrompt(context.Background(), id)
		if err != nil {
			return promptSourceErrorMsg{id: id, err: err}
		}
		return promptSourceLoadedMsg{prompt: p}
	}
}

// View renders the prompts view.
func (v *PromptsView) View() string {
	var b strings.Builder

	// Header
	b.WriteString(ViewHeader(packageCom.Styles, "SMITHERS", "Prompts", v.width, "[Esc] Back"))
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading prompts...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.prompts) == 0 {
		b.WriteString("  No prompts found.\n")
		return b.String()
	}

	// Split-pane layout: list on left, detail on right.
	listWidth := 30
	dividerWidth := 3
	detailWidth := v.width - listWidth - dividerWidth
	if v.width < 80 || detailWidth < 20 {
		// Compact fallback for narrow terminals.
		b.WriteString(v.renderListCompact())
		return b.String()
	}

	listContent := v.renderList(listWidth)
	detailContent := v.renderDetail(detailWidth)

	divider := lipgloss.NewStyle().Faint(true).Render(" \u2502 ")

	// Join list and detail side by side, line by line.
	listLines := strings.Split(listContent, "\n")
	detailLines := strings.Split(detailContent, "\n")

	maxLines := len(listLines)
	if len(detailLines) > maxLines {
		maxLines = len(detailLines)
	}

	// Cap to available height (leave 3 lines for header + padding).
	availHeight := v.height - 3
	if availHeight > 0 && maxLines > availHeight {
		maxLines = availHeight
	}

	for i := 0; i < maxLines; i++ {
		left := ""
		if i < len(listLines) {
			left = listLines[i]
		}
		right := ""
		if i < len(detailLines) {
			right = detailLines[i]
		}
		left = padRight(left, listWidth)
		b.WriteString(left + divider + right + "\n")
	}

	return b.String()
}

// renderList renders the prompt list pane.
func (v *PromptsView) renderList(width int) string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)

	for i, prompt := range v.prompts {
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "\u25b8 "
			nameStyle = nameStyle.Bold(true)
		}

		// Truncate ID if it would overflow the pane.
		id := prompt.ID
		if len(id) > width-4 {
			id = id[:width-7] + "..."
		}
		b.WriteString(cursor + nameStyle.Render(id) + "\n")

		// Show input count below the ID (use cached source if available).
		if loaded, ok := v.loadedSources[prompt.ID]; ok && len(loaded.Props) > 0 {
			var names []string
			for _, p := range loaded.Props {
				names = append(names, p.Name)
			}
			count := fmt.Sprintf("%d input", len(loaded.Props))
			if len(loaded.Props) != 1 {
				count += "s"
			}
			detail := count + ": " + strings.Join(names, ", ")
			if len(detail) > width-2 {
				detail = detail[:width-5] + "..."
			}
			b.WriteString("  " + faint.Render(detail) + "\n")
		}

		if i < len(v.prompts)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderDetail renders the MDX source and inputs pane for the selected prompt.
// When in editor focus, it delegates to renderDetailEditor.
func (v *PromptsView) renderDetail(width int) string {
	if v.cursor < 0 || v.cursor >= len(v.prompts) {
		return ""
	}

	var b strings.Builder
	labelStyle := lipgloss.NewStyle().Faint(true)

	if v.loadingSource {
		b.WriteString(labelStyle.Render("Loading source..."))
		return b.String()
	}

	if v.sourceErr != nil {
		b.WriteString(fmt.Sprintf("Error loading source: %v", v.sourceErr))
		return b.String()
	}

	selected := v.prompts[v.cursor]
	loaded, ok := v.loadedSources[selected.ID]
	if !ok {
		b.WriteString(labelStyle.Render("Select a prompt to view source"))
		return b.String()
	}

	if v.focus == focusEditor {
		return v.renderDetailEditor(width, loaded)
	}

	if v.focus == focusPreview {
		return v.renderDetailPreview(width, loaded)
	}

	return v.renderDetailStatic(width, loaded)
}

// renderDetailStatic renders the read-only source pane (list focus mode).
func (v *PromptsView) renderDetailStatic(width int, loaded *smithers.Prompt) string {
	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)

	// Source section header.
	b.WriteString(titleStyle.Render("Source") + "\n")
	b.WriteString(labelStyle.Render(loaded.EntryFile) + "\n\n")

	// Compute how many lines are available for source (reserve space for props section).
	reservedForProps := 0
	if len(loaded.Props) > 0 {
		reservedForProps = 3 + len(loaded.Props)
	}
	maxSourceLines := v.height - 5 - reservedForProps
	if maxSourceLines < 5 {
		maxSourceLines = 5
	}

	// Render source lines with simple hard-wrap at pane width.
	sourceLines := strings.Split(loaded.Source, "\n")
	printed := 0
	truncated := false
	for _, line := range sourceLines {
		if printed >= maxSourceLines {
			truncated = true
			break
		}
		// Hard-wrap long lines.
		for len(line) > width {
			b.WriteString(line[:width] + "\n")
			line = line[width:]
			printed++
			if printed >= maxSourceLines {
				truncated = true
				break
			}
		}
		if truncated {
			break
		}
		b.WriteString(line + "\n")
		printed++
	}
	if truncated {
		b.WriteString(labelStyle.Render("... (truncated)") + "\n")
	}

	// Inputs section: prefer dynamically discovered props, fall back to cached.
	displayProps := loaded.Props
	if len(v.discoveredProps) > 0 && v.cursor >= 0 && v.cursor < len(v.prompts) &&
		v.prompts[v.cursor].ID == loaded.ID {
		displayProps = v.discoveredProps
	}
	if len(displayProps) > 0 {
		headerLabel := "Inputs"
		if len(v.discoveredProps) > 0 {
			headerLabel = "Inputs (discovered)"
		}
		b.WriteString("\n" + titleStyle.Render(headerLabel) + "\n")
		for _, prop := range displayProps {
			var defaultStr string
			if prop.DefaultValue != nil {
				defaultStr = fmt.Sprintf(" (default: %q)", *prop.DefaultValue)
			}
			b.WriteString(labelStyle.Render("  \u2022 ") + prop.Name +
				labelStyle.Render(" : "+prop.Type+defaultStr) + "\n")
		}
	}
	if v.discoveringProps {
		b.WriteString("\n" + labelStyle.Render("  Discovering props...") + "\n")
	}

	return b.String()
}

// renderDetailEditor renders the editable source pane (editor focus mode).
func (v *PromptsView) renderDetailEditor(width int, loaded *smithers.Prompt) string {
	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)
	modifiedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // amber

	// Source section header.
	b.WriteString(titleStyle.Render("Source") + "\n")

	// Entry file label with [modified] indicator when dirty.
	entryLabel := labelStyle.Render(loaded.EntryFile)
	if v.dirty {
		entryLabel += " " + modifiedStyle.Render("[modified]")
	}
	b.WriteString(entryLabel + "\n\n")

	// Textarea.
	b.WriteString(v.editor.View() + "\n")

	// Inputs section below the editor.
	if len(loaded.Props) > 0 {
		b.WriteString("\n" + titleStyle.Render("Inputs") + "\n")
		for _, prop := range loaded.Props {
			var defaultStr string
			if prop.DefaultValue != nil {
				defaultStr = fmt.Sprintf(" (default: %q)", *prop.DefaultValue)
			}
			b.WriteString(labelStyle.Render("  \u2022 ") + prop.Name +
				labelStyle.Render(" : "+prop.Type+defaultStr) + "\n")
		}
	}

	return b.String()
}

// renderDetailPreview renders the live preview pane for the selected prompt.
// It shows rendered output from PreviewPrompt alongside discovered props.
func (v *PromptsView) renderDetailPreview(width int, loaded *smithers.Prompt) string {
	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)

	b.WriteString(titleStyle.Render("Preview") + "\n")
	b.WriteString(labelStyle.Render(loaded.EntryFile) + "\n\n")

	// Props discovery overlay.
	if v.discoveringProps {
		b.WriteString(labelStyle.Render("Discovering props...") + "\n")
		return b.String()
	}
	if v.discoverPropsErr != nil {
		b.WriteString(fmt.Sprintf("Props error: %v\n", v.discoverPropsErr))
	}

	// Show discovered props with their current values.
	props := v.discoveredProps
	if len(props) == 0 && loaded != nil {
		props = loaded.Props // fall back to cached source props
	}
	if len(props) > 0 {
		b.WriteString(titleStyle.Render("Props") + "\n")
		for _, p := range props {
			val := v.propValues[p.Name]
			if val == "" {
				val = labelStyle.Render("(empty)")
			}
			b.WriteString(labelStyle.Render("  • ") + p.Name + labelStyle.Render(" = ") + val + "\n")
		}
		b.WriteString("\n")
	}

	// Rendered preview.
	b.WriteString(titleStyle.Render("Rendered") + "\n")
	if v.previewLoading {
		b.WriteString(labelStyle.Render("Loading preview...") + "\n")
		return b.String()
	}
	if v.previewErr != nil {
		b.WriteString(fmt.Sprintf("Preview error: %v\n", v.previewErr))
		return b.String()
	}
	if v.previewText == "" {
		b.WriteString(labelStyle.Render("(no preview — press r to refresh)") + "\n")
		return b.String()
	}

	// Render preview lines with simple hard-wrap.
	maxLines := v.height - 10
	if maxLines < 5 {
		maxLines = 5
	}
	printed := 0
	for _, line := range strings.Split(v.previewText, "\n") {
		if printed >= maxLines {
			b.WriteString(labelStyle.Render("... (truncated)") + "\n")
			break
		}
		for len(line) > width {
			b.WriteString(line[:width] + "\n")
			line = line[width:]
			printed++
			if printed >= maxLines {
				break
			}
		}
		b.WriteString(line + "\n")
		printed++
	}

	return b.String()
}

// renderListCompact renders a compact single-column layout for narrow terminals.
func (v *PromptsView) renderListCompact() string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)

	for i, prompt := range v.prompts {
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "\u25b8 "
			nameStyle = nameStyle.Bold(true)
		}
		b.WriteString(cursor + nameStyle.Render(prompt.ID) + "\n")

		// For the selected item, show a brief inline summary.
		if i == v.cursor {
			if v.loadingSource {
				b.WriteString(faint.Render("    Loading...") + "\n")
			} else if loaded, ok := v.loadedSources[prompt.ID]; ok {
				if len(loaded.Props) > 0 {
					var names []string
					for _, p := range loaded.Props {
						names = append(names, p.Name)
					}
					b.WriteString(faint.Render("    Inputs: "+strings.Join(names, ", ")) + "\n")
				}
				// Show the first few non-empty source lines.
				shown := 0
				for _, line := range strings.Split(loaded.Source, "\n") {
					if shown >= 3 {
						b.WriteString(faint.Render("    ...") + "\n")
						break
					}
					if strings.TrimSpace(line) != "" {
						b.WriteString(faint.Render("    "+line) + "\n")
						shown++
					}
				}
			}
		}

		if i < len(v.prompts)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// updatePreview handles key events when focus is on the preview pane.
func (v *PromptsView) updatePreview(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "v"))):
		v.focus = focusList
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		// Refresh the preview.
		return v, v.previewCmd()
	}
	return v, nil
}

// discoverPropsCmd issues a DiscoverPromptProps call for the selected prompt.
func (v *PromptsView) discoverPropsCmd() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.prompts) {
		return nil
	}
	id := v.prompts[v.cursor].ID
	client := v.client
	v.discoveringProps = true
	v.discoverPropsErr = nil
	return func() tea.Msg {
		props, err := client.DiscoverPromptProps(context.Background(), id)
		return promptPropsDiscoveredMsg{promptID: id, props: props, err: err}
	}
}

// previewCmd issues a PreviewPrompt call for the selected prompt using propValues.
func (v *PromptsView) previewCmd() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.prompts) {
		return nil
	}
	id := v.prompts[v.cursor].ID
	client := v.client

	// Build a map[string]any from the current propValues.
	props := make(map[string]any, len(v.propValues))
	for k, val := range v.propValues {
		props[k] = val
	}

	v.previewLoading = true
	v.previewErr = nil
	return func() tea.Msg {
		rendered, err := client.PreviewPrompt(context.Background(), id, props)
		return promptPreviewMsg{promptID: id, rendered: rendered, err: err}
	}
}

// savePromptCmd builds a tea.Cmd that calls UpdatePrompt and returns a promptSaveMsg.
func (v *PromptsView) savePromptCmd() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.prompts) {
		return nil
	}
	id := v.prompts[v.cursor].ID
	content := v.editor.Value()
	client := v.client
	return func() tea.Msg {
		err := client.UpdatePrompt(context.Background(), id, content)
		return promptSaveMsg{err: err}
	}
}

// openExternalEditorCmd writes the current editor content to a temp file,
// then hands off to $EDITOR via handoff.Handoff. The resulting HandoffMsg
// is received in Update and the textarea is updated from the file.
func (v *PromptsView) openExternalEditorCmd() tea.Cmd {
	editor := resolveEditorBinary()
	content := v.editor.Value()
	tmpFile, err := os.CreateTemp("", "prompt-*.mdx")
	if err != nil {
		return func() tea.Msg {
			return components.ShowToastMsg{
				Title: "Editor failed",
				Body:  fmt.Sprintf("create temp file: %v", err),
				Level: components.ToastLevelError,
			}
		}
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return func() tea.Msg {
			return components.ShowToastMsg{
				Title: "Editor failed",
				Body:  fmt.Sprintf("write temp file: %v", err),
				Level: components.ToastLevelError,
			}
		}
	}
	_ = tmpFile.Close()
	v.tmpPath = tmpFile.Name()
	return handoff.Handoff(handoff.Options{
		Binary: editor,
		Args:   []string{v.tmpPath},
		Tag:    promptEditorTag,
	})
}

// resolveEditorBinary returns the best available editor binary by checking
// $EDITOR and $VISUAL environment variables before falling back to "vi".
func resolveEditorBinary() string {
	for _, env := range []string{"EDITOR", "VISUAL"} {
		if e := os.Getenv(env); e != "" {
			if _, err := exec.LookPath(e); err == nil {
				return e
			}
		}
	}
	return "vi"
}

// Name returns the view identifier.
func (v *PromptsView) Name() string {
	return "prompts"
}

// ShortHelp returns context-sensitive keybinding hints for the help bar.
func (v *PromptsView) ShortHelp() []key.Binding {
	if v.focus == focusEditor {
		return []key.Binding{
			key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
			key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "open editor")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	if v.focus == focusPreview {
		return []key.Binding{
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh preview")),
			key.NewBinding(key.WithKeys("v", "esc"), key.WithHelp("v/esc", "close preview")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "discover props")),
		key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "preview")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
