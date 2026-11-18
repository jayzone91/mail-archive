CREATE TABLE accounts (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 email TEXT NOT NULL,
 username TEXT NOT NULL,
 host TEXT NOT NULL,
 port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
 tls_mode TEXT NOT NULL CHECK (tls_mode IN ('implicit', 'starttls')),
 credential BLOB,
 enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
 auto_backup INTEGER NOT NULL CHECK (auto_backup IN (0, 1)),
 retention_months INTEGER CHECK (retention_months BETWEEN 1 AND 1200),
 removed_at TEXT,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE TABLE audit_events (
 id INTEGER PRIMARY KEY,
 account_id TEXT REFERENCES accounts(id),
 action TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;
