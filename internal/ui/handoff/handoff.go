// Package handoff provides a reusable Bubble Tea utility for suspending the TUI,
// handing terminal control to an external CLI process (e.g. claude, codex, an
// $EDITOR), and seamlessly resuming the TUI when that process exits.
//
// Typical usage from a Bubble Tea Update function:
//
//	case someKeyMsg:
//	    return m, handoff.Handoff(handoff.Options{
//	        Binary: "claude",
//	        Args:   []string{"--continue"},
//	        Cwd:    m.projectDir,
//	        Tag:    "claude-session",
//	    })
//
// When the external process exits the model receives a [HandoffMsg].
package handoff

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Options configures a single handoff invocation.
type Options struct {
	// Binary is the name or absolute path of the program to run.
	// exec.LookPath is used to resolve relative names.
	Binary string

	// Args are the command-line arguments passed to Binary.
	Args []string

	// Cwd is the working directory for the child process.
	// When empty the current process's working directory is used.
	Cwd string

	// Env contains KEY=VALUE pairs that are merged on top of os.Environ().
	// These values take precedence over inherited environment variables.
	Env []string

	// Tag is an opaque caller-defined value that is echoed back in HandoffMsg.
	// It allows a model with multiple handoff paths to demultiplex results.
	Tag any
}

// HandoffResult carries the outcome of the external process.
type HandoffResult struct {
	// ExitCode is the exit code of the child process (0 on success).
	ExitCode int

	// Err is non-nil when the process could not be started, was killed by a
	// signal, or exited with a non-zero status.  A Ctrl+C interrupt from the
	// user arrives here as an *exec.ExitError with signal information.
	Err error

	// Duration is the wall-clock time the child process ran for.
	Duration time.Duration
}

// HandoffMsg is the Bubble Tea message dispatched to the model after the
// external process exits (or fails to start).
type HandoffMsg struct {
	// Tag is the caller-supplied Options.Tag value, unchanged.
	Tag any

	// Result holds the process outcome.
	Result HandoffResult
}

// Handoff builds a [tea.Cmd] that suspends the TUI, runs the external program
// described by opts, and returns a [HandoffMsg] when the program exits.
//
// Validation (binary path resolution, non-empty binary) happens eagerly inside
// the returned tea.Cmd so that the TUI is never suspended for a command that
// cannot run.
func Handoff(opts Options) tea.Cmd {
	return func() tea.Msg {
		if opts.Binary == "" {
			return HandoffMsg{
				Tag: opts.Tag,
				Result: HandoffResult{
					ExitCode: 1,
					Err:      errors.New("handoff: binary must not be empty"),
				},
			}
		}

		resolvedPath, err := exec.LookPath(opts.Binary)
		if err != nil {
			return HandoffMsg{
				Tag: opts.Tag,
				Result: HandoffResult{
					ExitCode: 1,
					Err:      fmt.Errorf("handoff: binary %q not found: %w", opts.Binary, err),
				},
			}
		}

		cmd, err := buildCmd(resolvedPath, opts.Args, opts.Cwd, opts.Env)
		if err != nil {
			return HandoffMsg{
				Tag: opts.Tag,
				Result: HandoffResult{
					ExitCode: 1,
					Err:      fmt.Errorf("handoff: could not build command: %w", err),
				},
			}
		}

		start := time.Now()

		// tea.ExecProcess suspends the TUI, hands the terminal to cmd, and
		// calls back when cmd exits (or fails).
		return tea.ExecProcess(cmd, func(procErr error) tea.Msg {
			result := HandoffResult{
				Err:      procErr,
				Duration: time.Since(start),
			}
			if procErr != nil {
				result.ExitCode = exitCodeFromError(procErr)
			}
			return HandoffMsg{Tag: opts.Tag, Result: result}
		})()
	}
}

// HandoffWithCallback is a lower-level variant that lets the caller supply a
// custom tea.ExecCallback instead of receiving a [HandoffMsg].  Use [Handoff]
// unless you need to return a domain-specific message type.
//
// Returns an error tea.Cmd immediately when the binary cannot be resolved or
// the command cannot be constructed; otherwise returns a tea.ExecProcess cmd.
func HandoffWithCallback(opts Options, callback tea.ExecCallback) tea.Cmd {
	if opts.Binary == "" {
		return func() tea.Msg {
			return callback(errors.New("handoff: binary must not be empty"))
		}
	}

	resolvedPath, err := exec.LookPath(opts.Binary)
	if err != nil {
		return func() tea.Msg {
			return callback(fmt.Errorf("handoff: binary %q not found: %w", opts.Binary, err))
		}
	}

	cmd, err := buildCmd(resolvedPath, opts.Args, opts.Cwd, opts.Env)
	if err != nil {
		return func() tea.Msg {
			return callback(fmt.Errorf("handoff: could not build command: %w", err))
		}
	}

	return tea.ExecProcess(cmd, callback)
}

// buildCmd constructs an *exec.Cmd from the resolved binary path, arguments,
// working directory, and extra environment variables.  It is an internal helper
// extracted here so that it can be unit-tested without touching the TUI.
//
// envOverrides are KEY=VALUE strings that override matching keys from
// os.Environ(); new keys are appended.
func buildCmd(resolvedPath string, args []string, cwd string, envOverrides []string) (*exec.Cmd, error) {
	cmd := exec.Command(resolvedPath, args...) //nolint:gosec // path validated by LookPath

	// Working directory.
	if cwd != "" {
		if _, err := os.Stat(cwd); err != nil {
			return nil, fmt.Errorf("working directory %q: %w", cwd, err)
		}
		cmd.Dir = cwd
	}

	// Build merged environment: start from the parent process, then apply
	// caller-supplied overrides so agent API keys etc. take effect.
	cmd.Env = mergeEnv(os.Environ(), envOverrides)

	return cmd, nil
}

// mergeEnv returns a copy of base with each entry from overrides applied.
// If an override shares a KEY= prefix with a base entry the base entry is
// replaced; otherwise the override is appended.
func mergeEnv(base, overrides []string) []string {
	if len(overrides) == 0 {
		result := make([]string, len(base))
		copy(result, base)
		return result
	}

	// Build a map of key → index in result for fast lookup.
	result := make([]string, len(base))
	copy(result, base)
	index := make(map[string]int, len(result))
	for i, entry := range result {
		key, _, _ := splitEnvEntry(entry)
		index[key] = i
	}

	for _, override := range overrides {
		key, _, ok := splitEnvEntry(override)
		if !ok {
			// Malformed entry — append as-is so callers can debug.
			result = append(result, override)
			continue
		}
		if i, exists := index[key]; exists {
			result[i] = override
		} else {
			index[key] = len(result)
			result = append(result, override)
		}
	}

	return result
}

// splitEnvEntry splits a KEY=VALUE string and returns (key, value, true).
// Returns ("", "", false) when the string contains no '=' separator.
func splitEnvEntry(entry string) (key, value string, ok bool) {
	for i := 0; i < len(entry); i++ {
		if entry[i] == '=' {
			return entry[:i], entry[i+1:], true
		}
	}
	return "", "", false
}

// exitCodeFromError extracts the numeric exit code from an error returned by
// exec.Cmd.Run.  Returns 1 for any non-exit-error (e.g. signal kill, I/O
// error).
func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}
