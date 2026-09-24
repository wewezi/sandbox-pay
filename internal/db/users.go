package db

import "database/sql"

// UsersRepo users 的数据访问。
type UsersRepo struct {
	ex executor
}

func scanUser(sc scanner) (*User, error) {
	var (
		u       User
		created sql.NullString
		updated sql.NullString
	)
	if err := sc.Scan(&u.ID, &u.Username, &u.Password, &u.Role, &created, &updated); err != nil {
		return nil, err
	}
	u.CreatedAt = fromSQL(created.String)
	u.UpdatedAt = fromSQL(updated.String)
	return &u, nil
}

// FindByUsername 按用户名查，找不到返回 (nil, nil)。
func (r *UsersRepo) FindByUsername(username string) (*User, error) {
	row := r.ex.QueryRow(
		`SELECT "id", "username", "password", "role", "createdAt", "updatedAt"
		 FROM "users" WHERE "username" = ?`, username,
	)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// Insert 建用户。
func (r *UsersRepo) Insert(username, password, role string) error {
	_, err := r.ex.Exec(
		`INSERT INTO "users" ("username", "password", "role") VALUES (?, ?, ?)`,
		username, password, role,
	)
	return err
}

// UpdateCredentials 改密码 / 改角色，create-admin 重复执行时走这条。
func (r *UsersRepo) UpdateCredentials(username, password, role string) error {
	_, err := r.ex.Exec(
		`UPDATE "users" SET "password" = ?, "role" = ?, "updatedAt" = ? WHERE "username" = ?`,
		password, role, nowSQL(), username,
	)
	return err
}
