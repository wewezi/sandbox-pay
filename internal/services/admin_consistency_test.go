package services

import (
	"testing"

	"sandbox-pay/internal/db"
)

func adminInFlightFixture(t *testing.T, status string) (*db.Store, *db.PaymentOrder, *db.RefundOperation, *PaymentService) {
	t.Helper()
	store := newTestStore(t)
	groupID := seedGroup1(t, store)
	order := createMockRefundOrder()
	order.GroupID = &groupID
	order.Status = db.StatusRefunding
	order.NotifyURL = nil
	insertOrder(t, store, order)
	op := &db.RefundOperation{
		ID: "operation-" + status, RequestID: "request-" + status,
		OrderNo: order.OrderNo, Amount: 500, Status: status,
	}
	if err := store.Refunds.Insert(op); err != nil {
		t.Fatal(err)
	}
	if err := store.Notifies.Insert(db.InsertNotifyInput{
		OrderNo: &order.OrderNo, URL: "https://example.com/notify", Body: "{}", Signature: "{}", Status: db.NotifyPending,
	}); err != nil {
		t.Fatal(err)
	}
	payments := NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(testFeeConfig()))
	return store, order, op, payments
}

func assertAdminKeptInFlightData(t *testing.T, store *db.Store, order *db.PaymentOrder, op *db.RefundOperation) {
	t.Helper()
	if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusRefunding || row.RefundedAmount != 0 {
		t.Fatalf("rejected administrative action changed the order: %+v", row)
	}
	stored, err := store.Refunds.FindByID(op.ID)
	if err != nil || stored == nil || stored.Status != op.Status || stored.Amount != op.Amount || stored.RequestID != op.RequestID {
		t.Fatalf("rejected administrative action changed the operation: %+v error=%v", stored, err)
	}
	if count, err := store.Notifies.Count(); err != nil || count != 1 {
		t.Fatalf("rejected administrative action changed notifications: count=%d error=%v", count, err)
	}
}

func TestAdminDeletePreservesInFlightRefund(t *testing.T) {
	for _, status := range []string{db.RefundProcessing, db.RefundPending, db.RefundUnknown} {
		t.Run(status, func(t *testing.T) {
			store, order, op, payments := adminInFlightFixture(t, status)
			admin := NewAdminService(store)
			result, err := admin.DeleteTransaction(order.OrderNo, "test-admin", nil)
			requireErrStatus(t, err, 409)
			if result != nil {
				t.Fatal("rejected deletion returned a success result")
			}
			assertAdminKeptInFlightData(t, store, order, op)
			if _, err := payments.completeRefund(order.OrderNo, op.Amount, "completed-"+status, nil, op.ID); err != nil {
				t.Fatal(err)
			}
			result, err = admin.DeleteTransaction(order.OrderNo, "test-admin", nil)
			if err != nil || result == nil || !result.Success {
				t.Fatalf("completed refund could not be deleted: result=%+v error=%v", result, err)
			}
			if row, err := store.Orders.FindByOrderNo(order.OrderNo, false); err != nil || row != nil {
				t.Fatalf("completed order was not deleted: row=%+v error=%v", row, err)
			}
			if row, err := store.Refunds.FindByID(op.ID); err != nil || row != nil {
				t.Fatalf("completed operation did not cascade with deletion: row=%+v error=%v", row, err)
			}
		})
	}
}

func TestAdminResetPreservesInFlightRefund(t *testing.T) {
	for _, status := range []string{db.RefundProcessing, db.RefundPending, db.RefundUnknown} {
		t.Run(status, func(t *testing.T) {
			store, order, op, payments := adminInFlightFixture(t, status)
			other := createMockRefundOrder()
			other.ID, other.OrderNo, other.ThirdPartyTradeNo = "other-id", "OTHER-ORDER", strPtr("OTHER-TRADE")
			other.NotifyURL = nil
			insertOrder(t, store, other)
			admin := NewAdminService(store)
			result, err := admin.ResetData("test-admin", nil)
			requireErrStatus(t, err, 409)
			if result != nil {
				t.Fatal("rejected reset returned a success result")
			}
			assertAdminKeptInFlightData(t, store, order, op)
			if count, err := store.Orders.Count(); err != nil || count != 2 {
				t.Fatalf("rejected reset partially deleted orders: count=%d error=%v", count, err)
			}
			if _, err := payments.completeRefund(order.OrderNo, op.Amount, "completed-"+status, nil, op.ID); err != nil {
				t.Fatal(err)
			}
			result, err = admin.ResetData("test-admin", nil)
			if err != nil || result == nil || result.Deleted.Orders != 2 || result.Deleted.Notifications != 1 {
				t.Fatalf("completed refund could not be reset: result=%+v error=%v", result, err)
			}
			if count, err := store.Orders.Count(); err != nil || count != 0 {
				t.Fatalf("reset retained orders: count=%d error=%v", count, err)
			}
			if count, err := store.Notifies.Count(); err != nil || count != 0 {
				t.Fatalf("reset retained notifications: count=%d error=%v", count, err)
			}
			if row, err := store.Refunds.FindByID(op.ID); err != nil || row != nil {
				t.Fatalf("reset retained completed operation: row=%+v error=%v", row, err)
			}
		})
	}
}

func TestAdminProtectsLegacyRefundingWithoutOperation(t *testing.T) {
	store := newTestStore(t)
	seedGroup1(t, store)
	order := createMockRefundOrder()
	order.Status = db.StatusRefunding
	order.NotifyURL = nil
	insertOrder(t, store, order)
	admin := NewAdminService(store)
	_, err := admin.DeleteTransaction(order.OrderNo, "test-admin", nil)
	requireErrStatus(t, err, 409)
	_, err = admin.ResetData("test-admin", nil)
	requireErrStatus(t, err, 409)
	if row := reloadOrder(t, store, order.OrderNo); row.Status != db.StatusRefunding {
		t.Fatal("legacy refund awaiting reconciliation was deleted or changed")
	}
	if count, err := store.Refunds.CountActive(); err != nil || count != 0 {
		t.Fatalf("legacy fixture unexpectedly has an operation: count=%d error=%v", count, err)
	}
}
