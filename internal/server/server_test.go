package server

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHostURL_TCP(t *testing.T) {
	u, err := ParseHostURL("tcp://localhost:8080")
	require.NoError(t, err)
	assert.Equal(t, "tcp", u.Scheme)
	assert.Equal(t, "localhost:8080", u.Host)
}

func TestParseHostURL_TCPWithPath(t *testing.T) {
	u, err := ParseHostURL("tcp://localhost:8080/v1")
	require.NoError(t, err)
	assert.Equal(t, "tcp", u.Scheme)
	assert.Equal(t, "localhost:8080", u.Host)
	assert.Equal(t, "/v1", u.Path)
}

func TestParseHostURL_Unix(t *testing.T) {
	u, err := ParseHostURL("unix:///tmp/codeplane.sock")
	require.NoError(t, err)
	assert.Equal(t, "unix", u.Scheme)
	assert.Equal(t, "/tmp/codeplane.sock", u.Host)
}

func TestParseHostURL_NamedPipe(t *testing.T) {
	u, err := ParseHostURL("npipe:////./pipe/codeplane.sock")
	require.NoError(t, err)
	assert.Equal(t, "npipe", u.Scheme)
}

func TestParseHostURL_InvalidFormat(t *testing.T) {
	_, err := ParseHostURL("no-scheme")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid host format")
}

func TestDefaultHost_ContainsCodeplane(t *testing.T) {
	host := DefaultHost()
	assert.Contains(t, host, "codeplane")
}

func TestDefaultHost_HasScheme(t *testing.T) {
	host := DefaultHost()
	if runtime.GOOS == "windows" {
		assert.True(t, strings.HasPrefix(host, "npipe://"), "expected npipe:// prefix, got: %s", host)
	} else {
		assert.True(t, strings.HasPrefix(host, "unix://"), "expected unix:// prefix, got: %s", host)
	}
}
