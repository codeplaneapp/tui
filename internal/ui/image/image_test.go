package image

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetCache(t *testing.T) {
	t.Parallel()

	cachedMutex.Lock()
	cachedImages[imageKey{id: "a", cols: 10, rows: 10}] = cachedImage{
		img:  image.NewRGBA(image.Rect(0, 0, 1, 1)),
		cols: 10,
		rows: 10,
	}
	cachedImages[imageKey{id: "b", cols: 20, rows: 20}] = cachedImage{
		img:  image.NewRGBA(image.Rect(0, 0, 1, 1)),
		cols: 20,
		rows: 20,
	}
	cachedMutex.Unlock()

	ResetCache()

	cachedMutex.RLock()
	length := len(cachedImages)
	cachedMutex.RUnlock()

	require.Equal(t, 0, length)
}

func TestResetIdempotent(t *testing.T) {
	t.Parallel()

	// Calling Reset on an empty cache should not panic.
	ResetCache()

	cachedMutex.RLock()
	length := len(cachedImages)
	cachedMutex.RUnlock()

	require.Equal(t, 0, length)
}

func TestImageKey_ID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      imageKey
		expected string
	}{
		{"basic", imageKey{id: "logo", cols: 10, rows: 5}, "logo-10x5"},
		{"zero dimensions", imageKey{id: "empty", cols: 0, rows: 0}, "empty-0x0"},
		{"large values", imageKey{id: "big", cols: 999, rows: 888}, "big-999x888"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.key.ID())
		})
	}
}

func TestImageKey_Hash_Deterministic(t *testing.T) {
	t.Parallel()

	key := imageKey{id: "test-img", cols: 40, rows: 20}
	h1 := key.Hash()
	h2 := key.Hash()
	assert.Equal(t, h1, h2, "Hash should return the same value on repeated calls")
}

func TestImageKey_Hash_DifferentKeys(t *testing.T) {
	t.Parallel()

	a := imageKey{id: "alpha", cols: 10, rows: 10}
	b := imageKey{id: "beta", cols: 10, rows: 10}
	// Different ids should (almost certainly) produce different hashes.
	assert.NotEqual(t, a.Hash(), b.Hash(), "different image keys should produce different hashes")
}

func TestHasTransmitted(t *testing.T) {
	t.Parallel()

	// Clean slate.
	ResetCache()

	// Not yet transmitted.
	assert.False(t, HasTransmitted("ht-test", 10, 10))

	// Manually cache it.
	key := imageKey{id: "ht-test", cols: 10, rows: 10}
	cachedMutex.Lock()
	cachedImages[key] = cachedImage{
		img:  image.NewRGBA(image.Rect(0, 0, 1, 1)),
		cols: 10,
		rows: 10,
	}
	cachedMutex.Unlock()

	assert.True(t, HasTransmitted("ht-test", 10, 10))
	// Different dimensions should not match.
	assert.False(t, HasTransmitted("ht-test", 20, 20))
}

func TestFitImage_NilImage(t *testing.T) {
	t.Parallel()

	result := fitImage("nil-test", nil, CellSize{Width: 10, Height: 10}, 40, 20)
	assert.Nil(t, result, "fitImage with nil input should return nil")
}

func TestFitImage_ZeroCellSize(t *testing.T) {
	t.Parallel()

	ResetCache()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Zero cell size means we cannot compute pixel dimensions -- should return
	// the original image unchanged.
	result := fitImage("zero-cell", img, CellSize{Width: 0, Height: 0}, 10, 10)
	assert.Equal(t, img, result)
}

func TestRender_Blocks_EmptyCache(t *testing.T) {
	t.Parallel()

	ResetCache()
	// Rendering an image that is not in the cache should return empty string.
	got := EncodingBlocks.Render("missing-id", 10, 10)
	assert.Empty(t, got)
}

func TestRender_Kitty_EmptyCache(t *testing.T) {
	t.Parallel()

	ResetCache()
	got := EncodingKitty.Render("missing-id", 10, 10)
	assert.Empty(t, got)
}
