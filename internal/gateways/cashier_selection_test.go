package gateways

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

func TestCashierPayPalSelectionRoundTripKeepsProviderAndDoesNotCaptureOldCallback(t *testing.T) {
	f := newPayPalProviderFixture(t)
	insertPayPalOrder(t, f.p, "PAY-SELECT")
	if _, err := f.p.CreateOrder("PAY-SELECT", "https://merchant.example"); err != nil {
		t.Fatal(err)
	}
	for _, selection := range []string{"native", "alipay", "native"} {
		if err := f.p.payments.SwitchPaymentMethod("PAY-SELECT", selection); err != nil {
			t.Fatal(err)
		}
		order := requirePayPalOrderStatus(t, f.p, "PAY-SELECT", db.StatusPending)
		if order.ProviderOrderID == nil || *order.ProviderOrderID != "PP-PAY-SELECT" || order.ForeignAmount == nil || *order.ForeignAmount != 1400 || order.Amount != 10000 {
			t.Fatalf("method selection lost the merchant/provider link: %+v", order)
		}
		result, err := f.p.CaptureOrder("PAY-SELECT", "PP-PAY-SELECT")
		if err != nil || result.Success || f.captured.Load() != 0 {
			t.Fatalf("old callback captured after changing the method: result=%+v err=%v captures=%d", result, err, f.captured.Load())
		}
	}
	if err := f.p.payments.SwitchPaymentMethod("PAY-SELECT", "paypal"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.CreateOrder("PAY-SELECT", "https://merchant.example"); err != nil {
		t.Fatal(err)
	}
	if f.created.Load() != 1 {
		t.Fatalf("method round trip created another provider order: %d", f.created.Load())
	}
}

type cashierCaptureTransport func(*http.Request) (*http.Response, error)

func (f cashierCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCashierPayPalCaptureAndMethodSwitchShareOrderLock(t *testing.T) {
	f := newPayPalProviderFixture(t)
	insertPayPalOrder(t, f.p, "PAY-CONCURRENT")
	if _, err := f.p.CreateOrder("PAY-CONCURRENT", "https://merchant.example"); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	base := f.p.cli.Client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	f.p.cli.Client.Transport = cashierCaptureTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/capture") {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return base.RoundTrip(r)
	})
	captured := make(chan error, 1)
	go func() {
		result, err := f.p.CaptureOrder("PAY-CONCURRENT", "PP-PAY-CONCURRENT")
		if err == nil && !result.Success {
			err = httperr.Conflict("capture failed")
		}
		captured <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("capture did not start")
	}
	switched := make(chan error, 1)
	go func() {
		unlock := f.p.payments.LockCashierChannel("PAY-CONCURRENT")
		defer unlock()
		switched <- f.p.payments.SwitchPaymentMethod("PAY-CONCURRENT", "native")
	}()
	select {
	case err := <-switched:
		t.Fatalf("selection changed during provider capture: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-captured; err != nil {
		t.Fatal(err)
	}
	if err := <-switched; httperr.As(err) == nil || httperr.As(err).Status != 409 {
		t.Fatalf("completed payment allowed method switch: %v", err)
	}
	order := requirePayPalOrderStatus(t, f.p, "PAY-CONCURRENT", db.StatusPaid)
	if order.PayMethod != "paypal" || f.captured.Load() != 1 {
		t.Fatalf("wrong settlement after concurrent selection: %+v captures=%d", order, f.captured.Load())
	}
}
