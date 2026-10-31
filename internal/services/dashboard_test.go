package services

import (
	"fmt"
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

func TestDashboardCohortsKeepRefundedPaymentsAndCalendarBoundaries(t *testing.T) {
	store := newTestStore(t)
	groupID := seedGroup1(t, store)
	service := NewAdminService(store)
	// UTC Oct 31 is already Nov 1 in Hong Kong.
	now := time.Date(2026, 10, 31, 17, 23, 0, 0, time.UTC)
	start := time.Date(2026, 10, 25, 16, 0, 0, 0, time.UTC)
	previousStart := start.AddDate(0, 0, -7)
	end := start.AddDate(0, 0, 7)
	orders := []struct {
		at            time.Time
		status        int
		channel       string
		amount        int64
		refunded, fee int64
	}{
		{previousStart.Add(-time.Millisecond), db.StatusPending, "native", 1, 0, 0},
		{previousStart, db.StatusPaid, "native", 111, 0, 1},
		{start.Add(-time.Millisecond), db.StatusPaid, "native", 200, 0, 2},
		{start, db.StatusPaid, "native", 1234, 0, 31},
		{start.Add(time.Hour), db.StatusPaid, "native", 321, 21, 3},
		{start.AddDate(0, 0, 2), db.StatusRefunded, "alipay", 500, 500, 5},
		{start.AddDate(0, 0, 6), db.StatusRefunding, "paypal", 999, 100, 7},
		{start.Add(time.Hour), db.StatusFailed, "native", 9999, 0, 99},
		{start.Add(time.Hour), db.StatusExpired, "native", 1, 0, 0},
		{start.Add(time.Hour), db.StatusPending, "native", 42, 0, 0},
		{end, db.StatusPaid, "native", 88888, 0, 888},
	}
	for i, item := range orders {
		order := &db.PaymentOrder{
			ID: fmt.Sprintf("dashboard-%d", i), OrderNo: fmt.Sprintf("PAY-DASHBOARD-%d", i),
			Amount: item.amount, ProductName: "dashboard test", PayMethod: item.channel,
			Status: item.status, RefundedAmount: item.refunded, Fee: item.fee,
			ExpireAt: item.at.Add(time.Hour), GroupID: &groupID,
		}
		insertOrder(t, store, order)
		if err := store.Orders.Update(order.OrderNo, map[string]any{"createdAt": item.at}); err != nil {
			t.Fatal(err)
		}
	}
	for _, status := range []int{db.NotifyPending, db.NotifyPending, db.NotifySuccess, db.NotifyFailed, db.NotifyExhausted} {
		if err := store.Notifies.Insert(db.InsertNotifyInput{URL: "https://example.com/notify", Body: "{}", Signature: "{}", Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	seedAPIKey(t, store, groupID, "active-key")
	disabledKey := seedAPIKey(t, store, groupID, "disabled-key")
	if err := store.ApiKeys.Update(disabledKey, map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	disabledGroup, err := store.Groups.Insert("disabled", false)
	if err != nil {
		t.Fatal(err)
	}
	seedAPIKey(t, store, disabledGroup.ID, "active-key-in-disabled-group")

	out, err := service.dashboardAt(7, 480, now)
	if err != nil {
		t.Fatal(err)
	}
	if out.DateFrom != "2026-10-26" || out.DateTo != "2026-11-01" {
		t.Fatalf("calendar window: %s..%s", out.DateFrom, out.DateTo)
	}
	if out.GeneratedAt != "2026-10-31T17:23:00.000Z" {
		t.Fatalf("generatedAt = %q", out.GeneratedAt)
	}
	if out.Summary.Amount != 30.54 || out.Summary.TotalCount != 7 || out.Summary.SuccessCount != 4 ||
		out.Summary.RefundedAmount != 6.21 || out.Summary.Fee != 0.46 ||
		out.Summary.SuccessRate == nil || *out.Summary.SuccessRate != float64(4)/7 {
		t.Fatalf("summary: %#v", out.Summary)
	}
	if out.Previous.TotalCount != 2 || out.Previous.SuccessCount != 2 || out.Previous.Amount != 3.11 || out.Previous.Fee != 0.03 {
		t.Fatalf("previous: %#v", out.Previous)
	}
	if len(out.Trend) != 7 || out.Trend[0].Date != "2026-10-26" || out.Trend[0].TotalCount != 5 ||
		out.Trend[0].Amount != 15.55 || out.Trend[1].TotalCount != 0 || out.Trend[2].Amount != 5 ||
		out.Trend[6].Date != "2026-11-01" || out.Trend[6].Amount != 9.99 {
		t.Fatalf("trend: %#v", out.Trend)
	}
	if len(out.Channels) != 3 || out.Channels[0].PayMethod != "native" || out.Channels[0].TotalCount != 5 ||
		out.Channels[0].SuccessCount != 2 || out.Channels[0].RefundedAmount != 0.21 ||
		out.Channels[1].PayMethod != "paypal" || out.Channels[2].PayMethod != "alipay" ||
		out.Channels[2].SuccessCount != 1 || out.Channels[2].RefundedAmount != 5 {
		t.Fatalf("channels: %#v", out.Channels)
	}
	wantTasks := db.DashboardTasks{PendingOrders: 2, RefundingOrders: 1, PendingNotifications: 2,
		FailedNotifications: 1, ExhaustedNotifications: 1, ActiveGroups: 1, ActiveKeys: 1}
	if out.Tasks != wantTasks {
		t.Fatalf("tasks: %#v, want %#v", out.Tasks, wantTasks)
	}
	// 最近订单不受统计区间影响：11 笔里给最新 8 笔，最新的那笔在区间外（end 当天）。
	if len(out.RecentOrders) != db.DashboardRecentLimit ||
		out.RecentOrders[0].OrderNo != "PAY-DASHBOARD-10" ||
		out.RecentOrders[1].OrderNo != "PAY-DASHBOARD-6" ||
		out.RecentOrders[0].Status != "SUCCESS" || out.RecentOrders[0].Amount != 888.88 ||
		out.RecentOrders[0].CreatedAt != "2026-11-01T16:00:00.000Z" {
		t.Fatalf("recent orders: %#v", out.RecentOrders)
	}
}

func TestDashboardEmptyWindowsAndOffsets(t *testing.T) {
	service := NewAdminService(newTestStore(t))
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		days, offset int
		dateTo       string
	}{
		{7, 0, "2026-10-31"}, {30, 840, "2026-11-01"}, {7, -720, "2026-10-31"},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.days, tc.offset), func(t *testing.T) {
			out, err := service.dashboardAt(tc.days, tc.offset, now)
			if err != nil {
				t.Fatal(err)
			}
			if out.DateTo != tc.dateTo || len(out.Trend) != tc.days || out.Summary.SuccessRate != nil ||
				out.Previous.SuccessRate != nil || out.Channels == nil || len(out.Channels) != 0 ||
				out.RecentOrders == nil || len(out.RecentOrders) != 0 {
				t.Fatalf("empty dashboard: %#v", out)
			}
			for i, day := range out.Trend {
				if day.TotalCount != 0 || day.Amount != 0 || day.SuccessCount != 0 {
					t.Fatalf("nonempty day %d: %#v", i, day)
				}
				if i > 0 {
					previous, err := time.Parse("2006-01-02", out.Trend[i-1].Date)
					if err != nil || previous.AddDate(0, 0, 1).Format("2006-01-02") != day.Date {
						t.Fatalf("noncontiguous dates at %d", i)
					}
				}
			}
		})
	}
}

func TestDashboardRejectsInvalidWindowsAndDatabaseFailures(t *testing.T) {
	store := newTestStore(t)
	service := NewAdminService(store)
	for _, tc := range [][2]int{{0, 0}, {8, 0}, {30, -721}, {7, 841}} {
		_, err := service.GetDashboard(tc[0], tc[1])
		requireErrStatus(t, err, 400)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := service.GetDashboard(7, 480); err == nil || out != nil {
		t.Fatalf("closed database returned dashboard: out=%v err=%v", out, err)
	}
}
