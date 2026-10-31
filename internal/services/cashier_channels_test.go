package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

func TestCashierRestoreRequiresMatchingPendingUnboundOrder(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		mutate  func(*db.PaymentOrder)
		restore bool
	}{
		{name: "unbound pending rejection", restore: true},
		{name: "payment completed", mutate: func(o *db.PaymentOrder) { o.Status = db.StatusPaid }},
		{name: "other current channel", mutate: func(o *db.PaymentOrder) { o.PayMethod = "alipay" }},
		{name: "provider bound", mutate: func(o *db.PaymentOrder) { o.ProviderOrderID = strPtr("PP-BOUND") }},
		{name: "trade bound", mutate: func(o *db.PaymentOrder) { o.ThirdPartyTradeNo = strPtr("PP-PAID") }},
		{name: "deadline passed", mutate: func(o *db.PaymentOrder) { o.ExpireAt = time.Now().Add(-time.Hour) }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newTestStore(t)
			svc, _ := createPaymentService(store)
			order := &db.PaymentOrder{ID: "restore", OrderNo: "PAY-RESTORE", Amount: 100, PayMethod: "paypal", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour)}
			original := *order
			original.PayMethod = "native"
			if scenario.mutate != nil {
				scenario.mutate(order)
			}
			insertOrder(t, store, order)
			if err := svc.RestoreCashierChannel(&original, "paypal"); err != nil {
				t.Fatal(err)
			}
			stored := reloadOrder(t, store, order.OrderNo)
			wantMethod := order.PayMethod
			if scenario.restore {
				wantMethod = "native"
			}
			if stored.PayMethod != wantMethod || stored.Status != order.Status {
				t.Fatalf("unsafe recovery: %+v", stored)
			}
		})
	}
}

func TestCashierChannelOperationsSerializeSameOrder(t *testing.T) {
	svc, _ := createPaymentService(newTestStore(t))
	var active, maximum atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			unlock := svc.LockCashierChannel("PAY-SAME")
			defer unlock()
			value := active.Add(1)
			for previous := maximum.Load(); value > previous && !maximum.CompareAndSwap(previous, value); previous = maximum.Load() {
			}
			active.Add(-1)
		}()
	}
	close(start)
	wg.Wait()
	if maximum.Load() != 1 || len(svc.channelOperations) != 0 {
		t.Fatalf("maximum=%d leftover locks=%d", maximum.Load(), len(svc.channelOperations))
	}
	// Holding one order must not block a distinct order.
	unlock := svc.LockCashierChannel("PAY-A")
	defer unlock()
	otherUnlock := svc.LockCashierChannel("PAY-B")
	otherUnlock()
}

func TestCashierVerifiedPaymentRecordsActualSourceAfterSelectionChanged(t *testing.T) {
	store := newTestStore(t)
	svc, _ := createPaymentService(store)
	svc.fees = NewFeeCalculator(config.Map{"FEE_RATE_NATIVE": "0", "FEE_RATE_ALIPAY": "0.03", "FEE_RATE_PAYPAL": "0.06", "CHANNEL_COST_ALIPAY": "0.01"})
	order := &db.PaymentOrder{ID: "source", OrderNo: "PAY-SOURCE", Amount: 10000, PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour)}
	insertOrder(t, store, order)
	paid, err := svc.MarkPaidForChannel("alipay", order.OrderNo, strPtr("ALIPAY-ACTUAL"), 10000, "CNY")
	if err != nil {
		t.Fatal(err)
	}
	if paid.PayMethod != "alipay" || paid.Fee != 300 || paid.ChannelCost != 100 || paid.SettleAmount != 9700 {
		t.Fatalf("settlement used the selected method instead of actual source: %+v", paid)
	}
	// A replay or another later source never changes the first receipt/fees.
	for _, source := range []string{"alipay", "paypal", "native"} {
		if _, err := svc.MarkPaidForChannel(source, order.OrderNo, strPtr("OTHER-"+source), 10000, "CNY"); err != nil {
			t.Fatal(err)
		}
		stored := reloadOrder(t, store, order.OrderNo)
		if stored.PayMethod != "alipay" || stored.ThirdPartyTradeNo == nil || *stored.ThirdPartyTradeNo != "ALIPAY-ACTUAL" || stored.Fee != 300 {
			t.Fatalf("later receipt changed original settlement: %+v", stored)
		}
	}
	requireErrStatus(t, svc.SwitchPaymentMethod(order.OrderNo, "native"), 409)
}

func TestCashierConcurrentVerifiedSourcesSettleAndNotifyOnce(t *testing.T) {
	store := newTestStore(t)
	svc, notify := createPaymentService(store)
	svc.fees = NewFeeCalculator(config.Map{"FEE_RATE_NATIVE": "0", "FEE_RATE_ALIPAY": "0.03"})
	order := &db.PaymentOrder{
		ID: "first-source", OrderNo: "PAY-FIRST-SOURCE", Amount: 10000, PayMethod: "paypal", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour),
		NotifyURL: strPtr("https://merchant.example/notify"), SigningSecret: strPtr("callback-test-secret"), SignatureVersion: 2,
	}
	insertOrder(t, store, order)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, source := range []string{"native", "alipay"} {
		wg.Add(1)
		go func(source string) {
			defer wg.Done()
			<-start
			_, err := svc.MarkPaidForChannel(source, order.OrderNo, strPtr("TRADE-"+source), 10000, "CNY")
			errors <- err
		}(source)
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored := reloadOrder(t, store, order.OrderNo)
	if stored.Status != db.StatusPaid || stored.ThirdPartyTradeNo == nil || *stored.ThirdPartyTradeNo != "TRADE-"+stored.PayMethod {
		t.Fatalf("source/trade not from one payment: %+v", stored)
	}
	wantFee := int64(0)
	if stored.PayMethod == "alipay" {
		wantFee = 300
	}
	if stored.Fee != wantFee || stored.SettleAmount != 10000-wantFee {
		t.Fatalf("incorrect source fee: %+v", stored)
	}
	count, err := store.Notifies.Count()
	if err != nil || count != 1 || len(notify.calls) != 1 || notify.calls[0].notification.PayMethod != stored.PayMethod {
		t.Fatalf("duplicate/inconsistent notification: count=%d calls=%+v err=%v", count, notify.calls, err)
	}
}
