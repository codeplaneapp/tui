package shell

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Benchmark to measure CPU efficiency
func BenchmarkShellQuickCommands(b *testing.B) {
	shell := NewShell(&Options{WorkingDir: b.TempDir()})

	b.ReportAllocs()

	for b.Loop() {
		_, _, err := shell.Exec(b.Context(), "echo test")
		exitCode := ExitCode(err)
		if err != nil || exitCode != 0 {
			b.Fatalf("Command failed: %v, exit code: %d", err, exitCode)
		}
	}
}

func TestTestTimeout(t *testing.T) {
	// XXX(@andreynering): This fails on Windows. Address once possible.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	t.Cleanup(cancel)

	shell := NewShell(&Options{WorkingDir: t.TempDir()})
	_, _, err := shell.Exec(ctx, "sleep 10")
	if status := ExitCode(err); status == 0 {
		t.Fatalf("Expected non-zero exit status, got %d", status)
	}
	if !IsInterrupt(err) {
		t.Fatalf("Expected command to be interrupted, but it was not")
	}
	if err == nil {
		t.Fatalf("Expected an error due to timeout, but got none")
	}
}

func TestTestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // immediately cancel the context

	shell := NewShell(&Options{WorkingDir: t.TempDir()})
	_, _, err := shell.Exec(ctx, "sleep 10")
	if status := ExitCode(err); status == 0 {
		t.Fatalf("Expected non-zero exit status, got %d", status)
	}
	if !IsInterrupt(err) {
		t.Fatalf("Expected command to be interrupted, but it was not")
	}
	if err == nil {
		t.Fatalf("Expected an error due to cancel, but got none")
	}
}

func TestRunCommandError(t *testing.T) {
	shell := NewShell(&Options{WorkingDir: t.TempDir()})
	_, _, err := shell.Exec(t.Context(), "nopenopenope")
	if status := ExitCode(err); status == 0 {
		t.Fatalf("Expected non-zero exit status, got %d", status)
	}
	if IsInterrupt(err) {
		t.Fatalf("Expected command to not be interrupted, but it was")
	}
	if err == nil {
		t.Fatalf("Expected an error, got nil")
	}
}

func TestRunContinuity(t *testing.T) {
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()

	shell := NewShell(&Options{WorkingDir: tempDir1})
	if _, _, err := shell.Exec(t.Context(), "export FOO=bar"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	if _, _, err := shell.Exec(t.Context(), "cd "+filepath.ToSlash(tempDir2)); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	out, _, err := shell.Exec(t.Context(), "echo $FOO ; pwd")
	if err != nil {
		t.Fatalf("failed to echo: %v", err)
	}
	expect := "bar\n" + tempDir2 + "\n"
	if out != expect {
		t.Fatalf("expected output %q, got %q", expect, out)
	}
}

func TestCrossPlatformExecution(t *testing.T) {
	shell := NewShell(&Options{WorkingDir: "."})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Test a simple command that should work on all platforms
	stdout, stderr, err := shell.Exec(ctx, "echo hello")
	if err != nil {
		t.Fatalf("Echo command failed: %v, stderr: %s", err, stderr)
	}

	if stdout == "" {
		t.Error("Echo command produced no output")
	}

	// The output should contain "hello" regardless of platform
	if !strings.Contains(strings.ToLower(stdout), "hello") {
		t.Errorf("Echo output should contain 'hello', got: %q", stdout)
	}
}

func TestShell_SetGetWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewShell(&Options{WorkingDir: t.TempDir()})

	require.NoError(t, s.SetWorkingDir(tmpDir))
	assert.Equal(t, tmpDir, s.GetWorkingDir())
}

func TestShell_SetWorkingDir_NonExistent(t *testing.T) {
	s := NewShell(&Options{WorkingDir: t.TempDir()})

	err := s.SetWorkingDir("/nonexistent/path/that/does/not/exist")
	require.Error(t, err, "SetWorkingDir should fail for a non-existent directory")
	assert.Contains(t, err.Error(), "directory does not exist")
}

func TestShell_SetGetEnv(t *testing.T) {
	s := NewShell(&Options{
		WorkingDir: t.TempDir(),
		Env:        []string{"EXISTING=value"},
	})

	s.SetEnv("MY_VAR", "hello")
	s.SetEnv("OTHER_VAR", "world")

	env := s.GetEnv()

	// Check that the new vars are present.
	assert.Contains(t, env, "MY_VAR=hello")
	assert.Contains(t, env, "OTHER_VAR=world")

	// Check that SetEnv updates (not duplicates) existing keys.
	s.SetEnv("MY_VAR", "updated")
	env = s.GetEnv()
	assert.Contains(t, env, "MY_VAR=updated")

	// Ensure there is exactly one MY_VAR entry.
	count := 0
	for _, e := range env {
		if strings.HasPrefix(e, "MY_VAR=") {
			count++
		}
	}
	assert.Equal(t, 1, count, "MY_VAR should appear exactly once after update")
}

func TestShell_GetEnv_ReturnsCopy(t *testing.T) {
	s := NewShell(&Options{
		WorkingDir: t.TempDir(),
		Env:        []string{"A=1"},
	})

	env1 := s.GetEnv()
	env1[0] = "MUTATED=yes"

	env2 := s.GetEnv()
	// Mutating the returned slice should not affect the shell's internal state.
	assert.NotEqual(t, "MUTATED=yes", env2[0], "GetEnv should return a defensive copy")
}

func TestShell_Exec_SimpleCommand(t *testing.T) {
	s := NewShell(&Options{WorkingDir: t.TempDir()})

	stdout, stderr, err := s.Exec(t.Context(), "echo hello")
	require.NoError(t, err)
	assert.Contains(t, stdout, "hello")
	assert.Empty(t, stderr)
}

func TestShell_ExitCode(t *testing.T) {
	t.Run("nil error returns 0", func(t *testing.T) {
		assert.Equal(t, 0, ExitCode(nil))
	})

	t.Run("generic error returns 1", func(t *testing.T) {
		assert.Equal(t, 1, ExitCode(fmt.Errorf("something failed")))
	})

	t.Run("command with non-zero exit", func(t *testing.T) {
		s := NewShell(&Options{WorkingDir: t.TempDir()})
		_, _, err := s.Exec(t.Context(), "exit 42")
		require.Error(t, err)
		assert.Equal(t, 42, ExitCode(err))
	})

	t.Run("command with exit 0", func(t *testing.T) {
		s := NewShell(&Options{WorkingDir: t.TempDir()})
		_, _, err := s.Exec(t.Context(), "exit 0")
		// exit 0 should not produce an error
		assert.NoError(t, err)
		assert.Equal(t, 0, ExitCode(err))
	})
}

func TestShell_IsInterrupt(t *testing.T) {
	t.Run("context.Canceled", func(t *testing.T) {
		assert.True(t, IsInterrupt(context.Canceled))
	})

	t.Run("context.DeadlineExceeded", func(t *testing.T) {
		assert.True(t, IsInterrupt(context.DeadlineExceeded))
	})

	t.Run("wrapped canceled error", func(t *testing.T) {
		wrapped := fmt.Errorf("operation failed: %w", context.Canceled)
		assert.True(t, IsInterrupt(wrapped))
	})

	t.Run("generic error is not interrupt", func(t *testing.T) {
		assert.False(t, IsInterrupt(errors.New("some error")))
	})

	t.Run("nil is not interrupt", func(t *testing.T) {
		assert.False(t, IsInterrupt(nil))
	})
}
