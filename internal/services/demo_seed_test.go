package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/jwt"
	"sandbox-pay/internal/signature"
)

func demoTestConfig() config.Map {
	return config.Map{
		"ENABLE_SANDBOX": "true",
		"JWT_SECRET":     "demo-test-0123456789abcdef-fedcba9876543210",
		"JWT_EXPIRES_IN": "60m",
	}
}

func demoTestNow() time.Time {
	return time.Date(2026, 10, 31, 20, 0, 0, 0, time.FixedZone("Asia/Hong_Kong", 8*60*60))
}

func demoReadDatabase(t *testing.T, store *db.Store) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+store.DatabasePath()+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func demoCount(t *testing.T, conn *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := conn.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestDemoSeedHistoriesReconcileAcrossEveryPage(t *testing.T) {
	store := newTestStore(t)
	cfg, now := demoTestConfig(), demoTestNow()
	result, err := SeedDemo(store, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Administrator == "" || len(result.Counts) == 0 || len(result.Examples) == 0 {
		t.Fatal("seed result must identify the login account, data counts and linked example lifecycles")
	}
	conn := demoReadDatabase(t, store)
	if count := demoCount(t, conn, `SELECT COUNT(*) FROM groups`); count != 4 {
		t.Fatalf("project count = %d; want four business projects", count)
	}
	if count := demoCount(t, conn, `SELECT COUNT(*) FROM payment_orders`); count < 300 {
		t.Fatalf("order count = %d; expected enough history for useful daily and monthly charts", count)
	}
	if count := demoCount(t, conn, `SELECT COUNT(*) FROM users`); count < 5 {
		t.Fatalf("user count = %d; expected administrator and project team members", count)
	}
	for _, status := range []int{db.StatusExpired, db.StatusPending, db.StatusPaid, db.StatusFailed, db.StatusRefunding, db.StatusRefunded} {
		if demoCount(t, conn, `SELECT COUNT(*) FROM payment_orders WHERE status=?`, status) == 0 {
			t.Fatalf("order status %d has no linked example lifecycle", status)
		}
	}
	for _, status := range []int{db.NotifyPending, db.NotifySuccess, db.NotifyFailed, db.NotifyExhausted} {
		if demoCount(t, conn, `SELECT COUNT(*) FROM notify_queues WHERE status=?`, status) == 0 {
			t.Fatalf("notification status %d has no delivery example", status)
		}
	}
	for _, scenario := range []struct{ name, query string }{
		{"paused project", `SELECT COUNT(*) FROM groups WHERE isActive=0`},
		{"retired credential", `SELECT COUNT(*) FROM api_keys WHERE isActive=0`},
		{"completed sandbox test", `SELECT COUNT(*) FROM sandbox_tasks WHERE status='completed'`},
		{"cancelled sandbox test", `SELECT COUNT(*) FROM sandbox_tasks WHERE status='cancelled'`},
		{"scheduled sandbox test", `SELECT COUNT(*) FROM sandbox_tasks WHERE status='pending'`},
		{"successful API request", `SELECT COUNT(*) FROM api_usage_logs WHERE statusCode>=200 AND statusCode<400`},
		{"failed API request", `SELECT COUNT(*) FROM api_usage_logs WHERE statusCode>=400`},
	} {
		if demoCount(t, conn, scenario.query) == 0 {
			t.Fatalf("missing linked demo scenario: %s", scenario.name)
		}
	}
	checks := []struct{ name, query string }{
		{"orders reference a credential in their own project", `SELECT COUNT(*) FROM payment_orders o LEFT JOIN api_keys k ON k.id=o.apiKeyId LEFT JOIN groups g ON g.id=o.groupId WHERE k.id IS NULL OR g.id IS NULL OR k.groupId<>o.groupId`},
		{"projects all participate in the same seeded business history", `SELECT COUNT(*) FROM groups g WHERE NOT EXISTS (SELECT 1 FROM payment_orders o WHERE o.groupId=g.id)`},
		{"membership refers to actual seed users and projects", `SELECT COUNT(*) FROM project_members m LEFT JOIN users u ON u.id=m.userId LEFT JOIN groups g ON g.id=m.groupId WHERE u.id IS NULL OR g.id IS NULL`},
		{"buyers are not fabricated administrator accounts", `SELECT COUNT(*) FROM payment_orders WHERE userId<>0`},
		{"unpaid orders have no fictitious payment or refund settlement", `SELECT COUNT(*) FROM payment_orders WHERE status IN (-1,0,2) AND (payAt IS NOT NULL OR thirdPartyTradeNo IS NOT NULL OR refundedAmount<>0 OR fee<>0 OR channelCost<>0 OR settleAmount<>0)`},
		{"paid orders carry their original settlement", `SELECT COUNT(*) FROM payment_orders WHERE status IN (1,3,4) AND (payAt IS NULL OR thirdPartyTradeNo IS NULL OR amount<=0 OR settleAmount<>MAX(0,amount-fee))`},
		{"completed refund ledger sums to the order balance", `SELECT COUNT(*) FROM payment_orders o WHERE o.refundedAmount<>COALESCE((SELECT SUM(f.amount) FROM refund_operations f WHERE f.orderNo=o.orderNo AND f.status='completed'),0) OR o.refundedAmount<0 OR o.refundedAmount>o.amount`},
		{"full and partial refund statuses match the balances", `SELECT COUNT(*) FROM payment_orders WHERE (status=4 AND refundedAmount<>amount) OR (status=1 AND refundedAmount=amount) OR (refundedAmount>0 AND status NOT IN (1,3,4))`},
		{"refund in flight has exactly one corresponding operation", `SELECT COUNT(*) FROM payment_orders o WHERE (o.status=3 AND (SELECT COUNT(*) FROM refund_operations f WHERE f.orderNo=o.orderNo AND f.status IN ('processing','pending','unknown'))<>1) OR (o.status<>3 AND EXISTS (SELECT 1 FROM refund_operations f WHERE f.orderNo=o.orderNo AND f.status IN ('processing','pending','unknown')))`},
		{"refund requests cannot precede the payment they refund", `SELECT COUNT(*) FROM refund_operations f LEFT JOIN payment_orders o ON o.orderNo=f.orderNo WHERE o.orderNo IS NULL OR julianday(f.createdAt)<julianday(o.payAt) OR julianday(f.updatedAt)<julianday(f.createdAt)`},
		{"order timestamps form an actual chronological lifecycle", `SELECT COUNT(*) FROM payment_orders o JOIN groups g ON g.id=o.groupId JOIN api_keys k ON k.id=o.apiKeyId WHERE julianday(o.createdAt)<julianday(g.createdAt) OR julianday(o.createdAt)<julianday(k.createdAt) OR julianday(o.updatedAt)<julianday(o.createdAt) OR (o.payAt IS NOT NULL AND julianday(o.payAt)<julianday(o.createdAt))`},
		{"usage log attribution agrees with the order and credential", `SELECT COUNT(*) FROM api_usage_logs l LEFT JOIN api_keys k ON k.id=l.apiKeyId LEFT JOIN groups g ON g.id=l.groupId LEFT JOIN payment_orders o ON o.orderNo=l.orderNo WHERE k.id IS NULL OR g.id IS NULL OR k.groupId<>l.groupId OR l.groupName<>g.name OR l.apiKeyName<>k.name OR l.keyPrefix<>k.keyPrefix OR l.keySuffix<>k.keySuffix OR (l.orderNo<>'' AND (o.orderNo IS NULL OR o.groupId<>l.groupId))`},
		{"usage observes gateway calls rather than inventing admin API traffic", `SELECT COUNT(*) FROM api_usage_logs WHERE path NOT LIKE '/v1/api/gateway/%'`},
		{"administrative audit actors exist", `SELECT COUNT(*) FROM admin_audit_logs a LEFT JOIN users u ON u.id=a.userId WHERE u.id IS NULL OR a.username<>u.username`},
		{"notifications all refer to actual paid order lifecycles", `SELECT COUNT(*) FROM notify_queues n LEFT JOIN payment_orders o ON o.orderNo=n.orderNo WHERE o.orderNo IS NULL OR o.payAt IS NULL`},
		{"sandbox tasks belong to supported local orders", `SELECT COUNT(*) FROM sandbox_tasks t LEFT JOIN payment_orders o ON o.orderNo=t.orderNo WHERE o.orderNo IS NULL OR o.payMethod<>'native'`},
		{"callback duplication retains the original signed event bytes", `SELECT COUNT(*) FROM notify_queues n JOIN notify_queues source ON source.id=n.sourceNotificationId WHERE n.orderNo<>source.orderNo OR n.body<>source.body OR n.signature<>source.signature`},
		{"disabled users have no active session", `SELECT COUNT(*) FROM admin_sessions s JOIN users u ON u.id=s.userId WHERE u.isActive=0 AND s.revokedAt IS NULL`},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if bad := demoCount(t, conn, tc.query); bad != 0 {
				t.Fatalf("%d related records disagree", bad)
			}
		})
	}
	var integrity string
	if err := conn.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("database integrity = %q, error %v", integrity, err)
	}
	if violations := demoCount(t, conn, `SELECT COUNT(*) FROM pragma_foreign_key_check`); violations != 0 {
		t.Fatalf("foreign key violations = %d", violations)
	}

	rows, err := conn.Query(`SELECT orderNo FROM payment_orders ORDER BY createdAt,orderNo`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	orders := make([]*db.PaymentOrder, 0, len(names))
	fees := NewFeeCalculator(cfg)
	for _, name := range names {
		order := reloadOrder(t, store, name)
		orders = append(orders, order)
		if order.CreatedAt.After(now) || order.UpdatedAt.After(now) {
			t.Fatalf("order %s is in the future relative to the seed clock", name)
		}
		if order.Status == db.StatusPending && !order.ExpireAt.After(now) {
			t.Fatalf("pending order %s has already expired", name)
		}
		if order.Status == db.StatusPaid || order.Status == db.StatusRefunding || order.Status == db.StatusRefunded {
			want := fees.Calculate(order.PayMethod, order.Amount)
			if order.Fee != want.Fee || order.ChannelCost != want.ChannelCost || order.SettleAmount != want.SettleAmount {
				t.Fatalf("order %s payment settlement does not match its channel and amount", name)
			}
		}
		if order.PayMethod == "paypal" {
			if order.ForeignAmount == nil || order.ExchangeRate == nil || order.ForeignCurrency == nil || *order.ForeignCurrency != "USD" || *order.ForeignAmount != int64(math.Max(1, math.Round(float64(order.Amount)*(*order.ExchangeRate)))) {
				t.Fatalf("PayPal order %s has an inconsistent currency conversion", name)
			}
		}
		operations, err := store.Refunds.ListByOrderNo(name)
		if err != nil {
			t.Fatal(err)
		}
		var lastCompleted *db.RefundOperation
		for _, operation := range operations {
			if operation.Status == db.RefundCompleted {
				lastCompleted = operation
			}
		}
		if lastCompleted != nil && (order.RefundAt == nil || order.RefundTradeNo == nil || lastCompleted.TradeNo == nil || *order.RefundTradeNo != *lastCompleted.TradeNo || !order.RefundAt.Equal(lastCompleted.UpdatedAt)) {
			t.Fatalf("order %s does not point to its latest completed refund", name)
		}
		notifications, err := store.Notifies.ListByOrderNo(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, notification := range notifications {
			var body PaymentNotification
			var meta struct {
				Timestamp string `json:"timestamp"`
				Nonce     string `json:"nonce"`
				Sign      string `json:"sign"`
				Version   int    `json:"version"`
			}
			if json.Unmarshal([]byte(notification.Body), &body) != nil || json.Unmarshal([]byte(notification.Signature), &meta) != nil {
				t.Fatalf("order %s callback has malformed persisted payload", name)
			}
			if body.OrderNo != name || body.Amount != float64(order.Amount)/100 || body.PayMethod != order.PayMethod || body.ExternalOrderNo == nil || order.ExternalOrderNo == nil || *body.ExternalOrderNo != *order.ExternalOrderNo || order.PayAt == nil || body.PayAt != JsTime(*order.PayAt) {
				t.Fatalf("order %s callback describes a different order or payment", name)
			}
			if body.ThirdPartyTradeNo == nil {
				t.Fatalf("order %s callback has no corresponding payment or refund transaction", name)
			}
			switch body.Status {
			case "paid":
				if order.ThirdPartyTradeNo == nil || *body.ThirdPartyTradeNo != *order.ThirdPartyTradeNo {
					t.Fatalf("order %s paid callback names a different payment transaction", name)
				}
			case "partial_refunded", "refunded":
				matched, cumulative := false, int64(0)
				for _, operation := range operations {
					if operation.Status != db.RefundCompleted {
						continue
					}
					cumulative += operation.Amount
					if operation.TradeNo != nil && *operation.TradeNo == *body.ThirdPartyTradeNo {
						matched = !notification.CreatedAt.Before(operation.UpdatedAt) &&
							((body.Status == "refunded" && cumulative == order.Amount) || (body.Status == "partial_refunded" && cumulative < order.Amount))
						break
					}
				}
				if !matched {
					t.Fatalf("order %s refund callback does not match a completed refund and its cumulative balance", name)
				}
			default:
				t.Fatalf("order %s callback declares an unsupported lifecycle state", name)
			}
			if order.SigningSecret == nil || meta.Version != 2 || !signature.Verify(signature.BuildPayload(notification.Body, meta.Timestamp, meta.Nonce), *order.SigningSecret, meta.Sign) {
				t.Fatalf("order %s callback signature does not verify against its signing snapshot", name)
			}
			if notification.CreatedAt.Before(*order.PayAt) {
				t.Fatalf("order %s callback predates payment", name)
			}
			attempts, err := store.Notifies.ListAttempts(notification.ID)
			if err != nil {
				t.Fatal(err)
			}
			previous := notification.CreatedAt
			for i, attempt := range attempts {
				if attempt.AttemptNo != int64(i+1) || attempt.CreatedAt.Before(previous) || attempt.DurationMs < 0 {
					t.Fatalf("notification %d attempt history is discontinuous or precedes the event", notification.ID)
				}
				previous = attempt.CreatedAt
			}
			if len(attempts) > 0 {
				last := attempts[len(attempts)-1]
				if notification.LastAttemptAt == nil || !notification.LastAttemptAt.Equal(last.CreatedAt) {
					t.Fatalf("notification %d does not point to its final recorded attempt", notification.ID)
				}
				if notification.Status == db.NotifySuccess && (last.HTTPStatus == nil || *last.HTTPStatus < 200 || *last.HTTPStatus >= 300) {
					t.Fatalf("notification %d claims success after an unsuccessful HTTP response", notification.ID)
				}
			}
			if notification.Status == db.NotifyExhausted && (notification.RetryCount != 5 || len(attempts) < 5) {
				t.Fatalf("notification %d was exhausted without its five failed delivery attempts", notification.ID)
			}
		}
	}
	for _, days := range []int{7, 30} {
		demoCheckDashboardAgainstOrders(t, NewAdminService(store), orders, now, days)
	}
}

func demoCheckDashboardAgainstOrders(t *testing.T, service *AdminService, orders []*db.PaymentOrder, now time.Time, days int) {
	t.Helper()
	out, err := service.dashboardAt(days, 480, now)
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.ParseInLocation("2006-01-02", out.DateFrom, now.Location())
	end := start.AddDate(0, 0, days)
	previousStart := start.AddDate(0, 0, -days)
	type amountTotals struct{ total, success, amount, refunded, fee int64 }
	current, previous := amountTotals{}, amountTotals{}
	daily := make(map[string]amountTotals)
	for _, order := range orders {
		var target *amountTotals
		if !order.CreatedAt.Before(start) && order.CreatedAt.Before(end) {
			target = &current
		} else if !order.CreatedAt.Before(previousStart) && order.CreatedAt.Before(start) {
			target = &previous
		}
		if target == nil {
			continue
		}
		value := amountTotals{total: 1}
		if order.Status == db.StatusPaid || order.Status == db.StatusRefunding || order.Status == db.StatusRefunded {
			value.success, value.amount, value.refunded, value.fee = 1, order.Amount, order.RefundedAmount, order.Fee
		}
		target.total += value.total
		target.success += value.success
		target.amount += value.amount
		target.refunded += value.refunded
		target.fee += value.fee
		if target == &current {
			date := order.CreatedAt.In(now.Location()).Format("2006-01-02")
			day := daily[date]
			day.total += value.total
			day.success += value.success
			day.amount += value.amount
			daily[date] = day
		}
	}
	for _, tc := range []struct {
		name string
		got  DashboardSummary
		want amountTotals
	}{{"current", out.Summary, current}, {"previous", out.Previous, previous}} {
		if tc.want.total == 0 || tc.got.TotalCount != tc.want.total || tc.got.SuccessCount != tc.want.success || tc.got.Amount != float64(tc.want.amount)/100 || tc.got.RefundedAmount != float64(tc.want.refunded)/100 || tc.got.Fee != float64(tc.want.fee)/100 {
			t.Fatalf("%d-day %s dashboard does not reconcile to its underlying orders", days, tc.name)
		}
	}
	if len(out.Trend) != days {
		t.Fatalf("%d-day dashboard returned %d trend dates", days, len(out.Trend))
	}
	for _, day := range out.Trend {
		want := daily[day.Date]
		if day.TotalCount != want.total || day.SuccessCount != want.success || day.Amount != float64(want.amount)/100 {
			t.Fatalf("trend %s does not reconcile to its actual dated orders", day.Date)
		}
	}
}

func TestDemoSeedReplacesOldBusinessDataAndInvalidatesOldSessions(t *testing.T) {
	store, auth, _, administrator, cfg := teamFixture(t)
	groupID := seedGroup1(t, store)
	keyID := seedAPIKey(t, store, groupID, "old-demo-business-token")
	old, err := NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(cfg)).CreateOrder(OrderCaller{GroupID: groupID, APIKeyID: &keyID}, CreateOrderInput{Amount: 1.23, ProductName: "old business record", PayMethod: "native"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueLocalToken(administrator)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET"))); err != nil {
		t.Fatal("fixture session must authenticate before replacement", err)
	}
	for generation := 0; generation < 2; generation++ {
		result, err := SeedDemo(store, cfg, demoTestNow().Add(time.Duration(generation)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if result.Administrator != administrator.Username {
			t.Fatal("seeding changed the login account unexpectedly")
		}
		current, err := auth.ValidateUser(administrator.Username, "root-password")
		if err != nil || current == nil || current.Password != administrator.Password {
			t.Fatal("demo replacement must retain the existing administrator password")
		}
		if _, _, err := auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET"))); err == nil {
			t.Fatal("a session issued before data replacement still authenticates")
		}
		order, err := store.Orders.FindByOrderNo(old.OrderNo, false)
		if err != nil || order != nil {
			t.Fatal("previous business order remains after replacement")
		}
		key, err := store.ApiKeys.FindByKey("old-demo-business-token")
		if err != nil || key != nil {
			t.Fatal("previous API credential remains after replacement")
		}
		conn := demoReadDatabase(t, store)
		if count := demoCount(t, conn, `SELECT COUNT(*) FROM groups WHERE name='seed'`); count != 0 {
			t.Fatal("previous project remains after replacement")
		}
		if count := demoCount(t, conn, `SELECT COUNT(*) FROM groups`); count != 4 {
			t.Fatalf("generation %d accumulated previous projects", generation)
		}
	}
}

func TestDemoSeedReplacementRollsBackSemanticallyInvalidRelationships(t *testing.T) {
	store := newTestStore(t)
	if _, err := SeedDemo(store, demoTestConfig(), demoTestNow()); err != nil {
		t.Fatal(err)
	}
	conn := demoReadDatabase(t, store)
	data := db.DemoData{}
	for _, table := range []string{"users", "groups", "api_keys", "payment_orders", "refund_operations", "notify_queues", "notify_attempts", "api_usage_logs", "sandbox_tasks", "project_members", "admin_sessions", "admin_audit_logs", "operation_settings", "operation_runs"} {
		rows, err := conn.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		data[table] = nil
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			row := make(map[string]any, len(columns))
			for i, column := range columns {
				row[column] = values[i]
			}
			data[table] = append(data[table], row)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	first := data["payment_orders"][0]
	oldOrderNo := first["orderNo"].(string)
	original := reloadOrder(t, store, oldOrderNo)
	for _, credential := range data["api_keys"] {
		if credential["groupId"] != first["groupId"] {
			first["apiKeyId"] = credential["id"]
			break
		}
	}
	if err := store.ReplaceDemoData(data); err == nil {
		t.Fatal("replacement accepted an order using another project's real credential")
	}
	retained := reloadOrder(t, store, oldOrderNo)
	if retained.GroupID == nil || retained.APIKeyID == nil || *retained.GroupID != *original.GroupID || *retained.APIKeyID != *original.APIKeyID {
		t.Fatal("failed replacement changed the previous live dataset")
	}
	if count := demoCount(t, conn, `SELECT COUNT(*) FROM payment_orders`); count != int64(len(data["payment_orders"])) {
		t.Fatal("failed replacement did not roll back removed orders")
	}
}
