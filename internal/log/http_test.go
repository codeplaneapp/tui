package log

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPRoundTripLogger(t *testing.T) {
	// Create a test server that returns a 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal server error", "code": 500}`))
	}))
	defer server.Close()

	// Create HTTP client with logging
	client := NewHTTPClient()

	// Make a request
	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL,
		strings.NewReader(`{"test": "data"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Verify response
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code 500, got %d", resp.StatusCode)
	}
}

func TestFormatHeaders(t *testing.T) {
	headers := http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Bearer secret-token"},
		"X-API-Key":     []string{"api-key-123"},
		"User-Agent":    []string{"test-agent"},
	}

	formatted := formatHeaders(headers)

	// Check that sensitive headers are redacted
	if formatted["Authorization"][0] != "[REDACTED]" {
		t.Error("Authorization header should be redacted")
	}
	if formatted["X-API-Key"][0] != "[REDACTED]" {
		t.Error("X-API-Key header should be redacted")
	}

	// Check that non-sensitive headers are preserved
	if formatted["Content-Type"][0] != "application/json" {
		t.Error("Content-Type header should be preserved")
	}
	if formatted["User-Agent"][0] != "test-agent" {
		t.Error("User-Agent header should be preserved")
	}
}

func TestBodyToStringRedactsSensitiveJSON(t *testing.T) {
	body := io.NopCloser(strings.NewReader(`{"api_key":"secret","nested":{"token":"hidden"},"session_id":"sess-123"}`))

	rendered := bodyToString(body, "application/json")

	if strings.Contains(rendered, "secret") || strings.Contains(rendered, "hidden") {
		t.Fatalf("rendered body leaked a secret: %s", rendered)
	}
	if !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("rendered body should contain redaction marker: %s", rendered)
	}
	if !strings.Contains(rendered, "sess-123") {
		t.Fatalf("rendered body should preserve session identifiers for debugging: %s", rendered)
	}
}

func TestNewHTTPClientWithTransportDoesNotDrainSSEBody(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	released := false
	releaseStream := func() {
		if !released {
			close(release)
			released = true
		}
	}
	defer releaseStream()

	client := NewHTTPClientWithTransport("test_stream", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		pr, pw := io.Pipe()
		go func() {
			_, _ = io.WriteString(pw, "data: first\n\n")
			<-release
			_ = pw.Close()
		}()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body: pr,
		}, nil
	}), 0)

	done := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/events", nil)
		if err != nil {
			done <- err
			return
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := client.Do(req)
		if err != nil {
			done <- err
			return
		}
		defer resp.Body.Close()

		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		if !strings.Contains(line, "data: first") {
			done <- io.ErrUnexpectedEOF
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		releaseStream()
		t.Fatal("HTTP client blocked on SSE body before returning the response")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
