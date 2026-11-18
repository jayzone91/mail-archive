CREATE TABLE folders (
 id INTEGER PRIMARY KEY,
 account_id TEXT NOT NULL REFERENCES accounts(id),
 name TEXT NOT NULL,
 delimiter TEXT NOT NULL,
 attributes TEXT NOT NULL,
 uid_validity INTEGER NOT NULL,
 uid_next INTEGER NOT NULL,
 present INTEGER NOT NULL DEFAULT 1,
 UNIQUE (account_id,name)
) STRICT;

CREATE TABLE messages (
 id INTEGER PRIMARY KEY,
 folder_id INTEGER NOT NULL REFERENCES folders(id),
 uid_validity INTEGER NOT NULL,
 uid INTEGER NOT NULL,
 digest TEXT NOT NULL CHECK (length(digest) = 64),
 size INTEGER NOT NULL CHECK (size > 0),
 internal_date TEXT NOT NULL,
 flags TEXT NOT NULL,
 envelope TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 present INTEGER NOT NULL DEFAULT 1,
 UNIQUE (folder_id,uid_validity,uid)
) STRICT;

CREATE TABLE message_flag_history (
 id INTEGER PRIMARY KEY,
 message_id INTEGER NOT NULL REFERENCES messages(id),
 flags TEXT NOT NULL,
 recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE TABLE jobs (
 id INTEGER PRIMARY KEY,
 account_id TEXT NOT NULL REFERENCES accounts(id),
 mode TEXT NOT NULL CHECK (mode IN ('full','incremental')),
 status TEXT NOT NULL CHECK (status IN ('queued','running','succeeded','failed')),
 processed INTEGER NOT NULL DEFAULT 0,
 started_at TEXT,
 finished_at TEXT,
 error TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE UNIQUE INDEX one_active_backup ON jobs(account_id) WHERE status IN ('queued','running');

CREATE TABLE backup_checkpoints (
 job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
 message_id INTEGER NOT NULL REFERENCES messages(id),
 PRIMARY KEY(job_id,message_id)
) STRICT;
