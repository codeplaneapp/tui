package stringext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapitalize(t *testing.T) {
	assert.Equal(t, "Hello World", Capitalize("hello world"))
	assert.Equal(t, "", Capitalize(""))
}

func TestNormalizeSpace_CRLF(t *testing.T) {
	input := "line1\r\nline2\r\nline3"
	result := NormalizeSpace(input)

	assert.NotContains(t, result, "\r\n")
	assert.Contains(t, result, "line1\nline2\nline3")
}

func TestNormalizeSpace_Tabs(t *testing.T) {
	input := "col1\tcol2"
	result := NormalizeSpace(input)

	assert.NotContains(t, result, "\t")
	assert.Equal(t, "col1    col2", result)
}

func TestNormalizeSpace_TrimWhitespace(t *testing.T) {
	input := "  hello  "
	result := NormalizeSpace(input)

	assert.Equal(t, "hello", result)
}

func TestNormalizeSpace_Combined(t *testing.T) {
	input := "  \thello\r\nworld\t  "
	result := NormalizeSpace(input)

	assert.Equal(t, "hello\nworld", result)
}
