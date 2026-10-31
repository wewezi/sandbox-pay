package gateways

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func TestCashierPayPalQuoteFreezesLocalUSDWithoutSwitchingChannel(t *testing.T) {
	for _, method := range []string{"native", "alipay", "paypal"} {
		t.Run(method, func(t *testing.T) {
			store, err := db.Open(filepath.Join(t.TempDir(), "quote.db"), "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			order := &db.PaymentOrder{
				ID: "cashier-quote", OrderNo: "PAY_CASHIER_QUOTE", Amount: 12345,
				ProductName: "Local quote regression", PayMethod: method, Status: db.StatusPending,
				ExpireAt: time.Now().Add(time.Hour),
			}
			if err := store.Orders.Insert(order); err != nil {
				t.Fatal(err)
			}
			// Omitting rates forces a fixed local fallback despite a live provider
			// in cfg. A nil HTTP client would panic if any fetch were attempted.
			cfg := config.Map{"EXCHANGE_RATE_PROVIDER": "open-er-api", "FALLBACK_CNY_TO_USD": "0.14", "EXCHANGE_RATE_MARGIN": "1.05"}
			probe := &cashierAlipayProbe{}
			cashier := NewNativePay(cfg, store, nil, probe)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil {
				t.Fatal(err)
			}
			quote := info.PayPalQuote
			if quote == nil || !quote.Available || quote.Currency != "USD" || quote.Amount == nil || *quote.Amount != 18.15 ||
				quote.ExchangeRate == nil || math.Abs(*quote.ExchangeRate-0.147) > 1e-12 {
				t.Fatalf("cashier quote does not match rounded USD amount: %+v", quote)
			}
			if info.Amount != 123.45 || info.PayMethod != method || info.ForeignAmount != nil || info.ForeignCurrency != "" || info.ExchangeRate != nil {
				t.Fatalf("quote changed base CNY order or impersonated a payment: %+v", info)
			}
			stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
			if err != nil || stored == nil || stored.Amount != order.Amount || stored.PayMethod != method || stored.Status != db.StatusPending ||
				stored.ProviderOrderID != nil || stored.ThirdPartyTradeNo != nil || stored.ForeignAmount == nil || *stored.ForeignAmount != 1815 ||
				stored.ForeignCurrency == nil || *stored.ForeignCurrency != "USD" || stored.ExchangeRate == nil || math.Abs(*stored.ExchangeRate-0.147) > 1e-12 {
				t.Fatalf("local quote changed merchant order or created provider payment: stored=%+v err=%v", stored, err)
			}
			cashierWithNewRate := NewNativePay(config.Map{"FALLBACK_CNY_TO_USD": "0.9", "EXCHANGE_RATE_MARGIN": "1"}, store, nil, probe)
			refreshed, err := cashierWithNewRate.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil || !reflect.DeepEqual(refreshed.PayPalQuote, quote) {
				t.Fatalf("refresh repriced the displayed quote: info=%+v err=%v", refreshed, err)
			}
			if probe.qrCalls != 0 {
				t.Fatal("a quote created an external payment QR")
			}
			encoded, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot map[string]any
			if err := json.Unmarshal(encoded, &snapshot); err != nil {
				t.Fatal(err)
			}
			jsonQuote, ok := snapshot["paypalQuote"].(map[string]any)
			if !ok || jsonQuote["amount"] != 18.15 || jsonQuote["currency"] != "USD" || jsonQuote["available"] != true || snapshot["amount"] != 123.45 {
				t.Fatalf("public quote contract is incorrect: %s", encoded)
			}
		})
	}
}

func TestCashierPayPalQuoteFailureKeepsOrderAndOtherChannels(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "quote-failure.db")
	store, err := db.Open(databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	order := &db.PaymentOrder{
		ID: "quote-failure", OrderNo: "PAY_QUOTE_FAILURE", Amount: 12800,
		ProductName: "Quote storage failure", PayMethod: "native", Status: db.StatusPending,
		ExpireAt: time.Now().Add(time.Hour),
	}
	if err := store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}
	// A test-only trigger rejects the quote write while order reads still work.
	conn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Exec(`CREATE TRIGGER reject_quote BEFORE UPDATE OF "exchangeRate", "foreignAmount", "foreignCurrency" ON "payment_orders" BEGIN SELECT RAISE(ABORT, 'private quote storage error'); END;`); err != nil {
		t.Fatal(err)
	}
	cashier := NewNativePay(config.Map{}, store, nil, nil)
	info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
	if err != nil || info == nil || info.Status != "pending" || info.Amount != 128 || info.PayMethod != "native" {
		t.Fatalf("quote storage failure hid the order: info=%+v err=%v", info, err)
	}
	if info.PayPalQuote == nil || info.PayPalQuote.Available || info.PayPalQuote.Currency != "USD" ||
		info.PayPalQuote.Amount != nil || info.PayPalQuote.ExchangeRate != nil || info.PayPalQuote.Reason != "PayPal 报价暂不可用，请稍后重试" {
		t.Fatalf("quote failure did not return a safe unavailable contract: %+v", info.PayPalQuote)
	}
	if !reflect.DeepEqual(info.AvailableChannels, []string{"native", "alipay", "paypal"}) || info.PaymentInfo == nil {
		t.Fatalf("quote failure removed other payment choices: %+v", info)
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private quote storage error") || !strings.Contains(string(encoded), `"amount":null`) || !strings.Contains(string(encoded), `"exchangeRate":null`) {
		t.Fatalf("unavailable quote must expose null values without internal errors: %s", encoded)
	}
}

func TestCashierPayPalPaidReceiptUsesOriginalUSDWithoutRepricing(t *testing.T) {
	for _, status := range []int{db.StatusPaid, db.StatusRefunding, db.StatusRefunded} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			store, err := db.Open(filepath.Join(t.TempDir(), "paid-usd.db"), "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			currency := "USD"
			rate := 0.14
			foreignCents := int64(1)
			order := &db.PaymentOrder{
				ID: "paid-usd", OrderNo: "PAY_PAID_USD", Amount: 1,
				ProductName: "Legacy payment receipt", PayMethod: "paypal", Status: status,
				ExpireAt: time.Now().Add(-time.Hour), ForeignAmount: &foreignCents,
				ForeignCurrency: &currency, ExchangeRate: &rate,
			}
			if err := store.Orders.Insert(order); err != nil {
				t.Fatal(err)
			}
			cashier := NewNativePay(config.Map{"FALLBACK_CNY_TO_USD": "0.9"}, store, nil, nil)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil || info.Status != "paid" || info.Amount != 0.01 || info.PayPalQuote != nil ||
				info.ForeignAmount == nil || *info.ForeignAmount != 0.01 || info.ForeignCurrency != "USD" || info.ExchangeRate == nil || *info.ExchangeRate != rate {
				t.Fatalf("paid USD receipt was repriced or rejected by new quote rules: info=%+v err=%v", info, err)
			}
			stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
			if err != nil || stored == nil || stored.Amount != 1 || stored.ForeignAmount == nil || *stored.ForeignAmount != 1 ||
				stored.ExchangeRate == nil || *stored.ExchangeRate != rate || stored.Status != status {
				t.Fatalf("paid snapshot was changed: stored=%+v err=%v", stored, err)
			}
		})
	}
}

func TestCashierPaidWithoutPayPalSnapshotOmitsForeignReceipt(t *testing.T) {
	for _, method := range []string{"paypal", "native", "alipay"} {
		t.Run(method, func(t *testing.T) {
			store, err := db.Open(filepath.Join(t.TempDir(), "paid-no-quote.db"), "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			order := &db.PaymentOrder{
				ID: "paid-no-quote", OrderNo: "PAY_PAID_NO_QUOTE", Amount: 12800,
				ProductName: "Paid without USD snapshot", PayMethod: method, Status: db.StatusPaid,
				ExpireAt: time.Now().Add(-time.Hour),
			}
			if method != "paypal" {
				// An unused PayPal quote can remain after selecting another method.
				// It must not be presented as the actual native/Alipay debit.
				currency, foreignCents, rate := "USD", int64(1792), 0.14
				order.ForeignCurrency, order.ForeignAmount, order.ExchangeRate = &currency, &foreignCents, &rate
			}
			if err := store.Orders.Insert(order); err != nil {
				t.Fatal(err)
			}
			cashier := NewNativePay(config.Map{}, store, nil, nil)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil || info.Status != "paid" || info.Amount != 128 || info.ForeignAmount != nil || info.ForeignCurrency != "" || info.ExchangeRate != nil || info.PayPalQuote != nil {
				t.Fatalf("paid order invented a USD receipt: info=%+v err=%v", info, err)
			}
			encoded, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "foreignAmount") || strings.Contains(string(encoded), "foreignCurrency") || strings.Contains(string(encoded), "exchangeRate") || strings.Contains(string(encoded), "paypalQuote") {
				t.Fatalf("paid order exposed nonexistent quote fields: %s", encoded)
			}
		})
	}
}

func TestCashierPayPalQuoteUsesInjectedRates(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "injected-rates.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	order := &db.PaymentOrder{
		ID: "injected-rates", OrderNo: "PAY_INJECTED_RATES", Amount: 12800,
		ProductName: "Injected local rates", PayMethod: "native", Status: db.StatusPending,
		ExpireAt: time.Now().Add(time.Hour),
	}
	if err := store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}
	rates := services.NewExchangeRateService(nil, config.Map{
		"EXCHANGE_RATE_PROVIDER": "fixed", "FALLBACK_CNY_TO_USD": "0.2", "EXCHANGE_RATE_MARGIN": "1",
	})
	cashier := NewNativePay(config.Map{"FALLBACK_CNY_TO_USD": "0.14"}, store, nil, nil, rates)
	info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
	if err != nil || info.PayPalQuote == nil || !info.PayPalQuote.Available || info.PayPalQuote.Amount == nil ||
		*info.PayPalQuote.Amount != 25.6 || info.PayPalQuote.ExchangeRate == nil || *info.PayPalQuote.ExchangeRate != 0.2 {
		t.Fatalf("cashier ignored its injected rates: info=%+v err=%v", info, err)
	}
}

type cashierAlipayProbe struct {
	queries int
	qrCalls int
}

func (p *cashierAlipayProbe) QueryAlipayTrade(string) bool {
	p.queries++
	return false
}

func (p *cashierAlipayProbe) CreateQrPay(string, string) string {
	p.qrCalls++
	return "https://alipay.example/pay/this-order"
}

func TestCashierPaymentQRMatchesOrderAndPendingChannelsRemainAvailable(t *testing.T) {
	for _, method := range []string{"", "native", "bank", "alipay", "paypal"} {
		for _, fixedQR := range []string{"", "data:image/png;base64,AA=="} {
			t.Run(method+"/fixed="+fixedQR, func(t *testing.T) {
				store, err := db.Open(filepath.Join(t.TempDir(), "cashier.db"), "")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				order := &db.PaymentOrder{
					ID: "cashier-channel", OrderNo: "PAY_CASHIER_CHANNEL", Amount: 12800,
					ProductName: "Channel binding regression", PayMethod: method,
					Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour),
				}
				if err := store.Orders.Insert(order); err != nil {
					t.Fatal(err)
				}
				probe := &cashierAlipayProbe{}
				cashier := NewNativePay(config.Map{"NATIVE_PAY_QR_URL": fixedQR}, store, nil, probe)
				info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
				if err != nil {
					t.Fatal(err)
				}
				if info.Status != "pending" || info.PayMethod != method || info.PaymentInfo == nil {
					t.Fatalf("pending snapshot lost order channel: %+v", info)
				}
				switch method {
				case "alipay":
					if probe.queries != 1 || probe.qrCalls != 0 || info.PaymentInfo.QRCodeChannel != "" || info.PaymentInfo.QRCodeURL != "" {
						t.Fatalf("cashier read created an Alipay payment: probe=%+v info=%+v", probe, info.PaymentInfo)
					}
				case "paypal":
					if probe.queries != 0 || probe.qrCalls != 0 || info.PaymentInfo.QRCodeURL != "" || info.PaymentInfo.QRCodeChannel != "" {
						t.Fatalf("PayPal opened another channel: probe=%+v info=%+v", probe, info.PaymentInfo)
					}
				default:
					if probe.queries != 0 || probe.qrCalls != 0 || info.PaymentInfo.QRCodeURL != fixedQR {
						t.Fatalf("native opened Alipay: probe=%+v info=%+v", probe, info.PaymentInfo)
					}
					if (fixedQR == "" && info.PaymentInfo.QRCodeChannel != "") || (fixedQR != "" && info.PaymentInfo.QRCodeChannel != "native") {
						t.Fatalf("native QR has wrong channel: %+v", info.PaymentInfo)
					}
				}
				channels := []string{"native", "alipay", "paypal"}
				if info.ChannelLockReason != "" {
					t.Fatal("pending order must allow selecting every channel")
				}
				if !reflect.DeepEqual(info.AvailableChannels, channels) {
					t.Fatalf("available channels=%v, want %v", info.AvailableChannels, channels)
				}
				stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
				if err != nil || stored == nil || stored.PayMethod != method || stored.Status != db.StatusPending {
					t.Fatalf("cashier changed the selected channel: stored=%+v err=%v", stored, err)
				}
			})
		}
	}
}

func TestCashierPendingPayPalSnapshotsKeepAllChannelsAvailable(t *testing.T) {
	providerID, tradeNo := "PAYPAL_PROVIDER_42", "PAYPAL_CAPTURE_42"
	foreignAmount := int64(1792)
	for _, test := range []struct {
		name          string
		provider      *string
		foreignAmount *int64
		tradeNo       *string
	}{
		{name: "preselected only"},
		{name: "provider bound", provider: &providerID, foreignAmount: &foreignAmount},
		{name: "unknown provider result", foreignAmount: &foreignAmount},
		{name: "trade snapshot", tradeNo: &tradeNo},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := db.Open(filepath.Join(t.TempDir(), "paypal-cashier.db"), "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			order := &db.PaymentOrder{
				ID: "cashier-paypal", OrderNo: "PAY_CASHIER_PAYPAL", Amount: 12800,
				ProductName: "PayPal selection regression", PayMethod: "paypal", Status: db.StatusPending,
				ExpireAt: time.Now().Add(time.Hour), ProviderOrderID: test.provider,
				ForeignAmount: test.foreignAmount, ThirdPartyTradeNo: test.tradeNo,
			}
			if err := store.Orders.Insert(order); err != nil {
				t.Fatal(err)
			}
			probe := &cashierAlipayProbe{}
			cashier := NewNativePay(config.Map{"NATIVE_PAY_QR_URL": "data:image/png;base64,AA=="}, store, nil, probe)
			info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
			if err != nil {
				t.Fatal(err)
			}
			wantChannels := []string{"native", "alipay", "paypal"}
			if !reflect.DeepEqual(info.AvailableChannels, wantChannels) || info.ChannelLockReason != "" {
				t.Fatalf("provider snapshot blocked pending channel selection: %+v", info)
			}
			if info.PaymentInfo == nil || info.PaymentInfo.QRCodeURL != "" || info.PaymentInfo.QRCodeChannel != "" || probe.queries != 0 || probe.qrCalls != 0 {
				t.Fatalf("PayPal cashier created another channel payment: info=%+v probe=%+v", info, probe)
			}
		})
	}
}

func TestCashierReturnDestinationInEveryState(t *testing.T) {
	configured := "https://merchant.example/checkout/complete?cart=42#receipt"
	unsafe := "javascript:alert(1)"
	for _, status := range []int{db.StatusPending, db.StatusPaid, db.StatusRefunding, db.StatusRefunded, db.StatusExpired, db.StatusFailed} {
		for _, destination := range []struct {
			name  string
			value *string
			want  string
		}{
			{name: "configured", value: &configured, want: configured},
			{name: "missing", want: "/sandbox"},
			{name: "legacy unsafe", value: &unsafe, want: "/sandbox"},
		} {
			t.Run(destination.name+"/status="+strconv.Itoa(status), func(t *testing.T) {
				store, err := db.Open(filepath.Join(t.TempDir(), "return.db"), "")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				order := &db.PaymentOrder{
					ID: "cashier-return", OrderNo: "PAY_CASHIER_RETURN", Amount: 150,
					ProductName: "Return regression", PayMethod: "native", Status: status,
					ExpireAt: time.Now().Add(time.Hour), ReturnURL: destination.value,
				}
				if err := store.Orders.Insert(order); err != nil {
					t.Fatal(err)
				}
				cashier := NewNativePay(config.Map{}, store, nil, nil)
				info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
				if err != nil {
					t.Fatal(err)
				}
				if info.ReturnURL != destination.want {
					t.Fatalf("return URL=%q, want %q", info.ReturnURL, destination.want)
				}
				raw, err := json.Marshal(info)
				if err != nil {
					t.Fatal(err)
				}
				var snapshot map[string]any
				if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot["returnUrl"] != destination.want {
					t.Fatalf("return destination missing from JSON: %s err=%v", raw, err)
				}
				if status != db.StatusPending && (len(info.AvailableChannels) > 0 || info.ChannelLockReason != "" || info.PaymentInfo != nil) {
					t.Fatalf("terminal order offers payment actions: %+v", info)
				}
			})
		}
	}
}

func TestCashierKeepsCompletedPaymentAfterRefund(t *testing.T) {
	states := []struct {
		name     string
		status   int
		refunded int64
	}{
		{name: "paid", status: db.StatusPaid},
		{name: "refunding", status: db.StatusRefunding, refunded: 50},
		{name: "refunded", status: db.StatusRefunded, refunded: 150},
	}
	deadlines := []struct {
		name   string
		offset time.Duration
	}{
		{name: "before payment deadline", offset: time.Hour},
		{name: "after payment deadline", offset: -time.Hour},
	}
	for _, state := range states {
		for _, deadline := range deadlines {
			t.Run(state.name+"/"+deadline.name, func(t *testing.T) {
				store, err := db.Open(filepath.Join(t.TempDir(), "cashier.db"), "")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				paidAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)
				tradeNo := "SP_WALLET_ORIGINAL"
				order := &db.PaymentOrder{
					ID: "cashier-payment", OrderNo: "PAY_CASHIER_PAYMENT", Amount: 150,
					ProductName: "Cashier refund regression", PayMethod: "native",
					Status: state.status, ExpireAt: time.Now().Add(deadline.offset),
					PayAt: &paidAt, ThirdPartyTradeNo: &tradeNo, RefundedAmount: state.refunded,
				}
				if err := store.Orders.Insert(order); err != nil {
					t.Fatal(err)
				}
				// A fixed local QR avoids every third-party call even if the
				// regression incorrectly falls through to pending payment.
				cashier := NewNativePay(config.Map{"NATIVE_PAY_QR_URL": "data:image/png;base64,AA=="}, store, nil, nil)
				info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
				if err != nil {
					t.Fatal(err)
				}
				if info.Status != "paid" {
					t.Fatalf("completed payment became %q in refund state %d", info.Status, state.status)
				}
				if info.OrderNo != order.OrderNo || info.Amount != 1.5 || info.ProductName != order.ProductName || info.PayMethod != order.PayMethod {
					t.Fatalf("original order details changed: %+v", info)
				}
				if info.ThirdPartyTradeNo != tradeNo || info.PayAt == nil || *info.PayAt != services.JsTime(paidAt) {
					t.Fatalf("original payment receipt changed: %+v", info)
				}
				if info.ExpireAt != nil || info.PaymentInfo != nil {
					t.Fatalf("completed payment still offered pending payment information: %+v", info)
				}
				stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
				if err != nil {
					t.Fatal(err)
				}
				if stored == nil || stored.Status != state.status || stored.RefundedAmount != state.refunded {
					t.Fatalf("cashier query changed the refund state: %+v", stored)
				}
			})
		}
	}
}
