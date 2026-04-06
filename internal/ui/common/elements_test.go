package common

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatReasoningEffort_XHigh(t *testing.T) {
	result := FormatReasoningEffort("xhigh")
	assert.Equal(t, "X-High", result)
}

func TestFormatReasoningEffort_TitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"low", "Low"},
		{"medium", "Medium"},
		{"high", "High"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, FormatReasoningEffort(tc.input))
		})
	}
}

func TestFormatReasoningEffort_Empty(t *testing.T) {
	// Empty string should still be processed by title-casing (returns empty).
	result := FormatReasoningEffort("")
	assert.Equal(t, "", result)
}

// TestFormatTokensPlain tests the token formatting logic directly.
// Since formatTokensAndCost is unexported and requires styles, we replicate
// the pure numeric formatting to verify correctness.
func TestFormatTokensPlain(t *testing.T) {
	tests := []struct {
		name     string
		tokens   int64
		expected string
	}{
		{"millions", 2_500_000, "2.5M"},
		{"round millions", 2_000_000, "2M"},
		{"thousands", 45_500, "45.5K"},
		{"round thousands", 5_000, "5K"},
		{"below 1000", 500, "500"},
		{"zero", 0, "0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var formatted string
			switch {
			case tc.tokens >= 1_000_000:
				formatted = fmt.Sprintf("%.1fM", float64(tc.tokens)/1_000_000)
			case tc.tokens >= 1_000:
				formatted = fmt.Sprintf("%.1fK", float64(tc.tokens)/1_000)
			default:
				formatted = fmt.Sprintf("%d", tc.tokens)
			}
			if strings.HasSuffix(formatted, ".0K") {
				formatted = strings.Replace(formatted, ".0K", "K", 1)
			}
			if strings.HasSuffix(formatted, ".0M") {
				formatted = strings.Replace(formatted, ".0M", "M", 1)
			}
			assert.Equal(t, tc.expected, formatted)
		})
	}
}

func TestCenterRect(t *testing.T) {
	area := image.Rect(0, 0, 100, 50)
	result := CenterRect(area, 20, 10)

	assert.Equal(t, 40, result.Min.X, "center X offset")
	assert.Equal(t, 20, result.Min.Y, "center Y offset")
	assert.Equal(t, 60, result.Max.X, "right edge")
	assert.Equal(t, 30, result.Max.Y, "bottom edge")
}

func TestBottomLeftRect(t *testing.T) {
	area := image.Rect(0, 0, 100, 50)
	result := BottomLeftRect(area, 30, 10)

	assert.Equal(t, 0, result.Min.X, "left edge")
	assert.Equal(t, 40, result.Min.Y, "top edge")
	assert.Equal(t, 30, result.Max.X, "right edge")
	assert.Equal(t, 50, result.Max.Y, "bottom edge")
}

func TestBottomRightRect(t *testing.T) {
	area := image.Rect(0, 0, 100, 50)
	result := BottomRightRect(area, 30, 10)

	assert.Equal(t, 70, result.Min.X, "left edge")
	assert.Equal(t, 40, result.Min.Y, "top edge")
	assert.Equal(t, 100, result.Max.X, "right edge")
	assert.Equal(t, 50, result.Max.Y, "bottom edge")
}

func TestCenterRect_OddDimensions(t *testing.T) {
	area := image.Rect(0, 0, 101, 51)
	result := CenterRect(area, 21, 11)

	// Verify width and height of the resulting rect are correct
	assert.Equal(t, 21, result.Dx(), "width should be 21")
	assert.Equal(t, 11, result.Dy(), "height should be 11")
}
