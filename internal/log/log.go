package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/crush/internal/event"
	"github.com/charmbracelet/x/term"
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

func RecoverPanic(name string, cleanup func()) {
	if r := recover(); r != nil {
		event.Error(r, "panic", true, "name", name)

		// Create a timestamped panic log file
		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("crush-panic-%s-%s.log", name, timestamp)

		file, err := os.Create(filename)
		if err == nil {
			defer file.Close()

			// Write panic information and stack trace
			fmt.Fprintf(file, "Panic in %s: %v\n\n", name, r)
			fmt.Fprintf(file, "Time: %s\n\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(file, "Stack Trace:\n%s\n", debug.Stack())

			// Execute cleanup function if provided
			if cleanup != nil {
				cleanup()
			}
		}
	}
}
