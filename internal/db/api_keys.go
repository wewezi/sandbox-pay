package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
)

// HashAPIKey hashes the original credential, including its case and prefix.
// A stored hash cannot itself be used as a Bearer token.
func HashAPIKey(key string) string {
	digest := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func keyDisplayParts(key string) (string, string) {
	characters := []rune(key)
	prefixLength := min(16, len(characters)/2)
	suffixLength := min(4, (len(characters)-prefixLength)/2)
	return string(characters[:prefixLength]), string(characters[len(characters)-suffixLength:])
}

func generateWebhookSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(buf), nil
}

// ApiKeysRepo api_keys 的数据访问。
type ApiKeysRepo struct {
	ex executor
}

// apiKeyAllColumns 单查 api_keys 表时的 SELECT 列表，不带前导逗号。
// （带逗号的那份在 orders.go，只用于接在订单列后面。）
const apiKeyAllColumns = `k."id", k."groupId", k."name", k."key",
  k."isActive", k."lastUsedAt", k."createdAt", k."updatedAt", k."keyPrefix", k."keySuffix"`

func scanAPIKey(sc scanner) (*APIKey, error) {
	var (
		k        APIKey
		isActive int64
		lastUsed sql.NullString
		created  sql.NullString
		updated  sql.NullString
	)
	if err := sc.Scan(&k.ID, &k.GroupID, &k.Name, &k.KeyHash,
		&isActive, &lastUsed, &created, &updated, &k.KeyPrefix, &k.KeySuffix); err != nil {
		return nil, err
	}
	k.IsActive = boolFromInt(isActive)
	k.LastUsedAt = nullTime(lastUsed)
	k.CreatedAt = fromSQL(created.String)
	k.UpdatedAt = fromSQL(updated.String)
	return &k, nil
}

func scanGroup(sc scanner) (*Group, error) {
	var (
		g        Group
		isActive int64
		created  sql.NullString
		updated  sql.NullString
	)
	if err := sc.Scan(&g.ID, &g.Name, &isActive, &created, &updated, &g.WebhookSecret); err != nil {
		return nil, err
	}
	g.IsActive = boolFromInt(isActive)
	g.CreatedAt = fromSQL(created.String)
	g.UpdatedAt = fromSQL(updated.String)
	return &g, nil
}

// selectKey 按别名选一把 Key。alias 为 "k" 时列带 k_ 前缀语义（和 JOIN 查询共用列序）。
func (r *ApiKeysRepo) find(query string, args ...any) (*APIKey, error) {
	row := r.ex.QueryRow(query, args...)
	k, err := scanAPIKey(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return k, err
}

// FindByID 按 id 查，找不到返回 (nil, nil)。
func (r *ApiKeysRepo) FindByID(id int64) (*APIKey, error) {
	return r.find(`SELECT `+apiKeyAllColumns+` FROM "api_keys" k WHERE k."id" = ?`, id)
}

// FindByKey 按 key 全局查一行，不管有没有停用。
//
// 和 FindActiveByKey 的区别是不过滤 isActive —— 种子逻辑要靠它判断
// .env 里那把 Key 是不是已经在库里（只是被停用了），
// 否则重复 insert 会撞 UQ_api_keys_key。
func (r *ApiKeysRepo) FindByKey(key string) (*APIKey, error) {
	return r.find(`SELECT `+apiKeyAllColumns+` FROM "api_keys" k WHERE k."key" = ?`, HashAPIKey(key))
}

// FindActiveByKey 入站鉴权：key 对得上、Key 自己没停、分组也没停。
// 一次 JOIN 把两边都拿回来，免得守卫再查一次。
func (r *ApiKeysRepo) FindActiveByKey(key string) (*ActiveKeyHit, error) {
	row := r.ex.QueryRow(`
		SELECT `+apiKeyAllColumns+`,
		   m."id", m."name", m."isActive", m."createdAt", m."updatedAt", m."webhookSecret"
		 FROM "api_keys" k
		 JOIN "groups" m ON m."id" = k."groupId"
		 WHERE k."key" = ? AND k."isActive" = 1 AND m."isActive" = 1`, HashAPIKey(key))

	var (
		k         APIKey
		kIsActive int64
		kLastUsed sql.NullString
		kCreated  sql.NullString
		kUpdated  sql.NullString
		g         Group
		gIsActive int64
		gCreated  sql.NullString
		gUpdated  sql.NullString
	)
	err := row.Scan(&k.ID, &k.GroupID, &k.Name, &k.KeyHash,
		&kIsActive, &kLastUsed, &kCreated, &kUpdated, &k.KeyPrefix, &k.KeySuffix,
		&g.ID, &g.Name, &gIsActive, &gCreated, &gUpdated, &g.WebhookSecret)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	k.IsActive = boolFromInt(kIsActive)
	k.LastUsedAt = nullTime(kLastUsed)
	k.CreatedAt = fromSQL(kCreated.String)
	k.UpdatedAt = fromSQL(kUpdated.String)
	g.IsActive = boolFromInt(gIsActive)
	g.CreatedAt = fromSQL(gCreated.String)
	g.UpdatedAt = fromSQL(gUpdated.String)
	return &ActiveKeyHit{Key: &k, Group: &g}, nil
}

// ListByGroup 分组名下的所有 Key，按 id 升序。
func (r *ApiKeysRepo) ListByGroup(groupID int64) ([]*APIKey, error) {
	rows, err := r.ex.Query(
		`SELECT `+apiKeyAllColumns+` FROM "api_keys" k WHERE k."groupId" = ? ORDER BY k."id" ASC`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// InsertKeyInput 建 Key 的入参。
type InsertKeyInput struct {
	GroupID  int64
	Name     string
	Key      string
	IsActive bool
}

// Insert 签发一把 Key，返回建好的行。
func (r *ApiKeysRepo) Insert(in InsertKeyInput) (*APIKey, error) {
	prefix, suffix := keyDisplayParts(in.Key)
	res, err := r.ex.Exec(
		`INSERT INTO "api_keys" ("groupId", "name", "key", "isActive", "keyPrefix", "keySuffix") VALUES (?, ?, ?, ?, ?, ?)`,
		in.GroupID, in.Name, HashAPIKey(in.Key), boolToInt(in.IsActive), prefix, suffix,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	key, err := r.FindByID(id)
	if err != nil {
		return nil, err
	}
	key.Key = in.Key
	return key, nil
}

// Update 按 id 局部更新，顺手刷 updatedAt。patch 的 key 是列名，nil 值写 NULL。
func (r *ApiKeysRepo) Update(id int64, patch map[string]any) error {
	if patch == nil {
		return nil
	}
	patch["updatedAt"] = nowSQL()
	set, args := setClause(patch)
	if set == "" {
		return nil
	}
	_, err := r.ex.Exec(
		`UPDATE "api_keys" SET `+set+` WHERE "id" = ?`,
		append(args, id)...,
	)
	return err
}

// Touch 守卫校验通过后刷一下，用来判断哪把 Key 还在用。失败不影响鉴权。
func (r *ApiKeysRepo) Touch(id int64) error {
	_, err := r.ex.Exec(`UPDATE "api_keys" SET "lastUsedAt" = ? WHERE "id" = ?`, nowSQL(), id)
	return err
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
