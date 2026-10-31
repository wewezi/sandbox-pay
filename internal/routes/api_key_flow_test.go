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

func TestBearerSchemeCompatibility(t *testing.T) {
	h := newHarness(t, nil)
	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "bEaReR"} {
		t.Run(scheme, func(t *testing.T) {
			query := h.do(t, reqSpec{
				Method: "GET", Path: "/v1/api/gateway/query?orderNo=NOPE",
				Headers: map[string]string{"Authorization": scheme + " " + h.apiKey},
			})
			eq(t, "valid API token reaches query", query.Status, http.StatusNotFound)
			admin := h.do(t, reqSpec{
				Method: "GET", Path: "/v1/api/admin/groups",
				Headers: map[string]string{"Authorization": scheme + " " + strings.TrimPrefix(h.auth["Authorization"], "Bearer ")},
			})
			eq(t, "valid admin token reaches groups", admin.Status, http.StatusOK)
		})
	}
	for _, auth := range []string{"Bearer", "Basic " + h.apiKey, "Bearer " + strings.ToUpper(h.apiKey), "Bearer " + h.apiKey + " extra", h.auth["Authorization"]} {
		query := h.do(t, reqSpec{
			Method: "GET", Path: "/v1/api/gateway/query?orderNo=NOPE",
			Headers: map[string]string{"Authorization": auth},
		})
		eq(t, "invalid or wrong kind of token rejected", query.Status, http.StatusUnauthorized)
	}
	admin := h.do(t, reqSpec{
		Method: "GET", Path: "/v1/api/admin/groups", Headers: h.groupAuth,
	})
	eq(t, "API token cannot administer tokens", admin.Status, http.StatusUnauthorized)
}

func TestIssuedAPIKeyPaymentLifecycle(t *testing.T) {
	h := newHarness(t, nil)
	adminPost := func(path string, body map[string]any) *resp {
		t.Helper()
		return h.do(t, reqSpec{Method: "POST", Path: path, Body: body,
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"})})
	}
	created := adminPost("/v1/api/admin/groups", map[string]any{"name": "令牌链路", "keyName": "原令牌"})
	eq(t, "create group", created.Status, http.StatusOK)
	group := created.data()
	groupID := int64(num(t, group["id"], "group id"))
	configuration := h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/groups/%d/webhook-secret", groupID), Headers: h.auth})
	eq(t, "read independent signing secret", configuration.Status, http.StatusOK)
	webhookSecret := str(t, configuration.data()["secret"], "webhook secret")
	first := group["keys"].([]any)[0].(map[string]any)
	firstID := int64(num(t, first["id"], "first key id"))
	firstSecret := str(t, first["key"], "first key")
	issued := adminPost(fmt.Sprintf("/v1/api/admin/groups/%d/keys", groupID), map[string]any{"name": "轮换令牌"})
	eq(t, "issue replacement", issued.Status, http.StatusOK)
	secondID := int64(num(t, issued.data()["id"], "second key id"))
	secondSecret := str(t, issued.data()["key"], "second key")
	if secondID == firstID || secondSecret == firstSecret {
		t.Fatal("issuing replacement reused original token")
	}
	if webhookSecret == firstSecret || webhookSecret == secondSecret {
		t.Fatal("webhook secret must be independent of API tokens")
	}
	firstHeaders := map[string]string{"Authorization": "Bearer " + firstSecret, "Content-Type": "application/json"}
	secondHeaders := map[string]string{"Authorization": "Bearer " + secondSecret, "Content-Type": "application/json"}
	payBody := map[string]any{"amount": 2.5, "productName": "令牌订单", "payMethod": "native",
		"externalOrderNo": "TOKEN-FLOW-001", "notifyUrl": "https://merchant.example/notify"}
	pay := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Body: payBody, Headers: firstHeaders})
	eq(t, "newly issued token pays", pay.Status, http.StatusOK)
	orderNo := cashierOrderNo(t, pay)
	key, err := h.store.ApiKeys.FindByID(firstID)
	if err != nil || key == nil || key.LastUsedAt == nil {
		t.Fatalf("successful authentication did not record last use: %v", err)
	}
	lastUsed := key.LastUsedAt.Add(-time.Hour)
	if err := h.store.ApiKeys.Update(firstID, map[string]any{"lastUsedAt": lastUsed}); err != nil {
		t.Fatal(err)
	}
	order, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || order == nil || order.APIKeyID == nil || *order.APIKeyID != firstID || order.GroupID == nil || *order.GroupID != groupID {
		t.Fatalf("issued token context was not saved on order: %v", err)
	}
	if order.SigningSecret == nil || *order.SigningSecret != webhookSecret || order.SignatureVersion != 2 {
		t.Fatal("order must snapshot project webhook secret and signing version")
	}
	retry := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Body: payBody, Headers: secondHeaders})
	eq(t, "same group replacement retries", retry.Status, http.StatusOK)
	eq(t, "retry preserves original order", cashierOrderNo(t, retry), orderNo)
	for _, path := range []string{"/v1/api/gateway/query?orderNo=" + orderNo, "/v1/api/gateway/refund"} {
		method := "GET"
		var body map[string]any
		if strings.HasSuffix(path, "/refund") {
			method, body = "POST", map[string]any{"orderNo": orderNo, "amount": 2.5}
		}
		denied := h.do(t, reqSpec{Method: method, Path: path, Body: body, Headers: h.groupAuth})
		eq(t, "another group cannot access order", denied.Status, http.StatusNotFound)
	}
	off := adminPost(fmt.Sprintf("/v1/api/admin/api-keys/%d/toggle", firstID), nil)
	eq(t, "disable original", off.Status, http.StatusOK)
	denied := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Body: payBody, Headers: firstHeaders})
	eq(t, "disabled token rejected", denied.Status, http.StatusUnauthorized)
	key, err = h.store.ApiKeys.FindByID(firstID)
	if err != nil || key == nil || key.LastUsedAt == nil || !key.LastUsedAt.Equal(lastUsed) {
		t.Fatalf("rejected authentication changed last use: %v", err)
	}
	groupOff := adminPost(fmt.Sprintf("/v1/api/admin/groups/%d/toggle", groupID), nil)
	eq(t, "disable group", groupOff.Status, http.StatusOK)
	denied = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + orderNo, Headers: secondHeaders})
	eq(t, "disabled group blocks active replacement", denied.Status, http.StatusUnauthorized)
	confirmed := adminPost("/v1/api/native-pay/confirm", map[string]any{
		"orderNo": orderNo, "walletUser": "wallet-user", "walletPass": "secret1",
	})
	eq(t, "existing order still completes", confirmed.Status, http.StatusOK)
	partial := adminPost("/v1/api/admin/refund", map[string]any{"orderNo": orderNo, "amount": 0.5})
	eq(t, "existing order still partially refunds", partial.Status, http.StatusOK)
	refunded := adminPost("/v1/api/admin/refund", map[string]any{"orderNo": orderNo, "amount": 2})
	eq(t, "existing order still refunds", refunded.Status, http.StatusOK)
	callbacks, total, err := h.store.Notifies.Search(db.NotifySearchOpts{OrderNo: orderNo, Limit: 10})
	if err != nil || total != 3 || len(callbacks) != 3 {
		t.Fatalf("payment/refund callbacks missing: count=%d error=%v", total, err)
	}
	statuses := make(map[string]bool)
	for _, callback := range callbacks {
		var meta struct {
			Timestamp string `json:"timestamp"`
			Nonce     string `json:"nonce"`
			Sign      string `json:"sign"`
			Version   int    `json:"version"`
		}
		if err := json.Unmarshal([]byte(callback.Signature), &meta); err != nil {
			t.Fatal(err)
		}
		payload := signature.BuildPayload(callback.Body, meta.Timestamp, meta.Nonce)
		if !signature.Verify(payload, webhookSecret, meta.Sign) || signature.Verify(payload, firstSecret, meta.Sign) || signature.Verify(payload, secondSecret, meta.Sign) || meta.Version != 2 {
			t.Fatal("callback must retain independent project signing snapshot after rotation/disable")
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(callback.Body), &body); err != nil {
			t.Fatal(err)
		}
		statuses[body.Status] = true
	}
	if !statuses["paid"] || !statuses["partial_refunded"] || !statuses["refunded"] {
		t.Fatalf("expected payment, partial refund and full refund events: %v", statuses)
	}
	eq(t, "enable group", adminPost(fmt.Sprintf("/v1/api/admin/groups/%d/toggle", groupID), nil).Status, http.StatusOK)
	denied = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + orderNo, Headers: firstHeaders})
	eq(t, "enabling group keeps original token disabled", denied.Status, http.StatusUnauthorized)
	query := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + orderNo, Headers: secondHeaders})
	eq(t, "replacement restored with group", query.Status, http.StatusOK)
	eq(t, "new order's expected callback version", num(t, query.data()["callbackSignatureVersion"], "callback version"), 2)
	eq(t, "enable original", adminPost(fmt.Sprintf("/v1/api/admin/api-keys/%d/toggle", firstID), nil).Status, http.StatusOK)
	query = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + orderNo, Headers: firstHeaders})
	eq(t, "original token restored explicitly", query.Status, http.StatusOK)
	lacksSubstr(t, query.Raw, firstSecret, "gateway order response")
}

func TestHistoricalOrderRetainsLegacySigningSnapshot(t *testing.T) {
	h := newHarness(t, nil)
	key, err := h.store.ApiKeys.FindByKey(testAPIKey)
	if err != nil || key == nil || key.Key != "" {
		t.Fatalf("hashed credential lookup failed: %v", err)
	}
	project, err := h.store.Groups.FindByID(h.groupID)
	if err != nil || project == nil {
		t.Fatalf("project lookup failed: %v", err)
	}
	legacySecret, notifyURL := testAPIKey, "https://merchant.example/legacy-notify"
	order := &db.PaymentOrder{
		ID: "legacy-signing-fixture", OrderNo: "PAYLEGACYSIGNING", Amount: 100,
		ProductName: "历史订单", PayMethod: "native", Status: db.StatusPending,
		ExpireAt: time.Now().Add(time.Hour), GroupID: &h.groupID, APIKeyID: &key.ID,
		NotifyURL: &notifyURL, SigningSecret: &legacySecret, SignatureVersion: 1,
	}
	if err := h.store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}
	query := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + order.OrderNo, Headers: h.groupAuth})
	eq(t, "query legacy order", query.Status, http.StatusOK)
	eq(t, "bind expected historical signature version", num(t, query.data()["callbackSignatureVersion"], "callback version"), 1)
	if _, err := h.c.Groups.ToggleKey(key.ID); err != nil {
		t.Fatal(err)
	}
	confirmed := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/native-pay/confirm",
		Body:    map[string]any{"orderNo": order.OrderNo, "walletUser": "wallet-user", "walletPass": "secret1"},
		Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"})})
	eq(t, "historical order completes after token disabled", confirmed.Status, http.StatusOK)
	callbacks, total, err := h.store.Notifies.Search(db.NotifySearchOpts{OrderNo: order.OrderNo, Limit: 10})
	if err != nil || total != 1 || len(callbacks) != 1 {
		t.Fatalf("legacy callback missing: %v", err)
	}
	var meta struct {
		Timestamp string `json:"timestamp"`
		Nonce     string `json:"nonce"`
		Sign      string `json:"sign"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(callbacks[0].Signature), &meta); err != nil {
		t.Fatal(err)
	}
	payload := signature.BuildPayload(callbacks[0].Body, meta.Timestamp, meta.Nonce)
	if !signature.Verify(payload, legacySecret, meta.Sign) || signature.Verify(payload, project.WebhookSecret, meta.Sign) || meta.Version > 1 {
		t.Fatal("historical callback changed signing rules")
	}
}
