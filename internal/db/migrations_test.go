package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func legacyDatabase(t *testing.T, duplicate bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	oldSchema := strings.Replace(schemaSQL, "  \"providerOrderId\" varchar(128),\n", "", 1)
	if _, err := conn.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO "groups" ("name") VALUES ('existing group'); INSERT INTO "users" ("username", "password") VALUES ('existing', 'unchanged hash'); INSERT INTO "api_keys" ("groupId", "name", "key", "isActive") VALUES (1, 'disabled key', 'old-key', 0)`); err != nil {
		t.Fatal(err)
	}
	orders := 1
	if duplicate {
		orders = 2
	}
	for i := 0; i < orders; i++ {
		if _, err := conn.Exec(`INSERT INTO "payment_orders" ("id", "orderNo", "amount", "productName", "payMethod", "expireAt", "externalOrderNo", "groupId", "apiKeyId") VALUES (?, ?, 100, 'old item', 'native', '2030-01-01 00:00:00.000', 'same-external', 1, 1)`, string(rune('a'+i)), string(rune('A'+i))); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestUpgradePreservesExistingData(t *testing.T) {
	path := legacyDatabase(t, false)
	store, err := Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	order, err := store.Orders.FindByOrderNo("A", true)
	if err != nil || order == nil || order.Amount != 100 || order.APIKey == nil || order.APIKey.IsActive {
		t.Fatalf("existing order/key changed: %+v, %v", order, err)
	}
	user, err := store.Users.FindByUsername("existing")
	if err != nil || user == nil || user.Password != "unchanged hash" {
		t.Fatalf("existing user changed: %+v, %v", user, err)
	}
	var version int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("version = %d, %v", version, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, "")
	if err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	defer store.Close()
	if count, _ := store.Orders.Count(); count != 1 {
		t.Fatalf("orders after reopening = %d", count)
	}
	if order.ClientIP != nil || order.ClientLocation != nil {
		t.Fatalf("migration fabricated metadata for historical order: %+v", order)
	}
	copy := *order
	copy.ID, copy.OrderNo = "B", "B"
	if err := store.Orders.Insert(&copy); err == nil {
		t.Fatal("unique idempotency constraint did not reject duplicate")
	}
}

func TestUpgradeRejectsDuplicatesWithoutDeletingData(t *testing.T) {
	path := legacyDatabase(t, true)
	store, err := Open(path, "")
	if err == nil {
		store.Close()
		t.Fatal("duplicate orders should stop migration")
	}
	if !strings.Contains(err.Error(), "重复订单") {
		t.Fatalf("unexpected migration error: %v", err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var count, version, hasColumn, hasRefundTable int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM "payment_orders"`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("existing orders lost: %d, %v", count, err)
	}
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 0 {
		t.Fatalf("failed migration version = %d, %v", version, err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('payment_orders') WHERE name='providerOrderId'`).Scan(&hasColumn); err != nil || hasColumn != 0 {
		t.Fatalf("ALTER did not roll back: %d, %v", hasColumn, err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='refund_operations'`).Scan(&hasRefundTable); err != nil || hasRefundTable != 0 {
		t.Fatalf("refund table survived failed migration: %d, %v", hasRefundTable, err)
	}
}

func version1Database(t *testing.T) string {
	t.Helper()
	path := legacyDatabase(t, false)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateVersion1(tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO "refund_operations" ("id", "requestId", "reason", "orderNo", "amount", "status", "createdAt", "updatedAt")
		VALUES ('old-refund', 'old-request', 'existing reason', 'A', 1, 'completed', '2026-01-01 00:00:00.000', '2026-01-01 00:00:00.000');
		INSERT INTO "notify_queues" ("orderNo", "url", "body", "signature", "status") VALUES ('A', 'https://merchant.example/notify', '{}', 'old-signature', 1);`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUpgradeVersion1AddsNullableClientMetadataAndPreservesHistory(t *testing.T) {
	path := version1Database(t)
	s, err := Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	var version, columns int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("version = %d, err=%v", version, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('payment_orders') WHERE name IN ('clientIp', 'clientLocation') AND "notnull" = 0`).Scan(&columns); err != nil || columns != 2 {
		t.Fatalf("nullable metadata columns = %d, err=%v", columns, err)
	}
	o, err := s.Orders.FindByOrderNo("A", true)
	if err != nil || o == nil || o.ClientIP != nil || o.ClientLocation != nil || o.APIKey == nil || o.APIKey.IsActive || o.Amount != 100 {
		t.Fatalf("historical order changed: %+v, err=%v", o, err)
	}
	refund, err := s.Refunds.FindByID("old-refund")
	if err != nil || refund == nil || refund.Reason != "existing reason" || refund.Status != RefundCompleted {
		t.Fatalf("historical refund changed: %+v, err=%v", refund, err)
	}
	notifications, err := s.Notifies.ListByOrderNo("A")
	if err != nil || len(notifications) != 1 || notifications[0].Signature != "old-signature" {
		t.Fatalf("historical callback changed: %+v, err=%v", notifications, err)
	}
	if err := s.Orders.Update("A", map[string]any{"clientIp": "2001:db8::1", "clientLocation": `{"status":"reserved"}`}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	o, err = s.Orders.FindByOrderNo("A", false)
	if err != nil || o == nil || o.ClientIP == nil || *o.ClientIP != "2001:db8::1" || o.ClientLocation == nil || *o.ClientLocation != `{"status":"reserved"}` {
		t.Fatalf("recorded metadata changed after reopen: %+v, err=%v", o, err)
	}
}

func TestUpgradeRefusesFutureDatabaseVersionWithoutChangingOrders(t *testing.T) {
	path := version1Database(t)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`PRAGMA user_version = 6`); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	s, err := Open(path, "")
	if err == nil {
		s.Close()
		t.Fatal("future database version was accepted")
	}
	if !strings.Contains(err.Error(), "版本 6") || !strings.Contains(err.Error(), "版本 5") {
		t.Fatalf("unexpected future-version error: %v", err)
	}
	conn, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var version, orders, ipColumns int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("future version changed: %d, err=%v", version, err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM "payment_orders"`).Scan(&orders); err != nil || orders != 1 {
		t.Fatalf("future database order lost: %d, err=%v", orders, err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('payment_orders') WHERE name='clientIp'`).Scan(&ipColumns); err != nil || ipColumns != 0 {
		t.Fatalf("future database was migrated: %d, err=%v", ipColumns, err)
	}
}
