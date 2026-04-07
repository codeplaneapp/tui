package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- findWordBoundaries tests ---

func TestFindWordBoundaries_EmptyLine(t *testing.T) {
	t.Parallel()

	start, end := findWordBoundaries("", 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, end)
}

func TestFindWordBoundaries_NegativeCol(t *testing.T) {
	t.Parallel()

	start, end := findWordBoundaries("hello world", -1)
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, end)
}

func TestFindWordBoundaries_SingleWord(t *testing.T) {
	t.Parallel()

	// Clicking in the middle of "hello" should select the whole word.
	start, end := findWordBoundaries("hello", 2)
	assert.Equal(t, 0, start, "start should be beginning of word")
	assert.Equal(t, 5, end, "end should be after last char of word")
}

func TestFindWordBoundaries_ClickOnWhitespace(t *testing.T) {
	t.Parallel()

	// Clicking on the space between "hello" and "world" should return empty selection.
	start, end := findWordBoundaries("hello world", 5)
	assert.Equal(t, start, end, "clicking on whitespace should return empty selection")
}

func TestFindWordBoundaries_SecondWord(t *testing.T) {
	t.Parallel()

	// Clicking on "world" (starts at col 6).
	start, end := findWordBoundaries("hello world", 7)
	assert.Equal(t, 6, start, "start should be beginning of 'world'")
	assert.Equal(t, 11, end, "end should be after 'world'")
}

func TestFindWordBoundaries_ClickPastEnd(t *testing.T) {
	t.Parallel()

	// Clicking past the end of line should return col,col.
	start, end := findWordBoundaries("hi", 10)
	assert.Equal(t, start, end, "clicking past end should return empty selection")
}

func TestFindWordBoundaries_FirstCharOfWord(t *testing.T) {
	t.Parallel()

	// Clicking at the very start of the first word.
	start, end := findWordBoundaries("hello world", 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 5, end)
}

// --- abs tests ---

func TestAbs_Positive(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5, abs(5))
}

func TestAbs_Negative(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5, abs(-5))
}

func TestAbs_Zero(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, abs(0))
}
