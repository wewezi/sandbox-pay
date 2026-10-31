package routes

import (
	"net/http"
	"testing"
	"time"

	"sandbox-pay/internal/services"
)

func TestAdminTransactionsLocalDateDrilldown(t *testing.T) {
	h := newHarness(t, nil)
	start := time.Date(2026, 10, 30, 16, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	for _, created := range []time.Time{start.Add(-time.Millisecond), start, end.Add(-time.Millisecond), end} {
		order, err := h.c.Payment.CreateOrder(services.OrderCaller{GroupID: h.groupID}, services.CreateOrderInput{
			Amount: 1, ProductName: "运营日期筛选", PayMethod: "native",
		})
		if err != nil {
			t.Fatalf("CreateOrder: %v", err)
		}
		if err := h.store.Orders.Update(order.OrderNo, map[string]any{"createdAt": created}); err != nil {
			t.Fatalf("set createdAt: %v", err)
		}
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions?dateFrom=2026-10-31&dateTo=2026-10-31&utcOffset=480", Headers: h.auth})
	eq(t, "status", r.Status, http.StatusOK)
	eq(t, "local date count", num(t, r.data()["total"], "total"), 2)
	items := r.data()["items"].([]any)
	eq(t, "last millisecond", items[0].(map[string]any)["createdAt"], "2026-10-31T15:59:59.999Z")
	eq(t, "first millisecond", items[1].(map[string]any)["createdAt"], "2026-10-30T16:00:00.000Z")
}

func TestAdminTransactionsRejectsInvalidUTCOffset(t *testing.T) {
	h := newHarness(t, nil)
	for _, value := range []string{"abc", "480.5", "-721", "841"} {
		t.Run(value, func(t *testing.T) {
			r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions?utcOffset=" + value, Headers: h.auth})
			eq(t, "status", r.Status, http.StatusBadRequest)
		})
	}
}
