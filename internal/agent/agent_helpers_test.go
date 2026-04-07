package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/openrouter"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMinimalAgent creates a bare sessionAgent suitable for testing
// pure/helper methods that don't require a DB or running model.
func newMinimalAgent() *sessionAgent {
	return &sessionAgent{
		largeModel:   csync.NewValue(Model{}),
		smallModel:   csync.NewValue(Model{}),
		systemPrompt: csync.NewValue(""),
		systemPromptPrefix: csync.NewValue(""),
		tools:          csync.NewSliceFrom[fantasy.AgentTool](nil),
		messageQueue:   csync.NewMap[string, []SessionAgentCall](),
		activeRequests: csync.NewMap[string, context.CancelFunc](),
	}
}

// ---------------------------------------------------------------------------
// buildSummaryPrompt
// ---------------------------------------------------------------------------

func TestBuildSummaryPrompt_NoTodos(t *testing.T) {
	t.Parallel()
	result := buildSummaryPrompt(nil)
	assert.Equal(t, "Provide a detailed summary of our conversation above.", result)
}

func TestBuildSummaryPrompt_WithTodos(t *testing.T) {
	t.Parallel()
	todos := []session.Todo{
		{Status: session.TodoStatusPending, Content: "Write unit tests"},
		{Status: session.TodoStatusCompleted, Content: "Set up CI"},
		{Status: session.TodoStatusInProgress, Content: "Refactor auth"},
	}
	result := buildSummaryPrompt(todos)

	assert.Contains(t, result, "Provide a detailed summary of our conversation above.")
	assert.Contains(t, result, "## Current Todo List")
	assert.Contains(t, result, "- [pending] Write unit tests")
	assert.Contains(t, result, "- [completed] Set up CI")
	assert.Contains(t, result, "- [in_progress] Refactor auth")
	assert.Contains(t, result, "Instruct the resuming assistant to use the `todos` tool")
}

func TestBuildSummaryPrompt_EmptySlice(t *testing.T) {
	t.Parallel()
	// An empty (non-nil) slice should behave like nil: no todo section.
	result := buildSummaryPrompt([]session.Todo{})
	assert.NotContains(t, result, "## Current Todo List")
}

// ---------------------------------------------------------------------------
// openrouterCost
// ---------------------------------------------------------------------------

func TestOpenrouterCost_WithOpenRouterMetadata(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	metadata := fantasy.ProviderMetadata{
		openrouter.Name: &openrouter.ProviderMetadata{
			Usage: openrouter.UsageAccounting{
				Cost: 0.0042,
			},
		},
	}
	cost := a.openrouterCost(metadata)
	require.NotNil(t, cost)
	assert.InDelta(t, 0.0042, *cost, 1e-9)
}

func TestOpenrouterCost_WithoutOpenRouterMetadata(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	metadata := fantasy.ProviderMetadata{}
	cost := a.openrouterCost(metadata)
	assert.Nil(t, cost)
}

func TestOpenrouterCost_NilMetadata(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	cost := a.openrouterCost(nil)
	assert.Nil(t, cost)
}

// ---------------------------------------------------------------------------
// convertToToolResult
// ---------------------------------------------------------------------------

func TestConvertToToolResult_TextResult(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	input := fantasy.ToolResultContent{
		ToolCallID:     "call-1",
		ToolName:       "view",
		ClientMetadata: `{"key":"val"}`,
		Result:         fantasy.ToolResultOutputContentText{Text: "file contents here"},
	}
	got := a.convertToToolResult(input)

	assert.Equal(t, "call-1", got.ToolCallID)
	assert.Equal(t, "view", got.Name)
	assert.Equal(t, "file contents here", got.Content)
	assert.Equal(t, `{"key":"val"}`, got.Metadata)
	assert.False(t, got.IsError)
}

func TestConvertToToolResult_ErrorResult(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	input := fantasy.ToolResultContent{
		ToolCallID: "call-2",
		ToolName:   "bash",
		Result:     fantasy.ToolResultOutputContentError{Error: errors.New("permission denied")},
	}
	got := a.convertToToolResult(input)

	assert.Equal(t, "call-2", got.ToolCallID)
	assert.Equal(t, "bash", got.Name)
	assert.Equal(t, "permission denied", got.Content)
	assert.True(t, got.IsError)
}

func TestConvertToToolResult_MediaResult(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	input := fantasy.ToolResultContent{
		ToolCallID: "call-3",
		ToolName:   "screenshot",
		Result: fantasy.ToolResultOutputContentMedia{
			Data:      base64.StdEncoding.EncodeToString([]byte("fake-png-data")),
			MediaType: "image/png",
			Text:      "Screenshot of editor",
		},
	}
	got := a.convertToToolResult(input)

	assert.Equal(t, "call-3", got.ToolCallID)
	assert.Equal(t, "screenshot", got.Name)
	assert.Equal(t, "Screenshot of editor", got.Content)
	assert.Equal(t, "image/png", got.MIMEType)
	assert.NotEmpty(t, got.Data)
	assert.False(t, got.IsError)
}

func TestConvertToToolResult_MediaResult_NoText(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	input := fantasy.ToolResultContent{
		ToolCallID: "call-4",
		ToolName:   "screenshot",
		Result: fantasy.ToolResultOutputContentMedia{
			Data:      base64.StdEncoding.EncodeToString([]byte("data")),
			MediaType: "image/jpeg",
			Text:      "", // empty text triggers fallback message
		},
	}
	got := a.convertToToolResult(input)
	assert.Equal(t, "Loaded image/jpeg content", got.Content)
}

// ---------------------------------------------------------------------------
// updateSessionUsage
// ---------------------------------------------------------------------------

func TestUpdateSessionUsage_CatwalkCost(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	model := Model{
		CatwalkCfg: catwalk.Model{
			CostPer1MIn:       3.0,
			CostPer1MOut:      15.0,
			CostPer1MInCached: 0.3,
			CostPer1MOutCached: 1.5,
		},
	}
	sess := &session.Session{ID: "s1", Cost: 0}
	usage := fantasy.Usage{
		InputTokens:         1_000_000,
		OutputTokens:        100_000,
		CacheCreationTokens: 0,
		CacheReadTokens:     0,
	}
	a.updateSessionUsage(model, sess, usage, nil)

	// Expected: 3.0 * 1 (input) + 15.0 * 0.1 (output) = 4.5
	assert.InDelta(t, 4.5, sess.Cost, 1e-6)
	assert.Equal(t, int64(100_000), sess.CompletionTokens)
	assert.Equal(t, int64(1_000_000), sess.PromptTokens)
}

func TestUpdateSessionUsage_OverrideCost(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	model := Model{
		CatwalkCfg: catwalk.Model{
			CostPer1MIn:  3.0,
			CostPer1MOut: 15.0,
		},
	}
	sess := &session.Session{ID: "s2", Cost: 1.0}
	usage := fantasy.Usage{
		InputTokens:  500_000,
		OutputTokens: 50_000,
	}
	override := 0.99
	a.updateSessionUsage(model, sess, usage, &override)

	// When override cost is provided, it should be used instead of computed cost.
	assert.InDelta(t, 1.99, sess.Cost, 1e-9, "should add override cost to existing session cost")
}

// ---------------------------------------------------------------------------
// workaroundProviderMediaLimitations
// ---------------------------------------------------------------------------

func TestWorkaroundProviderMediaLimitations_AnthropicPassthrough(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	model := Model{
		ModelCfg: config.SelectedModel{Provider: "anthropic"},
	}
	mediaData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	msgs := []fantasy.Message{
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "tc1",
					Output: fantasy.ToolResultOutputContentMedia{
						Data:      mediaData,
						MediaType: "image/png",
					},
				},
			},
		},
	}
	result := a.workaroundProviderMediaLimitations(msgs, model)

	// Anthropic supports media in tool results natively, so messages should be unchanged.
	require.Len(t, result, 1)
	assert.Equal(t, fantasy.MessageRoleTool, result[0].Role)
}

func TestWorkaroundProviderMediaLimitations_OpenAIConverts(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	model := Model{
		ModelCfg: config.SelectedModel{Provider: "openai"},
	}
	mediaData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	msgs := []fantasy.Message{
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "tc1",
					Output: fantasy.ToolResultOutputContentMedia{
						Data:      mediaData,
						MediaType: "image/png",
					},
				},
			},
		},
	}
	result := a.workaroundProviderMediaLimitations(msgs, model)

	// For OpenAI: tool result should be replaced with text + extra user message with attachment.
	require.Len(t, result, 2, "should have tool message + injected user message")

	assert.Equal(t, fantasy.MessageRoleTool, result[0].Role)
	// The tool result text should be the placeholder.
	toolPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](result[0].Content[0])
	require.True(t, ok)
	textOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](toolPart.Output)
	require.True(t, ok)
	assert.Contains(t, textOutput.Text, "Image/media content loaded")

	assert.Equal(t, fantasy.MessageRoleUser, result[1].Role)
}

func TestWorkaroundProviderMediaLimitations_NonToolMessagesUntouched(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	model := Model{
		ModelCfg: config.SelectedModel{Provider: "openai"},
	}
	msgs := []fantasy.Message{
		fantasy.NewUserMessage("hello"),
		fantasy.NewSystemMessage("you are helpful"),
	}
	result := a.workaroundProviderMediaLimitations(msgs, model)
	require.Len(t, result, 2)
	assert.Equal(t, fantasy.MessageRoleUser, result[0].Role)
	assert.Equal(t, fantasy.MessageRoleSystem, result[1].Role)
}

func TestWorkaroundProviderMediaLimitations_TextToolResultUnchanged(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	model := Model{
		ModelCfg: config.SelectedModel{Provider: "openai"},
	}
	msgs := []fantasy.Message{
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "tc1",
					Output:     fantasy.ToolResultOutputContentText{Text: "file content"},
				},
			},
		},
	}
	result := a.workaroundProviderMediaLimitations(msgs, model)

	// Text tool results should not be split even for non-Anthropic providers.
	require.Len(t, result, 1)
	assert.Equal(t, fantasy.MessageRoleTool, result[0].Role)
}

// ---------------------------------------------------------------------------
// preparePrompt
// ---------------------------------------------------------------------------

func TestPreparePrompt_EmptyMessages(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()

	history, files := a.preparePrompt(nil)

	// Non-sub-agent adds a system reminder as the first user message.
	require.Len(t, history, 1)
	assert.Equal(t, fantasy.MessageRoleUser, history[0].Role)
	assert.Empty(t, files)
}

func TestPreparePrompt_SubAgent_EmptyMessages(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	a.isSubAgent = true

	history, files := a.preparePrompt(nil)

	// Sub-agent should not inject the system reminder.
	assert.Empty(t, history)
	assert.Empty(t, files)
}

func TestPreparePrompt_SkipsEmptyAssistantMessages(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	a.isSubAgent = true // skip reminder injection for clarity

	msgs := []message.Message{
		{
			ID:   "m1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Hello"},
			},
		},
		{
			ID:    "m2",
			Role:  message.Assistant,
			Parts: []message.ContentPart{}, // empty assistant message (cancelled)
		},
		{
			ID:   "m3",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "try again"},
			},
		},
	}

	history, _ := a.preparePrompt(msgs)

	// The empty assistant message should be skipped, so we should have 2 messages.
	assert.Len(t, history, 2)
}

func TestPreparePrompt_BinaryAttachmentsReturnedAsFiles(t *testing.T) {
	t.Parallel()
	a := newMinimalAgent()
	a.isSubAgent = true

	attachments := []message.Attachment{
		{FileName: "photo.png", MimeType: "image/png", Content: []byte("fake-png")},
		{FileName: "readme.txt", MimeType: "text/plain", Content: []byte("hello")},
	}

	_, files := a.preparePrompt(nil, attachments...)

	// Only non-text attachments should be returned as files.
	require.Len(t, files, 1)
	assert.Equal(t, "photo.png", files[0].Filename)
	assert.Equal(t, "image/png", files[0].MediaType)
}

// ---------------------------------------------------------------------------
// thinkTagRegex
// ---------------------------------------------------------------------------

func TestThinkTagRegex(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{"<think>inner thoughts</think>Clean Title", "Clean Title"},
		{"No tags here", "No tags here"},
		{"<think>a</think>Middle<think>b</think>End", "MiddleEnd"},
		{"<think></think>", ""},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got := thinkTagRegex.ReplaceAllString(tc.input, "")
			assert.Equal(t, tc.expected, got)
		})
	}
}
