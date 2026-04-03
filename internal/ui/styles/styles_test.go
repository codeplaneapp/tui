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
