package db

import (
	"database/sql"
	"fmt"
	"time"
)

// NotifyRepo notify_queues 的数据访问。
type NotifyRepo struct {
	ex executor
}

var notifyColumns = `"id", "orderNo", "url", "body", "signature",
  "retryCount", "status", "createdAt", "lastAttemptAt", "lastError", "availableAt"`

func scanNotify(sc scanner) (*NotifyQueue, error) {
	var (
		n           NotifyQueue
		orderNo     sql.NullString
		createdAt   sql.NullString
		lastAttempt sql.NullString
		lastError   sql.NullString
		availableAt sql.NullString
	)
	if err := sc.Scan(&n.ID, &orderNo, &n.URL, &n.Body, &n.Signature,
		&n.RetryCount, &n.Status, &createdAt, &lastAttempt, &lastError, &availableAt); err != nil {
		return nil, err
	}
	n.OrderNo = nullString(orderNo)
	n.CreatedAt = fromSQL(createdAt.String)
	n.LastAttemptAt = nullTime(lastAttempt)
	n.LastError = nullString(lastError)
	n.AvailableAt = nullTime(availableAt)
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
	OrderNo     *string
	URL         string
	Body        string
	Signature   string
	Status      int
	AvailableAt *time.Time
}

// Insert 入队一条回调。
func (r *NotifyRepo) Insert(in InsertNotifyInput) error {
	_, err := r.InsertReturningID(in)
	return err
}

func (r *NotifyRepo) InsertReturningID(in InsertNotifyInput) (int64, error) {
	var available any
	if in.AvailableAt != nil {
		available = toSQL(*in.AvailableAt)
	}
	result, err := r.ex.Exec(`INSERT INTO "notify_queues" ("orderNo", "url", "body", "signature", "status", "availableAt") VALUES (?, ?, ?, ?, ?, ?)`, in.OrderNo, in.URL, in.Body, in.Signature, in.Status, available)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FindRetryable 重试扫描：取待发和失败的，按入队顺序。
func (r *NotifyRepo) FindRetryable(limit int64) ([]*NotifyQueue, error) {
	rows, err := r.ex.Query(
		`SELECT `+notifyColumns+` FROM "notify_queues"
		 WHERE "status" IN (?, ?)
		 AND ("availableAt" IS NULL OR "availableAt" <= ?)
		 AND ("leaseUntil" IS NULL OR "leaseUntil" <= ?)
		 ORDER BY "createdAt" ASC LIMIT ?`,
		int64(NotifyPending), int64(NotifyFailed), toSQL(time.Now()), toSQL(time.Now()), limit,
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
	ProjectIDs []int64
	Status     *int
	OrderNo    string
	Offset     int64
	Limit      int64
}

// Search 后台通知列表：分页 + 过滤。
func (r *NotifyRepo) Search(opts NotifySearchOpts) ([]*NotifyQueue, int64, error) {
	w := &Where{}
	if opts.ProjectIDs != nil {
		clause, args := ProjectScopeSQL(`"groupId"`, opts.ProjectIDs)
		w.Add(`"orderNo" IN (SELECT "orderNo" FROM "payment_orders" WHERE `+clause+`)`, args...)
	}
	if opts.Status != nil {
		w.Add(`"status" = ?`, int64(*opts.Status))
	}
	if opts.OrderNo != "" {
		w.Add(`"orderNo" = ?`, opts.OrderNo)
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
	_, err := r.ex.Exec(`DELETE FROM "sqlite_sequence" WHERE "name" IN ('notify_queues', 'notify_attempts')`)
	return err
}
