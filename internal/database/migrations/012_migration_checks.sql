CREATE TABLE backup_folder_checks (
 job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
 folder_id INTEGER NOT NULL REFERENCES folders(id),
 seen INTEGER NOT NULL,
 covered INTEGER NOT NULL,
 PRIMARY KEY(job_id,folder_id)
) STRICT;

ALTER TABLE jobs ADD COLUMN source_checked INTEGER NOT NULL DEFAULT 0 CHECK(source_checked IN (0,1));
