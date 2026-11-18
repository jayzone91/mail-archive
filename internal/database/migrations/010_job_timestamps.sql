ALTER TABLE jobs ADD COLUMN created_at TEXT;
ALTER TABLE restore_jobs ADD COLUMN started_at TEXT;
ALTER TABLE restore_jobs ADD COLUMN finished_at TEXT;
