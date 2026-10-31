package db

import "database/sql"

func migrateSandboxVersion5(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE "sandbox_tasks" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "orderNo" TEXT NOT NULL,
  "scenario" TEXT NOT NULL,
  "status" TEXT NOT NULL CHECK ("status" IN ('pending','running','completed','failed','cancelled')),
  "amount" INTEGER NOT NULL DEFAULT 0,
  "runAt" DATETIME NOT NULL,
  "createdAt" DATETIME NOT NULL,
  "updatedAt" DATETIME NOT NULL,
  "lastError" TEXT NOT NULL DEFAULT ''
);
CREATE INDEX "IDX_sandbox_tasks_due" ON "sandbox_tasks" ("status", "runAt", "id");
CREATE INDEX "IDX_sandbox_tasks_order" ON "sandbox_tasks" ("orderNo", "id" DESC);`)
	return err
}
