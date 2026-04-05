package smithers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrBinaryNotFound is returned when the smithers CLI binary cannot be located.
var ErrBinaryNotFound = errors.New("smithers binary not found")

// ExecError wraps a non-zero exit from the smithers CLI with structured fields.
type ExecError struct {
	Command string // e.g. "smithers ps --format json"
	Stderr  string // captured stderr
	Exit    int    // exit code
}

func (e *ExecError) Error() string {
	return fmt.Sprintf("smithers %s (exit %d): %s", e.Command, e.Exit, e.Stderr)
}

// JSONParseError wraps a JSON decode failure from CLI output.
type JSONParseError struct {
	Command string
	Output  []byte
	Err     error
}

func (e *JSONParseError) Error() string {
	return fmt.Sprintf("parse output of smithers %s: %s", e.Command, e.Err)
}

// Unwrap implements errors.Unwrap for errors.Is/As traversal.
func (e *JSONParseError) Unwrap() error { return e.Err }

// Logger is an optional interface for transport-level diagnostics.
// Implementations should be safe for concurrent use.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
}

// WithBinaryPath sets the path to the smithers CLI binary.
// Defaults to "smithers" (resolved via $PATH).
func WithBinaryPath(path string) ClientOption {
	return func(c *Client) { c.binaryPath = path }
}

// WithExecTimeout sets a default timeout for exec invocations.
// The timeout is applied only when the context passed to execSmithers has no
// deadline set. A value of 0 (the default) means no timeout is applied.
func WithExecTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.execTimeout = d }
}

// WithWorkingDir sets the working directory for exec invocations.
// The smithers CLI discovers .smithers/ project context from its cwd.
// When empty (the default), the TUI process's cwd is inherited.
func WithWorkingDir(dir string) ClientOption {
	return func(c *Client) { c.workingDir = dir }
}

// WithLogger attaches a Logger to the client for transport-level diagnostics.
// When nil (the default), all log calls are silently discarded.
func WithLogger(l Logger) ClientOption {
	return func(c *Client) { c.logger = l }
}

// hasBinary returns true if the smithers CLI binary can be found.
func (c *Client) hasBinary() bool {
	_, err := c.lookPath(c.binaryPath)
	return err == nil
}

// execSmithers shells out to the smithers CLI and returns stdout.
// Preference order for exec:
//  1. If c.execFunc is set (test injection), delegate entirely to it.
//  2. Check binary presence via c.lookPath and return ErrBinaryNotFound early.
//  3. Build exec.CommandContext with working-dir and optional timeout wrapping.
//  4. Wrap non-zero exits as *ExecError; wrap other errors with context.
func (c *Client) execSmithers(ctx context.Context, args ...string) ([]byte, error) {
	if c.execFunc != nil {
		return c.execFunc(ctx, args...)
	}

	if !c.hasBinary() {
		if c.logger != nil {
			c.logger.Warn("smithers binary not found", "binaryPath", c.binaryPath)
		}
		return nil, ErrBinaryNotFound
	}

	// Apply default exec timeout when the context has no deadline.
	if c.execTimeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.execTimeout)
			defer cancel()
		}
	}

	if c.logger != nil {
		c.logger.Debug("exec invocation",
			"command", c.binaryPath,
			"args", strings.Join(args, " "),
			"workingDir", c.workingDir,
		)
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	if c.workingDir != "" {
		cmd.Dir = c.workingDir
	}

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, &ExecError{
				Command: strings.Join(args, " "),
				Stderr:  strings.TrimSpace(string(exitErr.Stderr)),
				Exit:    exitErr.ExitCode(),
			}
		}
		return nil, fmt.Errorf("smithers %s: %w", strings.Join(args, " "), err)
	}

	if c.logger != nil {
		c.logger.Debug("exec completed",
			"command", c.binaryPath,
			"args", strings.Join(args, " "),
			"duration", time.Since(start),
			"outputBytes", len(out),
		)
	}

	return out, nil
}
