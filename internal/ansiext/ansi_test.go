package ansiext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscape_NormalText(t *testing.T) {
	input := "Hello, World! 123 ~`@#$%"
	assert.Equal(t, input, Escape(input))
}

func TestEscape_NullByte(t *testing.T) {
	assert.Equal(t, "\u2400", Escape("\x00"))
}

func TestEscape_ControlCharRange(t *testing.T) {
	// SOH (0x01) -> U+2401
	assert.Equal(t, "\u2401", Escape("\x01"))
	// BEL (0x07) -> U+2407
	assert.Equal(t, "\u2407", Escape("\x07"))
	// ESC (0x1b) -> U+241B
	assert.Equal(t, "\u241b", Escape("\x1b"))
}

func TestEscape_DEL(t *testing.T) {
	assert.Equal(t, "\u2421", Escape("\x7f"))
}

func TestEscape_MixedContent(t *testing.T) {
	input := "hello\x00world\x1b!"
	expected := "hello\u2400world\u241b!"
	assert.Equal(t, expected, Escape(input))
}
