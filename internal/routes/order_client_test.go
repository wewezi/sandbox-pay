package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func assertRecordedOrderClient(t *testing.T, h *harness, o *db.PaymentOrder, wantIP string) {
	t.Helper()
	if o.ClientIP == nil || *o.ClientIP != wantIP {
		t.Fatalf("recorded client IP = %v, want %q", o.ClientIP, wantIP)
	}
	if o.ClientLocation == nil {
		t.Fatal("valid client IP has no persisted location snapshot")
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(*o.ClientLocation), &snapshot); err != nil {
		t.Fatalf("location snapshot is not JSON: %v", err)
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/" + o.OrderNo, Headers: h.auth})
	eq(t, "detail status", r.Status, http.StatusOK)
	d := r.data()
	eq(t, "detail clientIp", str(t, d["clientIp"], "clientIp"), wantIP)
	if !reflect.DeepEqual(d["clientLocation"], snapshot) {
		t.Fatalf("detail location differs from creation snapshot: got=%#v want=%#v", d["clientLocation"], snapshot)
	}
	list := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions?keyword=" + o.OrderNo, Headers: h.auth})
	eq(t, "list status", list.Status, http.StatusOK)
	items := list.data()["items"].([]any)
	eq(t, "list count", len(items), 1)
	item := items[0].(map[string]any)
	eq(t, "list clientIp", item["clientIp"], any(wantIP))
	if !reflect.DeepEqual(item["clientLocation"], snapshot) {
		t.Fatalf("list location differs from creation snapshot: got=%#v want=%#v", item["clientLocation"], snapshot)
	}
}

func TestOrderCreationRecordsCanonicalClientIPWithTrustedProxyRules(t *testing.T) {
	cases := []struct {
		name, peer, forwarded, realIP, trusted, want string
	}{
		{"ipv4", "8.8.8.8", "", "", "", "8.8.8.8"},
		{"ipv6", "2001:4860:4860::8888", "", "", "", "2001:4860:4860::8888"},
		{"mapped ipv4", "::ffff:8.8.4.4", "", "", "", "8.8.4.4"},
		{"untrusted forwarded spoof", "8.8.8.8", "1.1.1.1", "9.9.9.9", "", "8.8.8.8"},
		{"trusted proxy", "10.1.1.1", "1.1.1.1", "", "10.0.0.0/8", "1.1.1.1"},
		{"trusted chain", "10.1.1.1", "9.9.9.9, 1.1.1.1, 10.2.2.2", "", "10.0.0.0/8", "1.1.1.1"},
		{"trusted mapped forwarded", "10.1.1.1", "::ffff:8.8.4.4", "", "10.0.0.0/8", "8.8.4.4"},
		{"malformed forwarded fails closed", "10.1.1.1", "not-an-ip", "1.1.1.1", "10.0.0.0/8", "10.1.1.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, map[string]string{"TRUSTED_PROXY_CIDRS": tc.trusted})
			r := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", IP: tc.peer,
				Headers: withHeaders(h.groupAuth, map[string]string{"X-Forwarded-For": tc.forwarded, "X-Real-IP": tc.realIP}),
				Body:    map[string]any{"amount": 1.23, "productName": "IP fixture", "payMethod": "native"},
			})
			eq(t, "create status", r.Status, http.StatusOK)
			o, err := h.store.Orders.FindByOrderNo(cashierOrderNo(t, r), false)
			if err != nil || o == nil {
				t.Fatalf("order not persisted: %+v, %v", o, err)
			}
			assertRecordedOrderClient(t, h, o, tc.want)
		})
	}
}

func TestEveryHTTPOrderCreationPathRecordsRequestSource(t *testing.T) {
	for _, path := range []string{
		"/v1/api/gateway/pay", "/v1/api/gateway/native/pay", "/v1/api/gateway/alipay/pay", "/v1/api/gateway/paypal/pay",
		"/v1/api/admin/test-pay", "/v1/api/native-pay/public-test-pay",
	} {
		t.Run(path, func(t *testing.T) {
			h := newHarness(t, nil)
			headers := h.groupAuth
			body := map[string]any{"amount": 1.23, "productName": "request source fixture"}
			if path == "/v1/api/gateway/pay" {
				body["payMethod"] = "native"
			}
			if path == "/v1/api/admin/test-pay" {
				headers = h.auth
			}
			if path == "/v1/api/native-pay/public-test-pay" {
				headers = nil
				// The public fixed-amount demo ignores its body; it cannot override
				// metadata using an attacker-supplied value either.
				body = map[string]any{"clientIp": "1.1.1.1", "clientLocation": map[string]any{"country": "spoofed"}}
			}
			r := h.do(t, reqSpec{Method: "POST", Path: path, IP: "8.8.8.8", Headers: headers, Body: body})
			if path != "/v1/api/gateway/alipay/pay" && path != "/v1/api/gateway/paypal/pay" {
				eq(t, "create status", r.Status, http.StatusOK)
			}
			// Unconfigured provider dispatch may fail after creation; the source
			// must still be recorded on the order used for diagnosing that failure.
			orders, count, err := h.store.Orders.Search(db.SearchOpts{Limit: 20})
			if err != nil || count != 1 || len(orders) != 1 {
				t.Fatalf("order missing after %s: %+v count=%d err=%v response=%s", path, orders, count, err, r.Raw)
			}
			assertRecordedOrderClient(t, h, orders[0], "8.8.8.8")
		})
	}
}

func TestOrderCreationRejectsBodySuppliedClientMetadata(t *testing.T) {
	for _, path := range []string{"/v1/api/gateway/pay", "/v1/api/gateway/native/pay", "/v1/api/admin/test-pay"} {
		for _, field := range []string{"clientIp", "clientLocation"} {
			t.Run(path+"/"+field, func(t *testing.T) {
				h := newHarness(t, nil)
				headers := h.groupAuth
				body := map[string]any{"amount": 1.23, "productName": "spoof fixture", field: "spoofed"}
				if path == "/v1/api/gateway/pay" {
					body["payMethod"] = "native"
				}
				if path == "/v1/api/admin/test-pay" {
					headers = h.auth
				}
				r := h.do(t, reqSpec{Method: "POST", Path: path, IP: "8.8.8.8", Headers: headers, Body: body})
				eq(t, "spoofed body rejected", r.Status, http.StatusBadRequest)
				count, err := h.store.Orders.Count()
				if err != nil || count != 0 {
					t.Fatalf("invalid request created an order: count=%d err=%v", count, err)
				}
			})
		}
	}
}

func TestOrderIdempotencyRetainsOriginalClientSnapshot(t *testing.T) {
	h := newHarness(t, nil)
	body := map[string]any{"amount": 1.23, "productName": "idempotent IP fixture", "payMethod": "native", "externalOrderNo": "IP-IDEMPOTENT-001"}
	first := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", IP: "8.8.8.8", Headers: h.groupAuth, Body: body})
	eq(t, "first create status", first.Status, http.StatusOK)
	orderNo := cashierOrderNo(t, first)
	original, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || original == nil || original.ClientLocation == nil {
		t.Fatalf("original metadata missing: %+v, err=%v", original, err)
	}
	retry := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", IP: "1.1.1.1", Headers: h.groupAuth, Body: body})
	eq(t, "retry create status", retry.Status, http.StatusOK)
	eq(t, "same idempotent order", cashierOrderNo(t, retry), orderNo)
	o, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || o == nil || o.ClientLocation == nil || *o.ClientLocation != *original.ClientLocation {
		t.Fatalf("retry overwrote original location snapshot: %+v, err=%v", o, err)
	}
	assertRecordedOrderClient(t, h, o, "8.8.8.8")
}

func TestInvalidRequestAddressDoesNotInventClientMetadata(t *testing.T) {
	for _, peer := range []string{"not-an-ip", "0.0.0.0", "224.0.0.1", "fe80::1%eth0"} {
		t.Run(peer, func(t *testing.T) {
			h := newHarness(t, nil)
			r := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/test-pay", IP: peer, Headers: h.auth, Body: map[string]any{"amount": 1.23}})
			eq(t, "create status", r.Status, http.StatusOK)
			o, err := h.store.Orders.FindByOrderNo(cashierOrderNo(t, r), false)
			if err != nil || o == nil || o.ClientIP != nil || o.ClientLocation != nil {
				t.Fatalf("invalid peer produced invented source metadata: %+v, err=%v", o, err)
			}
		})
	}
}

func TestMissingRequestAddressLeavesClientMetadataAbsent(t *testing.T) {
	ip, location := "1.1.1.1", `{"network":"public","country":"spoofed"}`
	keyID := int64(7)
	caller := services.OrderCaller{GroupID: 3, APIKeyID: &keyID, ClientIP: &ip, ClientLocation: &location}
	r := httptest.NewRequest(http.MethodPost, "/v1/api/admin/test-pay", nil)
	r.RemoteAddr = ""
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	a := &App{c: &Container{}}
	got := a.orderClient(r, caller)
	if got.ClientIP != nil || got.ClientLocation != nil || got.GroupID != caller.GroupID || got.APIKeyID != caller.APIKeyID {
		t.Fatalf("missing peer address invented source or changed caller identity: %+v", got)
	}
}

func TestMalformedHistoricalLocationDoesNotBreakOrderResponses(t *testing.T) {
	h := newHarness(t, nil)
	r := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/test-pay", IP: "8.8.8.8", Headers: h.auth, Body: map[string]any{"amount": 1.23}})
	eq(t, "create status", r.Status, http.StatusOK)
	orderNo := cashierOrderNo(t, r)
	for _, invalid := range []any{nil, "not-json", "null", `[]`, `{"network":{}}`} {
		if err := h.store.Orders.Update(orderNo, map[string]any{"clientLocation": invalid}); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/v1/api/admin/transactions/" + orderNo, "/v1/api/admin/transactions?keyword=" + orderNo} {
			response := h.do(t, reqSpec{Method: "GET", Path: path, Headers: h.auth})
			eq(t, "historical detail/list status", response.Status, http.StatusOK)
			data := response.data()
			if items, ok := data["items"].([]any); ok {
				data = items[0].(map[string]any)
			}
			if _, present := data["clientLocation"]; present {
				t.Fatalf("invalid historical snapshot exposed as a location: %#v", data["clientLocation"])
			}
			eq(t, "IP retained despite invalid location", data["clientIp"], any("8.8.8.8"))
		}
	}
}
