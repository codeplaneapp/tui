package workspace

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent/notify"
	mcp "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/history"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- protoToSession / sessionToProto round-trip --

func TestProtoToSession_AllFields(t *testing.T) {
	ps := proto.Session{
		ID:               "sess-1",
		ParentSessionID:  "parent-1",
		Title:            "My Session",
		SummaryMessageID: "summary-42",
		MessageCount:     10,
		PromptTokens:     500,
		CompletionTokens: 200,
		Cost:             0.05,
		CreatedAt:        1000,
		UpdatedAt:        2000,
	}

	s := protoToSession(ps)
	assert.Equal(t, "sess-1", s.ID)
	assert.Equal(t, "parent-1", s.ParentSessionID)
	assert.Equal(t, "My Session", s.Title)
	assert.Equal(t, "summary-42", s.SummaryMessageID)
	assert.Equal(t, int64(10), s.MessageCount)
	assert.Equal(t, int64(500), s.PromptTokens)
	assert.Equal(t, int64(200), s.CompletionTokens)
	assert.Equal(t, 0.05, s.Cost)
	assert.Equal(t, int64(1000), s.CreatedAt)
	assert.Equal(t, int64(2000), s.UpdatedAt)
}

func TestSessionToProto_RoundTrip(t *testing.T) {
	original := session.Session{
		ID:               "sess-rt",
		ParentSessionID:  "parent-rt",
		Title:            "Round Trip",
		SummaryMessageID: "summ-rt",
		MessageCount:     7,
		PromptTokens:     300,
		CompletionTokens: 150,
		Cost:             0.03,
		CreatedAt:        111,
		UpdatedAt:        222,
	}

	ps := sessionToProto(original)
	restored := protoToSession(ps)

	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.ParentSessionID, restored.ParentSessionID)
	assert.Equal(t, original.Title, restored.Title)
	assert.Equal(t, original.SummaryMessageID, restored.SummaryMessageID)
	assert.Equal(t, original.MessageCount, restored.MessageCount)
	assert.Equal(t, original.PromptTokens, restored.PromptTokens)
	assert.Equal(t, original.CompletionTokens, restored.CompletionTokens)
	assert.Equal(t, original.Cost, restored.Cost)
	assert.Equal(t, original.CreatedAt, restored.CreatedAt)
	assert.Equal(t, original.UpdatedAt, restored.UpdatedAt)
}

// -- protoToFile --

func TestProtoToFile(t *testing.T) {
	pf := proto.File{
		ID:        "file-1",
		SessionID: "sess-1",
		Path:      "/src/main.go",
		Content:   "package main",
		Version:   3,
		CreatedAt: 100,
		UpdatedAt: 200,
	}

	f := protoToFile(pf)
	assert.Equal(t, "file-1", f.ID)
	assert.Equal(t, "sess-1", f.SessionID)
	assert.Equal(t, "/src/main.go", f.Path)
	assert.Equal(t, "package main", f.Content)
	assert.Equal(t, int64(3), f.Version)
	assert.Equal(t, int64(100), f.CreatedAt)
	assert.Equal(t, int64(200), f.UpdatedAt)
}

func TestProtoToFiles_Batch(t *testing.T) {
	files := []proto.File{
		{ID: "f1", Path: "/a.go"},
		{ID: "f2", Path: "/b.go"},
	}
	result := protoToFiles(files)
	require.Len(t, result, 2)
	assert.Equal(t, "f1", result[0].ID)
	assert.Equal(t, "f2", result[1].ID)
}

// -- protoToMessage with various part types --

func TestProtoToMessage_AllPartTypes(t *testing.T) {
	pm := proto.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      proto.Assistant,
		Model:     "claude-3",
		Provider:  "anthropic",
		CreatedAt: 100,
		UpdatedAt: 200,
		Parts: []proto.ContentPart{
			proto.TextContent{Text: "Hello"},
			proto.ReasoningContent{Thinking: "hmm", Signature: "sig", StartedAt: 10, FinishedAt: 20},
			proto.ToolCall{ID: "tc-1", Name: "bash", Input: `{"cmd":"ls"}`, Finished: true},
			proto.ToolResult{ToolCallID: "tc-1", Name: "bash", Content: "file.go", IsError: false},
			proto.Finish{Reason: proto.FinishReasonEndTurn, Time: 300, Message: "done", Details: "ok"},
			proto.ImageURLContent{URL: "https://img.png", Detail: "high"},
			proto.BinaryContent{Path: "/tmp/f", MIMEType: "image/png", Data: []byte{0x89}},
		},
	}

	m := protoToMessage(pm)

	assert.Equal(t, "msg-1", m.ID)
	assert.Equal(t, "sess-1", m.SessionID)
	assert.Equal(t, message.MessageRole("assistant"), m.Role)
	assert.Equal(t, "claude-3", m.Model)
	assert.Equal(t, "anthropic", m.Provider)
	assert.Equal(t, int64(100), m.CreatedAt)
	assert.Equal(t, int64(200), m.UpdatedAt)

	require.Len(t, m.Parts, 7)

	// TextContent
	tc, ok := m.Parts[0].(message.TextContent)
	require.True(t, ok)
	assert.Equal(t, "Hello", tc.Text)

	// ReasoningContent
	rc, ok := m.Parts[1].(message.ReasoningContent)
	require.True(t, ok)
	assert.Equal(t, "hmm", rc.Thinking)
	assert.Equal(t, "sig", rc.Signature)
	assert.Equal(t, int64(10), rc.StartedAt)
	assert.Equal(t, int64(20), rc.FinishedAt)

	// ToolCall
	tcall, ok := m.Parts[2].(message.ToolCall)
	require.True(t, ok)
	assert.Equal(t, "tc-1", tcall.ID)
	assert.Equal(t, "bash", tcall.Name)
	assert.True(t, tcall.Finished)

	// ToolResult
	tr, ok := m.Parts[3].(message.ToolResult)
	require.True(t, ok)
	assert.Equal(t, "tc-1", tr.ToolCallID)
	assert.Equal(t, "file.go", tr.Content)
	assert.False(t, tr.IsError)

	// Finish
	fin, ok := m.Parts[4].(message.Finish)
	require.True(t, ok)
	assert.Equal(t, message.FinishReason("end_turn"), fin.Reason)
	assert.Equal(t, "done", fin.Message)

	// ImageURLContent
	img, ok := m.Parts[5].(message.ImageURLContent)
	require.True(t, ok)
	assert.Equal(t, "https://img.png", img.URL)
	assert.Equal(t, "high", img.Detail)

	// BinaryContent
	bin, ok := m.Parts[6].(message.BinaryContent)
	require.True(t, ok)
	assert.Equal(t, "/tmp/f", bin.Path)
	assert.Equal(t, "image/png", bin.MIMEType)
	assert.Equal(t, []byte{0x89}, bin.Data)
}

func TestProtoToMessages_EmptySlice(t *testing.T) {
	result := protoToMessages(nil)
	require.Len(t, result, 0)
}

// -- translateEvent --

func TestTranslateEvent_LSPEvent(t *testing.T) {
	input := pubsub.Event[proto.LSPEvent]{
		Type: pubsub.UpdatedEvent,
		Payload: proto.LSPEvent{
			Type:            proto.LSPEventStateChanged,
			Name:            "gopls",
			State:           lsp.StateReady,
			Error:           errors.New("test error"),
			DiagnosticCount: 5,
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[LSPEvent])
	require.True(t, ok)
	assert.Equal(t, pubsub.UpdatedEvent, ev.Type)
	assert.Equal(t, LSPEventStateChanged, ev.Payload.Type)
	assert.Equal(t, "gopls", ev.Payload.Name)
	assert.Equal(t, lsp.StateReady, ev.Payload.State)
	assert.EqualError(t, ev.Payload.Error, "test error")
	assert.Equal(t, 5, ev.Payload.DiagnosticCount)
}

func TestTranslateEvent_MCPEvent(t *testing.T) {
	input := pubsub.Event[proto.MCPEvent]{
		Type: pubsub.CreatedEvent,
		Payload: proto.MCPEvent{
			Type:          proto.MCPEventToolsListChanged,
			Name:          "docker-mcp",
			State:         proto.MCPStateConnected,
			ToolCount:     3,
			PromptCount:   1,
			ResourceCount: 2,
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[mcp.Event])
	require.True(t, ok)
	assert.Equal(t, mcp.EventToolsListChanged, ev.Payload.Type)
	assert.Equal(t, "docker-mcp", ev.Payload.Name)
	assert.Equal(t, mcp.State(proto.MCPStateConnected), ev.Payload.State)
	assert.Equal(t, 3, ev.Payload.Counts.Tools)
	assert.Equal(t, 1, ev.Payload.Counts.Prompts)
	assert.Equal(t, 2, ev.Payload.Counts.Resources)
}

func TestTranslateEvent_PermissionRequest(t *testing.T) {
	input := pubsub.Event[proto.PermissionRequest]{
		Type: pubsub.CreatedEvent,
		Payload: proto.PermissionRequest{
			ID:          "perm-1",
			SessionID:   "sess-1",
			ToolCallID:  "tc-1",
			ToolName:    "bash",
			Description: "run ls",
			Action:      "execute",
			Path:        "/tmp",
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[permission.PermissionRequest])
	require.True(t, ok)
	assert.Equal(t, "perm-1", ev.Payload.ID)
	assert.Equal(t, "sess-1", ev.Payload.SessionID)
	assert.Equal(t, "bash", ev.Payload.ToolName)
	assert.Equal(t, "run ls", ev.Payload.Description)
}

func TestTranslateEvent_PermissionNotification(t *testing.T) {
	input := pubsub.Event[proto.PermissionNotification]{
		Type: pubsub.UpdatedEvent,
		Payload: proto.PermissionNotification{
			ToolCallID: "tc-1",
			Granted:    true,
			Denied:     false,
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[permission.PermissionNotification])
	require.True(t, ok)
	assert.Equal(t, "tc-1", ev.Payload.ToolCallID)
	assert.True(t, ev.Payload.Granted)
	assert.False(t, ev.Payload.Denied)
}

func TestTranslateEvent_SessionEvent(t *testing.T) {
	input := pubsub.Event[proto.Session]{
		Type: pubsub.CreatedEvent,
		Payload: proto.Session{
			ID:    "sess-new",
			Title: "New Session",
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[session.Session])
	require.True(t, ok)
	assert.Equal(t, "sess-new", ev.Payload.ID)
	assert.Equal(t, "New Session", ev.Payload.Title)
}

func TestTranslateEvent_FileEvent(t *testing.T) {
	input := pubsub.Event[proto.File]{
		Type: pubsub.CreatedEvent,
		Payload: proto.File{
			ID:        "file-1",
			SessionID: "sess-1",
			Path:      "/main.go",
			Content:   "package main",
			Version:   1,
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[history.File])
	require.True(t, ok)
	assert.Equal(t, "file-1", ev.Payload.ID)
	assert.Equal(t, "/main.go", ev.Payload.Path)
}

func TestTranslateEvent_AgentEvent(t *testing.T) {
	input := pubsub.Event[proto.AgentEvent]{
		Type: pubsub.UpdatedEvent,
		Payload: proto.AgentEvent{
			Type:         proto.AgentEventTypeResponse,
			SessionID:    "sess-1",
			SessionTitle: "My Chat",
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[notify.Notification])
	require.True(t, ok)
	assert.Equal(t, "sess-1", ev.Payload.SessionID)
	assert.Equal(t, "My Chat", ev.Payload.SessionTitle)
	assert.Equal(t, notify.Type("response"), ev.Payload.Type)
}

func TestTranslateEvent_UnknownType_ReturnsNil(t *testing.T) {
	result := translateEvent("something unexpected")
	assert.Nil(t, result)
}

// -- protoToMCPEventType --

func TestProtoToMCPEventType(t *testing.T) {
	tests := []struct {
		input    proto.MCPEventType
		expected mcp.EventType
	}{
		{proto.MCPEventStateChanged, mcp.EventStateChanged},
		{proto.MCPEventToolsListChanged, mcp.EventToolsListChanged},
		{proto.MCPEventPromptsListChanged, mcp.EventPromptsListChanged},
		{proto.MCPEventResourcesListChanged, mcp.EventResourcesListChanged},
		{proto.MCPEventType("unknown"), mcp.EventStateChanged}, // default case
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.expected, protoToMCPEventType(tt.input))
		})
	}
}

// -- ClientWorkspace agent tool session helpers --

func TestClientWorkspace_CreateAgentToolSessionID(t *testing.T) {
	cw := &ClientWorkspace{}
	id := cw.CreateAgentToolSessionID("msg-123", "tool-456")
	assert.Equal(t, "msg-123$$tool-456", id)
}

func TestClientWorkspace_ParseAgentToolSessionID(t *testing.T) {
	cw := &ClientWorkspace{}

	t.Run("valid", func(t *testing.T) {
		msgID, toolID, ok := cw.ParseAgentToolSessionID("msg-123$$tool-456")
		require.True(t, ok)
		assert.Equal(t, "msg-123", msgID)
		assert.Equal(t, "tool-456", toolID)
	})

	t.Run("invalid_no_separator", func(t *testing.T) {
		_, _, ok := cw.ParseAgentToolSessionID("no-separator")
		assert.False(t, ok)
	})

	t.Run("invalid_too_many_parts", func(t *testing.T) {
		_, _, ok := cw.ParseAgentToolSessionID("a$$b$$c")
		assert.False(t, ok)
	})
}

// -- LSPGetDiagnosticCounts severity mapping --

// -- Config and WorkingDir on ClientWorkspace --

func TestClientWorkspace_Config_ReturnsFromCachedWorkspace(t *testing.T) {
	cw := NewClientWorkspace(nil, proto.Workspace{
		ID:   "ws-1",
		Path: "/home/user/project",
	})
	assert.Equal(t, "/home/user/project", cw.WorkingDir())
}

// -- protoToMessages with content --

func TestProtoToMessage_NoParts(t *testing.T) {
	pm := proto.Message{
		ID:        "msg-empty",
		SessionID: "sess-1",
		Role:      proto.User,
	}

	m := protoToMessage(pm)
	assert.Equal(t, "msg-empty", m.ID)
	assert.Empty(t, m.Parts)
}

// -- NewClientWorkspace caches workspace snapshot --

func TestNewClientWorkspace_CachesWorkspace(t *testing.T) {
	ws := proto.Workspace{
		ID:   "ws-cached",
		Path: "/cached/path",
	}
	cw := NewClientWorkspace(nil, ws)
	assert.Equal(t, "ws-cached", cw.workspaceID())
	assert.Equal(t, "/cached/path", cw.WorkingDir())
}


// -- Verify Workspace interface is satisfied for both implementations --

func TestClientWorkspace_ImplementsWorkspace(t *testing.T) {
	// Compile-time verification (also done in source via var _), but this
	// makes it explicit in the test suite.
	var _ Workspace = (*ClientWorkspace)(nil)
}

// -- parseAgentToolSessionID edge cases on ClientWorkspace --

func TestClientWorkspace_ParseAgentToolSessionID_Empty(t *testing.T) {
	cw := &ClientWorkspace{}
	_, _, ok := cw.ParseAgentToolSessionID("")
	assert.False(t, ok)
}

// -- formatToolSessionID --

func TestClientWorkspace_CreateAgentToolSessionID_EmptyParts(t *testing.T) {
	cw := &ClientWorkspace{}
	id := cw.CreateAgentToolSessionID("", "")
	assert.Equal(t, "$$", id)
}

// -- protoToMessage preserves message-level fields when no parts --

func TestProtoToMessage_MessageMetadata(t *testing.T) {
	now := time.Now().Unix()
	pm := proto.Message{
		ID:        "msg-meta",
		SessionID: "sess-meta",
		Role:      proto.Tool,
		Model:     "gpt-4o",
		Provider:  "openai",
		CreatedAt: now,
		UpdatedAt: now + 1,
	}

	m := protoToMessage(pm)
	assert.Equal(t, message.MessageRole("tool"), m.Role)
	assert.Equal(t, "gpt-4o", m.Model)
	assert.Equal(t, "openai", m.Provider)
	assert.Equal(t, now, m.CreatedAt)
	assert.Equal(t, now+1, m.UpdatedAt)
}

// -- protoToMessages batch --

func TestProtoToMessages_Batch(t *testing.T) {
	msgs := []proto.Message{
		{ID: "m1", Role: proto.User},
		{ID: "m2", Role: proto.Assistant, Parts: []proto.ContentPart{proto.TextContent{Text: "hi"}}},
		{ID: "m3", Role: proto.Tool},
	}
	result := protoToMessages(msgs)
	require.Len(t, result, 3)
	assert.Equal(t, "m1", result[0].ID)
	assert.Equal(t, "m2", result[1].ID)
	assert.Equal(t, "m3", result[2].ID)

	// Verify the second message has the text part converted.
	require.Len(t, result[1].Parts, 1)
	tc, ok := result[1].Parts[0].(message.TextContent)
	require.True(t, ok)
	assert.Equal(t, "hi", tc.Text)
}

// -- translateEvent for message event --

func TestTranslateEvent_MessageEvent(t *testing.T) {
	input := pubsub.Event[proto.Message]{
		Type: pubsub.UpdatedEvent,
		Payload: proto.Message{
			ID:        "msg-streamed",
			SessionID: "sess-1",
			Role:      proto.Assistant,
			Parts:     []proto.ContentPart{proto.TextContent{Text: "hello world"}},
		},
	}

	result := translateEvent(input)
	require.NotNil(t, result)

	ev, ok := result.(pubsub.Event[message.Message])
	require.True(t, ok)
	assert.Equal(t, pubsub.UpdatedEvent, ev.Type)
	assert.Equal(t, "msg-streamed", ev.Payload.ID)
	require.Len(t, ev.Payload.Parts, 1)

	tc, ok := ev.Payload.Parts[0].(message.TextContent)
	require.True(t, ok)
	assert.Equal(t, "hello world", tc.Text)
}

// -- CreateAgentToolSessionID format --

func TestClientWorkspace_CreateAgentToolSessionID_Format(t *testing.T) {
	cw := &ClientWorkspace{}

	tests := []struct {
		messageID  string
		toolCallID string
		expected   string
	}{
		{"msg1", "tc1", "msg1$$tc1"},
		{"", "tc1", "$$tc1"},
		{"msg1", "", "msg1$$"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.messageID, tt.toolCallID), func(t *testing.T) {
			assert.Equal(t, tt.expected, cw.CreateAgentToolSessionID(tt.messageID, tt.toolCallID))
		})
	}
}
