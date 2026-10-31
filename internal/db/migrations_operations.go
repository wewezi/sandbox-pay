package db

import "database/sql"

func migrateOperationsVersion5(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE operation_settings (
 id INTEGER PRIMARY KEY CHECK (id=1),
 usageRetentionDays INTEGER NOT NULL DEFAULT 90 CHECK (usageRetentionDays BETWEEN 1 AND 3650),
 auditRetentionDays INTEGER NOT NULL DEFAULT 180 CHECK (auditRetentionDays BETWEEN 1 AND 3650),
 attemptRetentionDays INTEGER NOT NULL DEFAULT 90 CHECK (attemptRetentionDays BETWEEN 1 AND 3650),
 backupIntervalHours INTEGER NOT NULL DEFAULT 24 CHECK (backupIntervalHours BETWEEN 0 AND 720),
 backupKeepCount INTEGER NOT NULL DEFAULT 7 CHECK (backupKeepCount BETWEEN 1 AND 100),
 updatedAt DATETIME NOT NULL
);
INSERT INTO operation_settings (id,updatedAt) VALUES (1,datetime('now'));
CREATE TABLE admin_audit_logs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 userId INTEGER,
 username TEXT NOT NULL,
 method TEXT NOT NULL,
 path TEXT NOT NULL,
 action TEXT NOT NULL,
 target TEXT NOT NULL DEFAULT '',
 clientIp TEXT NOT NULL DEFAULT '',
 statusCode INTEGER NOT NULL,
 durationMs REAL NOT NULL,
 requestId TEXT NOT NULL DEFAULT '',
 createdAt DATETIME NOT NULL
);
CREATE INDEX IDX_audit_created ON admin_audit_logs(createdAt DESC,id DESC);
CREATE INDEX IDX_audit_user ON admin_audit_logs(userId,createdAt DESC);
CREATE TABLE operation_runs (
 name TEXT PRIMARY KEY,
 lastRunAt DATETIME NOT NULL,
 lastError TEXT NOT NULL DEFAULT ''
);
`)
	return err
}
