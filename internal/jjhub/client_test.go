package jjhub

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeBin creates a fake shell script named `name` in a temp dir and
// prepends it to PATH so exec.Command finds it instead of the real binary.
func writeFakeBin(t *testing.T, name, script string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake binary helper uses a POSIX shell script")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeFakeJJHub installs a fake jjhub binary.
func writeFakeJJHub(t *testing.T, script string) {
	t.Helper()
	writeFakeBin(t, "jjhub", script)
}

// writeFakeJJ installs a fake jj binary.
func writeFakeJJ(t *testing.T, script string) {
	t.Helper()
	writeFakeBin(t, "jj", script)
}

// ---- repoArgs tests ----

func TestClient_RepoArgs_WithRepo(t *testing.T) {
	c := NewClient("owner/repo")
	args := c.repoArgs()
	if len(args) != 2 || args[0] != "-R" || args[1] != "owner/repo" {
		t.Fatalf("expected [-R owner/repo], got %v", args)
	}
}

func TestClient_RepoArgs_Empty(t *testing.T) {
	c := NewClient("")
	args := c.repoArgs()
	if args != nil {
		t.Fatalf("expected nil, got %v", args)
	}
}

// ---- GetCurrentRepo repo-scoping tests ----

func TestGetCurrentRepo_PassesRepoFlag(t *testing.T) {
	// Fake jjhub that prints its arguments so we can verify -R is passed.
	writeFakeJJHub(t, `#!/bin/sh
# Echo args as JSON-encoded repo whose FullName is the raw arg list.
# This lets us inspect what flags were passed.
ARGS="$*"
	printf '{"full_name":"%s"}' "$ARGS"
`)

	c := NewClient("owner/repo")
	repo, err := c.GetCurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(repo.FullName, "-R") {
		t.Errorf("expected -R flag in args, got: %s", repo.FullName)
	}
	if !strings.Contains(repo.FullName, "owner/repo") {
		t.Errorf("expected owner/repo in args, got: %s", repo.FullName)
	}
}

func TestGetCurrentRepo_NoRepoFlag_WhenEmpty(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
ARGS="$*"
	printf '{"full_name":"%s"}' "$ARGS"
`)

	c := NewClient("")
	repo, err := c.GetCurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(repo.FullName, "-R") {
		t.Errorf("expected no -R flag for empty repo, got: %s", repo.FullName)
	}
}

// ---- WorkingCopyDiff repo-scoping tests ----

func TestWorkingCopyDiff_PassesRepoFlag(t *testing.T) {
	// Fake jj that prints its arguments so we can verify -R is passed.
	writeFakeJJ(t, `#!/bin/sh
echo "$*"
`)

	c := NewClient("owner/repo")
	out, err := c.WorkingCopyDiff(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "-R") {
		t.Errorf("expected -R flag in jj args, got: %s", strings.TrimSpace(out))
	}
	if !strings.Contains(out, "owner/repo") {
		t.Errorf("expected owner/repo in jj args, got: %s", strings.TrimSpace(out))
	}
}

func TestWorkingCopyDiff_NoRepoFlag_WhenEmpty(t *testing.T) {
	writeFakeJJ(t, `#!/bin/sh
echo "$*"
`)

	c := NewClient("")
	out, err := c.WorkingCopyDiff(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "-R") {
		t.Errorf("expected no -R flag for empty repo, got: %s", strings.TrimSpace(out))
	}
}

// ---- ListChanges repo-scoping tests ----

func TestListChanges_PassesRepoFlag(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
ARGS="$*"
# Return valid JSON array with the args embedded in the description.
printf '[{"change_id":"test","description":"%s"}]' "$ARGS"
`)

	c := NewClient("owner/repo")
	changes, err := c.ListChanges(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one change")
	}
	if !strings.Contains(changes[0].Description, "-R") {
		t.Errorf("expected -R flag in args, got: %s", changes[0].Description)
	}
	if !strings.Contains(changes[0].Description, "owner/repo") {
		t.Errorf("expected owner/repo in args, got: %s", changes[0].Description)
	}
}

func TestListChanges_NoRepoFlag_WhenEmpty(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
ARGS="$*"
printf '[{"change_id":"test","description":"%s"}]' "$ARGS"
`)

	c := NewClient("")
	changes, err := c.ListChanges(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one change")
	}
	if strings.Contains(changes[0].Description, "-R") {
		t.Errorf("expected no -R flag for empty repo, got: %s", changes[0].Description)
	}
}

// ---- ChangeDiff repo-scoping tests ----

func TestChangeDiff_PassesRepoFlag(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
echo "$*"
`)

	c := NewClient("owner/repo")
	out, err := c.ChangeDiff(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "-R") {
		t.Errorf("expected -R flag in args, got: %s", strings.TrimSpace(out))
	}
	if !strings.Contains(out, "owner/repo") {
		t.Errorf("expected owner/repo in args, got: %s", strings.TrimSpace(out))
	}
}

func TestChangeDiff_NoRepoFlag_WhenEmpty(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
echo "$*"
`)

	c := NewClient("")
	out, err := c.ChangeDiff(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "-R") {
		t.Errorf("expected no -R flag for empty repo, got: %s", strings.TrimSpace(out))
	}
}

// ---- Verify already-correct methods still pass -R ----

func TestListLandings_PassesRepoFlag(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
ARGS="$*"
printf '[{"number":1,"title":"%s"}]' "$ARGS"
`)

	c := NewClient("owner/repo")
	landings, err := c.ListLandings(context.Background(), "open", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(landings) == 0 {
		t.Fatal("expected at least one landing")
	}
	if !strings.Contains(landings[0].Title, "-R") || !strings.Contains(landings[0].Title, "owner/repo") {
		t.Errorf("expected -R owner/repo in args, got: %s", landings[0].Title)
	}
}

func TestViewLanding_PassesRepoFlag(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
ARGS="$*"
printf '{"landing":{"number":42,"title":"%s"},"changes":[],"conflicts":{},"reviews":[]}' "$ARGS"
`)

	c := NewClient("owner/repo")
	detail, err := c.ViewLanding(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(detail.Landing.Title, "-R") || !strings.Contains(detail.Landing.Title, "owner/repo") {
		t.Errorf("expected -R owner/repo in args, got: %s", detail.Landing.Title)
	}
}

func TestLandingDiff_PassesRepoFlag(t *testing.T) {
	writeFakeJJHub(t, `#!/bin/sh
if [ "$1" = "land" ] && [ "$2" = "view" ]; then
	printf '{"landing":{"number":42,"title":"landing"},"changes":[{"change_id":"abc123"}],"conflicts":{},"reviews":[]}'
	exit 0
fi
echo "$*"
`)

	c := NewClient("owner/repo")
	out, err := c.LandingDiff(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "-R") || !strings.Contains(out, "owner/repo") {
		t.Errorf("expected -R owner/repo in args, got: %s", strings.TrimSpace(out))
	}
}
