package archive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jayzone91/mail-archive/internal/platform"
)

type Location struct {
	db      *sql.DB
	dataDir string
	mu      sync.RWMutex
	root    atomic.Value
}

type Move struct {
	ID                    int64
	Target, Status, Error string
}

var ErrMoveTarget = errors.New("Ein absolutes, separates Archivverzeichnis mit vorhandenem übergeordneten Verzeichnis ist erforderlich.")
var ErrMovePending = errors.New("Eine Archivverschiebung ist bereits eingeplant oder läuft.")
var ErrNonEmpty = errors.New("Das Zielverzeichnis enthält fremde Dateien. Bitte ein leeres Archivverzeichnis wählen.")

func OpenLocation(ctx context.Context, db *sql.DB, dataDir string) (*Location, error) {
	base, err := filepath.EvalSymlinks(dataDir)
	if err != nil {
		return nil, err
	}
	location := &Location{db: db, dataDir: base}
	var root string
	if err := db.QueryRowContext(ctx, "SELECT archive_path FROM settings WHERE id=1").Scan(&root); err != nil {
		return nil, err
	}
	if root == "" {
		root = filepath.Join(base, "archive")
	}
	location.root.Store(root)
	return location, nil
}

func (l *Location) Store() Store { return Store{location: l} }

func (l *Location) Current() string {
	return l.root.Load().(string)
}

func (l *Location) LastMove(ctx context.Context) (Move, error) {
	var move Move
	err := l.db.QueryRowContext(ctx, "SELECT id,target,status,error FROM archive_moves ORDER BY id DESC LIMIT 1").
		Scan(&move.ID, &move.Target, &move.Status, &move.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return Move{}, nil
	}
	return move, err
}

func (l *Location) Queue(ctx context.Context, target string) error {
	target, err := resolveTarget(target)
	if err != nil {
		return err
	}
	// Leave the UI responsive during a long migration.
	if !l.mu.TryRLock() {
		return ErrMovePending
	}
	defer l.mu.RUnlock()
	root := l.Current()
	if overlaps(root, target) || isWithin(target, l.dataDir) ||
		(isWithin(l.dataDir, target) && !equalPath(target, filepath.Join(l.dataDir, "archive"))) {
		return ErrMoveTarget
	}
	result, err := l.db.ExecContext(ctx, `INSERT INTO archive_moves(source,target,status)
 VALUES (?,?,'queued') ON CONFLICT DO NOTHING`, root, target)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrMovePending
	}
	return err
}

func resolveTarget(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrMoveTarget
	}
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if parent == path {
		return "", ErrMoveTarget
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", ErrMoveTarget
	}
	path = filepath.Join(resolved, filepath.Base(path))
	if stat, err := os.Lstat(path); err == nil && !stat.IsDir() {
		return "", ErrMoveTarget
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", ErrMoveTarget
	}
	if actual, err := filepath.EvalSymlinks(path); err == nil && !equalPath(actual, path) {
		return "", ErrMoveTarget
	}
	return path, nil
}

func equalPath(a, b string) bool {
	relative, err := filepath.Rel(a, b)
	return err == nil && relative == "."
}

func overlaps(a, b string) bool { return isWithin(a, b) || isWithin(b, a) }

func isWithin(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func (l *Location) Work(ctx context.Context, log *slog.Logger) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		var move Move
		var source string
		var prepared bool
		err := l.db.QueryRowContext(ctx, `UPDATE archive_moves SET status='running'
  WHERE id=(SELECT id FROM archive_moves WHERE status IN ('queued','running') ORDER BY id LIMIT 1)
  RETURNING id,source,target,prepared`).Scan(&move.ID, &source, &move.Target, &prepared)
		if err == nil {
			if err := l.migrate(ctx, move, source, prepared); err != nil && ctx.Err() == nil {
				log.Error("Archivpfadwechsel fehlgeschlagen", "job", move.ID, "error", err)
				if _, saveErr := l.db.ExecContext(ctx, "UPDATE archive_moves SET status='failed',error=? WHERE id=?", err.Error(), move.ID); saveErr != nil && ctx.Err() == nil {
					log.Error("Archivpfadwechsel-Status speichern", "job", move.ID, "error", saveErr)
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			log.Error("Archivpfadwechsel abrufen", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (l *Location) migrate(ctx context.Context, move Move, source string, prepared bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !equalPath(l.Current(), source) {
		return ErrMoveTarget
	}
	target, err := resolveTarget(move.Target)
	if err != nil || !equalPath(target, move.Target) || overlaps(source, target) {
		return ErrMoveTarget
	}
	if !prepared {
		if err := l.prepare(ctx, move.ID, target); err != nil {
			return err
		}
	} else if info, err := os.Lstat(target); err != nil || !info.IsDir() {
		return ErrMoveTarget
	}
	rows, err := l.db.QueryContext(ctx, "SELECT DISTINCT digest,size FROM messages ORDER BY digest")
	if err != nil {
		return err
	}
	defer rows.Close()
	from, to := Store{Root: source}, Store{Root: target, requireExisting: true}
	for rows.Next() {
		var object Object
		if err := rows.Scan(&object.Digest, &object.Size); err != nil {
			return err
		}
		if err := copyForMove(ctx, from, to, object); err != nil {
			return fmt.Errorf("Archivobjekt %s: %w", object.Digest, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE settings SET archive_path=?,updated_at=CURRENT_TIMESTAMP WHERE id=1", target); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE archive_moves SET status='succeeded',error='' WHERE id=?", move.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(action) VALUES ('archive_path_changed')"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	l.root.Store(target)
	return nil
}

func (l *Location) prepare(ctx context.Context, moveID int64, target string) error {
	entries, err := os.ReadDir(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(entries) != 0 {
		var known bool
		if err := l.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM archive_moves
   WHERE (source=? AND status='succeeded') OR (target=? AND prepared=1))`, target, target).Scan(&known); err != nil {
			return err
		}
		if !known {
			return ErrNonEmpty
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.Type().IsRegular() || !(strings.HasSuffix(name, ".eml") || strings.HasPrefix(name, ".incoming-")) {
				return ErrNonEmpty
			}
		}
	}
	if err := os.Mkdir(target, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := platform.PrivateDir(target); err != nil {
		return err
	}
	_, err = l.db.ExecContext(ctx, "UPDATE archive_moves SET prepared=1 WHERE id=?", moveID)
	return err
}

func copyForMove(ctx context.Context, from, to Store, object Object) error {
	source, err := from.Open(object.Digest)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := to.path(object.Digest)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return ErrNonEmpty
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := to.Verify(object); err == nil {
		return from.Verify(object)
	}
	_, err = to.put(contextReader{ctx, source}, object.Size, object.Digest)
	return err
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
