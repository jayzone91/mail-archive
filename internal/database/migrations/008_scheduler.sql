ALTER TABLE settings ADD COLUMN automatic_backup INTEGER NOT NULL DEFAULT 0 CHECK(automatic_backup IN (0,1));
ALTER TABLE jobs ADD COLUMN scheduled INTEGER NOT NULL DEFAULT 0 CHECK(scheduled IN (0,1));
ALTER TABLE jobs ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN available_at TEXT NOT NULL DEFAULT '';
CREATE INDEX jobs_ready ON jobs(status,available_at,id);
CREATE INDEX jobs_account_history ON jobs(account_id,mode,status,finished_at);
