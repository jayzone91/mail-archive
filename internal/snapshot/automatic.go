package snapshot

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
)

func AutomaticTarget(ctx context.Context, db *sql.DB) (string, error) {
	var path string
	err := db.QueryRowContext(ctx, "SELECT snapshot_target FROM settings WHERE id=1").Scan(&path)
	return path, err
}

// ConfigureAutomatic activates automatic snapshots for the validated destination.
// Clearing the destination stops future automatic snapshots.
func ConfigureAutomatic(ctx context.Context, db *sql.DB, dataDir, archiveDir, target string) error {
	if target != "" {
		actual, err := validateTarget(dataDir, archiveDir, target)
		if err != nil {
			return err
		}
		target = actual
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE settings SET snapshot_target=?,
 snapshot_requested=snapshot_requested+CASE WHEN ?<>'' THEN 1 ELSE 0 END,
 updated_at=CURRENT_TIMESTAMP WHERE id=1 AND snapshot_target<>?`, target, target, target); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE backup_snapshots
 SET status='failed',error='Das automatische Zweitsicherungsziel wurde geändert oder deaktiviert.',finished_at=CURRENT_TIMESTAMP,retry_at=NULL
 WHERE status='queued' AND request_generation IS NOT NULL AND target<>?`, target); err != nil {
		return err
	}
	return tx.Commit()
}

func scheduleAutomatic(ctx context.Context, db *sql.DB) error {
	const due = `s.id=1 AND s.snapshot_target<>'' AND s.snapshot_requested>
 COALESCE((SELECT max(b.request_generation) FROM backup_snapshots b WHERE b.target=s.snapshot_target),0)
 AND NOT EXISTS(SELECT 1 FROM backup_snapshots WHERE status IN ('queued','running'))`
	var pending bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM settings s WHERE "+due+")").Scan(&pending); err != nil || !pending {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO backup_snapshots(id,target,status,request_generation)
 SELECT ?,s.snapshot_target,'queued',s.snapshot_requested FROM settings s WHERE `+due+" ON CONFLICT DO NOTHING", rand.Text())
	return err
}

func claimSnapshot(ctx context.Context, db *sql.DB) (string, string, error) {
	var id, target string
	err := db.QueryRowContext(ctx, `UPDATE backup_snapshots SET status='running',retry_at=NULL,error=''
 WHERE id=COALESCE(
   (SELECT id FROM backup_snapshots WHERE status IN ('queued','running') ORDER BY status='running' DESC,rowid LIMIT 1),
   (SELECT b.id FROM backup_snapshots b CROSS JOIN settings s
    WHERE b.status='failed' AND b.retry_at<=CURRENT_TIMESTAMP AND b.target=s.snapshot_target
    AND b.request_generation=s.snapshot_requested ORDER BY b.rowid DESC LIMIT 1))
 RETURNING id,target`).Scan(&id, &target)
	return id, target, err
}

func recordFailure(ctx context.Context, db *sql.DB, id string, cause error) error {
	detail := "Der Snapshot konnte nicht erstellt oder verifiziert werden. Details stehen im Anwendungsprotokoll."
	switch {
	case errors.Is(cause, ErrSameStorage):
		detail = ErrSameStorage.Error()
	case errors.Is(cause, ErrTarget):
		detail = "Das Zweitsicherungsziel ist nicht verfügbar oder überschneidet sich mit den Primärdaten."
	}
	_, err := db.ExecContext(ctx, `UPDATE backup_snapshots SET status='failed',error=?,finished_at=CURRENT_TIMESTAMP,
 retry_at=CASE WHEN request_generation IS NOT NULL THEN datetime('now','+5 minutes') END WHERE id=?`, detail, id)
	return err
}
