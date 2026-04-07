package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEven(t *testing.T) {
	assert.True(t, isEven(0))
	assert.True(t, isEven(2))
	assert.True(t, isEven(100))
	assert.False(t, isEven(1))
	assert.False(t, isEven(3))
	assert.False(t, isEven(-1))
}

func TestHashString_Deterministic(t *testing.T) {
	a := hashString("test-input")
	b := hashString("test-input")
	assert.Equal(t, a, b, "hashString should be deterministic")
	assert.Len(t, a, 64, "HMAC-SHA256 hex output should be 64 characters")
}

func TestHashString_DifferentInputs(t *testing.T) {
	a := hashString("input-a")
	b := hashString("input-b")
	assert.NotEqual(t, a, b, "different inputs should produce different hashes")
}
