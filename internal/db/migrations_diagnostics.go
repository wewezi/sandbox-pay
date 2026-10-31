package db

import "database/sql"

func migrateDiagnosticsVersion5(tx *sql.Tx) error {
	_, err := tx.Exec(`
ALTER TABLE "notify_queues" ADD COLUMN "availableAt" datetime;
ALTER TABLE "notify_queues" ADD COLUMN "leaseUntil" datetime;
ALTER TABLE "notify_queues" ADD COLUMN "sourceNotificationId" integer;
ALTER TABLE "notify_queues" ADD COLUMN "simulationMode" text NOT NULL DEFAULT '';
CREATE INDEX "IDX_notify_due" ON "notify_queues" ("status", "availableAt", "leaseUntil", "createdAt");
CREATE TABLE "notify_attempts" (
 "id" integer PRIMARY KEY AUTOINCREMENT,
 "notifyId" integer NOT NULL REFERENCES "notify_queues" ("id") ON DELETE CASCADE,
 "attemptNo" integer NOT NULL CHECK ("attemptNo" > 0),
 "httpStatus" integer,
 "responseSummary" text NOT NULL DEFAULT '',
 "durationMs" integer NOT NULL DEFAULT 0,
 "errorMessage" text NOT NULL DEFAULT '',
 "createdAt" datetime NOT NULL,
 UNIQUE ("notifyId", "attemptNo")
);
CREATE INDEX "IDX_notify_attempt_history" ON "notify_attempts" ("notifyId", "attemptNo");`)
	return err
}
