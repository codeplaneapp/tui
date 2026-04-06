package message

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allPartTypes returns one instance of every ContentPart implementation.
func allPartTypes() []ContentPart {
	return []ContentPart{
		ReasoningContent{
			Thinking:  "deep thought",
			Signature: "sig-abc",
			StartedAt: 1000,
		},
		TextContent{Text: "hello world"},
		ImageURLContent{URL: "https://example.com/img.png", Detail: "high"},
		BinaryContent{Path: "/tmp/bin", MIMEType: "application/octet-stream", Data: []byte{0xDE, 0xAD}},
		ToolCall{ID: "tc-1", Name: "bash", Input: `{"cmd":"ls"}`, Finished: true},
		ToolResult{ToolCallID: "tc-1", Name: "bash", Content: "file.txt", IsError: false},
		Finish{Reason: FinishReasonEndTurn, Time: 9999, Message: "done"},
	}
}

func TestMarshalUnmarshalParts_AllTypes(t *testing.T) {
	original := allPartTypes()

	data, err := marshalParts(original)
	require.NoError(t, err)

	restored, err := unmarshalParts(data)
	require.NoError(t, err)

	require.Len(t, restored, len(original))

	// ReasoningContent
	rc, ok := restored[0].(ReasoningContent)
	require.True(t, ok, "part 0 should be ReasoningContent")
	assert.Equal(t, "deep thought", rc.Thinking)
	assert.Equal(t, "sig-abc", rc.Signature)
	assert.Equal(t, int64(1000), rc.StartedAt)

	// TextContent
	tc, ok := restored[1].(TextContent)
	require.True(t, ok, "part 1 should be TextContent")
	assert.Equal(t, "hello world", tc.Text)

	// ImageURLContent
	img, ok := restored[2].(ImageURLContent)
	require.True(t, ok, "part 2 should be ImageURLContent")
	assert.Equal(t, "https://example.com/img.png", img.URL)
	assert.Equal(t, "high", img.Detail)

	// BinaryContent — note: BinaryContent fields are not exported with json
	// tags, so only the exported fields that json can see will survive.
	bc, ok := restored[3].(BinaryContent)
	require.True(t, ok, "part 3 should be BinaryContent")
	// BinaryContent has no json tags, so fields serialize by name.
	assert.Equal(t, "/tmp/bin", bc.Path)
	assert.Equal(t, "application/octet-stream", bc.MIMEType)
	assert.Equal(t, []byte{0xDE, 0xAD}, bc.Data)

	// ToolCall
	toolCall, ok := restored[4].(ToolCall)
	require.True(t, ok, "part 4 should be ToolCall")
	assert.Equal(t, "tc-1", toolCall.ID)
	assert.Equal(t, "bash", toolCall.Name)
	assert.Equal(t, `{"cmd":"ls"}`, toolCall.Input)
	assert.True(t, toolCall.Finished)

	// ToolResult
	toolRes, ok := restored[5].(ToolResult)
	require.True(t, ok, "part 5 should be ToolResult")
	assert.Equal(t, "tc-1", toolRes.ToolCallID)
	assert.Equal(t, "bash", toolRes.Name)
	assert.Equal(t, "file.txt", toolRes.Content)
	assert.False(t, toolRes.IsError)

	// Finish
	fin, ok := restored[6].(Finish)
	require.True(t, ok, "part 6 should be Finish")
	assert.Equal(t, FinishReasonEndTurn, fin.Reason)
	assert.Equal(t, int64(9999), fin.Time)
	assert.Equal(t, "done", fin.Message)
}

func TestUnmarshalParts_UnknownType(t *testing.T) {
	raw := `[{"type":"alien","data":{"x":1}}]`
	_, err := unmarshalParts([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown part type")
}

func TestMarshalParts_EmptySlice(t *testing.T) {
	data, err := marshalParts([]ContentPart{})
	require.NoError(t, err)

	// Should produce a valid JSON array.
	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &arr))
	assert.Len(t, arr, 0)
	assert.Equal(t, "[]", string(data))
}

// badPart is a ContentPart that marshalParts does not know about.
type badPart struct{}

func (badPart) isPart() {}

func TestMarshalParts_UnknownPartType(t *testing.T) {
	parts := []ContentPart{badPart{}}
	_, err := marshalParts(parts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown part type")
}

func TestMessage_Clone_IsolatesSlice(t *testing.T) {
	original := Message{
		ID:   "msg-1",
		Role: Assistant,
		Parts: []ContentPart{
			TextContent{Text: "first"},
		},
	}

	clone := original.Clone()

	// Mutate the clone's Parts slice.
	clone.Parts = append(clone.Parts, TextContent{Text: "second"})

	// Original must be unaffected.
	require.Len(t, original.Parts, 1)
	assert.Equal(t, TextContent{Text: "first"}, original.Parts[0])

	// Clone has both parts.
	require.Len(t, clone.Parts, 2)
}

func TestMessage_ToolCalls_Returns_Only_ToolCalls(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			TextContent{Text: "preamble"},
			ToolCall{ID: "tc-1", Name: "read_file", Input: `{"path":"/a"}`, Finished: true},
			ReasoningContent{Thinking: "hmm"},
			ToolCall{ID: "tc-2", Name: "write_file", Input: `{"path":"/b"}`, Finished: false},
			Finish{Reason: FinishReasonToolUse},
		},
	}

	calls := msg.ToolCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, "tc-1", calls[0].ID)
	assert.Equal(t, "tc-2", calls[1].ID)
}

func TestMessage_TextContent_Concatenation(t *testing.T) {
	// Content() returns only the *first* TextContent part found.
	// If the caller needs concatenation, they should use AppendContent to
	// accumulate into a single TextContent part. This test verifies both
	// the single-return behavior and that AppendContent merges correctly.

	t.Run("Content returns first TextContent", func(t *testing.T) {
		msg := Message{
			Parts: []ContentPart{
				ReasoningContent{Thinking: "ignore me"},
				TextContent{Text: "alpha"},
				TextContent{Text: "beta"},
				ToolCall{ID: "tc-1", Name: "x"},
			},
		}
		got := msg.Content()
		assert.Equal(t, "alpha", got.Text, "Content() should return the first TextContent")
	})

	t.Run("AppendContent concatenates into single TextContent", func(t *testing.T) {
		msg := Message{
			Parts: []ContentPart{
				TextContent{Text: "hello"},
			},
		}
		msg.AppendContent(" world")
		msg.AppendContent("!")

		got := msg.Content()
		assert.Equal(t, "hello world!", got.Text)
	})

	t.Run("Content returns empty when no TextContent", func(t *testing.T) {
		msg := Message{
			Parts: []ContentPart{
				ToolCall{ID: "tc-1", Name: "bash"},
				Finish{Reason: FinishReasonEndTurn},
			},
		}
		got := msg.Content()
		assert.Equal(t, "", got.Text)
	})
}
