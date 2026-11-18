ALTER TABLE restore_jobs ADD COLUMN source_account TEXT REFERENCES accounts(id);

CREATE TABLE migration_mappings (
 source TEXT PRIMARY KEY REFERENCES accounts(id),
 target TEXT NOT NULL UNIQUE REFERENCES accounts(id),
 CHECK(source<>target)
) STRICT;
