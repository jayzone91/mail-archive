package snapshot

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"

	"github.com/jayzone91/mail-archive/internal/database"
)

type MailboxCoverage struct {
	AccountID, Account, Email, Host string
	Total, Covered                  int64
}

type Coverage struct {
	ID, Target, VerifiedAt string
	Mailboxes              []MailboxCoverage
}

func CurrentCoverage(ctx context.Context, db *sql.DB, automaticTarget string) (Coverage, error) {
	var coverage Coverage
	err := db.QueryRowContext(ctx, `SELECT id,target,finished_at FROM backup_snapshots
 WHERE status='verified' AND coverage_recorded=1 AND (?='' OR target=?)
 ORDER BY finished_at DESC,rowid DESC LIMIT 1`, automaticTarget, automaticTarget).
		Scan(&coverage.ID, &coverage.Target, &coverage.VerifiedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return coverage, err
	}
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.name,a.email,a.host,count(m.id),count(c.message_id)
 FROM accounts a LEFT JOIN folders f ON f.account_id=a.id LEFT JOIN messages m ON m.folder_id=f.id
 LEFT JOIN snapshot_message_coverage c ON c.snapshot_id=? AND c.message_id=m.id AND c.digest=m.digest AND c.size=m.size
 GROUP BY a.id HAVING a.removed_at IS NULL OR count(m.id)>0 ORDER BY a.name,a.email,a.id`, coverage.ID)
	if err != nil {
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var mailbox MailboxCoverage
		if err := rows.Scan(&mailbox.AccountID, &mailbox.Account, &mailbox.Email, &mailbox.Host, &mailbox.Total, &mailbox.Covered); err != nil {
			return coverage, err
		}
		coverage.Mailboxes = append(coverage.Mailboxes, mailbox)
	}
	return coverage, rows.Err()
}

func recordVerified(ctx context.Context, db *sql.DB, id, target string, manifest Manifest) error {
	image, err := database.OpenSnapshot(filepath.Join(target, id, "mailarchive.sqlite"))
	if err != nil {
		return err
	}
	defer image.Close()
	rows, err := image.QueryContext(ctx, "SELECT id,digest,size FROM messages")
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM snapshot_message_coverage WHERE snapshot_id IN
  (SELECT id FROM backup_snapshots WHERE target=?)`, target); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE backup_snapshots SET coverage_recorded=0 WHERE target=? AND coverage_recorded=1", target); err != nil {
		return err
	}
	insert, err := tx.PrepareContext(ctx, "INSERT INTO snapshot_message_coverage(snapshot_id,message_id,digest,size) VALUES (?,?,?,?)")
	if err != nil {
		return err
	}
	defer insert.Close()
	for rows.Next() {
		var messageID, size int64
		var digest string
		if err := rows.Scan(&messageID, &digest, &size); err != nil {
			return err
		}
		if _, err := insert.ExecContext(ctx, id, messageID, digest, size); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	size := manifest.Database.Size
	for _, object := range manifest.Objects {
		size += object.Size
	}
	if _, err := tx.ExecContext(ctx, `UPDATE backup_snapshots SET status='verified',coverage_recorded=1,
  objects=?,total_bytes=?,finished_at=CURRENT_TIMESTAMP WHERE id=?`, len(manifest.Objects), size, id); err != nil {
		return err
	}
	return tx.Commit()
}
