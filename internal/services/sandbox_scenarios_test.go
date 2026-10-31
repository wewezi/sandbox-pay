package services

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

func sandboxFixture(t *testing.T) (*db.Store, *SandboxScenarioService, *PaymentService) {
	t.Helper()
	store := newTestStore(t)
	groupID := seedGroup1(t, store)
	secret, url := "fixture-callback-secret", "https://merchant.example/notify"
	insertOrder(t, store, &db.PaymentOrder{ID: "scenario-id", OrderNo: "SCENARIO", Amount: 1000, ProductName: "scenario", PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour), GroupID: &groupID, NotifyURL: &url, SigningSecret: &secret, SignatureVersion: 2})
	payment := NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(config.Map{"FEE_RATE_NATIVE": "0", "CHANNEL_COST_NATIVE": "0"}))
	return store, NewSandboxScenarioService(store, payment), payment
}

func TestSandboxImmediateScenarios(t *testing.T) {
	for _, scenario := range []string{"paid", "failed", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			store, service, _ := sandboxFixture(t)
			task, err := service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: scenario})
			if err != nil {
				t.Fatal(err)
			}
			if task.Status != "completed" {
				t.Fatalf("task=%#v", task)
			}
			order := reloadOrder(t, store, "SCENARIO")
			want := map[string]int{"paid": db.StatusPaid, "failed": db.StatusFailed, "expired": db.StatusExpired}[scenario]
			if order.Status != want {
				t.Fatalf("status=%d want=%d", order.Status, want)
			}
			if scenario == "expired" && order.ExpireAt.After(time.Now()) {
				t.Fatal("expiry not advanced")
			}
			if err := service.RunDue(); err != nil {
				t.Fatal(err)
			}
			if scenario == "paid" {
				_, count, err := store.Notifies.Search(db.NotifySearchOpts{OrderNo: "SCENARIO", Limit: 20})
				if err != nil || count != 1 {
					t.Fatalf("notification count=%d err=%v", count, err)
				}
			}
		})
	}
}

func TestSandboxTasksPersistAcrossRestartAndRejectConflictingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	store, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	groupID := seedGroup1(t, store)
	insertOrder(t, store, &db.PaymentOrder{ID: "restart-id", OrderNo: "RESTART", Amount: 100, ProductName: "restart", PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour), GroupID: &groupID})
	payment := NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(config.Map{}))
	service := NewSandboxScenarioService(store, payment)
	task, err := service.Create(SandboxScenarioInput{OrderNo: "RESTART", Scenario: "delayed_paid", DelaySeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(SandboxScenarioInput{OrderNo: "RESTART", Scenario: "failed"})
	requireErrStatus(t, err, http.StatusConflict)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service = NewSandboxScenarioService(store, NewPaymentService(store, &fakeNotify{}, NewFeeCalculator(config.Map{})))
	restored, err := store.SandboxTasks().Find(task.ID)
	if err != nil || restored == nil || restored.Status != "pending" {
		t.Fatalf("restored=%#v err=%v", restored, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := service.RunDue(); err != nil {
		t.Fatal(err)
	}
	if reloadOrder(t, store, "RESTART").Status != db.StatusPaid {
		t.Fatal("delayed payment not completed after restart")
	}
	restored, _ = store.SandboxTasks().Find(task.ID)
	if restored.Status != "completed" {
		t.Fatalf("task status=%s", restored.Status)
	}
}

func TestSandboxPendingRefundCancellationAndCompletion(t *testing.T) {
	store, service, payment := sandboxFixture(t)
	trade := "ORIGINAL"
	if _, err := payment.MarkPaid("SCENARIO", &trade, 1000, "CNY"); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: "refund_failed", Amount: 1})
	if err != nil {
		t.Fatal(err)
	}
	failedOp, err := store.Refunds.FindByID(refundOperationID("SCENARIO", sandboxRefundRequestID(failed.ID)))
	if err != nil || failedOp == nil || failedOp.Status != db.RefundFailed {
		t.Fatalf("failed operation=%#v err=%v", failedOp, err)
	}
	if order := reloadOrder(t, store, "SCENARIO"); order.Status != db.StatusPaid || order.RefundedAmount != 0 {
		t.Fatalf("failed refund changed balance: %#v", order)
	}
	pending, err := service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: "refund_pending", DelaySeconds: 60, Amount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if reloadOrder(t, store, "SCENARIO").Status != db.StatusRefunding {
		t.Fatal("missing pending state")
	}
	if _, err := service.Cancel(pending.ID); err != nil {
		t.Fatal(err)
	}
	if order := reloadOrder(t, store, "SCENARIO"); order.Status != db.StatusPaid || order.RefundedAmount != 0 {
		t.Fatal("cancellation did not restore paid state")
	}
	completed, err := service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: "refund_pending", DelaySeconds: 1, Amount: 3})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := service.RunDue(); err != nil {
		t.Fatal(err)
	}
	if order := reloadOrder(t, store, "SCENARIO"); order.Status != db.StatusPaid || order.RefundedAmount != 300 {
		t.Fatalf("wrong partial refund balance: %#v", order)
	}
	op, err := store.Refunds.FindByID(refundOperationID("SCENARIO", sandboxRefundRequestID(completed.ID)))
	if err != nil || op == nil || op.Status != db.RefundCompleted {
		t.Fatalf("completed operation=%#v err=%v", op, err)
	}
	if err := service.RunDue(); err != nil {
		t.Fatal(err)
	}
	if reloadOrder(t, store, "SCENARIO").RefundedAmount != 300 {
		t.Fatal("duplicate refund after repeated worker run")
	}
}

func TestSandboxRejectsExternalChannelsAndOversizedRefunds(t *testing.T) {
	store, service, payment := sandboxFixture(t)
	if err := store.Orders.Update("SCENARIO", map[string]any{"payMethod": "paypal"}); err != nil {
		t.Fatal(err)
	}
	_, err := service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: "paid"})
	requireErrStatus(t, err, http.StatusConflict)
	if err := store.Orders.Update("SCENARIO", map[string]any{"payMethod": "native"}); err != nil {
		t.Fatal(err)
	}
	trade := "PAID"
	if _, err := payment.MarkPaid("SCENARIO", &trade, 1000, "CNY"); err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: "refund_pending", Amount: 10.01})
	requireErrStatus(t, err, http.StatusBadRequest)
	_, err = service.Create(SandboxScenarioInput{OrderNo: "SCENARIO", Scenario: "failed"})
	requireErrStatus(t, err, http.StatusConflict)
	items, err := store.SandboxTasks().List("")
	if err != nil || len(items) != 0 {
		t.Fatalf("rejected scenarios created tasks %s err=%v", fmt.Sprint(items), err)
	}
}
