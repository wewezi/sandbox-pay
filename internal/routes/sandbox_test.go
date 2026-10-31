package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/signature"
)

func testPayPost(t *testing.T, h *harness, body map[string]any) *resp {
	t.Helper()
	return h.do(t, reqSpec{
		Method: "POST", Path: "/v1/api/admin/test-pay", Body: body,
		Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
	})
}

func TestAdminTestPayDefaults(t *testing.T) {
	h := newHarness(t, nil)
	r := testPayPost(t, h, map[string]any{"amount": 0.01})
	eq(t, "status", r.Status, http.StatusOK)
	d := r.data()
	eq(t, "legacy result type", d["type"], "url")
	eq(t, "amount", d["amount"], 0.01)
	orderNo := str(t, d["orderNo"], "orderNo")
	if !strings.Contains(str(t, d["data"], "data"), "/cashier?orderNo="+orderNo) {
		t.Fatalf("cashier URL does not contain orderNo: %s", r.Raw)
	}
	if _, err := time.Parse(time.RFC3339, str(t, d["expireAt"], "expireAt")); err != nil {
		t.Fatalf("invalid expiry: %v", err)
	}
	order, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || order == nil {
		t.Fatalf("persisted order missing: %v", err)
	}
	eq(t, "product", order.ProductName, "Mintweft Test Order")
	eq(t, "amount cents", order.Amount, int64(1))
	eq(t, "channel", order.PayMethod, "native")
	eq(t, "group", *order.GroupID, h.groupID)
	if order.APIKeyID != nil || order.NotifyURL != nil || order.ReturnURL != nil || order.ExternalOrderNo != nil {
		t.Fatalf("default order acquired optional context: %#v", order)
	}
}

func TestAdminTestPayStoresContextAndSignsCallback(t *testing.T) {
	h := newHarness(t, nil)
	group, err := h.c.Groups.CreateGroup("本地联调", "CI")
	if err != nil {
		t.Fatal(err)
	}
	key := group.Keys[0]
	project, err := h.store.Groups.FindByID(group.ID)
	if err != nil || project == nil {
		t.Fatalf("project configuration missing: %v", err)
	}
	body := map[string]any{
		"amount": 12.99, "productName": "开发订单", "externalOrderNo": "DEV-001", "apiKeyId": key.ID,
		"notifyUrl": "https://merchant.example/notify", "returnUrl": "https://merchant.example/result?from=sandbox",
	}
	r := testPayPost(t, h, body)
	eq(t, "status", r.Status, http.StatusOK)
	d := r.data()
	orderNo := str(t, d["orderNo"], "orderNo")
	eq(t, "group label", d["groupName"], "本地联调")
	eq(t, "key label", d["apiKeyName"], "CI")
	order, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || order == nil {
		t.Fatalf("order missing: %v", err)
	}
	eq(t, "group", *order.GroupID, group.ID)
	eq(t, "key", *order.APIKeyID, key.ID)
	eq(t, "external order", *order.ExternalOrderNo, "DEV-001")
	eq(t, "notify URL", *order.NotifyURL, "https://merchant.example/notify")
	eq(t, "return URL", *order.ReturnURL, "https://merchant.example/result?from=sandbox")
	if strings.Contains(string(r.Raw), key.Key) {
		t.Fatal("test-pay receipt leaked Key secret")
	}
	tradeNo := "NATIVE_DEV_001"
	if _, err := h.c.Payment.MarkPaid(orderNo, &tradeNo, 1299, "CNY"); err != nil {
		t.Fatal(err)
	}
	callbacks, total, err := h.store.Notifies.Search(db.NotifySearchOpts{Limit: 20})
	if err != nil || total != 1 || len(callbacks) != 1 {
		t.Fatalf("callback not queued: total=%d rows=%d err=%v", total, len(callbacks), err)
	}
	callback := callbacks[0]
	eq(t, "callback order", *callback.OrderNo, orderNo)
	eq(t, "callback destination", callback.URL, "https://merchant.example/notify")
	var sign struct {
		Timestamp string `json:"timestamp"`
		Nonce     string `json:"nonce"`
		Sign      string `json:"sign"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(callback.Signature), &sign); err != nil {
		t.Fatal(err)
	}
	payload := signature.BuildPayload(callback.Body, sign.Timestamp, sign.Nonce)
	if !signature.Verify(payload, project.WebhookSecret, sign.Sign) || signature.Verify(payload, key.Key, sign.Sign) || sign.Version != 2 {
		t.Fatal("callback must use the selected project's independent webhook secret")
	}
	var notification map[string]any
	if err := json.Unmarshal([]byte(callback.Body), &notification); err != nil {
		t.Fatal(err)
	}
	eq(t, "callback external order", notification["externalOrderNo"], "DEV-001")
	eq(t, "callback amount", notification["amount"], 12.99)
}

func TestAdminTestPayIdempotencyPreservesStoredReceipt(t *testing.T) {
	h := newHarness(t, nil)
	group, err := h.c.Groups.CreateGroup("回调测试", "原始 Key")
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := h.c.Groups.IssueKey(group.ID, "另一个 Key")
	if err != nil {
		t.Fatal(err)
	}
	first := testPayPost(t, h, map[string]any{
		"amount": 2.99, "productName": "原商品", "externalOrderNo": "IDEMPOTENT-001",
		"apiKeyId": group.Keys[0].ID, "notifyUrl": "https://original.example/notify",
	})
	eq(t, "first status", first.Status, http.StatusOK)
	retry := testPayPost(t, h, map[string]any{
		"amount": 2.99, "productName": "修改的商品", "externalOrderNo": "IDEMPOTENT-001",
		"apiKeyId": secondKey.ID, "notifyUrl": "https://changed.example/notify", "returnUrl": "https://changed.example/result",
	})
	eq(t, "retry status", retry.Status, http.StatusOK)
	eq(t, "same order", retry.data()["orderNo"], first.data()["orderNo"])
	eq(t, "original product", retry.data()["productName"], "原商品")
	eq(t, "original callback", retry.data()["notifyUrl"], "https://original.example/notify")
	eq(t, "original key", retry.data()["apiKeyName"], "原始 Key")
	if _, exists := retry.data()["returnUrl"]; exists {
		t.Fatal("retry claimed a new returnUrl was saved")
	}
	stats, err := h.c.Admin.GetStats(nil)
	if err != nil || stats.TotalCount != 1 {
		t.Fatalf("idempotent retry created extra order: stats=%#v err=%v", stats, err)
	}
	changedAmount := testPayPost(t, h, map[string]any{
		"amount": 3.99, "externalOrderNo": "IDEMPOTENT-001", "apiKeyId": group.Keys[0].ID,
	})
	eq(t, "amount conflict", changedAmount.Status, http.StatusBadRequest)
}

func TestAdminTestPayGuardsAndValidation(t *testing.T) {
	t.Run("requires administrator", func(t *testing.T) {
		h := newHarness(t, nil)
		r := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/test-pay", Body: map[string]any{"amount": 1}})
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})
	t.Run("sandbox disabled", func(t *testing.T) {
		h := newHarness(t, map[string]string{"ENABLE_SANDBOX": "false"})
		r := testPayPost(t, h, map[string]any{"amount": 1})
		eq(t, "status", r.Status, http.StatusForbidden)
	})
	for _, value := range []any{0, -1, 1.5, "NaN", "Infinity", 1e20, 999999} {
		t.Run(fmt.Sprintf("invalid Key %v", value), func(t *testing.T) {
			h := newHarness(t, nil)
			r := testPayPost(t, h, map[string]any{"amount": 1, "apiKeyId": value})
			eq(t, "status", r.Status, http.StatusBadRequest)
			stats, err := h.c.Admin.GetStats(nil)
			if err != nil || stats.TotalCount != 0 {
				t.Fatalf("invalid input created order: %#v %v", stats, err)
			}
		})
	}
	t.Run("default project callback needs no API token selection", func(t *testing.T) {
		h := newHarness(t, nil)
		r := testPayPost(t, h, map[string]any{"amount": 1, "notifyUrl": "https://merchant.example/notify"})
		eq(t, "status", r.Status, http.StatusOK)
		order, err := h.store.Orders.FindByOrderNo(str(t, r.data()["orderNo"], "order no"), false)
		if err != nil || order == nil || order.APIKeyID != nil || order.SigningSecret == nil || order.SignatureVersion != 2 {
			t.Fatalf("default project webhook context was not saved: %v", err)
		}
	})
	t.Run("disabled default project cannot create callback order", func(t *testing.T) {
		h := newHarness(t, nil)
		if _, err := h.c.Groups.ToggleGroup(h.groupID); err != nil {
			t.Fatal(err)
		}
		r := testPayPost(t, h, map[string]any{"amount": 1, "notifyUrl": "https://merchant.example/notify"})
		eq(t, "status", r.Status, http.StatusBadRequest)
	})
	for _, disabled := range []string{"key", "group"} {
		t.Run("disabled "+disabled, func(t *testing.T) {
			h := newHarness(t, nil)
			group, key, _, err := h.c.Groups.EnsureSeed()
			if err != nil {
				t.Fatal(err)
			}
			if disabled == "key" {
				_, err = h.c.Groups.ToggleKey(key.ID)
			} else {
				_, err = h.c.Groups.ToggleGroup(group.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			r := testPayPost(t, h, map[string]any{"amount": 1, "apiKeyId": key.ID})
			eq(t, "status", r.Status, http.StatusBadRequest)
		})
	}
}

func TestAdminTestPayWithoutKeyPreservesDisabledSeed(t *testing.T) {
	h := newHarness(t, nil)
	_, key, _, err := h.c.Groups.EnsureSeed()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Groups.ToggleKey(key.ID); err != nil {
		t.Fatal(err)
	}
	r := testPayPost(t, h, map[string]any{"amount": 1})
	eq(t, "default test still works", r.Status, http.StatusOK)
	storedKey, err := h.store.ApiKeys.FindByID(key.ID)
	if err != nil || storedKey.IsActive {
		t.Fatalf("default test changed disabled Key: %#v %v", storedKey, err)
	}
	order, err := h.store.Orders.FindByOrderNo(str(t, r.data()["orderNo"], "orderNo"), false)
	if err != nil || order.APIKeyID != nil {
		t.Fatalf("default test unexpectedly bound Key: %#v %v", order, err)
	}
}
