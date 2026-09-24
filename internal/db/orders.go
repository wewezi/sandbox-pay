package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// OrdersRepo payment_orders 的数据访问。
// 需要 order.APIKey.Key 签回调的地方走 withAPIKey 版本，其余查询不 JOIN。
type OrdersRepo struct {
	ex executor
}

var orderColumns = []string{
	"id", "orderNo", "amount", "productName", "payMethod", "status",
	"createdAt", "updatedAt", "expireAt", "payAt", "exchangeRate",
	"foreignAmount", "foreignCurrency", "externalOrderNo", "thirdPartyTradeNo",
	"refundedAmount", "refundTradeNo", "refundAt", "channelCost", "fee",
	"settleAmount", "returnUrl", "notifyUrl", "groupId", "apiKeyId", "userId",
}

// joinAPIKey 只有需要签回调时才 JOIN，Key 列加 k_ 前缀避免和订单列撞名。
const joinAPIKey = ` LEFT JOIN "api_keys" k ON k."id" = o."apiKeyId"`

// apiKeyColumns 拼在订单列**之后**的 Key 列，所以带前导逗号 ——
// 它只给 orders.go 的 selectSQL 用。单查 api_keys 表要用下面的
// apiKeyAllColumns（不带逗号），拼上去会变成 `SELECT , k."id"` 直接语法错误。
const apiKeyColumns = `,
  k."id", k."groupId", k."name", k."key",
  k."isActive", k."lastUsedAt", k."createdAt", k."updatedAt"`

func selectSQL(withAPIKey bool) string {
	cols := make([]string, 0, len(orderColumns))
	for _, c := range orderColumns {
		cols = append(cols, `o.`+fmt.Sprintf("%q", c))
	}
	sqlText := `SELECT ` + strings.Join(cols, ", ")
	// apiKeyColumns 的前导逗号就是给这里留的：它得接在订单列后面。
	if withAPIKey {
		sqlText += apiKeyColumns
	}
	sqlText += ` FROM "payment_orders" o`
	if withAPIKey {
		sqlText += joinAPIKey
	}
	return sqlText
}

// scanner *sql.Row / *sql.Rows 都满足。
type scanner interface {
	Scan(dest ...any) error
}

func nullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := fromSQL(ns.String)
	return &t
}

func nullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func nullInt64(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	v := ni.Int64
	return &v
}

func nullFloat64(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	v := nf.Float64
	return &v
}

func boolFromInt(n int64) bool { return n != 0 }

func scanOrder(sc scanner, withAPIKey bool) (*PaymentOrder, error) {
	var (
		o          PaymentOrder
		created    sql.NullString
		updated    sql.NullString
		expire     sql.NullString
		payAt      sql.NullString
		refundAt   sql.NullString
		exRate     sql.NullFloat64
		foreignAmt sql.NullInt64
		foreignCur sql.NullString
		extOrderNo sql.NullString
		thirdParty sql.NullString
		refundTrNo sql.NullString
		returnURL  sql.NullString
		notifyURL  sql.NullString
		groupID    sql.NullInt64
		apiKeyID   sql.NullInt64
	)

	dest := []any{
		&o.ID, &o.OrderNo, &o.Amount, &o.ProductName, &o.PayMethod, &o.Status,
		&created, &updated, &expire, &payAt, &exRate,
		&foreignAmt, &foreignCur, &extOrderNo, &thirdParty,
		&o.RefundedAmount, &refundTrNo, &refundAt, &o.ChannelCost, &o.Fee,
		&o.SettleAmount, &returnURL, &notifyURL, &groupID, &apiKeyID, &o.UserID,
	}
	if withAPIKey {
		// apiKeyId 为空（后台/测试下单）时 LEFT JOIN 全是 NULL ——
		// 直接扫进 int64 会报「converting NULL」，所以 Key 的列走 Null* 类型，
		// 扫完再决定 o.APIKey 是不是 nil。签回调的地方靠它判「这单有没有 Key」。
		var keyID, keyGroupID sql.NullInt64
		var keyName, keyValue sql.NullString
		var keyActive sql.NullInt64
		var keyLastUsed, keyCreated, keyUpdated sql.NullString
		dest = append(dest,
			&keyID, &keyGroupID, &keyName, &keyValue,
			&keyActive, &keyLastUsed, &keyCreated, &keyUpdated)
		if err := sc.Scan(dest...); err != nil {
			return nil, err
		}
		if !keyID.Valid {
			o.APIKey = nil
		} else {
			key := &APIKey{
				ID:         keyID.Int64,
				GroupID:    keyGroupID.Int64,
				Name:       keyName.String,
				Key:        keyValue.String,
				IsActive:   boolFromInt(keyActive.Int64),
				LastUsedAt: nullTime(keyLastUsed),
				CreatedAt:  fromSQL(keyCreated.String),
				UpdatedAt:  fromSQL(keyUpdated.String),
			}
			o.APIKey = key
		}
	} else {
		if err := sc.Scan(dest...); err != nil {
			return nil, err
		}
	}

	o.CreatedAt = fromSQL(created.String)
	o.UpdatedAt = fromSQL(updated.String)
	o.ExpireAt = fromSQL(expire.String)
	o.PayAt = nullTime(payAt)
	o.RefundAt = nullTime(refundAt)
	o.ExchangeRate = nullFloat64(exRate)
	o.ForeignAmount = nullInt64(foreignAmt)
	o.ForeignCurrency = nullString(foreignCur)
	o.ExternalOrderNo = nullString(extOrderNo)
	o.ThirdPartyTradeNo = nullString(thirdParty)
	o.RefundTradeNo = nullString(refundTrNo)
	o.ReturnURL = nullString(returnURL)
	o.NotifyURL = nullString(notifyURL)
	o.GroupID = nullInt64(groupID)
	o.APIKeyID = nullInt64(apiKeyID)
	return &o, nil
}

func (r *OrdersRepo) one(query string, args ...any) (*PaymentOrder, error) {
	row := r.ex.QueryRow(query, args...)
	order, err := scanOrder(row, strings.Contains(query, `k."id"`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return order, err
}

// FindByOrderNo 按订单号查。找不到返回 (nil, nil)。
func (r *OrdersRepo) FindByOrderNo(orderNo string, withAPIKey bool) (*PaymentOrder, error) {
	return r.one(selectSQL(withAPIKey)+` WHERE o."orderNo" = ?`, orderNo)
}

// FindByOrderNoAndGroup 分组隔离的查单：别的分组查不到这一单。
func (r *OrdersRepo) FindByOrderNoAndGroup(orderNo string, groupID int64) (*PaymentOrder, error) {
	return r.one(selectSQL(false)+` WHERE o."orderNo" = ? AND o."groupId" = ?`, orderNo, groupID)
}

// FindLatestByExternalOrderNo 幂等去重用：同分组同 externalOrderNo 取最新一条。
func (r *OrdersRepo) FindLatestByExternalOrderNo(groupID int64, externalOrderNo string) (*PaymentOrder, error) {
	return r.one(selectSQL(false)+` WHERE o."groupId" = ? AND o."externalOrderNo" = ?
       ORDER BY o."createdAt" DESC LIMIT 1`, groupID, externalOrderNo)
}

// many 从 *sql.Rows 扫出多行。withAPIKey 由查询文本决定。
func (r *OrdersRepo) many(query string, args ...any) ([]*PaymentOrder, error) {
	rows, err := r.ex.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PaymentOrder
	for rows.Next() {
		order, err := scanOrder(rows, strings.Contains(query, `k."id"`))
		if err != nil {
			return nil, err
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

// Insert 建单。createdAt/updatedAt 一律取当前时间（和原版 @CreateDateColumn 一致）。
func (r *OrdersRepo) Insert(o *PaymentOrder) error {
	now := nowSQL()
	cols := make([]string, 0, len(orderColumns))
	placeholders := make([]string, 0, len(orderColumns))
	args := make([]any, 0, len(orderColumns))
	vals := map[string]any{
		"id": o.ID, "orderNo": o.OrderNo, "amount": o.Amount,
		"productName": o.ProductName, "payMethod": o.PayMethod, "status": o.Status,
		"expireAt": o.ExpireAt, "payAt": o.PayAt, "exchangeRate": o.ExchangeRate,
		"foreignAmount": o.ForeignAmount, "foreignCurrency": o.ForeignCurrency,
		"externalOrderNo": o.ExternalOrderNo, "thirdPartyTradeNo": o.ThirdPartyTradeNo,
		"refundedAmount": o.RefundedAmount, "refundTradeNo": o.RefundTradeNo,
		"refundAt": o.RefundAt, "channelCost": o.ChannelCost, "fee": o.Fee,
		"settleAmount": o.SettleAmount, "returnUrl": o.ReturnURL,
		"notifyUrl": o.NotifyURL, "groupId": o.GroupID, "apiKeyId": o.APIKeyID,
		"userId": o.UserID,
	}
	for _, c := range orderColumns {
		cols = append(cols, fmt.Sprintf("%q", c))
		placeholders = append(placeholders, "?")
		switch c {
		case "createdAt", "updatedAt":
			args = append(args, now)
		default:
			args = append(args, bindValue(vals[c]))
		}
	}
	_, err := r.ex.Exec(
		fmt.Sprintf(`INSERT INTO "payment_orders" (%s) VALUES (%s)`,
			strings.Join(cols, ", "), strings.Join(placeholders, ", ")),
		args...,
	)
	return err
}

// Update 按 orderNo 局部更新，顺手刷 updatedAt（原版由 @UpdateDateColumn 负责）。
// patch 的 key 是列名，值为 nil 表示写 NULL。
func (r *OrdersRepo) Update(orderNo string, patch map[string]any) error {
	if patch == nil {
		patch = map[string]any{}
	}
	patch["updatedAt"] = time.Now()
	set, args := setClause(patch)
	if set == "" {
		return nil
	}
	_, err := r.ex.Exec(
		`UPDATE "payment_orders" SET `+set+` WHERE "orderNo" = ?`,
		append(args, orderNo)...,
	)
	return err
}

// DeleteByOrderNo 删单。
func (r *OrdersRepo) DeleteByOrderNo(orderNo string) error {
	_, err := r.ex.Exec(`DELETE FROM "payment_orders" WHERE "orderNo" = ?`, orderNo)
	return err
}

// Count 总单数。
func (r *OrdersRepo) Count() (int64, error) {
	var c int64
	err := r.ex.QueryRow(`SELECT COUNT(*) FROM "payment_orders"`).Scan(&c)
	return c, err
}

// Clear 清空并把自增序列归零。
func (r *OrdersRepo) Clear() error {
	if _, err := r.ex.Exec(`DELETE FROM "payment_orders"`); err != nil {
		return err
	}
	_, err := r.ex.Exec(`DELETE FROM "sqlite_sequence" WHERE "name" = ?`, "payment_orders")
	return err
}

// ExpirePending 定时任务：把过期的待支付订单批量关掉，返回受影响行数。
func (r *OrdersRepo) ExpirePending(limit int64) (int64, error) {
	res, err := r.ex.Exec(
		`UPDATE "payment_orders" SET "status" = ?, "updatedAt" = ?
		 WHERE "id" IN (
		   SELECT "id" FROM "payment_orders"
		   WHERE "status" = ? AND "expireAt" < ?
		   LIMIT ?
		 )`,
		StatusExpired, nowSQL(), StatusPending, nowSQL(), limit,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SearchOpts 后台列表的过滤 + 分页。
type SearchOpts struct {
	GroupID   *int64
	Status    *int
	PayMethod string
	DateFrom  string // YYYY-MM-DD，含当天 0 点
	DateTo    string // YYYY-MM-DD，含当天 23:59:59
	Keyword   string
	Offset    int64
	Limit     int64
}

// buildWhere 从 opts 拼 WHERE。
func (o SearchOpts) where() *Where {
	w := &Where{}
	if o.GroupID != nil {
		w.Add(`o."groupId" = ?`, *o.GroupID)
	}
	if o.Status != nil {
		w.Add(`o."status" = ?`, int64(*o.Status))
	}
	if o.PayMethod != "" {
		w.Add(`o."payMethod" = ?`, o.PayMethod)
	}
	if o.DateFrom != "" {
		w.Add(`o."createdAt" >= ?`, toSQL(parseDateStart(o.DateFrom)))
	}
	if o.DateTo != "" {
		w.Add(`o."createdAt" <= ?`, toSQL(parseDateEnd(o.DateTo)))
	}
	if o.Keyword != "" {
		like := "%" + o.Keyword + "%"
		w.Add(`(o."orderNo" LIKE ? OR o."externalOrderNo" LIKE ?)`, like, like)
	}
	return w
}

// parseDateStart "YYYY-MM-DD" → 当天 00:00:00.000 (UTC)。解析不了就原样按零值处理。
func parseDateStart(s string) time.Time {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	if t, err := time.Parse(sqlTimeLayout, s); err == nil {
		return t
	}
	return time.Time{}
}

// parseDateEnd "YYYY-MM-DD" → 当天 23:59:59 (UTC)。
func parseDateEnd(s string) time.Time {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}
	if t, err := time.Parse(sqlTimeLayout, s); err == nil {
		return t
	}
	return time.Time{}
}

// Search 后台列表：分页 + 过滤。
func (r *OrdersRepo) Search(opts SearchOpts) ([]*PaymentOrder, int64, error) {
	where := opts.where()

	var total int64
	if err := r.ex.QueryRow(
		`SELECT COUNT(*) FROM "payment_orders" o`+where.SQL(), where.Args()...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	items, err := r.many(
		selectSQL(false)+where.SQL()+` ORDER BY o."createdAt" DESC LIMIT ? OFFSET ?`,
		append(append([]any{}, where.Args()...), opts.Limit, opts.Offset)...,
	)
	return items, total, err
}

// Stats 后台统计的聚合值。
type Stats struct {
	TotalCount       int64
	SuccessCount     int64
	TotalAmountCents int64
}

// Stats 后台统计：只回聚合值，不把行读进内存。
func (r *OrdersRepo) Stats(groupID *int64) (Stats, error) {
	var out Stats
	w := &Where{}
	w.AddIf(groupID, `"groupId" = ?`, groupID)
	if err := r.ex.QueryRow(
		`SELECT COUNT(*) FROM "payment_orders"`+w.SQL(), w.Args()...,
	).Scan(&out.TotalCount); err != nil {
		return out, err
	}

	paidWhere := &Where{}
	paidWhere.AddIf(groupID, `"groupId" = ?`, groupID)
	paidWhere.Add(`"status" = ?`, int64(StatusPaid))
	if err := r.ex.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM("amount"), 0) FROM "payment_orders"`+paidWhere.SQL(),
		paidWhere.Args()...,
	).Scan(&out.SuccessCount, &out.TotalAmountCents); err != nil {
		return out, err
	}
	return out, nil
}
