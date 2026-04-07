package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrandingConstantsUseCodeplane(t *testing.T) {
	require.Contains(t, githubApiURL, "/charmbracelet/codeplane/releases/latest")
	require.Equal(t, "codeplane/1.0", userAgent)
}

func TestCheckForUpdate_Old(t *testing.T) {
	info, err := Check(t.Context(), "v0.10.0", testClient{"v0.11.0"})
	require.NoError(t, err)
	require.NotNil(t, info)
	require.True(t, info.Available())
}

func TestCheckForUpdate_Beta(t *testing.T) {
	t.Run("current is stable", func(t *testing.T) {
		info, err := Check(t.Context(), "v0.10.0", testClient{"v0.11.0-beta.1"})
		require.NoError(t, err)
		require.NotNil(t, info)
		require.False(t, info.Available())
	})

	t.Run("current is also beta", func(t *testing.T) {
		info, err := Check(t.Context(), "v0.11.0-beta.1", testClient{"v0.11.0-beta.2"})
		require.NoError(t, err)
		require.NotNil(t, info)
		require.True(t, info.Available())
	})

	t.Run("current is beta, latest isn't", func(t *testing.T) {
		info, err := Check(t.Context(), "v0.11.0-beta.1", testClient{"v0.11.0"})
		require.NoError(t, err)
		require.NotNil(t, info)
		require.True(t, info.Available())
	})
}

func TestInfo_IsDevelopment_Devel(t *testing.T) {
	info := Info{Current: "devel"}
	assert.True(t, info.IsDevelopment())
}

func TestInfo_IsDevelopment_Unknown(t *testing.T) {
	info := Info{Current: "unknown"}
	assert.True(t, info.IsDevelopment())
}

func TestInfo_IsDevelopment_Dirty(t *testing.T) {
	info := Info{Current: "0.10.0-dirty"}
	assert.True(t, info.IsDevelopment())
}

func TestInfo_IsDevelopment_GoInstall(t *testing.T) {
	info := Info{Current: "v0.0.0-0.20251231235959-06c807842604"}
	assert.True(t, info.IsDevelopment())
}

func TestInfo_IsDevelopment_StableVersion(t *testing.T) {
	info := Info{Current: "0.10.0"}
	assert.False(t, info.IsDevelopment())
}

func TestInfo_Available_SameVersion(t *testing.T) {
	info := Info{Current: "0.10.0", Latest: "0.10.0"}
	assert.False(t, info.Available())
}

func TestInfo_Available_DifferentStable(t *testing.T) {
	info := Info{Current: "0.10.0", Latest: "0.11.0"}
	assert.True(t, info.Available())
}

func TestCheck_TrimsVPrefix(t *testing.T) {
	info, err := Check(t.Context(), "v0.10.0", testClient{"v0.11.0"})
	require.NoError(t, err)
	assert.Equal(t, "0.10.0", info.Current)
}

func TestCheck_PropagatesURL(t *testing.T) {
	info, err := Check(t.Context(), "v0.10.0", testClient{"v0.11.0"})
	require.NoError(t, err)
	assert.Equal(t, "https://example.org", info.URL)
}

type testClient struct{ tag string }

// Latest implements Client.
func (t testClient) Latest(ctx context.Context) (*Release, error) {
	return &Release{
		TagName: t.tag,
		HTMLURL: "https://example.org",
	}, nil
}
