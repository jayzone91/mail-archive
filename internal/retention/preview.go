package retention

import (
	"context"
	"database/sql"
	"time"
)

type Candidate struct {
	MessageID            int64
	Folder, InternalDate string
}

type Group struct {
	Account, Cutoff string
	Total           int64
	Sample          []Candidate
}

// Cutoff subtracts calendar months, clamping to the last day when ending in
// a shorter month. The original wall clock time and timezone are retained.
func Cutoff(now time.Time, months int) time.Time {
	year, month, day := now.Date()
	hour, minute, second := now.Clock()
	first := time.Date(year, month, 1, 0, 0, 0, 0, now.Location()).AddDate(0, -months, 0)
	next := first.AddDate(0, 1, 0)
	last := next.AddDate(0, 0, -1).Day()
	return time.Date(first.Year(), first.Month(), min(last, day), hour, minute, second, now.Nanosecond(), now.Location())
}

// Preview uses committed local metadata. It makes no IMAP requests.
func Preview(ctx context.Context, db *sql.DB, now time.Time) ([]Group, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.name,COALESCE(a.retention_months,s.retention_months)
 FROM accounts a CROSS JOIN settings s WHERE a.enabled=1 AND a.removed_at IS NULL ORDER BY a.name,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var id, name string
		var months int
		if err := rows.Scan(&id, &name, &months); err != nil {
			return nil, err
		}
		cutoff := Cutoff(now, months).UTC()
		group, err := previewAccount(ctx, db, id, name, cutoff, now)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func previewAccount(ctx context.Context, db *sql.DB, id, name string, cutoff, now time.Time) (Group, error) {
	group := Group{Account: name, Cutoff: cutoff.Format(time.RFC3339)}
	rows, err := db.QueryContext(ctx, `SELECT m.id,f.name,m.internal_date,count(*) OVER()
 FROM messages m JOIN folders f ON f.id=m.folder_id
 WHERE f.account_id=? AND f.present=1 AND m.present=1 AND m.uid_validity=f.uid_validity
 AND unixepoch(m.internal_date)<unixepoch(?)
 AND NOT EXISTS(SELECT 1 FROM restore_history r WHERE r.target=f.account_id
 AND (r.folder=f.name OR (r.folder='INBOX' AND f.name='INBOX' COLLATE NOCASE))
 AND r.validity=m.uid_validity AND r.target_uid=m.uid AND r.restored_at>?)
 ORDER BY unixepoch(m.internal_date),m.id LIMIT 20`, id, group.Cutoff, now.UTC().Add(-30*24*time.Hour).Format("2006-01-02 15:04:05"))
	if err != nil {
		return group, err
	}
	defer rows.Close()
	for rows.Next() {
		var candidate Candidate
		if err := rows.Scan(&candidate.MessageID, &candidate.Folder, &candidate.InternalDate, &group.Total); err != nil {
			return group, err
		}
		group.Sample = append(group.Sample, candidate)
	}
	return group, rows.Err()
}
