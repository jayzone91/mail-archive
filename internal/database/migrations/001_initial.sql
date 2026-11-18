CREATE TABLE settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    backup_hours INTEGER NOT NULL DEFAULT 6 CHECK (backup_hours > 0),
    retention_months INTEGER NOT NULL DEFAULT 2 CHECK (retention_months > 0),
    cleanup_hours INTEGER NOT NULL DEFAULT 24 CHECK (cleanup_hours > 0),
    workers INTEGER NOT NULL DEFAULT 2 CHECK (workers > 0),
    retries INTEGER NOT NULL DEFAULT 3 CHECK (retries >= 0),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

INSERT INTO settings (id) VALUES (1);

CREATE TABLE administrator (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    salt BLOB NOT NULL,
    password_hash BLOB NOT NULL
) STRICT;
