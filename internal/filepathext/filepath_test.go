package filepathext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSmartIsAbs_AbsolutePath(t *testing.T) {
	assert.True(t, SmartIsAbs("/foo/bar"))
}

func TestSmartIsAbs_RelativePath(t *testing.T) {
	assert.False(t, SmartIsAbs("foo/bar"))
}

func TestSmartJoin_SecondAbsolute(t *testing.T) {
	result := SmartJoin("base", "/abs/path")
	assert.Equal(t, "/abs/path", result)
}

func TestSmartJoin_SecondRelative(t *testing.T) {
	result := SmartJoin("/base", "rel/path")
	assert.Equal(t, "/base/rel/path", result)
}

func TestSmartJoin_BothRelative(t *testing.T) {
	result := SmartJoin("a", "b")
	assert.Equal(t, "a/b", result)
}
