package restore

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/imap"
)

type Source struct {
	Kind, ID string
}

type Request struct {
	Source                      Source
	Target, Folder              string
	PreserveDate, PreserveFlags bool
}

type Job struct {
	ID                               int64
	Target, Status                   string
	Total, Verified, Failed, Review  int
	CreatedAt, StartedAt, FinishedAt string
}

type Issue struct {
	MessageID      int64
	Folder, Detail string
}

var ErrSelection = errors.New("Bitte eine gültige Quelle, ein aktives Zielpostfach und einen gültigen Zielordner auswählen.")
var ErrEmpty = errors.New("Die Auswahl enthält keine archivierten Nachrichten.")

func selection(src Source) (string, error) {
	switch src.Kind {
	case "message":
		return "m.id=?", nil
	case "messages":
		return "m.id IN (SELECT value FROM json_each(?))", nil
	case "folder":
		return "f.id=?", nil
	case "account":
		return "f.account_id=?", nil
	default:
		return "", ErrSelection
	}
}

func selectionArgs(src Source) (string, string, error) {
	condition, err := selection(src)
	if err != nil {
		return "", "", err
	}
	if src.Kind != "messages" {
		return condition, src.ID, nil
	}
	ids := strings.Split(src.ID, ",")
	if len(ids) > 100 {
		return "", "", ErrSelection
	}
	for i, id := range ids {
		value, err := strconv.ParseInt(id, 10, 64)
		if err != nil || value < 1 {
			return "", "", ErrSelection
		}
		ids[i] = strconv.FormatInt(value, 10)
	}
	return condition, "[" + strings.Join(ids, ",") + "]", nil
}

// Bulk restore collapses identical bytes across UIDVALIDITY changes while preserving
// the largest recorded number of identical messages in a single folder generation.
func sourceQuery(src Source, condition string) string {
	from := " FROM messages m JOIN folders f ON f.id=m.folder_id WHERE " + condition
	if src.Kind != "account" && src.Kind != "folder" {
		return "SELECT m.id,f.name" + from
	}
	return `SELECT id,name FROM (
	 SELECT id,name,row_number() OVER(PARTITION BY folder_id,digest,occurrence ORDER BY current_generation DESC,id DESC) AS choice
	 FROM (SELECT m.id,f.name,m.folder_id,m.digest,m.uid_validity=f.uid_validity AS current_generation,
	 row_number() OVER(PARTITION BY m.folder_id,m.uid_validity,m.digest ORDER BY m.uid) AS occurrence` + from + `)
	) WHERE choice=1`
}

// AccountComparison uses the same message selection as a full mailbox restore.
func AccountComparison(ctx context.Context, db *sql.DB, account string, jobID int64) (planned, verified int, errorValue error) {
	err := db.QueryRowContext(ctx, `SELECT count(*),count(i.message_id) FROM (`+
		sourceQuery(Source{Kind: "account"}, "f.account_id=?")+`) choice
		LEFT JOIN restore_items i ON i.message_id=choice.id AND i.job_id=? AND i.status='verified'`, account, jobID).
		Scan(&planned, &verified)
	return planned, verified, err
}

func Preview(ctx context.Context, db *sql.DB, req Request) (int, error) {
	condition, argument, err := selectionArgs(req.Source)
	if err != nil {
		return 0, err
	}
	if req.Source.ID == "" || req.Target == "" || strings.ContainsAny(req.Folder, "\x00\r\n") {
		return 0, ErrSelection
	}
	var active bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=? AND enabled=1 AND removed_at IS NULL)", req.Target).Scan(&active); err != nil {
		return 0, err
	}
	if !active {
		return 0, ErrSelection
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM ("+sourceQuery(req.Source, condition)+")", argument).Scan(&total); err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, ErrEmpty
	}
	return total, nil
}

func Queue(ctx context.Context, db *sql.DB, req Request) error {
	if _, err := Preview(ctx, db, req); err != nil {
		return err
	}
	condition, argument, _ := selectionArgs(req.Source)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var job int64
	err = tx.QueryRowContext(ctx, `INSERT INTO restore_jobs(target,preserve_date,preserve_flags,source_account)
 SELECT id,?,?,CASE WHEN ?='account' THEN ? END FROM accounts WHERE id=? AND enabled=1 AND removed_at IS NULL ON CONFLICT DO NOTHING RETURNING id`,
		req.PreserveDate, req.PreserveFlags, req.Source.Kind, req.Source.ID, req.Target).Scan(&job)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO restore_items(job_id,message_id,folder,original_path)
	SELECT ?,id,COALESCE(NULLIF(?,''),name),? FROM (`+sourceQuery(req.Source, condition)+")", job, req.Folder, req.Folder == "", argument)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrEmpty
	}
	return tx.Commit()
}

func Retry(ctx context.Context, db *sql.DB, job int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `UPDATE OR IGNORE restore_jobs SET status='queued',finished_at=NULL
 WHERE id=? AND status='failed' AND EXISTS(SELECT 1 FROM accounts WHERE id=target AND enabled=1 AND removed_at IS NULL)
 RETURNING id`, job).Scan(&id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE restore_items SET status='queued',error='' WHERE job_id=? AND status IN ('failed','review')", job); err != nil {
		return err
	}
	return tx.Commit()
}

func List(ctx context.Context, db *sql.DB) ([]Job, error) {
	rows, err := db.QueryContext(ctx, `SELECT j.id,a.name,j.status,count(*),sum(i.status='verified'),sum(i.status='failed'),sum(i.status='review'),
 j.created_at,COALESCE(j.started_at,''),COALESCE(j.finished_at,'')
 FROM restore_jobs j JOIN accounts a ON a.id=j.target JOIN restore_items i ON i.job_id=j.id
 GROUP BY j.id ORDER BY j.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.Target, &job.Status, &job.Total, &job.Verified, &job.Failed, &job.Review,
			&job.CreatedAt, &job.StartedAt, &job.FinishedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func Issues(ctx context.Context, db *sql.DB, job int64) ([]Issue, error) {
	rows, err := db.QueryContext(ctx, `SELECT message_id,folder,error FROM restore_items WHERE job_id=? AND status IN ('failed','review')
 ORDER BY message_id LIMIT 100`, job)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []Issue
	for rows.Next() {
		var item Issue
		if err := rows.Scan(&item.MessageID, &item.Folder, &item.Detail); err != nil {
			return nil, err
		}
		issues = append(issues, item)
	}
	return issues, rows.Err()
}

type Service struct {
	DB       *sql.DB
	Accounts accounts.Store
	Archive  archive.Store
}

func (s Service) Work(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		var id int64
		var target string
		var keepDate, keepFlags bool
		err := s.DB.QueryRowContext(ctx, `UPDATE restore_jobs SET status='running',started_at=COALESCE(started_at,CURRENT_TIMESTAMP) WHERE id=(
   SELECT r.id FROM restore_jobs r WHERE r.status IN ('queued','running')
   AND NOT EXISTS(SELECT 1 FROM jobs b WHERE b.account_id=r.target AND b.status='running')
   ORDER BY r.status='running' DESC,r.id LIMIT 1)
   RETURNING id,target,preserve_date,preserve_flags`).Scan(&id, &target, &keepDate, &keepFlags)
		if err == nil {
			err = s.run(ctx, id, target, keepDate, keepFlags)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Error("IMAP-Wiederherstellung fehlgeschlagen", "job", id, "error", err)
			}
			status := "succeeded"
			if err != nil {
				status = "failed"
			}
			if _, err := s.DB.ExecContext(ctx, "UPDATE restore_jobs SET status=?,finished_at=CURRENT_TIMESTAMP WHERE id=?", status, id); err == nil {
				continue
			} else if ctx.Err() == nil {
				log.Error("Restore-Status speichern", "job", id, "error", err)
			}
		} else if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			log.Error("Restore-Auftrag abrufen", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s Service) run(ctx context.Context, id int64, target string, keepDate, keepFlags bool) error {
	store, release := s.Archive.Pin()
	defer release()
	s.Archive = store
	if err := ctx.Err(); err != nil {
		return err
	}
	account, err := s.Accounts.Get(ctx, target)
	if err != nil {
		return err
	}
	if !account.Enabled {
		return ErrSelection
	}
	password, err := s.Accounts.Password(ctx, target)
	if err != nil {
		return err
	}
	defer clear(password)
	client, closeClient, err := imap.Connect(ctx, account.Endpoint, password)
	if err != nil {
		return err
	}
	defer closeClient()
	return s.restore(ctx, client, id, target, keepDate, keepFlags)
}
