package routes

import (
	"net/http"
	"testing"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func TestPaymentReceiptAndExternalRecovery(t *testing.T) {
	h := newHarness(t, nil)
	body := map[string]any{"amount": 1.5, "productName": "receipt", "payMethod": "native", "externalOrderNo": "RECOVERY", "notifyUrl": "https://merchant.example/notify"}
	created := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Body: body, Headers: h.groupAuth})
	eq(t, "created", created.Status, http.StatusOK)
	data := created.data()
	orderNo := str(t, data["orderNo"], "orderNo")
	eq(t, "legacy data equals paymentUrl", str(t, data["data"], "data"), str(t, data["paymentUrl"], "paymentUrl"))
	eq(t, "status", num(t, data["status"], "status"), float64(db.StatusPending))
	eq(t, "amount", num(t, data["amount"], "amount"), 1.5)
	if str(t, data["expireAt"], "expireAt") == "" {
		t.Fatal("missing expiry")
	}
	recovered := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?externalOrderNo=RECOVERY", Headers: h.groupAuth})
	eq(t, "recovered", recovered.Status, http.StatusOK)
	eq(t, "order recovered", str(t, recovered.data()["orderNo"], "orderNo"), orderNo)
	group, err := h.c.Groups.CreateGroup("other", "other-key")
	if err != nil {
		t.Fatal(err)
	}
	denied := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?externalOrderNo=RECOVERY", Headers: map[string]string{"Authorization": "Bearer " + group.Keys[0].Key}})
	eq(t, "other group", denied.Status, http.StatusNotFound)
	trade := "CONFIRMED"
	if _, err := h.c.Payment.MarkPaid(orderNo, &trade, 150, "CNY"); err != nil {
		t.Fatal(err)
	}
	for _, state := range []int{db.StatusPaid, db.StatusRefunding, db.StatusRefunded, db.StatusFailed, db.StatusExpired} {
		if err := h.store.Orders.Update(orderNo, map[string]any{"status": state}); err != nil {
			t.Fatal(err)
		}
		retry := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Body: body, Headers: h.groupAuth})
		eq(t, "terminal retry", retry.Status, http.StatusOK)
		eq(t, "same order", str(t, retry.data()["orderNo"], "orderNo"), orderNo)
		eq(t, "terminal state", num(t, retry.data()["status"], "status"), float64(state))
	}
	count, err := h.store.Orders.Count()
	if err != nil || count != 1 {
		t.Fatalf("duplicate order count=%d err=%v", count, err)
	}
	_, notifications, err := h.store.Notifies.Search(db.NotifySearchOpts{OrderNo: orderNo, Limit: 20})
	if err != nil || notifications != 1 {
		t.Fatalf("duplicate callbacks=%d err=%v", notifications, err)
	}
}

func TestExternalRecoveryRejectsAmbiguousIdentifiers(t *testing.T) {
	h := newHarness(t, nil)
	for _, path := range []string{"/v1/api/gateway/query", "/v1/api/gateway/query?orderNo=ONE&externalOrderNo=TWO"} {
		response := h.do(t, reqSpec{Method: "GET", Path: path, Headers: h.groupAuth})
		eq(t, "invalid query", response.Status, http.StatusBadRequest)
	}
}

func TestTerminalExternalPaymentRetryDoesNotRequireProviderCredentials(t *testing.T) {
	h := newHarness(t, nil)
	for _, method := range []string{"alipay", "paypal"} {
		external := "TERMINAL-" + method
		order, err := h.c.Payment.CreateOrder(services.OrderCaller{GroupID: h.groupID}, services.CreateOrderInput{Amount: 1, ProductName: "terminal", PayMethod: method, ExternalOrderNo: &external})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.store.Orders.Update(order.OrderNo, map[string]any{"status": db.StatusPaid}); err != nil {
			t.Fatal(err)
		}
		response := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Headers: h.groupAuth, Body: map[string]any{"amount": 1, "productName": "terminal", "payMethod": method, "externalOrderNo": external}})
		eq(t, "provider must not be initialized on terminal retry", response.Status, http.StatusOK)
		eq(t, "terminal order", str(t, response.data()["orderNo"], "orderNo"), order.OrderNo)
	}
}
