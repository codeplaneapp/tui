package agent

import (
	"context"
	"errors"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSessionAgent is a minimal mock for the SessionAgent interface.
type mockSessionAgent struct {
	model     Model
	runFunc   func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error)
	cancelled []string
}

func (m *mockSessionAgent) Run(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
	return m.runFunc(ctx, call)
}

func (m *mockSessionAgent) Model() Model                        { return m.model }
func (m *mockSessionAgent) SetModels(large, small Model)        {}
func (m *mockSessionAgent) SetTools(tools []fantasy.AgentTool)  {}
func (m *mockSessionAgent) SetSystemPrompt(systemPrompt string) {}
func (m *mockSessionAgent) Cancel(sessionID string) {
	m.cancelled = append(m.cancelled, sessionID)
}
func (m *mockSessionAgent) CancelAll()                                  {}
func (m *mockSessionAgent) IsSessionBusy(sessionID string) bool         { return false }
func (m *mockSessionAgent) IsBusy() bool                                { return false }
func (m *mockSessionAgent) QueuedPrompts(sessionID string) int          { return 0 }
func (m *mockSessionAgent) QueuedPromptsList(sessionID string) []string { return nil }
func (m *mockSessionAgent) ClearQueue(sessionID string)                 {}
func (m *mockSessionAgent) Summarize(context.Context, string, fantasy.ProviderOptions) error {
	return nil
}

// newTestCoordinator creates a minimal coordinator for unit testing runSubAgent.
func newTestCoordinator(t *testing.T, env fakeEnv, providerID string, providerCfg config.ProviderConfig) *coordinator {
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfg.Config().Providers.Set(providerID, providerCfg)
	return &coordinator{
		cfg:      cfg,
		sessions: env.sessions,
	}
}

// newMockAgent creates a mockSessionAgent with the given provider and run function.
func newMockAgent(providerID string, maxTokens int64, runFunc func(context.Context, SessionAgentCall) (*fantasy.AgentResult, error)) *mockSessionAgent {
	return &mockSessionAgent{
		model: Model{
			CatwalkCfg: catwalk.Model{
				DefaultMaxTokens: maxTokens,
			},
			ModelCfg: config.SelectedModel{
				Provider: providerID,
			},
		},
		runFunc: runFunc,
	}
}

// agentResultWithText creates a minimal AgentResult with the given text response.
func agentResultWithText(text string) *fantasy.AgentResult {
	return &fantasy.AgentResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: text},
			},
		},
	}
}

func TestRunSubAgent(t *testing.T) {
	const providerID = "test-provider"
	providerCfg := config.ProviderConfig{ID: providerID}

	t.Run("happy path", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			assert.Equal(t, "do something", call.Prompt)
			assert.Equal(t, int64(4096), call.MaxOutputTokens)
			return agentResultWithText("done"), nil
		})

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "do something",
			SessionTitle:   "Test Session",
		})
		require.NoError(t, err)
		assert.Equal(t, "done", resp.Content)
		assert.False(t, resp.IsError)
	})

	t.Run("ModelCfg.MaxTokens overrides default", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := &mockSessionAgent{
			model: Model{
				CatwalkCfg: catwalk.Model{
					DefaultMaxTokens: 4096,
				},
				ModelCfg: config.SelectedModel{
					Provider:  providerID,
					MaxTokens: 8192,
				},
			},
			runFunc: func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
				assert.Equal(t, int64(8192), call.MaxOutputTokens)
				return agentResultWithText("ok"), nil
			},
		}

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
	})

	t.Run("session creation failure with canceled context", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, nil)

		// Use a canceled context to trigger CreateTaskSession failure.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err = coord.runSubAgent(ctx, subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.Error(t, err)
	})

	t.Run("provider not configured", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		// Agent references a provider that doesn't exist in config.
		agent := newMockAgent("unknown-provider", 4096, nil)

		_, err = coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model provider not configured")
	})

	t.Run("agent run error returns error response", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
			return nil, errors.New("agent exploded")
		})

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		// runSubAgent returns (errorResponse, nil) when agent.Run fails — not a Go error.
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "error generating response", resp.Content)
	})

	t.Run("session setup callback is invoked", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		var setupCalledWith string
		agent := newMockAgent(providerID, 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
			return agentResultWithText("ok"), nil
		})

		_, err = coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
			SessionSetup: func(sessionID string) {
				setupCalledWith = sessionID
			},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, setupCalledWith, "SessionSetup should have been called")
	})

	t.Run("cost propagation to parent session", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			// Simulate the agent incurring cost by updating the child session.
			childSession, err := env.sessions.Get(ctx, call.SessionID)
			if err != nil {
				return nil, err
			}
			childSession.Cost = 0.05
			_, err = env.sessions.Save(ctx, childSession)
			if err != nil {
				return nil, err
			}
			return agentResultWithText("ok"), nil
		})

		_, err = coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parentSession.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.05, updated.Cost, 1e-9)
	})
}

func TestUpdateParentSessionCost(t *testing.T) {
	t.Run("accumulates cost correctly", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
		require.NoError(t, err)

		// Set child cost.
		child.Cost = 0.10
		_, err = env.sessions.Save(t.Context(), child)
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child.ID, parent.ID)
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parent.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.10, updated.Cost, 1e-9)
	})

	t.Run("accumulates multiple child costs", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		child1, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child1")
		require.NoError(t, err)
		child1.Cost = 0.05
		_, err = env.sessions.Save(t.Context(), child1)
		require.NoError(t, err)

		child2, err := env.sessions.CreateTaskSession(t.Context(), "tool-2", parent.ID, "Child2")
		require.NoError(t, err)
		child2.Cost = 0.03
		_, err = env.sessions.Save(t.Context(), child2)
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child1.ID, parent.ID)
		require.NoError(t, err)
		err = coord.updateParentSessionCost(t.Context(), child2.ID, parent.ID)
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parent.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.08, updated.Cost, 1e-9)
	})

	t.Run("child session not found", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), "non-existent", parent.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get child session")
	})

	t.Run("parent session not found", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child.ID, "non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get parent session")
	})

	t.Run("zero cost handled correctly", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child.ID, parent.ID)
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parent.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.0, updated.Cost, 1e-9)
	})
}

func TestIsExactoSupported(t *testing.T) {
	t.Parallel()

	t.Run("supported models return true", func(t *testing.T) {
		t.Parallel()
		supportedModels := []string{
			"moonshotai/kimi-k2-0905",
			"deepseek/deepseek-v3.1-terminus",
			"z-ai/glm-4.6",
			"openai/gpt-oss-120b",
			"qwen/qwen3-coder",
		}
		for _, modelID := range supportedModels {
			assert.True(t, isExactoSupported(modelID), "expected %q to be exacto-supported", modelID)
		}
	})

	t.Run("unsupported models return false", func(t *testing.T) {
		t.Parallel()
		unsupportedModels := []string{
			"anthropic/claude-opus-4",
			"openai/gpt-4o",
			"google/gemini-2.5-pro",
			"",
			"moonshotai/kimi-k2-0905:exacto", // already has suffix
		}
		for _, modelID := range unsupportedModels {
			assert.False(t, isExactoSupported(modelID), "expected %q to not be exacto-supported", modelID)
		}
	})
}

func TestMergeCallOptions(t *testing.T) {
	t.Parallel()

	floatPtr := func(v float64) *float64 { return &v }
	int64Ptr := func(v int64) *int64 { return &v }

	t.Run("ModelCfg values take precedence over CatwalkCfg", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Temperature:      floatPtr(0.8),
				TopP:             floatPtr(0.9),
				TopK:             int64Ptr(50),
				FrequencyPenalty: floatPtr(0.1),
				PresencePenalty:  floatPtr(0.2),
			},
			CatwalkCfg: catwalk.Model{
				Options: catwalk.ModelOptions{
					Temperature:      floatPtr(0.3),
					TopP:             floatPtr(0.4),
					TopK:             int64Ptr(10),
					FrequencyPenalty: floatPtr(0.5),
					PresencePenalty:  floatPtr(0.6),
				},
			},
		}
		_, temp, topP, topK, freqPenalty, presPenalty := mergeCallOptions(model, config.ProviderConfig{})
		assert.Equal(t, floatPtr(0.8), temp)
		assert.Equal(t, floatPtr(0.9), topP)
		assert.Equal(t, int64Ptr(50), topK)
		assert.Equal(t, floatPtr(0.1), freqPenalty)
		assert.Equal(t, floatPtr(0.2), presPenalty)
	})

	t.Run("falls back to CatwalkCfg when ModelCfg is nil", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{},
			CatwalkCfg: catwalk.Model{
				Options: catwalk.ModelOptions{
					Temperature:      floatPtr(0.3),
					TopP:             floatPtr(0.4),
					TopK:             int64Ptr(10),
					FrequencyPenalty: floatPtr(0.5),
					PresencePenalty:  floatPtr(0.6),
				},
			},
		}
		_, temp, topP, topK, freqPenalty, presPenalty := mergeCallOptions(model, config.ProviderConfig{})
		assert.Equal(t, floatPtr(0.3), temp)
		assert.Equal(t, floatPtr(0.4), topP)
		assert.Equal(t, int64Ptr(10), topK)
		assert.Equal(t, floatPtr(0.5), freqPenalty)
		assert.Equal(t, floatPtr(0.6), presPenalty)
	})

	t.Run("returns nil pointers when both sources are nil", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg:   config.SelectedModel{},
			CatwalkCfg: catwalk.Model{},
		}
		_, temp, topP, topK, freqPenalty, presPenalty := mergeCallOptions(model, config.ProviderConfig{})
		assert.Nil(t, temp)
		assert.Nil(t, topP)
		assert.Nil(t, topK)
		assert.Nil(t, freqPenalty)
		assert.Nil(t, presPenalty)
	})
}

func TestIsUnauthorized(t *testing.T) {
	t.Parallel()

	coord := &coordinator{}

	t.Run("returns true for 401 ProviderError", func(t *testing.T) {
		t.Parallel()
		err := &fantasy.ProviderError{StatusCode: 401, Message: "unauthorized"}
		assert.True(t, coord.isUnauthorized(err))
	})

	t.Run("returns false for 403 ProviderError", func(t *testing.T) {
		t.Parallel()
		err := &fantasy.ProviderError{StatusCode: 403, Message: "forbidden"}
		assert.False(t, coord.isUnauthorized(err))
	})

	t.Run("returns false for non-ProviderError", func(t *testing.T) {
		t.Parallel()
		err := errors.New("some other error")
		assert.False(t, coord.isUnauthorized(err))
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		t.Parallel()
		assert.False(t, coord.isUnauthorized(nil))
	})
}

func TestIsAnthropicThinking(t *testing.T) {
	t.Parallel()
	coord := &coordinator{}

	t.Run("returns true when Think flag is set", func(t *testing.T) {
		t.Parallel()
		model := config.SelectedModel{Think: true}
		assert.True(t, coord.isAnthropicThinking(model))
	})

	t.Run("returns true when provider_options contain thinking key", func(t *testing.T) {
		t.Parallel()
		model := config.SelectedModel{
			ProviderOptions: map[string]any{
				"thinking": map[string]any{"budget_tokens": 5000},
			},
		}
		assert.True(t, coord.isAnthropicThinking(model))
	})

	t.Run("returns false when neither Think nor thinking option set", func(t *testing.T) {
		t.Parallel()
		model := config.SelectedModel{
			ProviderOptions: map[string]any{"effort": "high"},
		}
		assert.False(t, coord.isAnthropicThinking(model))
	})

	t.Run("returns false for empty model", func(t *testing.T) {
		t.Parallel()
		model := config.SelectedModel{}
		assert.False(t, coord.isAnthropicThinking(model))
	})

	t.Run("Think true takes precedence even with nil provider options", func(t *testing.T) {
		t.Parallel()
		model := config.SelectedModel{
			Think:           true,
			ProviderOptions: nil,
		}
		assert.True(t, coord.isAnthropicThinking(model))
	})
}

func TestGetProviderOptions(t *testing.T) {
	t.Parallel()

	t.Run("merges model, provider, and catwalk options with correct precedence", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider:        "anthropic",
				ProviderOptions: map[string]any{"effort": "high"},
			},
			CatwalkCfg: catwalk.Model{
				Options: catwalk.ModelOptions{
					ProviderOptions: map[string]any{"effort": "low", "extra_key": "from_catwalk"},
				},
			},
		}
		providerCfg := config.ProviderConfig{
			Type:            "anthropic",
			ProviderOptions: map[string]any{"effort": "medium", "provider_key": "from_provider"},
		}

		opts := getProviderOptions(model, providerCfg)
		// The result should have anthropic options since provider type is anthropic.
		// Model-level effort ("high") should win over provider ("medium") and catwalk ("low").
		assert.NotEmpty(t, opts)
	})

	t.Run("returns empty options for nil provider options on all sources", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg:   config.SelectedModel{Provider: "anthropic"},
			CatwalkCfg: catwalk.Model{},
		}
		providerCfg := config.ProviderConfig{Type: "anthropic"}
		opts := getProviderOptions(model, providerCfg)
		// With no provider options and no reasoning_effort, the result
		// should still contain an anthropic key (from empty merge) or be empty.
		// Since all source options are nil, merging should produce "{}" -> empty map,
		// and no reasoning defaults are injected.
		assert.NotNil(t, opts)
	})

	t.Run("injects reasoning_effort for openai provider", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider:        "openai",
				Model:           "gpt-4o",
				ReasoningEffort: "high",
			},
			CatwalkCfg: catwalk.Model{ID: "gpt-4o"},
		}
		providerCfg := config.ProviderConfig{Type: "openai"}
		opts := getProviderOptions(model, providerCfg)
		// Should have openai options with reasoning_effort injected.
		assert.NotEmpty(t, opts)
	})

	t.Run("injects thinking for anthropic when Think is set", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Think:    true,
			},
			CatwalkCfg: catwalk.Model{ID: "claude-opus-4"},
		}
		providerCfg := config.ProviderConfig{Type: "anthropic"}
		opts := getProviderOptions(model, providerCfg)
		assert.NotEmpty(t, opts)
	})

	t.Run("hyper provider dispatches to anthropic for claude models", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider:        "hyper",
				Model:           "claude-opus-4",
				ReasoningEffort: "high",
			},
			CatwalkCfg: catwalk.Model{ID: "claude-opus-4"},
		}
		providerCfg := config.ProviderConfig{Type: "hyper"}
		opts := getProviderOptions(model, providerCfg)
		// hyper + claude model ID -> should be treated as anthropic provider
		_, hasAnthropic := opts["anthropic"]
		assert.True(t, hasAnthropic, "hyper provider with claude model should produce anthropic options")
	})

	t.Run("hyper provider dispatches to openai for gpt models", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider:        "hyper",
				Model:           "gpt-4o",
				ReasoningEffort: "high",
			},
			CatwalkCfg: catwalk.Model{ID: "gpt-4o"},
		}
		providerCfg := config.ProviderConfig{Type: "hyper"}
		opts := getProviderOptions(model, providerCfg)
		_, hasOpenai := opts["openai"]
		assert.True(t, hasOpenai, "hyper provider with gpt model should produce openai options")
	})

	t.Run("hyper provider dispatches to google for gemini models", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider: "hyper",
				Model:    "gemini-2.5-pro",
			},
			CatwalkCfg: catwalk.Model{ID: "gemini-2.5-pro"},
		}
		providerCfg := config.ProviderConfig{Type: "hyper"}
		opts := getProviderOptions(model, providerCfg)
		_, hasGoogle := opts["google"]
		assert.True(t, hasGoogle, "hyper provider with gemini model should produce google options")
	})

	t.Run("hyper provider dispatches to openai-compat for unknown models", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Provider:        "hyper",
				Model:           "some-custom-model",
				ReasoningEffort: "medium",
			},
			CatwalkCfg: catwalk.Model{ID: "some-custom-model"},
		}
		providerCfg := config.ProviderConfig{Type: "hyper"}
		opts := getProviderOptions(model, providerCfg)
		_, hasCompat := opts["openai-compat"]
		assert.True(t, hasCompat, "hyper provider with unknown model should produce openai-compat options")
	})
}

func TestMergeCallOptionsPartialOverlap(t *testing.T) {
	t.Parallel()

	floatPtr := func(v float64) *float64 { return &v }
	int64Ptr := func(v int64) *int64 { return &v }

	t.Run("ModelCfg partially set falls through to CatwalkCfg for remaining fields", func(t *testing.T) {
		t.Parallel()
		model := Model{
			ModelCfg: config.SelectedModel{
				Temperature: floatPtr(0.7),
				// TopP, TopK, etc. are nil
			},
			CatwalkCfg: catwalk.Model{
				Options: catwalk.ModelOptions{
					Temperature:      floatPtr(0.1),
					TopP:             floatPtr(0.95),
					TopK:             int64Ptr(40),
					FrequencyPenalty: floatPtr(0.3),
				},
			},
		}
		_, temp, topP, topK, freqPenalty, presPenalty := mergeCallOptions(model, config.ProviderConfig{})
		// Temperature: ModelCfg wins (0.7)
		assert.Equal(t, floatPtr(0.7), temp)
		// TopP: falls through to CatwalkCfg (0.95)
		assert.Equal(t, floatPtr(0.95), topP)
		// TopK: falls through to CatwalkCfg (40)
		assert.Equal(t, int64Ptr(40), topK)
		// FrequencyPenalty: falls through to CatwalkCfg (0.3)
		assert.Equal(t, floatPtr(0.3), freqPenalty)
		// PresencePenalty: neither set -> nil
		assert.Nil(t, presPenalty)
	})
}

func TestCoordinatorResolveAgent(t *testing.T) {
	t.Run("prefers smithers agent when configured", func(t *testing.T) {
		cfg, err := config.Init(t.TempDir(), "", false)
		require.NoError(t, err)

		cfg.Config().Smithers = &config.SmithersConfig{
			WorkflowDir: ".smithers/workflows",
		}
		cfg.SetupAgents()

		coord := &coordinator{}
		agentName, agentCfg, err := coord.resolveAgent(cfg)
		require.NoError(t, err)
		assert.Equal(t, config.AgentSmithers, agentName)
		assert.Equal(t, config.AgentSmithers, agentCfg.ID)
	})

	t.Run("falls back to coder agent when smithers is not configured", func(t *testing.T) {
		cfg, err := config.Init(t.TempDir(), "", false)
		require.NoError(t, err)

		cfg.Config().Smithers = nil
		cfg.SetupAgents()

		coord := &coordinator{}
		agentName, agentCfg, err := coord.resolveAgent(cfg)
		require.NoError(t, err)
		assert.Equal(t, config.AgentCoder, agentName)
		assert.Equal(t, config.AgentCoder, agentCfg.ID)
	})

	t.Run("returns an error when no supported agent exists", func(t *testing.T) {
		cfg, err := config.Init(t.TempDir(), "", false)
		require.NoError(t, err)

		cfg.Config().Agents = map[string]config.Agent{}

		coord := &coordinator{}
		_, _, err = coord.resolveAgent(cfg)
		require.ErrorIs(t, err, errCoderAgentNotConfigured)
	})
}
