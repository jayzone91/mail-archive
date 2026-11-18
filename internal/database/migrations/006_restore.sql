CREATE TABLE restore_jobs (
 id INTEGER PRIMARY KEY,
 target TEXT NOT NULL REFERENCES accounts(id),
 status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','succeeded','failed')),
 preserve_date INTEGER NOT NULL,
 preserve_flags INTEGER NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;
CREATE UNIQUE INDEX one_active_restore ON restore_jobs(target) WHERE status IN ('queued','running');
CREATE TABLE restore_items (
 job_id INTEGER NOT NULL REFERENCES restore_jobs(id),
 message_id INTEGER NOT NULL REFERENCES messages(id),
 folder TEXT NOT NULL,
 original_path INTEGER NOT NULL,
 mapped_folder TEXT,
 status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','verified','failed','review')),
 error TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(job_id,message_id)
) STRICT;
CREATE TABLE restore_history (
 target TEXT NOT NULL REFERENCES accounts(id),
 folder TEXT NOT NULL,
 message_id INTEGER NOT NULL REFERENCES messages(id),
 validity INTEGER NOT NULL,
 first_uid INTEGER NOT NULL,
 target_uid INTEGER,
 restored_at TEXT,
 PRIMARY KEY(target,folder,message_id),
 UNIQUE(target,folder,validity,target_uid)
) STRICT;
