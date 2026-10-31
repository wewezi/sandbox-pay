package gateways

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func cashierMetadataStore(t *testing.T) (*db.Store, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "metadata.db")
	store, err := db.Open(databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, databasePath
}

func cashierMetadataOrder(t *testing.T, store *db.Store, groupID *int64, external *string) *db.PaymentOrder {
	t.Helper()
	order := &db.PaymentOrder{
		ID: "cashier-metadata", OrderNo: "PAY_CASHIER_METADATA", Amount: 12800,
		ProductName: "Order metadata regression", PayMethod: "native", Status: db.StatusPending,
		ExpireAt: time.Now().Add(time.Hour), GroupID: groupID, ExternalOrderNo: external,
	}
	if err := store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)
	if err := store.Orders.Update(order.OrderNo, map[string]any{"createdAt": createdAt}); err != nil {
		t.Fatal(err)
	}
	order.CreatedAt = createdAt
	return order
}

func cashierMetadataJSON(t *testing.T, info *CashierInfo) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestCashierMetadataUsesExactOrderProjectInEveryState(t *testing.T) {
	for _, state := range []struct {
		name       string
		stored     int
		expired    bool
		wantStatus string
	}{
		{name: "pending", stored: db.StatusPending, wantStatus: "pending"},
		{name: "paid", stored: db.StatusPaid, expired: true, wantStatus: "paid"},
		{name: "refunding", stored: db.StatusRefunding, expired: true, wantStatus: "paid"},
		{name: "refunded", stored: db.StatusRefunded, expired: true, wantStatus: "paid"},
		{name: "expired", stored: db.StatusExpired, wantStatus: "expired"},
		{name: "failed", stored: db.StatusFailed, wantStatus: "failed"},
		{name: "deadline passed", stored: db.StatusPending, expired: true, wantStatus: "expired"},
	} {
		t.Run(state.name, func(t *testing.T) {
			store, _ := cashierMetadataStore(t)
			first, err := store.Groups.Insert("Unrelated default project", true)
			if err != nil {
				t.Fatal(err)
			}
			group, err := store.Groups.Insert("  Fanzhou Design Cloud  ", true)
			if err != nil {
				t.Fatal(err)
			}
			if first.ID == group.ID {
				t.Fatal("fixture must contain two distinct projects")
			}
			external := " SO-2026-0042 "
			order := cashierMetadataOrder(t, store, &group.ID, &external)
			patch := map[string]any{"status": state.stored}
			if state.expired {
				patch["expireAt"] = time.Now().Add(-time.Hour)
			}
			if state.wantStatus == "paid" {
				patch["payAt"] = order.CreatedAt.Add(15 * time.Minute)
				patch["thirdPartyTradeNo"] = "SP_WALLET_METADATA"
			}
			if err := store.Orders.Update(order.OrderNo, patch); err != nil {
				t.Fatal(err)
			}
			cashier := NewNativePay(config.Map{}, store, nil, nil)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil {
				t.Fatal(err)
			}
			body := cashierMetadataJSON(t, info)
			if info.Status != state.wantStatus || body["projectName"] != "Fanzhou Design Cloud" ||
				body["externalOrderNo"] != external || body["createdAt"] != services.JsTime(order.CreatedAt) ||
				body["orderNo"] != order.OrderNo || body["amount"] != 128.0 || body["productName"] != order.ProductName {
				t.Fatalf("order state lost or invented metadata: %+v", body)
			}
		})
	}
}

func TestCashierMetadataProjectFallbackNeverUsesAnotherGroup(t *testing.T) {
	for _, test := range []struct {
		name      string
		groupName string
		active    bool
		noGroup   bool
		delete    bool
		wantName  string
	}{
		{name: "no group", groupName: "Actual merchant", active: true, noGroup: true},
		{name: "deleted group", groupName: "Deleted merchant", active: true, delete: true},
		{name: "empty group name", groupName: " \t ", active: true},
		{name: "inactive group", groupName: "  Legacy Store  ", wantName: "Legacy Store"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, databasePath := cashierMetadataStore(t)
			if _, err := store.Groups.Insert("Wrong default merchant", true); err != nil {
				t.Fatal(err)
			}
			group, err := store.Groups.Insert(test.groupName, test.active)
			if err != nil {
				t.Fatal(err)
			}
			groupID := &group.ID
			if test.noGroup {
				groupID = nil
			}
			order := cashierMetadataOrder(t, store, groupID, nil)
			if test.delete {
				conn, err := sql.Open("sqlite", databasePath)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				if _, err := conn.Exec(`PRAGMA foreign_keys = ON`); err != nil {
					t.Fatal(err)
				}
				if _, err := conn.Exec(`DELETE FROM "groups" WHERE "id" = ?`, group.ID); err != nil {
					t.Fatal(err)
				}
				stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
				if err != nil || stored == nil || stored.GroupID != nil {
					t.Fatal("deleted project must leave a valid order with no group association")
				}
			}
			cashier := NewNativePay(config.Map{}, store, nil, nil)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil || info == nil || info.Status != "pending" {
				t.Fatalf("optional project metadata prevented order display: %v", err)
			}
			body := cashierMetadataJSON(t, info)
			name, present := body["projectName"]
			if test.wantName == "" && present || test.wantName != "" && name != test.wantName {
				t.Fatalf("project association or fallback is incorrect: %+v", body)
			}
		})
	}
}

func TestCashierMetadataQueryFailureKeepsOrderAvailable(t *testing.T) {
	store, databasePath := cashierMetadataStore(t)
	group, err := store.Groups.Insert("Unavailable project metadata", true)
	if err != nil {
		t.Fatal(err)
	}
	external := "EXT-RESILIENT-42"
	order := cashierMetadataOrder(t, store, &group.ID, &external)
	conn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Renaming preserves the referenced table and readable order, while the
	// optional project-name query fails without any production dependency.
	if _, err := conn.Exec(`ALTER TABLE "groups" RENAME TO "metadata_temporarily_unavailable"`); err != nil {
		t.Fatal(err)
	}
	cashier := NewNativePay(config.Map{}, store, nil, nil)
	info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
	if err != nil || info == nil || info.Status != "pending" || info.Amount != 128 {
		t.Fatalf("project metadata failure hid the payment order: %v", err)
	}
	body := cashierMetadataJSON(t, info)
	if _, present := body["projectName"]; present || body["externalOrderNo"] != external || body["createdAt"] != services.JsTime(order.CreatedAt) {
		t.Fatalf("metadata failure changed unrelated fields: %+v", body)
	}
}

func TestCashierMetadataOmitsUnavailableExternalIDAndCreationTime(t *testing.T) {
	empty, whitespace := "", " \t\r\n "
	for _, external := range []*string{nil, &empty, &whitespace} {
		label := "nil"
		if external != nil {
			label = "length=" + strings.Repeat("x", len(*external))
		}
		t.Run(label, func(t *testing.T) {
			store, _ := cashierMetadataStore(t)
			order := cashierMetadataOrder(t, store, nil, external)
			if err := store.Orders.Update(order.OrderNo, map[string]any{"createdAt": time.Time{}}); err != nil {
				t.Fatal(err)
			}
			stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
			if err != nil || stored == nil || !stored.CreatedAt.IsZero() {
				t.Fatal("fixture must represent an unavailable creation timestamp")
			}
			cashier := NewNativePay(config.Map{}, store, nil, nil)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil {
				t.Fatal(err)
			}
			body := cashierMetadataJSON(t, info)
			for _, field := range []string{"projectName", "externalOrderNo", "createdAt"} {
				if _, present := body[field]; present {
					t.Fatalf("cashier invented unavailable %s: %+v", field, body)
				}
			}
		})
	}
}

func TestCashierMetadataDoesNotExposeProjectOrOrderSecrets(t *testing.T) {
	store, _ := cashierMetadataStore(t)
	group, err := store.Groups.Insert("Merchant Display Name", true)
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.ApiKeys.Insert(db.InsertKeyInput{
		GroupID: group.ID, Name: "Private server credential", Key: "cashier-metadata-test-credential", IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	external := "EXT-PUBLIC-42"
	order := cashierMetadataOrder(t, store, &group.ID, &external)
	if err := store.Orders.Update(order.OrderNo, map[string]any{
		"apiKeyId": key.ID, "signingSecret": "cashier-metadata-test-signing-secret", "signatureVersion": 2,
	}); err != nil {
		t.Fatal(err)
	}
	cashier := NewNativePay(config.Map{}, store, nil, nil)
	info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"groupid", "apikey", "signingsecret", "webhook", "clientip", "clientlocation", "members",
		"cashier-metadata-test-credential", "cashier-metadata-test-signing-secret", strings.ToLower(group.WebhookSecret),
	} {
		if forbidden != "" && strings.Contains(lower, forbidden) {
			t.Fatal("cashier metadata exposed a private field or credential")
		}
	}
	if cashierMetadataJSON(t, info)["projectName"] != group.Name {
		t.Fatal("public display name should remain visible")
	}
}

type cashierMetadataAlipayQuery struct {
	store *db.Store
	t     *testing.T
}

func (p *cashierMetadataAlipayQuery) QueryAlipayTrade(orderNo string) bool {
	if err := p.store.Orders.Update(orderNo, map[string]any{
		"status": db.StatusPaid, "payAt": time.Now(), "thirdPartyTradeNo": "ALIPAY_METADATA_CONFIRMED",
	}); err != nil {
		p.t.Fatal(err)
	}
	return true
}

func TestCashierMetadataSurvivesActiveAlipayConfirmation(t *testing.T) {
	store, _ := cashierMetadataStore(t)
	group, err := store.Groups.Insert("Alipay Merchant", true)
	if err != nil {
		t.Fatal(err)
	}
	external := "EXT-ALIPAY-42"
	order := cashierMetadataOrder(t, store, &group.ID, &external)
	if err := store.Orders.Update(order.OrderNo, map[string]any{"payMethod": "alipay"}); err != nil {
		t.Fatal(err)
	}
	cashier := NewNativePay(config.Map{}, store, nil, &cashierMetadataAlipayQuery{store: store, t: t})
	info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
	if err != nil || info == nil || info.Status != "paid" || info.PayMethod != "alipay" {
		t.Fatalf("active confirmation failed: %v", err)
	}
	body := cashierMetadataJSON(t, info)
	if body["projectName"] != group.Name || body["externalOrderNo"] != external || body["createdAt"] != services.JsTime(order.CreatedAt) {
		t.Fatalf("active confirmation lost shared order metadata: %+v", body)
	}
}
