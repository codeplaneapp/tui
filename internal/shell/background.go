package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	crushlog "github.com/charmbracelet/crush/internal/log"
	"github.com/charmbracelet/crush/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// MaxBackgroundJobs is the maximum number of concurrent background jobs allowed
	MaxBackgroundJobs = 50
	// CompletedJobRetentionMinutes is how long to keep completed jobs before auto-cleanup (8 hours)
	CompletedJobRetentionMinutes = 8 * 60
)

// syncBuffer is a thread-safe wrapper around bytes.Buffer.
type syncBuffer struct {
	buf bytes.Buffer
	mu  sync.RWMutex
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) WriteString(s string) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.WriteString(s)
}

func (sb *syncBuffer) String() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.buf.String()
}

// BackgroundShell represents a shell running in the background.
type BackgroundShell struct {
	ID          string
	Command     string
	Description string
	Shell       *Shell
	WorkingDir  string
	ctx         context.Context
	cancel      context.CancelFunc
	stdout      *syncBuffer
	stderr      *syncBuffer
	done        chan struct{}
	exitErr     error
	startedAt   time.Time
	completedAt atomic.Int64 // Unix timestamp when job completed (0 if still running)
}

// BackgroundShellManager manages background shell instances.
type BackgroundShellManager struct {
	shells *csync.Map[string, *BackgroundShell]
}

var (
	backgroundManager     *BackgroundShellManager
	backgroundManagerOnce sync.Once
	idCounter             atomic.Uint64
)

// newBackgroundShellManager creates a new BackgroundShellManager instance.
func newBackgroundShellManager() *BackgroundShellManager {
	return &BackgroundShellManager{
		shells: csync.NewMap[string, *BackgroundShell](),
	}
}

// GetBackgroundShellManager returns the singleton background shell manager.
func GetBackgroundShellManager() *BackgroundShellManager {
	backgroundManagerOnce.Do(func() {
		backgroundManager = newBackgroundShellManager()
	})
	return backgroundManager
}

// Start creates and starts a new background shell with the given command.
func (m *BackgroundShellManager) Start(ctx context.Context, workingDir string, blockFuncs []BlockFunc, command string, description string) (*BackgroundShell, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Check job limit
	if m.shells.Len() >= MaxBackgroundJobs {
		observability.RecordBackgroundJobLifecycle("rejected_limit")
		return nil, fmt.Errorf("maximum number of background jobs (%d) reached. Please terminate or wait for some jobs to complete", MaxBackgroundJobs)
	}

	id := fmt.Sprintf("%03X", idCounter.Add(1))
	jobCtx := observability.WithComponent(ctx, "background_shell")
	jobCtx, span := observability.StartSpan(jobCtx, "shell.background", backgroundShellAttributes(jobCtx, id, workingDir, command)...)
	shellCtx, cancel := context.WithCancel(jobCtx)

	shell := NewShell(&Options{
		WorkingDir: workingDir,
		BlockFuncs: blockFuncs,
	})

	bgShell := &BackgroundShell{
		ID:          id,
		Command:     command,
		Description: description,
		WorkingDir:  workingDir,
		Shell:       shell,
		ctx:         shellCtx,
		cancel:      cancel,
		stdout:      &syncBuffer{},
		stderr:      &syncBuffer{},
		done:        make(chan struct{}),
		startedAt:   time.Now(),
	}

	m.shells.Set(id, bgShell)
	observability.RecordBackgroundJob(1)
	observability.RecordBackgroundJobLifecycle("started")
	observability.SetBackgroundTrackedJobs(m.shells.Len())
	observability.LogAttrs(jobCtx, slog.LevelDebug, "Background shell started",
		slog.String("job_id", id),
		slog.String("working_dir", workingDir),
		slog.Int("command_length", len(command)),
	)

	go func() {
		var (
			err    error
			result = "completed"
		)

		defer close(bgShell.done)
		defer observability.RecordBackgroundJob(-1)
		defer func() {
			bgShell.exitErr = err
			bgShell.completedAt.Store(time.Now().Unix())
			observability.RecordBackgroundJobLifecycle(result)
			observability.RecordBackgroundJobDuration(result, time.Since(bgShell.startedAt))
			observability.RecordError(span, err)
			span.SetAttributes(
				attribute.String("background.job.result", result),
				attribute.Int("shell.exit_code", ExitCode(err)),
			)
			span.End()
			observability.LogAttrs(jobCtx, slog.LevelDebug, "Background shell finished",
				slog.String("job_id", id),
				slog.String("result", result),
				slog.Int("exit_code", ExitCode(err)),
				slog.Duration("duration", time.Since(bgShell.startedAt)),
				slog.Any("error", err),
			)
		}()
		defer crushlog.RecoverPanic(jobCtx, "BackgroundShellManager.Start", func() {
			result = "panic"
			err = fmt.Errorf("background shell panic")
		})

		err = shell.ExecStream(shellCtx, command, bgShell.stdout, bgShell.stderr)
		if errors.Is(err, context.Canceled) {
			result = "canceled"
		} else if err != nil {
			result = "failed"
		}
	}()

	return bgShell, nil
}

// Get retrieves a background shell by ID.
func (m *BackgroundShellManager) Get(id string) (*BackgroundShell, bool) {
	return m.shells.Get(id)
}

// Remove removes a background shell from the manager without terminating it.
// This is useful when a shell has already completed and you just want to clean up tracking.
func (m *BackgroundShellManager) Remove(id string) error {
	_, ok := m.shells.Take(id)
	if !ok {
		return fmt.Errorf("background shell not found: %s", id)
	}
	observability.RecordBackgroundJobLifecycle("removed")
	observability.SetBackgroundTrackedJobs(m.shells.Len())
	return nil
}

// Kill terminates a background shell by ID.
func (m *BackgroundShellManager) Kill(id string) error {
	shell, ok := m.shells.Take(id)
	if !ok {
		return fmt.Errorf("background shell not found: %s", id)
	}
	observability.RecordBackgroundJobLifecycle("kill_requested")
	observability.SetBackgroundTrackedJobs(m.shells.Len())
	addBackgroundShellEvent(shell.ctx, "kill_requested")
	observability.LogAttrs(shell.ctx, slog.LevelDebug, "Background shell cancel requested",
		slog.String("job_id", id),
	)

	shell.cancel()
	<-shell.done
	return nil
}

// BackgroundShellInfo contains information about a background shell.
type BackgroundShellInfo struct {
	ID          string
	Command     string
	Description string
}

// List returns all background shell IDs.
func (m *BackgroundShellManager) List() []string {
	ids := make([]string, 0, m.shells.Len())
	for id := range m.shells.Seq2() {
		ids = append(ids, id)
	}
	return ids
}

// Cleanup removes completed jobs that have been finished for more than the retention period
func (m *BackgroundShellManager) Cleanup() int {
	now := time.Now().Unix()
	retentionSeconds := int64(CompletedJobRetentionMinutes * 60)

	var toRemove []string
	for shell := range m.shells.Seq() {
		completedAt := shell.completedAt.Load()
		if completedAt > 0 && now-completedAt > retentionSeconds {
			toRemove = append(toRemove, shell.ID)
		}
	}

	for _, id := range toRemove {
		m.Remove(id)
	}

	if len(toRemove) > 0 {
		observability.RecordBackgroundJobLifecycle("cleaned")
		observability.SetBackgroundTrackedJobs(m.shells.Len())
	}

	return len(toRemove)
}

// KillAll terminates all background shells. The provided context bounds how
// long the function waits for each shell to exit.
func (m *BackgroundShellManager) KillAll(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	shells := slices.Collect(m.shells.Seq())
	m.shells.Reset(map[string]*BackgroundShell{})
	observability.SetBackgroundTrackedJobs(0)
	killCtx := observability.WithComponent(ctx, "background_shell_manager")
	killCtx, span := observability.StartSpan(killCtx, "shell.background.kill_all",
		attribute.Int("background.jobs.count", len(shells)),
	)
	start := time.Now()
	result := "ok"
	defer func() {
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result = "timeout"
			} else {
				result = "canceled"
			}
			observability.RecordError(span, ctx.Err())
		}
		span.SetAttributes(attribute.String("background.kill_all.result", result))
		span.End()
		observability.LogAttrs(killCtx, slog.LevelDebug, "Background shell manager kill-all finished",
			slog.String("result", result),
			slog.Int("jobs", len(shells)),
			slog.Duration("duration", time.Since(start)),
		)
	}()

	var wg sync.WaitGroup
	for _, shell := range shells {
		wg.Go(func() {
			addBackgroundShellEvent(shell.ctx, "kill_all_requested")
			shell.cancel()
			select {
			case <-shell.done:
			case <-ctx.Done():
			}
		})
	}
	wg.Wait()
}

func backgroundShellAttributes(ctx context.Context, id, workingDir, command string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("crush.background_job.id", id),
		attribute.String("shell.cwd", workingDir),
		attribute.Int("shell.command_length", len(command)),
	}
	if workspaceID := observability.WorkspaceIDFromContext(ctx); workspaceID != "" {
		attrs = append(attrs, attribute.String("crush.workspace_id", workspaceID))
	}
	if sessionID := observability.SessionIDFromContext(ctx); sessionID != "" {
		attrs = append(attrs, attribute.String("crush.session_id", sessionID))
	}
	if tool := observability.ToolFromContext(ctx); tool != "" {
		attrs = append(attrs, attribute.String("crush.tool", tool))
	}
	if toolCallID := observability.ToolCallIDFromContext(ctx); toolCallID != "" {
		attrs = append(attrs, attribute.String("crush.tool_call_id", toolCallID))
	}
	return attrs
}

func addBackgroundShellEvent(ctx context.Context, name string) {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return
	}
	span.AddEvent(name)
}

// GetOutput returns the current output of a background shell.
func (bs *BackgroundShell) GetOutput() (stdout string, stderr string, done bool, err error) {
	select {
	case <-bs.done:
		return bs.stdout.String(), bs.stderr.String(), true, bs.exitErr
	default:
		return bs.stdout.String(), bs.stderr.String(), false, nil
	}
}

// IsDone checks if the background shell has finished execution.
func (bs *BackgroundShell) IsDone() bool {
	select {
	case <-bs.done:
		return true
	default:
		return false
	}
}

// Wait blocks until the background shell completes.
func (bs *BackgroundShell) Wait() {
	<-bs.done
}

func (bs *BackgroundShell) WaitContext(ctx context.Context) bool {
	select {
	case <-bs.done:
		return true
	case <-ctx.Done():
		return false
	}
}
