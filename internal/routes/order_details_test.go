package routes

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func detailString(value string) *string { return &value }

func TestAdminOrderDetailsRequiresAdminAndHandlesMissingOrder(t *testing.T) {
	h := newHarness(t, nil)
	for _, headers := range []map[string]string{nil, h.groupAuth, {"Authorization": "Bearer invalid-token"}} {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/NOPE", Headers: headers})
		eq(t, "unauthorized", r.Status, http.StatusUnauthorized)
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/NOPE", Headers: h.auth})
	eq(t, "missing order", r.Status, http.StatusNotFound)
}

func TestAdminOrderDetailsIncludesReconciliationRecordsWithoutCredentials(t *testing.T) {
	h := newHarness(t, nil)
	group, key, _, err := h.c.Groups.EnsureSeed()
	if err != nil {
		t.Fatal(err)
	}
	paidAt := time.Date(2026, 10, 31, 8, 0, 0, 123000000, time.UTC)
	refundedAt := paidAt.Add(time.Hour)
	expiresAt := paidAt.Add(30 * time.Minute)
	rate, foreignAmount := 0.05, int64(617)
	order := &db.PaymentOrder{
		ID: "order-detail-fixture", OrderNo: "PAYDETAILFIXTURE", Amount: 12345,
		ProductName: "详细记录测试商品", PayMethod: "paypal", Status: db.StatusRefunding,
		ExpireAt: expiresAt, PayAt: &paidAt, GroupID: &group.ID, APIKeyID: &key.ID,
		ExternalOrderNo: detailString("EXT-DETAIL-001"), ThirdPartyTradeNo: detailString("CAPTURE-001"),
		ProviderOrderID: detailString("PROVIDER-001"), ReturnURL: detailString("https://merchant.example/return"),
		NotifyURL: detailString("https://merchant.example/notify"), ExchangeRate: &rate,
		ForeignAmount: &foreignAmount, ForeignCurrency: detailString("USD"),
		RefundedAmount: 100, RefundTradeNo: detailString("REFUND-001"), RefundAt: &refundedAt,
		Fee: 45, ChannelCost: 34, SettleAmount: 12300,
		SigningSecret: detailString("private-order-signing-snapshot"), SignatureVersion: 2,
	}
	if err := h.store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}
	other := &db.PaymentOrder{ID: "order-detail-other", OrderNo: "PAYDETAILOTHER", Amount: 100, ProductName: "other", PayMethod: "native", ExpireAt: expiresAt}
	if err := h.store.Orders.Insert(other); err != nil {
		t.Fatal(err)
	}
	for _, op := range []*db.RefundOperation{
		{ID: "refund-detail-completed", RequestID: "request-completed", OrderNo: order.OrderNo, Amount: 100, TradeNo: order.RefundTradeNo, Status: db.RefundCompleted, Reason: "部分退款"},
		{ID: "refund-detail-pending", RequestID: "request-pending", OrderNo: order.OrderNo, Amount: 500, Status: db.RefundPending, Reason: "等待渠道确认"},
		{ID: "refund-detail-other", RequestID: "request-other", OrderNo: other.OrderNo, Amount: 1, Status: db.RefundCompleted},
	} {
		if err := h.store.Refunds.Insert(op); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []db.InsertNotifyInput{
		{OrderNo: &order.OrderNo, URL: *order.NotifyURL, Body: `{"event":"paid"}`, Signature: "private-signature-old", Status: db.NotifySuccess},
		{OrderNo: &order.OrderNo, URL: *order.NotifyURL, Body: `{"event":"refunded","amount":1}`, Signature: "private-signature-latest", Status: db.NotifyFailed},
		{OrderNo: &other.OrderNo, URL: "https://other.example/notify", Body: "other-order-body", Signature: "other-private-signature", Status: db.NotifyExhausted},
	} {
		if err := h.store.Notifies.Insert(n); err != nil {
			t.Fatal(err)
		}
	}
	notifications, err := h.store.Notifies.ListByOrderNo(order.OrderNo)
	if err != nil || len(notifications) != 2 {
		t.Fatalf("notifications: %v %v", notifications, err)
	}
	latest := notifications[0]
	if err := h.store.Notifies.Update(latest.ID, map[string]any{"retryCount": int64(4), "lastError": "HTTP 502", "lastAttemptAt": refundedAt}); err != nil {
		t.Fatal(err)
	}
	// Disabling credentials must not erase attribution from historical records.
	if err := h.store.ApiKeys.Update(key.ID, map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.Groups.Update(group.ID, map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/" + order.OrderNo, Headers: h.auth})
	eq(t, "detail status", r.Status, http.StatusOK)
	d := r.data()
	for name, want := range map[string]float64{"amount": 123.45, "refundedAmount": 1, "fee": .45, "channelCost": .34, "settleAmount": 123, "foreignAmount": 6.17, "exchangeRate": rate, "rawStatus": 3, "pendingRefundAmount": 5, "notifyStatus": 2, "notifyRetryCount": 4} {
		eq(t, name, num(t, d[name], name), want)
	}
	for name, want := range map[string]string{
		"orderNo": order.OrderNo, "productName": order.ProductName, "externalOrderNo": *order.ExternalOrderNo,
		"thirdPartyTradeNo": *order.ThirdPartyTradeNo, "providerOrderId": *order.ProviderOrderID,
		"returnUrl": *order.ReturnURL, "notifyUrl": *order.NotifyURL, "foreignCurrency": "USD",
		"groupName": group.Name, "apiKeyName": key.Name, "refundTradeNo": *order.RefundTradeNo,
		"payAt": services.JsTime(paidAt), "expireAt": services.JsTime(expiresAt), "refundAt": services.JsTime(refundedAt),
		"refundRequestId": "request-pending", "notifyLastError": "HTTP 502",
	} {
		eq(t, name, str(t, d[name], name), want)
	}
	for _, name := range []string{"createdAt", "updatedAt"} {
		if _, err := time.Parse(time.RFC3339Nano, str(t, d[name], name)); err != nil {
			t.Fatalf("%s is not an ISO timestamp: %v", name, err)
		}
	}
	eq(t, "groupId", num(t, d["groupId"], "groupId"), float64(group.ID))
	eq(t, "apiKeyId", num(t, d["apiKeyId"], "apiKeyId"), float64(key.ID))
	refunds := d["refunds"].([]any)
	eq(t, "matching refunds only", len(refunds), 2)
	if refunds[0].(map[string]any)["amount"] != float64(1) || refunds[1].(map[string]any)["amount"] != float64(5) {
		t.Fatalf("refund amounts not in yuan: %+v", refunds)
	}
	callbacks := d["notifications"].([]any)
	eq(t, "matching notifications only", len(callbacks), 2)
	eq(t, "latest notification first", num(t, callbacks[0].(map[string]any)["id"], "latest id"), float64(latest.ID))
	eq(t, "callback body", str(t, callbacks[0].(map[string]any)["body"], "body"), latest.Body)
	for _, private := range []string{testAPIKey, key.KeyHash, group.WebhookSecret, *order.SigningSecret, "private-signature", "other-private-signature", "other-order-body", "request-other"} {
		if strings.Contains(r.Raw, private) {
			t.Fatalf("detail included unrelated/private data %q", private)
		}
	}
	list := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions?keyword=EXT-DETAIL-001", Headers: h.auth})
	eq(t, "list status", list.Status, http.StatusOK)
	items := list.data()["items"].([]any)
	eq(t, "list length", len(items), 1)
	item := items[0].(map[string]any)
	eq(t, "list historical source", item["apiKeyName"], any(key.Name))
	eq(t, "list latest notification", item["notifyStatus"], any(float64(db.NotifyFailed)))
	eq(t, "list active refund", item["refundRequestId"], any("request-pending"))
	if strings.Contains(list.Raw, testAPIKey) || strings.Contains(list.Raw, "private-signature") || strings.Contains(list.Raw, *order.SigningSecret) {
		t.Fatal("list included credentials")
	}
}

func TestAdminOrderDetailsKeepsExpiredStatusAndEmptyArrays(t *testing.T) {
	h := newHarness(t, nil)
	o := &db.PaymentOrder{ID: "detail-expired", OrderNo: "PAYEXPIREDDETAIL", Amount: 1, ProductName: "expired", PayMethod: "native", Status: db.StatusExpired, ExpireAt: time.Now().Add(-time.Hour)}
	if err := h.store.Orders.Insert(o); err != nil {
		t.Fatal(err)
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/" + o.OrderNo, Headers: h.auth})
	eq(t, "expired detail status", r.Status, http.StatusOK)
	d := r.data()
	eq(t, "raw expired status", num(t, d["rawStatus"], "rawStatus"), float64(db.StatusExpired))
	for _, name := range []string{"refunds", "notifications"} {
		items, ok := d[name].([]any)
		if !ok || len(items) != 0 {
			t.Fatalf("%s must be [], got %#v", name, d[name])
		}
	}
	for _, name := range []string{"apiKeyId", "apiKeyName", "notifyStatus", "notifyRetryCount", "refundRequestId", "pendingRefundAmount"} {
		if _, exists := d[name]; exists {
			t.Errorf("unrecorded optional field %s should be absent", name)
		}
	}
}
