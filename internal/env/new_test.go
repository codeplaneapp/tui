package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ReturnsOsEnv(t *testing.T) {
	e := New()
	require.NotNil(t, e)
	assert.IsType(t, &osEnv{}, e)
}

func TestNew_GetDelegatesToOS(t *testing.T) {
	t.Setenv("ENV_PKG_TEST_VAR", "hello_env")
	e := New()
	assert.Equal(t, "hello_env", e.Get("ENV_PKG_TEST_VAR"))
}

func TestNewFromMap_EmptyStringValue(t *testing.T) {
	// Verify that an empty-string value is distinguishable from a missing key
	// only by the fact that both return "". This exercises the map lookup path.
	e := NewFromMap(map[string]string{"PRESENT": ""})
	assert.Equal(t, "", e.Get("PRESENT"))
	assert.Equal(t, "", e.Get("ABSENT"))
}
