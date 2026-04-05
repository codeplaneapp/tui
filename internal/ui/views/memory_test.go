package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func testMemoryFacts() []smithers.MemoryFact {
	now := time.Now().UnixMilli()
	return []smithers.MemoryFact{
		{
			Namespace:   "workflow:code-review",
			Key:         "reviewer-preference",
			ValueJSON:   `{"style":"thorough"}`,
			UpdatedAtMs: now - 120_000,
		},
		{
			Namespace:   "global",
			Key:         "last-deploy-sha",
			ValueJSON:   `"a1b2c3d"`,
			UpdatedAtMs: now - 3_600_000,
		},
		{
			Namespace:   "agent:claude-code",
			Key:         "task-context",
			ValueJSON:   `{"task":"review"}`,
			UpdatedAtMs: now - 7_200_000,
		},
	}
}

func newTestMemoryView() *MemoryView {
	c := smithers.NewClient()
	return NewMemoryView(c)
}

func seedMemoryFacts(v *MemoryView, facts []smithers.MemoryFact) *MemoryView {
	updated, _ := v.Update(memoryLoadedMsg{facts: facts})
	return updated.(*MemoryView)
}

// --- Interface compliance ---

func TestMemoryView_ImplementsView(t *testing.T) {
	var _ View = (*MemoryView)(nil)
}

// --- Init ---

func TestMemoryView_Init(t *testing.T) {
	v := newTestMemoryView()
	assert.True(t, v.loading, "should start in loading state")
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil command")
}

// --- Update: loaded/error messages ---

func TestMemoryView_LoadedMsg(t *testing.T) {
	v := newTestMemoryView()
	facts := testMemoryFacts()[:2]
	updated, cmd := v.Update(memoryLoadedMsg{facts: facts})
	assert.Nil(t, cmd)

	mv := updated.(*MemoryView)
	assert.False(t, mv.loading)
	assert.Len(t, mv.facts, 2)
	assert.Nil(t, mv.err)

	// Both fact keys should appear in the rendered output.
	out := mv.View()
	assert.Contains(t, out, "reviewer-preference")
	assert.Contains(t, out, "last-deploy-sha")
}

func TestMemoryView_ErrorMsg(t *testing.T) {
	v := newTestMemoryView()
	dbErr := errors.New("db unavailable")
	updated, cmd := v.Update(memoryErrorMsg{err: dbErr})
	assert.Nil(t, cmd)

	mv := updated.(*MemoryView)
	assert.False(t, mv.loading)
	assert.NotNil(t, mv.err)

	out := mv.View()
	assert.Contains(t, out, "Error:")
	assert.Contains(t, out, "db unavailable")
}

func TestMemoryView_EmptyState(t *testing.T) {
	v := newTestMemoryView()
	updated, _ := v.Update(memoryLoadedMsg{facts: nil})
	mv := updated.(*MemoryView)

	out := mv.View()
	assert.Contains(t, out, "No memory facts found.")
}

// --- Update: cursor navigation ---

func TestMemoryView_CursorNavigation(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	assert.Equal(t, 0, v.cursor)

	// Move down twice.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	mv := updated.(*MemoryView)
	assert.Equal(t, 1, mv.cursor, "j should move cursor down")

	updated2, _ := mv.Update(tea.KeyPressMsg{Code: 'j'})
	mv2 := updated2.(*MemoryView)
	assert.Equal(t, 2, mv2.cursor)

	// Move up once.
	updated3, _ := mv2.Update(tea.KeyPressMsg{Code: 'k'})
	mv3 := updated3.(*MemoryView)
	assert.Equal(t, 1, mv3.cursor, "k should move cursor up")
}

func TestMemoryView_CursorClampsAtTop(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.cursor = 0

	// Pressing up at top should stay at 0.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	mv := updated.(*MemoryView)
	assert.Equal(t, 0, mv.cursor, "cursor should not go below zero")
}

func TestMemoryView_CursorClampsAtBottom(t *testing.T) {
	v := newTestMemoryView()
	facts := testMemoryFacts()[:2]
	v = seedMemoryFacts(v, facts)

	// Press down three times on a 2-item list — should clamp at 1.
	view := View(v)
	for range 3 {
		view, _ = view.Update(tea.KeyPressMsg{Code: 'j'})
	}
	mv := view.(*MemoryView)
	assert.Equal(t, 1, mv.cursor, "cursor should clamp at len-1")
}

func TestMemoryView_ArrowKeys_Navigate(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mv := updated.(*MemoryView)
	assert.Equal(t, 1, mv.cursor)

	updated2, _ := mv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mv2 := updated2.(*MemoryView)
	assert.Equal(t, 0, mv2.cursor)
}

// --- Update: Esc key ---

func TestMemoryView_EscPopView(t *testing.T) {
	v := newTestMemoryView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "Esc should return a non-nil command")

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

// --- Update: r (refresh) ---

func TestMemoryView_Refresh(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	assert.False(t, v.loading)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	mv := updated.(*MemoryView)
	assert.True(t, mv.loading, "'r' should set loading = true")
	assert.NotNil(t, cmd, "'r' should return a reload command")
}

// --- Update: window resize ---

func TestMemoryView_WindowSize(t *testing.T) {
	v := newTestMemoryView()
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Nil(t, cmd)

	mv := updated.(*MemoryView)
	assert.Equal(t, 120, mv.width)
	assert.Equal(t, 40, mv.height)
}

// --- Name / SetSize / ShortHelp ---

func TestMemoryView_Name(t *testing.T) {
	v := newTestMemoryView()
	assert.Equal(t, "memory", v.Name())
}

func TestMemoryView_SetSize(t *testing.T) {
	v := newTestMemoryView()
	v.SetSize(100, 50)
	assert.Equal(t, 100, v.width)
	assert.Equal(t, 50, v.height)
}

func TestMemoryView_ShortHelp_NotEmpty(t *testing.T) {
	v := newTestMemoryView()
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	var allDesc []string
	for _, b := range help {
		allDesc = append(allDesc, b.Help().Desc)
	}
	joined := strings.Join(allDesc, " ")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")
}

// --- View() rendering ---

func TestMemoryView_View_HeaderText(t *testing.T) {
	v := newTestMemoryView()
	v.width = 80
	v.height = 24
	out := v.View()
	assert.Contains(t, out, "SMITHERS")
	assert.Contains(t, out, "Memory")
}

func TestMemoryView_View_LoadingState(t *testing.T) {
	v := newTestMemoryView()
	v.width = 80
	out := v.View()
	assert.Contains(t, out, "Loading memory facts...")
}

func TestMemoryView_View_ErrorState(t *testing.T) {
	v := newTestMemoryView()
	v.loading = false
	v.err = errors.New("connection refused")
	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "connection refused")
}

func TestMemoryView_View_EmptyState(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, []smithers.MemoryFact{})
	out := v.View()
	assert.Contains(t, out, "No memory facts found.")
}

func TestMemoryView_View_CursorIndicator(t *testing.T) {
	v := newTestMemoryView()
	v.width = 80
	v = seedMemoryFacts(v, testMemoryFacts())
	v.cursor = 0
	out := v.View()
	assert.Contains(t, out, "\u25b8")
}

func TestMemoryView_View_ShowsNamespaceAndKey(t *testing.T) {
	v := newTestMemoryView()
	v.width = 80
	v = seedMemoryFacts(v, testMemoryFacts())
	out := v.View()
	assert.Contains(t, out, "workflow:code-review")
	assert.Contains(t, out, "reviewer-preference")
}

// --- factValuePreview helper ---

func TestFactValuePreview_StringLiteral_StripsQuotes(t *testing.T) {
	result := factValuePreview(`"hello"`, 60)
	assert.Equal(t, "hello", result, "outer quotes should be stripped from JSON string literals")
}

func TestFactValuePreview_ShortString(t *testing.T) {
	result := factValuePreview(`"hi"`, 60)
	assert.Equal(t, "hi", result)
}

func TestFactValuePreview_LongObject_Truncated(t *testing.T) {
	// Build a long JSON object value.
	long := `{"key": "value", "other": "data", "foo": "bar", "baz": "qux", "more": "stuff", "end": "here"}`
	result := factValuePreview(long, 60)
	assert.True(t, strings.HasSuffix(result, "..."), "long JSON should be truncated with ...")
	runes := []rune(result)
	assert.LessOrEqual(t, len(runes), 60, "result should be at most maxLen runes")
}

func TestFactValuePreview_ExactBoundary(t *testing.T) {
	// 63 rune input, maxLen=60 → should truncate to 60 runes (57 + "...").
	input := strings.Repeat("x", 63)
	result := factValuePreview(input, 60)
	runes := []rune(result)
	assert.Len(t, runes, 60)
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestFactValuePreview_Empty(t *testing.T) {
	result := factValuePreview("", 60)
	assert.Equal(t, "", result)
}

func TestFactValuePreview_NoTruncation_WhenShort(t *testing.T) {
	input := `{"x":1}`
	result := factValuePreview(input, 60)
	assert.Equal(t, input, result, "short JSON objects should not be truncated")
}

// --- factAge helper ---

func TestFactAge_Seconds(t *testing.T) {
	ts := time.Now().Add(-30 * time.Second).UnixMilli()
	result := factAge(ts)
	assert.Equal(t, "30s ago", result)
}

func TestFactAge_Minutes(t *testing.T) {
	ts := time.Now().Add(-5 * time.Minute).UnixMilli()
	result := factAge(ts)
	assert.Equal(t, "5m ago", result)
}

func TestFactAge_Hours(t *testing.T) {
	ts := time.Now().Add(-3 * time.Hour).UnixMilli()
	result := factAge(ts)
	assert.Equal(t, "3h ago", result)
}

func TestFactAge_Days(t *testing.T) {
	ts := time.Now().Add(-48 * time.Hour).UnixMilli()
	result := factAge(ts)
	assert.Equal(t, "2d ago", result)
}

func TestFactAge_Zero(t *testing.T) {
	result := factAge(0)
	assert.Equal(t, "", result)
}

func TestFactAge_Negative(t *testing.T) {
	result := factAge(-1)
	assert.Equal(t, "", result)
}

// --- Detail pane (enter key) ---

func TestMemoryView_EnterOpensDetailMode(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeDetail, mv.mode, "enter should switch to detail mode")
}

func TestMemoryView_DetailPaneShowsFullValue(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.cursor = 0

	// Enter detail mode.
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mv := updated.(*MemoryView)

	out := mv.View()
	assert.Contains(t, out, "Detail", "detail header should mention Detail")
	// The full JSON value should appear (not just the preview).
	assert.Contains(t, out, "reviewer-preference", "should show selected key")
	assert.Contains(t, out, "thorough", "should show full value content")
}

func TestMemoryView_DetailPaneEscReturnsToList(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.mode = memoryModeDetail

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeList, mv.mode, "esc in detail mode should return to list")
}

func TestMemoryView_DetailPaneQReturnsToList(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.mode = memoryModeDetail

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'q'})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeList, mv.mode, "'q' in detail mode should return to list")
}

func TestMemoryView_DetailPaneShowsTimestamps(t *testing.T) {
	v := newTestMemoryView()
	facts := []smithers.MemoryFact{
		{
			Namespace:   "global",
			Key:         "test-key",
			ValueJSON:   `"hello world"`,
			CreatedAtMs: time.Now().Add(-24 * time.Hour).UnixMilli(),
			UpdatedAtMs: time.Now().Add(-30 * time.Minute).UnixMilli(),
		},
	}
	v = seedMemoryFacts(v, facts)
	v.cursor = 0
	v.mode = memoryModeDetail

	out := v.View()
	assert.Contains(t, out, "Updated:", "should show updated timestamp")
	assert.Contains(t, out, "Created:", "should show created timestamp")
}

func TestMemoryView_DetailPaneEmptyWhenNoSelection(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.mode = memoryModeDetail
	v.cursor = 999 // out of bounds

	out := v.View()
	assert.Contains(t, out, "No fact selected.")
}

func TestMemoryView_EnterNoOpWhenEmpty(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, []smithers.MemoryFact{})

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mv := updated.(*MemoryView)
	// With no facts, enter should not switch to detail mode.
	assert.Equal(t, memoryModeList, mv.mode, "enter with no facts should stay in list mode")
}

// --- Semantic recall ('s' key) ---

func TestMemoryView_SKeyOpensRecallPrompt(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())

	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeRecall, mv.mode, "'s' should open recall prompt")
}

func TestMemoryView_RecallPromptRendersCorrectly(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.mode = memoryModeRecall
	v.recallQuery = "test query"

	out := v.View()
	assert.Contains(t, out, "Recall", "recall header should mention Recall")
	assert.Contains(t, out, "test query", "should show current query")
}

func TestMemoryView_RecallPromptBuildsQueryOnKeypress(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallQuery = ""

	// Type individual characters; Code with a rune acts as a character press.
	for _, ch := range []rune{'f', 'o', 'o'} {
		updated, _ := v.Update(tea.KeyPressMsg{Code: ch})
		v = updated.(*MemoryView)
	}
	assert.Equal(t, "foo", v.recallQuery, "typing should accumulate into recallQuery")
}

func TestMemoryView_RecallPromptBackspaceDeletesChar(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallQuery = "hello"

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	mv := updated.(*MemoryView)
	assert.Equal(t, "hell", mv.recallQuery, "backspace should delete last char")
}

func TestMemoryView_RecallPromptEscCancels(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallQuery = "test"

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeList, mv.mode, "esc in recall mode should return to list")
}

func TestMemoryView_RecallPromptEnterWithEmptyQueryCancels(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallQuery = "   " // whitespace only

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mv := updated.(*MemoryView)
	// Empty query → return to list without issuing recall.
	assert.Equal(t, memoryModeList, mv.mode, "empty query enter should return to list")
}

func TestMemoryView_RecallPromptEnterWithQueryIssuesCmd(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallQuery = "important context"

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mv := updated.(*MemoryView)
	assert.True(t, mv.recallLoading, "entering a query should set recallLoading")
	assert.NotNil(t, cmd, "entering a query should return a recall command")
}

func TestMemoryView_RecallResultMsgSetsResults(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallLoading = true

	results := []smithers.MemoryRecallResult{
		{Score: 0.92, Content: "The deployment was on Friday at 3pm."},
		{Score: 0.85, Content: "Release SHA abc123 was tagged."},
	}
	updated, _ := v.Update(memoryRecallResultMsg{results: results})
	mv := updated.(*MemoryView)

	assert.Equal(t, memoryModeResults, mv.mode, "recall result should switch to results mode")
	assert.False(t, mv.recallLoading)
	assert.Len(t, mv.recallResults, 2)
}

func TestMemoryView_RecallErrorMsgSetsError(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	v.recallLoading = true

	updated, _ := v.Update(memoryRecallErrorMsg{err: errors.New("vector service unavailable")})
	mv := updated.(*MemoryView)

	assert.Equal(t, memoryModeResults, mv.mode, "recall error should switch to results mode")
	assert.False(t, mv.recallLoading)
	require.NotNil(t, mv.recallErr)
	assert.Contains(t, mv.recallErr.Error(), "vector service unavailable")
}

func TestMemoryView_RecallResultsViewRendersResults(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeResults
	v.recallQuery = "deployment info"
	v.recallResults = []smithers.MemoryRecallResult{
		{Score: 0.93, Content: "Deploy happened on Friday."},
	}

	out := v.View()
	assert.Contains(t, out, "Recall Results")
	assert.Contains(t, out, "0.930")
	assert.Contains(t, out, "Deploy happened on Friday.")
}

func TestMemoryView_RecallResultsViewRendersError(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeResults
	v.recallErr = errors.New("timeout connecting to vector store")

	out := v.View()
	assert.Contains(t, out, "Error:")
	assert.Contains(t, out, "timeout connecting to vector store")
}

func TestMemoryView_RecallResultsViewNoResults(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeResults
	v.recallQuery = "xyzzy"
	v.recallResults = nil

	out := v.View()
	assert.Contains(t, out, "No results found.")
}

func TestMemoryView_RecallResultsEscReturnsToList(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeResults

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeList, mv.mode, "esc in results mode should return to list")
}

func TestMemoryView_RecallResultsSNewQuery(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeResults
	v.recallResults = []smithers.MemoryRecallResult{{Score: 0.9, Content: "x"}}

	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	mv := updated.(*MemoryView)
	assert.Equal(t, memoryModeRecall, mv.mode, "'s' in results mode should open new recall prompt")
	assert.Empty(t, mv.recallQuery, "new query should be empty")
	assert.Nil(t, mv.recallResults, "previous results should be cleared")
}

// --- Namespace filtering ('n' key) ---

func TestMemoryView_NKeyFiltersByFirstNamespace(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	// testMemoryFacts has 3 namespaces: "agent:claude-code", "global", "workflow:code-review"
	// sorted alphabetically.
	assert.Equal(t, "", v.activeNamespace, "starts with no filter")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	mv := updated.(*MemoryView)
	assert.NotEmpty(t, mv.activeNamespace, "'n' should activate first namespace filter")
}

func TestMemoryView_NKeyCyclesThroughNamespacesAndWraps(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())

	namespaces := extractNamespaces(testMemoryFacts())
	view := View(v)

	// Cycle through all namespaces.
	for _, ns := range namespaces {
		view, _ = view.Update(tea.KeyPressMsg{Code: 'n'})
		mv := view.(*MemoryView)
		assert.Equal(t, ns, mv.activeNamespace)
	}

	// One more press wraps back to "all".
	view, _ = view.Update(tea.KeyPressMsg{Code: 'n'})
	mv := view.(*MemoryView)
	assert.Equal(t, "", mv.activeNamespace, "cycling past last ns should reset to all")
}

func TestMemoryView_FilterResetsCursorOnChange(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.cursor = 2

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	mv := updated.(*MemoryView)
	assert.Equal(t, 0, mv.cursor, "namespace filter change should reset cursor to 0")
}

func TestMemoryView_FilteredFactsRespectNamespace(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.activeNamespace = "global"

	filtered := v.filteredFacts()
	assert.Len(t, filtered, 1)
	assert.Equal(t, "global", filtered[0].Namespace)
}

func TestMemoryView_FilteredFactsAllWhenEmpty(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.activeNamespace = ""

	filtered := v.filteredFacts()
	assert.Len(t, filtered, 3, "empty filter should return all facts")
}

func TestMemoryView_ListShowsActiveNamespaceTag(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.activeNamespace = "global"

	out := v.View()
	assert.Contains(t, out, "[global]", "header should show active namespace filter")
}

func TestMemoryView_ListShowsEmptyNamespaceState(t *testing.T) {
	v := newTestMemoryView()
	v = seedMemoryFacts(v, testMemoryFacts())
	v.activeNamespace = "nonexistent-namespace"

	out := v.View()
	assert.Contains(t, out, "nonexistent-namespace")
}

// --- extractNamespaces helper ---

func TestExtractNamespaces_Deduplicates(t *testing.T) {
	facts := []smithers.MemoryFact{
		{Namespace: "a"}, {Namespace: "b"}, {Namespace: "a"}, {Namespace: "c"},
	}
	ns := extractNamespaces(facts)
	assert.Len(t, ns, 3)
	assert.Equal(t, []string{"a", "b", "c"}, ns)
}

func TestExtractNamespaces_Empty(t *testing.T) {
	ns := extractNamespaces(nil)
	assert.Empty(t, ns)
}

// --- formatFactValue helper ---

func TestFormatFactValue_PrettyPrintsJSON(t *testing.T) {
	result := formatFactValue(`{"key":"value","n":42}`, 80)
	assert.Contains(t, result, "key")
	assert.Contains(t, result, "value")
}

func TestFormatFactValue_JSONStringStripped(t *testing.T) {
	result := formatFactValue(`"plain string"`, 80)
	// JSON strings should be pretty-printed (they unmarshal to a string).
	assert.Contains(t, result, "plain string")
}

func TestFormatFactValue_EmptyValue(t *testing.T) {
	result := formatFactValue("", 80)
	assert.Contains(t, result, "(empty)")
}

func TestFormatFactValue_FallsBackForInvalidJSON(t *testing.T) {
	result := formatFactValue("not json at all", 80)
	assert.Contains(t, result, "not json at all")
}

// --- ShortHelp changes by mode ---

func TestMemoryView_ShortHelp_ListMode(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeList
	help := v.ShortHelp()
	descs := collectHelpDescs(help)
	assert.Contains(t, descs, "recall")
	assert.Contains(t, descs, "filter ns")
	assert.Contains(t, descs, "refresh")
	assert.Contains(t, descs, "back")
}

func TestMemoryView_ShortHelp_DetailMode(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeDetail
	help := v.ShortHelp()
	descs := collectHelpDescs(help)
	assert.Contains(t, descs, "recall")
	assert.Contains(t, descs, "back")
}

func TestMemoryView_ShortHelp_RecallMode(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeRecall
	help := v.ShortHelp()
	descs := collectHelpDescs(help)
	assert.Contains(t, descs, "search")
	assert.Contains(t, descs, "cancel")
}

func TestMemoryView_ShortHelp_ResultsMode(t *testing.T) {
	v := newTestMemoryView()
	v.mode = memoryModeResults
	help := v.ShortHelp()
	descs := collectHelpDescs(help)
	assert.Contains(t, descs, "new query")
	assert.Contains(t, descs, "back")
}

// collectHelpDescs joins all Help().Desc strings from bindings.
func collectHelpDescs(bindings []key.Binding) string {
	var all []string
	for _, b := range bindings {
		all = append(all, b.Help().Desc)
	}
	return strings.Join(all, " ")
}
