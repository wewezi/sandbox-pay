package routes

import (
	"fmt"
	"net/http"
	"testing"

	"sandbox-pay/internal/db"
)

func scenarioPost(t *testing.T, h *harness, auth map[string]string, body map[string]any) *resp {
	t.Helper()
	return h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/sandbox/scenarios", Headers: withHeaders(auth, map[string]string{"Content-Type": "application/json"}), Body: body})
}

func TestSandboxScenarioRoutesAndSwitch(t *testing.T) {
	h := newHarness(t, nil)
	_, orderNo := h.createOrder(t, map[string]any{"amount": 1.0, "productName": "scene", "payMethod": "native"})
	unauthorized := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/sandbox/scenarios"})
	eq(t, "JWT required", unauthorized.Status, http.StatusUnauthorized)
	invalid := scenarioPost(t, h, h.auth, map[string]any{"orderNo": orderNo, "scenario": "delayed_paid", "delaySeconds": 1.5})
	eq(t, "fractional delay", invalid.Status, http.StatusBadRequest)
	created := scenarioPost(t, h, h.auth, map[string]any{"orderNo": orderNo, "scenario": "delayed_paid", "delaySeconds": 30})
	eq(t, "task created", created.Status, http.StatusOK)
	eq(t, "pending task", str(t, created.data()["status"], "status"), "pending")
	id := int64(num(t, created.data()["id"], "id"))
	listed := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/sandbox/scenarios", Headers: h.auth})
	eq(t, "list", listed.Status, http.StatusOK)
	eq(t, "list length", len(listed.dataArr()), 1)
	cancelled := h.do(t, reqSpec{Method: "POST", Path: fmt.Sprintf("/v1/api/admin/sandbox/scenarios/%d/cancel", id), Headers: h.auth})
	eq(t, "cancel", cancelled.Status, http.StatusOK)
	eq(t, "cancelled", str(t, cancelled.data()["status"], "status"), "cancelled")
	paid := scenarioPost(t, h, h.auth, map[string]any{"orderNo": orderNo, "scenario": "paid"})
	eq(t, "paid scene", paid.Status, http.StatusOK)
	order, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || order.Status != db.StatusPaid {
		t.Fatalf("state=%#v err=%v", order, err)
	}
	disabled := newHarness(t, map[string]string{"ENABLE_SANDBOX": "false"})
	for _, method := range []string{"GET", "POST"} {
		response := disabled.do(t, reqSpec{Method: method, Path: "/v1/api/admin/sandbox/scenarios", Headers: withHeaders(disabled.auth, map[string]string{"Content-Type": "application/json"}), Body: map[string]any{"orderNo": "NOPE", "scenario": "paid"}})
		eq(t, "disabled scenario", response.Status, http.StatusForbidden)
	}
}

func TestSandboxScenarioMemberOwnership(t *testing.T) {
	h := newHarness(t, nil)
	other, err := h.c.Groups.CreateGroup("member project", "member key")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := h.store.Users.FindByUsername(testAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.Users.Insert("scene-member", admin.Password, db.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	member, err := h.c.Auth.ValidateUser("scene-member", testAdminPass)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.Users.SetProjectIDs(member.ID, []int64{other.ID}); err != nil {
		t.Fatal(err)
	}
	token, err := h.c.Auth.IssueLocalToken(member)
	if err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{"Authorization": "Bearer " + token.Token}
	_, outside := h.createOrder(t, map[string]any{"amount": 1.0, "productName": "outside", "payMethod": "native"})
	denied := scenarioPost(t, h, auth, map[string]any{"orderNo": outside, "scenario": "paid"})
	eq(t, "cross project scenario", denied.Status, http.StatusNotFound)
	outsideTask := scenarioPost(t, h, h.auth, map[string]any{"orderNo": outside, "scenario": "delayed_paid", "delaySeconds": 30})
	eq(t, "outside task created", outsideTask.Status, http.StatusOK)
	outsideID := int64(num(t, outsideTask.data()["id"], "id"))
	denied = h.do(t, reqSpec{Method: "POST", Path: fmt.Sprintf("/v1/api/admin/sandbox/scenarios/%d/cancel", outsideID), Headers: auth})
	eq(t, "cross project cancel", denied.Status, http.StatusNotFound)
	listed := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/sandbox/scenarios", Headers: auth})
	eq(t, "member list", listed.Status, http.StatusOK)
	eq(t, "out of scope tasks hidden", len(listed.dataArr()), 0)
	created := testPayPost(t, h, map[string]any{"amount": 1.0, "apiKeyId": other.Keys[0].ID})
	eq(t, "owned order", created.Status, http.StatusOK)
	owned := str(t, created.data()["orderNo"], "orderNo")
	allowed := scenarioPost(t, h, auth, map[string]any{"orderNo": owned, "scenario": "delayed_paid", "delaySeconds": 30})
	eq(t, "owned scenario", allowed.Status, http.StatusOK)
	listed = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/sandbox/scenarios", Headers: auth})
	eq(t, "one owned task", len(listed.dataArr()), 1)
}
