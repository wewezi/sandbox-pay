package routes

import (
	"net/http"
	"testing"
)

func TestDashboardRouteAuthAndValidation(t *testing.T) {
	h := newHarness(t, nil)
	for _, headers := range []map[string]string{nil, {"Authorization": "Bearer invalid-token"}} {
		response := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/dashboard", Headers: headers})
		eq(t, "auth status", response.Status, http.StatusUnauthorized)
	}
	for _, query := range []string{
		"days=", "days=0", "days=14", "days=7.0", "days=abc", "days=7&days=30",
		"utcOffset=", "utcOffset=480.5", "utcOffset=841", "utcOffset=-721", "utcOffset=abc",
		"utcOffset=0&utcOffset=480",
	} {
		t.Run(query, func(t *testing.T) {
			response := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/dashboard?" + query, Headers: h.auth})
			eq(t, "validation status", response.Status, http.StatusBadRequest)
		})
	}
	for _, query := range []string{"", "?days=30&utcOffset=480", "?days=7&utcOffset=-720", "?utcOffset=840"} {
		response := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/dashboard" + query, Headers: h.auth})
		eq(t, "dashboard status", response.Status, http.StatusOK)
		data := response.data()
		wantDays := 7
		if query == "?days=30&utcOffset=480" {
			wantDays = 30
		}
		eq(t, "days", num(t, data["days"], "days"), float64(wantDays))
		trend, ok := data["trend"].([]any)
		if !ok || len(trend) != wantDays {
			t.Fatalf("trend: %#v", data["trend"])
		}
		channels, ok := data["channels"].([]any)
		if !ok || len(channels) != 0 {
			t.Fatalf("empty channels must be array: %#v", data["channels"])
		}
		recent, ok := data["recentOrders"].([]any)
		if !ok || len(recent) != 0 {
			t.Fatalf("empty recentOrders must be array: %#v", data["recentOrders"])
		}
		summary := data["summary"].(map[string]any)
		if summary["successRate"] != nil {
			t.Fatalf("empty success rate: %v", summary["successRate"])
		}
		tasks := data["tasks"].(map[string]any)
		eq(t, "activeGroups", num(t, tasks["activeGroups"], "activeGroups"), 1)
		eq(t, "activeKeys", num(t, tasks["activeKeys"], "activeKeys"), 1)
		if _, exists := data["keys"]; exists {
			t.Fatal("dashboard must not contain key credentials")
		}
	}
}
