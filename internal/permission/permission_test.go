package permission

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionService_AllowedCommands(t *testing.T) {
	tests := []struct {
		name         string
		allowedTools []string
		toolName     string
		action       string
		expected     bool
	}{
		{
			name:         "tool in allowlist",
			allowedTools: []string{"bash", "view"},
			toolName:     "bash",
			action:       "execute",
			expected:     true,
		},
		{
			name:         "tool:action in allowlist",
			allowedTools: []string{"bash:execute", "edit:create"},
			toolName:     "bash",
			action:       "execute",
			expected:     true,
		},
		{
			name:         "tool not in allowlist",
			allowedTools: []string{"view", "ls"},
			toolName:     "bash",
			action:       "execute",
			expected:     false,
		},
		{
			name:         "tool:action not in allowlist",
			allowedTools: []string{"bash:read", "edit:create"},
			toolName:     "bash",
			action:       "execute",
			expected:     false,
		},
		{
			name:         "empty allowlist",
			allowedTools: []string{},
			toolName:     "bash",
			action:       "execute",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewPermissionService("/tmp", false, tt.allowedTools)

			// Create a channel to capture the permission request
			// Since we're testing the allowlist logic, we need to simulate the request
			ps := service.(*permissionService)

			// Test the allowlist logic directly
			commandKey := tt.toolName + ":" + tt.action
			allowed := false
			for _, cmd := range ps.allowedTools {
				if cmd == commandKey || cmd == tt.toolName {
					allowed = true
					break
				}
			}

			if allowed != tt.expected {
				t.Errorf("expected %v, got %v for tool %s action %s with allowlist %v",
					tt.expected, allowed, tt.toolName, tt.action, tt.allowedTools)
			}
		})
	}
}

func TestPermissionService_SkipMode(t *testing.T) {
	service := NewPermissionService("/tmp", true, []string{})

	result, err := service.Request(t.Context(), CreatePermissionRequest{
		SessionID:   "test-session",
		ToolName:    "bash",
		Action:      "execute",
		Description: "test command",
		Path:        "/tmp",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected permission to be granted in skip mode")
	}
}

func TestPermissionService_SequentialProperties(t *testing.T) {
	t.Run("Sequential permission requests with persistent grants", func(t *testing.T) {
		service := NewPermissionService("/tmp", false, []string{})

		req1 := CreatePermissionRequest{
			SessionID:   "session1",
			ToolName:    "file_tool",
			Description: "Read file",
			Action:      "read",
			Params:      map[string]string{"file": "test.txt"},
			Path:        "/tmp/test.txt",
		}

		var result1 bool
		var wg sync.WaitGroup
		wg.Add(1)

		events := service.Subscribe(t.Context())

		go func() {
			defer wg.Done()
			result1, _ = service.Request(t.Context(), req1)
		}()

		var permissionReq PermissionRequest
		event := <-events

		permissionReq = event.Payload
		service.GrantPersistent(permissionReq)

		wg.Wait()
		assert.True(t, result1, "First request should be granted")

		// Second identical request should be automatically approved due to persistent permission
		req2 := CreatePermissionRequest{
			SessionID:   "session1",
			ToolName:    "file_tool",
			Description: "Read file again",
			Action:      "read",
			Params:      map[string]string{"file": "test.txt"},
			Path:        "/tmp/test.txt",
		}
		result2, err := service.Request(t.Context(), req2)
		require.NoError(t, err)
		assert.True(t, result2, "Second request should be auto-approved")
	})
	t.Run("Sequential requests with temporary grants", func(t *testing.T) {
		service := NewPermissionService("/tmp", false, []string{})

		req := CreatePermissionRequest{
			SessionID:   "session2",
			ToolName:    "file_tool",
			Description: "Write file",
			Action:      "write",
			Params:      map[string]string{"file": "test.txt"},
			Path:        "/tmp/test.txt",
		}

		events := service.Subscribe(t.Context())
		var result1 bool
		var wg sync.WaitGroup

		wg.Go(func() {
			result1, _ = service.Request(t.Context(), req)
		})

		var permissionReq PermissionRequest
		event := <-events
		permissionReq = event.Payload

		service.Grant(permissionReq)
		wg.Wait()
		assert.True(t, result1, "First request should be granted")

		var result2 bool

		wg.Go(func() {
			result2, _ = service.Request(t.Context(), req)
		})

		event = <-events
		permissionReq = event.Payload
		service.Deny(permissionReq)
		wg.Wait()
		assert.False(t, result2, "Second request should be denied")
	})
	t.Run("Concurrent requests with different outcomes", func(t *testing.T) {
		service := NewPermissionService("/tmp", false, []string{})

		events := service.Subscribe(t.Context())

		var wg sync.WaitGroup
		results := make([]bool, 3)

		requests := []CreatePermissionRequest{
			{
				SessionID:   "concurrent1",
				ToolName:    "tool1",
				Action:      "action1",
				Path:        "/tmp/file1.txt",
				Description: "First concurrent request",
			},
			{
				SessionID:   "concurrent2",
				ToolName:    "tool2",
				Action:      "action2",
				Path:        "/tmp/file2.txt",
				Description: "Second concurrent request",
			},
			{
				SessionID:   "concurrent3",
				ToolName:    "tool3",
				Action:      "action3",
				Path:        "/tmp/file3.txt",
				Description: "Third concurrent request",
			},
		}

		for i, req := range requests {
			wg.Add(1)
			go func(index int, request CreatePermissionRequest) {
				defer wg.Done()
				result, _ := service.Request(t.Context(), request)
				results[index] = result
			}(i, req)
		}

		for range 3 {
			event := <-events
			switch event.Payload.ToolName {
			case "tool1":
				service.Grant(event.Payload)
			case "tool2":
				service.GrantPersistent(event.Payload)
			case "tool3":
				service.Deny(event.Payload)
			}
		}
		wg.Wait()
		grantedCount := 0
		for _, result := range results {
			if result {
				grantedCount++
			}
		}

		assert.Equal(t, 2, grantedCount, "Should have 2 granted and 1 denied")
		secondReq := requests[1]
		secondReq.Description = "Repeat of second request"
		result, err := service.Request(t.Context(), secondReq)
		require.NoError(t, err)
		assert.True(t, result, "Repeated request should be auto-approved due to persistent permission")
	})
}

func TestPermission_RequestContextCancel(t *testing.T) {
	service := NewPermissionService("/tmp", false, []string{})

	ctx, cancel := context.WithCancel(t.Context())

	// Subscribe so we can observe the request being published.
	events := service.Subscribe(t.Context())

	var (
		granted bool
		reqErr  error
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		granted, reqErr = service.Request(ctx, CreatePermissionRequest{
			SessionID:  "cancel-session",
			ToolName:   "bash",
			Action:     "execute",
			Path:       "/tmp",
			ToolCallID: "tc-cancel-1",
		})
	}()

	// Wait for the permission request to be published.
	select {
	case <-events:
		// Request is now blocking in select.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for permission request event")
	}

	// Cancel the context before anyone grants/denies.
	cancel()

	wg.Wait()
	assert.False(t, granted, "should not be granted when context is canceled")
	assert.Error(t, reqErr, "should return an error on context cancellation")
	assert.ErrorIs(t, reqErr, context.Canceled)
}

func TestPermission_RequestRecordsLifecycleSpanAndNotifications(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})
	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	service := NewPermissionService("/tmp", false, []string{})
	requests := service.Subscribe(t.Context())
	notifications := service.SubscribeNotifications(t.Context())

	resultCh := make(chan struct {
		granted bool
		err     error
	}, 1)
	go func() {
		granted, err := service.Request(t.Context(), CreatePermissionRequest{
			SessionID:   "session-1",
			ToolCallID:  "tool-call-1",
			ToolName:    "bash",
			Action:      "execute",
			Description: "run command",
			Path:        "/tmp/test.sh",
		})
		resultCh <- struct {
			granted bool
			err     error
		}{granted: granted, err: err}
	}()

	pending := <-notifications
	require.Equal(t, "tool-call-1", pending.Payload.ToolCallID)
	require.False(t, pending.Payload.Granted)
	require.False(t, pending.Payload.Denied)

	request := <-requests
	service.Grant(request.Payload)

	granted := <-notifications
	require.Equal(t, "tool-call-1", granted.Payload.ToolCallID)
	require.True(t, granted.Payload.Granted)

	result := <-resultCh
	require.NoError(t, result.err)
	require.True(t, result.granted)

	spans := observability.RecentSpans(10)
	require.NotEmpty(t, spans)

	found := false
	for _, span := range spans {
		if span.Name != "permission.request" {
			continue
		}
		found = true
		require.Equal(t, "granted", span.Attributes["permission.result"])
		require.Equal(t, "bash", span.Attributes["permission.tool"])
		require.Equal(t, "execute", span.Attributes["permission.action"])
	}
	require.True(t, found)
}

func TestPermission_SessionAutoApprove(t *testing.T) {
	service := NewPermissionService("/tmp", false, []string{})

	sessionID := "auto-approve-session"

	// Enable auto-approve for this session.
	service.AutoApproveSession(sessionID)

	// Subsequent requests for this session should be auto-approved immediately.
	granted, err := service.Request(t.Context(), CreatePermissionRequest{
		SessionID:  sessionID,
		ToolName:   "bash",
		Action:     "execute",
		Path:       "/tmp",
		ToolCallID: "tc-auto-1",
	})
	require.NoError(t, err)
	assert.True(t, granted, "request should be auto-approved for the session")

	// A different session should NOT be auto-approved (it would block).
	// Verify by starting a request and denying it.
	events := service.Subscribe(t.Context())
	var (
		otherGranted bool
		wg           sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		otherGranted, _ = service.Request(t.Context(), CreatePermissionRequest{
			SessionID:  "other-session",
			ToolName:   "bash",
			Action:     "execute",
			Path:       "/tmp",
			ToolCallID: "tc-auto-2",
		})
	}()

	event := <-events
	service.Deny(event.Payload)
	wg.Wait()
	assert.False(t, otherGranted, "other session should not be auto-approved")
}

func TestPermission_SkipRequests(t *testing.T) {
	// Start with skip=false so requests would normally block.
	service := NewPermissionService("/tmp", false, []string{})

	// Enable skip at runtime.
	service.SetSkipRequests(true)
	assert.True(t, service.SkipRequests(), "SkipRequests should return true after SetSkipRequests(true)")

	// Request should return immediately without blocking.
	granted, err := service.Request(t.Context(), CreatePermissionRequest{
		SessionID:  "skip-session",
		ToolName:   "bash",
		Action:     "execute",
		Path:       "/tmp",
		ToolCallID: "tc-skip-1",
	})
	require.NoError(t, err)
	assert.True(t, granted, "request should be granted when skip is enabled")

	// Disable skip and verify the flag is toggled back.
	service.SetSkipRequests(false)
	assert.False(t, service.SkipRequests(), "SkipRequests should return false after SetSkipRequests(false)")
}
