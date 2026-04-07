package styles

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/charmbracelet/x/exp/charmtone"
	"github.com/stretchr/testify/require"
)

func TestDefaultStyles_SmithersPalette(t *testing.T) {
	t.Parallel()

	s := DefaultStyles()

	require.Equal(t, "#63b3ed", colorHex(s.Primary))
	require.Equal(t, "#e2e8f0", colorHex(s.Secondary))
	require.Equal(t, colorHex(s.Primary), colorHex(s.BorderColor))

	require.NotEqual(t, colorHex(charmtone.Charple), colorHex(s.Primary))
	require.NotEqual(t, colorHex(charmtone.Dolly), colorHex(s.Secondary))
}

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

func TestForegroundGrad_EmptyString(t *testing.T) {
	t.Parallel()
	s := DefaultStyles()
	result := ForegroundGrad(&s, "", false, s.Primary, s.Secondary)
	require.Len(t, result, 1)
	require.Equal(t, "", result[0])
}

func TestForegroundGrad_SingleChar(t *testing.T) {
	t.Parallel()
	s := DefaultStyles()
	result := ForegroundGrad(&s, "X", false, s.Primary, s.Secondary)
	require.Len(t, result, 1)
	require.Contains(t, result[0], "X")
}

func TestForegroundGrad_MultiChar_ProducesClusterPerGrapheme(t *testing.T) {
	t.Parallel()
	s := DefaultStyles()
	result := ForegroundGrad(&s, "ABC", true, s.Primary, s.Secondary)
	require.Len(t, result, 3, "should produce one styled cluster per grapheme")
}

func TestApplyForegroundGrad_NonEmpty(t *testing.T) {
	t.Parallel()
	s := DefaultStyles()
	out := ApplyForegroundGrad(&s, "HELLO", s.Primary, s.Secondary)
	require.NotEmpty(t, out)
	require.Contains(t, out, "H")
	require.Contains(t, out, "O")
}

func TestApplyBoldForegroundGrad_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()
	s := DefaultStyles()
	out := ApplyBoldForegroundGrad(&s, "", s.Primary, s.Secondary)
	require.Equal(t, "", out)
}
