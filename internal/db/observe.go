package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/crush/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func beginObservedQuery(ctx context.Context, query, kind string, inTx bool) (context.Context, trace.Span, string, time.Time) {
	queryName := queryTelemetryName(query)
	statementType := queryStatementType(query)
	operation := queryTelemetryOperation(queryName, statementType, kind)

	ctx = observability.WithComponent(ctx, "db")
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "sqlite"),
		attribute.String("db.query.kind", kind),
		attribute.String("db.query.name", queryName),
		attribute.String("db.operation", statementType),
		attribute.Bool("db.transaction", inTx),
	}
	ctx, span := observability.StartSpan(ctx, "db."+kind, attrs...)
	return ctx, span, operation, time.Now()
}

func finishObservedQuery(span trace.Span, operation string, started time.Time, err error) {
	observability.RecordError(span, err)
	span.End()
	observability.RecordDBOperation(operation, time.Since(started), err)
}

func queryTelemetryOperation(queryName, statementType, kind string) string {
	switch {
	case queryName != "":
		return "sqlc." + queryName + "." + kind
	case statementType != "":
		return "sql." + statementType + "." + kind
	default:
		return "sql." + kind
	}
}

func queryTelemetryName(query string) string {
	line, _, _ := strings.Cut(query, "\n")
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "-- name:") {
		return "unknown"
	}
	fields := strings.Fields(strings.TrimPrefix(line, "-- name:"))
	if len(fields) == 0 {
		return "unknown"
	}
	return toSnakeCase(fields[0])
}

func queryStatementType(query string) string {
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		keyword, _, _ := strings.Cut(line, " ")
		return strings.ToLower(strings.TrimSpace(keyword))
	}
	return "unknown"
}

func toSnakeCase(value string) string {
	if value == "" {
		return ""
	}

	var out strings.Builder
	for i, r := range value {
		if unicode.IsUpper(r) {
			if i > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(r))
			continue
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}

// ObservedTx wraps a sql.Tx with observability around begin/commit/rollback.
type ObservedTx struct {
	tx      *sql.Tx
	name    string
	started time.Time
	span    trace.Span
	once    sync.Once
}

// BeginObservedTx starts a traced SQL transaction with a low-cardinality name.
func BeginObservedTx(ctx context.Context, db *sql.DB, name string, opts *sql.TxOptions) (*ObservedTx, error) {
	ctx = observability.WithComponent(ctx, "db")
	ctx, span := observability.StartSpan(ctx, "db.tx",
		attribute.String("db.system", "sqlite"),
		attribute.String("db.tx.name", defaultTransactionName(name)),
	)
	started := time.Now()

	tx, err := db.BeginTx(ctx, opts)
	observability.RecordDBOperation("tx."+defaultTransactionName(name)+".begin", time.Since(started), err)
	if err != nil {
		observability.RecordError(span, err)
		span.End()
		return nil, err
	}

	return &ObservedTx{
		tx:      tx,
		name:    defaultTransactionName(name),
		started: started,
		span:    span,
	}, nil
}

// SQLTx returns the wrapped sql.Tx.
func (tx *ObservedTx) SQLTx() *sql.Tx {
	if tx == nil {
		return nil
	}
	return tx.tx
}

// Commit commits the transaction and records the result.
func (tx *ObservedTx) Commit() error {
	if tx == nil || tx.tx == nil {
		return nil
	}
	err := tx.tx.Commit()
	tx.finish("commit", err)
	return err
}

// Rollback rolls the transaction back and records the result.
func (tx *ObservedTx) Rollback() error {
	if tx == nil || tx.tx == nil {
		return nil
	}
	err := tx.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return err
	}
	tx.finish("rollback", err)
	return err
}

func (tx *ObservedTx) finish(phase string, err error) {
	tx.once.Do(func() {
		tx.span.SetAttributes(attribute.String("db.tx.phase", phase))
		observability.RecordError(tx.span, err)
		tx.span.End()
		observability.RecordDBOperation("tx."+tx.name+"."+phase, time.Since(tx.started), err)
	})
}

func defaultTransactionName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unnamed"
	}
	return toSnakeCase(name)
}
