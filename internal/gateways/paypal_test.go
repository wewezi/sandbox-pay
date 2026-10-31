package gateways

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plutov/paypal"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/services"
)

func testPayPalGateway(t *testing.T, handler http.HandlerFunc) *PayPal {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store, err := db.Open(filepath.Join(t.TempDir(), "paypal.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.Groups.Insert("paypal-test", true); err != nil {
		t.Fatal(err)
	}
	cfg := config.Map{
		"PAYPAL_CLIENT_ID": "test-client", "PAYPAL_CLIENT_SECRET": "test-secret",
		"PAYPAL_ENVIRONMENT": "sandbox", "EXCHANGE_RATE_PROVIDER": "fixed",
		"FALLBACK_CNY_TO_USD": "0.14", "EXCHANGE_RATE_MARGIN": "1",
	}
	notify := services.NewNotifyService(store.Notifies, httpclient.New())
	payments := services.NewPaymentService(store, notify, services.NewFeeCalculator(cfg))
	p := NewPayPal(cfg, store, services.NewExchangeRateService(httpclient.New(), cfg), payments)
	p.cli, err = paypal.NewClient("test-client", "test-secret", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	p.cli.SetHTTPClient(&http.Client{Timeout: time.Second})
	return p
}

func writePayPalJSON(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		t.Error(err)
	}
}

func writePayPalToken(t *testing.T, w http.ResponseWriter, r *http.Request, number int64) {
	t.Helper()
	id, secret, ok := r.BasicAuth()
	if !ok || id != "test-client" || secret != "test-secret" {
		t.Error("OAuth request did not use the configured credentials")
	}
	if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "client_credentials" {
		t.Error("OAuth request did not use the client credentials grant")
	}
	writePayPalJSON(t, w, map[string]any{
		"access_token": fmt.Sprintf("test-token-%d", number), "token_type": "Bearer", "expires_in": 3600,
	})
}

func TestPayPalOAuthConcurrentRefresh(t *testing.T) {
	var tokens, requests atomic.Int64
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, tokens.Add(1))
			return
		}
		requests.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-token-") {
			t.Error("business request lacks OAuth token")
		}
		writePayPalJSON(t, w, map[string]any{"ok": true})
	})
	batch := func() {
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var result map[string]any
				if err := p.send(http.MethodGet, "/test", nil, "", &result); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
	}
	batch()
	if tokens.Load() != 1 {
		t.Fatalf("first concurrent batch acquired %d tokens, want 1", tokens.Load())
	}
	p.mu.Lock()
	p.expiresAt = time.Now().Add(-time.Second)
	p.mu.Unlock()
	batch()
	if tokens.Load() != 2 || requests.Load() != 24 {
		t.Fatalf("tokens=%d requests=%d, want 2/24", tokens.Load(), requests.Load())
	}
}

func TestPayPalOAuthUnauthorizedRetry(t *testing.T) {
	var tokens, attempts atomic.Int64
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, tokens.Add(1))
			return
		}
		if r.Header.Get("PayPal-Request-Id") != "stable-request" {
			t.Error("retry lost the idempotency key")
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			writePayPalJSON(t, w, map[string]string{"name": "AUTHENTICATION_FAILURE"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token-2" {
			t.Error("retry did not acquire a fresh token")
		}
		writePayPalJSON(t, w, map[string]any{"ok": true})
	})
	var result map[string]any
	if err := p.send(http.MethodPost, "/test", map[string]string{"value": "same"}, "stable-request", &result); err != nil {
		t.Fatal(err)
	}
	if tokens.Load() != 2 || attempts.Load() != 2 {
		t.Fatalf("tokens=%d attempts=%d, want 2/2", tokens.Load(), attempts.Load())
	}
}

func TestPayPalRejectsInvalidOAuthToken(t *testing.T) {
	var requests atomic.Int64
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalJSON(t, w, map[string]any{"access_token": "", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		requests.Add(1)
	})
	if err := p.send(http.MethodGet, "/test", nil, "", nil); err == nil || requests.Load() != 0 {
		t.Fatalf("invalid OAuth token sent a business request: error=%v requests=%d", err, requests.Load())
	}
}

type paypalProviderFixture struct {
	p        *PayPal
	mu       sync.Mutex
	orders   map[string]map[string]any
	created  atomic.Int64
	captured atomic.Int64
	mutate   func(map[string]any)
	already  bool
}

func newPayPalProviderFixture(t *testing.T) *paypalProviderFixture {
	t.Helper()
	f := &paypalProviderFixture{orders: make(map[string]map[string]any)}
	f.p = testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, 1)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token-1" {
			t.Error("missing OAuth authorization")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == "/v2/checkout/orders" {
			var input paypalOrderResponse
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.PurchaseUnits) != 1 {
				t.Error("invalid order request")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			unit := input.PurchaseUnits[0]
			if r.Header.Get("PayPal-Request-Id") != unit.ReferenceID {
				t.Error("order creation did not use the local order number as idempotency key")
			}
			providerID := "PP-" + unit.ReferenceID
			f.created.Add(1)
			response := map[string]any{
				"id": providerID, "intent": "CAPTURE", "status": "CREATED",
				"purchase_units": []any{map[string]any{"reference_id": unit.ReferenceID, "amount": unit.Amount}},
				"links":          []any{map[string]string{"rel": "approve", "href": "https://www.sandbox.paypal.com/checkoutnow?token=" + providerID}},
			}
			f.orders[providerID] = response
			writePayPalJSON(t, w, response)
			return
		}
		providerID := strings.TrimPrefix(r.URL.Path, "/v2/checkout/orders/")
		providerID = strings.TrimSuffix(providerID, "/capture")
		response, ok := f.orders[providerID]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writePayPalJSON(t, w, map[string]string{"name": "RESOURCE_NOT_FOUND"})
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/capture") {
			f.captured.Add(1)
			unit := response["purchase_units"].([]any)[0].(map[string]any)
			response = map[string]any{
				"id": providerID, "status": "COMPLETED",
				"purchase_units": []any{map[string]any{
					"reference_id": unit["reference_id"],
					"payments": map[string]any{"captures": []any{map[string]any{
						"id": "CAP-" + providerID, "status": "COMPLETED", "amount": unit["amount"],
					}}},
				}},
			}
			if f.mutate != nil {
				f.mutate(response)
			}
			f.orders[providerID] = response
			if f.already {
				w.WriteHeader(http.StatusUnprocessableEntity)
				writePayPalJSON(t, w, map[string]any{"details": []any{map[string]string{"issue": "ORDER_ALREADY_CAPTURED"}}})
				return
			}
		}
		writePayPalJSON(t, w, response)
	})
	return f
}

func insertPayPalOrder(t *testing.T, p *PayPal, number string) {
	t.Helper()
	groupID := int64(1)
	now := time.Now()
	if err := p.store.Orders.Insert(&db.PaymentOrder{
		ID: number, OrderNo: number, Amount: 10000, PayMethod: "paypal", Status: db.StatusPending,
		ProductName: "test", GroupID: &groupID, CreatedAt: now, UpdatedAt: now, ExpireAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func requirePayPalOrderStatus(t *testing.T, p *PayPal, number string, status int) *db.PaymentOrder {
	t.Helper()
	order, err := p.store.Orders.FindByOrderNo(number, false)
	if err != nil || order == nil || order.Status != status {
		t.Fatalf("order status: order=%+v error=%v, want %d", order, err, status)
	}
	return order
}

func TestPayPalCreateOrderIdempotency(t *testing.T) {
	f := newPayPalProviderFixture(t)
	insertPayPalOrder(t, f.p, "PAY-ONE")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := f.p.CreateOrder("PAY-ONE", "https://example.com")
			if err != nil || result == nil || result.Data != "https://www.sandbox.paypal.com/checkoutnow?token=PP-PAY-ONE" {
				t.Errorf("CreateOrder: result=%+v error=%v", result, err)
			}
		}()
	}
	wg.Wait()
	order := requirePayPalOrderStatus(t, f.p, "PAY-ONE", db.StatusPending)
	if f.created.Load() != 1 || order.ProviderOrderID == nil || *order.ProviderOrderID != "PP-PAY-ONE" ||
		order.ForeignAmount == nil || *order.ForeignAmount != 1400 {
		t.Fatalf("repeated creation: creates=%d order=%+v", f.created.Load(), order)
	}
}

func TestPayPalRejectsCrossOrderToken(t *testing.T) {
	f := newPayPalProviderFixture(t)
	for _, number := range []string{"PAY-A", "PAY-B"} {
		insertPayPalOrder(t, f.p, number)
		if _, err := f.p.CreateOrder(number, "https://example.com"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := f.p.CaptureOrder("PAY-A", "PP-PAY-B")
	if err != nil || result.Success || f.captured.Load() != 0 {
		t.Fatalf("cross-order callback: result=%+v error=%v captures=%d", result, err, f.captured.Load())
	}
	requirePayPalOrderStatus(t, f.p, "PAY-A", db.StatusPending)
	requirePayPalOrderStatus(t, f.p, "PAY-B", db.StatusPending)
}

func TestPayPalCaptureValidation(t *testing.T) {
	unit := func(response map[string]any) map[string]any {
		return response["purchase_units"].([]any)[0].(map[string]any)
	}
	capture := func(response map[string]any) map[string]any {
		return unit(response)["payments"].(map[string]any)["captures"].([]any)[0].(map[string]any)
	}
	cases := map[string]func(map[string]any){
		"provider ID":        func(r map[string]any) { r["id"] = "PP-OTHER" },
		"reference":          func(r map[string]any) { unit(r)["reference_id"] = "PAY-OTHER" },
		"missing reference":  func(r map[string]any) { delete(unit(r), "reference_id") },
		"order incomplete":   func(r map[string]any) { r["status"] = "APPROVED" },
		"capture pending":    func(r map[string]any) { capture(r)["status"] = "PENDING" },
		"capture denied":     func(r map[string]any) { capture(r)["status"] = "DENIED" },
		"capture ID missing": func(r map[string]any) { delete(capture(r), "id") },
		"amount": func(r map[string]any) {
			capture(r)["amount"] = &paypal.PurchaseUnitAmount{Currency: "USD", Value: "13.99"}
		},
		"currency": func(r map[string]any) {
			capture(r)["amount"] = &paypal.PurchaseUnitAmount{Currency: "CNY", Value: "100.00"}
		},
		"currency missing": func(r map[string]any) { capture(r)["amount"] = &paypal.PurchaseUnitAmount{Value: "14.00"} },
		"fractional cent": func(r map[string]any) {
			capture(r)["amount"] = &paypal.PurchaseUnitAmount{Currency: "USD", Value: "14.001"}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPayPalProviderFixture(t)
			insertPayPalOrder(t, f.p, "PAY-ONE")
			if _, err := f.p.CreateOrder("PAY-ONE", "https://example.com"); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			f.mutate = mutate
			f.mu.Unlock()
			result, err := f.p.CaptureOrder("PAY-ONE", "PP-PAY-ONE")
			if err != nil || result.Success {
				t.Fatalf("invalid capture: result=%+v error=%v", result, err)
			}
			requirePayPalOrderStatus(t, f.p, "PAY-ONE", db.StatusPending)
		})
	}
}

func TestPayPalCaptureCompletedAndReplay(t *testing.T) {
	for _, alreadyCaptured := range []bool{false, true} {
		t.Run(fmt.Sprintf("alreadyCaptured=%v", alreadyCaptured), func(t *testing.T) {
			f := newPayPalProviderFixture(t)
			f.mu.Lock()
			f.already = alreadyCaptured
			f.mu.Unlock()
			insertPayPalOrder(t, f.p, "PAY-ONE")
			if _, err := f.p.CreateOrder("PAY-ONE", "https://example.com"); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				result, err := f.p.CaptureOrder("PAY-ONE", "PP-PAY-ONE")
				if err != nil || !result.Success || result.CaptureID == nil || *result.CaptureID != "CAP-PP-PAY-ONE" {
					t.Fatalf("capture: result=%+v error=%v", result, err)
				}
			}
			order := requirePayPalOrderStatus(t, f.p, "PAY-ONE", db.StatusPaid)
			if f.captured.Load() != 1 || order.ThirdPartyTradeNo == nil || *order.ThirdPartyTradeNo != "CAP-PP-PAY-ONE" {
				t.Fatalf("capture replay dispatched another capture: count=%d order=%+v", f.captured.Load(), order)
			}
		})
	}
}

func TestPayPalRefundOutcomes(t *testing.T) {
	for _, status := range []string{"COMPLETED", "PENDING", "FAILED", "UNKNOWN"} {
		t.Run(status, func(t *testing.T) {
			p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth2/token" {
					writePayPalToken(t, w, r, 1)
					return
				}
				if r.Header.Get("PayPal-Request-Id") != "refund-operation" {
					t.Error("refund did not preserve the durable request ID")
				}
				writePayPalJSON(t, w, paypal.RefundResponse{
					ID: "REFUND-ONE", Status: status, Amount: &paypal.PurchaseUnitAmount{Currency: "USD", Value: "7.00"},
				})
			})
			id, actual, err := p.RefundWithStatus("CAP-ONE", 700, "USD", "reason", "refund-operation")
			if id != "REFUND-ONE" || actual != status || (status != "UNKNOWN" && err != nil) || (status == "UNKNOWN" && err == nil) {
				t.Fatalf("refund: id=%s status=%s error=%v", id, actual, err)
			}
		})
	}
}

func TestPayPalRefundTransportAndRejection(t *testing.T) {
	for _, code := range []int{http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth2/token" {
					writePayPalToken(t, w, r, 1)
					return
				}
				w.WriteHeader(code)
				writePayPalJSON(t, w, map[string]string{"name": "REFUND_ERROR"})
			})
			_, status, err := p.RefundWithStatus("CAP-ONE", 700, "USD", "", "operation")
			want := "UNKNOWN"
			if code == http.StatusUnprocessableEntity {
				want = "FAILED"
			}
			if status != want || err == nil {
				t.Fatalf("refund status=%s error=%v, want %s/error", status, err, want)
			}
		})
	}
	t.Run("timeout", func(t *testing.T) {
		p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/oauth2/token" {
				writePayPalToken(t, w, r, 1)
				return
			}
			time.Sleep(40 * time.Millisecond)
		})
		p.cli.Client.Timeout = 10 * time.Millisecond
		_, status, err := p.RefundWithStatus("CAP-ONE", 700, "USD", "", "operation")
		if status != "UNKNOWN" || err == nil {
			t.Fatalf("transport error must leave an unknown outcome: %s/%v", status, err)
		}
	})
}

func TestPayPalLegacyRefundDoesNotCompletePending(t *testing.T) {
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, 1)
			return
		}
		writePayPalJSON(t, w, paypal.RefundResponse{
			ID: "REFUND-ONE", Status: "PENDING", Amount: &paypal.PurchaseUnitAmount{Currency: "USD", Value: money.ToYuanString(700)},
		})
	})
	if result := p.Refund("CAP-ONE", 700, "USD", ""); result != "" {
		t.Fatalf("pending refund incorrectly returned completed ID %s", result)
	}
}

func TestPayPalRefundIdempotentPendingRetry(t *testing.T) {
	var calls atomic.Int64
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, 1)
			return
		}
		if r.Header.Get("PayPal-Request-Id") != "refund-operation" {
			t.Error("pending retry did not retain the same provider idempotency key")
		}
		var input struct {
			Amount *paypal.PurchaseUnitAmount `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !validPayPalAmount(input.Amount, 700) {
			t.Error("pending retry changed the refund amount")
		}
		status := "PENDING"
		if calls.Add(1) > 1 {
			status = "COMPLETED"
		}
		writePayPalJSON(t, w, paypal.RefundResponse{
			ID: "REFUND-ONE", Status: status, Amount: &paypal.PurchaseUnitAmount{Currency: "USD", Value: "7.00"},
		})
	})
	for _, want := range []string{"PENDING", "COMPLETED"} {
		id, status, err := p.RefundWithStatus("CAP-ONE", 700, "USD", "reason", "refund-operation")
		if id != "REFUND-ONE" || status != want || err != nil {
			t.Fatalf("pending retry: id=%s status=%s error=%v, want same refund/%s", id, status, err, want)
		}
	}
}

func TestPayPalCaptureNetworkFailurePreservesPending(t *testing.T) {
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, 1)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		writePayPalJSON(t, w, map[string]string{"name": "INTERNAL_SERVER_ERROR"})
	})
	insertPayPalOrder(t, p, "PAY-ONE")
	if err := p.store.Orders.BindProviderOrder("PAY-ONE", "PP-PAY-ONE", 0.14, 1400, "USD"); err != nil {
		t.Fatal(err)
	}
	result, err := p.CaptureOrder("PAY-ONE", "PP-PAY-ONE")
	if err != nil || result.Success {
		t.Fatalf("network failure: result=%+v error=%v", result, err)
	}
	requirePayPalOrderStatus(t, p, "PAY-ONE", db.StatusPending)
}

func TestPayPalSandboxOnlyAndTimeout(t *testing.T) {
	p := NewPayPal(config.Map{
		"PAYPAL_CLIENT_ID": "test-client", "PAYPAL_CLIENT_SECRET": "test-secret", "PAYPAL_ENVIRONMENT": "sandbox",
	}, nil, nil, nil)
	cli, err := p.client()
	if err != nil || cli.Client.Timeout <= 0 || cli.APIBase != paypal.APIBaseSandBox {
		t.Fatalf("client must use sandbox with timeout: client=%+v error=%v", cli, err)
	}
	p.cfg = config.Map{"PAYPAL_ENVIRONMENT": "live"}
	if _, err := p.client(); err == nil {
		t.Fatal("live PayPal environment was accepted")
	}
}

func TestPayPalQueryRefundDoesNotCreateAnotherRefund(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		currency string
		value    string
		status   string
		code     int
		want     string
	}{
		{name: "completed", id: "REFUND-ONE", currency: "USD", value: "7.00", status: "COMPLETED", want: "COMPLETED"},
		{name: "pending", id: "REFUND-ONE", currency: "USD", value: "7.00", status: "PENDING", want: "PENDING"},
		{name: "failed", id: "REFUND-ONE", currency: "USD", value: "7.00", status: "FAILED", want: "FAILED"},
		{name: "wrong ID", id: "REFUND-OTHER", currency: "USD", value: "7.00", status: "COMPLETED", want: "UNKNOWN"},
		{name: "wrong amount", id: "REFUND-ONE", currency: "USD", value: "6.99", status: "COMPLETED", want: "UNKNOWN"},
		{name: "wrong currency", id: "REFUND-ONE", currency: "CNY", value: "7.00", status: "COMPLETED", want: "UNKNOWN"},
		{name: "missing currency", id: "REFUND-ONE", value: "7.00", status: "COMPLETED", want: "UNKNOWN"},
		{name: "query rejected", code: http.StatusNotFound, want: "UNKNOWN"},
		{name: "query unavailable", code: http.StatusInternalServerError, want: "UNKNOWN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var queries, posts atomic.Int64
			p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth2/token" {
					writePayPalToken(t, w, r, 1)
					return
				}
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
				if r.Method != http.MethodGet || r.URL.Path != "/v2/payments/refunds/REFUND-ONE" {
					t.Error("query issued a request other than GET to the persisted refund ID")
				}
				queries.Add(1)
				if tc.code != 0 {
					w.WriteHeader(tc.code)
					writePayPalJSON(t, w, map[string]string{"name": "QUERY_ERROR"})
					return
				}
				writePayPalJSON(t, w, paypal.RefundResponse{
					ID: tc.id, Status: tc.status, Amount: &paypal.PurchaseUnitAmount{Currency: tc.currency, Value: tc.value},
				})
			})
			status, err := p.QueryRefund("REFUND-ONE", 700, "USD")
			if status != tc.want || queries.Load() != 1 || posts.Load() != 0 || (tc.want == "UNKNOWN" && err == nil) {
				t.Fatalf("query: status=%s error=%v GET=%d POST=%d, want %s/1/0", status, err, queries.Load(), posts.Load(), tc.want)
			}
		})
	}
}
