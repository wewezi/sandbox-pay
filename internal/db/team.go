package db

import (
	"database/sql"
	"time"
)

const userColumns = `"id", "username", "password", "role", "createdAt", "updatedAt", "isActive", "sessionVersion"`

func (r *UsersRepo) FindByID(id int64) (*User, error) {
	u, err := scanUser(r.ex.QueryRow(`SELECT `+userColumns+` FROM "users" WHERE "id" = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (r *UsersRepo) List() ([]*User, error) {
	rows, err := r.ex.Query(`SELECT ` + userColumns + ` FROM "users" ORDER BY "id"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]*User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UsersRepo) CountActiveSuperAdmins() (int64, error) {
	var count int64
	err := r.ex.QueryRow(`SELECT COUNT(*) FROM "users" WHERE "role" = 'SuperAdmin' AND "isActive" = 1`).Scan(&count)
	return count, err
}

func (r *UsersRepo) SetAccess(id int64, role string, active bool) error {
	_, err := r.ex.Exec(`UPDATE "users" SET "role" = ?, "isActive" = ?, "sessionVersion" = "sessionVersion" + 1, "updatedAt" = ? WHERE "id" = ?`, role, active, nowSQL(), id)
	return err
}

func (r *UsersRepo) SetPassword(id int64, hash string) error {
	_, err := r.ex.Exec(`UPDATE "users" SET "password" = ?, "sessionVersion" = "sessionVersion" + 1, "updatedAt" = ? WHERE "id" = ?`, hash, nowSQL(), id)
	return err
}

func (r *UsersRepo) ProjectIDs(userID int64) ([]int64, error) {
	rows, err := r.ex.Query(`SELECT "groupId" FROM "project_members" WHERE "userId" = ? ORDER BY "groupId"`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *UsersRepo) SetProjectIDs(userID int64, ids []int64) error {
	if _, err := r.ex.Exec(`DELETE FROM "project_members" WHERE "userId" = ?`, userID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := r.ex.Exec(`INSERT INTO "project_members"("groupId", "userId", "createdAt") VALUES (?, ?, ?)`, id, userID, nowSQL()); err != nil {
			return err
		}
	}
	return nil
}

type AdminSession struct {
	ID             string
	UserID         int64
	SessionVersion int64
	ClientIP       string
	UserAgent      string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}

const sessionColumns = `"id", "userId", "sessionVersion", "clientIp", "userAgent", "createdAt", "lastSeenAt", "expiresAt", "revokedAt"`

func scanSession(row scanner) (*AdminSession, error) {
	var s AdminSession
	var created, seen, expires string
	var revoked sql.NullString
	if err := row.Scan(&s.ID, &s.UserID, &s.SessionVersion, &s.ClientIP, &s.UserAgent, &created, &seen, &expires, &revoked); err != nil {
		return nil, err
	}
	s.CreatedAt, s.LastSeenAt, s.ExpiresAt = fromSQL(created), fromSQL(seen), fromSQL(expires)
	s.RevokedAt = nullTime(revoked)
	return &s, nil
}

func (r *UsersRepo) InsertSession(s *AdminSession) error {
	_, err := r.ex.Exec(`INSERT INTO "admin_sessions" (`+sessionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`, s.ID, s.UserID, s.SessionVersion, s.ClientIP, s.UserAgent, toSQL(s.CreatedAt), toSQL(s.LastSeenAt), toSQL(s.ExpiresAt))
	return err
}

func (r *UsersRepo) FindSession(id string) (*AdminSession, error) {
	s, err := scanSession(r.ex.QueryRow(`SELECT `+sessionColumns+` FROM "admin_sessions" WHERE "id" = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *UsersRepo) ListSessions(userID, version int64) ([]*AdminSession, error) {
	rows, err := r.ex.Query(`SELECT `+sessionColumns+` FROM "admin_sessions" WHERE "userId" = ? AND "sessionVersion" = ? AND "revokedAt" IS NULL AND "expiresAt" > ? ORDER BY "createdAt" DESC`, userID, version, nowSQL())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*AdminSession, 0)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *UsersRepo) RevokeSession(id string, userID int64) error {
	_, err := r.ex.Exec(`UPDATE "admin_sessions" SET "revokedAt" = ? WHERE "id" = ? AND "userId" = ? AND "revokedAt" IS NULL`, nowSQL(), id, userID)
	return err
}

func (r *UsersRepo) RevokeOtherSessions(userID int64, currentID string) error {
	_, err := r.ex.Exec(`UPDATE "admin_sessions" SET "revokedAt" = ? WHERE "userId" = ? AND "id" != ? AND "revokedAt" IS NULL`, nowSQL(), userID, currentID)
	return err
}

func (r *UsersRepo) RevokeUserSessions(userID int64) error {
	_, err := r.ex.Exec(`UPDATE "admin_sessions" SET "revokedAt" = ? WHERE "userId" = ? AND "revokedAt" IS NULL`, nowSQL(), userID)
	return err
}

func (r *UsersRepo) TouchSession(id string) error {
	_, err := r.ex.Exec(`UPDATE "admin_sessions" SET "lastSeenAt" = ? WHERE "id" = ? AND "lastSeenAt" < ?`, nowSQL(), id, toSQL(time.Now().UTC().Add(-5*time.Minute)))
	return err
}

func (r *UsersRepo) PruneSessions() error {
	_, err := r.ex.Exec(`DELETE FROM "admin_sessions" WHERE "expiresAt" < ?`, toSQL(time.Now().UTC().Add(-7*24*time.Hour)))
	return err
}
