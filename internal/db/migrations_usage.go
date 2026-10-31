package db

import "database/sql"

func migrateVersion4(tx *sql.Tx) error {
	// Log references deliberately have no foreign keys: resetting orders or
	// removing a credential must not erase the request history or its snapshots.
	_, err := tx.Exec(`CREATE TABLE "api_usage_logs" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "requestId" TEXT NOT NULL DEFAULT '',
  "method" TEXT NOT NULL,
  "path" TEXT NOT NULL,
  "statusCode" INTEGER NOT NULL CHECK ("statusCode" BETWEEN 100 AND 599),
  "durationMs" REAL NOT NULL CHECK ("durationMs" >= 0),
  "groupId" INTEGER,
  "groupName" TEXT NOT NULL DEFAULT '',
  "apiKeyId" INTEGER,
  "apiKeyName" TEXT NOT NULL DEFAULT '',
  "keyPrefix" TEXT NOT NULL DEFAULT '',
  "keySuffix" TEXT NOT NULL DEFAULT '',
  "clientIp" TEXT NOT NULL DEFAULT '',
  "clientLocation" TEXT NOT NULL DEFAULT '',
  "orderNo" TEXT NOT NULL DEFAULT '',
  "errorMessage" TEXT NOT NULL DEFAULT '',
  "createdAt" DATETIME NOT NULL
);
CREATE INDEX "IDX_usage_created" ON "api_usage_logs" ("createdAt" DESC, "id" DESC);
CREATE INDEX "IDX_usage_group" ON "api_usage_logs" ("groupId", "createdAt" DESC);
CREATE INDEX "IDX_usage_key" ON "api_usage_logs" ("apiKeyId", "createdAt" DESC);
CREATE INDEX "IDX_usage_status" ON "api_usage_logs" ("statusCode", "createdAt" DESC);
PRAGMA user_version = 4;`)
	return err
}
