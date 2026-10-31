package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DashboardSummary describes a cohort of orders selected by their creation date.
// Paid orders remain successful after a partial or full refund.
type DashboardSummary struct {
	AmountCents         int64
	SuccessCount        int64
	TotalCount          int64
	RefundedAmountCents int64
	FeeCents            int64
}

type DashboardDay struct {
	Date         string
	AmountCents  int64
	SuccessCount int64
	TotalCount   int64
}

type DashboardChannel struct {
	PayMethod string
	DashboardSummary
}

// DashboardOrder 运营面板「最近订单」的一行。
//
// 只取表格要显示的列 —— 订单详情那些字段（回调地址、上游单号）
// 属于订单页，面板里既用不上也不该多带一份。
type DashboardOrder struct {
	OrderNo     string
	ProductName string
	PayMethod   string
	Status      int
	AmountCents int64
	CreatedAt   time.Time
}

// DashboardRecentLimit 「最近订单」条数。正好铺满一屏表格，不用翻页。
const DashboardRecentLimit = 8

type DashboardTasks struct {
	PendingOrders          int64 `json:"pendingOrders"`
	RefundingOrders        int64 `json:"refundingOrders"`
	PendingNotifications   int64 `json:"pendingNotifications"`
	FailedNotifications    int64 `json:"failedNotifications"`
	ExhaustedNotifications int64 `json:"exhaustedNotifications"`
	ActiveGroups           int64 `json:"activeGroups"`
	ActiveKeys             int64 `json:"activeKeys"`
}

type DashboardSnapshot struct {
	Summary  DashboardSummary
	Previous DashboardSummary
	Trend    []DashboardDay
	Channels []DashboardChannel
	// Recent 全部订单里最新的几条，不受统计区间限制 ——
	// 本期没有订单时，面板不该空着。
	Recent []DashboardOrder
	Tasks  DashboardTasks
}

// Dashboard uses one read transaction so its aggregates share the same snapshot.
// The half-open windows preserve midnight boundaries in every requested offset.
func (s *Store) Dashboard(previousStart, start, end time.Time, utcOffset int, scopes ...[]int64) (*DashboardSnapshot, error) {
	projectIDs := firstProjectScope(scopes)
	orderScope, scopeArgs := ProjectScopeSQL(`"groupId"`, projectIDs)
	tx, err := s.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := &DashboardSnapshot{
		Trend: []DashboardDay{}, Channels: []DashboardChannel{}, Recent: []DashboardOrder{},
	}
	if out.Summary, err = dashboardSummary(tx, start, end, projectIDs); err != nil {
		return nil, err
	}
	if out.Previous, err = dashboardSummary(tx, previousStart, start, projectIDs); err != nil {
		return nil, err
	}
	modifier := fmt.Sprintf("%+d minutes", utcOffset)
	rows, err := tx.Query(`SELECT date("createdAt", ?),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN "amount" ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN 1 ELSE 0 END), 0), COUNT(*)
		FROM "payment_orders"
		WHERE julianday("createdAt") >= julianday(?) AND julianday("createdAt") < julianday(?) AND `+orderScope+`
		GROUP BY date("createdAt", ?) ORDER BY date("createdAt", ?)`,
		append(append([]any{modifier, toSQL(start), toSQL(end)}, scopeArgs...), modifier, modifier)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var day DashboardDay
		if err := rows.Scan(&day.Date, &day.AmountCents, &day.SuccessCount, &day.TotalCount); err != nil {
			rows.Close()
			return nil, err
		}
		out.Trend = append(out.Trend, day)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = tx.Query(`SELECT "payMethod",
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN "amount" ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN 1 ELSE 0 END), 0), COUNT(*),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN "refundedAmount" ELSE 0 END), 0)
		FROM "payment_orders"
		WHERE julianday("createdAt") >= julianday(?) AND julianday("createdAt") < julianday(?) AND `+orderScope+`
		GROUP BY "payMethod" ORDER BY SUM(CASE WHEN "status" IN (1, 3, 4) THEN "amount" ELSE 0 END) DESC, "payMethod"`,
		append([]any{toSQL(start), toSQL(end)}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var channel DashboardChannel
		if err := rows.Scan(&channel.PayMethod, &channel.AmountCents, &channel.SuccessCount,
			&channel.TotalCount, &channel.RefundedAmountCents); err != nil {
			rows.Close()
			return nil, err
		}
		out.Channels = append(out.Channels, channel)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// createdAt 是文本列，同一毫秒的订单靠 rowid（插入顺序）兜底，
	// 保证多次刷新看到的是同一个顺序。
	rows, err = tx.Query(`SELECT "orderNo", "productName", "payMethod", "status", "amount", "createdAt"
		FROM "payment_orders" WHERE `+orderScope+` ORDER BY "createdAt" DESC, rowid DESC LIMIT ?`, append(append([]any{}, scopeArgs...), DashboardRecentLimit)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item DashboardOrder
		var createdAt string
		if err := rows.Scan(&item.OrderNo, &item.ProductName, &item.PayMethod,
			&item.Status, &item.AmountCents, &createdAt); err != nil {
			rows.Close()
			return nil, err
		}
		item.CreatedAt = fromSQL(createdAt)
		out.Recent = append(out.Recent, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	groupScope, groupArgs := ProjectScopeSQL(`"id"`, projectIDs)
	keyScope, keyArgs := ProjectScopeSQL(`k."groupId"`, projectIDs)
	notifyScope := `1=1`
	if projectIDs != nil {
		notifyScope = `"orderNo" IN (SELECT "orderNo" FROM "payment_orders" WHERE ` + orderScope + `)`
	}
	counts := []struct {
		query  string
		args   []any
		target *int64
	}{
		{`SELECT COUNT(*) FROM "payment_orders" WHERE "status"=0 AND ` + orderScope, scopeArgs, &out.Tasks.PendingOrders},
		{`SELECT COUNT(*) FROM "payment_orders" WHERE "status"=3 AND ` + orderScope, scopeArgs, &out.Tasks.RefundingOrders},
		{`SELECT COUNT(*) FROM "notify_queues" WHERE "status"=0 AND ` + notifyScope, scopeArgs, &out.Tasks.PendingNotifications},
		{`SELECT COUNT(*) FROM "notify_queues" WHERE "status"=2 AND ` + notifyScope, scopeArgs, &out.Tasks.FailedNotifications},
		{`SELECT COUNT(*) FROM "notify_queues" WHERE "status"=3 AND ` + notifyScope, scopeArgs, &out.Tasks.ExhaustedNotifications},
		{`SELECT COUNT(*) FROM "groups" WHERE "isActive"=1 AND ` + groupScope, groupArgs, &out.Tasks.ActiveGroups},
		{`SELECT COUNT(*) FROM "api_keys" k JOIN "groups" g ON g."id"=k."groupId" WHERE k."isActive"=1 AND g."isActive"=1 AND ` + keyScope, keyArgs, &out.Tasks.ActiveKeys},
	}
	for _, item := range counts {
		if err := tx.QueryRow(item.query, item.args...).Scan(item.target); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func dashboardSummary(ex executor, start, end time.Time, scopes ...[]int64) (DashboardSummary, error) {
	var out DashboardSummary
	scope, args := ProjectScopeSQL(`"groupId"`, firstProjectScope(scopes))
	err := ex.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN "amount" ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN 1 ELSE 0 END), 0), COUNT(*),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN "refundedAmount" ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN "status" IN (1, 3, 4) THEN "fee" ELSE 0 END), 0)
		FROM "payment_orders"
		WHERE julianday("createdAt") >= julianday(?) AND julianday("createdAt") < julianday(?) AND `+scope,
		append([]any{toSQL(start), toSQL(end)}, args...)...,
	).Scan(&out.AmountCents, &out.SuccessCount, &out.TotalCount, &out.RefundedAmountCents, &out.FeeCents)
	return out, err
}
