package backup

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jayzone91/mail-archive/internal/config"
)

type Job struct {
	ID                               int64
	Account, Mode, Status, Error     string
	Processed                        int64
	CreatedAt, StartedAt, FinishedAt string
}

func Enqueue(ctx context.Context, db *sql.DB, id, mode string) (bool, error) {
	if mode != "full" && mode != "incremental" {
		return false, errors.New("Ungültiger Sicherungsmodus.")
	}
	result, err := db.ExecContext(ctx, `INSERT INTO jobs(account_id,mode,status,created_at)
   SELECT id,?,'queued',CURRENT_TIMESTAMP FROM accounts WHERE (?='' OR id=?) AND enabled=1 AND removed_at IS NULL
   ON CONFLICT DO NOTHING`, mode, id, id)
	if err != nil {
		return false, err
	}
	added, err := result.RowsAffected()
	return added != 0, err
}

func Retry(ctx context.Context, db *sql.DB, id int64) (bool, error) {
	result, err := db.ExecContext(ctx, `UPDATE jobs SET status='queued',error='',finished_at=NULL,scheduled=0,available_at=''
	 WHERE id=? AND status='failed'
	 AND EXISTS(SELECT 1 FROM accounts WHERE accounts.id=jobs.account_id AND enabled=1 AND removed_at IS NULL)
	 AND NOT EXISTS(SELECT 1 FROM jobs AS other WHERE other.account_id=jobs.account_id AND other.status IN ('queued','running'))`, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count != 0, err
}

func List(ctx context.Context, db *sql.DB) ([]Job, error) {
	rows, err := db.QueryContext(ctx, `SELECT jobs.id,accounts.name,mode,status,error,processed,
  COALESCE(jobs.created_at,''),COALESCE(jobs.started_at,''),COALESCE(jobs.finished_at,'')
  FROM jobs JOIN accounts ON accounts.id=jobs.account_id ORDER BY jobs.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.Account, &job.Mode, &job.Status, &job.Error, &job.Processed,
			&job.CreatedAt, &job.StartedAt, &job.FinishedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// Work starts queued jobs up to the configured concurrency limit.
func (s Service) Work(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if _, err := s.DB.ExecContext(ctx, `UPDATE jobs SET status=CASE
		 WHEN scheduled=1 AND NOT EXISTS(SELECT 1 FROM settings WHERE automatic_backup=1) THEN 'failed' ELSE 'queued' END,
		 attempts=max(0,attempts-1) WHERE status='running'`); err == nil {
			break
		} else if ctx.Err() == nil {
			log.Error("Unterbrochene Sicherungen neu einplanen", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
	var pool sync.WaitGroup
	defer pool.Wait()
	active := make(chan struct{}, 64)
	var nextSchedule time.Time
	for ctx.Err() == nil {
		now := time.Now()
		settings, err := config.Load(ctx, s.DB)
		if err != nil && ctx.Err() == nil {
			log.Error("Sicherungseinstellungen laden", "error", err)
		}
		if err == nil && settings.AutomaticBackup && !now.Before(nextSchedule) {
			if err := ScheduleDue(ctx, s.DB, now); err != nil && ctx.Err() == nil {
				log.Error("Automatische Sicherungen einplanen", "error", err)
			}
			nextSchedule = now.Add(time.Minute)
		}
		for err == nil && ctx.Err() == nil && len(active) < min(settings.Workers, cap(active)) {
			job, claimErr := claim(ctx, s.DB, now)
			if claimErr != nil {
				if !errors.Is(claimErr, sql.ErrNoRows) && ctx.Err() == nil {
					log.Error("Sicherungsjob abrufen", "error", claimErr)
				}
				break
			}
			active <- struct{}{}
			pool.Go(func() {
				defer func() { <-active }()
				runErr := s.run(ctx, job.id, job.account, job.mode)
				if ctx.Err() != nil {
					return
				}
				if runErr != nil {
					log.Error("Sicherung fehlgeschlagen", "job", job.id, "error", runErr)
				}
				if err := finish(ctx, s.DB, job, runErr, time.Now()); err != nil && ctx.Err() == nil {
					log.Error("Jobstatus speichern", "job", job.id, "error", err)
				}
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
