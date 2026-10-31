package gateways

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/services"
)

func TestCashierPayPalCreationFailureRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		status   int
		rejected bool
		bound    bool
	}{
		{name: "definite rejection", status: http.StatusUnprocessableEntity, rejected: true},
		{name: "unknown server failure", status: http.StatusInternalServerError},
		{name: "invalid successful response", status: http.StatusOK},
		{name: "bound order without approval link", status: http.StatusOK, bound: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth2/token" {
					writePayPalToken(t, w, r, 1)
					return
				}
				w.WriteHeader(scenario.status)
				if scenario.bound {
					writePayPalJSON(t, w, map[string]any{
						"id": "PP-PAY-RECOVERY", "intent": "CAPTURE", "status": "CREATED",
						"purchase_units": []any{map[string]any{"reference_id": "PAY-RECOVERY", "amount": map[string]string{"currency_code": "USD", "value": "14.00"}}},
					})
					return
				}
				writePayPalJSON(t, w, map[string]string{"name": "CREATE_FAILED"})
			})
			insertPayPalOrder(t, p, "PAY-RECOVERY")
			if err := p.store.Orders.Update("PAY-RECOVERY", map[string]any{"payMethod": "native"}); err != nil {
				t.Fatal(err)
			}
			if err := p.PrepareCreateOrder(); err != nil {
				t.Fatal(err)
			}
			original := requirePayPalOrderStatus(t, p, "PAY-RECOVERY", db.StatusPending)
			if err := p.payments.SwitchPaymentMethod("PAY-RECOVERY", "paypal"); err != nil {
				t.Fatal(err)
			}
			_, err := p.CreateOrder("PAY-RECOVERY", "https://merchant.example")
			if err == nil || ChannelCreationRejected(err) != scenario.rejected {
				t.Fatalf("wrong creation certainty: err=%v rejected=%v", err, ChannelCreationRejected(err))
			}
			if ChannelCreationRejected(err) {
				if err := p.payments.RestoreCashierChannel(original, "paypal"); err != nil {
					t.Fatal(err)
				}
			}
			order := requirePayPalOrderStatus(t, p, "PAY-RECOVERY", db.StatusPending)
			wantMethod := "paypal"
			if scenario.rejected {
				wantMethod = "native"
			}
			if order.PayMethod != wantMethod || (order.ProviderOrderID != nil) != scenario.bound {
				t.Fatalf("incorrect channel recovery: order=%+v", order)
			}
			if scenario.rejected && (order.ProviderOrderID != nil || order.ThirdPartyTradeNo != nil || order.PayMethod != "native") {
				t.Fatalf("failed creation changed the merchant order or bound a payment: %+v", order)
			}
			if !scenario.rejected && order.ForeignAmount == nil {
				t.Fatal("unknown creation lost the original retry amount")
			}
		})
	}
}

func TestCashierPayPalUnknownCreationRetriesOriginalAmount(t *testing.T) {
	var attempts atomic.Int64
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			writePayPalToken(t, w, r, 1)
			return
		}
		var request paypalOrderResponse
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.PurchaseUnits) != 1 {
			t.Error("invalid request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		unit := request.PurchaseUnits[0]
		if r.Header.Get("PayPal-Request-Id") != "PAY-RETRY" || unit.Amount == nil || unit.Amount.Value != "14.00" || unit.Amount.Currency != "USD" {
			t.Errorf("retry changed idempotency key or amount: key=%q unit=%+v", r.Header.Get("PayPal-Request-Id"), unit)
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			writePayPalJSON(t, w, map[string]string{"name": "INTERNAL_SERVER_ERROR"})
			return
		}
		writePayPalJSON(t, w, map[string]any{
			"id": "PP-PAY-RETRY", "intent": "CAPTURE", "status": "CREATED",
			"purchase_units": []any{map[string]any{"reference_id": unit.ReferenceID, "amount": unit.Amount}},
			"links":          []any{map[string]string{"rel": "approve", "href": "https://www.sandbox.paypal.com/checkoutnow?token=PP-PAY-RETRY"}},
		})
	})
	insertPayPalOrder(t, p, "PAY-RETRY")
	original := requirePayPalOrderStatus(t, p, "PAY-RETRY", db.StatusPending)
	if _, err := p.CreateOrder("PAY-RETRY", "https://merchant.example"); err == nil || ChannelCreationRejected(err) {
		t.Fatalf("initial unknown creation unexpectedly safe: %v", err)
	}
	p.rates = services.NewExchangeRateService(httpclient.New(), config.Map{
		"EXCHANGE_RATE_PROVIDER": "fixed", "FALLBACK_CNY_TO_USD": "0.30", "EXCHANGE_RATE_MARGIN": "1",
	})
	result, err := p.CreateOrder("PAY-RETRY", "https://merchant.example")
	if err != nil || result == nil || attempts.Load() != 2 {
		t.Fatalf("retry result=%+v err=%v attempts=%d", result, err, attempts.Load())
	}
	order := requirePayPalOrderStatus(t, p, "PAY-RETRY", db.StatusPending)
	if order.ForeignAmount == nil || *order.ForeignAmount != 1400 || order.ExchangeRate == nil || *order.ExchangeRate != 0.14 {
		t.Fatalf("retry failed to preserve amount/rate: %+v", order)
	}
	if err := p.payments.RestoreCashierChannel(original, "paypal"); err != nil {
		t.Fatal(err)
	}
	if order := requirePayPalOrderStatus(t, p, "PAY-RETRY", db.StatusPending); order.PayMethod != "paypal" {
		t.Fatalf("bound provider order was restored to wallet: %+v", order)
	}
}
