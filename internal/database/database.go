package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteLocation(path, "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"))
	if err != nil {
		return nil, err
	}
	// WAL persists in the database. Set it once before other connections
	// start working, instead of issuing the pragma for each new connection.
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		db.Close()
		return nil, fmt.Errorf("SQLite-WAL aktivieren: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		db.Close()
		return nil, fmt.Errorf("SQLite hat WAL nicht aktiviert: %s", mode)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("SQLite-Migration: %w", err)
	}
	return db, nil
}

func IsBusy(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_BUSY
}

// OpenSnapshot opens a detached SQLite image without migration or WAL writes.
func OpenSnapshot(path string) (*sql.DB, error) {
	return sql.Open("sqlite", sqliteLocation(path, "mode=ro&immutable=1"))
}

func sqliteLocation(path, query string) string {
	filePath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}
	location := url.URL{
		Scheme:   "file",
		Path:     filePath,
		RawQuery: query,
	}
	return location.String()
}

func migrate(ctx context.Context, db *sql.DB) error {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY) STRICT"); err != nil {
		return err
	}
	for _, file := range files {
		var applied bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", file.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		script, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf("%s: %w", file.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (?)", file.Name()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
