package gateways

import (
	"github.com/plutov/paypal"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

func TestPayPalMinimumRejectsBeforeAnyGatewayRequest(t *testing.T) {
	for _, legacyBound := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "legacy bound clamp"}[legacyBound], func(t *testing.T) {
			var calls atomic.Int64
			p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			})
			insertPayPalOrder(t, p, "PAY-TINY")
			patch := map[string]any{"amount": int64(1)}
			if legacyBound {
				patch["providerOrderId"], patch["foreignAmount"], patch["foreignCurrency"], patch["exchangeRate"] = "PP-TINY", int64(1), "USD", 0.14
			}
			if err := p.store.Orders.Update("PAY-TINY", patch); err != nil {
				t.Fatal(err)
			}
			if err := p.PrepareCreateOrder("PAY-TINY"); httperr.As(err) == nil || httperr.As(err).Status != 400 || !strings.Contains(httperr.As(err).Message, "不足 USD 0.01") {
				t.Fatalf("tiny quote did not fail before OAuth: %s", GatewayErrorSummary(err, "paypal"))
			}
			if _, err := p.CreateOrder("PAY-TINY", "https://merchant.example"); err == nil || !ChannelCreationRejected(err) {
				t.Fatal("tiny amount submitted a provider creation")
			}
			if legacyBound {
				result, err := p.CaptureOrder("PAY-TINY", "PP-TINY")
				if err != nil || result.Success {
					t.Fatal("legacy minimum snapshot was captured")
				}
			}
			if calls.Load() != 0 {
				t.Fatalf("minimum guard contacted a gateway: %d requests", calls.Load())
			}
			order := requirePayPalOrderStatus(t, p, "PAY-TINY", db.StatusPending)
			if order.Amount != 1 || legacyBound && (order.ForeignAmount == nil || *order.ForeignAmount != 1) {
				t.Fatalf("rejection altered historical amounts: %+v", order)
			}
			if err := p.payments.SwitchPaymentMethod("PAY-TINY", "native"); err != nil {
				t.Fatal("minimum rejection prevented wallet selection")
			}
		})
	}
}

func TestPayPalCreateUsesPreviouslyDisplayedQuote(t *testing.T) {
	f := newPayPalProviderFixture(t)
	insertPayPalOrder(t, f.p, "PAY-QUOTED")
	cashier := NewNativePay(f.p.cfg, f.p.store, f.p.payments, nil, f.p.rates)
	info, err := cashier.GetCashierInfo("PAY-QUOTED", "https://merchant.example")
	if err != nil || info.PayPalQuote == nil || info.PayPalQuote.Amount == nil || *info.PayPalQuote.Amount != 14 {
		t.Fatalf("quote not USD14 before create: info=%+v err=%v", info, err)
	}
	if _, err := f.p.CreateOrder("PAY-QUOTED", "https://merchant.example"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	unit := f.orders["PP-PAY-QUOTED"]["purchase_units"].([]any)[0].(map[string]any)
	f.mu.Unlock()
	order := requirePayPalOrderStatus(t, f.p, "PAY-QUOTED", db.StatusPending)
	amount, ok := unit["amount"].(*paypal.PurchaseUnitAmount)
	if order.ForeignAmount == nil || *order.ForeignAmount != 1400 || unit["reference_id"] != "PAY-QUOTED" || !ok || amount.Value != "14.00" || amount.Currency != "USD" {
		t.Fatalf("quoted source amount differs from request: %+v unit=%+v", order, unit)
	}
	if f.created.Load() != 1 {
		t.Fatalf("quote created an external payment before selection: %d", f.created.Load())
	}
}
