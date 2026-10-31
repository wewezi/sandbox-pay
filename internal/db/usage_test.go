package db

import (
	"database/sql"
	"math"
	"strconv"
	"testing"
	"time"
)

func insertUsage(t *testing.T, s *Store, log UsageLog) *UsageLog {
	t.Helper()
	if log.Method == "" {
		log.Method = "POST"
	}
	if log.Path == "" {
		log.Path = "/v1/api/gateway/create"
	}
	if log.StatusCode == 0 {
		log.StatusCode = 200
	}
	if err := s.UsageLogs.Insert(&log); err != nil {
		t.Fatal(err)
	}
	return &log
}

func TestUsageLogRoundTripAndHistorySurvivesCredentialDeletion(t *testing.T) {
	s := newRepoStore(t)
	groupID := seedGroup(t, s)
	keyID := seedKey(t, s, groupID, "sp_usage_secret")
	log := insertUsage(t, s, UsageLog{
		RequestID: "request-one", DurationMs: 12.345, GroupID: &groupID,
		GroupName: "original project", APIKeyID: &keyID, APIKeyName: "original token",
		KeyPrefix: "sp_us", KeySuffix: "cret", ClientIP: "2001:db8::1",
		ClientLocation: `{"network":"reserved"}`, OrderNo: "PAY-HISTORICAL",
	})
	if log.ID < 1 || log.CreatedAt.IsZero() {
		t.Fatalf("Insert did not fill receipt: %+v", log)
	}
	if _, err := s.db.Exec(`UPDATE "groups" SET "name" = 'renamed' WHERE "id" = ?`, groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM "groups" WHERE "id" = ?`, groupID); err != nil {
		t.Fatal(err)
	}
	stored, err := s.UsageLogs.FindByID(log.ID)
	if err != nil || stored == nil {
		t.Fatalf("historical log disappeared: %+v, %v", stored, err)
	}
	if stored.GroupID == nil || *stored.GroupID != groupID || stored.APIKeyID == nil || *stored.APIKeyID != keyID ||
		stored.GroupName != "original project" || stored.APIKeyName != "original token" ||
		stored.ClientIP != log.ClientIP || stored.ClientLocation != log.ClientLocation ||
		stored.OrderNo != log.OrderNo || stored.RequestID != log.RequestID || stored.DurationMs != log.DurationMs {
		t.Fatalf("historical snapshot changed: %+v", stored)
	}
	if diff := log.CreatedAt.Sub(stored.CreatedAt); diff < 0 || diff >= time.Millisecond {
		t.Fatalf("timestamp lost more than sub-millisecond precision: %v", diff)
	}
	missing, err := s.UsageLogs.FindByID(log.ID + 1)
	if err != nil || missing != nil {
		t.Fatalf("FindByID missing = %+v, %v", missing, err)
	}
}

func TestUsageSearchFiltersAndSummaryCoverEntireCohort(t *testing.T) {
	s := newRepoStore(t)
	start := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	logs := []*UsageLog{
		insertUsage(t, s, UsageLog{RequestID: "REQ-One", StatusCode: 200, DurationMs: 10, GroupID: i64Ptr(1), APIKeyID: i64Ptr(10), CreatedAt: start, ClientIP: "1.2.3.4", OrderNo: "PAY-001"}),
		insertUsage(t, s, UsageLog{RequestID: "req_two%escaped", StatusCode: 422, DurationMs: 30, GroupID: i64Ptr(1), APIKeyID: i64Ptr(10), CreatedAt: start}),
		insertUsage(t, s, UsageLog{RequestID: "req-three", Method: "GET", StatusCode: 302, DurationMs: 20, GroupID: i64Ptr(1), APIKeyID: i64Ptr(11), CreatedAt: start.Add(time.Hour), Path: "/v1/api/gateway/query"}),
		insertUsage(t, s, UsageLog{RequestID: "req-four", StatusCode: 500, DurationMs: 40, GroupID: i64Ptr(2), APIKeyID: i64Ptr(12), CreatedAt: start.Add(2 * time.Hour)}),
		insertUsage(t, s, UsageLog{RequestID: "unauthenticated", StatusCode: 401, DurationMs: 50, CreatedAt: start.Add(3 * time.Hour)}),
	}
	items, total, summary, err := s.UsageLogs.Search(UsageSearchOpts{GroupID: i64Ptr(1), Limit: 1, Offset: 1})
	if err != nil || total != 3 || len(items) != 1 || items[0].ID != logs[1].ID || summary.TotalCount != 3 ||
		summary.SuccessCount != 2 || summary.FailureCount != 1 || summary.AverageDurationMs != 20 {
		t.Fatalf("filtered page/summary = %+v, %d, %+v, %v", items, total, summary, err)
	}
	status500 := 500
	for _, tc := range []struct {
		name                    string
		opts                    UsageSearchOpts
		total, success, failure int64
		average                 float64
	}{
		{"all", UsageSearchOpts{}, 5, 2, 3, 30},
		{"token", UsageSearchOpts{APIKeyID: i64Ptr(10)}, 2, 1, 1, 20},
		{"failed", UsageSearchOpts{Status: "failure"}, 3, 0, 3, 40},
		{"successful", UsageSearchOpts{Status: "success"}, 2, 2, 0, 15},
		{"method", UsageSearchOpts{Method: "get"}, 1, 1, 0, 20},
		{"http status", UsageSearchOpts{StatusCode: &status500}, 1, 0, 1, 40},
		{"IP", UsageSearchOpts{Keyword: "1.2.3.4"}, 1, 1, 0, 10},
		{"path", UsageSearchOpts{Keyword: "QUERY"}, 1, 1, 0, 20},
		{"request ID", UsageSearchOpts{Keyword: "req-one"}, 1, 1, 0, 10},
		{"order number", UsageSearchOpts{Keyword: "pay-001"}, 1, 1, 0, 10},
		{"literal percent", UsageSearchOpts{Keyword: "%"}, 1, 0, 1, 30},
		{"literal underscore", UsageSearchOpts{Keyword: "_"}, 1, 0, 1, 30},
		{"SQL text", UsageSearchOpts{Keyword: "' OR 1=1 --"}, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.Limit = 20
			items, total, summary, err := s.UsageLogs.Search(tc.opts)
			if err != nil || total != tc.total || int64(len(items)) != total || summary.TotalCount != total ||
				summary.SuccessCount != tc.success || summary.FailureCount != tc.failure || summary.AverageDurationMs != tc.average {
				t.Fatalf("Search = %+v, %d, %+v, %v", items, total, summary, err)
			}
			if items == nil {
				t.Fatal("empty collection should remain an array")
			}
		})
	}
}

func TestUsageSearchLocalDayBoundaries(t *testing.T) {
	for _, offset := range []int{0, 480, -300} {
		t.Run(strconv.Itoa(offset), func(t *testing.T) {
			s := newRepoStore(t)
			start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC).Add(-time.Duration(offset) * time.Minute)
			end := start.AddDate(0, 0, 1)
			for _, created := range []time.Time{start.Add(-time.Millisecond), start, end.Add(-time.Millisecond), end} {
				insertUsage(t, s, UsageLog{CreatedAt: created, DurationMs: 10})
			}
			items, total, summary, err := s.UsageLogs.Search(UsageSearchOpts{
				DateFrom: "2026-10-31", DateTo: "2026-10-31", UTCOffset: offset, Limit: 20,
			})
			if err != nil || total != 2 || len(items) != 2 || summary.TotalCount != 2 || summary.AverageDurationMs != 10 {
				t.Fatalf("local calendar boundary = %+v, %d, %+v, %v", items, total, summary, err)
			}
			if !items[0].CreatedAt.Equal(end.Add(-time.Millisecond)) || !items[1].CreatedAt.Equal(start) {
				t.Fatalf("included a neighboring calendar date: %+v", items)
			}
		})
	}
}

func TestUsageInsertRejectsInvalidMetrics(t *testing.T) {
	s := newRepoStore(t)
	for _, log := range []*UsageLog{nil, {StatusCode: 0}, {StatusCode: 600},
		{StatusCode: 200, DurationMs: -1}, {StatusCode: 200, DurationMs: math.NaN()}, {StatusCode: 200, DurationMs: math.Inf(1)}} {
		if err := s.UsageLogs.Insert(log); err == nil {
			t.Fatalf("accepted invalid log %+v", log)
		}
	}
}

func version3UsageDatabase(t *testing.T) string {
	t.Helper()
	path := version2CredentialsDatabase(t)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := migrateVersion3(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUsageMigrationFromEverySupportedVersionStartsWithoutFabricatedHistory(t *testing.T) {
	for _, tc := range []struct {
		version string
		fixture func(*testing.T) string
	}{
		{"0", func(t *testing.T) string { return legacyDatabase(t, false) }},
		{"1", version1Database}, {"2", version2CredentialsDatabase}, {"3", version3UsageDatabase},
	} {
		t.Run(tc.version, func(t *testing.T) {
			path := tc.fixture(t)
			s, err := Open(path, "")
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var version int
			if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
				t.Fatalf("version = %d, %v", version, err)
			}
			items, total, summary, err := s.UsageLogs.Search(UsageSearchOpts{Limit: 20})
			if err != nil || len(items) != 0 || total != 0 || summary != (UsageSummary{}) {
				t.Fatalf("migration fabricated usage history: %+v, %d, %+v, %v", items, total, summary, err)
			}
			order, err := s.Orders.FindByOrderNo("A", false)
			if err != nil || order == nil || order.Amount != 100 || order.ProductName != "old item" {
				t.Fatalf("historical order changed: %+v, %v", order, err)
			}
			if tc.version != "0" {
				notifications, err := s.Notifies.ListByOrderNo("A")
				if err != nil || len(notifications) != 1 || notifications[0].Signature != "old-signature" {
					t.Fatalf("callback history changed: %+v, %v", notifications, err)
				}
			}
			log := insertUsage(t, s, UsageLog{RequestID: "after-migration"})
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			second, err := Open(path, "")
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			if stored, err := second.UsageLogs.FindByID(log.ID); err != nil || stored == nil || stored.RequestID != log.RequestID {
				t.Fatalf("reopening erased request history: %+v, %v", stored, err)
			}
		})
	}
}
