package db

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQueriesCreateSessionEmitsObservedSpan(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})
	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))

	conn, err := Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	q := New(conn)
	_, err = q.CreateSession(t.Context(), CreateSessionParams{
		ID:    uuid.NewString(),
		Title: "observed session",
	})
	require.NoError(t, err)

	spans := observability.RecentSpans(20)
	require.NotEmpty(t, spans)

	var found bool
	for _, span := range spans {
		if span.Name != "db.query_row" {
			continue
		}
		found = true
		require.Equal(t, "create_session", span.Attributes["db.query.name"])
		require.Equal(t, "insert", span.Attributes["db.operation"])
		require.Equal(t, false, span.Attributes["db.transaction"])
	}
	require.True(t, found)
}

func TestBeginObservedTxCommitEmitsObservedSpan(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})
	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))

	conn, err := Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	tx, err := BeginObservedTx(t.Context(), conn, "unit_test", nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	spans := observability.RecentSpans(20)
	require.NotEmpty(t, spans)

	var found bool
	for _, span := range spans {
		if span.Name != "db.tx" {
			continue
		}
		found = true
		require.Equal(t, "unit_test", span.Attributes["db.tx.name"])
		require.Equal(t, "commit", span.Attributes["db.tx.phase"])
	}
	require.True(t, found)
}
