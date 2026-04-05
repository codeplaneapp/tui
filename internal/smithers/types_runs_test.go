package smithers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- SummariseRuns ---

func TestSummariseRuns_Mixed(t *testing.T) {
	runs := []RunSummary{
		{RunID: "a", Status: RunStatusRunning},
		{RunID: "b", Status: RunStatusWaitingApproval},
		{RunID: "c", Status: RunStatusWaitingEvent},
		{RunID: "d", Status: RunStatusFinished},
		{RunID: "e", Status: RunStatusFailed},
	}
	s := SummariseRuns(runs)
	assert.Equal(t, 3, s.ActiveRuns)
	assert.Equal(t, 1, s.PendingApprovals)
}

func TestSummariseRuns_Empty(t *testing.T) {
	s := SummariseRuns(nil)
	assert.Equal(t, 0, s.ActiveRuns)
	assert.Equal(t, 0, s.PendingApprovals)
}

func TestSummariseRuns_AllTerminal(t *testing.T) {
	runs := []RunSummary{
		{Status: RunStatusFinished},
		{Status: RunStatusCancelled},
		{Status: RunStatusFailed},
	}
	s := SummariseRuns(runs)
	assert.Equal(t, 0, s.ActiveRuns)
	assert.Equal(t, 0, s.PendingApprovals)
}

func TestSummariseRuns_MultipleApprovals(t *testing.T) {
	runs := []RunSummary{
		{Status: RunStatusWaitingApproval},
		{Status: RunStatusWaitingApproval},
		{Status: RunStatusRunning},
	}
	s := SummariseRuns(runs)
	assert.Equal(t, 3, s.ActiveRuns)
	assert.Equal(t, 2, s.PendingApprovals)
}

func TestSummariseRuns_OnlyWaitingEvent(t *testing.T) {
	runs := []RunSummary{
		{Status: RunStatusWaitingEvent},
		{Status: RunStatusWaitingEvent},
	}
	s := SummariseRuns(runs)
	assert.Equal(t, 2, s.ActiveRuns)
	assert.Equal(t, 0, s.PendingApprovals)
}

// --- ErrorReason ---

func TestRunSummaryErrorReason(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name      string
		errorJSON *string
		want      string
	}{
		{
			name:      "nil ErrorJSON returns empty",
			errorJSON: nil,
			want:      "",
		},
		{
			name:      "raw string returned trimmed",
			errorJSON: strPtr("  something went wrong  "),
			want:      "something went wrong",
		},
		{
			name:      "JSON with message key extracted",
			errorJSON: strPtr(`{"message":"disk quota exceeded"}`),
			want:      "disk quota exceeded",
		},
		{
			name:      "JSON without message key falls back to raw string",
			errorJSON: strPtr(`{"code":"ERR_TIMEOUT"}`),
			want:      `{"code":"ERR_TIMEOUT"}`,
		},
		{
			name:      "raw string longer than 80 chars is trimmed to 80",
			errorJSON: strPtr(strings.Repeat("x", 100)),
			want:      strings.Repeat("x", 80),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := RunSummary{ErrorJSON: tc.errorJSON}
			assert.Equal(t, tc.want, run.ErrorReason())
		})
	}
}

func TestSummariseRuns_SingleRunning(t *testing.T) {
	runs := []RunSummary{
		{RunID: "r1", WorkflowName: "code-review", Status: RunStatusRunning},
	}
	s := SummariseRuns(runs)
	assert.Equal(t, 1, s.ActiveRuns)
	assert.Equal(t, 0, s.PendingApprovals)
}

// =============================================================================
// feat-hijack-multi-engine-support: HijackSession.ResumeArgs() engine routing
// =============================================================================

func TestHijackSession_ResumeArgs_ClaudeCode(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "claude-code",
		ResumeToken:    "sess-123",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--resume", "sess-123"}, args,
		"claude-code should use --resume flag")
}

func TestHijackSession_ResumeArgs_Claude(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "claude",
		ResumeToken:    "tok-abc",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--resume", "tok-abc"}, args,
		"claude engine should use --resume flag")
}

func TestHijackSession_ResumeArgs_Amp(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "amp",
		ResumeToken:    "amp-session",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--resume", "amp-session"}, args,
		"amp should use --resume flag")
}

func TestHijackSession_ResumeArgs_Codex(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "codex",
		ResumeToken:    "codex-456",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--session-id", "codex-456"}, args,
		"codex should use --session-id flag")
}

func TestHijackSession_ResumeArgs_Gemini(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "gemini",
		ResumeToken:    "gemini-789",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--session", "gemini-789"}, args,
		"gemini should use --session flag")
}

func TestHijackSession_ResumeArgs_UnknownEngine_FallsBackToResume(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "some-future-agent",
		ResumeToken:    "tok-xyz",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--resume", "tok-xyz"}, args,
		"unknown engine should fall back to --resume flag")
}

func TestHijackSession_ResumeArgs_NoToken_ReturnsNil(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "claude-code",
		ResumeToken:    "",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Nil(t, args, "empty token should return nil args")
}

func TestHijackSession_ResumeArgs_SupportsResumeFalse_ReturnsNil(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "claude-code",
		ResumeToken:    "tok-abc",
		SupportsResume: false,
	}
	args := s.ResumeArgs()
	assert.Nil(t, args, "SupportsResume=false should return nil regardless of token")
}

func TestHijackSession_ResumeArgs_Codex_NoToken_ReturnsNil(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "codex",
		ResumeToken:    "",
		SupportsResume: true,
	}
	args := s.ResumeArgs()
	assert.Nil(t, args, "codex with empty token should return nil")
}

func TestHijackSession_ResumeArgs_Gemini_SupportsResumeFalse_ReturnsNil(t *testing.T) {
	s := &HijackSession{
		AgentEngine:    "gemini",
		ResumeToken:    "tok",
		SupportsResume: false,
	}
	args := s.ResumeArgs()
	assert.Nil(t, args, "gemini with SupportsResume=false should return nil")
}
