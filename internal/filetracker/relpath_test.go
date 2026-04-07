package filetracker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelpath(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Absolute path under cwd is made relative.
	abs := filepath.Join(cwd, "sub", "file.go")
	assert.Equal(t, filepath.Join("sub", "file.go"), relpath(abs))

	// Already-relative path is cleaned but returned as-is.
	assert.Equal(t, filepath.Join("foo", "bar.go"), relpath("foo/bar.go"))

	// Path outside cwd uses .. prefix.
	outside := filepath.Join(cwd, "..", "other", "file.go")
	result := relpath(outside)
	assert.NotEmpty(t, result)
	assert.False(t, filepath.IsAbs(result), "result should be relative")
}

func TestService_ListReadFiles(t *testing.T) {
	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	svc := NewService(q)

	sessionID := "list-session"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "List Test",
	})
	require.NoError(t, err)

	// No files read yet.
	files, err := svc.ListReadFiles(t.Context(), sessionID)
	require.NoError(t, err)
	assert.Empty(t, files)

	// Record two reads and verify they are listed.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	svc.RecordRead(t.Context(), sessionID, filepath.Join(cwd, "a.go"))
	svc.RecordRead(t.Context(), sessionID, filepath.Join(cwd, "b.go"))

	files, err = svc.ListReadFiles(t.Context(), sessionID)
	require.NoError(t, err)
	require.Len(t, files, 2)

	// Returned paths should be absolute (joined back to cwd).
	for _, f := range files {
		assert.True(t, filepath.IsAbs(f), "expected absolute path, got %s", f)
	}
}

func TestService_ListReadFiles_EmptySession(t *testing.T) {
	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	svc := NewService(q)

	sessionID := "empty-session"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "Empty",
	})
	require.NoError(t, err)

	files, err := svc.ListReadFiles(t.Context(), sessionID)
	require.NoError(t, err)
	assert.Empty(t, files)
}
