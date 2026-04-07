package hyper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceNameFromHostnameUsesCodeplaneBranding(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Codeplane", deviceNameFromHostname(""))
	require.Equal(t, "Codeplane (my-host)", deviceNameFromHostname("my-host"))
}

func TestDefaultUserAgentUsesCodeplaneBranding(t *testing.T) {
	t.Parallel()

	require.Equal(t, "codeplane", defaultUserAgent)
}
