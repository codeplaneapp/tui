package log

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/observability"
)

// NewHTTPClient creates an HTTP client with debug logging enabled when debug mode is on.
func NewHTTPClient() *http.Client {
	return NewHTTPClientWithComponent("http")
}

// NewHTTPClientWithComponent creates an HTTP client with observability labels.
func NewHTTPClientWithComponent(component string) *http.Client {
	return NewHTTPClientWithTransport(component, http.DefaultTransport, 0)
}

// NewHTTPClientWithTransport creates an HTTP client using the provided
// transport, observability component label, and timeout.
func NewHTTPClientWithTransport(component string, transport http.RoundTripper, timeout time.Duration) *http.Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &HTTPRoundTripLogger{
			Transport: transport,
			Component: component,
		},
	}
}

// HTTPRoundTripLogger is an http.RoundTripper that logs requests and responses.
type HTTPRoundTripLogger struct {
	Transport http.RoundTripper
	Component string
}

// RoundTrip implements http.RoundTripper interface with logging.
func (h *HTTPRoundTripLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	debugEnabled := slog.Default().Enabled(req.Context(), slog.LevelDebug)

	var (
		err  error
		save io.ReadCloser
	)
	if debugEnabled {
		save, req.Body, err = drainBody(req.Body)
		if err != nil {
			observability.LogAttrs(req.Context(), slog.LevelError, "HTTP request failed",
				slog.String("method", req.Method),
				slog.String("url", observability.RedactURLString(req.URL.String())),
				slog.Any("error", err),
			)
			return nil, err
		}

		observability.LogAttrs(req.Context(), slog.LevelDebug, "HTTP request",
			slog.String("method", req.Method),
			slog.String("url", observability.RedactURLString(req.URL.String())),
			slog.Any("headers", formatHeaders(req.Header)),
			slog.String("body", bodyToString(save, req.Header.Get("Content-Type"))),
		)
	}

	start := time.Now()
	resp, err := (&observability.InstrumentedRoundTripper{
		Transport: h.Transport,
		Component: defaultComponent(h.Component),
	}).RoundTrip(req)
	duration := time.Since(start)
	if err != nil {
		observability.LogAttrs(req.Context(), slog.LevelError, "HTTP request failed",
			slog.String("method", req.Method),
			slog.String("url", observability.RedactURLString(req.URL.String())),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.Any("error", err),
		)
		return resp, err
	}

	if debugEnabled && shouldLogResponseBody(req, resp) {
		save, resp.Body, err = drainBody(resp.Body)
		if err != nil {
			slog.Error("Failed to drain response body", "error", err)
			return resp, err
		}
		observability.LogAttrs(req.Context(), slog.LevelDebug, "HTTP response",
			slog.Int("status_code", resp.StatusCode),
			slog.String("status", resp.Status),
			slog.Any("headers", formatHeaders(resp.Header)),
			slog.String("body", bodyToString(save, resp.Header.Get("Content-Type"))),
			slog.Int64("content_length", resp.ContentLength),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	} else if debugEnabled {
		observability.LogAttrs(req.Context(), slog.LevelDebug, "HTTP response",
			slog.Int("status_code", resp.StatusCode),
			slog.String("status", resp.Status),
			slog.Any("headers", formatHeaders(resp.Header)),
			slog.String("body", "[stream omitted]"),
			slog.Int64("content_length", resp.ContentLength),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	}
	return resp, nil
}

func defaultComponent(component string) string {
	if strings.TrimSpace(component) == "" {
		return "http"
	}
	return component
}

func shouldLogResponseBody(req *http.Request, resp *http.Response) bool {
	if req != nil && strings.Contains(strings.ToLower(req.Header.Get("Accept")), "text/event-stream") {
		return false
	}
	if resp != nil && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return false
	}
	return true
}

func bodyToString(body io.ReadCloser, contentType string) string {
	if body == nil {
		return ""
	}
	src, err := io.ReadAll(body)
	if err != nil {
		slog.Error("Failed to read body", "error", err)
		return ""
	}
	return observability.RedactPayload(contentType, src)
}

// formatHeaders formats HTTP headers for logging, filtering out sensitive information.
func formatHeaders(headers http.Header) map[string][]string {
	return observability.RedactHeaders(headers)
}

func drainBody(b io.ReadCloser) (r1, r2 io.ReadCloser, err error) {
	if b == nil || b == http.NoBody {
		return http.NoBody, http.NoBody, nil
	}
	var buf bytes.Buffer
	if _, err = buf.ReadFrom(b); err != nil {
		return nil, b, err
	}
	if err = b.Close(); err != nil {
		return nil, b, err
	}
	return io.NopCloser(&buf), io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}
