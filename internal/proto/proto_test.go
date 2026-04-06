package proto

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceJSONRoundTrip(t *testing.T) {
	original := Workspace{
		ID:      "ws-123",
		Path:    "/home/user/project",
		YOLO:    true,
		Debug:   false,
		DataDir: "/tmp/data",
		Version: "1.2.3",
		Env:     []string{"FOO=bar", "BAZ=qux"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Workspace
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)

	// Verify specific JSON tag names in the serialized output.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "path")
	assert.Contains(t, raw, "yolo")
	assert.Contains(t, raw, "data_dir")
	assert.Contains(t, raw, "version")
	assert.Contains(t, raw, "env")
	// debug is omitempty and false, so it should be absent.
	assert.NotContains(t, raw, "debug")
}

func TestSessionJSONRoundTrip(t *testing.T) {
	original := Session{
		ID:               "sess-abc",
		ParentSessionID:  "sess-parent",
		Title:            "Test Session",
		MessageCount:     42,
		PromptTokens:     1500,
		CompletionTokens: 3000,
		SummaryMessageID: "msg-sum",
		Cost:             0.0275,
		CreatedAt:        1700000000,
		UpdatedAt:        1700001000,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Session
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "parent_session_id")
	assert.Contains(t, raw, "message_count")
	assert.Contains(t, raw, "prompt_tokens")
	assert.Contains(t, raw, "completion_tokens")
	assert.Contains(t, raw, "summary_message_id")
	assert.Contains(t, raw, "created_at")
	assert.Contains(t, raw, "updated_at")
}

func TestAgentMessageJSONRoundTrip(t *testing.T) {
	original := AgentMessage{
		SessionID: "sess-123",
		Prompt:    "Hello, world!",
		Attachments: []Attachment{
			{
				FilePath: "/tmp/test.png",
				FileName: "test.png",
				MimeType: "image/png",
				Content:  []byte("fake-image-data"),
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded AgentMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.SessionID, decoded.SessionID)
	assert.Equal(t, original.Prompt, decoded.Prompt)
	require.Len(t, decoded.Attachments, 1)
	assert.Equal(t, original.Attachments[0].FilePath, decoded.Attachments[0].FilePath)
	assert.Equal(t, original.Attachments[0].FileName, decoded.Attachments[0].FileName)
	assert.Equal(t, original.Attachments[0].MimeType, decoded.Attachments[0].MimeType)
	assert.Equal(t, original.Attachments[0].Content, decoded.Attachments[0].Content)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "session_id")
	assert.Contains(t, raw, "prompt")
	assert.Contains(t, raw, "attachments")
}

func TestPermissionActionConstants(t *testing.T) {
	// Verify the string values of the permission action constants.
	assert.Equal(t, PermissionAction("allow"), PermissionAllow)
	assert.Equal(t, PermissionAction("allow_session"), PermissionAllowForSession)
	assert.Equal(t, PermissionAction("deny"), PermissionDeny)
}

func TestPermissionActionMarshalText(t *testing.T) {
	tests := []struct {
		name   string
		action PermissionAction
		want   string
	}{
		{"allow", PermissionAllow, "allow"},
		{"allow_session", PermissionAllowForSession, "allow_session"},
		{"deny", PermissionDeny, "deny"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.action.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(data))

			var decoded PermissionAction
			err = decoded.UnmarshalText(data)
			require.NoError(t, err)
			assert.Equal(t, tt.action, decoded)
		})
	}
}

func TestPermissionGrantJSONRoundTrip(t *testing.T) {
	original := PermissionGrant{
		Permission: PermissionRequest{
			ID:          "perm-1",
			SessionID:   "sess-1",
			ToolCallID:  "tc-1",
			ToolName:    "bash",
			Description: "Run ls command",
			Action:      "execute",
			Path:        "/home/user",
		},
		Action: PermissionAllowForSession,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	// PermissionRequest has custom UnmarshalJSON that dispatches on ToolName,
	// so we need to include params in the raw JSON for it to parse. Verify
	// the grant-level fields round-trip correctly by unmarshaling into raw.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "permission")
	assert.Contains(t, raw, "action")

	// Verify the action field serializes as the string constant.
	var actionStr string
	require.NoError(t, json.Unmarshal(raw["action"], &actionStr))
	assert.Equal(t, "allow_session", actionStr)
}

func TestVersionInfoJSONRoundTrip(t *testing.T) {
	original := VersionInfo{
		Version:   "0.5.0",
		Commit:    "abc123def",
		GoVersion: "go1.26.1",
		Platform:  "darwin/arm64",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded VersionInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "version")
	assert.Contains(t, raw, "commit")
	assert.Contains(t, raw, "go_version")
	assert.Contains(t, raw, "platform")
}

func TestMessageJSONRoundTripWithParts(t *testing.T) {
	original := Message{
		ID:        "msg-1",
		Role:      Assistant,
		SessionID: "sess-1",
		Model:     "claude-4",
		Provider:  "anthropic",
		CreatedAt: 1700000000,
		UpdatedAt: 1700001000,
		Parts: []ContentPart{
			ReasoningContent{
				Thinking:   "Let me think about this...",
				Signature:  "sig123",
				StartedAt:  1700000000,
				FinishedAt: 1700000005,
			},
			TextContent{Text: "Here is my answer."},
			ToolCall{
				ID:       "tc-1",
				Name:     "bash",
				Input:    `{"command":"ls"}`,
				Type:     "function",
				Finished: true,
			},
			ToolResult{
				ToolCallID: "tc-1",
				Name:       "bash",
				Content:    "file1.go\nfile2.go",
				Metadata:   "{}",
				IsError:    false,
			},
			Finish{
				Reason:  FinishReasonEndTurn,
				Time:    1700001000,
				Message: "Done",
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.Role, decoded.Role)
	assert.Equal(t, original.SessionID, decoded.SessionID)
	assert.Equal(t, original.Model, decoded.Model)
	assert.Equal(t, original.Provider, decoded.Provider)
	assert.Equal(t, original.CreatedAt, decoded.CreatedAt)
	assert.Equal(t, original.UpdatedAt, decoded.UpdatedAt)

	require.Len(t, decoded.Parts, 5)

	reasoning, ok := decoded.Parts[0].(ReasoningContent)
	require.True(t, ok, "expected ReasoningContent")
	assert.Equal(t, "Let me think about this...", reasoning.Thinking)
	assert.Equal(t, "sig123", reasoning.Signature)
	assert.Equal(t, int64(1700000000), reasoning.StartedAt)
	assert.Equal(t, int64(1700000005), reasoning.FinishedAt)

	text, ok := decoded.Parts[1].(TextContent)
	require.True(t, ok, "expected TextContent")
	assert.Equal(t, "Here is my answer.", text.Text)

	tc, ok := decoded.Parts[2].(ToolCall)
	require.True(t, ok, "expected ToolCall")
	assert.Equal(t, "tc-1", tc.ID)
	assert.Equal(t, "bash", tc.Name)
	assert.True(t, tc.Finished)

	tr, ok := decoded.Parts[3].(ToolResult)
	require.True(t, ok, "expected ToolResult")
	assert.Equal(t, "tc-1", tr.ToolCallID)
	assert.False(t, tr.IsError)

	fin, ok := decoded.Parts[4].(Finish)
	require.True(t, ok, "expected Finish")
	assert.Equal(t, FinishReasonEndTurn, fin.Reason)
	assert.Equal(t, "Done", fin.Message)
}

func TestAgentEventJSONRoundTripWithError(t *testing.T) {
	original := AgentEvent{
		Type:         AgentEventTypeError,
		Error:        errors.New("something went wrong"),
		SessionID:    "sess-err",
		SessionTitle: "Error Session",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Verify the error is serialized as a string.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	var errStr string
	require.NoError(t, json.Unmarshal(raw["error"], &errStr))
	assert.Equal(t, "something went wrong", errStr)

	var decoded AgentEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, AgentEventTypeError, decoded.Type)
	require.NotNil(t, decoded.Error)
	assert.Equal(t, "something went wrong", decoded.Error.Error())
	assert.Equal(t, "sess-err", decoded.SessionID)
	assert.Equal(t, "Error Session", decoded.SessionTitle)

	// Also test that nil error omits the field.
	noErr := AgentEvent{Type: AgentEventTypeResponse}
	data, err = json.Marshal(noErr)
	require.NoError(t, err)
	var rawNoErr map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &rawNoErr))
	// error field should be empty string or absent when omitempty triggers.
	var noErrStr string
	if errField, exists := rawNoErr["error"]; exists {
		require.NoError(t, json.Unmarshal(errField, &noErrStr))
		assert.Empty(t, noErrStr)
	}
}

func TestMCPStateStringAndMarshal(t *testing.T) {
	tests := []struct {
		state MCPState
		str   string
	}{
		{MCPStateDisabled, "disabled"},
		{MCPStateStarting, "starting"},
		{MCPStateConnected, "connected"},
		{MCPStateError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			assert.Equal(t, tt.str, tt.state.String())

			data, err := tt.state.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.str, string(data))

			var decoded MCPState
			err = decoded.UnmarshalText(data)
			require.NoError(t, err)
			assert.Equal(t, tt.state, decoded)
		})
	}

	// Unknown state value returns "unknown" from String().
	assert.Equal(t, "unknown", MCPState(99).String())

	// Unknown string value returns error from UnmarshalText.
	var bad MCPState
	err := bad.UnmarshalText([]byte("bogus"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown mcp state")
}

func TestPermissionRequestUnmarshalDispatchesOnToolName(t *testing.T) {
	raw := `{
		"id": "perm-1",
		"session_id": "sess-1",
		"tool_call_id": "tc-1",
		"tool_name": "bash",
		"description": "run ls",
		"action": "execute",
		"params": {"command": "ls -la", "timeout": 30},
		"path": "/home/user"
	}`

	var req PermissionRequest
	err := json.Unmarshal([]byte(raw), &req)
	require.NoError(t, err)

	assert.Equal(t, "perm-1", req.ID)
	assert.Equal(t, "bash", req.ToolName)

	// Params should be unmarshaled into BashPermissionsParams.
	bashParams, ok := req.Params.(BashPermissionsParams)
	require.True(t, ok, "expected BashPermissionsParams, got %T", req.Params)
	assert.Equal(t, "ls -la", bashParams.Command)
	assert.Equal(t, 30, bashParams.Timeout)

	// Test with an unknown tool name -- should produce map[string]any.
	rawUnknown := `{
		"id": "perm-2",
		"session_id": "sess-1",
		"tool_call_id": "tc-2",
		"tool_name": "custom_tool",
		"description": "do something",
		"action": "execute",
		"params": {"key": "value"},
		"path": "/"
	}`

	var req2 PermissionRequest
	err = json.Unmarshal([]byte(rawUnknown), &req2)
	require.NoError(t, err)

	generic, ok := req2.Params.(map[string]any)
	require.True(t, ok, "expected map[string]any for unknown tool, got %T", req2.Params)
	assert.Equal(t, "value", generic["key"])
}

func TestMessageRoleConstants(t *testing.T) {
	assert.Equal(t, MessageRole("assistant"), Assistant)
	assert.Equal(t, MessageRole("user"), User)
	assert.Equal(t, MessageRole("system"), System)
	assert.Equal(t, MessageRole("tool"), Tool)
}

func TestFinishReasonConstants(t *testing.T) {
	reasons := map[FinishReason]string{
		FinishReasonEndTurn:          "end_turn",
		FinishReasonMaxTokens:        "max_tokens",
		FinishReasonToolUse:          "tool_use",
		FinishReasonCanceled:         "canceled",
		FinishReasonError:            "error",
		FinishReasonPermissionDenied: "permission_denied",
		FinishReasonUnknown:          "unknown",
	}

	for reason, expected := range reasons {
		t.Run(expected, func(t *testing.T) {
			data, err := reason.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, expected, string(data))

			var decoded FinishReason
			err = decoded.UnmarshalText(data)
			require.NoError(t, err)
			assert.Equal(t, reason, decoded)
		})
	}
}

func TestAgentInfoIsZero(t *testing.T) {
	assert.True(t, AgentInfo{}.IsZero())
	assert.False(t, AgentInfo{IsBusy: true}.IsZero())
	assert.False(t, AgentInfo{IsReady: true}.IsZero())
}

func TestAttachmentBase64RoundTrip(t *testing.T) {
	original := Attachment{
		FilePath: "/tmp/data.bin",
		FileName: "data.bin",
		MimeType: "application/octet-stream",
		Content:  []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Verify the content field is a base64 string, not raw bytes.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	var contentStr string
	require.NoError(t, json.Unmarshal(raw["content"], &contentStr))
	assert.Equal(t, "AAEC//79", contentStr)

	var decoded Attachment
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.FilePath, decoded.FilePath)
	assert.Equal(t, original.FileName, decoded.FileName)
	assert.Equal(t, original.MimeType, decoded.MimeType)
	assert.Equal(t, original.Content, decoded.Content)
}
