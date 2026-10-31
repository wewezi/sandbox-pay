package services

import (
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

func TestConcurrentOrderIdempotency(t *testing.T) {
	store := newTestStore(t)
	groupID := seedGroup1(t, store)
	svc, _ := createPaymentService(store)
	const requests = 32
	results := make(chan *OrderResult, requests)
	errors := make(chan error, requests)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := svc.CreateOrder(OrderCaller{GroupID: groupID}, CreateOrderInput{Amount: 10, ProductName: "item", PayMethod: "native", ExternalOrderNo: strPtr("same-order")})
			results <- result
			errors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("CreateOrder: %v", err)
		}
	}
	var orderNo string
	for result := range results {
		if orderNo == "" {
			orderNo = result.OrderNo
		}
		if result.OrderNo != orderNo {
			t.Fatalf("different orders for same idempotency key: %s / %s", orderNo, result.OrderNo)
		}
	}
	count, err := store.Orders.Count()
	if err != nil || count != 1 {
		t.Fatalf("order count = %d, %v; want 1", count, err)
	}
	_, err = svc.CreateOrder(OrderCaller{GroupID: groupID}, CreateOrderInput{Amount: 10, ProductName: "item", PayMethod: "paypal", ExternalOrderNo: strPtr("same-order")})
	requireErrStatus(t, err, 409)
}

type blockedRefund struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (f *blockedRefund) Refund(string, int64, string) string { panic("structured path required") }
func (f *blockedRefund) RefundWithStatus(_ string, _ int64, _, requestID string) (string, string, error) {
	f.calls.Add(1)
	f.started <- struct{}{}
	<-f.release
	return requestID, "COMPLETED", nil
}

func TestConcurrentRefundOnlyOneDispatches(t *testing.T) {
	store := newTestStore(t)
	seedGroup1(t, store)
	order := createMockRefundOrder()
	insertOrder(t, store, order)
	payment, _ := createPaymentService(store)
	upstream := &blockedRefund{started: make(chan struct{}, 16), release: make(chan struct{})}
	svc := NewRefundService(store, payment, upstream, newFakePayPal("unused"))
	first := make(chan error, 1)
	go func() {
		_, err := svc.Execute(RefundInput{OrderNo: order.OrderNo, Amount: 10}, adminOpts())
		first <- err
	}()
	select {
	case <-upstream.started:
	case <-time.After(5 * time.Second):
		close(upstream.release)
		t.Fatal("first refund did not dispatch")
	}
	const concurrent = 15
	errors := make(chan error, concurrent)
	for i := 0; i < concurrent; i++ {
		go func() {
			_, err := svc.Execute(RefundInput{OrderNo: order.OrderNo, Amount: 10}, adminOpts())
			errors <- err
		}()
	}
	for i := 0; i < concurrent; i++ {
		select {
		case err := <-errors:
			requireErrStatus(t, err, 409)
		case <-time.After(5 * time.Second):
			close(upstream.release)
			t.Fatal("concurrent refund reached upstream instead of rejecting")
		}
	}
	if upstream.calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstream.calls.Load())
	}
	close(upstream.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	row := reloadOrder(t, store, order.OrderNo)
	if row.RefundedAmount != 1000 || row.Status != db.StatusPaid {
		t.Fatalf("refund result = %+v", row)
	}
}

// 用真实SQLite触发器让通知INSERT失败，验证数据库写入失败也能回滚状态。
func TestStateAndNotificationCommitTogether(t *testing.T) {
	for _, refund := range []bool{false, true} {
		name := "payment"
		if refund {
			name = "refund"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "outbox.db")
			store, err := db.Open(path, "")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			conn, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			seedGroup1(t, store)
			key := seedAPIKey(t, store, 1, "outbox-signing-key")
			order := createMockPaymentOrder()
			order.Status = db.StatusPending
			if refund {
				order.Status = db.StatusPaid
			}
			order.NotifyURL, order.APIKeyID = strPtr("https://example.com/notify"), &key
			order.SigningSecret, order.SignatureVersion = strPtr("whsec_outbox_test"), 2
			insertOrder(t, store, order)
			svc, _ := createPaymentService(store)
			if _, err := conn.Exec(`CREATE TRIGGER reject_notify BEFORE INSERT ON "notify_queues" BEGIN SELECT RAISE(ABORT, 'reject notification'); END`); err != nil {
				t.Fatal(err)
			}
			apply := func() error {
				if refund {
					_, err := svc.MarkRefunded(order.OrderNo, 1000, "RF_1", nil)
					return err
				}
				_, err := svc.MarkPaid(order.OrderNo, strPtr("PAY_1"), order.Amount, "CNY")
				return err
			}
			if err := apply(); err == nil {
				t.Fatal("notification failure must fail transaction")
			}
			row := reloadOrder(t, store, order.OrderNo)
			if row.Status != order.Status || row.RefundedAmount != 0 || row.ThirdPartyTradeNo != nil {
				t.Fatalf("state changed despite failed notification: %+v", row)
			}
			count, err := store.Notifies.Count()
			if err != nil || count != 0 {
				t.Fatalf("notifications = %d, %v", count, err)
			}
			var refunds int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM "refund_operations"`).Scan(&refunds); err != nil || refunds != 0 {
				t.Fatalf("refund records = %d, %v", refunds, err)
			}
			if _, err := conn.Exec(`DROP TRIGGER reject_notify`); err != nil {
				t.Fatal(err)
			}
			if err := apply(); err != nil {
				t.Fatal(err)
			}
			if err := apply(); err != nil {
				t.Fatal(err)
			}
			count, err = store.Notifies.Count()
			if err != nil || count != 1 {
				t.Fatalf("duplicate event created %d notifications, %v; want 1", count, err)
			}
		})
	}
}

func TestHistoricalPartialRefundReplay(t *testing.T) {
	store := newTestStore(t)
	seedGroup1(t, store)
	order := createMockPaymentOrder()
	order.Status = db.StatusPaid
	insertOrder(t, store, order)
	svc, _ := createPaymentService(store)
	for _, trade := range []string{"RF_1", "RF_2", "RF_1"} {
		if _, err := svc.MarkRefunded(order.OrderNo, 1000, trade, nil); err != nil {
			t.Fatal(err)
		}
	}
	if amount := reloadOrder(t, store, order.OrderNo).RefundedAmount; amount != 2000 {
		t.Fatalf("replayed partial refund = %d, want 2000", amount)
	}
}
