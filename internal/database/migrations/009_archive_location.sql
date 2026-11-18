ALTER TABLE settings ADD COLUMN archive_path TEXT NOT NULL DEFAULT '';

CREATE TABLE archive_moves (
 id INTEGER PRIMARY KEY,
 source TEXT NOT NULL,
 target TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','running','succeeded','failed')),
 prepared INTEGER NOT NULL DEFAULT 0 CHECK(prepared IN(0,1)),
 error TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE UNIQUE INDEX one_archive_move ON archive_moves((1)) WHERE status IN ('queued','running');
