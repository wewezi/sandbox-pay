package db

import (
	"testing"
	"time"
)

func TestDashboardAggregatesHistoricalTimestampFormats(t *testing.T) {
	store := newRepoStore(t)
	seedGroup(t, store)
	start := time.Date(2026, 10, 25, 16, 0, 0, 0, time.UTC)
	for _, createdAt := range []string{
		"2026-10-25 16:00:00.000", "2026-10-25 16:00:00", "2026-10-25T16:00:00.000Z",
		"2026-10-25T16:00:00Z", "2026-10-26T00:00:00+08:00",
	} {
		order := makeOrder()
		order.Status = StatusPaid
		order.Amount = 123
		insertOrder(t, store, order)
		if _, err := store.db.Exec(`UPDATE "payment_orders" SET "createdAt" = ? WHERE "orderNo" = ?`, createdAt, order.OrderNo); err != nil {
			t.Fatal(err)
		}
	}
	out, err := store.Dashboard(start.AddDate(0, 0, -7), start, start.AddDate(0, 0, 7), 480)
	if err != nil {
		t.Fatal(err)
	}
	if out.Summary.TotalCount != 5 || out.Summary.AmountCents != 615 || len(out.Trend) != 1 ||
		out.Trend[0].Date != "2026-10-26" || out.Trend[0].TotalCount != 5 {
		t.Fatalf("historical timestamp aggregation: %#v", out)
	}
}

func TestDashboardRecentOrdersNewestFirst(t *testing.T) {
	store := newRepoStore(t)
	seedGroup(t, store)
	base := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	var want []string
	for i := 0; i < DashboardRecentLimit+2; i++ {
		order := makeOrder()
		order.Status = StatusPaid
		order.Amount = int64(100 + i)
		insertOrder(t, store, order)
		if _, err := store.db.Exec(`UPDATE "payment_orders" SET "createdAt" = ? WHERE "orderNo" = ?`,
			toSQL(base.Add(time.Duration(i)*time.Minute)), order.OrderNo); err != nil {
			t.Fatal(err)
		}
		want = append([]string{order.OrderNo}, want...)
	}
	out, err := store.Dashboard(base.AddDate(0, 0, -1), base, base.AddDate(0, 0, 1), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Recent) != DashboardRecentLimit {
		t.Fatalf("recent 条数 = %d, want %d: %#v", len(out.Recent), DashboardRecentLimit, out.Recent)
	}
	for i, item := range out.Recent {
		if item.OrderNo != want[i] {
			t.Fatalf("recent[%d] = %s, want %s", i, item.OrderNo, want[i])
		}
	}
	newest := out.Recent[0]
	if newest.AmountCents != int64(100+DashboardRecentLimit+1) || newest.PayMethod != "native" ||
		newest.Status != StatusPaid ||
		!newest.CreatedAt.Equal(base.Add(time.Duration(DashboardRecentLimit+1)*time.Minute)) {
		t.Fatalf("newest: %#v", newest)
	}
}

func TestDashboardReturnsQueryFailure(t *testing.T) {
	store := newRepoStore(t)
	if _, err := store.db.Exec(`DROP TABLE "notify_queues"`); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 25, 16, 0, 0, 0, time.UTC)
	if out, err := store.Dashboard(start.AddDate(0, 0, -7), start, start.AddDate(0, 0, 7), 480); err == nil || out != nil {
		t.Fatalf("missing table returned dashboard: out=%v err=%v", out, err)
	}
}
