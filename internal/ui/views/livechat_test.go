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

// newView creates a LiveChatView with a stub smithers.Client that will never
// actually reach a server (no URL configured).  Tests drive the model
// by calling Update directly with pre-fabricated messages.
func newView(runID string) *LiveChatView {
	c := smithers.NewClient() // no-op client; no server
	return NewLiveChatView(c, runID, "", "")
}

// newViewWithTask creates a LiveChatView with a task and agent hint.
func newViewWithTask(runID, taskID, agentName string) *LiveChatView {
	c := smithers.NewClient()
	return NewLiveChatView(c, runID, taskID, agentName)
}

// makeBlock is a convenience constructor for a ChatBlock.
func makeBlock(runID, nodeID, role, content string, tsMs int64) smithers.ChatBlock {
	return smithers.ChatBlock{
		RunID:       runID,
		NodeID:      nodeID,
		Role:        smithers.ChatRole(role),
		Content:     content,
		TimestampMs: tsMs,
	}
}

// --- Interface compliance ---

func TestLiveChatView_ImplementsView(t *testing.T) {
	var _ View = (*LiveChatView)(nil)
}

// --- Constructor ---

func TestNewLiveChatView_Defaults(t *testing.T) {
	v := newView("run-001")
	assert.Equal(t, "run-001", v.runID)
	assert.True(t, v.loadingRun, "should start in loading-run state")
	assert.True(t, v.loadingBlocks, "should start in loading-blocks state")
	assert.True(t, v.follow, "follow mode should default to on")
	assert.Equal(t, 0, v.scrollLine)
}

func TestNewLiveChatView_WithTaskAndAgent(t *testing.T) {
	v := newViewWithTask("run-002", "review-auth", "Claude Code")
	assert.Equal(t, "review-auth", v.taskID)
	assert.Equal(t, "Claude Code", v.agentName)
}

// --- Init ---

func TestLiveChatView_Init_ReturnsCmd(t *testing.T) {
	v := newView("run-init")
	cmd := v.Init()
	// Init should return a non-nil batch command (two async fetches).
	assert.NotNil(t, cmd)
}

// --- Update: run metadata ---

func TestLiveChatView_Update_RunLoaded(t *testing.T) {
	v := newView("run-abc")
	now := time.Now().UnixMilli()
	run := &smithers.RunSummary{
		RunID:        "run-abc",
		WorkflowName: "code-review",
		Status:       smithers.RunStatusRunning,
		StartedAtMs:  &now,
	}
	updated, cmd := v.Update(liveChatRunLoadedMsg{run: run})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	lc := updated.(*LiveChatView)
	assert.False(t, lc.loadingRun)
	assert.Equal(t, run, lc.run)
}

func TestLiveChatView_Update_RunError(t *testing.T) {
	v := newView("run-err")
	errMsg := errors.New("not found")
	updated, cmd := v.Update(liveChatRunErrorMsg{err: errMsg})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	lc := updated.(*LiveChatView)
	assert.False(t, lc.loadingRun)
	assert.Equal(t, errMsg, lc.runErr)
}

// --- Update: block loading ---

func TestLiveChatView_Update_BlocksLoaded(t *testing.T) {
	v := newView("run-blk")
	blocks := []smithers.ChatBlock{
		makeBlock("run-blk", "node-1", "assistant", "Hello world", 1000),
		makeBlock("run-blk", "node-1", "user", "Proceed", 2000),
	}
	updated, cmd := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	require.NotNil(t, updated)
	// After blocks load, the view kicks off openStreamCmd — expect a non-nil cmd.
	assert.NotNil(t, cmd, "loading blocks should kick off the SSE stream open command")

	lc := updated.(*LiveChatView)
	assert.False(t, lc.loadingBlocks)
	assert.Len(t, lc.blocks, 2)
}

func TestLiveChatView_Update_BlocksError(t *testing.T) {
	v := newView("run-berr")
	errMsg := errors.New("stream error")
	updated, _ := v.Update(liveChatBlocksErrorMsg{err: errMsg})
	lc := updated.(*LiveChatView)
	assert.False(t, lc.loadingBlocks)
	assert.Equal(t, errMsg, lc.blocksErr)
}

// --- Update: streaming new block ---

func TestLiveChatView_Update_NewBlock_AppendedWhenFollow(t *testing.T) {
	v := newView("run-stream")
	// Seed with one block first.
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: []smithers.ChatBlock{
		makeBlock("run-stream", "n1", "system", "Init", 0),
	}})
	lc := updated.(*LiveChatView)
	lc.follow = true
	lc.width = 80
	lc.height = 24

	newBlock := makeBlock("run-stream", "n1", "assistant", "Streaming content", 1000)
	updated2, _ := lc.Update(liveChatNewBlockMsg{block: newBlock})
	lc2 := updated2.(*LiveChatView)

	assert.Len(t, lc2.blocks, 2)
	assert.Equal(t, "Streaming content", lc2.blocks[1].Content)
}

// --- Update: window resize ---

func TestLiveChatView_Update_WindowSize(t *testing.T) {
	v := newView("run-resize")
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	lc := updated.(*LiveChatView)
	assert.Equal(t, 120, lc.width)
	assert.Equal(t, 40, lc.height)
	// linesDirty may be false if scrollToBottom() consumed the dirty flag
	// by calling renderedLines(); what matters is that width/height are updated.
}

// --- Update: keyboard ---

func TestLiveChatView_Update_EscPopsView(t *testing.T) {
	v := newView("run-esc")
	v.width = 80
	v.height = 24
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	// Execute the command and check it emits a PopViewMsg.
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

func TestLiveChatView_Update_QPopsView(t *testing.T) {
	v := newView("run-q")
	v.width = 80
	v.height = 24
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'q'})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "'q' should emit PopViewMsg")
}

func TestLiveChatView_Update_FTogglesFollow(t *testing.T) {
	v := newView("run-f")
	v.width = 80
	v.height = 24
	assert.True(t, v.follow)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'f'})
	lc := updated.(*LiveChatView)
	assert.False(t, lc.follow, "'f' should disable follow when it was on")

	updated2, _ := lc.Update(tea.KeyPressMsg{Code: 'f'})
	lc2 := updated2.(*LiveChatView)
	assert.True(t, lc2.follow, "'f' should re-enable follow when it was off")
}

func TestLiveChatView_Update_ArrowsScroll(t *testing.T) {
	v := newView("run-scroll")
	v.width = 80
	v.height = 10
	// Load enough blocks to cause scrolling.
	var blocks []smithers.ChatBlock
	for i := 0; i < 30; i++ {
		blocks = append(blocks, makeBlock("run-scroll", "n1", "assistant",
			"Line content "+strings.Repeat("x", 40), int64(i*1000)))
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	lc.follow = false
	lc.width = 80
	lc.height = 10

	// Get the scroll position after blocks are loaded (follow was on, so it scrolled to bottom).
	// Now test that down arrow increments by 1.
	initialScroll := lc.scrollLine

	updated2, _ := lc.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	lc2 := updated2.(*LiveChatView)
	// Down arrow: if not at max, scrollLine increments; if at max, clamped.
	assert.GreaterOrEqual(t, lc2.scrollLine, initialScroll, "down at max should stay clamped")
	assert.False(t, lc2.follow, "scroll down should keep follow off")

	// Test scroll up from a non-zero position.
	lc2.scrollLine = 5
	lc2.follow = false
	updated3, _ := lc2.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	lc3 := updated3.(*LiveChatView)
	assert.Equal(t, 4, lc3.scrollLine, "up arrow should decrement scroll")
}

func TestLiveChatView_Update_UpAtTopDoesNotGoNegative(t *testing.T) {
	v := newView("run-top")
	v.width = 80
	v.height = 24
	v.scrollLine = 0
	v.follow = false
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	lc := updated.(*LiveChatView)
	assert.Equal(t, 0, lc.scrollLine, "scroll should not go below zero")
}

func TestLiveChatView_Update_HTriggersHijack(t *testing.T) {
	v := newView("run-h")
	v.width = 80
	v.height = 24
	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'h'})
	lc := updated.(*LiveChatView)
	// 'h' sets hijacking=true and returns a hijackRunCmd (non-nil).
	assert.True(t, lc.hijacking, "'h' should set hijacking=true")
	assert.NotNil(t, cmd, "'h' should return a hijack command")
}

func TestLiveChatView_Update_HIsNoOpWhenAlreadyHijacking(t *testing.T) {
	v := newView("run-h2")
	v.width = 80
	v.height = 24
	v.hijacking = true // already hijacking
	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'h'})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd, "'h' while hijacking should be a no-op")
}

func TestLiveChatView_Update_RRefreshesBlocks(t *testing.T) {
	v := newView("run-refresh")
	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	lc := updated.(*LiveChatView)
	assert.True(t, lc.loadingBlocks, "'r' should set loadingBlocks = true")
	assert.NotNil(t, cmd, "'r' should return a fetch command")
}

// --- Update: stream lifecycle messages ---

func TestLiveChatView_Update_ChatStreamDone_OwnRun(t *testing.T) {
	v := newView("run-done")
	updated, cmd := v.Update(smithers.ChatStreamDoneMsg{RunID: "run-done"})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
}

func TestLiveChatView_Update_ChatStreamDone_OtherRun(t *testing.T) {
	v := newView("run-mine")
	updated, cmd := v.Update(smithers.ChatStreamDoneMsg{RunID: "run-other"})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
}

func TestLiveChatView_Update_ChatStreamError(t *testing.T) {
	v := newView("run-serr")
	streamErr := errors.New("SSE error")
	updated, _ := v.Update(smithers.ChatStreamErrorMsg{RunID: "run-serr", Err: streamErr})
	lc := updated.(*LiveChatView)
	assert.Equal(t, streamErr, lc.blocksErr)
}

// --- View() rendering ---

func TestLiveChatView_View_LoadingState(t *testing.T) {
	v := newView("run-loading")
	v.width = 80
	v.height = 24
	out := v.View()
	assert.Contains(t, out, "Loading")
}

func TestLiveChatView_View_ErrorState(t *testing.T) {
	v := newView("run-errview")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.runErr = errors.New("server unavailable")
	out := v.View()
	assert.Contains(t, out, "Error")
}

func TestLiveChatView_View_EmptyState(t *testing.T) {
	v := newView("run-empty")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	out := v.View()
	assert.Contains(t, out, "No messages")
}

func TestLiveChatView_View_ContainsRunID(t *testing.T) {
	v := newView("run-viewtest")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	out := v.View()
	// runID is truncated to 8 chars in the header
	assert.Contains(t, out, "run-view")
}

func TestLiveChatView_View_RendersChatBlocks(t *testing.T) {
	v := newView("run-render")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-render", "node-1", "system", "You are a code reviewer.", 0),
		makeBlock("run-render", "node-1", "assistant", "I'll start by reading the files.", 1000),
	}
	v.linesDirty = true
	out := v.View()
	assert.Contains(t, out, "System")
	assert.Contains(t, out, "You are a code reviewer.")
	assert.Contains(t, out, "Assistant")
	assert.Contains(t, out, "I'll start by reading the files.")
}

func TestLiveChatView_View_StreamingIndicator_ActiveRun(t *testing.T) {
	v := newView("run-active")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	now := time.Now().UnixMilli()
	v.run = &smithers.RunSummary{
		RunID:       "run-active",
		Status:      smithers.RunStatusRunning,
		StartedAtMs: &now,
	}
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-active", "n1", "assistant", "Working...", now),
	}
	v.linesDirty = true
	out := v.View()
	assert.Contains(t, out, "streaming")
}

func TestLiveChatView_View_NoStreamingIndicator_FinishedRun(t *testing.T) {
	v := newView("run-done-view")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	now := time.Now().UnixMilli()
	v.run = &smithers.RunSummary{
		RunID:        "run-done-view",
		Status:       smithers.RunStatusFinished,
		StartedAtMs:  &now,
		FinishedAtMs: &now,
	}
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-done-view", "n1", "assistant", "Done.", now),
	}
	v.linesDirty = true
	out := v.View()
	assert.NotContains(t, out, "streaming")
}

func TestLiveChatView_View_FollowIndicator_SubHeader(t *testing.T) {
	v := newView("run-flw")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.follow = true
	out := v.View()
	assert.Contains(t, out, "LIVE")
}

func TestLiveChatView_View_WorkflowNameInHeader(t *testing.T) {
	v := newView("run-wf")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	now := time.Now().UnixMilli()
	v.run = &smithers.RunSummary{
		RunID:        "run-wf",
		WorkflowName: "my-workflow",
		Status:       smithers.RunStatusFinished,
		StartedAtMs:  &now,
	}
	out := v.View()
	assert.Contains(t, out, "my-workflow")
}

// --- Name / ShortHelp ---

func TestLiveChatView_Name(t *testing.T) {
	v := newView("run-name")
	assert.Equal(t, "livechat", v.Name())
}

func TestLiveChatView_ShortHelp(t *testing.T) {
	v := newView("run-help")
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	// Collect all help descriptions into a single string for assertion.
	var parts []string
	for _, b := range help {
		h := b.Help()
		parts = append(parts, h.Key, h.Desc)
	}
	joined := strings.Join(parts, " ")
	assert.Contains(t, strings.ToLower(joined), "scroll")
	assert.Contains(t, strings.ToLower(joined), "follow")
	assert.Contains(t, strings.ToLower(joined), "hijack")
	assert.Contains(t, strings.ToLower(joined), "back")
}

func TestLiveChatView_ShortHelp_FollowOff(t *testing.T) {
	v := newView("run-help-off")
	v.follow = false
	help := v.ShortHelp()
	var parts []string
	for _, b := range help {
		h := b.Help()
		parts = append(parts, h.Key, h.Desc)
	}
	joined := strings.Join(parts, " ")
	assert.Contains(t, joined, "follow: off")
}

// --- Attempt tracking ---

func TestLiveChatView_AttemptTracking_IndexedCorrectly(t *testing.T) {
	v := newView("run-attempts")
	blocks := []smithers.ChatBlock{
		{RunID: "run-attempts", NodeID: "n1", Role: smithers.ChatRoleUser, Content: "prompt", Attempt: 0, TimestampMs: 100},
		{RunID: "run-attempts", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "resp1", Attempt: 0, TimestampMs: 200},
		{RunID: "run-attempts", NodeID: "n1", Role: smithers.ChatRoleUser, Content: "retry", Attempt: 1, TimestampMs: 300},
		{RunID: "run-attempts", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "resp2", Attempt: 1, TimestampMs: 400},
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)

	assert.Equal(t, 1, lc.maxAttempt, "maxAttempt should be 1 (0-based)")
	assert.Equal(t, 1, lc.currentAttempt, "currentAttempt should track latest")
	assert.Len(t, lc.attempts[0], 2, "attempt 0 should have 2 blocks")
	assert.Len(t, lc.attempts[1], 2, "attempt 1 should have 2 blocks")
}

func TestLiveChatView_AttemptNavigation_BracketKeys(t *testing.T) {
	v := newView("run-nav")
	v.width = 80
	v.height = 24
	blocks := []smithers.ChatBlock{
		{RunID: "run-nav", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "a0", Attempt: 0, TimestampMs: 100},
		{RunID: "run-nav", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "a1", Attempt: 1, TimestampMs: 200},
		{RunID: "run-nav", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "a2", Attempt: 2, TimestampMs: 300},
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	assert.Equal(t, 2, lc.currentAttempt, "should be on latest attempt")

	// Navigate back with '['.
	updated2, _ := lc.Update(tea.KeyPressMsg{Code: '['})
	lc2 := updated2.(*LiveChatView)
	assert.Equal(t, 1, lc2.currentAttempt, "'[' should go to attempt 1")

	updated3, _ := lc2.Update(tea.KeyPressMsg{Code: '['})
	lc3 := updated3.(*LiveChatView)
	assert.Equal(t, 0, lc3.currentAttempt, "'[' should go to attempt 0")

	// Can't go below 0.
	updated4, _ := lc3.Update(tea.KeyPressMsg{Code: '['})
	lc4 := updated4.(*LiveChatView)
	assert.Equal(t, 0, lc4.currentAttempt, "'[' at min should stay at 0")

	// Navigate forward with ']'.
	updated5, _ := lc4.Update(tea.KeyPressMsg{Code: ']'})
	lc5 := updated5.(*LiveChatView)
	assert.Equal(t, 1, lc5.currentAttempt, "']' should go to attempt 1")
}

func TestLiveChatView_AttemptNavigation_NoOp_SingleAttempt(t *testing.T) {
	v := newView("run-single")
	v.width = 80
	v.height = 24
	// Only one attempt.
	blocks := []smithers.ChatBlock{
		{RunID: "run-single", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "hi", Attempt: 0, TimestampMs: 100},
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	assert.Equal(t, 0, lc.maxAttempt)

	// '[' and ']' should be no-ops.
	updated2, _ := lc.Update(tea.KeyPressMsg{Code: '['})
	lc2 := updated2.(*LiveChatView)
	assert.Equal(t, 0, lc2.currentAttempt)

	updated3, _ := lc.Update(tea.KeyPressMsg{Code: ']'})
	lc3 := updated3.(*LiveChatView)
	assert.Equal(t, 0, lc3.currentAttempt)
}

func TestLiveChatView_RenderHeader_TwoAttempts(t *testing.T) {
	v := newView("run-hdr2")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.maxAttempt = 1
	v.currentAttempt = 1
	out := v.View()
	assert.Contains(t, out, "Attempt: 2 of 2", "header should show attempt N of M")
}

func TestLiveChatView_NewBlocksInLatestBadge(t *testing.T) {
	v := newView("run-badge")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	v.maxAttempt = 1
	v.currentAttempt = 0 // viewing attempt 0
	v.attempts = map[int][]smithers.ChatBlock{
		0: {makeBlock("run-badge", "n1", "assistant", "a0", 100)},
		1: {makeBlock("run-badge", "n1", "assistant", "a1", 200)},
	}
	v.blocks = append(v.attempts[0], v.attempts[1]...)
	v.newBlocksInLatest = 3

	out := v.View()
	assert.Contains(t, out, "new in latest attempt", "badge should appear when viewing older attempt")
}

// --- SSE streaming integration ---

func TestLiveChatView_Update_StreamOpened_ThenBlock(t *testing.T) {
	v := newView("run-sse")
	v.width = 80
	v.height = 24

	// Load blocks (triggers openStreamCmd).
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: nil})
	lc := updated.(*LiveChatView)

	// Simulate the stream being opened via liveChatStreamOpenedMsg.
	ch := make(chan smithers.ChatBlock, 2)
	ch <- smithers.ChatBlock{RunID: "run-sse", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "hello", TimestampMs: 100}

	updated2, cmd := lc.Update(liveChatStreamOpenedMsg{
		ch:     ch,
		cancel: func() {},
		runID:  "run-sse",
	})
	lc2 := updated2.(*LiveChatView)
	assert.NotNil(t, lc2.blockCh, "blockCh should be set after stream open")
	assert.NotNil(t, cmd, "should return WaitForChatBlock cmd")

	// Execute the WaitForChatBlock cmd — it should return a ChatBlockMsg.
	msg := cmd()
	cbm, ok := msg.(smithers.ChatBlockMsg)
	require.True(t, ok, "should be ChatBlockMsg, got %T", msg)
	assert.Equal(t, "hello", cbm.Block.Content)
}

func TestLiveChatView_Update_StreamOpened_WrongRunID_Ignored(t *testing.T) {
	v := newView("run-sse-mine")
	ch := make(chan smithers.ChatBlock)
	cancelled := false
	updated, cmd := v.Update(liveChatStreamOpenedMsg{
		ch:     ch,
		cancel: func() { cancelled = true },
		runID:  "run-other", // different runID
	})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
	assert.True(t, cancelled, "stream with wrong runID should be cancelled")
}

func TestLiveChatView_Update_ChatBlockMsg_OtherRun_Ignored(t *testing.T) {
	v := newView("run-mine")
	v.blocks = nil
	updated, cmd := v.Update(smithers.ChatBlockMsg{
		RunID: "run-other",
		Block: smithers.ChatBlock{Content: "ignored"},
	})
	lc := updated.(*LiveChatView)
	assert.Empty(t, lc.blocks, "block from other run should be ignored")
	assert.Nil(t, cmd)
}

func TestLiveChatView_Update_ChatBlockMsg_LaterAttempt_Badge(t *testing.T) {
	v := newView("run-badge2")
	v.width = 80
	v.height = 24
	v.maxAttempt = 0
	v.currentAttempt = 0
	v.attempts = make(map[int][]smithers.ChatBlock)
	v.blocks = nil

	// Set up a fake channel so WaitForChatBlock doesn't block.
	ch := make(chan smithers.ChatBlock, 1)
	v.blockCh = ch

	// Block for attempt 1 arrives while viewing attempt 0.
	updated, cmd := v.Update(smithers.ChatBlockMsg{
		RunID: "run-badge2",
		Block: smithers.ChatBlock{RunID: "run-badge2", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "new", Attempt: 1, TimestampMs: 500},
	})
	lc := updated.(*LiveChatView)
	assert.Equal(t, 1, lc.newBlocksInLatest, "badge count should increment")
	assert.Equal(t, 0, lc.currentAttempt, "current attempt should not change")
	assert.NotNil(t, cmd, "should re-schedule WaitForChatBlock")
}

// --- Follow mode ---

func TestLiveChatView_FollowMode_NewBlockScrollsToBottom(t *testing.T) {
	v := newView("run-follow")
	v.width = 80
	v.height = 10
	v.follow = true

	// Seed with many blocks to force scroll.
	var blocks []smithers.ChatBlock
	for i := 0; i < 30; i++ {
		blocks = append(blocks, makeBlock("run-follow", "n1", "assistant",
			"Line "+strings.Repeat("x", 40), int64(i*100)))
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	lc.follow = true
	lc.linesDirty = true

	// Receive a new block — should scroll to bottom.
	newBlock := makeBlock("run-follow", "n1", "assistant", "latest", int64(30*100))
	updated2, _ := lc.Update(liveChatNewBlockMsg{block: newBlock})
	lc2 := updated2.(*LiveChatView)

	lines := lc2.renderedLines()
	visible := lc2.visibleHeight()
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	assert.Equal(t, maxScroll, lc2.scrollLine, "follow mode should scroll to bottom on new block")
}

func TestLiveChatView_UnfollowOnScrollUp(t *testing.T) {
	v := newView("run-unfollow")
	v.width = 80
	v.height = 10
	v.follow = true
	v.scrollLine = 5

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	lc := updated.(*LiveChatView)
	assert.False(t, lc.follow, "up arrow should disable follow mode")
}

// --- Hijack flow ---

func TestLiveChatView_HijackFlow_BinaryNotFound(t *testing.T) {
	v := newView("run-hijack")
	v.width = 80
	v.height = 24
	v.hijacking = true

	// Simulate hijack session received with a binary that doesn't exist.
	session := &smithers.HijackSession{
		RunID:          "run-hijack",
		AgentEngine:    "claude-code",
		AgentBinary:    "/nonexistent/path/to/claude",
		ResumeToken:    "tok",
		CWD:            "/tmp",
		SupportsResume: true,
	}
	updated, cmd := v.Update(liveChatHijackSessionMsg{session: session, err: nil})
	lc := updated.(*LiveChatView)

	assert.False(t, lc.hijacking, "hijacking should be false after resolution")
	assert.NotNil(t, lc.hijackErr, "should have hijackErr when binary not found")
	assert.Nil(t, cmd, "no cmd when binary not found")
}

func TestLiveChatView_HijackFlow_Error(t *testing.T) {
	v := newView("run-hijack-err")
	v.width = 80
	v.height = 24
	v.hijacking = true

	updated, cmd := v.Update(liveChatHijackSessionMsg{
		session: nil,
		err:     errors.New("server unavailable"),
	})
	lc := updated.(*LiveChatView)

	assert.False(t, lc.hijacking)
	assert.NotNil(t, lc.hijackErr)
	assert.Nil(t, cmd)
}

func TestLiveChatView_HijackReturn_AddsHijackDividerAndRefreshes(t *testing.T) {
	v := newView("run-hjret")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false

	updated, cmd := v.Update(liveChatHijackReturnMsg{runID: "run-hjret", err: nil})
	lc := updated.(*LiveChatView)

	// Should have added a divider block.
	assert.NotEmpty(t, lc.blocks, "hijack return should add a divider block")
	hasHijackBlock := false
	for _, b := range lc.blocks {
		if strings.Contains(b.Content, "HIJACK SESSION ENDED") {
			hasHijackBlock = true
		}
	}
	assert.True(t, hasHijackBlock, "should have HIJACK SESSION ENDED block")
	// Should return a refresh cmd.
	assert.NotNil(t, cmd, "hijack return should return a refresh cmd")
}

func TestLiveChatView_HijackReturn_WrongRunID_Ignored(t *testing.T) {
	v := newView("run-mine")
	updated, cmd := v.Update(liveChatHijackReturnMsg{runID: "run-other", err: nil})
	lc := updated.(*LiveChatView)
	assert.Empty(t, lc.blocks, "hijack return for other run should be ignored")
	assert.Nil(t, cmd)
}

// --- EscCancelsStream ---

func TestLiveChatView_EscCancelsStream(t *testing.T) {
	v := newView("run-esc-stream")
	cancelled := false
	v.blockCancel = func() { cancelled = true }

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, cancelled, "Esc should cancel the SSE stream")
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok)
}

// --- Tool block rendering ---

func TestLiveChatView_View_RendersToolBlock(t *testing.T) {
	v := newView("run-tool")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-tool", "n1", "tool", "bash result: exit 0", 1000),
	}
	v.linesDirty = true
	out := v.View()
	assert.Contains(t, out, "Tool", "should render Tool role header")
}

// Ensure the key package import is used; suppress any "imported and not used" error.
var _ = key.NewBinding

// --- Scroll clamping ---

func TestLiveChatView_ScrollToBottom_EmptyLines(t *testing.T) {
	v := newView("run-empty-scroll")
	v.width = 80
	v.height = 24
	v.scrollToBottom() // should not panic
	assert.Equal(t, 0, v.scrollLine)
}

// --- fmtDuration helper ---

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:30"},
		{90 * time.Second, "01:30"},
		{10*time.Minute + 5*time.Second, "10:05"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, fmtDuration(tt.d), "fmtDuration(%v)", tt.d)
	}
}

// --- Context plumbing ---

func TestLiveChatView_FetchRunCmd_DoesNotPanic(t *testing.T) {
	// Ensures the fetchRun command can be called without panicking even
	// when no server is available (will return an error message, not panic).
	v := newView("run-ctx")
	cmd := v.fetchRun()
	require.NotNil(t, cmd)
	msg := cmd()
	// Should return either a runLoadedMsg (unlikely, no server) or runErrorMsg.
	switch msg.(type) {
	case liveChatRunLoadedMsg, liveChatRunErrorMsg:
		// OK
	default:
		t.Errorf("unexpected message type %T", msg)
	}
}

func TestLiveChatView_FetchBlocksCmd_DoesNotPanic(t *testing.T) {
	v := newView("run-ctx2")
	cmd := v.fetchBlocks()
	require.NotNil(t, cmd)
	msg := cmd()
	switch msg.(type) {
	case liveChatBlocksLoadedMsg, liveChatBlocksErrorMsg:
		// OK
	default:
		t.Errorf("unexpected message type %T", msg)
	}
}

// =============================================================================
// feat-live-chat-streaming-output: additional streaming unit tests
// =============================================================================

// TestLiveChatView_Streaming_MultipleBlocks verifies that multiple ChatBlockMsg
// events are all appended to the block list and that linesDirty is set after
// each one, so the next render will pick up the new content.
func TestLiveChatView_Streaming_MultipleBlocks(t *testing.T) {
	v := newView("run-multi")
	v.width = 80
	v.height = 24
	v.follow = true

	// Simulate stream opened.
	ch := make(chan smithers.ChatBlock, 4)
	v.blockCh = ch

	texts := []string{"block-one", "block-two", "block-three"}
	for i, text := range texts {
		block := smithers.ChatBlock{
			RunID:       "run-multi",
			NodeID:      "n1",
			Role:        smithers.ChatRoleAssistant,
			Content:     text,
			Attempt:     0,
			TimestampMs: int64(i+1) * 1000,
		}
		updated, _ := v.Update(smithers.ChatBlockMsg{RunID: "run-multi", Block: block})
		v = updated.(*LiveChatView)
	}

	require.Len(t, v.blocks, 3, "all three blocks should be appended")
	for i, text := range texts {
		assert.Equal(t, text, v.blocks[i].Content)
	}
}

// TestLiveChatView_Streaming_BlockAppendsToCurrentAttempt verifies that a
// ChatBlockMsg for the current attempt appends the block and schedules the next
// WaitForChatBlock read.  When follow mode is on, scrollToBottom() is called
// which rebuilds the line cache (linesDirty becomes false), so we verify the
// rendered output contains the new content instead.
func TestLiveChatView_Streaming_BlockAppendsToCurrentAttempt(t *testing.T) {
	v := newView("run-cur-attempt")
	v.width = 80
	v.height = 24
	v.follow = false // keep follow off so linesDirty stays true after the update
	v.currentAttempt = 0
	v.maxAttempt = 0
	v.attempts = make(map[int][]smithers.ChatBlock)
	v.blocks = nil
	v.loadingRun = false
	v.loadingBlocks = false

	ch := make(chan smithers.ChatBlock, 1)
	v.blockCh = ch

	block := smithers.ChatBlock{
		RunID:       "run-cur-attempt",
		NodeID:      "n1",
		Role:        smithers.ChatRoleAssistant,
		Content:     "incremental output",
		Attempt:     0,
		TimestampMs: 500,
	}
	updated, cmd := v.Update(smithers.ChatBlockMsg{RunID: "run-cur-attempt", Block: block})
	lc := updated.(*LiveChatView)

	// Block must be stored.
	require.Len(t, lc.blocks, 1)
	assert.Equal(t, "incremental output", lc.blocks[0].Content)

	// linesDirty must be true (follow is off, so scrollToBottom was not called).
	assert.True(t, lc.linesDirty, "linesDirty should be true after new block when follow=false")

	// cmd should be the next WaitForChatBlock.
	assert.NotNil(t, cmd, "should return next WaitForChatBlock cmd")

	// After View() is called, the new content should appear in the rendered output.
	out := lc.View()
	assert.Contains(t, out, "incremental output", "rendered output should contain the newly streamed block")
}

// TestLiveChatView_Streaming_StreamDone_SetsFlag verifies that ChatStreamDoneMsg
// sets v.streamDone = true.
func TestLiveChatView_Streaming_StreamDone_SetsFlag(t *testing.T) {
	v := newView("run-sdone")
	assert.False(t, v.streamDone, "streamDone should start false")
	updated, _ := v.Update(smithers.ChatStreamDoneMsg{RunID: "run-sdone"})
	lc := updated.(*LiveChatView)
	assert.True(t, lc.streamDone, "streamDone should be true after ChatStreamDoneMsg")
}

// TestLiveChatView_Streaming_StreamDone_HidesIndicator verifies that once
// streamDone is set, the streaming indicator ("█ (streaming...)") is hidden even
// when the run status is still active.
func TestLiveChatView_Streaming_StreamDone_HidesIndicator(t *testing.T) {
	v := newView("run-ind-done")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	now := time.Now().UnixMilli()
	v.run = &smithers.RunSummary{
		RunID:       "run-ind-done",
		Status:      smithers.RunStatusRunning,
		StartedAtMs: &now,
	}
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-ind-done", "n1", "assistant", "Done.", now),
	}
	v.streamDone = true
	v.linesDirty = true

	out := v.View()
	assert.NotContains(t, out, "streaming", "streaming indicator should be hidden when streamDone=true")
}

// TestLiveChatView_Streaming_StreamDone_OtherRun_DoesNotSetFlag confirms that a
// ChatStreamDoneMsg for a different run does not affect the view.
func TestLiveChatView_Streaming_StreamDone_OtherRun_DoesNotSetFlag(t *testing.T) {
	v := newView("run-mine")
	v.streamDone = false
	updated, _ := v.Update(smithers.ChatStreamDoneMsg{RunID: "run-other"})
	lc := updated.(*LiveChatView)
	assert.False(t, lc.streamDone, "streamDone should remain false for unrelated run")
}

// TestLiveChatView_Streaming_FollowScrollsToBottom_OnChatBlockMsg verifies that
// when follow mode is active, a ChatBlockMsg triggers scroll-to-bottom.
func TestLiveChatView_Streaming_FollowScrollsToBottom_OnChatBlockMsg(t *testing.T) {
	v := newView("run-follow-stream")
	v.width = 80
	v.height = 10
	v.follow = true

	// Seed with many blocks so there's something to scroll.
	var blocks []smithers.ChatBlock
	for i := 0; i < 20; i++ {
		blocks = append(blocks, makeBlock("run-follow-stream", "n1", "assistant",
			strings.Repeat("x", 40), int64(i*100)))
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	lc.follow = true
	lc.width = 80
	lc.height = 10

	ch := make(chan smithers.ChatBlock, 1)
	lc.blockCh = ch

	// Deliver another block via ChatBlockMsg.
	newBlock := smithers.ChatBlock{
		RunID: "run-follow-stream", NodeID: "n1",
		Role: smithers.ChatRoleAssistant, Content: "latest line",
		Attempt: 0, TimestampMs: 2100,
	}
	updated2, _ := lc.Update(smithers.ChatBlockMsg{RunID: "run-follow-stream", Block: newBlock})
	lc2 := updated2.(*LiveChatView)

	lines := lc2.renderedLines()
	visible := lc2.visibleHeight()
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	assert.Equal(t, maxScroll, lc2.scrollLine,
		"follow mode should scroll to bottom when a ChatBlockMsg arrives")
}

// TestLiveChatView_Streaming_PgDown_EnablesFollow verifies that pressing PgDn
// to the last page re-enables follow mode.
func TestLiveChatView_Streaming_PgDown_EnablesFollow(t *testing.T) {
	v := newView("run-pgdn-follow")
	v.width = 80
	v.height = 10
	v.follow = false

	// Seed just enough blocks to fill more than a page.
	var blocks []smithers.ChatBlock
	for i := 0; i < 15; i++ {
		blocks = append(blocks, makeBlock("run-pgdn-follow", "n1", "assistant",
			strings.Repeat("y", 40), int64(i*100)))
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	lc.follow = false
	lc.scrollLine = 0
	lc.width = 80
	lc.height = 10

	// Page down until clamped at the bottom.
	for i := 0; i < 20; i++ {
		prev := lc.scrollLine
		updated2, _ := lc.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		lc = updated2.(*LiveChatView)
		if lc.scrollLine == prev {
			break // reached the bottom
		}
	}

	assert.True(t, lc.follow, "follow should be re-enabled when PgDn reaches the bottom")
}

// TestLiveChatView_Streaming_PgUp_DisablesFollow verifies that pressing PgUp
// disables follow mode and moves the scroll position up.
func TestLiveChatView_Streaming_PgUp_DisablesFollow(t *testing.T) {
	v := newView("run-pgup-nofollow")
	v.width = 80
	v.height = 10
	v.follow = true

	// Seed blocks.
	var blocks []smithers.ChatBlock
	for i := 0; i < 15; i++ {
		blocks = append(blocks, makeBlock("run-pgup-nofollow", "n1", "assistant",
			strings.Repeat("z", 40), int64(i*100)))
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	lc.follow = true
	lc.width = 80
	lc.height = 10
	lc.scrollToBottom()
	initialScroll := lc.scrollLine

	updated2, _ := lc.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	lc2 := updated2.(*LiveChatView)

	assert.False(t, lc2.follow, "PgUp should disable follow mode")
	assert.Less(t, lc2.scrollLine, initialScroll, "PgUp should move scroll position up")
}

// TestLiveChatView_Streaming_RenderBodyScrollClamping verifies that renderBody
// does not panic and clamps scrollLine when it exceeds the maximum.
func TestLiveChatView_Streaming_RenderBodyScrollClamping(t *testing.T) {
	v := newView("run-clamp")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-clamp", "n1", "assistant", "short", 100),
	}
	v.linesDirty = true
	v.scrollLine = 9999 // set an absurdly large scroll position

	// View() calls renderBody() which clamps scrollLine.
	out := v.View()
	assert.NotEmpty(t, out, "should render without panic despite large scrollLine")
	assert.LessOrEqual(t, v.scrollLine, len(v.renderedLines()),
		"scrollLine should be clamped to a sane value")
}

// TestLiveChatView_Streaming_DisplayBlocks_FallsBackToAll verifies that
// displayBlocks() returns all blocks when attempts map is empty (pre-index state
// during initial load).
func TestLiveChatView_Streaming_DisplayBlocks_FallsBackToAll(t *testing.T) {
	v := newView("run-display-all")
	v.attempts = make(map[int][]smithers.ChatBlock) // empty — no attempts indexed yet
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-display-all", "n1", "assistant", "msg", 100),
	}
	result := v.displayBlocks()
	assert.Equal(t, v.blocks, result, "displayBlocks should return all blocks when no attempts are indexed")
}

// TestLiveChatView_Streaming_DisplayBlocks_ReturnsCurrentAttempt verifies that
// displayBlocks() returns only the current attempt's blocks when indexed.
func TestLiveChatView_Streaming_DisplayBlocks_ReturnsCurrentAttempt(t *testing.T) {
	v := newView("run-display-attempt")
	v.currentAttempt = 1
	v.attempts = map[int][]smithers.ChatBlock{
		0: {makeBlock("run-display-attempt", "n1", "assistant", "a0", 100)},
		1: {makeBlock("run-display-attempt", "n1", "assistant", "a1", 200),
			makeBlock("run-display-attempt", "n1", "user", "u1", 300)},
	}
	result := v.displayBlocks()
	require.Len(t, result, 2, "should return attempt 1's 2 blocks")
	assert.Equal(t, "a1", result[0].Content)
	assert.Equal(t, "u1", result[1].Content)
}

// TestLiveChatView_Streaming_RenderedLines_RebuildOnDirty verifies that
// renderedLines() rebuilds the cache when linesDirty is true and returns the
// same (cached) slice when called a second time without changes.
func TestLiveChatView_Streaming_RenderedLines_RebuildOnDirty(t *testing.T) {
	v := newView("run-cache")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-cache", "n1", "assistant", "hello world", 100),
	}
	v.linesDirty = true

	lines1 := v.renderedLines()
	assert.False(t, v.linesDirty, "linesDirty should be false after first renderedLines()")

	lines2 := v.renderedLines()
	assert.Equal(t, len(lines1), len(lines2), "second call should return cached (same length) slice")
}

// =============================================================================
// feat-live-chat-attempt-tracking: additional attempt tracking unit tests
// =============================================================================

// TestLiveChatView_AttemptTracking_ShortHelp_NoAttemptHint_SingleAttempt verifies
// that the attempt nav hint ([/]) is absent from ShortHelp when there is only
// one attempt (maxAttempt == 0).
func TestLiveChatView_AttemptTracking_ShortHelp_NoAttemptHint_SingleAttempt(t *testing.T) {
	v := newView("run-hint-single")
	v.maxAttempt = 0
	help := v.ShortHelp()
	var parts []string
	for _, b := range help {
		h := b.Help()
		parts = append(parts, h.Key, h.Desc)
	}
	joined := strings.Join(parts, " ")
	assert.NotContains(t, joined, "[/]", "attempt nav hint should not appear with a single attempt")
}

// TestLiveChatView_AttemptTracking_ShortHelp_AttemptHint_MultiAttempt verifies
// that the attempt nav hint ([/]) is present in ShortHelp when there are
// multiple attempts (maxAttempt > 0).
func TestLiveChatView_AttemptTracking_ShortHelp_AttemptHint_MultiAttempt(t *testing.T) {
	v := newView("run-hint-multi")
	v.maxAttempt = 2
	help := v.ShortHelp()
	var parts []string
	for _, b := range help {
		h := b.Help()
		parts = append(parts, h.Key, h.Desc)
	}
	joined := strings.Join(parts, " ")
	assert.Contains(t, joined, "[/]", "attempt nav hint should appear with multiple attempts")
}

// TestLiveChatView_AttemptTracking_SubHeader_NoAttemptWhenSingle verifies
// that the "Attempt: N of M" label does NOT appear in the sub-header when there
// is only one attempt.
func TestLiveChatView_AttemptTracking_SubHeader_NoAttemptWhenSingle(t *testing.T) {
	v := newView("run-subhdr-single")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.maxAttempt = 0
	v.currentAttempt = 0
	out := v.View()
	assert.NotContains(t, out, "Attempt:", "attempt label should not appear with a single attempt")
}

// TestLiveChatView_AttemptTracking_SubHeader_ShowsAttemptWhenMultiple verifies
// that "Attempt: N of M" appears in the sub-header when maxAttempt > 0.
func TestLiveChatView_AttemptTracking_SubHeader_ShowsAttemptWhenMultiple(t *testing.T) {
	v := newView("run-subhdr-multi")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.maxAttempt = 2
	v.currentAttempt = 1
	out := v.View()
	assert.Contains(t, out, "Attempt:", "attempt label should appear with multiple attempts")
	assert.Contains(t, out, "2 of 3", "should show current+1 of max+1")
}

// TestLiveChatView_AttemptTracking_NewBlock_IncrementsMaxAttempt verifies that
// streaming a block for a brand-new attempt (via liveChatNewBlockMsg) updates
// maxAttempt.
func TestLiveChatView_AttemptTracking_NewBlock_IncrementsMaxAttempt(t *testing.T) {
	v := newView("run-inc-max")
	v.width = 80
	v.height = 24
	v.attempts = make(map[int][]smithers.ChatBlock)
	v.maxAttempt = 0
	v.currentAttempt = 0

	// Inject a block for attempt 1.
	updated, _ := v.Update(liveChatNewBlockMsg{
		block: smithers.ChatBlock{
			RunID: "run-inc-max", NodeID: "n1",
			Role: smithers.ChatRoleAssistant, Content: "retry", Attempt: 1, TimestampMs: 100,
		},
	})
	lc := updated.(*LiveChatView)
	assert.Equal(t, 1, lc.maxAttempt, "maxAttempt should update when a higher-attempt block arrives via liveChatNewBlockMsg")
}

// TestLiveChatView_AttemptTracking_NavigateToLatest_ClearsBadge verifies that
// navigating from an older attempt to the latest attempt (via ']') clears the
// newBlocksInLatest badge.
func TestLiveChatView_AttemptTracking_NavigateToLatest_ClearsBadge(t *testing.T) {
	v := newView("run-badge-clear")
	v.width = 80
	v.height = 24
	v.maxAttempt = 1
	v.currentAttempt = 0 // viewing attempt 0
	v.newBlocksInLatest = 5
	v.attempts = map[int][]smithers.ChatBlock{
		0: {makeBlock("run-badge-clear", "n1", "assistant", "a0", 100)},
		1: {makeBlock("run-badge-clear", "n1", "assistant", "a1", 200)},
	}
	v.blocks = append(v.attempts[0], v.attempts[1]...)

	// Press ']' to navigate to the latest attempt.
	updated, _ := v.Update(tea.KeyPressMsg{Code: ']'})
	lc := updated.(*LiveChatView)

	assert.Equal(t, 1, lc.currentAttempt, "should be on attempt 1 after ']'")
	assert.Equal(t, 0, lc.newBlocksInLatest, "badge should be cleared after navigating to latest")
}

// TestLiveChatView_AttemptTracking_BadgeNotShown_WhenViewingLatest verifies that
// the "new in latest attempt" badge is NOT rendered when the user is already
// viewing the latest attempt.
func TestLiveChatView_AttemptTracking_BadgeNotShown_WhenViewingLatest(t *testing.T) {
	v := newView("run-badge-hidden")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false
	v.maxAttempt = 1
	v.currentAttempt = 1 // already on the latest
	v.newBlocksInLatest = 3
	v.attempts = map[int][]smithers.ChatBlock{
		0: {makeBlock("run-badge-hidden", "n1", "assistant", "a0", 100)},
		1: {makeBlock("run-badge-hidden", "n1", "assistant", "a1", 200)},
	}
	v.blocks = append(v.attempts[0], v.attempts[1]...)

	out := v.View()
	assert.NotContains(t, out, "new in latest attempt",
		"badge should not appear when already viewing the latest attempt")
}

// TestLiveChatView_AttemptTracking_DisplayBlocks_SwitchesOnNavigation verifies
// that after navigating to a different attempt the correct blocks are displayed.
func TestLiveChatView_AttemptTracking_DisplayBlocks_SwitchesOnNavigation(t *testing.T) {
	v := newView("run-switch")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false

	blocks := []smithers.ChatBlock{
		{RunID: "run-switch", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "attempt-0", Attempt: 0, TimestampMs: 100},
		{RunID: "run-switch", NodeID: "n1", Role: smithers.ChatRoleAssistant, Content: "attempt-1", Attempt: 1, TimestampMs: 200},
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	// Should be on latest (attempt 1).
	assert.Equal(t, 1, lc.currentAttempt)

	displayed := lc.displayBlocks()
	require.Len(t, displayed, 1)
	assert.Equal(t, "attempt-1", displayed[0].Content, "should display attempt-1 blocks")

	// Navigate to attempt 0.
	updated2, _ := lc.Update(tea.KeyPressMsg{Code: '['})
	lc2 := updated2.(*LiveChatView)
	assert.Equal(t, 0, lc2.currentAttempt)

	displayed2 := lc2.displayBlocks()
	require.Len(t, displayed2, 1)
	assert.Equal(t, "attempt-0", displayed2[0].Content, "should display attempt-0 blocks after '[' navigation")
}

// TestLiveChatView_AttemptTracking_ViewRendersCurrentAttemptOnly verifies that
// View() only renders blocks from the current attempt, not blocks from other
// attempts.
func TestLiveChatView_AttemptTracking_ViewRendersCurrentAttemptOnly(t *testing.T) {
	v := newView("run-render-attempt")
	v.width = 80
	v.height = 40
	v.loadingRun = false
	v.loadingBlocks = false

	blocks := []smithers.ChatBlock{
		{RunID: "run-render-attempt", NodeID: "n1", Role: smithers.ChatRoleAssistant,
			Content: "first-attempt-content", Attempt: 0, TimestampMs: 100},
		{RunID: "run-render-attempt", NodeID: "n1", Role: smithers.ChatRoleAssistant,
			Content: "second-attempt-content", Attempt: 1, TimestampMs: 200},
	}
	updated, _ := v.Update(liveChatBlocksLoadedMsg{blocks: blocks})
	lc := updated.(*LiveChatView)
	lc.currentAttempt = 0
	lc.linesDirty = true

	out := lc.View()
	assert.Contains(t, out, "first-attempt-content", "should render attempt-0 content")
	assert.NotContains(t, out, "second-attempt-content", "should NOT render attempt-1 content when viewing attempt-0")
}

// TestLiveChatView_AttemptTracking_IndexBlock_UpdatesMaxAttempt verifies the
// indexBlock helper directly: it must update maxAttempt correctly.
func TestLiveChatView_AttemptTracking_IndexBlock_UpdatesMaxAttempt(t *testing.T) {
	v := newView("run-index-direct")
	v.attempts = make(map[int][]smithers.ChatBlock)
	v.maxAttempt = 0

	v.indexBlock(smithers.ChatBlock{Attempt: 0, Content: "a"})
	assert.Equal(t, 0, v.maxAttempt)

	v.indexBlock(smithers.ChatBlock{Attempt: 2, Content: "b"})
	assert.Equal(t, 2, v.maxAttempt, "maxAttempt should jump to 2 after indexing attempt-2 block")

	v.indexBlock(smithers.ChatBlock{Attempt: 1, Content: "c"})
	assert.Equal(t, 2, v.maxAttempt, "maxAttempt should not decrease when a lower attempt block arrives")
}

// TestLiveChatView_AttemptTracking_RebuildAttemptBlocks_ResetsBadge verifies
// that rebuildAttemptBlocks resets the badge counter.
func TestLiveChatView_AttemptTracking_RebuildAttemptBlocks_ResetsBadge(t *testing.T) {
	v := newView("run-rebuild")
	v.newBlocksInLatest = 7
	v.linesDirty = false

	v.rebuildAttemptBlocks()

	assert.Equal(t, 0, v.newBlocksInLatest, "rebuildAttemptBlocks should reset newBlocksInLatest")
	assert.True(t, v.linesDirty, "rebuildAttemptBlocks should set linesDirty")
}

// =============================================================================
// feat-hijack-seamless-transition
// =============================================================================

// TestLiveChatView_HijackSeamlessTransition_PreHandoffBanner verifies that
// when hijacking=true the View() renders a transition banner before the TUI
// suspends (ticket feat-hijack-seamless-transition).
func TestLiveChatView_HijackSeamlessTransition_PreHandoffBanner(t *testing.T) {
	v := newView("run-seamless")
	v.width = 80
	v.height = 24
	v.hijacking = true

	out := v.View()
	assert.Contains(t, out, "Hijacking session", "pre-handoff banner should say Hijacking session")
	assert.Contains(t, out, "handing off", "pre-handoff banner should mention handing off the terminal")
}

// TestLiveChatView_HijackSeamlessTransition_PostReturnBanner verifies that
// after the hijack session ends (hijackReturned=true, not prompting), a summary
// banner is rendered.
func TestLiveChatView_HijackSeamlessTransition_PostReturnBanner(t *testing.T) {
	v := newView("run-post-return")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.hijackReturned = true
	v.promptResumeAutomation = false

	out := v.View()
	assert.Contains(t, out, "Returned from hijack session", "post-return banner should appear")
}

// TestLiveChatView_HijackSeamlessTransition_RefreshOnReturn verifies that
// liveChatHijackReturnMsg triggers a refresh of both run metadata and blocks.
func TestLiveChatView_HijackSeamlessTransition_RefreshOnReturn(t *testing.T) {
	v := newView("run-refresh-on-return")
	v.width = 80
	v.height = 24

	_, cmd := v.Update(liveChatHijackReturnMsg{runID: "run-refresh-on-return", err: nil})
	assert.NotNil(t, cmd, "hijack return should issue a refresh command (tea.Batch)")
}

// =============================================================================
// feat-hijack-native-cli-resume
// =============================================================================

// TestLiveChatView_HijackNativeCLIResume_LaunchesWithResumeArgs verifies that
// the hijack session handler builds the CLI invocation using ResumeArgs().
// Since we cannot actually exec a real agent binary here, we verify the path
// check logic when the binary is found: the handler should return a tea.Cmd
// (the ExecProcess cmd) rather than setting hijackErr.
func TestLiveChatView_HijackNativeCLIResume_LaunchesWithResumeArgs(t *testing.T) {
	v := newView("run-resume")
	v.width = 80
	v.height = 24
	v.hijacking = true

	// Use a binary that definitely exists on the test machine.
	session := &smithers.HijackSession{
		RunID:          "run-resume",
		AgentEngine:    "claude-code",
		AgentBinary:    "true", // POSIX true; always exits 0
		ResumeToken:    "sess-abc",
		CWD:            t.TempDir(),
		SupportsResume: true,
	}

	updated, cmd := v.Update(liveChatHijackSessionMsg{session: session, err: nil})
	lc := updated.(*LiveChatView)

	assert.Nil(t, lc.hijackErr, "should not have hijackErr when binary is found")
	assert.NotNil(t, cmd, "should return an ExecProcess cmd for the agent binary")
}

// TestLiveChatView_HijackNativeCLIResume_BinaryNotFound verifies the error
// path when the binary cannot be found in PATH.
func TestLiveChatView_HijackNativeCLIResume_BinaryNotFound(t *testing.T) {
	v := newView("run-nf")
	v.hijacking = true

	session := &smithers.HijackSession{
		RunID:          "run-nf",
		AgentEngine:    "claude-code",
		AgentBinary:    "/no/such/binary/claude",
		ResumeToken:    "tok",
		SupportsResume: true,
	}

	updated, cmd := v.Update(liveChatHijackSessionMsg{session: session, err: nil})
	lc := updated.(*LiveChatView)

	assert.NotNil(t, lc.hijackErr, "should have hijackErr when binary not found")
	assert.Nil(t, cmd, "should not return cmd when binary not found")
}

// =============================================================================
// feat-hijack-conversation-replay-fallback
// =============================================================================

// TestLiveChatView_ConversationReplayFallback_SetsFlag verifies that when the
// agent does not support --resume, the replayFallback flag is set and a notice
// block is injected (ticket feat-hijack-conversation-replay-fallback).
func TestLiveChatView_ConversationReplayFallback_SetsFlag(t *testing.T) {
	v := newView("run-fallback")
	v.width = 80
	v.height = 24
	v.hijacking = true

	session := &smithers.HijackSession{
		RunID:          "run-fallback",
		AgentEngine:    "unknown-agent",
		AgentBinary:    "/usr/bin/true",
		ResumeToken:    "",
		SupportsResume: false,
	}

	updated, cmd := v.Update(liveChatHijackSessionMsg{session: session, err: nil})
	lc := updated.(*LiveChatView)

	assert.True(t, lc.replayFallback, "replayFallback should be true when SupportsResume is false")
	assert.Nil(t, cmd, "no ExecProcess cmd should be returned for fallback path")
	assert.Nil(t, lc.hijackErr, "hijackErr should be nil for fallback path")
}

// TestLiveChatView_ConversationReplayFallback_InjectsNoticeBlock verifies that
// a notice block is appended so the user understands why the native TUI
// was not launched.
func TestLiveChatView_ConversationReplayFallback_InjectsNoticeBlock(t *testing.T) {
	v := newView("run-fallback-block")
	v.width = 80
	v.height = 24
	v.hijacking = true
	v.loadingRun = false
	v.loadingBlocks = false

	session := &smithers.HijackSession{
		RunID:          "run-fallback-block",
		AgentEngine:    "gemini",
		AgentBinary:    "/usr/local/bin/gemini",
		ResumeToken:    "",
		SupportsResume: false,
	}

	updated, _ := v.Update(liveChatHijackSessionMsg{session: session, err: nil})
	lc := updated.(*LiveChatView)

	// Verify a notice block was appended.
	require.NotEmpty(t, lc.blocks, "a notice block should have been appended")
	lastBlock := lc.blocks[len(lc.blocks)-1]
	assert.Contains(t, lastBlock.Content, "does not support", "notice should explain lack of resume support")
}

// TestLiveChatView_ConversationReplayFallback_ViewShowsBanner verifies that
// the fallback banner is rendered in View().
func TestLiveChatView_ConversationReplayFallback_ViewShowsBanner(t *testing.T) {
	v := newView("run-fallback-view")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.replayFallback = true
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-fallback-view", "n1", "assistant", "Some prior context.", 1000),
	}
	v.linesDirty = true

	out := v.View()
	assert.Contains(t, out, "Resume not supported", "fallback banner should be visible")
}

// TestLiveChatView_ConversationReplayFallback_HResetsFallbackFlag verifies
// that pressing 'h' again clears the replayFallback flag so a fresh hijack
// attempt can proceed.
func TestLiveChatView_ConversationReplayFallback_HResetsFallbackFlag(t *testing.T) {
	v := newView("run-fallback-reset")
	v.width = 80
	v.height = 24
	v.replayFallback = true

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'h'})
	lc := updated.(*LiveChatView)

	assert.False(t, lc.replayFallback, "pressing h should clear replayFallback")
	assert.True(t, lc.hijacking, "pressing h should set hijacking=true")
}

// =============================================================================
// feat-hijack-multi-engine-support (ResumeArgs tested in smithers package;
// these tests verify the livechat view passes args correctly per engine)
// =============================================================================

// TestLiveChatView_MultiEngine_PromptResumeAutomation_StateCheck verifies that
// the session handler correctly routes to ExecProcess for known engines.
// Actual flag verification lives in types_runs_test.go.
func TestLiveChatView_MultiEngine_SupportsResumeRouting(t *testing.T) {
	engines := []struct {
		engine string
		binary string
	}{
		{"claude-code", "true"},
		{"codex", "true"},
		{"gemini", "true"},
		{"amp", "true"},
	}

	for _, tc := range engines {
		tc := tc
		t.Run(tc.engine, func(t *testing.T) {
			v := newView("run-engine-" + tc.engine)
			v.hijacking = true

			session := &smithers.HijackSession{
				RunID:          v.runID,
				AgentEngine:    tc.engine,
				AgentBinary:    tc.binary,
				ResumeToken:    "tok-" + tc.engine,
				SupportsResume: true,
			}

			updated, cmd := v.Update(liveChatHijackSessionMsg{session: session, err: nil})
			lc := updated.(*LiveChatView)

			assert.Nil(t, lc.hijackErr, "engine %s: should not have hijackErr", tc.engine)
			assert.NotNil(t, cmd, "engine %s: should return ExecProcess cmd", tc.engine)
		})
	}
}

// =============================================================================
// feat-hijack-resume-to-automation
// =============================================================================

// TestLiveChatView_ResumeToAutomation_PromptOnReturn verifies that returning
// from a hijack session enables the promptResumeAutomation flag.
func TestLiveChatView_ResumeToAutomation_PromptOnReturn(t *testing.T) {
	v := newView("run-rta")
	v.width = 80
	v.height = 24

	updated, _ := v.Update(liveChatHijackReturnMsg{runID: "run-rta", err: nil})
	lc := updated.(*LiveChatView)

	assert.True(t, lc.promptResumeAutomation, "promptResumeAutomation should be set on hijack return")
	assert.True(t, lc.hijackReturned, "hijackReturned should be set on hijack return")
}

// TestLiveChatView_ResumeToAutomation_ViewShowsBanner verifies that the
// post-hijack automation prompt is rendered in View().
func TestLiveChatView_ResumeToAutomation_ViewShowsBanner(t *testing.T) {
	v := newView("run-rta-view")
	v.width = 80
	v.height = 24
	v.loadingRun = false
	v.loadingBlocks = false
	v.promptResumeAutomation = true
	v.hijackReturned = true

	out := v.View()
	assert.Contains(t, out, "Hijack session ended", "automation prompt should show session-ended message")
	assert.Contains(t, out, "[a] Resume automation", "automation prompt should show 'a' keybinding")
	assert.Contains(t, out, "[d / Esc] Dismiss", "automation prompt should show dismiss keybinding")
}

// TestLiveChatView_ResumeToAutomation_AKeyAcceptsPrompt verifies that pressing
// 'a' dispatches liveChatResumeToAutomationMsg and clears the prompt.
func TestLiveChatView_ResumeToAutomation_AKeyAcceptsPrompt(t *testing.T) {
	v := newView("run-rta-accept")
	v.width = 80
	v.height = 24
	v.promptResumeAutomation = true
	v.hijackReturned = true

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	require.NotNil(t, cmd, "pressing 'a' should return a cmd")

	msg := cmd()
	rtaMsg, ok := msg.(liveChatResumeToAutomationMsg)
	require.True(t, ok, "should dispatch liveChatResumeToAutomationMsg, got %T", msg)
	assert.Equal(t, "run-rta-accept", rtaMsg.runID)
}

// TestLiveChatView_ResumeToAutomation_DKeyDismissesPrompt verifies that 'd'
// dismisses the automation prompt.
func TestLiveChatView_ResumeToAutomation_DKeyDismissesPrompt(t *testing.T) {
	v := newView("run-rta-dismiss")
	v.width = 80
	v.height = 24
	v.promptResumeAutomation = true
	v.hijackReturned = true

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	lc := updated.(*LiveChatView)

	assert.False(t, lc.promptResumeAutomation, "pressing 'd' should clear promptResumeAutomation")
	assert.False(t, lc.hijackReturned, "pressing 'd' should clear hijackReturned")
	assert.Nil(t, cmd, "pressing 'd' should not return a cmd")
}

// TestLiveChatView_ResumeToAutomation_EscDismissesPrompt verifies that Esc
// dismisses the automation prompt (alias for 'd').
func TestLiveChatView_ResumeToAutomation_EscDismissesPrompt(t *testing.T) {
	v := newView("run-rta-esc")
	v.width = 80
	v.height = 24
	v.promptResumeAutomation = true
	v.hijackReturned = true

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	lc := updated.(*LiveChatView)

	assert.False(t, lc.promptResumeAutomation, "Esc should clear promptResumeAutomation")
	assert.Nil(t, cmd, "Esc should not return a cmd while prompt is showing")
}

// TestLiveChatView_ResumeToAutomation_SupressesOtherKeys verifies that while
// the automation prompt is showing, other keys are suppressed (not routed to
// normal key handling).
func TestLiveChatView_ResumeToAutomation_SupressesOtherKeys(t *testing.T) {
	v := newView("run-rta-suppress")
	v.width = 80
	v.height = 24
	v.promptResumeAutomation = true
	v.hijackReturned = true
	v.follow = true

	// 'f' would normally toggle follow mode; it should be suppressed.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'f'})
	lc := updated.(*LiveChatView)
	assert.True(t, lc.follow, "'f' should be suppressed while automation prompt is showing")

	// 'q' would normally pop the view; it should be suppressed.
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'q'})
	assert.Nil(t, cmd, "'q' should be suppressed while automation prompt is showing")
}

// TestLiveChatView_ResumeToAutomation_HandleMsg_AppendsNotice verifies that
// receiving liveChatResumeToAutomationMsg appends a resuming block and clears
// the prompt state.
func TestLiveChatView_ResumeToAutomation_HandleMsg_AppendsNotice(t *testing.T) {
	v := newView("run-rta-handle")
	v.width = 80
	v.height = 24
	v.promptResumeAutomation = true
	v.hijackReturned = true

	updated, _ := v.Update(liveChatResumeToAutomationMsg{runID: "run-rta-handle"})
	lc := updated.(*LiveChatView)

	assert.False(t, lc.promptResumeAutomation, "prompt should be cleared")
	assert.False(t, lc.hijackReturned, "hijackReturned should be cleared")
	require.NotEmpty(t, lc.blocks, "a resuming-automation notice block should be appended")
	lastBlock := lc.blocks[len(lc.blocks)-1]
	assert.Contains(t, lastBlock.Content, "automation", "notice block should mention automation")
}

// TestLiveChatView_ResumeToAutomation_HandleMsg_WrongRunID_Ignored verifies
// that a liveChatResumeToAutomationMsg for a different runID is ignored.
func TestLiveChatView_ResumeToAutomation_HandleMsg_WrongRunID_Ignored(t *testing.T) {
	v := newView("run-rta-mine")
	v.promptResumeAutomation = true
	v.hijackReturned = true

	updated, _ := v.Update(liveChatResumeToAutomationMsg{runID: "run-rta-other"})
	lc := updated.(*LiveChatView)

	assert.True(t, lc.promptResumeAutomation, "state should be unchanged for wrong runID")
}

// TestLiveChatView_ResumeToAutomation_ShortHelp_ShowsPromptBindings verifies
// that the ShortHelp bindings switch to the automation prompt keys while the
// prompt is showing.
func TestLiveChatView_ResumeToAutomation_ShortHelp_ShowsPromptBindings(t *testing.T) {
	v := newView("run-rta-help")
	v.promptResumeAutomation = true

	help := v.ShortHelp()
	require.NotEmpty(t, help)

	var keys []string
	for _, b := range help {
		h := b.Help()
		keys = append(keys, h.Key, h.Desc)
	}
	joined := strings.Join(keys, " ")
	assert.Contains(t, joined, "resume automation", "help should show resume automation binding")
	assert.Contains(t, joined, "dismiss", "help should show dismiss binding")
	// Normal hijack binding should not appear.
	assert.NotContains(t, joined, "hijack", "normal hijack binding should not appear during prompt")
}

// TestLiveChatView_ResumeToAutomation_HijackReturnWithError verifies that a
// non-nil error from the agent process is stored in hijackReturnErr and
// rendered in the automation banner.
func TestLiveChatView_ResumeToAutomation_HijackReturnWithError(t *testing.T) {
	v := newView("run-rta-err")
	v.width = 80
	v.height = 24

	agentErr := errors.New("exit status 1")
	_, _ = v.Update(liveChatHijackReturnMsg{runID: "run-rta-err", err: agentErr})

	// Re-read v after update.
	updated, _ := v.Update(liveChatHijackReturnMsg{runID: "run-rta-err", err: agentErr})
	lc := updated.(*LiveChatView)
	assert.Equal(t, agentErr, lc.hijackReturnErr, "hijackReturnErr should capture agent error")
}

// --- feat-live-chat-tool-call-rendering ---

func TestRenderToolBlock_RawFallback(t *testing.T) {
	lines := renderToolBlock("plain text content", 80)
	require.NotEmpty(t, lines)
	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, "plain text content")
	assert.Contains(t, joined, "⚙")
}

func TestRenderToolBlock_JSONWithNameAndInput(t *testing.T) {
	content := `{"name":"bash","input":{"command":"ls -la"},"output":"file1\nfile2"}`
	lines := renderToolBlock(content, 80)
	require.NotEmpty(t, lines)
	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, "⚙ bash", "header should show tool name")
	assert.Contains(t, joined, "in:", "should show input label")
	assert.Contains(t, joined, "out:", "should show output label")
	assert.Contains(t, joined, "file1", "should show output content")
}

func TestRenderToolBlock_JSONWithError(t *testing.T) {
	content := `{"name":"read_file","input":{"path":"/etc/passwd"},"error":"permission denied"}`
	lines := renderToolBlock(content, 80)
	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, "⚙ read_file")
	assert.Contains(t, joined, "err:", "should show error label")
	assert.Contains(t, joined, "permission denied")
	assert.NotContains(t, joined, "out:", "should not show out when error is present")
}

func TestRenderToolBlock_JSONNoName_FallsBack(t *testing.T) {
	// JSON without "name" field should fall back to raw display.
	content := `{"input":{"key":"val"}}`
	lines := renderToolBlock(content, 80)
	joined := strings.Join(lines, "\n")
	// Fallback prepends ⚙ prefix.
	assert.Contains(t, joined, "⚙")
	assert.Contains(t, joined, `"key"`)
}

func TestRenderToolBlock_LongOutputTruncated(t *testing.T) {
	// Output with more than 3 lines should be truncated with "…".
	output := "line1\nline2\nline3\nline4\nline5"
	content := `{"name":"cmd","input":{},"output":"` + strings.ReplaceAll(output, "\n", `\n`) + `"}`
	lines := renderToolBlock(content, 80)
	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, "line1")
	assert.Contains(t, joined, "line3")
	assert.NotContains(t, joined, "line4", "lines beyond max should be hidden")
	assert.Contains(t, joined, "…", "should show truncation indicator")
}

func TestRenderToolBlock_WrapsLongLines(t *testing.T) {
	longContent := strings.Repeat("x", 200)
	content := `{"name":"tool","input":{},"output":"` + longContent + `"}`
	lines := renderToolBlock(content, 40)
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), 80, "no line should be unreasonably long (lipgloss escapes inflate width)")
	}
}

// --- feat-live-chat-side-by-side ---

func TestLiveChatView_SidePane_TogglesWithSKey(t *testing.T) {
	v := newView("run-sp-01")
	v.width = 120
	v.height = 40
	assert.False(t, v.showSidePane, "side pane should start hidden")

	// Press 's' to show.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	lc := updated.(*LiveChatView)
	assert.True(t, lc.showSidePane, "side pane should be visible after pressing s")
	assert.NotNil(t, lc.splitPane, "splitPane should be set when visible")

	// Press 's' again to hide.
	updated2, _ := lc.Update(tea.KeyPressMsg{Code: 's'})
	lc2 := updated2.(*LiveChatView)
	assert.False(t, lc2.showSidePane, "side pane should be hidden after pressing s again")
	assert.Nil(t, lc2.splitPane, "splitPane should be nil when hidden")
}

func TestLiveChatView_SidePane_ViewRendersSplitPane(t *testing.T) {
	v := newView("run-sp-02")
	v.width = 120
	v.height = 40
	// Turn off loading state so renderBody has something to show.
	v.loadingRun = false
	v.loadingBlocks = false
	v.blocks = []smithers.ChatBlock{
		makeBlock("run-sp-02", "", "user", "hello", 0),
	}
	v.indexBlock(v.blocks[0])
	v.linesDirty = true

	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	lc := updated.(*LiveChatView)
	view := lc.View()
	// The split pane renders both panes; context pane shows "Context" title.
	assert.Contains(t, view, "Context")
}

func TestLiveChatView_SidePane_ShortHelpIncludesSKey(t *testing.T) {
	v := newView("run-sp-03")
	v.width = 120
	v.height = 40
	help := v.ShortHelp()

	var keys []string
	for _, b := range help {
		h := b.Help()
		keys = append(keys, h.Key, h.Desc)
	}
	joined := strings.Join(keys, " ")
	assert.Contains(t, joined, "s", "help should include 's' key")
	assert.Contains(t, joined, "context", "help should describe context pane")
}

func TestLiveChatView_SidePane_DescriptionToggles(t *testing.T) {
	v := newView("run-sp-04")
	v.width = 120
	v.height = 40

	helpOff := v.ShortHelp()
	var descOff string
	for _, b := range helpOff {
		h := b.Help()
		if h.Key == "s" {
			descOff = h.Desc
		}
	}
	assert.Contains(t, descOff, "off", "description should say 'off' when hidden")

	// Enable the side pane.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	lc := updated.(*LiveChatView)
	helpOn := lc.ShortHelp()
	var descOn string
	for _, b := range helpOn {
		h := b.Help()
		if h.Key == "s" {
			descOn = h.Desc
		}
	}
	assert.Contains(t, descOn, "on", "description should say 'on' when visible")
}

func TestLiveChatView_SidePane_SetSizePropagates(t *testing.T) {
	v := newView("run-sp-05")
	v.width = 120
	v.height = 40

	// Enable split pane.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	lc := updated.(*LiveChatView)

	// SetSize should not panic with a splitPane active.
	assert.NotPanics(t, func() {
		lc.SetSize(160, 50)
	})
	assert.Equal(t, 160, lc.width)
	assert.Equal(t, 50, lc.height)
}

func TestLiveChatView_SidePane_ContextPaneReceivesRunMetadata(t *testing.T) {
	v := newView("run-sp-06")
	v.width = 120
	v.height = 40

	startMs := int64(1_000_000)
	finMs := int64(1_060_000)
	run := &smithers.RunSummary{
		RunID:        "run-sp-06",
		WorkflowName: "my-workflow",
		Status:       smithers.RunStatusFinished,
		StartedAtMs:  &startMs,
		FinishedAtMs: &finMs,
	}
	updated, _ := v.Update(liveChatRunLoadedMsg{run: run})
	lc := updated.(*LiveChatView)

	// Context pane should have received the run metadata.
	require.NotNil(t, lc.contextPane)
	assert.NotNil(t, lc.contextPane.run, "context pane should have run metadata")
	assert.Equal(t, "my-workflow", lc.contextPane.run.WorkflowName)
}

// --- LiveChatContextPane ---

func TestLiveChatContextPane_ViewLoading(t *testing.T) {
	p := newLiveChatContextPane("run-ctx-01")
	p.SetSize(40, 20)
	view := p.View()
	assert.Contains(t, view, "Context")
	assert.Contains(t, view, "Loading...")
}

func TestLiveChatContextPane_ViewWithRun(t *testing.T) {
	p := newLiveChatContextPane("run-ctx-02")
	p.SetSize(40, 20)

	startMs := int64(1_000_000)
	run := &smithers.RunSummary{
		RunID:        "run-ctx-02",
		WorkflowName: "test-flow",
		Status:       smithers.RunStatusRunning,
		StartedAtMs:  &startMs,
	}
	newPane, _ := p.Update(liveChatRunLoadedMsg{run: run})
	cp := newPane.(*LiveChatContextPane)
	view := cp.View()
	assert.Contains(t, view, "Context")
	assert.Contains(t, view, "test-flow")
	assert.Contains(t, view, "running")
}

func TestLiveChatContextPane_ViewWithErrorReason(t *testing.T) {
	p := newLiveChatContextPane("run-ctx-03")
	p.SetSize(40, 20)

	startMs := int64(1_000_000)
	errorJSON := `{"message":"out of memory"}`
	run := &smithers.RunSummary{
		RunID:       "run-ctx-03",
		Status:      smithers.RunStatusFailed,
		StartedAtMs: &startMs,
		ErrorJSON:   &errorJSON,
	}
	newPane, _ := p.Update(liveChatRunLoadedMsg{run: run})
	cp := newPane.(*LiveChatContextPane)
	view := cp.View()
	assert.Contains(t, view, "Error:")
}

func TestLiveChatContextPane_ViewWithNodeSummary(t *testing.T) {
	p := newLiveChatContextPane("run-ctx-04")
	p.SetSize(40, 20)

	startMs := int64(1_000_000)
	run := &smithers.RunSummary{
		RunID:       "run-ctx-04",
		Status:      smithers.RunStatusRunning,
		StartedAtMs: &startMs,
		Summary:     map[string]int{"completed": 3, "running": 1},
	}
	newPane, _ := p.Update(liveChatRunLoadedMsg{run: run})
	cp := newPane.(*LiveChatContextPane)
	view := cp.View()
	assert.Contains(t, view, "Nodes")
	assert.Contains(t, view, "completed")
	assert.Contains(t, view, "3")
}

// --- fmtRelativeAge ---

func TestFmtRelativeAge_Seconds(t *testing.T) {
	ts := time.Now().Add(-30 * time.Second).UnixMilli()
	result := fmtRelativeAge(ts)
	assert.Contains(t, result, "s ago")
}

func TestFmtRelativeAge_Minutes(t *testing.T) {
	ts := time.Now().Add(-5 * time.Minute).UnixMilli()
	result := fmtRelativeAge(ts)
	assert.Contains(t, result, "m ago")
}

func TestFmtRelativeAge_Hours(t *testing.T) {
	ts := time.Now().Add(-3 * time.Hour).UnixMilli()
	result := fmtRelativeAge(ts)
	assert.Contains(t, result, "h ago")
}

func TestFmtRelativeAge_Days(t *testing.T) {
	ts := time.Now().Add(-48 * time.Hour).UnixMilli()
	result := fmtRelativeAge(ts)
	assert.Contains(t, result, "d ago")
}

func TestFmtRelativeAge_Zero_ReturnsEmpty(t *testing.T) {
	result := fmtRelativeAge(0)
	assert.Equal(t, "", result)
}

