package views

// sql.go — SQL Browser view (PRD §6.11 feat-sql-browser).
//
// Layout: split-pane, left = table sidebar, right = query editor + results.
// Keys:
//   - Up/Down/j/k   — navigate table list (left pane focused)
//   - Enter          — toggle column details for selected table (left pane)
//   - Tab            — toggle pane focus (left ↔ right)
//   - Ctrl+Enter     — execute query (right pane) — inserts newline when in multiline editor
//   - x              — execute query (right pane, shortcuts)
//   - Up/Down arrows — navigate query history (when query is empty, right pane)
//   - Backspace      — delete last char in query editor
//   - r              — refresh table list
//   - Esc            — pop view

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*SQLBrowserView)(nil)

// --- Internal message types ---

type sqlTablesLoadedMsg struct {
	tables []smithers.TableInfo
}

type sqlTablesErrorMsg struct {
	err error
}

type sqlQueryResultMsg struct {
	result *smithers.SQLResult
}

type sqlQueryErrorMsg struct {
	err error
}

// sqlSchemaLoadedMsg is emitted when a table schema is fetched.
type sqlSchemaLoadedMsg struct {
	tableName string
	schema    *smithers.TableSchema
}

// sqlSchemaErrorMsg is emitted when schema fetch fails.
type sqlSchemaErrorMsg struct {
	tableName string
	err       error
}

// --- Left pane: table sidebar ---

// sqlTableEntry holds a table with its optional expanded schema.
type sqlTableEntry struct {
	info     smithers.TableInfo
	expanded bool
	schema   *smithers.TableSchema
	loading  bool // schema is being fetched
}

// sqlTablePane lists tables on the left side.
type sqlTablePane struct {
	entries []sqlTableEntry
	cursor  int
	width   int
	height  int
	loading bool
}

func (p *sqlTablePane) Init() tea.Cmd { return nil }

func (p *sqlTablePane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
			if p.cursor > 0 {
				p.cursor--
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
			if p.cursor < len(p.entries)-1 {
				p.cursor++
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("home", "g"))):
			p.cursor = 0
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("end", "G"))):
			if len(p.entries) > 0 {
				p.cursor = len(p.entries) - 1
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
			if len(p.entries) > 0 && p.cursor < len(p.entries) {
				entry := &p.entries[p.cursor]
				if entry.expanded {
					// Collapse.
					entry.expanded = false
					return p, nil
				}
				// Expand: if we already have schema, just expand.
				if entry.schema != nil {
					entry.expanded = true
					return p, nil
				}
				// Need to fetch schema: mark loading and emit select msg for auto-fill,
				// and also request schema.
				entry.expanded = true
				entry.loading = true
				tbl := entry.info
				query := fmt.Sprintf("SELECT * FROM %s LIMIT 100", quoteTableName(tbl.Name))
				return p, tea.Batch(
					func() tea.Msg { return sqlTableSelectedMsg{query: query} },
					func() tea.Msg { return sqlFetchSchemaMsg{tableName: tbl.Name} },
				)
			}
		}
	}
	return p, nil
}

func (p *sqlTablePane) SetSize(w, h int) { p.width = w; p.height = h }

// lineCount returns the number of display lines for the entry at index i.
func (p *sqlTablePane) lineCount(i int) int {
	entry := p.entries[i]
	if !entry.expanded || entry.schema == nil {
		return 1
	}
	return 1 + len(entry.schema.Columns)
}

func (p *sqlTablePane) View() string {
	if p.loading {
		return lipgloss.NewStyle().Faint(true).Render("Loading tables...")
	}
	if len(p.entries) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("No tables found.")
	}

	var b strings.Builder

	// Compute scroll offset to keep cursor visible using line-based accounting.
	// First, compute cumulative line offsets per entry.
	cumLines := make([]int, len(p.entries)+1)
	for i := range p.entries {
		cumLines[i+1] = cumLines[i] + p.lineCount(i)
	}
	totalLines := cumLines[len(p.entries)]

	visibleLines := p.height
	if visibleLines <= 0 {
		visibleLines = totalLines
	}

	// Determine which line the cursor is on (first line of its entry).
	cursorLine := cumLines[p.cursor]

	// Scroll offset in lines.
	scrollLine := 0
	if cursorLine >= visibleLines {
		scrollLine = cursorLine - visibleLines + 1
	}

	linesRendered := 0
	for i := range p.entries {
		startLine := cumLines[i]
		entryLines := p.lineCount(i)
		endLine := startLine + entryLines

		// Skip entries that are fully before the scroll window.
		if endLine <= scrollLine {
			continue
		}
		// Stop once we've filled the pane.
		if linesRendered >= visibleLines {
			break
		}

		entry := p.entries[i]
		tbl := entry.info
		isCursor := i == p.cursor

		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if isCursor {
			cursor = "▸ "
			nameStyle = nameStyle.Bold(true)
		}

		// Truncate name to fit pane width minus cursor prefix (2 chars).
		maxNameWidth := p.width - 2
		if maxNameWidth < 5 {
			maxNameWidth = 5
		}
		name := tbl.Name
		if lipgloss.Width(name) > maxNameWidth {
			name = truncate(name, maxNameWidth)
		}

		colHint := ""
		if entry.expanded {
			expandIcon := "▾ "
			b.WriteString(expandIcon + nameStyle.Render(name) + "\n")
			linesRendered++

			// Render column details.
			if entry.loading {
				faintStyle := lipgloss.NewStyle().Faint(true)
				b.WriteString("    " + faintStyle.Render("loading schema...") + "\n")
				linesRendered++
			} else if entry.schema != nil {
				for _, col := range entry.schema.Columns {
					if linesRendered >= visibleLines {
						break
					}
					b.WriteString(renderColumnLine(col, p.width) + "\n")
					linesRendered++
				}
			}
			continue
		}

		// Not expanded: show inline row-count / view hint.
		if tbl.RowCount > 0 {
			colHint = lipgloss.NewStyle().Faint(true).Render(
				fmt.Sprintf(" (%d rows)", tbl.RowCount),
			)
		} else if tbl.Type == "view" {
			colHint = lipgloss.NewStyle().Faint(true).Render(" (view)")
		}

		b.WriteString(cursor + nameStyle.Render(name) + colHint + "\n")
		linesRendered++
	}

	return b.String()
}

// renderColumnLine renders a single Column as a sidebar detail line.
// The format is: "  • <name> <type>[constraints]"
// Constraints are appended compactly using single-letter hints when space is tight.
func renderColumnLine(col smithers.Column, paneWidth int) string {
	faintStyle := lipgloss.NewStyle().Faint(true)

	colType := col.Type
	if colType == "" {
		colType = "?"
	}

	var constraints []string
	if col.PrimaryKey {
		constraints = append(constraints, "PK")
	}
	if col.NotNull {
		constraints = append(constraints, "NOT NULL")
	}

	constraintStr := ""
	if len(constraints) > 0 {
		constraintStr = " " + strings.Join(constraints, " ")
	}

	// Build a compact line: "  • name type[constraints]"
	// We use a shorter indent (2 spaces + bullet + space) for narrow panes.
	line := fmt.Sprintf("  • %s %s%s", col.Name, colType, constraintStr)
	if paneWidth > 4 && lipgloss.Width(line) > paneWidth {
		// Truncate preserving as much as possible.
		line = truncate(line, paneWidth)
	}
	return faintStyle.Render(line)
}

// sqlFetchSchemaMsg requests a schema fetch from the view level.
type sqlFetchSchemaMsg struct {
	tableName string
}

// --- sqlTableSelectedMsg: emitted when Enter is pressed on a table ---

type sqlTableSelectedMsg struct {
	query string
}

// --- Right pane: query editor + results ---

// sqlEditorPane holds a multi-line text editor and result display.
//
// Multi-line editing:
//   - Enter inserts a newline in the query text.
//   - Ctrl+Enter / 'x' executes the query.
//   - Up/Down arrows navigate history when the cursor is on the first/last line.
//   - Left/Right move the cursor within the text (rune-granularity).
//
// Query history:
//   - Every successful execution pushes the query to history (up to 100 entries).
//   - Up arrow while cursor is on line 0 walks backward through history.
//   - Down arrow while at the latest history item restores the draft.
//
// Results table horizontal scroll:
//   - '<' / 'shift+left' scrolls the results table left by one column.
//   - '>' / 'shift+right' scrolls the results table right by one column.
type sqlEditorPane struct {
	query     string // current editor content (may contain \n)
	cursor    int    // byte-offset cursor within query
	result    *smithers.SQLResult
	resultErr error
	executing bool
	width     int
	height    int

	history      []string // executed queries, oldest first
	historyIndex int      // -1 = live draft; ≥0 = index into history (0=oldest)
	draft        string   // saved draft while browsing history

	// Horizontal scroll offset for the results table (column index of the
	// leftmost visible column).  Bounded to [0, len(columns)-1].
	resultColOffset int
}

// newSQLEditorPane creates an editor pane with correct initial state.
func newSQLEditorPane() *sqlEditorPane {
	return &sqlEditorPane{historyIndex: -1}
}

func (p *sqlEditorPane) Init() tea.Cmd { return nil }

// currentLine returns the 0-based line index of the cursor.
func (p *sqlEditorPane) currentLine() int {
	return strings.Count(p.query[:p.cursor], "\n")
}

// totalLines returns the number of lines in the query.
func (p *sqlEditorPane) totalLines() int {
	if p.query == "" {
		return 1
	}
	return strings.Count(p.query, "\n") + 1
}

// pushHistory adds a query to history (dedup consecutive identical entries).
func (p *sqlEditorPane) pushHistory(q string) {
	if q == "" {
		return
	}
	if len(p.history) > 0 && p.history[len(p.history)-1] == q {
		return
	}
	p.history = append(p.history, q)
	if len(p.history) > 100 {
		p.history = p.history[len(p.history)-100:]
	}
	p.historyIndex = -1
}

func (p *sqlEditorPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		// --- Cursor movement ---
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("left"))):
			if p.cursor > 0 {
				// Step back one rune.
				runes := []rune(p.query[:p.cursor])
				p.cursor -= len(string(runes[len(runes)-1]))
			}
			return p, nil

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("right"))):
			if p.cursor < len(p.query) {
				r, size := rune(p.query[p.cursor]), 1
				_ = r
				// Step forward one rune (UTF-8 safe).
				runes := []rune(p.query[p.cursor:])
				if len(runes) > 0 {
					p.cursor += len(string(runes[0]))
					_ = size
				}
			}
			return p, nil

		// --- History navigation (Up arrow) ---
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up"))):
			if len(p.history) == 0 {
				return p, nil
			}
			if p.historyIndex == -1 {
				// Entering history: save draft.
				p.draft = p.query
				p.historyIndex = len(p.history) - 1
			} else if p.historyIndex > 0 {
				p.historyIndex--
			}
			p.query = p.history[p.historyIndex]
			p.cursor = len(p.query)
			return p, nil

		// --- History navigation (Down arrow) ---
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down"))):
			if p.historyIndex == -1 {
				return p, nil
			}
			if p.historyIndex < len(p.history)-1 {
				p.historyIndex++
				p.query = p.history[p.historyIndex]
			} else {
				// Back to live draft.
				p.historyIndex = -1
				p.query = p.draft
			}
			p.cursor = len(p.query)
			return p, nil

		// --- Enter: insert newline ---
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
			p.query = p.query[:p.cursor] + "\n" + p.query[p.cursor:]
			p.cursor++
			return p, nil

		// --- Backspace: delete rune before cursor ---
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("backspace"))):
			if p.cursor > 0 {
				runes := []rune(p.query[:p.cursor])
				removed := string(runes[len(runes)-1])
				p.query = p.query[:p.cursor-len(removed)] + p.query[p.cursor:]
				p.cursor -= len(removed)
			}
			return p, nil

		// --- Horizontal scroll for results table ---
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("<", "shift+left"))):
			if p.result != nil && p.resultColOffset > 0 {
				p.resultColOffset--
			}
			return p, nil

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys(">", "shift+right"))):
			if p.result != nil && len(p.result.Columns) > 0 {
				maxOffset := len(p.result.Columns) - 1
				if p.resultColOffset < maxOffset {
					p.resultColOffset++
				}
			}
			return p, nil

		default:
			// Any printable character: insert at cursor.
			if ch := keyToChar(keyMsg); ch != "" {
				// Do not intercept '<' / '>' for editing when they are not
				// bound as scroll keys (they match the bindings above, so
				// this branch is only reached for other printable characters).
				p.query = p.query[:p.cursor] + ch + p.query[p.cursor:]
				p.cursor += len(ch)
				// Exit history mode on any editing.
				p.historyIndex = -1
			}
		}
	}
	return p, nil
}

func (p *sqlEditorPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *sqlEditorPane) View() string {
	var b strings.Builder

	// --- Query editor section ---
	boldStyle := lipgloss.NewStyle().Bold(true)
	faintStyle := lipgloss.NewStyle().Faint(true)

	histHint := ""
	if p.historyIndex >= 0 {
		histHint = faintStyle.Render(fmt.Sprintf(" [hist %d/%d]", p.historyIndex+1, len(p.history)))
	}
	b.WriteString(boldStyle.Render("SQL Query") + histHint + "\n")
	divWidth := p.width - 2
	if divWidth < 1 {
		divWidth = 1
	}
	b.WriteString(faintStyle.Render(strings.Repeat("─", divWidth)) + "\n")

	// Render query with block cursor injected at the cursor position.
	queryWithCursor := p.query[:p.cursor] + "█" + p.query[p.cursor:]
	queryLines := strings.Split(queryWithCursor, "\n")
	for _, line := range queryLines {
		wrapped := wrapQueryLines(line, p.width-2)
		for _, wl := range wrapped {
			b.WriteString("  " + wl + "\n")
		}
	}

	// Hints.
	hint := faintStyle.Render("[ctrl+enter / x] execute  [↑↓] history  [enter] newline")
	b.WriteString("\n" + "  " + hint + "\n\n")

	// --- Results section ---
	b.WriteString(boldStyle.Render("Results") + "\n")
	b.WriteString(faintStyle.Render(strings.Repeat("─", divWidth)) + "\n")

	if p.executing {
		b.WriteString("  " + faintStyle.Render("Executing...") + "\n")
		return b.String()
	}
	if p.resultErr != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString("  " + errStyle.Render(fmt.Sprintf("Error: %v", p.resultErr)) + "\n")
		return b.String()
	}
	if p.result == nil {
		b.WriteString("  " + faintStyle.Render("No results yet. Execute a query.") + "\n")
		return b.String()
	}
	if len(p.result.Columns) == 0 {
		b.WriteString("  " + faintStyle.Render("Query executed (no rows returned).") + "\n")
		return b.String()
	}

	b.WriteString(renderSQLResultTable(p.result, p.width, p.resultColOffset))

	// Show scroll hint when there are more columns than visible.
	if len(p.result.Columns) > 1 {
		scrollHint := fmt.Sprintf("  col %d/%d", p.resultColOffset+1, len(p.result.Columns))
		if p.resultColOffset > 0 {
			scrollHint += "  [<] scroll left"
		}
		if p.resultColOffset < len(p.result.Columns)-1 {
			scrollHint += "  [>] scroll right"
		}
		b.WriteString(faintStyle.Render(scrollHint) + "\n")
	}
	return b.String()
}

// --- SQLBrowserView (the top-level View) ---

// SQLBrowserView implements the SQL Browser (PRD §6.11 feat-sql-browser).
// Left pane: table list with expandable column schema. Right pane: query editor + results.
type SQLBrowserView struct {
	client     *smithers.Client
	tables     []smithers.TableInfo
	width      int
	height     int
	loading    bool
	err        error
	splitPane  *components.SplitPane
	tablePane  *sqlTablePane
	editorPane *sqlEditorPane
}

// NewSQLBrowserView creates a new SQL browser view.
func NewSQLBrowserView(client *smithers.Client) *SQLBrowserView {
	tblPane := &sqlTablePane{loading: true}
	edPane := newSQLEditorPane()
	sp := components.NewSplitPane(tblPane, edPane, components.SplitPaneOpts{
		LeftWidth:         30,
		CompactBreakpoint: 80,
	})
	return &SQLBrowserView{
		client:     client,
		loading:    true,
		splitPane:  sp,
		tablePane:  tblPane,
		editorPane: edPane,
	}
}

// Init loads tables from the client.
func (v *SQLBrowserView) Init() tea.Cmd {
	return v.loadTablesCmd()
}

func (v *SQLBrowserView) loadTablesCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		tables, err := client.ListTables(context.Background())
		if err != nil {
			return sqlTablesErrorMsg{err: err}
		}
		return sqlTablesLoadedMsg{tables: tables}
	}
}

func (v *SQLBrowserView) executeQueryCmd(query string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		result, err := client.ExecuteSQL(context.Background(), query)
		if err != nil {
			return sqlQueryErrorMsg{err: err}
		}
		return sqlQueryResultMsg{result: result}
	}
}

func (v *SQLBrowserView) fetchSchemaCmd(tableName string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		schema, err := client.GetTableSchema(context.Background(), tableName)
		if err != nil {
			return sqlSchemaErrorMsg{tableName: tableName, err: err}
		}
		return sqlSchemaLoadedMsg{tableName: tableName, schema: schema}
	}
}

// Update handles messages for the SQL browser view.
func (v *SQLBrowserView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case sqlTablesLoadedMsg:
		v.tables = msg.tables
		// Convert to entries.
		entries := make([]sqlTableEntry, len(msg.tables))
		for i, t := range msg.tables {
			entries[i] = sqlTableEntry{info: t}
		}
		v.tablePane.entries = entries
		v.tablePane.loading = false
		v.loading = false
		v.splitPane.SetSize(v.width, max(0, v.height-2))
		return v, nil

	case sqlTablesErrorMsg:
		v.err = msg.err
		v.tablePane.loading = false
		v.loading = false
		return v, nil

	case sqlQueryResultMsg:
		v.editorPane.result = msg.result
		v.editorPane.resultErr = nil
		v.editorPane.executing = false
		v.editorPane.resultColOffset = 0 // reset scroll on new result
		return v, nil

	case sqlQueryErrorMsg:
		v.editorPane.resultErr = msg.err
		v.editorPane.result = nil
		v.editorPane.executing = false
		v.editorPane.resultColOffset = 0 // reset scroll on error
		return v, nil

	case sqlTableSelectedMsg:
		// Auto-fill the query editor and shift focus to the right pane.
		v.editorPane.query = msg.query
		v.editorPane.cursor = len(msg.query)
		v.editorPane.result = nil
		v.editorPane.resultErr = nil
		v.splitPane.SetFocus(components.FocusRight)
		return v, nil

	case sqlFetchSchemaMsg:
		// Table pane emitted a request to fetch schema; dispatch cmd from view level.
		return v, v.fetchSchemaCmd(msg.tableName)

	case sqlSchemaLoadedMsg:
		for i := range v.tablePane.entries {
			if v.tablePane.entries[i].info.Name == msg.tableName {
				v.tablePane.entries[i].schema = msg.schema
				v.tablePane.entries[i].loading = false
				break
			}
		}
		return v, nil

	case sqlSchemaErrorMsg:
		for i := range v.tablePane.entries {
			if v.tablePane.entries[i].info.Name == msg.tableName {
				v.tablePane.entries[i].loading = false
				// Schema fetch failed: keep expanded=true, schema=nil to show no columns.
				break
			}
		}
		return v, nil

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			// Only refresh when the left (table) pane is focused; when right pane
			// is focused, 'r' is a printable character for the query editor.
			if v.splitPane.Focus() == components.FocusLeft {
				v.loading = true
				v.tablePane.loading = true
				v.err = nil
				return v, v.loadTablesCmd()
			}

		// Execute query: ctrl+enter (right pane focused only).
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+enter"))):
			if v.splitPane.Focus() == components.FocusRight {
				query := strings.TrimSpace(v.editorPane.query)
				if query != "" {
					v.editorPane.pushHistory(query)
					v.editorPane.executing = true
					return v, v.executeQueryCmd(query)
				}
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			if v.splitPane.Focus() == components.FocusRight {
				query := strings.TrimSpace(v.editorPane.query)
				if query != "" {
					v.editorPane.pushHistory(query)
					v.editorPane.executing = true
					return v, v.executeQueryCmd(query)
				}
			}
			// Fall through to split pane for left pane or typing.
		}
	}

	// Forward to split pane (handles Tab, j/k, Enter, typed chars, etc.).
	newSP, cmd := v.splitPane.Update(msg)
	v.splitPane = newSP
	return v, cmd
}

// View renders the SQL browser.
func (v *SQLBrowserView) View() string {
	var b strings.Builder

	// Header
	title := "SMITHERS › SQL Browser"
	header := lipgloss.NewStyle().Bold(true).Render(title)
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine + "\n\n")

	if v.loading {
		b.WriteString("  Loading tables...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	b.WriteString(v.splitPane.View())
	return b.String()
}

// Name returns the view name for the router.
func (v *SQLBrowserView) Name() string { return "sql" }

// SetSize stores terminal dimensions and propagates to the split pane.
func (v *SQLBrowserView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.splitPane.SetSize(width, max(0, height-2))
}

// ShortHelp returns keybinding hints for the help bar.
func (v *SQLBrowserView) ShortHelp() []key.Binding {
	if v.splitPane != nil && v.splitPane.Focus() == components.FocusRight {
		return []key.Binding{
			key.NewBinding(key.WithKeys("ctrl+enter", "x"), key.WithHelp("ctrl+enter/x", "execute")),
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "history")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "tables")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑↓/jk", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand/collapse")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "editor")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

// --- Exported accessors (for tests) ---

// Tables returns the current table list.
func (v *SQLBrowserView) Tables() []smithers.TableInfo { return v.tables }

// Loading reports whether the view is waiting for the initial table list.
func (v *SQLBrowserView) Loading() bool { return v.loading }

// Query returns the current editor content.
func (v *SQLBrowserView) Query() string { return v.editorPane.query }

// Result returns the most recent query result, or nil.
func (v *SQLBrowserView) Result() *smithers.SQLResult { return v.editorPane.result }

// ResultErr returns the most recent query error, or nil.
func (v *SQLBrowserView) ResultErr() error { return v.editorPane.resultErr }

// ResultColOffset returns the current horizontal scroll offset for the results
// table (the index of the leftmost visible column).
func (v *SQLBrowserView) ResultColOffset() int { return v.editorPane.resultColOffset }

// --- Helpers ---

// quoteTableName wraps a table name in double-quotes for safe SQL interpolation.
// Embedded double-quotes are escaped by doubling.
func quoteTableName(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// keyToChar converts a key press to the character it represents, if printable.
// Returns "" for non-printable or modifier keys.
func keyToChar(msg tea.KeyPressMsg) string {
	s := msg.Text
	if len(s) == 0 {
		return ""
	}
	// Filter out bare modifier keys, control sequences, etc.
	r := []rune(s)
	if len(r) == 1 {
		ch := r[0]
		// Printable ASCII range: space (32) through tilde (126), plus common extended chars.
		if ch >= 32 && ch != 127 {
			return string(ch)
		}
	}
	return ""
}

// wrapQueryLines wraps a query string to fit within maxWidth, splitting at spaces
// or hard-cutting when no space is available.
func wrapQueryLines(query string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	var lines []string
	for len(query) > 0 {
		runes := []rune(query)
		if len(runes) <= maxWidth {
			lines = append(lines, query)
			break
		}
		// Try to break at a space.
		cut := maxWidth
		for cut > 0 && runes[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = maxWidth // no space found; hard cut
		}
		lines = append(lines, string(runes[:cut]))
		query = strings.TrimLeft(string(runes[cut:]), " ")
	}
	return lines
}

// renderSQLResultTable renders a SQLResult as a plain-text table using lipgloss.
// colOffset is the index of the first visible column (for horizontal scrolling).
// Columns are padded to their widest value; total width is capped at maxWidth.
// When more columns exist to the right than can fit, a "▶" indicator is shown.
func renderSQLResultTable(result *smithers.SQLResult, maxWidth int, colOffset int) string {
	if result == nil || len(result.Columns) == 0 {
		return ""
	}

	// Clamp colOffset to valid range.
	if colOffset < 0 {
		colOffset = 0
	}
	if colOffset >= len(result.Columns) {
		colOffset = len(result.Columns) - 1
	}

	// Compute natural widths for all columns (header or widest cell).
	allWidths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		allWidths[i] = len(col)
	}
	for _, row := range result.Rows {
		for i, cell := range row {
			if i >= len(allWidths) {
				break
			}
			s := fmt.Sprintf("%v", cell)
			if len(s) > allWidths[i] {
				allWidths[i] = len(s)
			}
		}
	}

	// Determine which columns fit within maxWidth starting from colOffset.
	const indent = 2  // "  " prefix on every line
	const colSep = 2  // "  " separator between columns
	available := maxWidth - indent
	if available < 10 {
		available = 10
	}

	var visibleIdx []int // indices into result.Columns
	var visibleW []int   // display widths for each visible column
	used := 0

	for i := colOffset; i < len(result.Columns); i++ {
		w := allWidths[i]
		// Cap individual column width to half available space so one huge
		// column doesn't prevent others from appearing.
		maxColW := available / 2
		if maxColW < 8 {
			maxColW = 8
		}
		if w > maxColW {
			w = maxColW
		}
		need := w
		if len(visibleIdx) > 0 {
			need += colSep
		}
		if used+need > available && len(visibleIdx) > 0 {
			break // no room for another column
		}
		visibleIdx = append(visibleIdx, i)
		visibleW = append(visibleW, w)
		used += need
	}

	// Always show at least one column even on a very narrow terminal.
	if len(visibleIdx) == 0 {
		visibleIdx = []int{colOffset}
		visibleW = []int{available}
	}

	hasMore := colOffset+len(visibleIdx) < len(result.Columns)

	headerStyle := lipgloss.NewStyle().Bold(true)
	faintStyle := lipgloss.NewStyle().Faint(true)

	var b strings.Builder

	// Header row.
	b.WriteString("  ")
	for j, ci := range visibleIdx {
		w := visibleW[j]
		cell := result.Columns[ci]
		if len(cell) > w {
			cell = cell[:w-1] + "…"
		}
		b.WriteString(headerStyle.Render(padRight(cell, w)))
		if j < len(visibleIdx)-1 {
			b.WriteString("  ")
		}
	}
	if hasMore {
		b.WriteString(faintStyle.Render("  ▶"))
	}
	b.WriteString("\n")

	// Separator line.
	b.WriteString("  ")
	for j, w := range visibleW {
		b.WriteString(faintStyle.Render(strings.Repeat("─", w)))
		if j < len(visibleW)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")

	// Data rows (cap at 200 for performance).
	maxRows := 200
	if len(result.Rows) < maxRows {
		maxRows = len(result.Rows)
	}
	for _, row := range result.Rows[:maxRows] {
		b.WriteString("  ")
		for j, ci := range visibleIdx {
			w := visibleW[j]
			var s string
			if ci < len(row) {
				s = fmt.Sprintf("%v", row[ci])
			}
			if len(s) > w {
				s = s[:w-1] + "…"
			}
			b.WriteString(padRight(s, w))
			if j < len(visibleIdx)-1 {
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}

	if len(result.Rows) > maxRows {
		b.WriteString(faintStyle.Render(
			fmt.Sprintf("  … %d more rows", len(result.Rows)-maxRows),
		) + "\n")
	}

	rowLabel := "rows"
	if len(result.Rows) == 1 {
		rowLabel = "row"
	}
	b.WriteString(faintStyle.Render(
		fmt.Sprintf("  %d %s", len(result.Rows), rowLabel),
	) + "\n")

	return b.String()
}
