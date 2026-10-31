package routes

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

func TestUsageLogsAdministrationFiltersSummaryAndDetails(t *testing.T) {
	h := newHarness(t, nil)
	key, err := h.store.ApiKeys.FindByKey(testAPIKey)
	if err != nil || key == nil {
		t.Fatalf("seed credential missing: %v", err)
	}
	start := time.Date(2026, 10, 30, 16, 0, 0, 0, time.UTC)
	logs := []*db.UsageLog{
		{RequestID: "request-success", Method: "POST", Path: "/v1/api/gateway/pay", StatusCode: 200,
			DurationMs: 10, GroupID: &h.groupID, GroupName: "历史项目", APIKeyID: &key.ID, APIKeyName: "CI",
			KeyPrefix: key.KeyPrefix, KeySuffix: key.KeySuffix, ClientIP: "8.8.8.8",
			ClientLocation: `{"country":"美国","network":"public"}`, OrderNo: "PAYUSAGEFIXTURE", CreatedAt: start},
		{RequestID: "request-failure", Method: "GET", Path: "/v1/api/gateway/query", StatusCode: 404,
			DurationMs: 20, GroupID: &h.groupID, GroupName: "历史项目", APIKeyID: &key.ID, APIKeyName: "CI",
			KeyPrefix: key.KeyPrefix, KeySuffix: key.KeySuffix, ErrorMessage: "订单不存在", CreatedAt: start.Add(23 * time.Hour)},
		{RequestID: "next-local-day", Method: "GET", Path: "/v1/api/gateway/query", StatusCode: 401,
			DurationMs: 30, ErrorMessage: "无效的 API Key", CreatedAt: start.Add(24 * time.Hour)},
	}
	for _, item := range logs {
		if err := h.store.UsageLogs.Insert(item); err != nil {
			t.Fatal(err)
		}
	}
	base := "/v1/api/admin/usage-logs?dateFrom=2026-10-31&dateTo=2026-10-31&utcOffset=480"
	for _, headers := range []map[string]string{nil, h.groupAuth} {
		denied := h.do(t, reqSpec{Method: "GET", Path: base, Headers: headers})
		eq(t, "usage logs require administrator", denied.Status, http.StatusUnauthorized)
	}
	listed := h.do(t, reqSpec{Method: "GET", Path: base + "&pageSize=1", Headers: h.auth})
	eq(t, "list status", listed.Status, http.StatusOK)
	data := listed.data()
	eq(t, "local-day total", num(t, data["total"], "total"), 2)
	eq(t, "one-page rows", len(data["items"].([]any)), 1)
	summary := data["summary"].(map[string]any)
	eq(t, "summary total includes all pages", num(t, summary["totalCount"], "summary total"), 2)
	eq(t, "summary success", num(t, summary["successCount"], "success"), 1)
	eq(t, "summary failure", num(t, summary["failureCount"], "failure"), 1)
	eq(t, "summary mean includes all pages", num(t, summary["averageDurationMs"], "mean"), 15)
	filtered := h.do(t, reqSpec{Method: "GET", Path: base + fmt.Sprintf("&apiKeyId=%d&status=failure&method=GET", key.ID), Headers: h.auth})
	eq(t, "filtered status", filtered.Status, http.StatusOK)
	eq(t, "filtered total", num(t, filtered.data()["total"], "filtered total"), 1)
	row := filtered.data()["items"].([]any)[0].(map[string]any)
	eq(t, "filtered request", row["requestId"], any("request-failure"))
	detail := h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/usage-logs/%d", logs[0].ID), Headers: h.auth})
	eq(t, "detail status", detail.Status, http.StatusOK)
	eq(t, "detail location", detail.data()["clientLocation"].(map[string]any)["country"], any("美国"))
	eq(t, "detail historical project", detail.data()["groupName"], any("历史项目"))
	eq(t, "detail order", detail.data()["orderNo"], any("PAYUSAGEFIXTURE"))
	lacksSubstr(t, detail.Raw, testAPIKey, "usage log detail")
	lacksSubstr(t, listed.Raw, key.KeyHash, "usage log list")
	missing := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/usage-logs/999999", Headers: h.auth})
	eq(t, "missing usage log", missing.Status, http.StatusNotFound)
	_, total, _, err := h.store.UsageLogs.Search(db.UsageSearchOpts{Limit: 10})
	if err != nil || total != 3 {
		t.Fatalf("reading log administration created recursive usage records: total=%d error=%v", total, err)
	}
}

func TestUsageLogsRejectInvalidFilters(t *testing.T) {
	h := newHarness(t, nil)
	for _, query := range []string{"groupId=1.5", "apiKeyId=-1", "utcOffset=NaN", "dateFrom=2026-02-31", "dateFrom=2026-11-01&dateTo=2026-10-31", "status=unknown", "method=invalid"} {
		t.Run(query, func(t *testing.T) {
			result := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/usage-logs?" + query, Headers: h.auth})
			eq(t, "invalid filter", result.Status, http.StatusBadRequest)
		})
	}
}
