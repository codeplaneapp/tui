package anim

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColorIsUnset(t *testing.T) {
	t.Run("nil is unset", func(t *testing.T) {
		assert.True(t, colorIsUnset(nil))
	})

	t.Run("transparent color is unset", func(t *testing.T) {
		c := color.RGBA{R: 0xff, G: 0, B: 0, A: 0}
		assert.True(t, colorIsUnset(c))
	})

	t.Run("opaque color is not unset", func(t *testing.T) {
		c := color.RGBA{R: 0xff, G: 0, B: 0, A: 0xff}
		assert.False(t, colorIsUnset(c))
	})
}

func TestNextID(t *testing.T) {
	id1 := nextID()
	id2 := nextID()
	assert.Greater(t, id2, id1, "IDs should be monotonically increasing")
}

func TestSettingsHash(t *testing.T) {
	base := Settings{
		Size:       10,
		Label:      "thinking",
		LabelColor: defaultLabelColor,
		GradColorA: defaultGradColorA,
		GradColorB: defaultGradColorB,
	}

	t.Run("same settings produce same hash", func(t *testing.T) {
		h1 := settingsHash(base)
		h2 := settingsHash(base)
		assert.Equal(t, h1, h2)
	})

	t.Run("different settings produce different hash", func(t *testing.T) {
		other := base
		other.Label = "loading"
		assert.NotEqual(t, settingsHash(base), settingsHash(other))
	})
}

func TestMakeGradientRamp(t *testing.T) {
	t.Run("returns nil with fewer than 2 stops", func(t *testing.T) {
		assert.Nil(t, makeGradientRamp(10))
		assert.Nil(t, makeGradientRamp(10, defaultGradColorA))
	})

	t.Run("returns requested number of colors", func(t *testing.T) {
		ramp := makeGradientRamp(20, defaultGradColorA, defaultGradColorB)
		require.NotNil(t, ramp)
		assert.Len(t, ramp, 20)
	})

	t.Run("multi-stop gradient", func(t *testing.T) {
		white := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		ramp := makeGradientRamp(30, defaultGradColorA, white, defaultGradColorB)
		require.NotNil(t, ramp)
		assert.Len(t, ramp, 30)
	})
}
