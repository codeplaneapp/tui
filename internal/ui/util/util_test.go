package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmdHandler(t *testing.T) {
	msg := "hello"
	cmd := CmdHandler(msg)
	require.NotNil(t, cmd)
	got := cmd()
	assert.Equal(t, "hello", got)
}

func TestNewInfoMsg(t *testing.T) {
	m := NewInfoMsg("something happened")
	assert.Equal(t, InfoTypeInfo, m.Type)
	assert.Equal(t, "something happened", m.Msg)
}

func TestNewWarnMsg(t *testing.T) {
	m := NewWarnMsg("watch out")
	assert.Equal(t, InfoTypeWarn, m.Type)
	assert.Equal(t, "watch out", m.Msg)
}

func TestNewErrorMsg(t *testing.T) {
	err := errors.New("broken")
	m := NewErrorMsg(err)
	assert.Equal(t, InfoTypeError, m.Type)
	assert.Equal(t, "broken", m.Msg)
}

func TestInfoMsg_IsEmpty(t *testing.T) {
	t.Run("zero value is empty", func(t *testing.T) {
		var m InfoMsg
		assert.True(t, m.IsEmpty())
	})

	t.Run("info msg is not empty", func(t *testing.T) {
		m := NewInfoMsg("data")
		assert.False(t, m.IsEmpty())
	})

	t.Run("warn msg is not empty", func(t *testing.T) {
		m := NewWarnMsg("caution")
		assert.False(t, m.IsEmpty())
	})

	t.Run("error msg is not empty", func(t *testing.T) {
		m := NewErrorMsg(errors.New("fail"))
		assert.False(t, m.IsEmpty())
	})
}

func TestReportError(t *testing.T) {
	cmd := ReportError(errors.New("oops"))
	require.NotNil(t, cmd)
	got := cmd()
	msg, ok := got.(InfoMsg)
	require.True(t, ok)
	assert.Equal(t, InfoTypeError, msg.Type)
	assert.Equal(t, "oops", msg.Msg)
}

func TestReportInfo(t *testing.T) {
	cmd := ReportInfo("all good")
	require.NotNil(t, cmd)
	got := cmd()
	msg, ok := got.(InfoMsg)
	require.True(t, ok)
	assert.Equal(t, InfoTypeInfo, msg.Type)
	assert.Equal(t, "all good", msg.Msg)
}

func TestReportWarn(t *testing.T) {
	cmd := ReportWarn("heads up")
	require.NotNil(t, cmd)
	got := cmd()
	msg, ok := got.(InfoMsg)
	require.True(t, ok)
	assert.Equal(t, InfoTypeWarn, msg.Type)
	assert.Equal(t, "heads up", msg.Msg)
}
