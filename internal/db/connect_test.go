package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDBPath(t *testing.T) {
	t.Run("prefers smithers db when present", func(t *testing.T) {
		dataDir := t.TempDir()
		primary := filepath.Join(dataDir, "smithers-tui.db")
		legacy := filepath.Join(dataDir, "crush.db")

		require.NoError(t, os.WriteFile(primary, nil, 0o644))
		require.NoError(t, os.WriteFile(legacy, nil, 0o644))

		require.Equal(t, primary, resolveDBPath(dataDir))
	})

	t.Run("falls back to crush db when smithers db is absent", func(t *testing.T) {
		dataDir := t.TempDir()
		legacy := filepath.Join(dataDir, "crush.db")

		require.NoError(t, os.WriteFile(legacy, nil, 0o644))

		require.Equal(t, legacy, resolveDBPath(dataDir))
	})
}

func TestConnect_CreatesDatabase(t *testing.T) {
	dataDir := t.TempDir()
	ctx := t.Context()

	db, err := Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	dbPath := filepath.Join(dataDir, "smithers-tui.db")
	assert.FileExists(t, dbPath, "database file should be created after Connect")
}

func TestConnect_RunsMigrations(t *testing.T) {
	dataDir := t.TempDir()
	ctx := t.Context()

	db, err := Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// The initial migration creates the "sessions" table. Query it to verify
	// migrations ran successfully.
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next(), "sessions table should exist after migrations")

	var tableName string
	require.NoError(t, rows.Scan(&tableName))
	assert.Equal(t, "sessions", tableName)

	// Also verify the messages table exists (part of the same initial migration).
	rows2, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='messages'")
	require.NoError(t, err)
	defer rows2.Close()

	require.True(t, rows2.Next(), "messages table should exist after migrations")
}

func TestConnect_MultipleCalls(t *testing.T) {
	dataDir := t.TempDir()
	ctx := t.Context()

	db1, err := Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { db1.Close() })

	// A second connection to the same directory should succeed (WAL mode allows it).
	db2, err := Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { db2.Close() })

	// Both connections should be usable.
	var count1 int
	require.NoError(t, db1.QueryRowContext(ctx, "SELECT count(*) FROM sessions").Scan(&count1))
	assert.Equal(t, 0, count1)

	var count2 int
	require.NoError(t, db2.QueryRowContext(ctx, "SELECT count(*) FROM sessions").Scan(&count2))
	assert.Equal(t, 0, count2)
}

func TestConnect_EmptyDataDir(t *testing.T) {
	_, err := Connect(t.Context(), "")
	require.Error(t, err, "Connect with empty dataDir should return an error")
	assert.Contains(t, err.Error(), "data.dir is not set")
}
