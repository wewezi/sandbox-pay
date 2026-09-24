package db

import (
	"database/sql"
	"fmt"
)

// NotifyRepo notify_queues 的数据访问。
type NotifyRepo struct {
	ex executor
}

var notifyColumns = `"id", "orderNo", "url", "body", "signature",
  "retryCount", "status", "createdAt", "lastAttemptAt", "lastError"`

func scanNotify(sc scanner) (*NotifyQueue, error) {
	var (
		n           NotifyQueue
		orderNo     sql.NullString
		createdAt   sql.NullString
		lastAttempt sql.NullString
		lastError   sql.NullString
	)
	if err := sc.Scan(&n.ID, &orderNo, &n.URL, &n.Body, &n.Signature,
		&n.RetryCount, &n.Status, &createdAt, &lastAttempt, &lastError); err != nil {
		return nil, err
	}
	n.OrderNo = nullString(orderNo)
	n.CreatedAt = fromSQL(createdAt.String)
	n.LastAttemptAt = nullTime(lastAttempt)
	n.LastError = nullString(lastError)
	return &n, nil
}

// FindByID 按 id 查，找不到返回 (nil, nil)。
func (r *NotifyRepo) FindByID(id int64) (*NotifyQueue, error) {
	row := r.ex.QueryRow(`SELECT `+notifyColumns+` FROM "notify_queues" WHERE "id" = ?`, id)
	n, err := scanNotify(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return n, err
}

// InsertNotifyInput 入队的行。
type InsertNotifyInput struct {
	OrderNo   *string
	URL       string
	Body      string
	Signature string
	Status    int
}

// Insert 入队一条回调。
func (r *NotifyRepo) Insert(in InsertNotifyInput) error {
	_, err := r.ex.Exec(
		`INSERT INTO "notify_queues" ("orderNo", "url", "body", "signature", "status")
		 VALUES (?, ?, ?, ?, ?)`,
		in.OrderNo, in.URL, in.Body, in.Signature, in.Status,
	)
	return err
}

// FindRetryable 重试扫描：取待发和失败的，按入队顺序。
func (r *NotifyRepo) FindRetryable(limit int64) ([]*NotifyQueue, error) {
	rows, err := r.ex.Query(
		`SELECT `+notifyColumns+` FROM "notify_queues"
		 WHERE "status" IN (?, ?)
		 ORDER BY "createdAt" ASC LIMIT ?`,
		int64(NotifyPending), int64(NotifyFailed), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NotifyQueue
	for rows.Next() {
		n, err := scanNotify(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Update 按 id 局部更新。patch 的 key 是列名，nil 值写 NULL（清空 lastError 靠这个）。
func (r *NotifyRepo) Update(id int64, patch map[string]any) error {
	if patch == nil {
		return nil
	}
	set, args := setClause(patch)
	if set == "" {
		return nil
	}
	_, err := r.ex.Exec(
		fmt.Sprintf(`UPDATE "notify_queues" SET %s WHERE "id" = ?`, set),
		append(args, id)...,
	)
	return err
}

// SearchOpts 通知队列的过滤 + 分页。
type NotifySearchOpts struct {
	Status *int
	Offset int64
	Limit  int64
}

// Search 后台通知列表：分页 + 过滤。
func (r *NotifyRepo) Search(opts NotifySearchOpts) ([]*NotifyQueue, int64, error) {
	w := &Where{}
	if opts.Status != nil {
		w.Add(`"status" = ?`, int64(*opts.Status))
	}

	var total int64
	if err := r.ex.QueryRow(
		`SELECT COUNT(*) FROM "notify_queues"`+w.SQL(), w.Args()...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.ex.Query(
		`SELECT `+notifyColumns+` FROM "notify_queues"`+w.SQL()+`
		 ORDER BY "createdAt" DESC LIMIT ? OFFSET ?`,
		append(append([]any{}, w.Args()...), opts.Limit, opts.Offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*NotifyQueue
	for rows.Next() {
		n, err := scanNotify(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, n)
	}
	return out, total, rows.Err()
}

// Count 队列条数。
func (r *NotifyRepo) Count() (int64, error) {
	var c int64
	err := r.ex.QueryRow(`SELECT COUNT(*) FROM "notify_queues"`).Scan(&c)
	return c, err
}

// Clear 清空并把自增序列归零。
func (r *NotifyRepo) Clear() error {
	if _, err := r.ex.Exec(`DELETE FROM "notify_queues"`); err != nil {
		return err
	}
	_, err := r.ex.Exec(`DELETE FROM "sqlite_sequence" WHERE "name" = ?`, "notify_queues")
	return err
}
