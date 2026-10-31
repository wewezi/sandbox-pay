package routes

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/jwt"
	"sandbox-pay/internal/services"
)

func TestTeamHTTPMembershipAndAdministrativeBoundaries(t *testing.T) {
	h := newHarness(t, nil)
	root, err := h.store.Users.FindByUsername(testAdmin)
	if err != nil {
		t.Fatal(err)
	}
	created := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/team/users", Headers: h.auth, Body: map[string]any{"username": "member-user", "password": "member-password", "role": "Admin"}})
	eq(t, "create account", created.Status, 200)
	user, err := h.store.Users.FindByUsername("member-user")
	if err != nil || user == nil {
		t.Fatal(err)
	}
	team := services.NewTeamService(h.store)
	if err := team.SetProjects(root.ID, user.ID, []int64{h.groupID}); err != nil {
		t.Fatal(err)
	}
	token, err := h.c.Auth.IssueLocalToken(user)
	if err != nil {
		t.Fatal(err)
	}
	member := map[string]string{"Authorization": "Bearer " + token.Token}
	defaultSession := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session", Headers: member})
	if defaultSession.data()["sandboxDefaultProjectAllowed"] != true {
		t.Fatal("default-project member flag should be true")
	}
	for _, path := range []string{"/admin/team/users", "/admin/operations/status", "/admin/audit-logs"} {
		res := h.do(t, reqSpec{Method: "GET", Path: "/v1/api" + path, Headers: member})
		eq(t, path+" is restricted", res.Status, http.StatusForbidden)
	}
	res := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/projects", Headers: member, Body: map[string]any{"name": "unauthorized"}})
	eq(t, "project creation requires super admin", res.Status, 403)
	res = h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/reset-data", Headers: member})
	eq(t, "global reset requires super admin", res.Status, 403)
	other, err := h.store.Groups.Insert("other-team-project", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		id   int64
	}{{"TEAM-OWN", h.groupID}, {"TEAM-OTHER", other.ID}} {
		order := &db.PaymentOrder{ID: item.name, OrderNo: item.name, GroupID: &item.id, Amount: 100, ProductName: item.name, PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour)}
		if err := h.store.Orders.Insert(order); err != nil {
			t.Fatal(err)
		}
	}
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/TEAM-OWN", Headers: member})
	eq(t, "own project detail", res.Status, 200)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/TEAM-OTHER", Headers: member})
	eq(t, "other project detail concealed", res.Status, 404)
	res = h.do(t, reqSpec{Method: "DELETE", Path: "/v1/api/admin/transactions/TEAM-OTHER", Headers: member})
	eq(t, "other project deletion concealed", res.Status, 404)
	res = h.do(t, reqSpec{Method: "POST", Path: "/v1/api/native-pay/confirm", Headers: member, Body: map[string]any{"orderNo": "TEAM-OTHER", "walletUser": "demo", "walletPass": "123456"}})
	eq(t, "other project confirmation concealed", res.Status, 404)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions", Headers: member})
	eq(t, "scoped list", res.Status, 200)
	eq(t, "scoped total", num(t, res.data()["total"], "total"), 1)
	if err := team.SetProjects(root.ID, user.ID, []int64{}); err != nil {
		t.Fatal(err)
	}
	defaultSession = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session", Headers: member})
	if defaultSession.data()["sandboxDefaultProjectAllowed"] != false {
		t.Fatal("removed default-project membership flag should be false")
	}
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions", Headers: member})
	eq(t, "empty membership total", num(t, res.data()["total"], "total"), 0)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions/TEAM-OWN", Headers: member})
	eq(t, "removed membership immediately denied", res.Status, 404)
}

func TestTeamHTTPScopedAggregatesKeysUsageAndCallbacks(t *testing.T) {
	h := newHarness(t, nil)
	root, _ := h.store.Users.FindByUsername(testAdmin)
	team := services.NewTeamService(h.store)
	memberUser, err := team.CreateUser(root.ID, "scope-member", "scope-password", db.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := team.SetProjects(root.ID, memberUser.ID, []int64{h.groupID}); err != nil {
		t.Fatal(err)
	}
	user, _ := h.store.Users.FindByID(memberUser.ID)
	token, err := h.c.Auth.IssueLocalToken(user)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + token.Token}
	other, err := h.store.Groups.Insert("foreign-scope-project", true)
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, err := h.c.Groups.IssueKey(other.ID, "foreign-key")
	if err != nil {
		t.Fatal(err)
	}
	var ownNotificationID, foreignNotificationID, ownUsageID, foreignUsageID int64
	now := time.Now().UTC()
	for _, fixture := range []struct {
		name    string
		groupID int64
		amount  int64
		status  int
	}{{"SCOPED-OWN", h.groupID, 100, db.NotifyFailed}, {"SCOPED-OTHER", other.ID, 50000, db.NotifyPending}} {
		order := &db.PaymentOrder{ID: fixture.name, OrderNo: fixture.name, GroupID: &fixture.groupID, Amount: fixture.amount, ProductName: fixture.name, PayMethod: "native", Status: db.StatusPaid, PayAt: &now, CreatedAt: now, ExpireAt: now.Add(time.Hour)}
		if err := h.store.Orders.Insert(order); err != nil {
			t.Fatal(err)
		}
		id, err := h.store.Notifies.InsertReturningID(db.InsertNotifyInput{OrderNo: &fixture.name, URL: "https://merchant.example/notify", Body: `{"status":"paid"}`, Signature: `{}`, Status: fixture.status})
		if err != nil {
			t.Fatal(err)
		}
		usage := &db.UsageLog{RequestID: fixture.name, GroupID: &fixture.groupID, Path: "/v1/api/gateway/query", Method: "GET", StatusCode: 200, DurationMs: 1, OrderNo: fixture.name, CreatedAt: now}
		if err := h.store.UsageLogs.Insert(usage); err != nil {
			t.Fatal(err)
		}
		if fixture.groupID == h.groupID {
			ownNotificationID, ownUsageID = id, usage.ID
		} else {
			foreignNotificationID, foreignUsageID = id, usage.ID
		}
	}
	if err := h.store.UsageLogs.Insert(&db.UsageLog{Path: "/v1/api/gateway/query", Method: "GET", StatusCode: 401, DurationMs: 1}); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *resp {
		return h.do(t, reqSpec{Method: "GET", Path: "/v1/api" + path, Headers: headers})
	}
	stats := get("/admin/stats")
	eq(t, "stats status", stats.Status, 200)
	eq(t, "scoped stats count", num(t, stats.data()["totalCount"], "totalCount"), 1)
	eq(t, "scoped stats amount", num(t, stats.data()["amount"], "amount"), 1)
	dashboard := get("/admin/dashboard?days=7&utcOffset=0")
	eq(t, "dashboard status", dashboard.Status, 200)
	summary := dashboard.data()["summary"].(map[string]any)
	eq(t, "dashboard scoped amount", num(t, summary["amount"], "amount"), 1)
	eq(t, "dashboard scoped count", num(t, summary["totalCount"], "totalCount"), 1)
	eq(t, "dashboard scoped recent", len(dashboard.data()["recentOrders"].([]any)), 1)
	tasks := dashboard.data()["tasks"].(map[string]any)
	eq(t, "dashboard scoped projects", num(t, tasks["activeGroups"], "activeGroups"), 1)
	eq(t, "dashboard scoped keys", num(t, tasks["activeKeys"], "activeKeys"), 1)
	eq(t, "dashboard scoped failed callbacks", num(t, tasks["failedNotifications"], "failedNotifications"), 1)
	usage := get("/admin/usage-logs?pageSize=1")
	eq(t, "usage status", usage.Status, 200)
	eq(t, "usage total excludes foreign and anonymous", num(t, usage.data()["total"], "total"), 1)
	eq(t, "usage summary excludes foreign and anonymous", num(t, usage.data()["summary"].(map[string]any)["totalCount"], "totalCount"), 1)
	for _, item := range []struct {
		path     string
		expected int
	}{{fmt.Sprintf("/admin/usage-logs/%d", ownUsageID), 200}, {fmt.Sprintf("/admin/usage-logs/%d", foreignUsageID), 404}, {fmt.Sprintf("/admin/notifications/%d/detail", ownNotificationID), 200}, {fmt.Sprintf("/admin/notifications/%d/detail", foreignNotificationID), 404}, {fmt.Sprintf("/admin/groups/%d/webhook-secret", other.ID), 404}} {
		eq(t, "scoped detail "+item.path, get(item.path).Status, item.expected)
	}
	for _, path := range []string{"/admin/groups", "/admin/api-keys"} {
		list := get(path)
		eq(t, path+" status", list.Status, 200)
		eq(t, path+" scoped length", len(list.obj()["data"].([]any)), 1)
	}
	notifications := get("/admin/notifications?pageSize=1")
	eq(t, "callback list status", notifications.Status, 200)
	eq(t, "scoped callback total", num(t, notifications.data()["total"], "total"), 1)
	for _, action := range []struct {
		path string
		body map[string]any
	}{
		{fmt.Sprintf("/admin/api-keys/%d/toggle", foreignKey.ID), nil},
		{fmt.Sprintf("/admin/groups/%d/toggle", other.ID), nil},
		{fmt.Sprintf("/admin/notifications/%d/replay", foreignNotificationID), nil},
		{fmt.Sprintf("/admin/notifications/%d/simulate", foreignNotificationID), map[string]any{"statusCode": 200}},
		{"/admin/refund", map[string]any{"orderNo": "SCOPED-OTHER", "amount": 1}},
		{"/admin/test-pay", map[string]any{"apiKeyId": foreignKey.ID, "amount": 1}},
		{"/admin/api-keys", map[string]any{"groupId": other.ID}},
	} {
		res := h.do(t, reqSpec{Method: "POST", Path: "/v1/api" + action.path, Headers: headers, Body: action.body})
		eq(t, "foreign write "+action.path, res.Status, 404)
	}
	for _, path := range []string{fmt.Sprintf("/admin/stats?groupId=%d", other.ID), fmt.Sprintf("/admin/usage-logs?groupId=%d", other.ID), fmt.Sprintf("/admin/api-keys?groupId=%d", other.ID)} {
		eq(t, "foreign query "+path, get(path).Status, 404)
	}
	eq(t, "project admin developer docs", get("/admin/developer").Status, 200)
	if err := team.SetProjects(root.ID, user.ID, []int64{h.groupID, other.ID}); err != nil {
		t.Fatal(err)
	}
	stats = get("/admin/stats")
	eq(t, "multiple-member stats count", num(t, stats.data()["totalCount"], "totalCount"), 2)
	eq(t, "multiple-member stats amount", num(t, stats.data()["amount"], "amount"), 501)
	usage = get("/admin/usage-logs?pageSize=1")
	eq(t, "multiple-member usage total", num(t, usage.data()["total"], "total"), 2)
	eq(t, "multiple-member usage summary", num(t, usage.data()["summary"].(map[string]any)["totalCount"], "totalCount"), 2)
}

func TestTeamHTTPLogoutRevokesBearerAndSessionOwnership(t *testing.T) {
	h := newHarness(t, nil)
	root, _ := h.store.Users.FindByUsername(testAdmin)
	first, err := h.c.Auth.IssueLocalToken(root, services.SessionMetadata{ClientIP: "192.0.2.1", UserAgent: "test browser"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.c.Auth.IssueLocalToken(root)
	if err != nil {
		t.Fatal(err)
	}
	firstHeaders := map[string]string{"Authorization": "Bearer " + first.Token}
	secondHeaders := map[string]string{"Authorization": "Bearer " + second.Token}
	firstID := jwt.Verify(first.Token, h.cfg.Get("JWT_SECRET"))["jti"].(string)
	res := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/auth/sessions/revoke-others", Headers: firstHeaders})
	eq(t, "revoke others", res.Status, 200)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session", Headers: firstHeaders})
	eq(t, "current session survives", res.Status, 200)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session", Headers: secondHeaders})
	eq(t, "other session rejected", res.Status, 401)
	team := services.NewTeamService(h.store)
	other, err := team.CreateUser(root.ID, "other-session-user", "other-password", db.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	otherUser, _ := h.store.Users.FindByID(other.ID)
	otherToken, err := h.c.Auth.IssueLocalToken(otherUser)
	if err != nil {
		t.Fatal(err)
	}
	res = h.do(t, reqSpec{Method: "POST", Path: "/v1/api/auth/sessions/" + firstID + "/revoke", Headers: map[string]string{"Authorization": "Bearer " + otherToken.Token}})
	eq(t, "foreign session revoke is a safe no-op", res.Status, 200)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session", Headers: firstHeaders})
	eq(t, "foreign user did not revoke root", res.Status, 200)
	res = h.do(t, reqSpec{Method: "POST", Path: "/v1/api/auth/logout", Headers: firstHeaders})
	eq(t, "logout", res.Status, 200)
	res = h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session", Headers: firstHeaders})
	eq(t, "copied bearer invalid after logout", res.Status, 401)
}
