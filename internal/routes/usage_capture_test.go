package routes

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"sandbox-pay/internal/db"
)

func usageCaptureRecords(t *testing.T, h *harness) []*db.UsageLog {
	t.Helper()
	items, total, _, err := h.store.UsageLogs.Search(db.UsageSearchOpts{Limit: 1000})
	if err != nil || total != int64(len(items)) {
		t.Fatalf("read usage records: count=%d returned=%d error=%v", total, len(items), err)
	}
	return items
}

func TestGatewayUsageLogsEveryTokenRouteAndBusinessOrderAssociation(t *testing.T) {
	h := newHarness(t, nil)
	for _, path := range []string{
		"/v1/api/gateway/pay", "/v1/api/gateway/native/pay", "/v1/api/gateway/alipay/pay", "/v1/api/gateway/paypal/pay",
	} {
		body := map[string]any{"amount": 1.23, "productName": "usage capture"}
		if path == "/v1/api/gateway/pay" {
			body["payMethod"] = "native"
		}
		r := h.do(t, reqSpec{Method: "POST", Path: path, Headers: h.groupAuth, Body: body})
		items := usageCaptureRecords(t, h)
		log := items[0]
		if log.Path != path || log.StatusCode != r.Status || log.OrderNo == "" || log.APIKeyID == nil || log.GroupID == nil || *log.GroupID != h.groupID {
			t.Fatalf("API route result lacks completed usage metadata: route=%s response=%d log=%+v", path, r.Status, log)
		}
		order, err := h.store.Orders.FindByOrderNo(log.OrderNo, false)
		if err != nil || order == nil || order.APIKeyID == nil || *log.APIKeyID != *order.APIKeyID {
			t.Fatalf("created order association was lost: route=%s log=%+v error=%v", path, log, err)
		}
	}
	created := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Headers: h.groupAuth,
		Body: map[string]any{"amount": 1.23, "productName": "query/refund association", "payMethod": "native"}})
	orderNo := cashierOrderNo(t, created)
	queried := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + orderNo, Headers: h.groupAuth})
	eq(t, "query status", queried.Status, http.StatusOK)
	if _, err := h.c.Payment.MarkPaid(orderNo, nil, 123, "CNY"); err != nil {
		t.Fatal(err)
	}
	refunded := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/refund", Headers: h.groupAuth,
		Body: map[string]any{"orderNo": orderNo, "amount": 1.23, "reason": "usage fixture"}})
	eq(t, "refund status", refunded.Status, http.StatusOK)
	items := usageCaptureRecords(t, h)
	eq(t, "log count", len(items), 7)
	if items[0].OrderNo != orderNo || items[1].OrderNo != orderNo || items[0].Path != "/v1/api/gateway/refund" || items[1].Path != "/v1/api/gateway/query" {
		t.Fatalf("query/refund association missing: refund=%+v query=%+v", items[0], items[1])
	}
}

func TestGatewayUsageLogsAuthenticationFailuresAndDisabledTokenMetadata(t *testing.T) {
	h := newHarness(t, nil)
	for _, headers := range []map[string]string{
		nil, {"Authorization": "Bearer never-issued-token"},
	} {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=NOPE", Headers: headers})
		eq(t, "rejected status", r.Status, http.StatusUnauthorized)
		log := usageCaptureRecords(t, h)[0]
		if log.StatusCode != http.StatusUnauthorized || log.APIKeyID != nil || log.GroupID != nil || log.ErrorMessage == "" {
			t.Fatalf("unknown credential was attributed or not logged: %+v", log)
		}
	}
	key, err := h.store.ApiKeys.FindByKey(testAPIKey)
	if err != nil || key == nil {
		t.Fatal(err)
	}
	if err := h.store.ApiKeys.Update(key.ID, map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=NOPE", Headers: h.groupAuth})
	eq(t, "disabled token status", r.Status, http.StatusUnauthorized)
	log := usageCaptureRecords(t, h)[0]
	if log.APIKeyID == nil || *log.APIKeyID != key.ID || log.GroupID == nil || *log.GroupID != h.groupID || log.APIKeyName != key.Name {
		t.Fatalf("known disabled token lost metadata: %+v", log)
	}
	after, err := h.store.ApiKeys.FindByID(key.ID)
	if err != nil || !reflect.DeepEqual(after.LastUsedAt, key.LastUsedAt) {
		t.Fatalf("metadata lookup touched disabled token: %+v error=%v", after, err)
	}
}

func TestGatewayUsageCapturesRateLimitBeforeGuard(t *testing.T) {
	h := newHarness(t, map[string]string{"THROTTLE_LIMIT": "1"})
	for _, want := range []int{http.StatusNotFound, http.StatusTooManyRequests} {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=NOPE", Headers: h.groupAuth, IP: "10.71.1.1"})
		eq(t, "gateway status", r.Status, want)
	}
	items := usageCaptureRecords(t, h)
	eq(t, "recorded requests", len(items), 2)
	if items[0].StatusCode != http.StatusTooManyRequests || items[0].APIKeyID == nil || items[0].ErrorMessage == "" || items[1].StatusCode != http.StatusNotFound || items[1].OrderNo != "" {
		t.Fatalf("rate limit outside capture or attribution lost: %+v %+v", items[0], items[1])
	}
}

func TestGatewayUsageOrderAssociationRespectsExistenceAndProjectIsolation(t *testing.T) {
	h := newHarness(t, nil)
	created := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Headers: h.groupAuth,
		Body: map[string]any{"amount": 1.23, "productName": "isolated usage association", "payMethod": "native"}})
	orderNo := cashierOrderNo(t, created)
	other, err := h.c.Groups.CreateGroup("other usage project", "")
	if err != nil || len(other.Keys) == 0 {
		t.Fatalf("create other project: %v", err)
	}
	otherAuth := withHeaders(h.groupAuth, map[string]string{"Authorization": "Bearer " + other.Keys[0].Key})
	for _, tc := range []struct {
		name, orderNo string
		headers       map[string]string
	}{
		{"cross project", orderNo, otherAuth},
		{"missing order", "PAY_DOES_NOT_EXIST", h.groupAuth},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				spec := reqSpec{Method: method, Headers: tc.headers}
				if method == http.MethodGet {
					spec.Path = "/v1/api/gateway/query?orderNo=" + tc.orderNo
				} else {
					spec.Path = "/v1/api/gateway/refund"
					spec.Body = map[string]any{"orderNo": tc.orderNo, "amount": 0.01}
				}
				r := h.do(t, spec)
				eq(t, "isolated/missing order response", r.Status, http.StatusNotFound)
				log := usageCaptureRecords(t, h)[0]
				if log.OrderNo != "" || log.StatusCode != http.StatusNotFound {
					t.Fatalf("inaccessible request input became an associated order: %+v", log)
				}
			})
		}
	}
	// This order belongs to the caller but is still unpaid. The failed refund
	// retains the real order association for troubleshooting its business error.
	failed := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/refund", Headers: h.groupAuth,
		Body: map[string]any{"orderNo": orderNo, "amount": 0.01}})
	eq(t, "unpaid refund response", failed.Status, http.StatusConflict)
	log := usageCaptureRecords(t, h)[0]
	if log.OrderNo != orderNo || log.StatusCode != http.StatusConflict || log.ErrorMessage == "" {
		t.Fatalf("same-project failed refund lost verified association: %+v", log)
	}
}

func TestGatewayUsageLogsCanonicalTrustedSourceAndOfflineLocation(t *testing.T) {
	for _, tc := range []struct{ name, peer, forwarded, trusted, want string }{
		{"untrusted spoof", "8.8.8.8", "1.1.1.1", "", "8.8.8.8"},
		{"trusted proxy", "10.72.1.1", "1.1.1.1", "10.0.0.0/8", "1.1.1.1"},
		{"mapped IPv4", "::ffff:8.8.4.4", "", "", "8.8.4.4"},
		{"IPv6", "2001:4860:4860::8888", "", "", "2001:4860:4860::8888"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, map[string]string{"TRUSTED_PROXY_CIDRS": tc.trusted})
			r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=NOPE", IP: tc.peer,
				Headers: withHeaders(h.groupAuth, map[string]string{"X-Forwarded-For": tc.forwarded})})
			eq(t, "status", r.Status, http.StatusNotFound)
			log := usageCaptureRecords(t, h)[0]
			wantLocation, err := json.Marshal(h.c.Geolocation.Lookup(tc.want))
			if err != nil || log.ClientIP != tc.want || log.ClientLocation != string(wantLocation) {
				t.Fatalf("request source or location not canonical: log=%+v wantedIP=%q error=%v", log, tc.want, err)
			}
		})
	}
}

func TestGatewayUsageLogsSafeUnknownEndpointsAndExcludesAdminAndPreflight(t *testing.T) {
	h := newHarness(t, nil)
	secret := "whsec_secret-path"
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/" + secret + "?password=" + testAPIKey, Headers: h.groupAuth})
	eq(t, "unknown gateway status", r.Status, http.StatusNotFound)
	items := usageCaptureRecords(t, h)
	eq(t, "unknown endpoint recorded", len(items), 1)
	for _, path := range []string{"/v1/api/admin/stats", "/health", "/v1/api/native-pay/config", "/v1/api/gatewayish/nope"} {
		h.do(t, reqSpec{Method: "GET", Path: path, Headers: h.auth})
	}
	h.do(t, reqSpec{Method: "OPTIONS", Path: "/v1/api/gateway/pay", Headers: map[string]string{"Origin": "http://localhost:5173"}})
	eq(t, "non-token requests excluded", len(usageCaptureRecords(t, h)), 1)
	stored, err := json.Marshal(items[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, testAPIKey, db.HashAPIKey(testAPIKey), "password="} {
		if strings.Contains(string(stored), forbidden) {
			t.Fatalf("usage log stored credential/path/query material %q", forbidden)
		}
	}
	// Even the selected order reference and supplied request ID cannot disguise
	// the credential as ordinary request metadata.
	h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/query?orderNo=" + url.QueryEscape(testAPIKey),
		Headers: withHeaders(h.groupAuth, map[string]string{"X-Correlation-ID": testAPIKey})})
	log := usageCaptureRecords(t, h)[0]
	if log.OrderNo != "" || log.RequestID != "[REDACTED]" {
		t.Fatalf("credential hidden in request metadata: %+v", log)
	}
}

func TestGatewayUsageDatabaseFailureDoesNotBreakResponse(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/gateway/not-a-real-route", Headers: h.groupAuth})
	eq(t, "static 404 survives closed logging database", r.Status, http.StatusNotFound)
}
