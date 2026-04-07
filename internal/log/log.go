package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/crush/internal/event"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/x/term"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	initialized atomic.Bool
)

func Setup(logFile string, debug bool, ws ...io.Writer) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	var handlers []slog.Handler
	if logFile != "" {
		logRotator := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    10,    // Max size in MB
			MaxBackups: 0,     // Number of backups
			MaxAge:     30,    // Days
			Compress:   false, // Enable compression
		}
		handlers = append(handlers, slog.NewJSONHandler(logRotator, opts))
	}

	for _, w := range ws {
		if w == nil {
			continue
		}
		if f, ok := w.(term.File); ok && term.IsTerminal(f.Fd()) {
			handlers = append(handlers, slog.NewTextHandler(w, opts))
		} else {
			handlers = append(handlers, slog.NewJSONHandler(w, opts))
		}
	}

	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(io.Discard, opts))
	}

	slog.SetDefault(slog.New(slog.NewMultiHandler(handlers...)))
	initialized.Store(true)
}

func Initialized() bool {
	return initialized.Load()
}

func RecoverPanic(ctx context.Context, name string, cleanup func()) {
	if r := recover(); r != nil {
		if cleanup != nil {
			cleanup()
		}

		panicErr := fmt.Errorf("panic: %v", r)
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().IsValid() {
			span.AddEvent("panic",
				trace.WithAttributes(
					attribute.String("panic.name", name),
					attribute.String("panic.value", fmt.Sprint(r)),
				),
			)
			observability.RecordError(span, panicErr)
		}

		traceID := ""
		if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
			traceID = spanCtx.TraceID().String()
		}
		event.Error(r, "panic", true,
			"name", name,
			"request_id", observability.RequestIDFromContext(ctx),
			"trace_id", traceID,
		)
		observability.LogAttrs(ctx, slog.LevelError, "Recovered panic",
			slog.String("name", name),
			slog.Any("panic", r),
		)

		// Create a timestamped panic log file
		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("crush-panic-%s-%s.log", name, timestamp)

		file, err := os.Create(filename)
		if err == nil {
			defer file.Close()

			// Write panic information and stack trace
			fmt.Fprintf(file, "Panic in %s: %v\n\n", name, r)
			fmt.Fprintf(file, "Time: %s\n\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(file, "Request ID: %s\n", observability.RequestIDFromContext(ctx))
			if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
				fmt.Fprintf(file, "Trace ID: %s\n", spanCtx.TraceID().String())
				fmt.Fprintf(file, "Span ID: %s\n", spanCtx.SpanID().String())
			}
			for _, attr := range observability.ContextAttrs(ctx) {
				fmt.Fprintf(file, "%s: %s\n", attr.Key, attr.Value.String())
			}
			fmt.Fprintln(file)
			fmt.Fprintf(file, "Stack Trace:\n%s\n", debug.Stack())
			observability.LogAttrs(ctx, slog.LevelError, "Panic log written",
				slog.String("name", name),
				slog.String("file", filepath.Clean(filename)),
			)
		} else {
			observability.LogAttrs(ctx, slog.LevelError, "Failed to write panic log",
				slog.String("name", name),
				slog.Any("error", err),
			)
		}
	}
}
