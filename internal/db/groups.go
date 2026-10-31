package db

import "database/sql"

// GroupsRepo groups 的数据访问。
type GroupsRepo struct {
	ex executor
}

// FindFirst 沙箱种子分组：按 id 升序取第一条，给测试下单这种没有 Key 上下文的路径用。
func (r *GroupsRepo) FindFirst() (*Group, error) {
	row := r.ex.QueryRow(`SELECT "id", "name", "isActive", "createdAt", "updatedAt", "webhookSecret"
		FROM "groups" ORDER BY "id" ASC LIMIT 1`)
	g, err := scanGroup(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

// FindByID 按 id 查，找不到返回 (nil, nil)。
func (r *GroupsRepo) FindByID(id int64) (*Group, error) {
	row := r.ex.QueryRow(`SELECT "id", "name", "isActive", "createdAt", "updatedAt", "webhookSecret"
		FROM "groups" WHERE "id" = ?`, id)
	g, err := scanGroup(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

// List 所有分组，按 id 升序。
func (r *GroupsRepo) List(scopes ...[]int64) ([]*Group, error) {
	w := &Where{}
	w.AddProjectScope(`"id"`, firstProjectScope(scopes))
	rows, err := r.ex.Query(`SELECT "id", "name", "isActive", "createdAt", "updatedAt", "webhookSecret"
		FROM "groups"`+w.SQL()+` ORDER BY "id" ASC`, w.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Insert 新建分组，返回建好的行。
func (r *GroupsRepo) Insert(name string, isActive bool) (*Group, error) {
	secret, err := generateWebhookSecret()
	if err != nil {
		return nil, err
	}
	res, err := r.ex.Exec(
		`INSERT INTO "groups" ("name", "isActive", "webhookSecret") VALUES (?, ?, ?)`,
		name, boolToInt(isActive), secret,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.FindByID(id)
}

// Update 按 id 局部更新，顺手刷 updatedAt。patch 的 key 是列名。
func (r *GroupsRepo) Update(id int64, patch map[string]any) error {
	if patch == nil {
		return nil
	}
	patch["updatedAt"] = nowSQL()
	set, args := setClause(patch)
	if set == "" {
		return nil
	}
	_, err := r.ex.Exec(
		`UPDATE "groups" SET `+set+` WHERE "id" = ?`,
		append(args, id)...,
	)
	return err
}
