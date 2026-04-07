package list

import (
	"image"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToStyle_Bold(t *testing.T) {
	t.Parallel()
	lg := lipgloss.NewStyle().Bold(true)
	got := ToStyle(lg)

	assert.Equal(t, uint8(uv.AttrBold), got.Attrs&uv.AttrBold,
		"bold attribute should be set")
}

func TestToStyle_MultipleAttrs(t *testing.T) {
	t.Parallel()
	lg := lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Faint(true).
		Strikethrough(true).
		Reverse(true).
		Blink(true).
		Underline(true)

	got := ToStyle(lg)

	assert.NotZero(t, got.Attrs&uv.AttrBold, "bold should be set")
	assert.NotZero(t, got.Attrs&uv.AttrItalic, "italic should be set")
	assert.NotZero(t, got.Attrs&uv.AttrFaint, "faint should be set")
	assert.NotZero(t, got.Attrs&uv.AttrStrikethrough, "strikethrough should be set")
	assert.NotZero(t, got.Attrs&uv.AttrReverse, "reverse should be set")
	assert.NotZero(t, got.Attrs&uv.AttrBlink, "blink should be set")
	assert.Equal(t, uv.UnderlineSingle, got.Underline, "underline should be single")
}

func TestToStyle_NoAttrs(t *testing.T) {
	t.Parallel()
	lg := lipgloss.NewStyle()
	got := ToStyle(lg)

	assert.Equal(t, uint8(0), got.Attrs, "no attributes should be set on an empty style")
	assert.Equal(t, uv.Underline(0), got.Underline, "underline should be none")
}

func TestAdjustArea_ZeroPadding(t *testing.T) {
	t.Parallel()
	area := image.Rect(0, 0, 100, 50)
	style := lipgloss.NewStyle()

	got := AdjustArea(area, style)

	assert.Equal(t, area, got, "area should be unchanged when style has no margin/border/padding")
}

func TestAdjustArea_WithPadding(t *testing.T) {
	t.Parallel()
	area := image.Rect(0, 0, 100, 50)
	style := lipgloss.NewStyle().Padding(2, 4, 6, 8)

	got := AdjustArea(area, style)

	// Padding: top=2, right=4, bottom=6, left=8
	expected := image.Rect(8, 2, 96, 44)
	assert.Equal(t, expected, got)
}

func TestAdjustArea_WithMarginAndPadding(t *testing.T) {
	t.Parallel()
	area := image.Rect(10, 10, 110, 60)
	style := lipgloss.NewStyle().
		Margin(1, 2, 3, 4).
		Padding(1, 1, 1, 1)

	got := AdjustArea(area, style)

	// margin: top=1, right=2, bottom=3, left=4
	// padding: top=1, right=1, bottom=1, left=1
	// Min.X = 10 + 4 + 0(border) + 1 = 15
	// Min.Y = 10 + 1 + 0(border) + 1 = 12
	// Max.X = 110 - (2 + 0 + 1) = 107
	// Max.Y = 60 - (3 + 0 + 1) = 56
	expected := image.Rect(15, 12, 107, 56)
	assert.Equal(t, expected, got)
}

func TestToHighlighter_AppliesStyle(t *testing.T) {
	t.Parallel()
	lg := lipgloss.NewStyle().Bold(true).Italic(true)
	h := ToHighlighter(lg)

	cell := &uv.Cell{Content: "x"}
	result := h(0, 0, cell)

	require.NotNil(t, result)
	assert.NotZero(t, result.Style.Attrs&uv.AttrBold, "bold should be set on cell")
	assert.NotZero(t, result.Style.Attrs&uv.AttrItalic, "italic should be set on cell")
}

func TestToHighlighter_NilCell(t *testing.T) {
	t.Parallel()
	lg := lipgloss.NewStyle().Bold(true)
	h := ToHighlighter(lg)

	result := h(0, 0, nil)
	assert.Nil(t, result, "nil cell should remain nil")
}
