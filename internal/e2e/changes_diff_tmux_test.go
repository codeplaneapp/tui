package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type tmuxSession struct {
	name string
	t    *testing.T
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

func launchTmuxSession(t *testing.T, binary, workingDir, configDir, dataDir, fakeBin string) *tmuxSession {
	return launchTmuxSessionWithEnv(t, binary, workingDir, configDir, dataDir, fakeBin, nil)
}

func launchTmuxSessionWithEnv(
	t *testing.T,
	binary,
	workingDir,
	configDir,
	dataDir,
	fakeBin string,
	extraEnv map[string]string,
) *tmuxSession {
	t.Helper()

	session := fmt.Sprintf("crush-changes-%d", time.Now().UnixNano())
	scriptPath := filepath.Join(t.TempDir(), "launch.sh")
	var envBuilder strings.Builder
	for key, value := range extraEnv {
		envBuilder.WriteString(fmt.Sprintf("export %s=%q\n", key, value))
	}
	script := fmt.Sprintf(`#!/bin/sh
cd %q
export TERM=xterm-256color
export COLORTERM=truecolor
export LANG=en_US.UTF-8
export CRUSH_GLOBAL_CONFIG=%q
export CRUSH_GLOBAL_DATA=%q
export PATH=%q
%s
exec %q
`, workingDir, configDir, dataDir, fakeBin+":/usr/bin:/bin", envBuilder.String(), binary)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	cmd := exec.Command("tmux", "new-session", "-d", "-s", session, "-x", "120", "-y", "40", scriptPath)
	require.NoError(t, cmd.Run())

	return &tmuxSession{name: session, t: t}
}

func (s *tmuxSession) SendKeys(keys ...string) {
	s.t.Helper()
	args := append([]string{"send-keys", "-t", s.name}, keys...)
	require.NoError(s.t, exec.Command("tmux", args...).Run())
}

func (s *tmuxSession) Capture() string {
	s.t.Helper()
	out, err := exec.Command("tmux", "capture-pane", "-t", s.name, "-p").CombinedOutput()
	require.NoError(s.t, err)
	return strings.ReplaceAll(string(out), "\r", "")
}

func (s *tmuxSession) WaitForText(text string, timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.Capture(), text) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("waitForText: %q not found within %s\nPane:\n%s", text, timeout, s.Capture())
}

func (s *tmuxSession) WaitForNoText(text string, timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !strings.Contains(s.Capture(), text) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("waitForNoText: %q still present after %s\nPane:\n%s", text, timeout, s.Capture())
}

func (s *tmuxSession) Close() {
	s.t.Helper()
	_ = exec.Command("tmux", "kill-session", "-t", s.name).Run()
}

func buildTUIBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "smithers-tui")
	cmd := exec.Command("go", "build", "-o", binary, "./main.go")
	cmd.Dir = repoRoot(t)
	require.NoError(t, cmd.Run())
	return binary
}

func writeFakeJJHub(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	jjhubPath := filepath.Join(binDir, "jjhub")
	jjhubScript := `#!/bin/sh
case "$1 $2" in
  "change list")
    cat <<'EOF'
[{"change_id":"abc123","commit_id":"deadbeef12345678","description":"Test change for tmux e2e","author":{"name":"Test User","email":"test@example.com"},"timestamp":"2025-01-15T12:34:56Z","is_empty":false,"is_working_copy":true,"bookmarks":["main"]}]
EOF
    ;;
  "change diff")
    cat <<'EOF'
diff --git a/example.txt b/example.txt
index 1111111..2222222 100644
--- a/example.txt
+++ b/example.txt
@@ -1 +1 @@
-before
+after
EOF
    ;;
  "land list"|"issue list"|"workspace list")
    printf '[]\n'
    ;;
  "repo view")
    cat <<'EOF'
{"name":"demo","full_name":"demo/repo"}
EOF
    ;;
  *)
    printf '[]\n'
    ;;
esac
`
	require.NoError(t, os.WriteFile(jjhubPath, []byte(jjhubScript), 0o755))

	jjPath := filepath.Join(binDir, "jj")
	jjScript := `#!/bin/sh
if [ "$1" = "diff" ]; then
  cat <<'EOF'
diff --git a/example.txt b/example.txt
index 1111111..2222222 100644
--- a/example.txt
+++ b/example.txt
@@ -1 +1 @@
-before
+after
EOF
  exit 0
fi

printf 'unsupported jj invocation: %s\n' "$*" >&2
exit 1
`
	require.NoError(t, os.WriteFile(jjPath, []byte(jjScript), 0o755))
	return binDir
}

func writeFakeDiffnav(t *testing.T, binDir string) (string, string) {
	t.Helper()

	argsFile := filepath.Join(binDir, "diffnav-args.txt")
	stdinFile := filepath.Join(binDir, "diffnav-stdin.patch")
	scriptPath := filepath.Join(binDir, "diffnav")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat > %q
`, argsFile, stdinFile)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return argsFile, stdinFile
}

func writeFakeFailingDiffnav(t *testing.T, binDir string) {
	t.Helper()

	scriptPath := filepath.Join(binDir, "diffnav")
	script := `#!/bin/sh
cat >/dev/null
printf 'Caught panic: divide by zero\n' >&2
exit 1
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
}

func writeFakePager(t *testing.T, binDir string) (string, string, string) {
	t.Helper()

	pathFile := filepath.Join(binDir, "pager-path.txt")
	contentFile := filepath.Join(binDir, "pager-content.diff")
	scriptPath := filepath.Join(binDir, "fake-pager")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$1" > %q
cat "$1" > %q
`, pathFile, contentFile)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath, pathFile, contentFile
}

func TestChangesView_DiffPromptAndEscape_TmuxE2E(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") != "1" {
		t.Skip("set CRUSH_TUI_E2E=1 to run terminal E2E tests")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for this e2e test")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "options": {
    "disable_default_providers": true
  },
  "providers": {
    "test": {
      "api_key": "test-key",
      "base_url": "https://example.invalid/v1",
      "models": [
        {
          "id": "test-model"
        }
      ]
    }
  },
  "models": {
    "large": {
      "provider": "test",
      "model": "test-model"
    },
    "small": {
      "provider": "test",
      "model": "test-model"
    }
  },
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	binary := buildTUIBinary(t)
	fakeBin := writeFakeJJHub(t)
	workingDir := repoRoot(t)

	session := launchTmuxSession(t, binary, workingDir, configDir, dataDir, fakeBin)
	defer session.Close()

	session.WaitForText("SMITHERS", 15*time.Second)
	session.SendKeys("6")
	time.Sleep(300 * time.Millisecond)
	session.SendKeys("Enter")
	session.WaitForText("JJHub › Changes", 10*time.Second)
	session.WaitForText("Test change for tmux e2e", 10*time.Second)

	session.SendKeys("d")
	session.WaitForText("diffnav not installed", 5*time.Second)

	session.SendKeys("Escape")
	session.WaitForNoText("JJHub › Changes", 5*time.Second)
	session.WaitForText("SMITHERS", 5*time.Second)
}

func TestChangesView_InstalledDiffnavUsesStdin_TmuxE2E(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") != "1" {
		t.Skip("set CRUSH_TUI_E2E=1 to run terminal E2E tests")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for this e2e test")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "options": {
    "disable_default_providers": true
  },
  "providers": {
    "test": {
      "api_key": "test-key",
      "base_url": "https://example.invalid/v1",
      "models": [
        {
          "id": "test-model"
        }
      ]
    }
  },
  "models": {
    "large": {
      "provider": "test",
      "model": "test-model"
    },
    "small": {
      "provider": "test",
      "model": "test-model"
    }
  },
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	binary := buildTUIBinary(t)
	fakeBin := writeFakeJJHub(t)
	argsFile, stdinFile := writeFakeDiffnav(t, fakeBin)
	workingDir := repoRoot(t)

	session := launchTmuxSession(t, binary, workingDir, configDir, dataDir, fakeBin)
	defer session.Close()

	session.WaitForText("SMITHERS", 15*time.Second)
	session.SendKeys("6")
	time.Sleep(300 * time.Millisecond)
	session.SendKeys("Enter")
	session.WaitForText("JJHub › Changes", 10*time.Second)
	session.WaitForText("Test change for tmux e2e", 10*time.Second)
	session.SendKeys("d")

	require.Eventually(t, func() bool {
		_, err := os.Stat(stdinFile)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	require.Equal(t, "", strings.TrimSpace(string(args)))

	patch, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	require.Contains(t, string(patch), "diff --git a/example.txt b/example.txt")
	require.Contains(t, string(patch), "+after")
}

func TestChangesView_DiffnavFailureFallsBackToPager_TmuxE2E(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") != "1" {
		t.Skip("set CRUSH_TUI_E2E=1 to run terminal E2E tests")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for this e2e test")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "options": {
    "disable_default_providers": true
  },
  "providers": {
    "test": {
      "api_key": "test-key",
      "base_url": "https://example.invalid/v1",
      "models": [
        {
          "id": "test-model"
        }
      ]
    }
  },
  "models": {
    "large": {
      "provider": "test",
      "model": "test-model"
    },
    "small": {
      "provider": "test",
      "model": "test-model"
    }
  },
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	binary := buildTUIBinary(t)
	fakeBin := writeFakeJJHub(t)
	writeFakeFailingDiffnav(t, fakeBin)
	pagerPath, pagerPathFile, pagerContentFile := writeFakePager(t, fakeBin)
	workingDir := repoRoot(t)

	session := launchTmuxSessionWithEnv(t, binary, workingDir, configDir, dataDir, fakeBin, map[string]string{
		"PAGER": pagerPath,
	})
	defer session.Close()

	session.WaitForText("SMITHERS", 15*time.Second)
	session.SendKeys("6")
	time.Sleep(300 * time.Millisecond)
	session.SendKeys("Enter")
	session.WaitForText("JJHub › Changes", 10*time.Second)
	session.WaitForText("Test change for tmux e2e", 10*time.Second)
	session.SendKeys("d")

	require.Eventually(t, func() bool {
		_, err := os.Stat(pagerContentFile)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	content, err := os.ReadFile(pagerContentFile)
	require.NoError(t, err)
	require.Contains(t, string(content), "diff --git a/example.txt b/example.txt")
	require.Contains(t, string(content), "+after")

	pathBytes, err := os.ReadFile(pagerPathFile)
	require.NoError(t, err)
	diffPath := strings.TrimSpace(string(pathBytes))
	require.NotEmpty(t, diffPath)

	require.Eventually(t, func() bool {
		_, err := os.Stat(diffPath)
		return os.IsNotExist(err)
	}, 5*time.Second, 100*time.Millisecond)

	session.WaitForText("JJHub › Changes", 5*time.Second)
}
