package routes

import (
	"fmt"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/services"
)

type callbackDiagnosticHTTP struct{ deliveries int }

func (f *callbackDiagnosticHTTP) Get(string, time.Duration) (*httpclient.Response, error) {
	return &httpclient.Response{Status: 200}, nil
}
func (f *callbackDiagnosticHTTP) Post(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
	f.deliveries++
	return &httpclient.Response{Status: 200, BodySummary: "accepted"}, nil
}

func insertRouteDiagnostic(t *testing.T, h *harness, orderNo string, groupID int64) int64 {
	t.Helper()
	order := &db.PaymentOrder{ID: orderNo, OrderNo: orderNo, GroupID: &groupID, Amount: 100, ProductName: orderNo, PayMethod: "native", Status: db.StatusPaid, ExpireAt: time.Now().Add(time.Hour)}
	if err := h.store.Orders.Insert(order); err != nil {
		t.Fatal(err)
	}
	id, err := h.store.Notifies.InsertReturningID(db.InsertNotifyInput{OrderNo: &orderNo, URL: "https://example.test/callback", Body: `{"orderNo":"` + orderNo + `","status":"paid"}`, Signature: `{"sign":"private-signature","nonce":"private-nonce","timestamp":"1793412345678"}`, Status: db.NotifySuccess})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCallbackDiagnosticsHTTPAuthenticationSandboxAndValidation(t *testing.T) {
	h := newHarness(t, nil)
	id := insertRouteDiagnostic(t, h, "CALLBACK-OWN", h.groupID)
	fake := &callbackDiagnosticHTTP{}
	h.c.Notify = services.NewNotifyService(h.store.Notifies, fake)
	path := fmt.Sprintf("/v1/api/admin/notifications/%d", id)
	res := h.do(t, reqSpec{Method: "GET", Path: path + "/detail"})
	eq(t, "unauthenticated detail", res.Status, 401)
	res = h.do(t, reqSpec{Method: "GET", Path: path + "/detail", Headers: h.auth})
	eq(t, "detail", res.Status, 200)
	if _, present := res.data()["signature"]; present {
		t.Fatal("detail exposes signature")
	}
	if enabled, _ := res.data()["simulationEnabled"].(bool); !enabled {
		t.Fatal("sandbox simulation disabled")
	}
	for _, body := range []any{map[string]any{"mode": "wrong"}, map[string]any{"mode": "delayed", "delaySeconds": 0}, map[string]any{"mode": "delayed", "delaySeconds": 1.5}, map[string]any{"mode": "delayed", "delaySeconds": 3601}, map[string]any{"mode": "duplicate", "delaySeconds": 1}} {
		res = h.do(t, reqSpec{Method: "POST", Path: path + "/simulate", Headers: h.auth, Body: body})
		eq(t, "invalid simulation", res.Status, 400)
	}
	res = h.do(t, reqSpec{Method: "POST", Path: path + "/simulate", Headers: h.auth, Body: map[string]any{"mode": "duplicate"}})
	eq(t, "duplicate", res.Status, 200)
	if fake.deliveries != 1 {
		t.Fatalf("deliveries=%d", fake.deliveries)
	}
	newID := int64(num(t, res.data()["id"], "id"))
	res = h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/notifications/%d/detail", newID), Headers: h.auth})
	eq(t, "duplicate detail", res.Status, 200)
	attempts, _ := res.data()["attempts"].([]any)
	if len(attempts) != 1 {
		t.Fatalf("duplicate attempts=%v", attempts)
	}
	res = h.do(t, reqSpec{Method: "POST", Path: path + "/simulate", Headers: h.auth, Body: map[string]any{"mode": "delayed", "delaySeconds": 30}})
	eq(t, "delayed", res.Status, 200)
	if fake.deliveries != 1 {
		t.Fatal("delayed callback sent immediately")
	}
	production := newHarness(t, map[string]string{"ENABLE_SANDBOX": "false"})
	prodID := insertRouteDiagnostic(t, production, "PROD-CALLBACK", production.groupID)
	prodPath := fmt.Sprintf("/v1/api/admin/notifications/%d", prodID)
	res = production.do(t, reqSpec{Method: "GET", Path: prodPath + "/detail", Headers: production.auth})
	eq(t, "production diagnostics allowed", res.Status, 200)
	if enabled, _ := res.data()["simulationEnabled"].(bool); enabled {
		t.Fatal("production exposes simulation controls")
	}
	res = production.do(t, reqSpec{Method: "POST", Path: prodPath + "/simulate", Headers: production.auth, Body: map[string]any{"mode": "duplicate"}})
	eq(t, "production simulation denied", res.Status, 403)
}

func TestCallbackDiagnosticsHTTPProjectScope(t *testing.T) {
	h := newHarness(t, nil)
	other, err := h.store.Groups.Insert("other-callback-project", true)
	if err != nil {
		t.Fatal(err)
	}
	ownID := insertRouteDiagnostic(t, h, "MEMBER-CALLBACK", h.groupID)
	otherID := insertRouteDiagnostic(t, h, "OTHER-CALLBACK", other.ID)
	if err := h.store.Users.Insert("callback-member", "unused", db.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	user, err := h.store.Users.FindByUsername("callback-member")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.Users.SetProjectIDs(user.ID, []int64{h.groupID}); err != nil {
		t.Fatal(err)
	}
	session, err := h.c.Auth.IssueLocalToken(user)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	res := h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/notifications/%d/detail", ownID), Headers: headers})
	eq(t, "member own detail", res.Status, 200)
	res = h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/notifications/%d/detail", otherID), Headers: headers})
	eq(t, "other detail concealed", res.Status, 404)
	res = h.do(t, reqSpec{Method: "POST", Path: fmt.Sprintf("/v1/api/admin/notifications/%d/simulate", otherID), Headers: headers, Body: map[string]any{"mode": "delayed", "delaySeconds": 10}})
	eq(t, "other simulation concealed", res.Status, 404)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/notifications?pageSize=1", Headers: headers})
	eq(t, "scoped list", res.Status, 200)
	eq(t, "scoped total", num(t, res.data()["total"], "total"), 1)
	if err := h.store.Users.SetProjectIDs(user.ID, []int64{}); err != nil {
		t.Fatal(err)
	}
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/notifications", Headers: headers})
	eq(t, "empty scoped total", num(t, res.data()["total"], "total"), 0)
}
