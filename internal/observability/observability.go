package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/charmbracelet/codeplane"

const redactedValue = "[REDACTED]"

type Mode string

const (
	ModeLocal  Mode = "local"
	ModeServer Mode = "server"
	ModeClient Mode = "client"
)

type Config struct {
	ServiceName      string
	ServiceVersion   string
	Mode             Mode
	DebugServerAddr  string
	EnableHTTPServer bool
	TraceBufferSize  int
	TraceSampleRatio float64
	OTLPEndpoint     string
	OTLPHeaders      map[string]string
	OTLPInsecure     bool
}

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	workspaceIDKey contextKey = "workspace_id"
	sessionIDKey   contextKey = "session_id"
	messageIDKey   contextKey = "message_id"
	agentKey       contextKey = "agent"
	toolKey        contextKey = "tool"
	toolCallIDKey  contextKey = "tool_call_id"
	lspKey         contextKey = "lsp"
	componentKey   contextKey = "component"
)

type spanEventRecord struct {
	Name       string         `json:"name"`
	Time       time.Time      `json:"time"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type spanRecord struct {
	Name              string            `json:"name"`
	TraceID           string            `json:"trace_id"`
	SpanID            string            `json:"span_id"`
	ParentSpanID      string            `json:"parent_span_id,omitempty"`
	Kind              string            `json:"kind"`
	StartTime         time.Time         `json:"start_time"`
	EndTime           time.Time         `json:"end_time"`
	DurationMillis    int64             `json:"duration_millis"`
	StatusCode        string            `json:"status_code"`
	StatusDescription string            `json:"status_description,omitempty"`
	Attributes        map[string]any    `json:"attributes,omitempty"`
	Events            []spanEventRecord `json:"events,omitempty"`
}

type memoryExporter struct {
	mu    sync.RWMutex
	limit int
	spans []spanRecord
}

func newMemoryExporter(limit int) *memoryExporter {
	if limit <= 0 {
		limit = 512
	}
	return &memoryExporter{limit: limit}
}

func (e *memoryExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	records := make([]spanRecord, 0, len(spans))
	for _, span := range spans {
		record := spanRecord{
			Name:              span.Name(),
			TraceID:           span.SpanContext().TraceID().String(),
			SpanID:            span.SpanContext().SpanID().String(),
			ParentSpanID:      span.Parent().SpanID().String(),
			Kind:              span.SpanKind().String(),
			StartTime:         span.StartTime(),
			EndTime:           span.EndTime(),
			DurationMillis:    span.EndTime().Sub(span.StartTime()).Milliseconds(),
			StatusCode:        span.Status().Code.String(),
			StatusDescription: span.Status().Description,
			Attributes:        attributesToMap(span.Attributes()),
			Events:            spanEventsToRecords(span.Events()),
		}
		records = append(records, record)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.spans = append(e.spans, records...)
	if extra := len(e.spans) - e.limit; extra > 0 {
		e.spans = append([]spanRecord(nil), e.spans[extra:]...)
	}
	return nil
}

func (e *memoryExporter) Shutdown(context.Context) error { return nil }

func (e *memoryExporter) ForceFlush(context.Context) error { return nil }

func (e *memoryExporter) Snapshot(limit int) []spanRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 || limit > len(e.spans) {
		limit = len(e.spans)
	}
	start := len(e.spans) - limit
	out := make([]spanRecord, limit)
	copy(out, e.spans[start:])
	return out
}

type metrics struct {
	registry *prometheus.Registry

	appStartTotal              prometheus.Counter
	appShutdownTotal           *prometheus.CounterVec
	agentRunsTotal             *prometheus.CounterVec
	agentRunDuration           *prometheus.HistogramVec
	toolCallsTotal             *prometheus.CounterVec
	toolCallDuration           *prometheus.HistogramVec
	shellCommandsTotal         *prometheus.CounterVec
	shellDuration              *prometheus.HistogramVec
	lspOpsTotal                *prometheus.CounterVec
	lspOpDuration              *prometheus.HistogramVec
	sessionOpsTotal            *prometheus.CounterVec
	sessionOpDuration          *prometheus.HistogramVec
	messageOpsTotal            *prometheus.CounterVec
	messageOpDuration          *prometheus.HistogramVec
	permissionsTotal           *prometheus.CounterVec
	permissionDuration         *prometheus.HistogramVec
	httpServerTotal            *prometheus.CounterVec
	httpServerDuration         *prometheus.HistogramVec
	httpClientTotal            *prometheus.CounterVec
	httpClientDuration         *prometheus.HistogramVec
	dbOpsTotal                 *prometheus.CounterVec
	dbOpDuration               *prometheus.HistogramVec
	retriesTotal               *prometheus.CounterVec
	retryDelay                 *prometheus.HistogramVec
	workItemPromotionsTotal    *prometheus.CounterVec
	workItemPromotionDuration  *prometheus.HistogramVec
	pubsubEventsTotal          *prometheus.CounterVec
	pubsubSubscribers          *prometheus.GaugeVec
	sseEventsTotal             *prometheus.CounterVec
	sseConnections             *prometheus.GaugeVec
	sseDuration                *prometheus.HistogramVec
	backgroundJobsTotal        *prometheus.CounterVec
	backgroundJobDuration      *prometheus.HistogramVec
	backgroundTrackedJobs      prometheus.Gauge
	permissionBacklog          prometheus.Gauge
	permissionActive           prometheus.Gauge
	permissionQueueDelay       *prometheus.HistogramVec
	workspaceLifecycleTotal    *prometheus.CounterVec
	workspaceLifecycleDuration *prometheus.HistogramVec
	droppedEventsTotal         *prometheus.CounterVec
	startupFlowsTotal          *prometheus.CounterVec
	uiNavigationTotal          *prometheus.CounterVec
	snapshotOpsTotal           *prometheus.CounterVec
	snapshotOpDuration         *prometheus.HistogramVec
	activeAgentRuns            prometheus.Gauge
	backgroundJobs             prometheus.Gauge
	activeWorkspaces           prometheus.Gauge
}

func newMetrics() *metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &metrics{
		registry: registry,

		appStartTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "codeplane_app_starts_total",
			Help: "Total number of Codeplane process starts.",
		}),
		appShutdownTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_app_shutdowns_total",
			Help: "Total number of Codeplane process shutdowns by result.",
		}, []string{"result"}),
		agentRunsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_agent_runs_total",
			Help: "Total number of agent runs by outcome.",
		}, []string{"agent", "provider", "model", "result"}),
		agentRunDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_agent_run_duration_seconds",
			Help:    "Duration of agent runs.",
			Buckets: prometheus.DefBuckets,
		}, []string{"agent", "provider", "model"}),
		toolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_tool_calls_total",
			Help: "Total number of tool calls by outcome.",
		}, []string{"tool", "result"}),
		toolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_tool_call_duration_seconds",
			Help:    "Duration of tool calls.",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool"}),
		shellCommandsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_shell_commands_total",
			Help: "Total number of shell commands by outcome.",
		}, []string{"result"}),
		shellDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_shell_command_duration_seconds",
			Help:    "Duration of shell commands.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
		lspOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_lsp_operations_total",
			Help: "Total number of LSP operations by outcome.",
		}, []string{"lsp", "operation", "result"}),
		lspOpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_lsp_operation_duration_seconds",
			Help:    "Duration of LSP operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"lsp", "operation"}),
		sessionOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_session_operations_total",
			Help: "Total number of session operations by outcome.",
		}, []string{"operation", "result"}),
		sessionOpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_session_operation_duration_seconds",
			Help:    "Duration of session operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		messageOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_message_operations_total",
			Help: "Total number of message operations by outcome.",
		}, []string{"operation", "role", "result"}),
		messageOpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_message_operation_duration_seconds",
			Help:    "Duration of message operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation", "role"}),
		permissionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_permission_requests_total",
			Help: "Total number of permission requests by outcome.",
		}, []string{"tool", "action", "result"}),
		permissionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_permission_request_duration_seconds",
			Help:    "Duration of permission requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool", "action"}),
		httpServerTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_http_server_requests_total",
			Help: "Total number of HTTP server requests by status.",
		}, []string{"method", "route", "status"}),
		httpServerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_http_server_request_duration_seconds",
			Help:    "Duration of HTTP server requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		httpClientTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_http_client_requests_total",
			Help: "Total number of HTTP client requests by status.",
		}, []string{"component", "method", "host", "status"}),
		httpClientDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_http_client_request_duration_seconds",
			Help:    "Duration of HTTP client requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"component", "method", "host"}),
		dbOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_db_operations_total",
			Help: "Total number of database and migration operations by outcome.",
		}, []string{"operation", "result"}),
		dbOpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_db_operation_duration_seconds",
			Help:    "Duration of database and migration operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		retriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_retries_total",
			Help: "Total number of retry attempts by component and reason.",
		}, []string{"component", "provider", "reason"}),
		retryDelay: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_retry_delay_seconds",
			Help:    "Delay before retry attempts.",
			Buckets: prometheus.DefBuckets,
		}, []string{"component", "provider", "reason"}),
		workItemPromotionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_work_item_promotions_total",
			Help: "Total number of local work item promotions by target and outcome.",
		}, []string{"target", "result"}),
		workItemPromotionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_work_item_promotion_duration_seconds",
			Help:    "Duration of local work item promotions.",
			Buckets: prometheus.DefBuckets,
		}, []string{"target", "result"}),
		pubsubEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_pubsub_events_total",
			Help: "Pubsub events and fanout outcomes by broker.",
		}, []string{"broker", "event_type", "result"}),
		pubsubSubscribers: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "codeplane_pubsub_subscribers",
			Help: "Current number of pubsub subscribers by broker.",
		}, []string{"broker"}),
		sseEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_sse_events_total",
			Help: "Total number of SSE stream events and failures by stream.",
		}, []string{"stream", "result"}),
		sseConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "codeplane_sse_connections",
			Help: "Current number of active SSE connections by stream.",
		}, []string{"stream"}),
		sseDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_sse_connection_duration_seconds",
			Help:    "Duration of SSE connections.",
			Buckets: prometheus.DefBuckets,
		}, []string{"stream", "result"}),
		backgroundJobsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_background_jobs_total",
			Help: "Background shell job lifecycle events.",
		}, []string{"result"}),
		backgroundJobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_background_job_duration_seconds",
			Help:    "Duration of completed background shell jobs.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
		backgroundTrackedJobs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codeplane_background_jobs_tracked",
			Help: "Current number of background jobs tracked by the manager.",
		}),
		permissionBacklog: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codeplane_permission_backlog",
			Help: "Current number of pending permission requests awaiting resolution.",
		}),
		permissionActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codeplane_permission_active_requests",
			Help: "Current number of permission requests actively blocking execution.",
		}),
		permissionQueueDelay: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_permission_queue_delay_seconds",
			Help:    "Time spent waiting to become the active permission request.",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool", "action"}),
		workspaceLifecycleTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_workspace_lifecycle_total",
			Help: "Workspace lifecycle operations by outcome.",
		}, []string{"operation", "result"}),
		workspaceLifecycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_workspace_lifecycle_duration_seconds",
			Help:    "Duration of workspace lifecycle operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		droppedEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_dropped_events_total",
			Help: "Dropped asynchronous events by subsystem.",
		}, []string{"subsystem"}),
		startupFlowsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_startup_flows_total",
			Help: "Codeplane startup and command flow events by source and outcome.",
		}, []string{"flow", "source", "result"}),
		uiNavigationTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_ui_navigation_total",
			Help: "UI navigation events by entrypoint, target, and result.",
		}, []string{"entrypoint", "target", "result"}),
		snapshotOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codeplane_snapshot_operations_total",
			Help: "Snapshot UI operations by operation and result.",
		}, []string{"operation", "result"}),
		snapshotOpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codeplane_snapshot_operation_duration_seconds",
			Help:    "Duration of snapshot UI operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		activeAgentRuns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codeplane_active_agent_runs",
			Help: "Current number of active agent runs.",
		}),
		backgroundJobs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codeplane_background_jobs",
			Help: "Current number of background shell jobs.",
		}),
		activeWorkspaces: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codeplane_active_workspaces",
			Help: "Current number of active workspaces.",
		}),
	}

	registry.MustRegister(
		m.appStartTotal,
		m.appShutdownTotal,
		m.agentRunsTotal,
		m.agentRunDuration,
		m.toolCallsTotal,
		m.toolCallDuration,
		m.shellCommandsTotal,
		m.shellDuration,
		m.lspOpsTotal,
		m.lspOpDuration,
		m.sessionOpsTotal,
		m.sessionOpDuration,
		m.messageOpsTotal,
		m.messageOpDuration,
		m.permissionsTotal,
		m.permissionDuration,
		m.httpServerTotal,
		m.httpServerDuration,
		m.httpClientTotal,
		m.httpClientDuration,
		m.dbOpsTotal,
		m.dbOpDuration,
		m.retriesTotal,
		m.retryDelay,
		m.workItemPromotionsTotal,
		m.workItemPromotionDuration,
		m.pubsubEventsTotal,
		m.pubsubSubscribers,
		m.sseEventsTotal,
		m.sseConnections,
		m.sseDuration,
		m.backgroundJobsTotal,
		m.backgroundJobDuration,
		m.backgroundTrackedJobs,
		m.permissionBacklog,
		m.permissionActive,
		m.permissionQueueDelay,
		m.workspaceLifecycleTotal,
		m.workspaceLifecycleDuration,
		m.droppedEventsTotal,
		m.startupFlowsTotal,
		m.uiNavigationTotal,
		m.snapshotOpsTotal,
		m.snapshotOpDuration,
		m.activeAgentRuns,
		m.backgroundJobs,
		m.activeWorkspaces,
	)

	return m
}

type state struct {
	cfg         Config
	startedAt   time.Time
	metrics     *metrics
	exporter    *memoryExporter
	debugServer *http.Server
	tp          *sdktrace.TracerProvider
}

var (
	stateMu sync.RWMutex
	current *state

	expvarOnce sync.Once
)

func Configure(ctx context.Context, cfg Config) error {
	cfg = normalizeConfig(cfg)

	stateMu.Lock()
	defer stateMu.Unlock()

	if current != nil {
		return nil
	}

	metrics := newMetrics()
	exporter := newMemoryExporter(cfg.TraceBufferSize)

	resource := resource.NewSchemaless(
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("service.version", cfg.ServiceVersion),
		attribute.String("service.instance.id", processInstanceID()),
		attribute.String("codeplane.mode", string(cfg.Mode)),
	)

	tpOptions := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio))),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	}

	if cfg.OTLPEndpoint != "" {
		clientOptions := []otlptracehttp.Option{
			otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint),
			otlptracehttp.WithHeaders(cfg.OTLPHeaders),
		}
		if cfg.OTLPInsecure {
			clientOptions = append(clientOptions, otlptracehttp.WithInsecure())
		}
		otlpExporter, err := otlptracehttp.New(ctx, clientOptions...)
		if err != nil {
			return fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		tpOptions = append(tpOptions, sdktrace.WithBatcher(otlpExporter))
	}

	tp := sdktrace.NewTracerProvider(tpOptions...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	st := &state{
		cfg:       cfg,
		startedAt: time.Now(),
		metrics:   metrics,
		exporter:  exporter,
		tp:        tp,
	}

	if cfg.EnableHTTPServer && cfg.DebugServerAddr != "" {
		server, err := startDebugServer(st)
		if err != nil {
			_ = tp.Shutdown(ctx)
			return err
		}
		st.debugServer = server
	}

	expvarOnce.Do(func() {
		expvar.Publish("codeplane_observability", expvar.Func(func() any {
			return Snapshot()
		}))
	})

	current = st
	metrics.appStartTotal.Inc()
	slog.Info("Observability configured",
		"mode", cfg.Mode,
		"debug_server", cfg.DebugServerAddr,
		"otlp_enabled", cfg.OTLPEndpoint != "",
		"trace_buffer_size", cfg.TraceBufferSize,
		"trace_sample_ratio", cfg.TraceSampleRatio,
	)
	return nil
}

func Shutdown(ctx context.Context) error {
	stateMu.Lock()
	st := current
	current = nil
	stateMu.Unlock()

	if st == nil {
		return nil
	}

	st.metrics.appShutdownTotal.WithLabelValues("ok").Inc()

	var errs []error
	if st.debugServer != nil {
		if err := st.debugServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, err)
		}
	}
	if st.tp != nil {
		if err := st.tp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errors.Join(errs...)
	}
}

func Snapshot() map[string]any {
	stateMu.RLock()
	st := current
	stateMu.RUnlock()

	if st == nil {
		return map[string]any{
			"configured": false,
		}
	}

	return map[string]any{
		"configured":            true,
		"service_name":          st.cfg.ServiceName,
		"service_version":       st.cfg.ServiceVersion,
		"mode":                  st.cfg.Mode,
		"started_at":            st.startedAt,
		"uptime_seconds":        time.Since(st.startedAt).Seconds(),
		"debug_server_addr":     st.cfg.DebugServerAddr,
		"debug_server_loopback": isLoopbackAddress(st.cfg.DebugServerAddr),
		"trace_buffer_size":     st.cfg.TraceBufferSize,
		"trace_sample_ratio":    st.cfg.TraceSampleRatio,
		"otlp_enabled":          st.cfg.OTLPEndpoint != "",
		"otlp_endpoint":         RedactURLString(st.cfg.OTLPEndpoint),
		"otlp_header_keys":      sortedKeys(st.cfg.OTLPHeaders),
		"recent_spans":          len(st.exporter.Snapshot(st.cfg.TraceBufferSize)),
		"goroutines":            runtime.NumGoroutine(),
	}
}

func RecentSpans(limit int) []spanRecord {
	stateMu.RLock()
	st := current
	stateMu.RUnlock()
	if st == nil {
		return nil
	}
	return st.exporter.Snapshot(limit)
}

func startDebugServer(st *state) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(st.metrics.registry, promhttp.HandlerOpts{}))
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/debug/traces", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		writeJSON(w, RecentSpans(limit))
	})
	mux.HandleFunc("/debug/observability", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, Snapshot())
	})
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:              st.cfg.DebugServerAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		level := slog.LevelInfo
		if !isLoopbackAddress(st.cfg.DebugServerAddr) {
			level = slog.LevelWarn
		}
		LogAttrs(context.Background(), level, "Starting observability server",
			slog.String("addr", st.cfg.DebugServerAddr),
			slog.Bool("loopback_only", isLoopbackAddress(st.cfg.DebugServerAddr)),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			LogAttrs(context.Background(), slog.LevelError, "Observability server failed",
				slog.String("addr", st.cfg.DebugServerAddr),
				slog.Any("error", err),
			)
		}
	}()

	return server, nil
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func normalizeConfig(cfg Config) Config {
	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	if cfg.ServiceName == "" {
		cfg.ServiceName = "codeplane"
	}
	if cfg.TraceBufferSize <= 0 {
		cfg.TraceBufferSize = 512
	}
	if cfg.TraceSampleRatio < 0 {
		cfg.TraceSampleRatio = 0
	}
	if cfg.TraceSampleRatio > 1 {
		cfg.TraceSampleRatio = 1
	}
	if cfg.TraceSampleRatio == 0 {
		cfg.TraceSampleRatio = 1
	}
	if cfg.OTLPHeaders == nil {
		cfg.OTLPHeaders = map[string]string{}
	}
	return cfg
}

func processInstanceID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func Tracer(name string) trace.Tracer {
	if name == "" {
		name = tracerName
	}
	return otel.Tracer(name)
}

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

func RecordError(span trace.Span, err error) {
	if err == nil || span == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	return contextValue(ctx, requestIDKey)
}

func WithWorkspaceID(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, workspaceIDKey, workspaceID)
}

func WorkspaceIDFromContext(ctx context.Context) string {
	return contextValue(ctx, workspaceIDKey)
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

func SessionIDFromContext(ctx context.Context) string {
	return contextValue(ctx, sessionIDKey)
}

func WithMessageID(ctx context.Context, messageID string) context.Context {
	return context.WithValue(ctx, messageIDKey, messageID)
}

func MessageIDFromContext(ctx context.Context) string {
	return contextValue(ctx, messageIDKey)
}

func WithAgent(ctx context.Context, agent string) context.Context {
	return context.WithValue(ctx, agentKey, agent)
}

func AgentFromContext(ctx context.Context) string {
	return contextValue(ctx, agentKey)
}

func WithTool(ctx context.Context, toolName, toolCallID string) context.Context {
	ctx = context.WithValue(ctx, toolKey, toolName)
	if toolCallID != "" {
		ctx = context.WithValue(ctx, toolCallIDKey, toolCallID)
	}
	return ctx
}

func ToolFromContext(ctx context.Context) string {
	return contextValue(ctx, toolKey)
}

func ToolCallIDFromContext(ctx context.Context) string {
	return contextValue(ctx, toolCallIDKey)
}

func WithLSP(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, lspKey, name)
}

func LSPFromContext(ctx context.Context) string {
	return contextValue(ctx, lspKey)
}

func WithComponent(ctx context.Context, component string) context.Context {
	return context.WithValue(ctx, componentKey, component)
}

func ComponentFromContext(ctx context.Context) string {
	return contextValue(ctx, componentKey)
}

func LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	all := append(ContextAttrs(ctx), attrs...)
	slog.LogAttrs(ctx, level, msg, all...)
}

func ContextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, 10)
	appendAttr := func(key, value string) {
		if value != "" {
			attrs = append(attrs, slog.String(key, value))
		}
	}

	appendAttr("request_id", contextValue(ctx, requestIDKey))
	appendAttr("workspace_id", contextValue(ctx, workspaceIDKey))
	appendAttr("session_id", contextValue(ctx, sessionIDKey))
	appendAttr("message_id", contextValue(ctx, messageIDKey))
	appendAttr("agent", contextValue(ctx, agentKey))
	appendAttr("tool", contextValue(ctx, toolKey))
	appendAttr("tool_call_id", contextValue(ctx, toolCallIDKey))
	appendAttr("lsp", contextValue(ctx, lspKey))
	appendAttr("component", contextValue(ctx, componentKey))

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		appendAttr("trace_id", spanCtx.TraceID().String())
		appendAttr("span_id", spanCtx.SpanID().String())
	}

	return attrs
}

func contextValue(ctx context.Context, key contextKey) string {
	value, _ := ctx.Value(key).(string)
	return value
}

func HTTPServerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx = WithRequestID(ctx, requestIDFromHeaders(r.Header))
		if workspaceID := r.PathValue("id"); workspaceID != "" {
			ctx = WithWorkspaceID(ctx, workspaceID)
		}
		if sessionID := r.PathValue("sid"); sessionID != "" {
			ctx = WithSessionID(ctx, sessionID)
		}
		if lspName := r.PathValue("lsp"); lspName != "" {
			ctx = WithLSP(ctx, lspName)
		}
		r = r.WithContext(ctx)

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		ctx, span := StartSpan(ctx, "http.server",
			attribute.String("http.method", r.Method),
			attribute.String("http.target", r.URL.Path),
		)
		r = r.WithContext(ctx)
		recorder.Header().Set("X-Request-ID", RequestIDFromContext(ctx))
		if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
			recorder.Header().Set("X-Trace-ID", spanCtx.TraceID().String())
		}

		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("panic: %v", recovered)
				recorder.statusCode = http.StatusInternalServerError
				RecordError(span, err)
				LogAttrs(ctx, slog.LevelError, "HTTP handler panic",
					slog.Any("panic", recovered),
				)
				http.Error(recorder, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}

			route := r.Pattern
			if route == "" {
				route = r.URL.Path
			}
			if workspaceID := contextValue(ctx, workspaceIDKey); workspaceID != "" {
				span.SetAttributes(attribute.String("codeplane.workspace_id", workspaceID))
			}
			if sessionID := contextValue(ctx, sessionIDKey); sessionID != "" {
				span.SetAttributes(attribute.String("codeplane.session_id", sessionID))
			}
			if lspName := contextValue(ctx, lspKey); lspName != "" {
				span.SetAttributes(attribute.String("codeplane.lsp", lspName))
			}
			duration := time.Since(start)
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.Int("http.status_code", recorder.statusCode),
				attribute.String("http.request_id", RequestIDFromContext(ctx)),
			)
			if recorder.statusCode >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(recorder.statusCode))
			}
			span.End()

			RecordHTTPServer(r.Method, route, recorder.statusCode, duration)
			LogAttrs(ctx, slog.LevelDebug, "HTTP response",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", recorder.statusCode),
				slog.Duration("duration", duration),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			)
		}()

		LogAttrs(ctx, slog.LevelDebug, "HTTP request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)

		next.ServeHTTP(recorder, r)
	})
}

type InstrumentedRoundTripper struct {
	Transport http.RoundTripper
	Component string
}

func (rt *InstrumentedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	ctx := WithRequestID(req.Context(), RequestIDFromContext(req.Context()))
	ctx, span := StartSpan(ctx, "http.client",
		attribute.String("http.method", req.Method),
		attribute.String("http.url", RedactURLString(req.URL.String())),
		attribute.String("http.host", req.URL.Host),
		attribute.String("codeplane.http.component", rt.Component),
		attribute.String("http.request_id", RequestIDFromContext(ctx)),
	)
	defer span.End()

	req = req.Clone(ctx)
	req.Header.Set("X-Request-ID", RequestIDFromContext(ctx))
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	start := time.Now()
	resp, err := base.RoundTrip(req)
	duration := time.Since(start)

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		if resp.StatusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, resp.Status)
		}
	}
	if err != nil {
		RecordError(span, err)
	}

	RecordHTTPClient(rt.Component, req.Method, req.URL.Host, statusCode, duration)
	return resp, err
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func requestIDFromHeaders(headers http.Header) string {
	for _, key := range []string{"X-Request-ID", "X-Correlation-ID"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func RecordAppShutdown(result string) {
	if st := getState(); st != nil {
		st.metrics.appShutdownTotal.WithLabelValues(defaultResult(result)).Inc()
	}
}

func RecordAgentRunStart() {
	if st := getState(); st != nil {
		st.metrics.activeAgentRuns.Inc()
	}
}

func RecordAgentRunFinish(agent, provider, model string, duration time.Duration, err error) {
	if st := getState(); st != nil {
		result := resultFromError(err)
		st.metrics.agentRunsTotal.WithLabelValues(defaultValue(agent, "unknown"), defaultValue(provider, "unknown"), defaultValue(model, "unknown"), result).Inc()
		st.metrics.agentRunDuration.WithLabelValues(defaultValue(agent, "unknown"), defaultValue(provider, "unknown"), defaultValue(model, "unknown")).Observe(duration.Seconds())
		st.metrics.activeAgentRuns.Dec()
	}
}

func RecordToolCall(tool string, duration time.Duration, err error) {
	if st := getState(); st != nil {
		st.metrics.toolCallsTotal.WithLabelValues(defaultValue(tool, "unknown"), resultFromError(err)).Inc()
		st.metrics.toolCallDuration.WithLabelValues(defaultValue(tool, "unknown")).Observe(duration.Seconds())
	}
}

func RecordShellCommand(duration time.Duration, err error) {
	if st := getState(); st != nil {
		result := resultFromError(err)
		st.metrics.shellCommandsTotal.WithLabelValues(result).Inc()
		st.metrics.shellDuration.WithLabelValues(result).Observe(duration.Seconds())
	}
}

func RecordBackgroundJob(delta int) {
	if st := getState(); st != nil {
		st.metrics.backgroundJobs.Add(float64(delta))
	}
}

func RecordLSPOperation(name, operation string, duration time.Duration, err error) {
	if st := getState(); st != nil {
		name = defaultValue(name, "unknown")
		operation = defaultValue(operation, "unknown")
		st.metrics.lspOpsTotal.WithLabelValues(name, operation, resultFromError(err)).Inc()
		st.metrics.lspOpDuration.WithLabelValues(name, operation).Observe(duration.Seconds())
	}
}

func RecordSessionOperation(operation string, duration time.Duration, err error) {
	if st := getState(); st != nil {
		operation = defaultValue(operation, "unknown")
		st.metrics.sessionOpsTotal.WithLabelValues(operation, resultFromError(err)).Inc()
		st.metrics.sessionOpDuration.WithLabelValues(operation).Observe(duration.Seconds())
	}
}

func RecordMessageOperation(operation, role string, duration time.Duration, err error) {
	if st := getState(); st != nil {
		operation = defaultValue(operation, "unknown")
		role = defaultValue(role, "unknown")
		st.metrics.messageOpsTotal.WithLabelValues(operation, role, resultFromError(err)).Inc()
		st.metrics.messageOpDuration.WithLabelValues(operation, role).Observe(duration.Seconds())
	}
}

func RecordPermissionRequest(tool, action, result string, duration time.Duration) {
	if st := getState(); st != nil {
		tool = defaultValue(tool, "unknown")
		action = defaultValue(action, "unknown")
		result = defaultResult(result)
		st.metrics.permissionsTotal.WithLabelValues(tool, action, result).Inc()
		st.metrics.permissionDuration.WithLabelValues(tool, action).Observe(duration.Seconds())
	}
}

func RecordHTTPServer(method, route string, statusCode int, duration time.Duration) {
	if st := getState(); st != nil {
		method = defaultValue(method, "unknown")
		route = defaultValue(route, "unknown")
		status := strconv.Itoa(statusCode)
		st.metrics.httpServerTotal.WithLabelValues(method, route, status).Inc()
		st.metrics.httpServerDuration.WithLabelValues(method, route).Observe(duration.Seconds())
	}
}

func RecordHTTPClient(component, method, host string, statusCode int, duration time.Duration) {
	if st := getState(); st != nil {
		component = defaultValue(component, "unknown")
		method = defaultValue(method, "unknown")
		host = defaultValue(host, "unknown")
		status := strconv.Itoa(statusCode)
		if statusCode == 0 {
			status = "error"
		}
		st.metrics.httpClientTotal.WithLabelValues(component, method, host, status).Inc()
		st.metrics.httpClientDuration.WithLabelValues(component, method, host).Observe(duration.Seconds())
	}
}

func RecordDBOperation(operation string, duration time.Duration, err error) {
	if st := getState(); st != nil {
		operation = defaultValue(operation, "unknown")
		st.metrics.dbOpsTotal.WithLabelValues(operation, resultFromError(err)).Inc()
		st.metrics.dbOpDuration.WithLabelValues(operation).Observe(duration.Seconds())
	}
}

func RecordRetry(component, provider, reason string, delay time.Duration) {
	if st := getState(); st != nil {
		component = defaultValue(component, "unknown")
		provider = defaultValue(provider, "unknown")
		reason = defaultResult(reason)
		st.metrics.retriesTotal.WithLabelValues(component, provider, reason).Inc()
		st.metrics.retryDelay.WithLabelValues(component, provider, reason).Observe(delay.Seconds())
	}
}

func RecordWorkItemPromotion(target, result string, duration time.Duration) {
	if st := getState(); st != nil {
		target = defaultValue(target, "unknown")
		result = defaultResult(result)
		st.metrics.workItemPromotionsTotal.WithLabelValues(target, result).Inc()
		st.metrics.workItemPromotionDuration.WithLabelValues(target, result).Observe(duration.Seconds())
	}
}

func RecordPubSubEvent(broker, eventType, result string) {
	if st := getState(); st != nil {
		broker = defaultValue(broker, "unknown")
		eventType = defaultValue(eventType, "unknown")
		result = defaultResult(result)
		st.metrics.pubsubEventsTotal.WithLabelValues(broker, eventType, result).Inc()
	}
}

func SetPubSubSubscribers(broker string, count int) {
	if st := getState(); st != nil && strings.TrimSpace(broker) != "" {
		st.metrics.pubsubSubscribers.WithLabelValues(broker).Set(float64(count))
	}
}

func RecordSSEEvent(stream, result string) {
	if st := getState(); st != nil {
		stream = defaultValue(stream, "unknown")
		result = defaultResult(result)
		st.metrics.sseEventsTotal.WithLabelValues(stream, result).Inc()
	}
}

func RecordSSEConnection(stream string, delta int) {
	if st := getState(); st != nil {
		stream = defaultValue(stream, "unknown")
		st.metrics.sseConnections.WithLabelValues(stream).Add(float64(delta))
	}
}

func RecordSSEStreamDuration(stream, result string, duration time.Duration) {
	if st := getState(); st != nil {
		stream = defaultValue(stream, "unknown")
		result = defaultResult(result)
		st.metrics.sseDuration.WithLabelValues(stream, result).Observe(duration.Seconds())
	}
}

func RecordBackgroundJobLifecycle(result string) {
	if st := getState(); st != nil {
		st.metrics.backgroundJobsTotal.WithLabelValues(defaultResult(result)).Inc()
	}
}

func RecordBackgroundJobDuration(result string, duration time.Duration) {
	if st := getState(); st != nil {
		st.metrics.backgroundJobDuration.WithLabelValues(defaultResult(result)).Observe(duration.Seconds())
	}
}

func SetBackgroundTrackedJobs(count int) {
	if st := getState(); st != nil {
		st.metrics.backgroundTrackedJobs.Set(float64(count))
	}
}

func SetPermissionBacklog(count int) {
	if st := getState(); st != nil {
		st.metrics.permissionBacklog.Set(float64(count))
	}
}

func SetPermissionActive(active bool) {
	if st := getState(); st != nil {
		if active {
			st.metrics.permissionActive.Set(1)
			return
		}
		st.metrics.permissionActive.Set(0)
	}
}

func RecordPermissionQueueDelay(tool, action string, duration time.Duration) {
	if st := getState(); st != nil {
		tool = defaultValue(tool, "unknown")
		action = defaultValue(action, "unknown")
		st.metrics.permissionQueueDelay.WithLabelValues(tool, action).Observe(duration.Seconds())
	}
}

func RecordWorkspaceLifecycle(operation, result string, duration time.Duration) {
	if st := getState(); st != nil {
		operation = defaultValue(operation, "unknown")
		result = defaultResult(result)
		st.metrics.workspaceLifecycleTotal.WithLabelValues(operation, result).Inc()
		st.metrics.workspaceLifecycleDuration.WithLabelValues(operation).Observe(duration.Seconds())
	}
}

func SetActiveWorkspaces(count int) {
	if st := getState(); st != nil {
		st.metrics.activeWorkspaces.Set(float64(count))
	}
}

func RecordDroppedEvent(subsystem string) {
	if st := getState(); st != nil {
		st.metrics.droppedEventsTotal.WithLabelValues(defaultValue(subsystem, "unknown")).Inc()
	}
}

func RecordStartupFlow(flow, source, result string, attrs ...attribute.KeyValue) {
	flow = defaultValue(flow, "unknown")
	source = defaultValue(source, "unknown")
	result = defaultResult(result)

	if st := getState(); st != nil {
		st.metrics.startupFlowsTotal.WithLabelValues(flow, source, result).Inc()
	}

	ctx := WithComponent(context.Background(), "startup_flow")
	attrs = append([]attribute.KeyValue{
		attribute.String("codeplane.startup.flow", flow),
		attribute.String("codeplane.startup.source", source),
		attribute.String("codeplane.startup.result", result),
	}, attrs...)
	_, span := StartSpan(ctx, "codeplane.startup."+flow, attrs...)
	if result != "ok" {
		span.SetStatus(codes.Error, result)
	}
	span.End()
}

func RecordUINavigation(entrypoint, target, result string, attrs ...attribute.KeyValue) {
	entrypoint = defaultValue(entrypoint, "unknown")
	target = defaultValue(target, "unknown")
	result = defaultResult(result)

	if st := getState(); st != nil {
		st.metrics.uiNavigationTotal.WithLabelValues(entrypoint, target, result).Inc()
	}

	ctx := WithComponent(context.Background(), "ui_navigation")
	attrs = append([]attribute.KeyValue{
		attribute.String("codeplane.ui.entrypoint", entrypoint),
		attribute.String("codeplane.ui.target", target),
		attribute.String("codeplane.ui.result", result),
	}, attrs...)
	_, span := StartSpan(ctx, "ui.navigation", attrs...)
	if result != "ok" {
		span.SetStatus(codes.Error, result)
	}
	span.End()
}

func RecordSnapshotOperation(operation string, duration time.Duration, err error, attrs ...attribute.KeyValue) {
	operation = defaultValue(operation, "unknown")
	result := resultFromError(err)

	if st := getState(); st != nil {
		st.metrics.snapshotOpsTotal.WithLabelValues(operation, result).Inc()
		st.metrics.snapshotOpDuration.WithLabelValues(operation).Observe(duration.Seconds())
	}

	ctx := WithComponent(context.Background(), "ui_snapshots")
	attrs = append([]attribute.KeyValue{
		attribute.String("codeplane.snapshot.operation", operation),
		attribute.String("codeplane.snapshot.result", result),
		attribute.Int64("codeplane.snapshot.duration_ms", duration.Milliseconds()),
	}, attrs...)
	_, span := StartSpan(ctx, "ui.snapshots."+operation, attrs...)
	RecordError(span, err)
	span.End()
}

func getState() *state {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return current
}

func resultFromError(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}

func defaultResult(result string) string {
	if strings.TrimSpace(result) == "" {
		return "unknown"
	}
	return result
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func attributesToMap(attrs []attribute.KeyValue) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		key := string(attr.Key)
		out[key] = sanitizeAttributeValue(key, attributeValue(attr.Value))
	}
	return out
}

func spanEventsToRecords(events []sdktrace.Event) []spanEventRecord {
	if len(events) == 0 {
		return nil
	}
	out := make([]spanEventRecord, 0, len(events))
	for _, event := range events {
		out = append(out, spanEventRecord{
			Name:       event.Name,
			Time:       event.Time,
			Attributes: attributesToMap(event.Attributes),
		})
	}
	return out
}

func attributeValue(value attribute.Value) any {
	switch value.Type() {
	case attribute.BOOL:
		return value.AsBool()
	case attribute.INT64:
		return value.AsInt64()
	case attribute.FLOAT64:
		return value.AsFloat64()
	case attribute.STRING:
		return value.AsString()
	case attribute.BOOLSLICE:
		return value.AsBoolSlice()
	case attribute.INT64SLICE:
		return value.AsInt64Slice()
	case attribute.FLOAT64SLICE:
		return value.AsFloat64Slice()
	case attribute.STRINGSLICE:
		return value.AsStringSlice()
	default:
		return value.Emit()
	}
}

func sanitizeAttributeValue(key string, value any) any {
	if value == nil {
		return nil
	}
	if isSensitiveKey(key) {
		return redactedValue
	}
	switch v := value.(type) {
	case string:
		switch {
		case looksLikeURLKey(key):
			return RedactURLString(v)
		case looksLikePayloadKey(key):
			return RedactPayload("", []byte(v))
		default:
			return truncateString(v, 2048)
		}
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = fmt.Sprint(sanitizeAttributeValue(key, item))
		}
		return out
	default:
		return value
	}
}

func RedactHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	filtered := make(map[string][]string, len(headers))
	for key, values := range headers {
		if isSensitiveKey(key) {
			filtered[key] = []string{redactedValue}
			continue
		}
		filtered[key] = append([]string(nil), values...)
	}
	return filtered
}

func RedactStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	filtered := make(map[string]string, len(values))
	for key, value := range values {
		if isSensitiveKey(key) {
			filtered[key] = redactedValue
			continue
		}
		filtered[key] = truncateString(value, 256)
	}
	return filtered
}

func RedactURLString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return truncateString(raw, 512)
	}
	return redactURL(parsed).String()
}

func RedactPayload(contentType string, payload []byte) string {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return ""
	}

	originalLen := len(payload)
	const maxLoggedPayloadBytes = 8 << 10
	truncated := false
	if len(payload) > maxLoggedPayloadBytes {
		payload = payload[:maxLoggedPayloadBytes]
		truncated = true
	}

	var rendered string
	switch {
	case isJSONContentType(contentType) || json.Valid(payload):
		rendered = redactJSONPayload(payload)
	case isFormContentType(contentType):
		rendered = redactFormPayload(payload)
	default:
		rendered = redactFreeformPayload(payload)
	}
	if truncated {
		rendered = fmt.Sprintf("%s\n[truncated to %d of %d bytes]", rendered, maxLoggedPayloadBytes, originalLen)
	}
	return rendered
}

func redactJSONPayload(payload []byte) string {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return truncateString(string(payload), 2048)
	}
	value = redactJSONValue("", value)
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return truncateString(string(payload), 2048)
	}
	return string(pretty)
}

func redactFormPayload(payload []byte) string {
	values, err := url.ParseQuery(string(payload))
	if err != nil {
		return truncateString(string(payload), 2048)
	}
	for key := range values {
		if isSensitiveKey(key) {
			values.Set(key, redactedValue)
		}
	}
	return values.Encode()
}

func redactJSONValue(parentKey string, value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveKey(key) {
				out[key] = redactedValue
				continue
			}
			out[key] = redactJSONValue(key, item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactJSONValue(parentKey, item)
		}
		return out
	case string:
		if isSensitiveKey(parentKey) {
			return redactedValue
		}
		if looksLikeURLKey(parentKey) {
			return RedactURLString(v)
		}
		return truncateString(v, 2048)
	default:
		return value
	}
}

func redactURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	cloned := *u
	cloned.Fragment = ""
	if len(cloned.RawQuery) == 0 {
		return &cloned
	}
	query := cloned.Query()
	for key, values := range query {
		if isSensitiveKey(key) {
			query[key] = []string{redactedValue}
			continue
		}
		for i, value := range values {
			query[key][i] = truncateString(value, 128)
		}
	}
	cloned.RawQuery = query.Encode()
	return &cloned
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	if key == "auth" || strings.HasSuffix(key, "_auth") || strings.HasSuffix(key, "-auth") {
		return true
	}
	for _, fragment := range []string{
		"api_key",
		"apikey",
		"authorization",
		"cookie",
		"credential",
		"password",
		"secret",
		"signature",
		"token",
		"x-api-key",
		"set-cookie",
		"client_secret",
		"private_key",
	} {
		if key == fragment ||
			strings.HasSuffix(key, "_"+fragment) ||
			strings.HasSuffix(key, "-"+fragment) ||
			strings.Contains(key, "."+fragment) {
			return true
		}
	}
	return false
}

func looksLikeURLKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "url") || strings.Contains(key, "endpoint")
}

func looksLikePayloadKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "body") || strings.Contains(key, "payload")
}

func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "application/json") || strings.Contains(contentType, "+json")
}

func isFormContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "application/x-www-form-urlencoded")
}

func truncateString(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func redactFreeformPayload(payload []byte) string {
	text := string(payload)
	lower := strings.ToLower(text)
	for _, fragment := range []string{
		"authorization:",
		"authorization=",
		"bearer ",
		"password",
		"secret",
		"token=",
		"x-api-key",
	} {
		if strings.Contains(lower, fragment) {
			return fmt.Sprintf("[payload omitted, %d bytes]", len(payload))
		}
	}
	return truncateString(text, 2048)
}

func isLoopbackAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	host := addr
	if parsedHost, _, err := net.SplitHostPort(addr); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "", "localhost":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sortedKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
