package hyper

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ioReadAllLimit
// ---------------------------------------------------------------------------

func TestIOReadAllLimit_ReadsAllWhenUnderLimit(t *testing.T) {
	data := "hello world"
	b, err := ioReadAllLimit(strings.NewReader(data), 1024)
	require.NoError(t, err)
	assert.Equal(t, data, string(b))
}

func TestIOReadAllLimit_TruncatesAtLimit(t *testing.T) {
	data := "hello world"
	b, err := ioReadAllLimit(strings.NewReader(data), 5)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(b))
	assert.Len(t, b, 5)
}

func TestIOReadAllLimit_ExactLimit(t *testing.T) {
	data := "abcdef"
	b, err := ioReadAllLimit(strings.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, data, string(b))
}

func TestIOReadAllLimit_EmptyReader(t *testing.T) {
	b, err := ioReadAllLimit(strings.NewReader(""), 1024)
	require.NoError(t, err)
	assert.Empty(t, b)
}

func TestIOReadAllLimit_ZeroLimitDefaultsTo1MB(t *testing.T) {
	data := "short"
	b, err := ioReadAllLimit(strings.NewReader(data), 0)
	require.NoError(t, err)
	assert.Equal(t, data, string(b))
}

func TestIOReadAllLimit_NegativeLimitDefaultsTo1MB(t *testing.T) {
	data := "short"
	b, err := ioReadAllLimit(strings.NewReader(data), -10)
	require.NoError(t, err)
	assert.Equal(t, data, string(b))
}

func TestIOReadAllLimit_LargeDataTruncated(t *testing.T) {
	data := strings.Repeat("x", 10000)
	b, err := ioReadAllLimit(strings.NewReader(data), 100)
	require.NoError(t, err)
	assert.Len(t, b, 100)
}

func TestIOReadAllLimit_PropagatesReaderError(t *testing.T) {
	r := &errReader{err: io.ErrUnexpectedEOF}
	_, err := ioReadAllLimit(r, 1024)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

// ---------------------------------------------------------------------------
// retryAfter
// ---------------------------------------------------------------------------

func TestRetryAfter_ValidSeconds(t *testing.T) {
	tests := []struct {
		name     string
		seconds  string
		contains string
	}{
		{"1 second", "1", "1s"},
		{"30 seconds", "30", "30s"},
		{"120 seconds", "120", "2m0s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			resp.Header.Set("Retry-After", tc.seconds)
			msg := retryAfter(resp)
			assert.Contains(t, msg, "Try again in")
			assert.Contains(t, msg, tc.contains)
		})
	}
}

func TestRetryAfter_MissingHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	msg := retryAfter(resp)
	assert.Equal(t, "Try again later", msg)
}

func TestRetryAfter_NonNumericHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "Wed, 21 Oct 2025 07:28:00 GMT")
	msg := retryAfter(resp)
	assert.Equal(t, "Try again later", msg)
}

func TestRetryAfter_ZeroSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "0")
	msg := retryAfter(resp)
	assert.Equal(t, "Try again later", msg)
}

func TestRetryAfter_NegativeSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "-5")
	msg := retryAfter(resp)
	assert.Equal(t, "Try again later", msg)
}

func TestRetryAfter_EmptyHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "")
	msg := retryAfter(resp)
	assert.Equal(t, "Try again later", msg)
}

// ---------------------------------------------------------------------------
// toProviderError
// ---------------------------------------------------------------------------

func TestToProviderError_429(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
	}
	err := toProviderError(resp, "slow down")
	require.Error(t, err)

	var provErr *fantasy.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, http.StatusTooManyRequests, provErr.StatusCode)
	assert.Equal(t, "slow down", provErr.Message)
	assert.Equal(t, "too many requests", provErr.Title)
}

func TestToProviderError_401(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{},
	}
	err := toProviderError(resp, "bad token")
	require.Error(t, err)

	var provErr *fantasy.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, http.StatusUnauthorized, provErr.StatusCode)
	assert.Equal(t, "bad token", provErr.Message)
	assert.Equal(t, "unauthorized", provErr.Title)
}

func TestToProviderError_500(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{},
	}
	err := toProviderError(resp, "")
	require.Error(t, err)

	var provErr *fantasy.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, http.StatusInternalServerError, provErr.StatusCode)
	assert.Empty(t, provErr.Message)
	assert.Equal(t, "internal server error", provErr.Title)
}

func TestToProviderError_EmptyMessage(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
	}
	err := toProviderError(resp, "")
	require.Error(t, err)

	var provErr *fantasy.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "service unavailable", provErr.Title)
	assert.Empty(t, provErr.Message)
}

// ---------------------------------------------------------------------------
// Option builders
// ---------------------------------------------------------------------------

func TestWithBaseURL(t *testing.T) {
	o := &options{}
	WithBaseURL("http://localhost:9999")(o)
	assert.Equal(t, "http://localhost:9999", o.baseURL)
}

func TestWithName(t *testing.T) {
	o := &options{}
	WithName("custom-name")(o)
	assert.Equal(t, "custom-name", o.name)
}

func TestWithHTTPClient(t *testing.T) {
	c := &http.Client{}
	o := &options{}
	WithHTTPClient(c)(o)
	assert.Same(t, c, o.client)
}

func TestWithAPIKey(t *testing.T) {
	o := &options{}
	WithAPIKey("sk-secret")(o)
	assert.Equal(t, "sk-secret", o.apiKey)
}

func TestWithHeaders_MergesIntoExisting(t *testing.T) {
	o := &options{
		headers: map[string]string{"existing": "value"},
	}
	WithHeaders(map[string]string{"new-key": "new-val", "another": "one"})(o)
	assert.Equal(t, "value", o.headers["existing"])
	assert.Equal(t, "new-val", o.headers["new-key"])
	assert.Equal(t, "one", o.headers["another"])
}

func TestWithHeaders_OverridesExistingKey(t *testing.T) {
	o := &options{
		headers: map[string]string{"key": "old"},
	}
	WithHeaders(map[string]string{"key": "new"})(o)
	assert.Equal(t, "new", o.headers["key"])
}

func TestWithHeaders_EmptyMap(t *testing.T) {
	o := &options{
		headers: map[string]string{"key": "val"},
	}
	WithHeaders(map[string]string{})(o)
	assert.Equal(t, "val", o.headers["key"])
}

// ---------------------------------------------------------------------------
// provider.Name() and LanguageModel
// ---------------------------------------------------------------------------

func TestProviderName(t *testing.T) {
	p := &provider{options: options{name: "test-provider"}}
	assert.Equal(t, "test-provider", p.Name())
}

func TestLanguageModel_EmptyModelID(t *testing.T) {
	p := &provider{options: options{name: "test"}}
	_, err := p.LanguageModel(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing model id")
}

func TestLanguageModel_ValidModelID(t *testing.T) {
	p := &provider{options: options{name: "test-prov"}}
	lm, err := p.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)
	require.NotNil(t, lm)
	assert.Equal(t, "gpt-4", lm.Model())
	assert.Equal(t, "test-prov", lm.Provider())
}

// ---------------------------------------------------------------------------
// Constants and package-level values
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	assert.Equal(t, "hyper", Name)
	assert.Equal(t, "https://hyper.charm.land", defaultBaseURL)
}

func TestErrNoCredits(t *testing.T) {
	assert.EqualError(t, ErrNoCredits, "you're out of credits")
}

// ---------------------------------------------------------------------------
// doRequest builds correct URLs
// ---------------------------------------------------------------------------

func TestDoRequest_StreamURL(t *testing.T) {
	// Use a custom round-tripper to capture the outgoing request.
	var captured *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     http.Header{},
			}, nil
		}),
	}

	m := &languageModel{
		modelID:  "claude-3",
		provider: "test",
		opts: options{
			baseURL: "http://localhost:8080/api/v1/fantasy",
			client:  client,
			headers: map[string]string{"x-custom": "yes"},
			apiKey:  "my-key",
		},
	}

	resp, err := m.doRequest(context.Background(), true, fantasy.Call{})
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, captured)
	assert.Equal(t, "http://localhost:8080/api/v1/fantasy/claude-3/stream", captured.URL.String())
	assert.Equal(t, "text/event-stream", captured.Header.Get("Accept"))
	assert.Equal(t, "application/json", captured.Header.Get("Content-Type"))
	assert.Equal(t, "yes", captured.Header.Get("x-custom"))
	assert.Equal(t, "my-key", captured.Header.Get("Authorization"))
}

func TestDoRequest_GenerateURL(t *testing.T) {
	var captured *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     http.Header{},
			}, nil
		}),
	}

	m := &languageModel{
		modelID:  "gpt-4",
		provider: "test",
		opts: options{
			baseURL: "https://example.com/api",
			client:  client,
			headers: map[string]string{},
		},
	}

	resp, err := m.doRequest(context.Background(), false, fantasy.Call{})
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, captured)
	assert.Equal(t, "https://example.com/api/gpt-4/generate", captured.URL.String())
	assert.Equal(t, "application/json", captured.Header.Get("Accept"))
	assert.Empty(t, captured.Header.Get("Authorization"))
}

func TestDoRequest_NoAPIKeyOmitsAuthHeader(t *testing.T) {
	var captured *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     http.Header{},
			}, nil
		}),
	}

	m := &languageModel{
		modelID:  "model",
		provider: "p",
		opts: options{
			baseURL: "http://localhost/api",
			client:  client,
			headers: map[string]string{},
			apiKey:  "",
		},
	}

	resp, err := m.doRequest(context.Background(), false, fantasy.Call{})
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, captured)
	assert.Empty(t, captured.Header.Get("Authorization"))
}

func TestDoRequest_InvalidBaseURL(t *testing.T) {
	m := &languageModel{
		modelID: "model",
		opts: options{
			baseURL: "://bad-url",
			client:  &http.Client{},
			headers: map[string]string{},
		},
	}
	_, err := m.doRequest(context.Background(), false, fantasy.Call{})
	require.Error(t, err)
}

// roundTripFunc is an adapter to use a function as http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
