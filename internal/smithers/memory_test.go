package smithers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ListAllMemoryFacts ---

func TestListAllMemoryFacts_SQLite(t *testing.T) {
	// Open an in-memory SQLite database and seed it with facts.
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE _smithers_memory_facts (
		namespace     TEXT    NOT NULL,
		key           TEXT    NOT NULL,
		value_json    TEXT    NOT NULL,
		schema_sig    TEXT    NOT NULL DEFAULT '',
		created_at_ms INTEGER NOT NULL DEFAULT 0,
		updated_at_ms INTEGER NOT NULL DEFAULT 0,
		ttl_ms        INTEGER,
		PRIMARY KEY (namespace, key)
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO _smithers_memory_facts
		(namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms)
		VALUES
		('global',          'test-key-1', '{"x":1}', '', 1000, 2000),
		('workflow:review', 'test-key-2', '"hello"', '', 1000, 1000)`)
	require.NoError(t, err)

	// Inject the test DB directly.
	c := NewClient()
	c.db = db

	facts, err := c.ListAllMemoryFacts(context.Background())
	require.NoError(t, err)
	// ORDER BY updated_at_ms DESC — global/test-key-1 (updated_at=2000) comes first.
	require.Len(t, facts, 2)
	assert.Equal(t, "global", facts[0].Namespace)
	assert.Equal(t, "test-key-1", facts[0].Key)
	assert.Equal(t, `{"x":1}`, facts[0].ValueJSON)
	assert.Equal(t, int64(2000), facts[0].UpdatedAtMs)

	assert.Equal(t, "workflow:review", facts[1].Namespace)
	assert.Equal(t, "test-key-2", facts[1].Key)
}

func TestListAllMemoryFacts_SQLite_Empty(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE _smithers_memory_facts (
		namespace     TEXT    NOT NULL,
		key           TEXT    NOT NULL,
		value_json    TEXT    NOT NULL,
		schema_sig    TEXT    NOT NULL DEFAULT '',
		created_at_ms INTEGER NOT NULL DEFAULT 0,
		updated_at_ms INTEGER NOT NULL DEFAULT 0,
		ttl_ms        INTEGER
	)`)
	require.NoError(t, err)

	c := NewClient()
	c.db = db

	facts, err := c.ListAllMemoryFacts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, facts, "empty table should return empty slice, not error")
}

func TestListAllMemoryFacts_Exec(t *testing.T) {
	want := []MemoryFact{
		{Namespace: "global", Key: "k1", ValueJSON: `"v"`, UpdatedAtMs: 1000},
	}
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "memory", args[0])
		assert.Equal(t, "list", args[1])
		assert.Equal(t, "--all", args[2])
		assert.Equal(t, "--format", args[3])
		assert.Equal(t, "json", args[4])
		return json.Marshal(want)
	})

	facts, err := c.ListAllMemoryFacts(context.Background())
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "global", facts[0].Namespace)
	assert.Equal(t, "k1", facts[0].Key)
	assert.Equal(t, `"v"`, facts[0].ValueJSON)
	assert.Equal(t, int64(1000), facts[0].UpdatedAtMs)
}

func TestListAllMemoryFacts_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, errors.New("smithers binary not found")
	})

	_, err := c.ListAllMemoryFacts(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListAllMemoryFacts_Exec_EmptyResult(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("[]"), nil
	})

	facts, err := c.ListAllMemoryFacts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, facts)
}
