package copilot

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedRequest is a helper to inspect the request that arrives at the
// downstream transport after initiatorTransport has decorated it.
type capturedRequest struct {
	header http.Header
	body   []byte
}

func newCapturingServer(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.header = r.Header.Clone()
		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				captured.body = bodyBytes
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func TestInitiatorTransport_UserInitiator(t *testing.T) {
	var cap capturedRequest
	srv := newCapturingServer(t, &cap)
	defer srv.Close()

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewBufferString(body))
	require.NoError(t, err)

	transport := &initiatorTransport{debug: false, isSubAgent: false}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "user", cap.header.Get("X-Initiator"))
}

func TestInitiatorTransport_AgentInitiator(t *testing.T) {
	var cap capturedRequest
	srv := newCapturingServer(t, &cap)
	defer srv.Close()

	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewBufferString(body))
	require.NoError(t, err)

	transport := &initiatorTransport{debug: false, isSubAgent: false}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "agent", cap.header.Get("X-Initiator"))
}

func TestInitiatorTransport_SubAgent(t *testing.T) {
	var cap capturedRequest
	srv := newCapturingServer(t, &cap)
	defer srv.Close()

	// Body has no assistant role, but isSubAgent is true → should be "agent".
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewBufferString(body))
	require.NoError(t, err)

	transport := &initiatorTransport{debug: false, isSubAgent: true}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "agent", cap.header.Get("X-Initiator"))
}

func TestInitiatorTransport_NoBody(t *testing.T) {
	var cap capturedRequest
	srv := newCapturingServer(t, &cap)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, http.NoBody)
	require.NoError(t, err)

	transport := &initiatorTransport{debug: false, isSubAgent: false}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "user", cap.header.Get("X-Initiator"))
}

func TestInitiatorTransport_NilRequest(t *testing.T) {
	transport := &initiatorTransport{debug: false, isSubAgent: false}
	resp, err := transport.RoundTrip(nil)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// Verify that the body is still readable after the transport inspects it.
func TestInitiatorTransport_BodyPreserved(t *testing.T) {
	var cap capturedRequest
	srv := newCapturingServer(t, &cap)
	defer srv.Close()

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewBufferString(body))
	require.NoError(t, err)

	transport := &initiatorTransport{debug: false, isSubAgent: false}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// The server should have received the full body.
	assert.JSONEq(t, body, string(cap.body))
}
