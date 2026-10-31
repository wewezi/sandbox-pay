package db

import "database/sql"

// Existing users had global administrative access before project membership.
// Preserve that access during upgrade; newly created Admin users are scoped.
func migrateTeamVersion5(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE "users" ADD COLUMN "isActive" INTEGER NOT NULL DEFAULT 1 CHECK ("isActive" IN (0,1));
ALTER TABLE "users" ADD COLUMN "sessionVersion" INTEGER NOT NULL DEFAULT 1;
UPDATE "users" SET "role" = 'SuperAdmin' WHERE "role" = 'Admin';
CREATE TABLE "admin_sessions" (
  "id" TEXT PRIMARY KEY NOT NULL,
  "userId" INTEGER NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "sessionVersion" INTEGER NOT NULL,
  "clientIp" TEXT NOT NULL DEFAULT '',
  "userAgent" TEXT NOT NULL DEFAULT '',
  "createdAt" DATETIME NOT NULL,
  "lastSeenAt" DATETIME NOT NULL,
  "expiresAt" DATETIME NOT NULL,
  "revokedAt" DATETIME
);
CREATE INDEX "IDX_sessions_user" ON "admin_sessions"("userId", "expiresAt");
CREATE TABLE "project_members" (
  "groupId" INTEGER NOT NULL REFERENCES "groups"("id") ON DELETE CASCADE,
  "userId" INTEGER NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "createdAt" DATETIME NOT NULL,
  PRIMARY KEY ("groupId", "userId")
);
CREATE INDEX "IDX_project_members_user" ON "project_members"("userId", "groupId");`)
	return err
}
