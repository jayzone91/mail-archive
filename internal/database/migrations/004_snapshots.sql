CREATE TABLE backup_snapshots (
 id TEXT PRIMARY KEY,
 target TEXT NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('queued','running','verified','failed')),
 objects INTEGER,
 total_bytes INTEGER,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 finished_at TEXT
) STRICT;
CREATE UNIQUE INDEX one_active_snapshot ON backup_snapshots((1)) WHERE status IN ('queued','running');
