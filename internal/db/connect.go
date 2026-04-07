package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/otel/attribute"
)

var pragmas = map[string]string{
	"foreign_keys":  "ON",
	"journal_mode":  "WAL",
	"page_size":     "4096",
	"cache_size":    "-8000",
	"synchronous":   "NORMAL",
	"secure_delete": "ON",
	"busy_timeout":  "30000",
}

// Connect opens a SQLite database connection and runs migrations.
func Connect(ctx context.Context, dataDir string) (*sql.DB, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data.dir is not set")
	}
	dbPath := resolveDBPath(dataDir)
	ctx = observability.WithComponent(ctx, "db")
	ctx, span := observability.StartSpan(ctx, "db.connect",
		attribute.String("db.system", "sqlite"),
		attribute.String("db.name", filepath.Base(dbPath)),
	)
	defer span.End()

	record := func(operation string, started time.Time, err error) {
		observability.RecordDBOperation(operation, time.Since(started), err)
	}

	start := time.Now()
	db, err := openDB(dbPath)
	if err != nil {
		record("open", start, err)
		observability.RecordError(span, err)
		return nil, err
	}
	record("open", start, nil)

	start = time.Now()
	if err = db.PingContext(ctx); err != nil {
		record("ping", start, err)
		db.Close()
		observability.RecordError(span, err)
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	record("ping", start, nil)

	goose.SetBaseFS(FS)

	start = time.Now()
	if err := goose.SetDialect("sqlite3"); err != nil {
		record("set_dialect", start, err)
		_ = db.Close()
		slog.Error("Failed to set dialect", "error", err)
		observability.RecordError(span, err)
		return nil, fmt.Errorf("failed to set dialect: %w", err)
	}
	record("set_dialect", start, nil)

	if version, versionErr := goose.GetDBVersionContext(ctx, db); versionErr == nil {
		span.SetAttributes(attribute.Int64("db.migration.version.before", version))
	}

	start = time.Now()
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		record("migrate", start, err)
		_ = db.Close()
		slog.Error("Failed to apply migrations", "error", err)
		observability.RecordError(span, err)
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	record("migrate", start, nil)
	if version, versionErr := goose.GetDBVersionContext(ctx, db); versionErr == nil {
		span.SetAttributes(attribute.Int64("db.migration.version.after", version))
	}

	return db, nil
}

func resolveDBPath(dataDir string) string {
	primary := filepath.Join(dataDir, "codeplane.db")
	legacySmithers := filepath.Join(dataDir, "smithers-tui.db")
	legacyCrush := filepath.Join(dataDir, "crush.db")
	if _, err := os.Stat(primary); err == nil {
		return primary
	}
	if _, err := os.Stat(legacySmithers); err == nil {
		return legacySmithers
	}
	if _, err := os.Stat(legacyCrush); err == nil {
		return legacyCrush
	}
	return primary
}
