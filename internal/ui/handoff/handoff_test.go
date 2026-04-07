package handoff

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// buildCmd
// ---------------------------------------------------------------------------

func TestBuildCmd_ValidBinary(t *testing.T) {
	t.Parallel()

	// Use the current test binary path itself as a guaranteed-present executable.
	self, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Skip("cannot resolve own executable:", err)
	}

	cmd, err := buildCmd(self, []string{"--help"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path != self {
		t.Errorf("cmd.Path = %q, want %q", cmd.Path, self)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "--help" {
		t.Errorf("cmd.Args = %v, want [%s --help]", cmd.Args, self)
	}
}

func TestBuildCmd_InvalidCwd(t *testing.T) {
	t.Parallel()

	self, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Skip("cannot resolve own executable:", err)
	}

	_, err = buildCmd(self, nil, "/this/path/does/not/exist/ever", nil)
	if err == nil {
		t.Fatal("expected error for non-existent working directory, got nil")
	}
}

func TestBuildCmd_ValidCwd(t *testing.T) {
	t.Parallel()

	self, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Skip("cannot resolve own executable:", err)
	}

	tmp := t.TempDir()
	cmd, err := buildCmd(self, nil, tmp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Dir != tmp {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, tmp)
	}
}

func TestBuildCmd_EnvMerge(t *testing.T) {
	t.Parallel()

	self, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Skip("cannot resolve own executable:", err)
	}

	overrides := []string{
		"MY_CUSTOM_KEY=hello",
		"PATH=/custom/path",
	}
	cmd, err := buildCmd(self, nil, "", overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envMap := make(map[string]string, len(cmd.Env))
	for _, entry := range cmd.Env {
		k, v, ok := splitEnvEntry(entry)
		if ok {
			envMap[k] = v
		}
	}

	if envMap["MY_CUSTOM_KEY"] != "hello" {
		t.Errorf("MY_CUSTOM_KEY = %q, want %q", envMap["MY_CUSTOM_KEY"], "hello")
	}
	if envMap["PATH"] != "/custom/path" {
		t.Errorf("PATH = %q, want %q", envMap["PATH"], "/custom/path")
	}
}

func TestBuildCmd_NoEnvOverride_InheritsParent(t *testing.T) {
	// t.Setenv modifies the process environment so this test must not run in
	// parallel with others that also rely on os.Environ().
	const sentinel = "HANDOFF_TEST_SENTINEL"
	t.Setenv(sentinel, "present")

	self, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Skip("cannot resolve own executable:", err)
	}

	cmd, err := buildCmd(self, nil, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, sentinel+"=") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected parent env var %q to be inherited", sentinel)
	}
}

// ---------------------------------------------------------------------------
// mergeEnv
// ---------------------------------------------------------------------------

func TestMergeEnv_Override(t *testing.T) {
	t.Parallel()

	base := []string{"FOO=old", "BAR=keep"}
	result := mergeEnv(base, []string{"FOO=new"})

	got := make(map[string]string)
	for _, e := range result {
		k, v, _ := splitEnvEntry(e)
		got[k] = v
	}

	if got["FOO"] != "new" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "new")
	}
	if got["BAR"] != "keep" {
		t.Errorf("BAR = %q, want %q", got["BAR"], "keep")
	}
}

func TestMergeEnv_Append(t *testing.T) {
	t.Parallel()

	base := []string{"FOO=1"}
	result := mergeEnv(base, []string{"NEW_KEY=42"})

	got := make(map[string]string)
	for _, e := range result {
		k, v, _ := splitEnvEntry(e)
		got[k] = v
	}

	if got["NEW_KEY"] != "42" {
		t.Errorf("NEW_KEY = %q, want %q", got["NEW_KEY"], "42")
	}
	if got["FOO"] != "1" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "1")
	}
}

func TestMergeEnv_NoMutation(t *testing.T) {
	t.Parallel()

	base := []string{"A=1", "B=2"}
	original := make([]string, len(base))
	copy(original, base)

	_ = mergeEnv(base, []string{"A=99"})

	for i, v := range base {
		if v != original[i] {
			t.Errorf("mergeEnv mutated base[%d]: got %q, want %q", i, v, original[i])
		}
	}
}

func TestMergeEnv_EmptyOverrides(t *testing.T) {
	t.Parallel()

	base := []string{"X=1"}
	result := mergeEnv(base, nil)
	if len(result) != len(base) {
		t.Errorf("len(result) = %d, want %d", len(result), len(base))
	}
}

// ---------------------------------------------------------------------------
// splitEnvEntry
// ---------------------------------------------------------------------------

func TestSplitEnvEntry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		wantKey  string
		wantVal  string
		wantOK   bool
	}{
		{"KEY=VALUE", "KEY", "VALUE", true},
		{"KEY=", "KEY", "", true},
		{"KEY=a=b", "KEY", "a=b", true}, // value may contain '='
		{"NOEQUALS", "", "", false},
		{"", "", "", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			k, v, ok := splitEnvEntry(tc.input)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if k != tc.wantKey {
				t.Errorf("key = %q, want %q", k, tc.wantKey)
			}
			if v != tc.wantVal {
				t.Errorf("val = %q, want %q", v, tc.wantVal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// exitCodeFromError
// ---------------------------------------------------------------------------

func TestExitCodeFromError_Nil(t *testing.T) {
	t.Parallel()

	if code := exitCodeFromError(nil); code != 0 {
		t.Errorf("exitCodeFromError(nil) = %d, want 0", code)
	}
}

func TestExitCodeFromError_ExitError(t *testing.T) {
	t.Parallel()

	// Run a process that exits with a known non-zero code.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", "exit 42")
	} else {
		cmd = exec.Command("sh", "-c", "exit 42")
	}
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}

	code := exitCodeFromError(err)
	if code != 42 {
		t.Errorf("exitCodeFromError = %d, want 42", code)
	}
}

func TestExitCodeFromError_GenericError(t *testing.T) {
	t.Parallel()

	err := errors.New("something went wrong")
	if code := exitCodeFromError(err); code != 1 {
		t.Errorf("exitCodeFromError(generic) = %d, want 1", code)
	}
}

// ---------------------------------------------------------------------------
// HandoffResult state
// ---------------------------------------------------------------------------

func TestHandoffResult_ZeroValue(t *testing.T) {
	t.Parallel()

	var r HandoffResult
	if r.ExitCode != 0 {
		t.Errorf("zero ExitCode = %d, want 0", r.ExitCode)
	}
	if r.Err != nil {
		t.Errorf("zero Err = %v, want nil", r.Err)
	}
	if r.Duration != 0 {
		t.Errorf("zero Duration = %v, want 0", r.Duration)
	}
}

func TestHandoffResult_NonZeroFields(t *testing.T) {
	t.Parallel()

	r := HandoffResult{
		ExitCode: 2,
		Err:      errors.New("exit status 2"),
		Duration: 500 * time.Millisecond,
	}
	if r.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", r.ExitCode)
	}
	if r.Err == nil {
		t.Error("Err should be non-nil")
	}
	if r.Duration != 500*time.Millisecond {
		t.Errorf("Duration = %v, want 500ms", r.Duration)
	}
}

// ---------------------------------------------------------------------------
// HandoffMsg Tag round-trip
// ---------------------------------------------------------------------------

func TestHandoffMsg_TagRoundTrip(t *testing.T) {
	t.Parallel()

	type myTag struct{ id int }
	tag := myTag{id: 7}

	msg := HandoffMsg{Tag: tag, Result: HandoffResult{ExitCode: 0}}
	got, ok := msg.Tag.(myTag)
	if !ok {
		t.Fatalf("tag type assertion failed")
	}
	if got.id != 7 {
		t.Errorf("tag.id = %d, want 7", got.id)
	}
}

// ---------------------------------------------------------------------------
// Handoff — pre-flight validation (empty binary, unknown binary)
// ---------------------------------------------------------------------------

func TestHandoff_EmptyBinary(t *testing.T) {
	t.Parallel()

	cmd := Handoff(Options{Binary: "", Tag: "test"})
	msg := cmd()

	hm, ok := msg.(HandoffMsg)
	if !ok {
		t.Fatalf("expected HandoffMsg, got %T", msg)
	}
	if hm.Result.ExitCode == 0 {
		t.Error("expected non-zero exit code for empty binary")
	}
	if hm.Result.Err == nil {
		t.Error("expected non-nil error for empty binary")
	}
	if hm.Tag != "test" {
		t.Errorf("Tag = %v, want %q", hm.Tag, "test")
	}
}

func TestHandoff_UnknownBinary(t *testing.T) {
	t.Parallel()

	cmd := Handoff(Options{
		Binary: "this-binary-certainly-does-not-exist-on-any-sane-system",
		Tag:    42,
	})
	msg := cmd()

	hm, ok := msg.(HandoffMsg)
	if !ok {
		t.Fatalf("expected HandoffMsg, got %T", msg)
	}
	if hm.Result.ExitCode == 0 {
		t.Error("expected non-zero exit code for unknown binary")
	}
	if hm.Result.Err == nil {
		t.Error("expected non-nil error for unknown binary")
	}
	if hm.Tag != 42 {
		t.Errorf("Tag = %v, want 42", hm.Tag)
	}
}

// ---------------------------------------------------------------------------
// HandoffWithCallback — pre-flight validation
// ---------------------------------------------------------------------------

func TestHandoffWithCallback_EmptyBinary(t *testing.T) {
	t.Parallel()

	// tea.ExecCallback is func(error) tea.Msg.  tea.Msg is `any`, so we can
	// use a plain func(error) any here without importing bubbletea in tests.
	type sentinel struct{}
	errReceived := make(chan error, 1)

	var cb tea.ExecCallback = func(err error) tea.Msg {
		errReceived <- err
		return sentinel{}
	}

	resultCmd := HandoffWithCallback(Options{Binary: ""}, cb)

	// Invoking the tea.Cmd must synchronously produce a message.  For the
	// error path the cmd is a plain function, not a tea.ExecProcess, so it is
	// safe to call here without a running Program.
	_ = resultCmd()

	select {
	case err := <-errReceived:
		if err == nil {
			t.Error("expected non-nil error in callback")
		}
	default:
		// Callback was not fired — the empty-binary guard returned a msg
		// directly.  Check that the returned msg was a sentinel (i.e., the
		// callback was called).
		t.Error("callback was not called for empty binary")
	}
}

// ---------------------------------------------------------------------------
// Options — Env and Cwd propagation via buildCmd (integration-style)
// ---------------------------------------------------------------------------

func TestHandoff_InvalidCwd(t *testing.T) {
	t.Parallel()

	// Use "true" (or "cmd /C exit 0" on Windows) as a valid binary, but
	// supply a non-existent cwd.  LookPath will succeed but buildCmd should
	// fail.
	var binary string
	if runtime.GOOS == "windows" {
		binary = "cmd"
	} else {
		binary = "true"
	}

	path, err := exec.LookPath(binary)
	if err != nil {
		t.Skip("binary not found:", err)
	}

	_, buildErr := buildCmd(path, nil, "/no/such/directory/ever", nil)
	if buildErr == nil {
		t.Error("expected error for non-existent working directory, got nil")
	}
}

// TestHandoff_CwdAbsolutePath verifies that when cwd is a temp directory,
// the resulting *exec.Cmd has cmd.Dir set correctly.
func TestHandoff_CwdAbsolutePath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	nested := filepath.Join(tmp, "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var binary string
	if runtime.GOOS == "windows" {
		binary = "cmd"
	} else {
		binary = "true"
	}

	path, err := exec.LookPath(binary)
	if err != nil {
		t.Skip("binary not found:", err)
	}

	cmd, err := buildCmd(path, nil, nested, nil)
	if err != nil {
		t.Fatalf("buildCmd: %v", err)
	}
	if cmd.Dir != nested {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, nested)
	}
}

// ---------------------------------------------------------------------------
// mergeEnv — malformed entries
// ---------------------------------------------------------------------------

func TestMergeEnv_MalformedOverride(t *testing.T) {
	t.Parallel()

	// An override without '=' should be appended as-is so the caller can debug.
	base := []string{"FOO=1"}
	result := mergeEnv(base, []string{"NOEQUALS"})

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[1] != "NOEQUALS" {
		t.Errorf("result[1] = %q, want %q", result[1], "NOEQUALS")
	}
}

func TestMergeEnv_DuplicateOverrides(t *testing.T) {
	t.Parallel()

	// When the same key appears twice in overrides, the last one wins.
	base := []string{"K=old"}
	result := mergeEnv(base, []string{"K=first", "K=second"})

	got := make(map[string]string)
	for _, e := range result {
		k, v, _ := splitEnvEntry(e)
		got[k] = v
	}
	if got["K"] != "second" {
		t.Errorf("K = %q, want %q", got["K"], "second")
	}
}

// ---------------------------------------------------------------------------
// HandoffWithCallback — unknown binary
// ---------------------------------------------------------------------------

func TestHandoffWithCallback_UnknownBinary(t *testing.T) {
	t.Parallel()

	errReceived := make(chan error, 1)
	cb := func(err error) tea.Msg {
		errReceived <- err
		return nil
	}

	cmd := HandoffWithCallback(Options{
		Binary: "this-binary-certainly-does-not-exist",
	}, cb)

	_ = cmd()

	select {
	case err := <-errReceived:
		if err == nil {
			t.Error("expected non-nil error in callback for unknown binary")
		}
	default:
		t.Error("callback was not called for unknown binary")
	}
}
