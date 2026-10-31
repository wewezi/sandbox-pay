package services

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

type isolationRefundCall struct {
	orderOrCapture string
	requestID      string
	cents          int64
}

type isolationPayPalGateway struct {
	calls   []isolationRefundCall
	respond func(isolationRefundCall) (string, string, error)
}

type isolationRefundQuery struct {
	tradeNo  string
	cents    int64
	currency string
}

// Only this extended mock supports querying. Existing tests intentionally
// continue exercising the structured idempotent POST fallback.
type isolationQueriedPayPalGateway struct {
	*isolationPayPalGateway
	queries []isolationRefundQuery
	query   func(isolationRefundQuery) (string, error)
}

func (g *isolationQueriedPayPalGateway) QueryRefund(tradeNo string, cents int64, currency string) (string, error) {
	call := isolationRefundQuery{tradeNo: tradeNo, cents: cents, currency: currency}
	g.queries = append(g.queries, call)
	return g.query(call)
}

func (g *isolationPayPalGateway) Refund(string, int64, string, string) string {
	panic("expected the structured refund interface")
}

func (g *isolationPayPalGateway) RefundWithStatus(capture string, cents int64, currency, reason, requestID string) (string, string, error) {
	call := isolationRefundCall{orderOrCapture: capture, requestID: requestID, cents: cents}
	g.calls = append(g.calls, call)
	if g.respond != nil {
		return g.respond(call)
	}
	return "trade-" + requestID, "COMPLETED", nil
}

func newIsolationRefundService(t *testing.T) (*RefundService, *db.Store, *isolationPayPalGateway, int64, int64) {
	t.Helper()
	store := newTestStore(t)
	first := seedGroup1(t, store)
	second, err := store.Groups.Insert("second group", true)
	if err != nil {
		t.Fatal(err)
	}
	gateway := &isolationPayPalGateway{}
	payments := NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(testFeeConfig()))
	service := NewRefundService(store, payments, newFakeAlipay("unused"), gateway)
	return service, store, gateway, first, second.ID
}

func insertIsolationOrder(t *testing.T, store *db.Store, orderNo string, groupID int64) *db.PaymentOrder {
	t.Helper()
	order := createMockRefundOrder()
	order.ID, order.OrderNo, order.GroupID = orderNo, orderNo, &groupID
	order.PayMethod = "paypal"
	order.ThirdPartyTradeNo = strPtr("capture-" + orderNo)
	order.ForeignAmount, order.ForeignCurrency = i64Ptr(1400), strPtr("USD")
	order.NotifyURL = nil
	insertOrder(t, store, order)
	return order
}

func TestRefundRequestIsolation(t *testing.T) {
	service, store, gateway, first, second := newIsolationRefundService(t)
	orderA := insertIsolationOrder(t, store, "ORDER-A", first)
	orderB := insertIsolationOrder(t, store, "ORDER-B", second)
	// A maximum-length merchant ID must still fit the provider's smaller limit.
	publicID := strings.Repeat("r", 64)
	for _, order := range []*db.PaymentOrder{orderA, orderB} {
		result, err := service.Execute(RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: publicID}, RefundOptions{GroupID: order.GroupID})
		if err != nil || result == nil || result.Status != "completed" || result.RequestID != publicID {
			t.Fatalf("refund result=%+v error=%v", result, err)
		}
		op, err := store.Refunds.FindByTradeNo(order.OrderNo, result.RefundTradeNo)
		if err != nil || op == nil || op.RequestID != publicID || op.Status != db.RefundCompleted {
			t.Fatalf("operation=%+v error=%v", op, err)
		}
		if row := reloadOrder(t, store, order.OrderNo); row.RefundedAmount != 1000 || row.Status != db.StatusPaid {
			t.Fatalf("refund did not update only its own order: %+v", row)
		}
	}
	if len(gateway.calls) != 2 || gateway.calls[0].requestID == gateway.calls[1].requestID {
		t.Fatalf("groups shared a provider idempotency ID: %+v", gateway.calls)
	}
	for _, call := range gateway.calls {
		if call.requestID == publicID || len(call.requestID) > 38 {
			t.Fatalf("unsafe provider request ID: %q", call.requestID)
		}
	}
	// Replaying the completed request does not contact the gateway or double debit.
	result, err := service.Execute(RefundInput{OrderNo: orderA.OrderNo, Amount: 10, RequestID: publicID}, RefundOptions{GroupID: &first})
	if err != nil || result == nil || result.Status != "completed" || len(gateway.calls) != 2 {
		t.Fatalf("completed replay result=%+v error=%v calls=%d", result, err, len(gateway.calls))
	}
	if row := reloadOrder(t, store, orderA.OrderNo); row.RefundedAmount != 1000 {
		t.Fatal("replay counted the refund twice")
	}
	// Knowing an existing order/request pair cannot bypass the group boundary.
	_, err = service.Execute(RefundInput{OrderNo: orderA.OrderNo, Amount: 10, RequestID: publicID}, RefundOptions{GroupID: &second})
	requireErrStatus(t, err, 404)
	if len(gateway.calls) != 2 {
		t.Fatal("cross-group access contacted the gateway")
	}
}

func TestRefundRetryKeepsOperationAndOwnership(t *testing.T) {
	for _, initialStatus := range []string{"PENDING", "UNKNOWN"} {
		t.Run(initialStatus, func(t *testing.T) {
			service, store, gateway, first, second := newIsolationRefundService(t)
			order := insertIsolationOrder(t, store, "ORDER-RETRY", first)
			gateway.respond = func(call isolationRefundCall) (string, string, error) {
				if len(gateway.calls) == 1 {
					if initialStatus == "UNKNOWN" {
						return "", "UNKNOWN", errors.New("simulated timeout after sending refund")
					}
					return "trade-" + call.requestID, "PENDING", nil
				}
				return "trade-" + call.requestID, "COMPLETED", nil
			}
			input := RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: "merchant-refund-1"}
			result, err := service.Execute(input, RefundOptions{GroupID: &first})
			if err != nil || result == nil || result.Status != "pending" || result.RequestID != input.RequestID {
				t.Fatalf("in-flight result=%+v error=%v", result, err)
			}
			operationID := gateway.calls[0].requestID
			op, err := store.Refunds.FindByID(operationID)
			if err != nil || op == nil || op.Status == db.RefundCompleted {
				t.Fatalf("in-flight operation=%+v error=%v", op, err)
			}
			if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusRefunding || row.RefundedAmount != 0 {
				t.Fatal("pending/unknown refund was counted as completed")
			}
			_, err = service.Execute(input, RefundOptions{GroupID: &second})
			requireErrStatus(t, err, 404)
			changed := input
			changed.Amount = 20
			_, err = service.Execute(changed, RefundOptions{GroupID: &first})
			requireErrStatus(t, err, 409)
			if len(gateway.calls) != 1 {
				t.Fatal("wrong owner or changed amount redispatched the refund")
			}
			result, err = service.Execute(input, RefundOptions{GroupID: &first})
			if err != nil || result == nil || result.Status != "completed" || result.RequestID != input.RequestID {
				t.Fatalf("retry result=%+v error=%v", result, err)
			}
			if len(gateway.calls) != 2 || gateway.calls[1].requestID != operationID {
				t.Fatal("retry issued a different provider operation")
			}
			if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusPaid || row.RefundedAmount != 1000 {
				t.Fatalf("retry did not complete exactly once: %+v", row)
			}
			op, err = store.Refunds.FindByID(operationID)
			if err != nil || op == nil || op.Status != db.RefundCompleted {
				t.Fatalf("completed operation=%+v error=%v", op, err)
			}
			if _, err := service.Execute(input, RefundOptions{GroupID: &first}); err != nil || len(gateway.calls) != 2 {
				t.Fatal("completed retry reached the gateway again")
			}
		})
	}
}

func TestRefundRejectsTradeFromAnotherOperation(t *testing.T) {
	service, store, gateway, first, _ := newIsolationRefundService(t)
	order := insertIsolationOrder(t, store, "ORDER-DUPLICATE-TRADE", first)
	gateway.respond = func(isolationRefundCall) (string, string, error) {
		return "same-upstream-trade", "COMPLETED", nil
	}
	firstRequest := RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: "refund-first"}
	if _, err := service.Execute(firstRequest, RefundOptions{GroupID: &first}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: "refund-second"}, RefundOptions{GroupID: &first})
	requireErrStatus(t, err, 409)
	if result != nil {
		t.Fatal("a different operation's trade was reported as completed")
	}
	row := reloadOrder(t, store, order.OrderNo)
	if row.RefundedAmount != 1000 || row.Status != db.StatusRefunding {
		t.Fatalf("duplicate trade changed the balance or dropped the in-flight state: %+v", row)
	}
	firstOp, err := store.Refunds.FindByID(gateway.calls[0].requestID)
	if err != nil || firstOp == nil || firstOp.Status != db.RefundCompleted {
		t.Fatalf("first operation=%+v error=%v", firstOp, err)
	}
	secondOp, err := store.Refunds.FindByID(gateway.calls[1].requestID)
	if err != nil || secondOp == nil || secondOp.Status == db.RefundCompleted {
		t.Fatalf("second operation=%+v error=%v", secondOp, err)
	}
}

func TestRefundInProgressCannotRedispatch(t *testing.T) {
	service, store, gateway, first, _ := newIsolationRefundService(t)
	order := insertIsolationOrder(t, store, "ORDER-PROCESSING", first)
	entered, release := make(chan struct{}), make(chan struct{}, 1)
	defer close(release)
	gateway.respond = func(call isolationRefundCall) (string, string, error) {
		close(entered)
		<-release
		return "trade-" + call.requestID, "COMPLETED", nil
	}
	input := RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: "refund-processing"}
	done := make(chan error, 1)
	go func() {
		_, err := service.Execute(input, RefundOptions{GroupID: &first})
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("refund returned before gateway dispatch: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("refund did not reach the fake gateway")
	}
	_, err := service.Execute(input, RefundOptions{GroupID: &first})
	requireErrStatus(t, err, 409)
	other := input
	other.RequestID = "a-second-refund"
	_, err = service.Execute(other, RefundOptions{GroupID: &first})
	requireErrStatus(t, err, 409)
	if len(gateway.calls) != 1 {
		t.Fatal("an in-progress refund was sent to the gateway twice")
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if row := reloadOrder(t, store, order.OrderNo); row.RefundedAmount != 1000 || row.Status != db.StatusPaid {
		t.Fatalf("refund reservation did not complete atomically: %+v", row)
	}
}

func TestRefundKnownTradeRetryQueriesWithoutRedispatch(t *testing.T) {
	service, store, gateway, first, _ := newIsolationRefundService(t)
	order := insertIsolationOrder(t, store, "ORDER-KNOWN-TRADE", first)
	gateway.respond = func(isolationRefundCall) (string, string, error) {
		return "persisted-provider-refund", "PENDING", nil
	}
	querier := &isolationQueriedPayPalGateway{isolationPayPalGateway: gateway}
	querier.query = func(call isolationRefundQuery) (string, error) {
		if call.tradeNo != "persisted-provider-refund" || call.cents != 140 || call.currency != "USD" {
			t.Fatalf("query did not use the persisted refund and original converted amount: %+v", call)
		}
		if len(querier.queries) == 1 {
			return "UNKNOWN", errors.New("simulated query network failure")
		}
		return "COMPLETED", nil
	}
	service.paypal = querier
	input := RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: "known-trade-request"}
	result, err := service.Execute(input, RefundOptions{GroupID: &first})
	if err != nil || result == nil || result.Status != "pending" || len(gateway.calls) != 1 || len(querier.queries) != 0 {
		t.Fatalf("initial dispatch result=%+v error=%v POSTs=%d queries=%d", result, err, len(gateway.calls), len(querier.queries))
	}
	operationID := gateway.calls[0].requestID
	result, err = service.Execute(input, RefundOptions{GroupID: &first})
	if err != nil || result == nil || result.Status != "pending" || result.RefundTradeNo != "persisted-provider-refund" {
		t.Fatalf("query failure was not retained for safe retry: result=%+v error=%v", result, err)
	}
	if len(gateway.calls) != 1 || len(querier.queries) != 1 {
		t.Fatal("query failure issued another refund POST")
	}
	op, err := store.Refunds.FindByID(operationID)
	if err != nil || op == nil || op.Status == db.RefundCompleted || strDeref(op.TradeNo, "") != "persisted-provider-refund" {
		t.Fatalf("query failure lost its durable trade: operation=%+v error=%v", op, err)
	}
	if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusRefunding || row.RefundedAmount != 0 {
		t.Fatal("query failure was counted as a completed refund")
	}
	result, err = service.Execute(input, RefundOptions{GroupID: &first})
	if err != nil || result == nil || result.Status != "completed" || result.RequestID != input.RequestID {
		t.Fatalf("completed query result=%+v error=%v", result, err)
	}
	if len(gateway.calls) != 1 || len(querier.queries) != 2 {
		t.Fatal("known refund was reissued instead of queried")
	}
	if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusPaid || row.RefundedAmount != 1000 {
		t.Fatalf("query did not complete exactly once: %+v", row)
	}
	if _, err := service.Execute(input, RefundOptions{GroupID: &first}); err != nil || len(gateway.calls) != 1 || len(querier.queries) != 2 {
		t.Fatal("completed query replay contacted the gateway")
	}
	if row := reloadOrder(t, store, order.OrderNo); row.RefundedAmount != 1000 {
		t.Fatal("completed query replay counted the refund twice")
	}
}

func TestRefundUnknownWithoutTradeExpiresSafeRetryWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aged-refund.db")
	store, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first := seedGroup1(t, store)
	order := insertIsolationOrder(t, store, "ORDER-EXPIRED-UNKNOWN", first)
	gateway := &isolationPayPalGateway{respond: func(isolationRefundCall) (string, string, error) {
		return "", "UNKNOWN", errors.New("simulated lost response")
	}}
	service := NewRefundService(store, NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(testFeeConfig())), newFakeAlipay("unused"), gateway)
	input := RefundInput{OrderNo: order.OrderNo, Amount: 10, RequestID: "expired-unknown-request"}
	result, err := service.Execute(input, RefundOptions{GroupID: &first})
	if err != nil || result == nil || result.Status != "pending" || len(gateway.calls) != 1 {
		t.Fatalf("initial unknown result=%+v error=%v", result, err)
	}
	operationID := gateway.calls[0].requestID
	// A second connection ages the persisted operation without accessing Store's
	// private connection or using a wall-clock wait.
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	aged := time.Now().Add(-31 * time.Minute).UTC().Format("2006-01-02 15:04:05.000")
	if _, err := conn.Exec(`UPDATE "refund_operations" SET "createdAt" = ? WHERE "id" = ?`, aged, operationID); err != nil {
		t.Fatal(err)
	}
	_, err = service.Execute(input, RefundOptions{GroupID: &first})
	requireErrStatus(t, err, 409)
	if len(gateway.calls) != 1 {
		t.Fatal("expired unknown result issued a second refund POST")
	}
	op, err := store.Refunds.FindByID(operationID)
	if err != nil || op == nil || op.Status != db.RefundUnknown || op.TradeNo != nil {
		t.Fatalf("expired operation changed: %+v error=%v", op, err)
	}
	if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusRefunding || row.RefundedAmount != 0 {
		t.Fatal("expired unknown result changed the refund balance")
	}
}
