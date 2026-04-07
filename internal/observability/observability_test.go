package observability

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"

	"github.com/stretchr/testify/require"
)

func TestContextAttrsIncludeContextFieldsAndTrace(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-123")
	ctx = WithWorkspaceID(ctx, "ws-123")
	ctx = WithSessionID(ctx, "sess-123")
	ctx = WithMessageID(ctx, "msg-123")
	ctx = WithAgent(ctx, "coder")
	ctx = WithTool(ctx, "bash", "tool-123")
	ctx = WithLSP(ctx, "gopls")
	ctx = WithComponent(ctx, "test")

	ctx, span := StartSpan(ctx, "test-span")
	span.End()

	attrs := ContextAttrs(ctx)
	require.NotEmpty(t, attrs)

	got := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		got[attr.Key] = attr.Value.String()
	}

	require.Equal(t, "req-123", got["request_id"])
	require.Equal(t, "ws-123", got["workspace_id"])
	require.Equal(t, "sess-123", got["session_id"])
	require.Equal(t, "msg-123", got["message_id"])
	require.Equal(t, "coder", got["agent"])
	require.Equal(t, "bash", got["tool"])
	require.Equal(t, "tool-123", got["tool_call_id"])
	require.Equal(t, "gopls", got["lsp"])
	require.Equal(t, "test", got["component"])
	require.NotEmpty(t, got["trace_id"])
	require.NotEmpty(t, got["span_id"])
}

func TestRecentSpansUsesTraceBufferLimit(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  1,
		TraceSampleRatio: 1,
	}))

	ctx := context.Background()
	_, span1 := StartSpan(ctx, "first")
	span1.End()
	_, span2 := StartSpan(ctx, "second")
	span2.End()

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "second", spans[0].Name)
}

func TestRecentSpansRedactSensitiveAttributes(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	ctx := context.Background()
	_, span := StartSpan(ctx, "redaction-test",
		attribute.String("authorization", "Bearer super-secret"),
		attribute.String("http.url", "https://example.com/search?token=super-secret&q=trace"),
		attribute.String("session_id", "sess-123"),
	)
	span.End()

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, redactedValue, spans[0].Attributes["authorization"])
	require.Equal(t, "sess-123", spans[0].Attributes["session_id"])
	require.NotContains(t, fmt.Sprint(spans[0].Attributes["http.url"]), "super-secret")
	require.Contains(t, fmt.Sprint(spans[0].Attributes["http.url"]), url.QueryEscape(redactedValue))
}

func TestInstrumentedRoundTripperPropagatesRequestIDAndRedactsURL(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	var receivedRequestID string
	server := httptest.NewServer(HTTPServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequestID = r.Header.Get("X-Request-ID")
		require.NotEmpty(t, receivedRequestID)
		w.WriteHeader(http.StatusNoContent)
	})))
	defer server.Close()

	client := &http.Client{
		Transport: &InstrumentedRoundTripper{
			Transport: http.DefaultTransport,
			Component: "test_client",
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/events?token=top-secret&ok=1", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, receivedRequestID, resp.Header.Get("X-Request-ID"))
	require.NotEmpty(t, resp.Header.Get("X-Trace-ID"))

	var clientSpan *spanRecord
	spans := RecentSpans(10)
	for i := range spans {
		if spans[i].Name == "http.client" {
			clientSpan = &spans[i]
			break
		}
	}
	require.NotNil(t, clientSpan)
	require.Equal(t, receivedRequestID, clientSpan.Attributes["http.request_id"])
	require.NotContains(t, fmt.Sprint(clientSpan.Attributes["http.url"]), "top-secret")
	require.Contains(t, fmt.Sprint(clientSpan.Attributes["http.url"]), url.QueryEscape(redactedValue))
}

func TestRecordWorkItemPromotionTracksCountersAndDuration(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	RecordWorkItemPromotion("github_issues", "success", 250*time.Millisecond)
	RecordWorkItemPromotion("github_issues", "warning", 125*time.Millisecond)

	st := getState()
	require.NotNil(t, st)

	successMetric := gatherMetric(t, st, "codeplane_work_item_promotions_total", map[string]string{
		"target": "github_issues",
		"result": "success",
	})
	require.Equal(t, float64(1), successMetric.GetCounter().GetValue())

	warningMetric := gatherMetric(t, st, "codeplane_work_item_promotions_total", map[string]string{
		"target": "github_issues",
		"result": "warning",
	})
	require.Equal(t, float64(1), warningMetric.GetCounter().GetValue())

	metric := gatherMetric(t, st, "codeplane_work_item_promotion_duration_seconds", map[string]string{
		"target": "github_issues",
		"result": "success",
	})
	require.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
	require.InDelta(t, 0.25, metric.GetHistogram().GetSampleSum(), 0.0001)
}

func TestRecordUINavigation_CapturesSpanAndMetric(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	RecordUINavigation("runs", "snapshots", "ok",
		attribute.String("codeplane.run_id", "run-123"),
	)

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "ui.navigation", spans[0].Name)
	require.Equal(t, "runs", spans[0].Attributes["codeplane.ui.entrypoint"])
	require.Equal(t, "snapshots", spans[0].Attributes["codeplane.ui.target"])
	require.Equal(t, "ok", spans[0].Attributes["codeplane.ui.result"])
	require.Equal(t, "run-123", spans[0].Attributes["codeplane.run_id"])

	st := getState()
	require.NotNil(t, st)
	counter := gatherMetric(t, st, "codeplane_ui_navigation_total", map[string]string{
		"entrypoint": "runs",
		"target":     "snapshots",
		"result":     "ok",
	})
	require.Equal(t, float64(1), counter.GetCounter().GetValue())
}

func TestRecordStartupFlow_CapturesSpanAndMetric(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	RecordStartupFlow("config_source", "global_config", "legacy",
		attribute.String("codeplane.config.path", "/tmp/crush/crush.json"),
	)

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "codeplane.startup.config_source", spans[0].Name)
	require.Equal(t, "config_source", spans[0].Attributes["codeplane.startup.flow"])
	require.Equal(t, "global_config", spans[0].Attributes["codeplane.startup.source"])
	require.Equal(t, "legacy", spans[0].Attributes["codeplane.startup.result"])
	require.Equal(t, "/tmp/crush/crush.json", spans[0].Attributes["codeplane.config.path"])

	st := getState()
	require.NotNil(t, st)
	counter := gatherMetric(t, st, "codeplane_startup_flows_total", map[string]string{
		"flow":   "config_source",
		"source": "global_config",
		"result": "legacy",
	})
	require.Equal(t, float64(1), counter.GetCounter().GetValue())
}

func TestRecordSnapshotOperation_CapturesSpanAndMetric(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, Shutdown(context.Background()))
	})

	require.NoError(t, Configure(context.Background(), Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	RecordSnapshotOperation("load", 250*time.Millisecond, nil,
		attribute.String("codeplane.run_id", "run-123"),
		attribute.Int("codeplane.snapshot.count", 3),
	)

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "ui.snapshots.load", spans[0].Name)
	require.Equal(t, "load", spans[0].Attributes["codeplane.snapshot.operation"])
	require.Equal(t, "ok", spans[0].Attributes["codeplane.snapshot.result"])
	require.Equal(t, "run-123", spans[0].Attributes["codeplane.run_id"])
	require.EqualValues(t, 3, spans[0].Attributes["codeplane.snapshot.count"])

	st := getState()
	require.NotNil(t, st)

	counter := gatherMetric(t, st, "codeplane_snapshot_operations_total", map[string]string{
		"operation": "load",
		"result":    "ok",
	})
	require.Equal(t, float64(1), counter.GetCounter().GetValue())

	metric := gatherMetric(t, st, "codeplane_snapshot_operation_duration_seconds", map[string]string{
		"operation": "load",
	})
	require.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
	require.InDelta(t, 0.25, metric.GetHistogram().GetSampleSum(), 0.0001)
}

func TestNormalizeConfigAppliesDefaults(t *testing.T) {
	tests := []struct {
		name   string
		input  Config
		assert func(t *testing.T, cfg Config)
	}{
		{
			name:  "empty service name defaults to codeplane",
			input: Config{},
			assert: func(t *testing.T, cfg Config) {
				require.Equal(t, "codeplane", cfg.ServiceName)
			},
		},
		{
			name:  "whitespace-only service name defaults to codeplane",
			input: Config{ServiceName: "   "},
			assert: func(t *testing.T, cfg Config) {
				require.Equal(t, "codeplane", cfg.ServiceName)
			},
		},
		{
			name:  "explicit service name preserved",
			input: Config{ServiceName: "myapp"},
			assert: func(t *testing.T, cfg Config) {
				require.Equal(t, "myapp", cfg.ServiceName)
			},
		},
		{
			name:  "zero trace buffer defaults to 512",
			input: Config{},
			assert: func(t *testing.T, cfg Config) {
				require.Equal(t, 512, cfg.TraceBufferSize)
			},
		},
		{
			name:  "negative trace buffer defaults to 512",
			input: Config{TraceBufferSize: -10},
			assert: func(t *testing.T, cfg Config) {
				require.Equal(t, 512, cfg.TraceBufferSize)
			},
		},
		{
			name:  "zero sample ratio defaults to 1",
			input: Config{},
			assert: func(t *testing.T, cfg Config) {
				require.InDelta(t, 1.0, cfg.TraceSampleRatio, 0.001)
			},
		},
		{
			name:  "negative sample ratio clamped to 1",
			input: Config{TraceSampleRatio: -0.5},
			assert: func(t *testing.T, cfg Config) {
				require.InDelta(t, 1.0, cfg.TraceSampleRatio, 0.001)
			},
		},
		{
			name:  "sample ratio above 1 clamped to 1",
			input: Config{TraceSampleRatio: 2.5},
			assert: func(t *testing.T, cfg Config) {
				require.InDelta(t, 1.0, cfg.TraceSampleRatio, 0.001)
			},
		},
		{
			name:  "valid sample ratio preserved",
			input: Config{TraceSampleRatio: 0.5},
			assert: func(t *testing.T, cfg Config) {
				require.InDelta(t, 0.5, cfg.TraceSampleRatio, 0.001)
			},
		},
		{
			name:  "nil OTLP headers initialized to empty map",
			input: Config{},
			assert: func(t *testing.T, cfg Config) {
				require.NotNil(t, cfg.OTLPHeaders)
				require.Empty(t, cfg.OTLPHeaders)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := normalizeConfig(tt.input)
			tt.assert(t, cfg)
		})
	}
}

func TestRedactHeaders(t *testing.T) {
	headers := http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Bearer secret-token"},
		"X-Api-Key":     []string{"key-123"},
		"Accept":        []string{"text/html"},
	}

	redacted := RedactHeaders(headers)
	require.Equal(t, []string{"application/json"}, redacted["Content-Type"])
	require.Equal(t, []string{redactedValue}, redacted["Authorization"])
	require.Equal(t, []string{redactedValue}, redacted["X-Api-Key"])
	require.Equal(t, []string{"text/html"}, redacted["Accept"])
}

func TestRedactHeadersNilReturnsNil(t *testing.T) {
	require.Nil(t, RedactHeaders(nil))
	require.Nil(t, RedactHeaders(http.Header{}))
}

func TestRedactStringMap(t *testing.T) {
	input := map[string]string{
		"host":     "example.com",
		"password": "supersecret",
		"token":    "tok-abc",
		"mode":     "debug",
	}

	redacted := RedactStringMap(input)
	require.Equal(t, "example.com", redacted["host"])
	require.Equal(t, redactedValue, redacted["password"])
	require.Equal(t, redactedValue, redacted["token"])
	require.Equal(t, "debug", redacted["mode"])
}

func TestRedactStringMapNilReturnsNil(t *testing.T) {
	require.Nil(t, RedactStringMap(nil))
	require.Nil(t, RedactStringMap(map[string]string{}))
}

func TestRedactURLStringScrubsSensitiveQueryParams(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, result string)
	}{
		{
			name:  "empty string",
			input: "",
			check: func(t *testing.T, result string) {
				require.Empty(t, result)
			},
		},
		{
			name:  "no query params unchanged",
			input: "https://example.com/path",
			check: func(t *testing.T, result string) {
				require.Contains(t, result, "example.com/path")
			},
		},
		{
			name:  "token param redacted",
			input: "https://example.com/api?token=secret123&page=1",
			check: func(t *testing.T, result string) {
				require.NotContains(t, result, "secret123")
				require.Contains(t, result, "page=1")
				require.Contains(t, result, url.QueryEscape(redactedValue))
			},
		},
		{
			name:  "api_key param redacted",
			input: "https://example.com/data?api_key=key-abc&format=json",
			check: func(t *testing.T, result string) {
				require.NotContains(t, result, "key-abc")
				require.Contains(t, result, "format=json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactURLString(tt.input)
			tt.check(t, result)
		})
	}
}

func TestIsLoopbackAddress(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"localhost", true},
		{"localhost:8080", true},
		{"127.0.0.1", true},
		{"127.0.0.1:9090", true},
		{"[::1]:8080", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.1:8080", false},
		{"example.com:443", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			require.Equal(t, tt.want, isLoopbackAddress(tt.addr))
		})
	}
}

func gatherMetric(t *testing.T, st *state, name string, labels map[string]string) *dto.Metric {
	t.Helper()

	families, err := st.metrics.registry.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabelsMatch(metric.GetLabel(), labels) {
				return metric
			}
		}
	}

	t.Fatalf("metric %s with labels %v not found", name, labels)
	return nil
}

func metricLabelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(want) == 0 {
		return len(pairs) == 0
	}

	got := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		got[pair.GetName()] = pair.GetValue()
	}
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
