package snapshot

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/platform"
)

type Job struct {
	ID, Target, Status, FinishedAt, Error string
	Objects, Bytes                        int64
	Automatic                             bool
}

var ErrTarget = errors.New("Ein vorhandenes absolutes Zielverzeichnis außerhalb des MailArchive-Datenverzeichnisses ist erforderlich.")
var ErrSameStorage = errors.New("Das Ziel liegt auf demselben Datenträger wie das Primärarchiv.")

func Enqueue(ctx context.Context, db *sql.DB, dataDir, archiveDir, target string) (bool, error) {
	destination, err := validateTarget(dataDir, archiveDir, target)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT INTO backup_snapshots(id,target,status) VALUES (?,?,'queued')
  ON CONFLICT DO NOTHING`, rand.Text(), destination)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	return changed != 0, err
}

func validateTarget(dataDir, archiveDir, target string) (string, error) {
	if !filepath.IsAbs(target) {
		return "", ErrTarget
	}
	destination, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", ErrTarget
	}
	primary, err := filepath.EvalSymlinks(dataDir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() || within(primary, destination) || within(destination, primary) {
		return "", ErrTarget
	}
	if actual, err := filepath.EvalSymlinks(archiveDir); err == nil {
		archiveDir = actual
	}
	if within(archiveDir, destination) || within(destination, archiveDir) {
		return "", ErrTarget
	}
	for _, source := range []string{primary, archiveDir} {
		if same, err := platform.SharesStorage(source, destination); err == nil && same {
			return "", ErrSameStorage
		}
	}
	return destination, nil
}

func within(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func List(ctx context.Context, db *sql.DB) ([]Job, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,target,status,COALESCE(objects,0),COALESCE(total_bytes,0),
  COALESCE(finished_at,''),error,request_generation IS NOT NULL
  FROM backup_snapshots ORDER BY created_at DESC,rowid DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Job
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.Target, &job.Status, &job.Objects, &job.Bytes,
			&job.FinishedAt, &job.Error, &job.Automatic); err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

func Work(ctx context.Context, db *sql.DB, source archive.Store, dataDir string, log *slog.Logger) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		if err := scheduleAutomatic(ctx, db); err != nil && ctx.Err() == nil {
			log.Error("Automatische Zweitsicherung einplanen", "error", err)
		}
		id, target, err := claimSnapshot(ctx, db)
		if err == nil {
			manifest, err := createAtTarget(ctx, db, source, dataDir, target, id)
			if err == nil {
				if err := recordVerified(ctx, db, id, target, manifest); err == nil {
					continue
				} else if ctx.Err() == nil {
					log.Error("Snapshot-Abdeckung speichern", "snapshot", id, "error", err)
				}
			}
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Error("Zweitsicherung fehlgeschlagen", "snapshot", id, "error", err)
				if err := recordFailure(ctx, db, id, err); err != nil && ctx.Err() == nil {
					log.Error("Snapshot-Status speichern", "snapshot", id, "error", err)
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			log.Error("Snapshot-Auftrag abrufen", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func createAtTarget(ctx context.Context, db *sql.DB, source archive.Store, dataDir, target, id string) (Manifest, error) {
	store, release := source.Pin()
	defer release()
	destination, err := validateTarget(dataDir, store.Root, target)
	if err != nil {
		return Manifest{}, err
	}
	return Create(ctx, db, store, destination, id)
}
