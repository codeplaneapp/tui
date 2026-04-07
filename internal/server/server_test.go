package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/version"
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

// newTestController creates a controllerV1 with a minimal backend for handler
// testing. The returned shutdownCalled atomic tracks whether the backend's
// shutdown callback was invoked.
func newTestController(t *testing.T) (*controllerV1, *atomic.Bool) {
	t.Helper()
	cfg, err := config.Load(t.TempDir(), t.TempDir(), false)
	require.NoError(t, err)

	shutdownCalled := &atomic.Bool{}
	b := backend.New(context.Background(), cfg, func() {
		shutdownCalled.Store(true)
	})

	s := &Server{}
	c := &controllerV1{backend: b, server: s}
	return c, shutdownCalled
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

func TestHandleGetHealth(t *testing.T) {
	c, _ := newTestController(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()

	c.handleGetHealth(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetVersion(t *testing.T) {
	c, _ := newTestController(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	rec := httptest.NewRecorder()

	c.handleGetVersion(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var info proto.VersionInfo
	err := json.NewDecoder(rec.Body).Decode(&info)
	require.NoError(t, err)

	assert.Equal(t, version.Version, info.Version)
	assert.Equal(t, version.Commit, info.Commit)
	assert.Equal(t, runtime.Version(), info.GoVersion)
	assert.Equal(t, fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), info.Platform)
}

func TestHandlePostControl_Shutdown(t *testing.T) {
	c, shutdownCalled := newTestController(t)

	body := strings.NewReader(`{"command":"shutdown"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/control", body)
	rec := httptest.NewRecorder()

	c.handlePostControl(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, shutdownCalled.Load(), "expected backend shutdown callback to be invoked")
}

func TestHandlePostControl_UnknownCommand(t *testing.T) {
	c, _ := newTestController(t)

	body := strings.NewReader(`{"command":"reboot"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/control", body)
	rec := httptest.NewRecorder()

	c.handlePostControl(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var errResp proto.Error
	err := json.NewDecoder(rec.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "unknown command", errResp.Message)
}

func TestHandlePostControl_InvalidJSON(t *testing.T) {
	c, _ := newTestController(t)

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/v1/control", body)
	rec := httptest.NewRecorder()

	c.handlePostControl(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp proto.Error
	err := json.NewDecoder(rec.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "failed to decode request", errResp.Message)
}

func TestHandleGetConfig(t *testing.T) {
	c, _ := newTestController(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	rec := httptest.NewRecorder()

	c.handleGetConfig(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	// The response should be valid JSON representing the ConfigStore.
	var raw json.RawMessage
	err := json.NewDecoder(rec.Body).Decode(&raw)
	require.NoError(t, err, "response body should be valid JSON")
	assert.True(t, len(raw) > 0, "response body should not be empty")
}

// ---------------------------------------------------------------------------
// JSON helper tests
// ---------------------------------------------------------------------------

func TestJsonError(t *testing.T) {
	rec := httptest.NewRecorder()

	jsonError(rec, http.StatusNotFound, "workspace not found")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var errResp proto.Error
	err := json.NewDecoder(rec.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "workspace not found", errResp.Message)
}

func TestJsonError_DifferentCodes(t *testing.T) {
	tests := []struct {
		status  int
		message string
	}{
		{http.StatusBadRequest, "bad request"},
		{http.StatusInternalServerError, "internal error"},
		{http.StatusUnauthorized, "unauthorized"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			jsonError(rec, tt.status, tt.message)

			assert.Equal(t, tt.status, rec.Code)

			var errResp proto.Error
			err := json.NewDecoder(rec.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, tt.message, errResp.Message)
		})
	}
}

func TestJsonEncode(t *testing.T) {
	rec := httptest.NewRecorder()

	payload := map[string]string{"hello": "world"}
	jsonEncode(rec, payload)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var result map[string]string
	err := json.NewDecoder(rec.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "world", result["hello"])
}

func TestJsonEncode_Struct(t *testing.T) {
	rec := httptest.NewRecorder()

	info := proto.VersionInfo{
		Version:   "1.2.3",
		Commit:    "abc123",
		GoVersion: "go1.22",
		Platform:  "linux/amd64",
	}
	jsonEncode(rec, info)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var decoded proto.VersionInfo
	err := json.NewDecoder(rec.Body).Decode(&decoded)
	require.NoError(t, err)
	assert.Equal(t, info, decoded)
}

func TestJsonEncode_Nil(t *testing.T) {
	rec := httptest.NewRecorder()

	jsonEncode(rec, nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "null\n", rec.Body.String())
}
