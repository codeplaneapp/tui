package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashID_Deterministic(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"

	hash1 := HashID(id)
	hash2 := HashID(id)
	assert.Equal(t, hash1, hash2, "same input must produce same hash")

	other := "661f9511-f3ac-52e5-b827-557766551111"
	hashOther := HashID(other)
	assert.NotEqual(t, hash1, hashOther, "different inputs must produce different hashes")

	// Hash should be a non-empty hex string.
	assert.NotEmpty(t, hash1)
}

func TestHasIncompleteTodos_AllCompleted(t *testing.T) {
	todos := []Todo{
		{Content: "first", Status: TodoStatusCompleted},
		{Content: "second", Status: TodoStatusCompleted},
		{Content: "third", Status: TodoStatusCompleted},
	}
	assert.False(t, HasIncompleteTodos(todos))
}

func TestHasIncompleteTodos_SomePending(t *testing.T) {
	todos := []Todo{
		{Content: "done", Status: TodoStatusCompleted},
		{Content: "still pending", Status: TodoStatusPending},
		{Content: "in progress", Status: TodoStatusInProgress},
	}
	assert.True(t, HasIncompleteTodos(todos))
}

func TestHasIncompleteTodos_EmptyList(t *testing.T) {
	assert.False(t, HasIncompleteTodos([]Todo{}))
}

func TestMarshalUnmarshalTodos_RoundTrip(t *testing.T) {
	original := []Todo{
		{Content: "write tests", Status: TodoStatusPending, ActiveForm: "testing"},
		{Content: "deploy", Status: TodoStatusInProgress, ActiveForm: "ops"},
		{Content: "celebrate", Status: TodoStatusCompleted, ActiveForm: ""},
	}

	data, err := marshalTodos(original)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	restored, err := unmarshalTodos(data)
	require.NoError(t, err)
	assert.Equal(t, original, restored)
}

func TestUnmarshalTodos_EmptyString(t *testing.T) {
	todos, err := unmarshalTodos("")
	require.NoError(t, err)
	assert.Empty(t, todos)
	// Ensure it returns an initialized (non-nil) empty slice.
	assert.NotNil(t, todos)
}

func TestUnmarshalTodos_InvalidJSON(t *testing.T) {
	_, err := unmarshalTodos("{not valid json!!")
	assert.Error(t, err)
}

// newTestService returns a minimal service that doesn't need a database.
func newTestService() Service {
	return NewService(nil, nil)
}

func TestCreateAgentToolSessionID(t *testing.T) {
	svc := newTestService()

	id := svc.CreateAgentToolSessionID("msg-123", "tc-456")
	assert.Equal(t, "msg-123$$tc-456", id)
}

func TestParseAgentToolSessionID_Valid(t *testing.T) {
	svc := newTestService()

	msgID, toolID, ok := svc.ParseAgentToolSessionID("msg-abc$$tc-xyz")
	require.True(t, ok)
	assert.Equal(t, "msg-abc", msgID)
	assert.Equal(t, "tc-xyz", toolID)
}

func TestParseAgentToolSessionID_Invalid(t *testing.T) {
	svc := newTestService()

	_, _, ok := svc.ParseAgentToolSessionID("no-separator-here")
	assert.False(t, ok)

	// Also invalid: more than one separator produces 3 parts.
	_, _, ok = svc.ParseAgentToolSessionID("a$$b$$c")
	assert.False(t, ok)
}

func TestIsAgentToolSession(t *testing.T) {
	svc := newTestService()

	assert.True(t, svc.IsAgentToolSession("msg-1$$tc-2"))
	assert.False(t, svc.IsAgentToolSession("550e8400-e29b-41d4-a716-446655440000"))
	assert.False(t, svc.IsAgentToolSession(""))
}
