package cmd

import (
	"database/sql"
	"errors"
	"syscall"
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToInt64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"int64 value", int64(42), 42},
		{"float64 value", float64(3.7), 3},
		{"int value", int(100), 100},
		{"string returns zero", "hello", 0},
		{"nil returns zero", nil, 0},
		{"negative int64", int64(-5), -5},
		{"zero float64", float64(0), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toInt64(tt.in))
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{"float64 value", float64(3.14), 3.14},
		{"int64 value", int64(42), 42.0},
		{"int value", int(7), 7.0},
		{"string returns zero", "nope", 0},
		{"nil returns zero", nil, 0},
		{"negative float64", float64(-2.5), -2.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, toFloat64(tt.in), 0.001)
		})
	}
}

func TestNullFloat64ToInt64(t *testing.T) {
	tests := []struct {
		name string
		in   sql.NullFloat64
		want int64
	}{
		{"valid positive", sql.NullFloat64{Float64: 99.9, Valid: true}, 99},
		{"valid zero", sql.NullFloat64{Float64: 0, Valid: true}, 0},
		{"valid negative", sql.NullFloat64{Float64: -10.5, Valid: true}, -10},
		{"null returns zero", sql.NullFloat64{Float64: 42, Valid: false}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nullFloat64ToInt64(tt.in))
		})
	}
}

func TestIsBrokenPipe(t *testing.T) {
	assert.False(t, isBrokenPipe(nil))
	assert.True(t, isBrokenPipe(syscall.EPIPE))
	assert.True(t, isBrokenPipe(errors.New("write: broken pipe")))
	assert.False(t, isBrokenPipe(errors.New("connection refused")))
}

func TestParseModelString(t *testing.T) {
	tests := []struct {
		input        string
		wantProvider string
		wantModel    string
	}{
		{"", "", ""},
		{"gpt-4", "", "gpt-4"},
		{"openai/gpt-4", "openai", "gpt-4"},
		{"provider/model/extra", "provider", "model/extra"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			provider, model := parseModelString(tt.input)
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantModel, model)
		})
	}
}

func TestMatchesModel(t *testing.T) {
	// Empty wantID never matches.
	assert.False(t, matchesModel("", "", "gpt-4", "openai"))

	// Model ID match, no provider filter.
	assert.True(t, matchesModel("gpt-4", "", "gpt-4", "openai"))

	// Case insensitive.
	assert.True(t, matchesModel("GPT-4", "", "gpt-4", "openai"))

	// Provider mismatch.
	assert.False(t, matchesModel("gpt-4", "anthropic", "gpt-4", "openai"))

	// Provider match.
	assert.True(t, matchesModel("gpt-4", "openai", "gpt-4", "openai"))
}

func TestValidateModelMatches(t *testing.T) {
	// No matches.
	_, err := validateModelMatches(nil, "gpt-4", "large")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Exactly one match.
	m, err := validateModelMatches([]modelMatch{{provider: "openai", modelID: "gpt-4"}}, "gpt-4", "large")
	require.NoError(t, err)
	assert.Equal(t, "openai", m.provider)

	// Multiple matches.
	_, err = validateModelMatches([]modelMatch{
		{provider: "openai", modelID: "gpt-4"},
		{provider: "azure", modelID: "gpt-4"},
	}, "gpt-4", "large")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple providers")
}

func TestMessagePtrs(t *testing.T) {
	// nil input produces a zero-length slice.
	require.Empty(t, messagePtrs(nil))

	// Each pointer should reference the original slice element.
	msgs := make([]message.Message, 3)
	msgs[1].ID = "second"
	ptrs := messagePtrs(msgs)
	require.Len(t, ptrs, 3)
	for _, p := range ptrs {
		require.NotNil(t, p)
	}
	assert.Equal(t, "second", ptrs[1].ID)
}
