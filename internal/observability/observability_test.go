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

	successMetric := gatherMetric(t, st, "crush_work_item_promotions_total", map[string]string{
		"target": "github_issues",
		"result": "success",
	})
	require.Equal(t, float64(1), successMetric.GetCounter().GetValue())

	warningMetric := gatherMetric(t, st, "crush_work_item_promotions_total", map[string]string{
		"target": "github_issues",
		"result": "warning",
	})
	require.Equal(t, float64(1), warningMetric.GetCounter().GetValue())

	metric := gatherMetric(t, st, "crush_work_item_promotion_duration_seconds", map[string]string{
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
		attribute.String("crush.run_id", "run-123"),
	)

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "ui.navigation", spans[0].Name)
	require.Equal(t, "runs", spans[0].Attributes["crush.ui.entrypoint"])
	require.Equal(t, "snapshots", spans[0].Attributes["crush.ui.target"])
	require.Equal(t, "ok", spans[0].Attributes["crush.ui.result"])
	require.Equal(t, "run-123", spans[0].Attributes["crush.run_id"])

	st := getState()
	require.NotNil(t, st)
	counter := gatherMetric(t, st, "crush_ui_navigation_total", map[string]string{
		"entrypoint": "runs",
		"target":     "snapshots",
		"result":     "ok",
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
		attribute.String("crush.run_id", "run-123"),
		attribute.Int("crush.snapshot.count", 3),
	)

	spans := RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "ui.snapshots.load", spans[0].Name)
	require.Equal(t, "load", spans[0].Attributes["crush.snapshot.operation"])
	require.Equal(t, "ok", spans[0].Attributes["crush.snapshot.result"])
	require.Equal(t, "run-123", spans[0].Attributes["crush.run_id"])
	require.EqualValues(t, 3, spans[0].Attributes["crush.snapshot.count"])

	st := getState()
	require.NotNil(t, st)

	counter := gatherMetric(t, st, "crush_snapshot_operations_total", map[string]string{
		"operation": "load",
		"result":    "ok",
	})
	require.Equal(t, float64(1), counter.GetCounter().GetValue())

	metric := gatherMetric(t, st, "crush_snapshot_operation_duration_seconds", map[string]string{
		"operation": "load",
	})
	require.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
	require.InDelta(t, 0.25, metric.GetHistogram().GetSampleSum(), 0.0001)
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
