package db

import (
	"strconv"
	"testing"
	"time"
)

func TestOrderSearchCalendarDayBoundaries(t *testing.T) {
	for _, offset := range []int{0, 480, -300} {
		t.Run(strconv.Itoa(offset), func(t *testing.T) {
			s := newRepoStore(t)
			seedGroup(t, s)
			start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC).Add(-time.Duration(offset) * time.Minute)
			end := start.AddDate(0, 0, 1)
			times := []time.Time{start.Add(-time.Millisecond), start, end.Add(-time.Millisecond), end}
			orders := make([]*PaymentOrder, len(times))
			for i, created := range times {
				orders[i] = makeOrder()
				insertOrder(t, s, orders[i])
				if err := s.Orders.Update(orders[i].OrderNo, map[string]any{"createdAt": created}); err != nil {
					t.Fatalf("set createdAt: %v", err)
				}
			}
			items, total, err := s.Orders.Search(SearchOpts{
				DateFrom: "2026-10-31", DateTo: "2026-10-31", UTCOffset: offset, Limit: 20,
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if total != 2 || len(items) != 2 {
				t.Fatalf("Search total=%d items=%d, want both day-boundary orders", total, len(items))
			}
			if items[0].OrderNo != orders[2].OrderNo || items[1].OrderNo != orders[1].OrderNo {
				t.Fatalf("Search included an order from an adjacent local date: %+v", items)
			}
		})
	}
}

func TestOrderSearchTimestampBoundsRemainUTC(t *testing.T) {
	s := newRepoStore(t)
	seedGroup(t, s)
	created := time.Date(2026, 10, 31, 23, 59, 59, 999_000_000, time.UTC)
	o := makeOrder()
	insertOrder(t, s, o)
	if err := s.Orders.Update(o.OrderNo, map[string]any{"createdAt": created}); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.Orders.Search(SearchOpts{
		DateFrom: toSQL(created), DateTo: toSQL(created), UTCOffset: 480, Limit: 20,
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].OrderNo != o.OrderNo {
		t.Fatalf("explicit timestamp Search = (%v, %d, %v), want exact UTC order", items, total, err)
	}
}

func TestOrderSearchHistoricalTimestampsMatchDashboard(t *testing.T) {
	for _, offset := range []int{0, 480} {
		t.Run(strconv.Itoa(offset), func(t *testing.T) {
			s := newRepoStore(t)
			seedGroup(t, s)
			start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC).Add(-time.Duration(offset) * time.Minute)
			end := start.AddDate(0, 0, 1)
			local := time.FixedZone("local", offset*60)
			cases := []struct {
				createdAt string
				included  bool
			}{
				{start.Add(-time.Second).Format("2006-01-02 15:04:05"), false},
				{start.Format("2006-01-02 15:04:05"), true},
				{start.Format(time.RFC3339), true},
				{start.In(local).Format(time.RFC3339), true},
				{start.Add(12 * time.Hour).Format(time.RFC3339), true},
				{end.Add(-time.Millisecond).Format(sqlTimeLayout), true},
				{end.Add(-time.Millisecond).Format(time.RFC3339Nano), true},
				{end.Format("2006-01-02 15:04:05"), false},
				{end.Format(time.RFC3339), false},
				{end.In(local).Format(time.RFC3339), false},
			}
			wanted := map[string]bool{}
			for _, tc := range cases {
				o := makeOrder()
				o.Status = StatusPaid
				o.Amount = 123
				insertOrder(t, s, o)
				if _, err := s.db.Exec(`UPDATE "payment_orders" SET "createdAt" = ? WHERE "orderNo" = ?`, tc.createdAt, o.OrderNo); err != nil {
					t.Fatalf("set historical timestamp: %v", err)
				}
				if tc.included {
					wanted[o.OrderNo] = true
				}
			}
			items, total, err := s.Orders.Search(SearchOpts{
				DateFrom: "2026-10-31", DateTo: "2026-10-31", UTCOffset: offset, Limit: 20,
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if total != int64(len(wanted)) || len(items) != len(wanted) {
				t.Fatalf("Search total=%d items=%d, want %d historical orders", total, len(items), len(wanted))
			}
			for _, item := range items {
				if !wanted[item.OrderNo] {
					t.Errorf("Search included neighboring local date: %s at %s", item.OrderNo, item.CreatedAt)
				}
			}
			snapshot, err := s.Dashboard(start.AddDate(0, 0, -1), start, end, offset)
			if err != nil {
				t.Fatalf("Dashboard: %v", err)
			}
			if snapshot.Summary.TotalCount != total || snapshot.Summary.AmountCents != total*123 {
				t.Fatalf("dashboard cohort and order drilldown differ: summary=%+v total=%d", snapshot.Summary, total)
			}
		})
	}
}
