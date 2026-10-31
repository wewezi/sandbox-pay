package gateways

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/smartwalle/alipay/v3"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

type alipayQueryTransport func(*http.Request) (*http.Response, error)

func (f alipayQueryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCashierAlipayQueryTimeoutKeepsPendingOrderAndPaymentMethods(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "query-timeout.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	order := &db.PaymentOrder{
		ID: "query-timeout", OrderNo: "PAY_QUERY_TIMEOUT", Amount: 10000,
		ProductName: "Alipay timeout regression", PayMethod: "alipay", Status: db.StatusPending,
		ExpireAt: time.Now().Add(time.Hour),
	}
	if err := store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	requests := 0
	cancelled := false
	sdk, err := alipay.New("local-query-test", privatePEM, false,
		alipay.WithSandboxGateway("https://alipay.test.invalid/gateway.do"),
		alipay.WithHTTPClient(&http.Client{Transport: alipayQueryTransport(func(r *http.Request) (*http.Response, error) {
			requests++
			if err := r.ParseForm(); err != nil || r.Form.Get("method") != "alipay.trade.query" {
				t.Error("cashier sent a request other than an Alipay trade query")
				return nil, errors.New("unexpected Alipay request")
			}
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 3*time.Second || time.Until(deadline) < 2*time.Second {
				t.Error("Alipay trade query did not carry its 3 second deadline to the HTTP request")
				return nil, errors.New("query lacks its bounded deadline")
			}
			// Exercise cancellation through the real SDK HTTP path. The transport
			// returns no provider response and must never settle the order.
			select {
			case <-r.Context().Done():
				cancelled = errors.Is(r.Context().Err(), context.DeadlineExceeded)
				return nil, r.Context().Err()
			case <-time.After(4 * time.Second):
				t.Error("stalled Alipay query was not cancelled")
				return nil, errors.New("Alipay query cancellation failed")
			}
		})}),
	)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Map{
		"EXCHANGE_RATE_PROVIDER": "fixed", "FALLBACK_CNY_TO_USD": "0.14", "EXCHANGE_RATE_MARGIN": "1",
	}
	a := NewAlipay(cfg, store, nil)
	a.cli = sdk
	cashier := NewNativePay(cfg, store, nil, a)
	started := time.Now()
	info, err := cashier.GetCashierInfo(order.OrderNo, "http://localhost")
	if err != nil || info == nil {
		t.Fatalf("Alipay query timeout hid the cashier order: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second || !cancelled || requests != 1 {
		t.Fatalf("query did not release the cashier after its deadline: elapsed=%s cancelled=%v requests=%d", elapsed, cancelled, requests)
	}
	if info.Status != "pending" || info.Amount != 100 || info.PayMethod != "alipay" ||
		!reflect.DeepEqual(info.AvailableChannels, []string{"native", "alipay", "paypal"}) ||
		info.PayPalQuote == nil || !info.PayPalQuote.Available || info.PayPalQuote.Amount == nil || *info.PayPalQuote.Amount != 14 {
		t.Fatalf("query timeout lost the pending order, payment methods or frozen quote: %+v", info)
	}
	stored, err := store.Orders.FindByOrderNo(order.OrderNo, false)
	if err != nil || stored == nil || stored.Status != db.StatusPending || stored.PayMethod != "alipay" ||
		stored.Amount != 10000 || stored.ThirdPartyTradeNo != nil || stored.ProviderOrderID != nil || stored.PayAt != nil {
		t.Fatalf("query timeout settled or changed the order: stored=%+v err=%v", stored, err)
	}
}
