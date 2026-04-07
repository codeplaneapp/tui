package home

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_WithXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	assert.Equal(t, "/custom/config", Config())
}

func TestConfig_FallbackToHomeDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	want := filepath.Join(Dir(), ".config")
	assert.Equal(t, want, Config())
}
