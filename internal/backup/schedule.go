package backup

import (
	"context"
	"database/sql"
	"time"
)

// ScheduleDue queues one job per eligible account. Job history is the durable
// schedule, so missed runs coalesce after a restart.
func ScheduleDue(ctx context.Context, db *sql.DB, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO jobs(account_id,mode,status,scheduled,available_at,created_at)
 SELECT a.id,CASE WHEN EXISTS(SELECT 1 FROM jobs previous
 WHERE previous.account_id=a.id AND previous.mode='full' AND previous.status='succeeded')
 THEN 'incremental' ELSE 'full' END,'queued',1,?,?
 FROM accounts a CROSS JOIN settings s
 WHERE s.automatic_backup=1 AND a.enabled=1 AND a.auto_backup=1 AND a.removed_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM jobs active WHERE active.account_id=a.id AND active.status IN ('queued','running'))
 AND COALESCE((SELECT max(j.finished_at) FROM jobs j WHERE j.account_id=a.id),'') <=
 datetime(?,printf('-%d hours',s.backup_hours))
 ON CONFLICT DO NOTHING`, sqlTime(now), sqlTime(now), sqlTime(now))
	return err
}

func sqlTime(now time.Time) string { return now.UTC().Format("2006-01-02 15:04:05") }

type claimedJob struct {
	id            int64
	account, mode string
	scheduled     bool
	attempts      int
}

func claim(ctx context.Context, db *sql.DB, now time.Time) (claimedJob, error) {
	var job claimedJob
	err := db.QueryRowContext(ctx, `UPDATE jobs SET status='running', processed=0, attempts=attempts+1,source_checked=0,
 started_at=COALESCE(started_at,CURRENT_TIMESTAMP)
 WHERE id=(SELECT b.id FROM jobs b WHERE b.status='queued' AND b.available_at<=?
 AND (b.scheduled=0 OR EXISTS(SELECT 1 FROM settings WHERE automatic_backup=1))
 AND NOT EXISTS(SELECT 1 FROM restore_jobs r WHERE r.target=b.account_id AND r.status='running')
 ORDER BY b.scheduled,b.id LIMIT 1)
 RETURNING id,account_id,mode,scheduled,attempts`, sqlTime(now)).Scan(&job.id, &job.account, &job.mode, &job.scheduled, &job.attempts)
	return job, err
}

func finish(ctx context.Context, db *sql.DB, job claimedJob, jobErr error, now time.Time) error {
	if jobErr == nil {
		_, err := db.ExecContext(ctx, "UPDATE jobs SET status='succeeded',error='',finished_at=? WHERE id=?", sqlTime(now), job.id)
		return err
	}
	message := "Sicherung fehlgeschlagen. Details stehen im Anwendungsprotokoll."
	delay := time.Minute << min(max(job.attempts-1, 0), 6)
	_, err := db.ExecContext(ctx, `UPDATE jobs SET error=?,
 status=CASE WHEN scheduled=1 AND attempts<=(SELECT retries FROM settings WHERE automatic_backup=1) THEN 'queued' ELSE 'failed' END,
 finished_at=CASE WHEN scheduled=1 AND attempts<=(SELECT retries FROM settings WHERE automatic_backup=1) THEN NULL ELSE ? END,
 available_at=?
 WHERE id=?`, message, sqlTime(now), sqlTime(now.Add(delay)), job.id)
	return err
}
