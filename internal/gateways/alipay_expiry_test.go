package gateways

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartwalle/alipay/v3"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

type alipayExpiryTransport func(*http.Request) (*http.Response, error)

func (f alipayExpiryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func alipayExpiryFixture(t *testing.T, transport http.RoundTripper) (*Alipay, *db.PaymentOrder) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "alipay-expiry.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	deadline := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	order := &db.PaymentOrder{
		ID: "alipay-expiry", OrderNo: "PAY_ALIPAY_EXPIRY", ProductName: "Stored payment window",
		Amount: 1250, PayMethod: "alipay", Status: db.StatusPending, ExpireAt: deadline,
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
	sdk, err := alipay.New("local-expiry-test", privatePEM, false,
		alipay.WithSandboxGateway("https://alipay.test.invalid/gateway.do"),
		alipay.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAlipay(config.Map{}, store, nil)
	a.cli = sdk
	return a, order
}

func TestAlipayPagePayUsesStoredAbsoluteDeadlineOnEveryRetry(t *testing.T) {
	requests := 0
	a, order := alipayExpiryFixture(t, alipayExpiryTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("fixture must not contact an Alipay provider")
	}))
	for retry := 0; retry < 2; retry++ {
		result, err := a.CreatePagePay(order.OrderNo, "https://merchant.example")
		if err != nil || result == nil {
			t.Fatalf("local Alipay link creation failed: %v", err)
		}
		link, err := url.Parse(result.Data)
		if err != nil {
			t.Fatal("Alipay link is invalid")
		}
		var trade struct {
			OrderNo        string `json:"out_trade_no"`
			TotalAmount    string `json:"total_amount"`
			TimeExpire     string `json:"time_expire"`
			TimeoutExpress string `json:"timeout_express"`
		}
		if err := json.Unmarshal([]byte(link.Query().Get("biz_content")), &trade); err != nil {
			t.Fatal("Alipay link has no valid trade payload")
		}
		// The provider's unzoned timestamp is UTC+8 even when the application's
		// stored time is UTC. A retry must reuse that exact absolute deadline.
		providerDeadline, err := time.ParseInLocation("2006-01-02 15:04:05", trade.TimeExpire, time.FixedZone("UTC+8", 8*60*60))
		if err != nil || !providerDeadline.Equal(order.ExpireAt) || trade.TimeoutExpress != "" ||
			trade.OrderNo != order.OrderNo || trade.TotalAmount != "12.50" {
			t.Fatalf("Alipay retry changed the stored deadline or amount: deadline=%s orderDeadline=%s", trade.TimeExpire, order.ExpireAt)
		}
	}
	if requests != 0 {
		t.Fatal("building an Alipay link contacted the provider")
	}
}

func TestAlipayQRRequestUsesSameStoredDeadline(t *testing.T) {
	requests := 0
	var payload struct {
		TimeExpire string `json:"time_expire"`
		OrderNo    string `json:"out_trade_no"`
	}
	a, order := alipayExpiryFixture(t, alipayExpiryTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if err := r.ParseForm(); err != nil || r.Form.Get("method") != "alipay.trade.precreate" {
			t.Error("unexpected Alipay QR request")
			return nil, errors.New("unexpected local fixture request")
		}
		if err := json.Unmarshal([]byte(r.Form.Get("biz_content")), &payload); err != nil {
			t.Error("Alipay QR request has no valid trade payload")
		}
		return nil, errors.New("stop the local fixture before any provider payment")
	}))
	if qr := a.CreateQrPay(order.OrderNo, "https://merchant.example"); qr != "" {
		t.Fatal("intercepted provider request returned a payment QR")
	}
	providerDeadline, err := time.ParseInLocation("2006-01-02 15:04:05", payload.TimeExpire, time.FixedZone("UTC+8", 8*60*60))
	if requests != 1 || err != nil || !providerDeadline.Equal(order.ExpireAt) || payload.OrderNo != order.OrderNo {
		t.Fatal("QR creation did not send the order's existing absolute deadline")
	}
}

func TestAlipayExpiredOrderDoesNotInitializeProviderOrReturnPaymentLink(t *testing.T) {
	a, order := alipayExpiryFixture(t, alipayExpiryTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("expired order contacted Alipay")
		return nil, errors.New("expired order must remain local")
	}))
	if err := a.store.Orders.Update(order.OrderNo, map[string]any{"expireAt": time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	// Empty credentials and no initialized SDK distinguish expiry validation
	// from a provider/configuration failure.
	a.cli = nil
	result, err := a.CreatePagePay(order.OrderNo, "https://merchant.example")
	if result != nil || httperr.As(err) == nil || httperr.As(err).Status != http.StatusConflict || a.cli != nil {
		t.Fatal("expired order was not rejected before Alipay initialization")
	}
	if qr := a.CreateQrPay(order.OrderNo, "https://merchant.example"); qr != "" || a.cli != nil {
		t.Fatal("expired order returned a QR or initialized Alipay")
	}
}
