package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"sandbox-pay/internal/signature"
)

// DemoData is produced by the scenario builder, never accepted from HTTP.
type DemoData map[string][]map[string]any

var demoTables = []string{"users", "groups", "api_keys", "project_members", "admin_sessions", "payment_orders", "refund_operations", "notify_queues", "notify_attempts", "sandbox_tasks", "api_usage_logs", "admin_audit_logs", "operation_settings", "operation_runs"}

// DemoAdministrator retains only a login credential when replacing the data.
func (s *Store) DemoAdministrator() (*User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT id,username,password,role,createdAt,updatedAt,isActive,sessionVersion FROM users WHERE isActive=1 AND role='SuperAdmin' ORDER BY id LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// ReplaceDemoData replaces every application table atomically. A bad reference,
// amount, timeline or signature rolls the entire replacement back.
func (s *Store) ReplaceDemoData(data DemoData) error {
	allowed := map[string]bool{}
	for _, table := range demoTables {
		allowed[table] = true
	}
	for table := range data {
		if !allowed[table] {
			return fmt.Errorf("未知种子表 %s", table)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := len(demoTables) - 1; i >= 0; i-- {
		if _, err := tx.Exec(`DELETE FROM "` + demoTables[i] + `"`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM sqlite_sequence`); err != nil {
		return err
	}
	for _, table := range demoTables {
		for _, row := range data[table] {
			columns := make([]string, 0, len(row))
			for column := range row {
				columns = append(columns, column)
			}
			sort.Strings(columns)
			quoted, placeholders, values := make([]string, len(columns)), make([]string, len(columns)), make([]any, len(columns))
			for i, column := range columns {
				quoted[i] = `"` + strings.ReplaceAll(column, `"`, `""`) + `"`
				placeholders[i], values[i] = "?", bindValue(row[column])
			}
			query := `INSERT INTO "` + table + `" (` + strings.Join(quoted, ",") + `) VALUES (` + strings.Join(placeholders, ",") + `)`
			if _, err := tx.Exec(query, values...); err != nil {
				return fmt.Errorf("种子表 %s 写入失败: %w", table, err)
			}
		}
	}
	if err := validateDemo(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func validateDemo(tx *sql.Tx) error {
	checks := []struct{ name, query string }{
		{"订单项目/令牌", `SELECT COUNT(*) FROM payment_orders o LEFT JOIN groups g ON g.id=o.groupId LEFT JOIN api_keys k ON k.id=o.apiKeyId WHERE g.id IS NULL OR k.id IS NULL OR k.groupId<>o.groupId OR o.signingSecret<>g.webhookSecret OR o.signatureVersion<>2 OR o.userId<>0 OR julianday(k.createdAt)>julianday(o.createdAt)`},
		{"支付金额/时间", `SELECT COUNT(*) FROM payment_orders WHERE amount<=0 OR refundedAmount<0 OR refundedAmount>amount OR julianday(createdAt)>julianday(updatedAt) OR (status IN (1,3,4) AND (payAt IS NULL OR julianday(payAt)<julianday(createdAt) OR julianday(payAt)>julianday(expireAt) OR fee<0 OR channelCost<0 OR settleAmount<>MAX(0,amount-fee))) OR (status IN (-1,0,2) AND (payAt IS NOT NULL OR fee<>0 OR settleAmount<>0 OR channelCost<>0 OR refundedAmount<>0))`},
		{"退款累计", `SELECT COUNT(*) FROM payment_orders o WHERE refundedAmount<>(SELECT COALESCE(SUM(amount),0) FROM refund_operations r WHERE r.orderNo=o.orderNo AND r.status='completed') OR (o.status=4 AND refundedAmount<>amount) OR (o.status=1 AND refundedAmount=amount)`},
		{"退款在途", `SELECT COUNT(*) FROM payment_orders o WHERE (status=3 AND (SELECT COUNT(*) FROM refund_operations r WHERE r.orderNo=o.orderNo AND r.status IN ('pending','processing','unknown'))<>1) OR (status<>3 AND EXISTS(SELECT 1 FROM refund_operations r WHERE r.orderNo=o.orderNo AND r.status IN ('pending','processing','unknown')))`},
		{"退款明细时间", `SELECT COUNT(*) FROM refund_operations r LEFT JOIN payment_orders o ON o.orderNo=r.orderNo WHERE o.orderNo IS NULL OR r.amount<=0 OR r.amount>o.amount OR o.payAt IS NULL OR julianday(r.createdAt)<julianday(o.payAt) OR julianday(r.updatedAt)<julianday(r.createdAt) OR (r.status='completed' AND r.tradeNo IS NULL)`},
		{"使用日志归属", `SELECT COUNT(*) FROM api_usage_logs l LEFT JOIN payment_orders o ON o.orderNo=l.orderNo LEFT JOIN api_keys k ON k.id=l.apiKeyId LEFT JOIN groups g ON g.id=l.groupId WHERE o.orderNo IS NULL OR g.id IS NULL OR k.id IS NULL OR o.groupId<>l.groupId OR k.groupId<>l.groupId OR l.groupName<>g.name OR l.apiKeyName<>k.name OR l.keyPrefix<>k.keyPrefix OR l.keySuffix<>k.keySuffix OR l.path NOT LIKE '/v1/api/gateway/%' OR julianday(l.createdAt)<julianday(o.createdAt)`},
		{"团队及审计", `SELECT COUNT(*) FROM admin_audit_logs a LEFT JOIN users u ON u.id=a.userId WHERE u.id IS NULL OR a.username<>u.username OR julianday(a.createdAt)<julianday(u.createdAt)`},
		{"沙箱任务归属", `SELECT COUNT(*) FROM sandbox_tasks t LEFT JOIN payment_orders o ON o.orderNo=t.orderNo WHERE o.orderNo IS NULL OR o.payMethod<>'native' OR julianday(t.createdAt)<julianday(o.createdAt) OR julianday(t.updatedAt)<julianday(t.createdAt)`},
		{"回调尝试时间", `SELECT COUNT(*) FROM notify_attempts a LEFT JOIN notify_queues n ON n.id=a.notifyId WHERE n.id IS NULL OR julianday(a.createdAt)<julianday(n.createdAt)`},
		{"回调归属", `SELECT COUNT(*) FROM notify_queues n LEFT JOIN payment_orders o ON o.orderNo=n.orderNo WHERE o.orderNo IS NULL OR n.url<>o.notifyUrl OR julianday(n.createdAt)<julianday(o.payAt)`},
		{"运行设置", `SELECT CASE WHEN COUNT(*)=1 THEN 0 ELSE 1 END FROM operation_settings WHERE id=1`},
		{"管理员", `SELECT CASE WHEN COUNT(*)>0 THEN 0 ELSE 1 END FROM users WHERE role='SuperAdmin' AND isActive=1`},
	}
	for _, check := range checks {
		var invalid int
		if err := tx.QueryRow(check.query).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("种子关联检查失败：%s (%d)", check.name, invalid)
		}
	}
	rows, err := tx.Query(`SELECT n.body,n.signature,o.signingSecret,o.orderNo,o.externalOrderNo,o.amount,o.payMethod,n.sourceNotificationId,n.id FROM notify_queues n JOIN payment_orders o ON o.orderNo=n.orderNo ORDER BY n.id`)
	if err != nil {
		return err
	}
	type original struct{ body, signature string }
	seen := map[int64]original{}
	for rows.Next() {
		var body, signed, secret, orderNo, external, method string
		var amount, id int64
		var source sql.NullInt64
		if err := rows.Scan(&body, &signed, &secret, &orderNo, &external, &amount, &method, &source, &id); err != nil {
			rows.Close()
			return err
		}
		var event struct {
			OrderNo, ExternalOrderNo, PayMethod string
			Amount                              float64
		}
		var metadata struct {
			Timestamp, Nonce, Sign string
			Version                int
		}
		if json.Unmarshal([]byte(body), &event) != nil || json.Unmarshal([]byte(signed), &metadata) != nil || metadata.Version != 2 || event.OrderNo != orderNo || event.ExternalOrderNo != external || event.PayMethod != method || int64(event.Amount*100+0.5) != amount || !signature.Verify(signature.BuildPayload(body, metadata.Timestamp, metadata.Nonce), secret, metadata.Sign) {
			rows.Close()
			return fmt.Errorf("订单 %s 的种子回调与签名不一致", orderNo)
		}
		if source.Valid && seen[source.Int64] != (original{body, signed}) {
			rows.Close()
			return fmt.Errorf("种子重复回调 %d 不对应原始报文", id)
		}
		seen[id] = original{body, signed}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("种子数据存在外键冲突")
	}
	return rows.Err()
}
