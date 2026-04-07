package notification_test

import (
	"errors"
	"testing"

	"github.com/charmbracelet/crush/internal/ui/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopBackend_Send(t *testing.T) {
	t.Parallel()

	backend := notification.NoopBackend{}
	err := backend.Send(notification.Notification{
		Title:   "Test Title",
		Message: "Test Message",
	})
	require.NoError(t, err)
}

func TestNativeBackend_Send(t *testing.T) {
	t.Parallel()

	backend := notification.NewNativeBackend(nil)

	var capturedTitle, capturedMessage string
	var capturedIcon any
	backend.SetNotifyFunc(func(title, message string, icon any) error {
		capturedTitle = title
		capturedMessage = message
		capturedIcon = icon
		return nil
	})

	err := backend.Send(notification.Notification{
		Title:   "Hello",
		Message: "World",
	})
	require.NoError(t, err)
	require.Equal(t, "Hello", capturedTitle)
	require.Equal(t, "World", capturedMessage)
	require.Nil(t, capturedIcon)
}

func TestNativeBackend_Send_WithIcon(t *testing.T) {
	t.Parallel()

	iconData := []byte("fake-icon-data")
	backend := notification.NewNativeBackend(iconData)

	var capturedIcon any
	backend.SetNotifyFunc(func(title, message string, icon any) error {
		capturedIcon = icon
		return nil
	})

	err := backend.Send(notification.Notification{Title: "T", Message: "M"})
	require.NoError(t, err)
	assert.Equal(t, iconData, capturedIcon, "icon should be forwarded to notifyFunc")
}

func TestNativeBackend_Send_Error(t *testing.T) {
	t.Parallel()

	backend := notification.NewNativeBackend(nil)
	backend.SetNotifyFunc(func(title, message string, icon any) error {
		return errors.New("notification daemon unavailable")
	})

	err := backend.Send(notification.Notification{Title: "T", Message: "M"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notification daemon unavailable")
}

func TestNoopBackend_ImplementsBackendInterface(t *testing.T) {
	t.Parallel()

	// Compile-time check is implicit, but confirm at runtime that the zero
	// value satisfies the Backend interface without panicking.
	var backend notification.Backend = notification.NoopBackend{}
	err := backend.Send(notification.Notification{})
	require.NoError(t, err)
}
