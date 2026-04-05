package views

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

// sampleTables returns n synthetic TableInfo values.
func sampleTables(n int) []smithers.TableInfo {
	tables := make([]smithers.TableInfo, n)
	for i := range n {
		tables[i] = smithers.TableInfo{
			Name:     fmt.Sprintf("_smithers_table_%02d", i+1),
			Type:     "table",
			RowCount: int64((i + 1) * 10),
		}
	}
	return tables
}

// loadedSQL fires a sqlTablesLoadedMsg and returns the resulting view.
func loadedSQL(tables []smithers.TableInfo, width, height int) *SQLBrowserView {
	v := NewSQLBrowserView(nil)
	v.SetSize(width, height)
	updated, _ := v.Update(sqlTablesLoadedMsg{tables: tables})
	return updated.(*SQLBrowserView)
}

// sqlPressKey sends a single key press to the SQL browser view.
func sqlPressKey(v *SQLBrowserView, code rune) (*SQLBrowserView, tea.Cmd) {
	updated, cmd := v.Update(tea.KeyPressMsg{Code: code})
	return updated.(*SQLBrowserView), cmd
}

// sqlTypeChar appends a rune to the editor (simulates typing a printable character).
func sqlTypeChar(v *SQLBrowserView, ch rune) *SQLBrowserView {
	updated, _ := v.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	return updated.(*SQLBrowserView)
}

// sqlFocusRight moves focus to the right (editor) pane.
func sqlFocusRight(v *SQLBrowserView) *SQLBrowserView {
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return updated.(*SQLBrowserView)
}

// sqlHelpKeys extracts help text key labels from a binding slice.
func sqlHelpKeys(bindings []key.Binding) []string {
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.Help().Key)
	}
	return out
}

// --- 1. Construction and initial state ---

func TestSQLBrowserView_NewViewLoading(t *testing.T) {
	v := NewSQLBrowserView(nil)
	assert.True(t, v.Loading())
	assert.Nil(t, v.Tables())
	assert.Equal(t, "sql", v.Name())
}

func TestSQLBrowserView_InitReturnsCmd(t *testing.T) {
	v := NewSQLBrowserView(nil)
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init must return a non-nil command")
}

// --- 2. Table loading ---

func TestSQLBrowserView_TablesLoadedMsg(t *testing.T) {
	v := loadedSQL(sampleTables(3), 120, 40)
	assert.False(t, v.Loading())
	require.Len(t, v.Tables(), 3)
	out := v.View()
	assert.Contains(t, out, "_smithers_table_01")
	assert.Contains(t, out, "_smithers_table_02")
	assert.Contains(t, out, "_smithers_table_03")
}

func TestSQLBrowserView_TablesErrorMsg(t *testing.T) {
	v := NewSQLBrowserView(nil)
	v.SetSize(120, 40)
	updated, _ := v.Update(sqlTablesErrorMsg{err: errors.New("connection refused")})
	sv := updated.(*SQLBrowserView)
	assert.False(t, sv.Loading())
	out := sv.View()
	assert.Contains(t, out, "Error:")
	assert.Contains(t, out, "connection refused")
}

func TestSQLBrowserView_EmptyTableList(t *testing.T) {
	v := loadedSQL([]smithers.TableInfo{}, 120, 40)
	out := v.View()
	assert.Contains(t, out, "No tables found")
}

func TestSQLBrowserView_LoadingStateDuringInit(t *testing.T) {
	v := NewSQLBrowserView(nil)
	out := v.View()
	assert.Contains(t, out, "Loading tables")
}

// --- 3. Header rendering ---

func TestSQLBrowserView_HeaderContainsSQLBrowser(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	out := v.View()
	assert.Contains(t, out, "SQL Browser")
}

func TestSQLBrowserView_HeaderContainsEscHint(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	out := v.View()
	assert.Contains(t, out, "Esc")
}

// --- 4. Table sidebar navigation ---

func TestSQLBrowserView_CursorDown(t *testing.T) {
	v := loadedSQL(sampleTables(5), 120, 40)
	assert.Equal(t, 0, v.tablePane.cursor)

	for i := 1; i <= 3; i++ {
		v, _ = sqlPressKey(v, 'j')
		assert.Equal(t, i, v.tablePane.cursor)
	}
}

func TestSQLBrowserView_CursorUp(t *testing.T) {
	v := loadedSQL(sampleTables(5), 120, 40)
	// Move down first.
	v, _ = sqlPressKey(v, 'j')
	v, _ = sqlPressKey(v, 'j')
	assert.Equal(t, 2, v.tablePane.cursor)

	v, _ = sqlPressKey(v, 'k')
	assert.Equal(t, 1, v.tablePane.cursor)
}

func TestSQLBrowserView_CursorClampedAtBottom(t *testing.T) {
	tables := sampleTables(3)
	v := loadedSQL(tables, 120, 40)
	for range 10 {
		v, _ = sqlPressKey(v, 'j')
	}
	assert.Equal(t, len(tables)-1, v.tablePane.cursor)
}

func TestSQLBrowserView_CursorClampedAtTop(t *testing.T) {
	v := loadedSQL(sampleTables(3), 120, 40)
	for range 5 {
		v, _ = sqlPressKey(v, 'k')
	}
	assert.Equal(t, 0, v.tablePane.cursor)
}

func TestSQLBrowserView_ArrowKeysNavigate(t *testing.T) {
	v := loadedSQL(sampleTables(3), 120, 40)
	v, _ = sqlPressKey(v, tea.KeyDown)
	assert.Equal(t, 1, v.tablePane.cursor)
	v, _ = sqlPressKey(v, tea.KeyUp)
	assert.Equal(t, 0, v.tablePane.cursor)
}

// --- 5. Enter on table auto-fills query and requests schema ---

func TestSQLBrowserView_EnterAutoFillsQuery(t *testing.T) {
	v := loadedSQL(sampleTables(3), 120, 40)
	// cursor=0 → _smithers_table_01

	_, cmd := sqlPressKey(v, tea.KeyEnter)
	require.NotNil(t, cmd)

	// Enter now emits a batch: sqlTableSelectedMsg + sqlFetchSchemaMsg.
	msgs := drainBatchCmd(t, cmd)
	var sel sqlTableSelectedMsg
	found := false
	for _, m := range msgs {
		if s, ok := m.(sqlTableSelectedMsg); ok {
			sel = s
			found = true
		}
	}
	require.True(t, found, "Enter must emit sqlTableSelectedMsg in batch, got %v", msgs)
	assert.Contains(t, sel.query, `"_smithers_table_01"`)
	assert.Contains(t, sel.query, "SELECT * FROM")
	assert.Contains(t, sel.query, "LIMIT 100")
}

func TestSQLBrowserView_EnterOnSecondTable(t *testing.T) {
	v := loadedSQL(sampleTables(3), 120, 40)
	v, _ = sqlPressKey(v, 'j') // move to index 1
	_, cmd := sqlPressKey(v, tea.KeyEnter)
	require.NotNil(t, cmd)
	msgs := drainBatchCmd(t, cmd)
	var sel sqlTableSelectedMsg
	found := false
	for _, m := range msgs {
		if s, ok := m.(sqlTableSelectedMsg); ok {
			sel = s
			found = true
		}
	}
	require.True(t, found)
	assert.Contains(t, sel.query, `"_smithers_table_02"`)
}

func TestSQLBrowserView_EnterEmitsFetchSchemaMsg(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	_, cmd := sqlPressKey(v, tea.KeyEnter)
	require.NotNil(t, cmd)
	msgs := drainBatchCmd(t, cmd)
	var fetchMsg sqlFetchSchemaMsg
	found := false
	for _, m := range msgs {
		if f, ok := m.(sqlFetchSchemaMsg); ok {
			fetchMsg = f
			found = true
		}
	}
	require.True(t, found, "Enter must include sqlFetchSchemaMsg in batch")
	assert.Equal(t, "_smithers_table_01", fetchMsg.tableName)
}

func TestSQLBrowserView_EnterCollapsesExpandedTable(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	// Expand entry 0 by setting expanded=true and providing a schema.
	v.tablePane.entries[0].expanded = true
	v.tablePane.entries[0].schema = &smithers.TableSchema{
		TableName: "_smithers_table_01",
		Columns: []smithers.Column{
			{Name: "id", Type: "INTEGER", PrimaryKey: true, NotNull: true},
		},
	}
	// Press Enter again to collapse.
	v, cmd := sqlPressKey(v, tea.KeyEnter)
	assert.Nil(t, cmd, "collapsing should emit no cmd")
	assert.False(t, v.tablePane.entries[0].expanded, "entry should be collapsed after second Enter")
}

func TestSQLBrowserView_EnterExpandsWithCachedSchema(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	// Pre-load schema.
	v.tablePane.entries[0].schema = &smithers.TableSchema{
		TableName: "_smithers_table_01",
		Columns:   []smithers.Column{{Name: "id", Type: "INTEGER"}},
	}
	// First Enter: should expand without fetching (schema already cached).
	_, cmd := sqlPressKey(v, tea.KeyEnter)
	require.Nil(t, cmd, "should not fetch schema when cached")
	assert.True(t, v.tablePane.entries[0].expanded)
}

func TestSQLBrowserView_TableSelectedMsgSetsQuery(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	updated, _ := v.Update(sqlTableSelectedMsg{query: `SELECT * FROM "test" LIMIT 100`})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, `SELECT * FROM "test" LIMIT 100`, sv.Query())
}

func TestSQLBrowserView_TableSelectedMsgShiftsFocusRight(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	assert.Equal(t, components.FocusLeft, v.splitPane.Focus())
	updated, _ := v.Update(sqlTableSelectedMsg{query: `SELECT * FROM "test" LIMIT 100`})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, components.FocusRight, sv.splitPane.Focus())
}

// --- 6. Query execution ---

func TestSQLBrowserView_XKeyExecutesWhenRightFocused(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.query = "SELECT 1"

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	sv := updated.(*SQLBrowserView)
	assert.NotNil(t, cmd, "x key with non-empty query should return exec command")
	assert.True(t, sv.editorPane.executing)
}

func TestSQLBrowserView_XKeyDoesNotExecuteWhenLeftFocused(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	// Focus is left by default.
	assert.Equal(t, components.FocusLeft, v.splitPane.Focus())

	v.editorPane.query = "SELECT 1"
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	// x when left-focused: does NOT execute (falls through to split pane).
	// The table pane ignores 'x', so cmd is nil.
	assert.Nil(t, cmd)
}

func TestSQLBrowserView_EmptyQueryNotExecuted(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.query = "   " // only whitespace

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	sv := updated.(*SQLBrowserView)
	assert.Nil(t, cmd, "empty/whitespace query should not trigger execution")
	assert.False(t, sv.editorPane.executing)
}

// --- 7. Query results ---

func TestSQLBrowserView_QueryResultMsgStored(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	result := &smithers.SQLResult{
		Columns: []string{"id", "name"},
		Rows:    [][]interface{}{{"1", "alice"}, {"2", "bob"}},
	}
	updated, _ := v.Update(sqlQueryResultMsg{result: result})
	sv := updated.(*SQLBrowserView)
	require.NotNil(t, sv.Result())
	assert.Len(t, sv.Result().Columns, 2)
	assert.Len(t, sv.Result().Rows, 2)
	assert.Nil(t, sv.ResultErr())
	assert.False(t, sv.editorPane.executing)
}

func TestSQLBrowserView_QueryErrorMsgStored(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v.editorPane.executing = true
	updated, _ := v.Update(sqlQueryErrorMsg{err: errors.New("syntax error")})
	sv := updated.(*SQLBrowserView)
	assert.NotNil(t, sv.ResultErr())
	assert.Contains(t, sv.ResultErr().Error(), "syntax error")
	assert.Nil(t, sv.Result())
	assert.False(t, sv.editorPane.executing)
}

func TestSQLBrowserView_ResultRenderedInView(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	result := &smithers.SQLResult{
		Columns: []string{"id", "email"},
		Rows:    [][]interface{}{{"42", "test@example.com"}},
	}
	updated, _ := v.Update(sqlQueryResultMsg{result: result})
	sv := updated.(*SQLBrowserView)
	out := sv.View()
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "email")
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "test@example.com")
}

func TestSQLBrowserView_ErrorRenderedInEditorView(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	updated, _ := v.Update(sqlQueryErrorMsg{err: errors.New("no such table")})
	sv := updated.(*SQLBrowserView)
	out := sv.View()
	assert.Contains(t, out, "no such table")
}

// --- 8. Query editor text input ---

func TestSQLBrowserView_TypingAppendsToQuery(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v) // focus editor

	v = sqlTypeChar(v, 'S')
	v = sqlTypeChar(v, 'E')
	v = sqlTypeChar(v, 'L')
	assert.Equal(t, "SEL", v.Query())
}

func TestSQLBrowserView_BackspaceRemovesLastChar(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'A')
	v = sqlTypeChar(v, 'B')
	v = sqlTypeChar(v, 'C')

	v, _ = sqlPressKey(v, tea.KeyBackspace)
	assert.Equal(t, "AB", v.Query())

	v, _ = sqlPressKey(v, tea.KeyBackspace)
	assert.Equal(t, "A", v.Query())
}

func TestSQLBrowserView_BackspaceOnEmptyQueryNoOp(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v, _ = sqlPressKey(v, tea.KeyBackspace)
	assert.Equal(t, "", v.Query())
}

// --- 9. Escape / pop view ---

func TestSQLBrowserView_EscapeEmitsPopViewMsg(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	_, cmd := sqlPressKey(v, tea.KeyEscape)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc must emit PopViewMsg, got %T", msg)
}

// --- 10. Refresh ---

func TestSQLBrowserView_RefreshSetsLoadingAndReturnsCmd(t *testing.T) {
	v := loadedSQL(sampleTables(3), 120, 40)
	assert.False(t, v.Loading())

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	sv := updated.(*SQLBrowserView)
	assert.True(t, sv.Loading())
	assert.NotNil(t, cmd)
}

// --- 11. Window resize ---

func TestSQLBrowserView_WindowResizePropagated(t *testing.T) {
	v := loadedSQL(sampleTables(2), 80, 24)
	updated, _ := v.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, 160, sv.width)
	assert.Equal(t, 50, sv.height)
}

// --- 12. ShortHelp ---

func TestSQLBrowserView_ShortHelpLeftFocus(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	assert.Equal(t, components.FocusLeft, v.splitPane.Focus())
	bindings := v.ShortHelp()
	assert.NotEmpty(t, bindings)
	keys := sqlHelpKeys(bindings)
	assert.Contains(t, keys, "↑↓/jk")
	assert.Contains(t, keys, "enter")
}

func TestSQLBrowserView_ShortHelpRightFocus(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	bindings := v.ShortHelp()
	keys := sqlHelpKeys(bindings)
	assert.Contains(t, keys, "ctrl+enter/x")
}

// --- 13. Helper functions ---

func TestQuoteTableName_Simple(t *testing.T) {
	assert.Equal(t, `"users"`, quoteTableName("users"))
}

func TestQuoteTableName_WithDoubleQuote(t *testing.T) {
	assert.Equal(t, `"ta""ble"`, quoteTableName(`ta"ble`))
}

func TestQuoteTableName_Underscore(t *testing.T) {
	assert.Equal(t, `"_smithers_runs"`, quoteTableName("_smithers_runs"))
}

func TestKeyToChar_PrintableASCII(t *testing.T) {
	assert.Equal(t, "a", keyToChar(tea.KeyPressMsg{Code: 'a', Text: "a"}))
	assert.Equal(t, "Z", keyToChar(tea.KeyPressMsg{Code: 'Z', Text: "Z"}))
	assert.Equal(t, " ", keyToChar(tea.KeyPressMsg{Code: ' ', Text: " "}))
	assert.Equal(t, "*", keyToChar(tea.KeyPressMsg{Code: '*', Text: "*"}))
}

func TestKeyToChar_ControlKey(t *testing.T) {
	// Ctrl+c: code='c' with Mod=ModCtrl → Text will be empty.
	assert.Equal(t, "", keyToChar(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}))
	assert.Equal(t, "", keyToChar(tea.KeyPressMsg{Code: tea.KeyEscape}))
	assert.Equal(t, "", keyToChar(tea.KeyPressMsg{Code: tea.KeyEnter}))
}

func TestKeyToChar_Empty(t *testing.T) {
	assert.Equal(t, "", keyToChar(tea.KeyPressMsg{}))
}

func TestWrapQueryLines_Short(t *testing.T) {
	lines := wrapQueryLines("SELECT 1", 80)
	require.Len(t, lines, 1)
	assert.Equal(t, "SELECT 1", lines[0])
}

func TestWrapQueryLines_LongWrapsAtSpace(t *testing.T) {
	q := "SELECT * FROM users WHERE id = 1 AND name = 'alice'"
	lines := wrapQueryLines(q, 20)
	assert.Greater(t, len(lines), 1)
	// All content preserved (joined).
	joined := strings.Join(lines, " ")
	assert.Contains(t, joined, "SELECT")
	assert.Contains(t, joined, "users")
}

func TestWrapQueryLines_NoSpaceHardCut(t *testing.T) {
	q := strings.Repeat("X", 50)
	lines := wrapQueryLines(q, 20)
	assert.Greater(t, len(lines), 1)
	for i, l := range lines {
		if i < len(lines)-1 {
			assert.LessOrEqual(t, len(l), 20)
		}
	}
}

func TestRenderSQLResultTable_Basic(t *testing.T) {
	result := &smithers.SQLResult{
		Columns: []string{"id", "name"},
		Rows: [][]interface{}{
			{"1", "alice"},
			{"2", "bob"},
		},
	}
	out := renderSQLResultTable(result, 80, 0)
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "alice")
	assert.Contains(t, out, "bob")
	assert.Contains(t, out, "2 rows")
}

func TestRenderSQLResultTable_SingleRow(t *testing.T) {
	result := &smithers.SQLResult{
		Columns: []string{"count"},
		Rows:    [][]interface{}{{42}},
	}
	out := renderSQLResultTable(result, 80, 0)
	assert.Contains(t, out, "1 row")
}

func TestRenderSQLResultTable_Empty(t *testing.T) {
	out := renderSQLResultTable(nil, 80, 0)
	assert.Equal(t, "", out)
}

func TestRenderSQLResultTable_NoColumns(t *testing.T) {
	result := &smithers.SQLResult{}
	out := renderSQLResultTable(result, 80, 0)
	assert.Equal(t, "", out)
}

// --- 14. Editor shows "No results yet" initially ---

func TestSQLBrowserView_EditorShowsNoResultsInitially(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	out := v.View()
	assert.Contains(t, out, "No results yet")
}

// --- 15. Executing state is shown ---

func TestSQLBrowserView_ExecutingStateRendered(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.executing = true
	out := v.View()
	assert.Contains(t, out, "Executing")
}

// --- 16. Registry includes SQL view ---

func TestDefaultRegistry_IncludesSQLView(t *testing.T) {
	r := DefaultRegistry()
	names := r.Names()
	assert.Contains(t, names, "sql")

	v, ok := r.Open("sql", nil)
	require.True(t, ok)
	assert.Equal(t, "sql", v.Name())
}

// --- 17. View renders RowCount hint ---

func TestSQLBrowserView_RowCountHintRendered(t *testing.T) {
	tables := []smithers.TableInfo{
		{Name: "users", Type: "table", RowCount: 42},
	}
	v := loadedSQL(tables, 120, 40)
	out := v.View()
	assert.Contains(t, out, "42 rows")
}

func TestSQLBrowserView_ViewTypeHintRendered(t *testing.T) {
	tables := []smithers.TableInfo{
		{Name: "active_runs", Type: "view", RowCount: 0},
	}
	v := loadedSQL(tables, 120, 40)
	out := v.View()
	assert.Contains(t, out, "(view)")
}

// --- 18. Interface compliance ---

func TestSQLBrowserView_ImplementsView(t *testing.T) {
	var _ View = (*SQLBrowserView)(nil)
}

// --- 19. Query cleared on new table selection ---

func TestSQLBrowserView_QueryClearedOnNewTableSelection(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	// Set an old result.
	v.editorPane.result = &smithers.SQLResult{Columns: []string{"x"}}

	updated, _ := v.Update(sqlTableSelectedMsg{query: `SELECT * FROM "new_table" LIMIT 100`})
	sv := updated.(*SQLBrowserView)
	assert.Nil(t, sv.Result(), "result should be cleared on new table selection")
	assert.Nil(t, sv.ResultErr())
}

// --- 20. SetSize propagates to split pane ---

func TestSQLBrowserView_SetSizePropagates(t *testing.T) {
	v := NewSQLBrowserView(nil)
	v.SetSize(200, 60)
	assert.Equal(t, 200, v.width)
	assert.Equal(t, 60, v.height)
	// splitPane should have received height-2 = 58.
	assert.Equal(t, 200, v.splitPane.Width())
	assert.Equal(t, 58, v.splitPane.Height())
}

// --- 21. Multi-line query editing (feat-sql-query-editor) ---

func TestSQLEditorPane_EnterInsertsNewline(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'S')
	v = sqlTypeChar(v, 'E')
	v = sqlTypeChar(v, 'L')

	// Press Enter in the editor pane (right pane focused).
	v, _ = sqlPressKey(v, tea.KeyEnter)
	assert.Equal(t, "SEL\n", v.Query())
	assert.Equal(t, 4, v.editorPane.cursor, "cursor should be after the newline")
}

func TestSQLEditorPane_MultilineQueryTyping(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	for _, ch := range "SELECT *" {
		v = sqlTypeChar(v, ch)
	}
	v, _ = sqlPressKey(v, tea.KeyEnter)
	for _, ch := range "FROM users" {
		v = sqlTypeChar(v, ch)
	}
	assert.Equal(t, "SELECT *\nFROM users", v.Query())
}

func TestSQLEditorPane_BackspaceDeletesNewline(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'A')
	v, _ = sqlPressKey(v, tea.KeyEnter)
	v = sqlTypeChar(v, 'B')
	assert.Equal(t, "A\nB", v.Query())

	v, _ = sqlPressKey(v, tea.KeyBackspace) // removes 'B'
	assert.Equal(t, "A\n", v.Query())

	v, _ = sqlPressKey(v, tea.KeyBackspace) // removes '\n'
	assert.Equal(t, "A", v.Query())
}

func TestSQLEditorPane_MultilineRenderedInView(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	for _, ch := range "SELECT *" {
		v = sqlTypeChar(v, ch)
	}
	v, _ = sqlPressKey(v, tea.KeyEnter)
	for _, ch := range "FROM users" {
		v = sqlTypeChar(v, ch)
	}
	out := v.View()
	assert.Contains(t, out, "SELECT *")
	assert.Contains(t, out, "FROM users")
}

// --- 22. Cursor movement within query (feat-sql-query-editor) ---

func TestSQLEditorPane_LeftMovesBackward(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'A')
	v = sqlTypeChar(v, 'B')
	v = sqlTypeChar(v, 'C')
	assert.Equal(t, 3, v.editorPane.cursor)

	v, _ = sqlPressKey(v, tea.KeyLeft)
	assert.Equal(t, 2, v.editorPane.cursor)

	v, _ = sqlPressKey(v, tea.KeyLeft)
	assert.Equal(t, 1, v.editorPane.cursor)
}

func TestSQLEditorPane_RightMovesForward(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'A')
	v = sqlTypeChar(v, 'B')
	v, _ = sqlPressKey(v, tea.KeyLeft) // cursor at 1
	v, _ = sqlPressKey(v, tea.KeyLeft) // cursor at 0
	assert.Equal(t, 0, v.editorPane.cursor)

	v, _ = sqlPressKey(v, tea.KeyRight)
	assert.Equal(t, 1, v.editorPane.cursor)
}

func TestSQLEditorPane_LeftClampedAtStart(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'X')
	v, _ = sqlPressKey(v, tea.KeyLeft) // cursor at 0
	v, _ = sqlPressKey(v, tea.KeyLeft) // should stay at 0
	assert.Equal(t, 0, v.editorPane.cursor)
}

func TestSQLEditorPane_RightClampedAtEnd(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'X')
	assert.Equal(t, 1, v.editorPane.cursor)
	v, _ = sqlPressKey(v, tea.KeyRight) // already at end, should stay
	assert.Equal(t, 1, v.editorPane.cursor)
}

func TestSQLEditorPane_InsertAtCursor(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v = sqlTypeChar(v, 'A')
	v = sqlTypeChar(v, 'C')
	// Move left and insert 'B' between A and C.
	v, _ = sqlPressKey(v, tea.KeyLeft)
	v = sqlTypeChar(v, 'B')
	assert.Equal(t, "ABC", v.Query())
}

// --- 23. Query history (feat-sql-query-editor) ---

func TestSQLEditorPane_HistoryPushedOnExecution(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.query = "SELECT 1"
	v.editorPane.cursor = len(v.editorPane.query)

	// Execute via 'x'.
	v, _ = sqlPressKey(v, 'x')
	require.Len(t, v.editorPane.history, 1)
	assert.Equal(t, "SELECT 1", v.editorPane.history[0])
}

func TestSQLEditorPane_UpArrowRecallsHistory(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.history = []string{"SELECT 1", "SELECT 2"}
	v.editorPane.query = ""

	// Up arrow: should show most recent history entry ("SELECT 2").
	v, _ = sqlPressKey(v, tea.KeyUp)
	assert.Equal(t, "SELECT 2", v.Query())
	assert.Equal(t, 1, v.editorPane.historyIndex)
}

func TestSQLEditorPane_UpArrowWalksBackThroughHistory(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.history = []string{"SELECT 1", "SELECT 2", "SELECT 3"}
	v.editorPane.query = ""

	v, _ = sqlPressKey(v, tea.KeyUp)
	assert.Equal(t, "SELECT 3", v.Query())

	v, _ = sqlPressKey(v, tea.KeyUp)
	assert.Equal(t, "SELECT 2", v.Query())

	v, _ = sqlPressKey(v, tea.KeyUp)
	assert.Equal(t, "SELECT 1", v.Query())

	// Already at oldest; up arrow should stay.
	v, _ = sqlPressKey(v, tea.KeyUp)
	assert.Equal(t, "SELECT 1", v.Query())
}

func TestSQLEditorPane_DownArrowRestoresDraft(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.history = []string{"SELECT 1", "SELECT 2"}
	v.editorPane.query = "draft query"
	v.editorPane.cursor = len(v.editorPane.query)

	v, _ = sqlPressKey(v, tea.KeyUp) // "SELECT 2"
	v, _ = sqlPressKey(v, tea.KeyDown) // back to draft
	assert.Equal(t, "draft query", v.Query())
	assert.Equal(t, -1, v.editorPane.historyIndex)
}

func TestSQLEditorPane_DownArrowWalksForwardInHistory(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.history = []string{"SELECT 1", "SELECT 2", "SELECT 3"}
	v.editorPane.query = ""

	// Walk all the way back.
	v, _ = sqlPressKey(v, tea.KeyUp) // "SELECT 3"
	v, _ = sqlPressKey(v, tea.KeyUp) // "SELECT 2"
	v, _ = sqlPressKey(v, tea.KeyUp) // "SELECT 1"

	// Now walk forward.
	v, _ = sqlPressKey(v, tea.KeyDown) // "SELECT 2"
	assert.Equal(t, "SELECT 2", v.Query())

	v, _ = sqlPressKey(v, tea.KeyDown) // "SELECT 3"
	assert.Equal(t, "SELECT 3", v.Query())
}

func TestSQLEditorPane_HistoryDeduplicatesConsecutive(t *testing.T) {
	p := &sqlEditorPane{}
	p.pushHistory("SELECT 1")
	p.pushHistory("SELECT 1") // duplicate
	assert.Len(t, p.history, 1)
}

func TestSQLEditorPane_HistoryCappedAt100(t *testing.T) {
	p := &sqlEditorPane{}
	for i := range 105 {
		p.pushHistory(fmt.Sprintf("SELECT %d", i))
	}
	assert.LessOrEqual(t, len(p.history), 100)
}

func TestSQLEditorPane_HistoryHintShownInView(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.history = []string{"SELECT 1", "SELECT 2"}
	v.editorPane.query = ""

	v, _ = sqlPressKey(v, tea.KeyUp) // enters history
	out := v.View()
	assert.Contains(t, out, "hist")
}

func TestSQLEditorPane_HistoryIndexResetOnExecution(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.history = []string{"SELECT 1"}
	v.editorPane.query = "SELECT 2"
	v.editorPane.cursor = len(v.editorPane.query)

	// Execute.
	v, _ = sqlPressKey(v, 'x')
	assert.Equal(t, -1, v.editorPane.historyIndex, "historyIndex reset to -1 after execution")
}

// --- 24. Schema sidebar (feat-sql-table-sidebar) ---

func TestSQLTablePane_SchemaLoadedMsgUpdatesEntry(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	schema := &smithers.TableSchema{
		TableName: "_smithers_table_01",
		Columns: []smithers.Column{
			{CID: 0, Name: "id", Type: "INTEGER", PrimaryKey: true, NotNull: true},
			{CID: 1, Name: "name", Type: "TEXT", NotNull: false},
		},
	}
	updated, _ := v.Update(sqlSchemaLoadedMsg{tableName: "_smithers_table_01", schema: schema})
	sv := updated.(*SQLBrowserView)
	entry := sv.tablePane.entries[0]
	require.NotNil(t, entry.schema)
	assert.Equal(t, 2, len(entry.schema.Columns))
	assert.False(t, entry.loading)
}

func TestSQLTablePane_SchemaErrorMsgClearsLoading(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	v.tablePane.entries[0].loading = true
	v.tablePane.entries[0].expanded = true

	updated, _ := v.Update(sqlSchemaErrorMsg{tableName: "_smithers_table_01", err: errors.New("PRAGMA failed")})
	sv := updated.(*SQLBrowserView)
	entry := sv.tablePane.entries[0]
	assert.False(t, entry.loading)
	assert.Nil(t, entry.schema)
	assert.True(t, entry.expanded, "entry stays expanded even if schema fetch failed")
}

func TestSQLTablePane_ExpandedEntryShowsColumns(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	v.tablePane.entries[0].expanded = true
	v.tablePane.entries[0].schema = &smithers.TableSchema{
		TableName: "_smithers_table_01",
		Columns: []smithers.Column{
			{Name: "id", Type: "INTEGER", PrimaryKey: true, NotNull: true},
			{Name: "email", Type: "TEXT"},
		},
	}
	out := v.View()
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "INTEGER")
	assert.Contains(t, out, "email")
}

func TestSQLTablePane_ExpandedEntryShowsLoadingWhileFetching(t *testing.T) {
	v := loadedSQL(sampleTables(2), 120, 40)
	v.tablePane.entries[0].expanded = true
	v.tablePane.entries[0].loading = true
	out := v.View()
	assert.Contains(t, out, "loading schema")
}

func TestSQLTablePane_ExpandedPKAndNotNullAnnotations(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v.tablePane.entries[0].expanded = true
	v.tablePane.entries[0].schema = &smithers.TableSchema{
		TableName: "_smithers_table_01",
		Columns: []smithers.Column{
			{Name: "id", Type: "INTEGER", PrimaryKey: true, NotNull: true},
			{Name: "val", Type: "REAL"},
		},
	}
	out := v.View()
	assert.Contains(t, out, "PK")
	assert.Contains(t, out, "NOT NULL")
}

func TestRenderColumnLine_Basic(t *testing.T) {
	col := smithers.Column{Name: "user_id", Type: "INTEGER", PrimaryKey: true, NotNull: true}
	line := renderColumnLine(col, 80)
	assert.Contains(t, line, "user_id")
	assert.Contains(t, line, "INTEGER")
	assert.Contains(t, line, "PK")
	assert.Contains(t, line, "NOT NULL")
}

func TestRenderColumnLine_NullableNoAnnotations(t *testing.T) {
	col := smithers.Column{Name: "description", Type: "TEXT"}
	line := renderColumnLine(col, 80)
	assert.Contains(t, line, "description")
	assert.NotContains(t, line, "PK")
	assert.NotContains(t, line, "NOT NULL")
}

func TestRenderColumnLine_EmptyTypeFallback(t *testing.T) {
	col := smithers.Column{Name: "mystery"}
	line := renderColumnLine(col, 80)
	assert.Contains(t, line, "?")
}

func TestSQLTablePane_FetchSchemaMsgRoutedToClient(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	// Directly update with sqlFetchSchemaMsg — when client is nil, fetchSchemaCmd
	// returns a cmd that would panic if called; we just verify that the view
	// returns a non-nil cmd (meaning it accepted the message).
	_, cmd := v.Update(sqlFetchSchemaMsg{tableName: "_smithers_table_01"})
	assert.NotNil(t, cmd, "sqlFetchSchemaMsg should produce a fetch command")
}

// --- 25. ShortHelp includes history hint when right-focused ---

func TestSQLBrowserView_ShortHelpRightFocusIncludesHistory(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	bindings := v.ShortHelp()
	keys := sqlHelpKeys(bindings)
	assert.Contains(t, keys, "↑↓", "right-pane help should include history navigation")
}

// --- 26. Left pane Enter hint updated to expand/collapse ---

func TestSQLBrowserView_ShortHelpLeftFocusExpandCollapse(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	bindings := v.ShortHelp()
	keys := sqlHelpKeys(bindings)
	assert.Contains(t, keys, "enter")
	// Verify the help text mentions expand/collapse.
	for _, b := range bindings {
		if b.Help().Key == "enter" {
			assert.Contains(t, b.Help().Desc, "expand")
		}
	}
}

// ============================================================
// feat-sql-results-table: horizontal scroll support
// ============================================================

// --- 27. renderSQLResultTable with colOffset=0 shows all columns when they fit ---

func TestRenderSQLResultTable_ColOffset0_ShowsAllColumns(t *testing.T) {
	result := &smithers.SQLResult{
		Columns: []string{"id", "name", "status"},
		Rows: [][]interface{}{
			{1, "Alice", "active"},
		},
	}
	out := renderSQLResultTable(result, 200, 0)
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "status")
	assert.Contains(t, out, "Alice")
}

// --- 28. renderSQLResultTable with colOffset skips leading columns ---

func TestRenderSQLResultTable_ColOffset_SkipsLeadingColumns(t *testing.T) {
	result := &smithers.SQLResult{
		Columns: []string{"id", "name", "status"},
		Rows: [][]interface{}{
			{1, "Alice", "active"},
		},
	}
	// Start at column 1 (skip "id").
	out := renderSQLResultTable(result, 200, 1)
	assert.NotContains(t, out, "  id", "id column should be scrolled off")
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "status")
}

// --- 29. renderSQLResultTable with hasMore shows ▶ indicator ---

func TestRenderSQLResultTable_HasMore_ShowsIndicator(t *testing.T) {
	result := &smithers.SQLResult{
		// Long column names so they cannot all fit in 20 chars.
		Columns: []string{"column_alpha", "column_beta", "column_gamma"},
		Rows:    [][]interface{}{{1, 2, 3}},
	}
	// maxWidth=20 forces clipping: "  " indent + one 8-char cap column + sep fills ~12 chars,
	// leaving no room for a second column, so ▶ should appear.
	out := renderSQLResultTable(result, 20, 0)
	assert.Contains(t, out, "▶", "should show ▶ when more columns exist to the right")
}

// --- 30. renderSQLResultTable clamps negative colOffset to 0 ---

func TestRenderSQLResultTable_NegativeOffset_ClampedTo0(t *testing.T) {
	result := &smithers.SQLResult{
		Columns: []string{"id", "name"},
		Rows:    [][]interface{}{{1, "Alice"}},
	}
	out := renderSQLResultTable(result, 200, -5)
	assert.Contains(t, out, "id", "negative offset should be clamped to 0")
	assert.Contains(t, out, "name")
}

// --- 31. renderSQLResultTable clamps colOffset >= len(columns) ---

func TestRenderSQLResultTable_OffsetBeyondMax_Clamped(t *testing.T) {
	result := &smithers.SQLResult{
		Columns: []string{"id", "name"},
		Rows:    [][]interface{}{{1, "Alice"}},
	}
	out := renderSQLResultTable(result, 200, 99)
	// Should show at least the last column without panicking.
	assert.NotEmpty(t, out)
}

// --- 32. '>' key scrolls results right ---

func TestSQLBrowserView_GreaterThan_ScrollsRight(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	// Inject a multi-column result.
	v.editorPane.result = &smithers.SQLResult{
		Columns: []string{"col1", "col2", "col3"},
		Rows:    [][]interface{}{{1, 2, 3}},
	}
	v.editorPane.resultColOffset = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: '>', Text: ">"})
	sv := updated.(*SQLBrowserView)
	assert.Nil(t, cmd)
	assert.Equal(t, 1, sv.editorPane.resultColOffset, "'>' should increment col offset")
}

// --- 33. '<' key scrolls results left ---

func TestSQLBrowserView_LessThan_ScrollsLeft(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.result = &smithers.SQLResult{
		Columns: []string{"col1", "col2", "col3"},
		Rows:    [][]interface{}{{1, 2, 3}},
	}
	v.editorPane.resultColOffset = 2

	updated, cmd := v.Update(tea.KeyPressMsg{Code: '<', Text: "<"})
	sv := updated.(*SQLBrowserView)
	assert.Nil(t, cmd)
	assert.Equal(t, 1, sv.editorPane.resultColOffset, "'<' should decrement col offset")
}

// --- 34. '<' key does not scroll below 0 ---

func TestSQLBrowserView_LessThan_ClampsAt0(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.result = &smithers.SQLResult{
		Columns: []string{"col1", "col2"},
		Rows:    [][]interface{}{{1, 2}},
	}
	v.editorPane.resultColOffset = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: '<', Text: "<"})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, 0, sv.editorPane.resultColOffset, "'<' at offset 0 should not go negative")
}

// --- 35. '>' key does not scroll past last column ---

func TestSQLBrowserView_GreaterThan_ClampsAtMax(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.result = &smithers.SQLResult{
		Columns: []string{"col1", "col2"},
		Rows:    [][]interface{}{{1, 2}},
	}
	v.editorPane.resultColOffset = 1 // last index

	updated, _ := v.Update(tea.KeyPressMsg{Code: '>', Text: ">"})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, 1, sv.editorPane.resultColOffset, "'>' at max offset should not exceed last column")
}

// --- 36. New query result resets colOffset ---

func TestSQLBrowserView_NewResult_ResetsColOffset(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v.editorPane.resultColOffset = 3

	newResult := &smithers.SQLResult{
		Columns: []string{"a", "b"},
		Rows:    [][]interface{}{{1, 2}},
	}
	updated, _ := v.Update(sqlQueryResultMsg{result: newResult})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, 0, sv.editorPane.resultColOffset, "new result should reset colOffset to 0")
}

// --- 37. Query error resets colOffset ---

func TestSQLBrowserView_QueryError_ResetsColOffset(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v.editorPane.resultColOffset = 2

	updated, _ := v.Update(sqlQueryErrorMsg{err: errors.New("syntax error")})
	sv := updated.(*SQLBrowserView)
	assert.Equal(t, 0, sv.editorPane.resultColOffset, "query error should reset colOffset to 0")
}

// --- 38. ResultColOffset accessor ---

func TestSQLBrowserView_ResultColOffset_Accessor(t *testing.T) {
	v := NewSQLBrowserView(nil)
	assert.Equal(t, 0, v.ResultColOffset(), "initial ResultColOffset should be 0")
	v.editorPane.resultColOffset = 5
	assert.Equal(t, 5, v.ResultColOffset())
}

// --- 39. View shows scroll hint when multiple columns exist ---

func TestSQLBrowserView_View_ShowsScrollHintForMultipleColumns(t *testing.T) {
	v := loadedSQL(sampleTables(1), 120, 40)
	v = sqlFocusRight(v)
	v.editorPane.result = &smithers.SQLResult{
		Columns: []string{"col1", "col2", "col3"},
		Rows:    [][]interface{}{{1, 2, 3}},
	}
	v.editorPane.resultColOffset = 0

	out := v.View()
	assert.Contains(t, out, "col 1/3", "view should show column position hint")
}
