ALTER TABLE settings ADD COLUMN snapshot_target TEXT NOT NULL DEFAULT '';
ALTER TABLE settings ADD COLUMN snapshot_requested INTEGER NOT NULL DEFAULT 0;
ALTER TABLE backup_snapshots ADD COLUMN request_generation INTEGER;
ALTER TABLE backup_snapshots ADD COLUMN retry_at TEXT;
ALTER TABLE backup_snapshots ADD COLUMN error TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_snapshots ADD COLUMN coverage_recorded INTEGER NOT NULL DEFAULT 0;

CREATE TRIGGER request_snapshot_after_backup
AFTER UPDATE OF status ON jobs
WHEN NEW.status = 'succeeded' AND OLD.status <> 'succeeded'
BEGIN
 UPDATE settings SET snapshot_requested = snapshot_requested + 1
 WHERE id = 1 AND snapshot_target <> '';
END;

CREATE TABLE snapshot_message_coverage (
 snapshot_id TEXT NOT NULL REFERENCES backup_snapshots(id) ON DELETE CASCADE,
 message_id INTEGER NOT NULL,
 digest TEXT NOT NULL,
 size INTEGER NOT NULL,
 PRIMARY KEY(snapshot_id,message_id)
) WITHOUT ROWID;
