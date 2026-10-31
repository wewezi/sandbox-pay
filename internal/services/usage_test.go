package services

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

func TestUsageServiceQueriesMetadataAndSummary(t *testing.T) {
	store := newTestStore(t)
	service := NewUsageService(store)
	groupID, keyID := int64(99), int64(199)
	created := time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		log := db.UsageLog{
			RequestID: "safe-request-" + strconv.Itoa(i), Method: "POST", Path: "/v1/api/gateway/create",
			StatusCode: 200, DurationMs: float64(i+1) * 10, GroupID: &groupID, GroupName: "original project",
			APIKeyID: &keyID, APIKeyName: "original token", KeyPrefix: "sp_pr", KeySuffix: "7890",
			ClientIP: "1.2.3.4", ClientLocation: `{"network":"public","country":"中国","region":"浙江","city":"杭州","isp":"电信"}`,
			OrderNo: "PAY-" + strconv.Itoa(i), CreatedAt: created.Add(time.Duration(i) * time.Minute),
		}
		if i == 1 {
			log.StatusCode, log.ErrorMessage = 422, "金额必须大于0"
		}
		if err := store.UsageLogs.Insert(&log); err != nil {
			t.Fatal(err)
		}
	}
	page, err := service.GetLogs(UsageLogOpts{Page: "2", PageSize: "1", GroupID: &groupID,
		APIKeyID: &keyID, Method: "post", DateFrom: "2026-11-01", DateTo: "2026-11-01", UTCOffset: 480})
	if err != nil || page == nil || page.Page != 2 || page.PageSize != 1 || page.Total != 3 || len(page.Items) != 1 ||
		page.Summary.TotalCount != 3 || page.Summary.SuccessCount != 2 || page.Summary.FailureCount != 1 || page.Summary.AverageDurationMs != 20 {
		t.Fatalf("GetLogs = %+v, %v", page, err)
	}
	item := page.Items[0]
	if item.RequestID != "safe-request-1" || item.ClientLocation == nil || item.ClientLocation.City != "杭州" || item.StatusCode != 422 || item.ErrorMessage != "金额必须大于0" {
		t.Fatalf("metadata lost: %+v", item)
	}
	detail, err := service.GetLog(item.ID)
	if err != nil || detail == nil || detail.OrderNo != item.OrderNo {
		t.Fatalf("GetLog = %+v, %v", detail, err)
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"key":`, `"keyHash":`, `"signingSecret":`, `"webhookSecret":`, `"authorization":`, `"body":`, `"headers":`} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("usage DTO contains credential/body field %s", forbidden)
		}
	}
}

func TestUsageServiceInvalidFiltersReturnBadRequest(t *testing.T) {
	service := NewUsageService(newTestStore(t))
	negativeID := int64(-1)
	for _, opts := range []UsageLogOpts{
		{DateFrom: "2026-02-30"}, {DateTo: "2026-10-31T00:00:00Z"}, {DateFrom: "2026-1-1"},
		{DateFrom: "2026-11-01", DateTo: "2026-10-31"}, {UTCOffset: 841}, {UTCOffset: -841},
		{GroupID: &negativeID}, {APIKeyID: &negativeID}, {Method: "INVALID"}, {Status: "paid"},
		{Keyword: strings.Repeat("字", 201)}, {Page: "9223372036854775807", PageSize: "200"},
	} {
		_, err := service.GetLogs(opts)
		requireErrStatus(t, err, 400)
	}
	_, err := service.GetLog(0)
	requireErrStatus(t, err, 400)
	_, err = service.GetLog(1)
	requireErrStatus(t, err, 404)
}

func TestUsageServiceEmptyArrayPaginationAndMalformedLocation(t *testing.T) {
	store := newTestStore(t)
	service := NewUsageService(store)
	page, err := service.GetLogs(UsageLogOpts{Page: "garbage", PageSize: "999"})
	if err != nil || page.Items == nil || len(page.Items) != 0 || page.Page != 1 || page.PageSize != 200 || page.Summary != (db.UsageSummary{}) {
		t.Fatalf("empty page = %+v, %v", page, err)
	}
	for _, raw := range []string{"", "null", "invalid-json", "[]"} {
		log := db.UsageLog{Method: "POST", Path: "/test", StatusCode: 401, ClientLocation: raw}
		if err := store.UsageLogs.Insert(&log); err != nil {
			t.Fatal(err)
		}
		detail, err := service.GetLog(log.ID)
		if err != nil || detail == nil || detail.ClientLocation != nil {
			t.Fatalf("malformed/missing snapshot fabricated location: %+v, %v", detail, err)
		}
	}
}

func TestUsageHistorySurvivesOrderReset(t *testing.T) {
	store := newTestStore(t)
	groupID := seedGroup1(t, store)
	order := &db.PaymentOrder{ID: "usage-reset-id", OrderNo: "USAGE-RESET", Amount: 100,
		ProductName: "usage reset", PayMethod: "native", GroupID: &groupID, ExpireAt: time.Now().Add(time.Hour)}
	insertOrder(t, store, order)
	log := db.UsageLog{RequestID: "before-reset", Method: "POST", Path: "/v1/api/gateway/create",
		StatusCode: 200, OrderNo: order.OrderNo, GroupID: &groupID, GroupName: "original project"}
	if err := store.UsageLogs.Insert(&log); err != nil {
		t.Fatal(err)
	}
	result, err := NewAdminService(store).ResetData("test", nil)
	if err != nil || result.Deleted.Orders != 1 {
		t.Fatalf("ResetData = %+v, %v", result, err)
	}
	stored, err := NewUsageService(store).GetLog(log.ID)
	if err != nil || stored == nil || stored.OrderNo != order.OrderNo || stored.RequestID != "before-reset" {
		t.Fatalf("order reset erased usage history: %+v, %v", stored, err)
	}
}
