package diffview

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPad(t *testing.T) {
	tests := []struct {
		input    any
		width    int
		expected string
	}{
		{7, 2, " 7"},
		{7, 3, "  7"},
		{"a", 2, " a"},
		{"a", 3, "  a"},
		{"…", 2, " …"},
		{"…", 3, "  …"},
	}

	for _, tt := range tests {
		result := pad(tt.input, tt.width)
		if result != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, result)
		}
	}
}

func TestPad_ExactWidth(t *testing.T) {
	// When the value already matches or exceeds the width, no padding is added.
	assert.Equal(t, "abc", pad("abc", 3), "exact width should produce no padding")
	assert.Equal(t, "abcd", pad("abcd", 3), "wider than width should return as-is")
}

func TestIsEven(t *testing.T) {
	assert.True(t, isEven(0))
	assert.True(t, isEven(2))
	assert.True(t, isEven(-4))
	assert.False(t, isEven(1))
	assert.False(t, isEven(3))
}

func TestIsOdd(t *testing.T) {
	assert.True(t, isOdd(1))
	assert.True(t, isOdd(3))
	assert.True(t, isOdd(-1))
	assert.False(t, isOdd(0))
	assert.False(t, isOdd(2))
}

func TestBtoi(t *testing.T) {
	assert.Equal(t, 1, btoi(true))
	assert.Equal(t, 0, btoi(false))
}

func TestTernary(t *testing.T) {
	assert.Equal(t, "yes", ternary(true, "yes", "no"))
	assert.Equal(t, "no", ternary(false, "yes", "no"))
	assert.Equal(t, 42, ternary(true, 42, 99))
	assert.Equal(t, 99, ternary(false, 42, 99))
}
