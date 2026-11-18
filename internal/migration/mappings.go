package migration

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jayzone91/mail-archive/internal/restore"
	"github.com/jayzone91/mail-archive/internal/snapshot"
)

type Mapping struct {
	Source, SourceName, SourceHost, Target, TargetName, LastFull, LastSnapshot, RestoreStatus string
	LastSourceCheck, SnapshotTarget, TargetCheckedAt                                          string
	RestoreID                                                                                 int64
	Archived, Verified, Planned                                                               int
	SourceSeen, SourceCovered, SnapshotCovered                                                int64
}

var ErrMapping = errors.New("Zwei unterschiedliche Postfächer und ein aktives Ziel sind erforderlich. Ein Ziel darf nur einer Quelle zugeordnet werden.")

func Save(ctx context.Context, db *sql.DB, source, target string) error {
	if source == "" || target == "" || source == target {
		return ErrMapping
	}
	result, err := db.ExecContext(ctx, `INSERT INTO migration_mappings(source,target)
 SELECT s.id,t.id FROM accounts s,accounts t
 WHERE s.id=? AND t.id=? AND t.enabled=1 AND t.removed_at IS NULL
 AND (s.removed_at IS NULL OR EXISTS(SELECT 1 FROM folders f JOIN messages m ON m.folder_id=f.id WHERE f.account_id=s.id))
 AND NOT EXISTS(SELECT 1 FROM migration_mappings WHERE target=t.id AND source<>s.id)
 ON CONFLICT(source) DO UPDATE SET target=excluded.target`, source, target)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrMapping
	}
	return err
}

func List(ctx context.Context, db *sql.DB) ([]Mapping, error) {
	target, err := snapshot.AutomaticTarget(ctx, db)
	if err != nil {
		return nil, err
	}
	coverage, err := snapshot.CurrentCoverage(ctx, db, target)
	if err != nil {
		return nil, err
	}
	coverageByAccount := make(map[string]int64, len(coverage.Mailboxes))
	for _, entry := range coverage.Mailboxes {
		coverageByAccount[entry.AccountID] = entry.Covered
	}
	rows, err := db.QueryContext(ctx, `SELECT s.id,s.name,s.host,COALESCE(t.id,''),COALESCE(t.name,''),
   COALESCE((SELECT max(finished_at) FROM jobs WHERE account_id=s.id AND mode='full' AND status='succeeded'),''),
   COALESCE(r.id,0),COALESCE(r.status,''),COALESCE(r.finished_at,''),count(m.id),
   COALESCE(proof.finished_at,''),
   COALESCE((SELECT sum(seen) FROM backup_folder_checks WHERE job_id=proof.id),0),
   COALESCE((SELECT sum(covered) FROM backup_folder_checks WHERE job_id=proof.id),0)
  FROM accounts s
  LEFT JOIN folders f ON f.account_id=s.id LEFT JOIN messages m ON m.folder_id=f.id
  LEFT JOIN migration_mappings direction ON direction.source=s.id
  LEFT JOIN accounts t ON t.id=direction.target
  LEFT JOIN restore_jobs r ON r.id=(SELECT max(j.id) FROM restore_jobs j WHERE j.source_account=s.id AND j.target=t.id
    AND NOT EXISTS(SELECT 1 FROM restore_items WHERE job_id=j.id AND original_path=0))
  LEFT JOIN jobs proof ON proof.id=(SELECT id FROM jobs WHERE account_id=s.id AND status='succeeded' AND source_checked=1
   ORDER BY finished_at DESC,id DESC LIMIT 1)
  GROUP BY s.id HAVING s.removed_at IS NULL OR count(m.id)>0 ORDER BY s.name,s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mappings []Mapping
	for rows.Next() {
		var m Mapping
		if err := rows.Scan(&m.Source, &m.SourceName, &m.SourceHost, &m.Target, &m.TargetName, &m.LastFull, &m.RestoreID,
			&m.RestoreStatus, &m.TargetCheckedAt, &m.Archived, &m.LastSourceCheck, &m.SourceSeen, &m.SourceCovered); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range mappings {
		m := &mappings[i]
		m.LastSnapshot, m.SnapshotTarget = coverage.VerifiedAt, coverage.Target
		m.SnapshotCovered = coverageByAccount[m.Source]
		if m.Target != "" {
			m.Planned, m.Verified, err = restore.AccountComparison(ctx, db, m.Source, m.RestoreID)
			if err != nil {
				return nil, err
			}
		}
	}
	return mappings, nil
}
