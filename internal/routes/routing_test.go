package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func TestProtectedRouteGroupsRejectAnonymousRequests(t *testing.T) {
	h := newHarness(t, nil)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/api/admin/stats"},
		{http.MethodGet, "/v1/api/admin/dashboard"},
		{http.MethodGet, "/v1/api/admin/transactions/missing"},
		{http.MethodDelete, "/v1/api/admin/transactions/missing"},
		{http.MethodPost, "/v1/api/admin/projects"},
		{http.MethodPost, "/v1/api/admin/api-keys"},
		{http.MethodGet, "/v1/api/admin/usage-logs"},
		{http.MethodPost, "/v1/api/admin/notifications/1/simulate"},
		{http.MethodPost, "/v1/api/admin/sandbox/scenarios"},
		{http.MethodGet, "/v1/api/admin/operations/status"},
		{http.MethodGet, "/v1/api/admin/developer"},
		{http.MethodPost, "/v1/api/admin/team/users"},
		{http.MethodPatch, "/v1/api/admin/team/users/1"},
		{http.MethodPut, "/v1/api/admin/team/users/1/projects"},
		{http.MethodGet, "/v1/api/auth/session"},
		{http.MethodGet, "/v1/api/auth/sessions"},
		{http.MethodPost, "/v1/api/auth/password"},
		{http.MethodPost, "/v1/api/native-pay/confirm"},
		{http.MethodPost, "/v1/api/gateway/pay"},
		{http.MethodPost, "/v1/api/gateway/alipay/pay"},
		{http.MethodPost, "/v1/api/gateway/paypal/pay"},
		{http.MethodPost, "/v1/api/gateway/native/pay"},
		{http.MethodGet, "/v1/api/gateway/query?orderNo=missing"},
		{http.MethodPost, "/v1/api/gateway/refund"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			r := h.do(t, reqSpec{Method: route.method, Path: route.path})
			eq(t, "status", r.Status, http.StatusUnauthorized)
			eq(t, "error code", num(t, r.obj()["code"], "code"), 401)
		})
	}
}

func TestRoutingErrorsStayJSONAndDoNotServeSPA(t *testing.T) {
	h := newHarness(t, nil)
	h.handler = New(h.cfg, h.c, newStaticSPAFixture(t))
	for _, route := range []struct {
		method  string
		path    string
		status  int
		headers map[string]string
		allowed []string
	}{
		{http.MethodGet, "/v1/api/missing", http.StatusNotFound, nil, nil},
		{http.MethodGet, "/v1/api/admin/missing", http.StatusNotFound, h.auth, nil},
		{http.MethodPost, "/v1/api/openapi.json", http.StatusMethodNotAllowed, nil, []string{http.MethodGet, http.MethodHead}},
		{http.MethodPut, "/v1/api/admin/stats", http.StatusMethodNotAllowed, h.auth, []string{http.MethodGet, http.MethodHead}},
		{http.MethodGet, "/v1/api/gateway/refund", http.StatusMethodNotAllowed, h.groupAuth, []string{http.MethodPost}},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			r := h.do(t, reqSpec{Method: route.method, Path: route.path, Headers: route.headers})
			eq(t, "status", r.Status, route.status)
			eq(t, "error code", num(t, r.obj()["code"], "code"), float64(route.status))
			eq(t, "error path", str(t, r.obj()["path"], "path"), route.path)
			if r.Header.Get("X-Correlation-ID") == "" {
				t.Fatal("routing error is missing the correlation header")
			}
			if route.allowed != nil {
				allowed := map[string]bool{}
				for _, method := range strings.Split(strings.Join(r.Header.Values("Allow"), ","), ",") {
					allowed[strings.TrimSpace(method)] = true
				}
				eq(t, "allowed method count", len(allowed), len(route.allowed))
				for _, method := range route.allowed {
					if !allowed[method] {
						t.Errorf("Allow = %q, missing %s", r.Header.Values("Allow"), method)
					}
				}
			}
		})
	}
}

func TestHealthHEADReturnsHeadersWithoutBody(t *testing.T) {
	h := newHarness(t, nil)
	server := httptest.NewServer(h.handler)
	t.Cleanup(server.Close)
	r, err := server.Client().Head(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "HEAD status", r.StatusCode, http.StatusOK)
	eq(t, "HEAD body", string(body), "")
	eq(t, "HEAD content type", r.Header.Get("Content-Type"), "application/json; charset=utf-8")
	if r.Header.Get("X-Correlation-ID") == "" {
		t.Fatal("HEAD response is missing the correlation header")
	}
}

func TestAlipayNotificationIsIndependentOfAPIRateLimit(t *testing.T) {
	h := newHarness(t, map[string]string{"THROTTLE_LIMIT": "1"})
	ip := h.nextIP()
	notification := reqSpec{
		Method: http.MethodPost, Path: "/v1/api/alipay/notify", IP: ip,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	}
	beforeLimit := h.do(t, notification)
	eq(t, "invalid notification before limit", beforeLimit.Status, http.StatusBadRequest)
	first := h.do(t, reqSpec{Method: http.MethodGet, Path: "/v1/api/gateway/query", IP: ip})
	eq(t, "first API request", first.Status, http.StatusUnauthorized)
	limited := h.do(t, reqSpec{Method: http.MethodGet, Path: "/v1/api/gateway/query", IP: ip})
	eq(t, "exhausted API rate limit", limited.Status, http.StatusTooManyRequests)
	for i := 0; i < 3; i++ {
		notify := h.do(t, notification)
		eq(t, "invalid notification after limit", notify.Status, beforeLimit.Status)
		eq(t, "notification error remains the same", notify.obj()["message"], beforeLimit.obj()["message"])
	}
}

func TestOrderDetailPathDecodesURLParameterOnce(t *testing.T) {
	for _, test := range []struct {
		name    string
		orderNo string
	}{
		{"escaped separator", "PAY/中文+%2F"},
		{"literal escape", "PAY%2F"},
		{"unicode literal escape", "订单%2F"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, nil)
			order := &db.PaymentOrder{
				ID: "encoded-order-path", OrderNo: test.orderNo, Amount: 100,
				ProductName: "路径参数订单", PayMethod: "native", Status: db.StatusPending,
				ExpireAt: time.Now().Add(time.Hour), GroupID: &h.groupID,
			}
			if err := h.store.Orders.Insert(order); err != nil {
				t.Fatal(err)
			}
			r := h.do(t, reqSpec{
				Method:  http.MethodGet,
				Path:    "/v1/api/admin/transactions/" + url.PathEscape(order.OrderNo),
				Headers: h.auth,
			})
			eq(t, "detail status", r.Status, http.StatusOK)
			eq(t, "decoded order number", str(t, r.data()["orderNo"], "orderNo"), order.OrderNo)
		})
	}
}

func TestProjectMemberHEADAndEncodedOrderPaths(t *testing.T) {
	h := newHarness(t, nil)
	root, err := h.store.Users.FindByUsername(testAdmin)
	if err != nil || root == nil {
		t.Fatalf("find administrator: %v", err)
	}
	project, err := h.store.Groups.Insert("member-routing-project", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Groups.IssueKey(project.ID, "member-routing-key"); err != nil {
		t.Fatal(err)
	}
	team := services.NewTeamService(h.store)
	member, err := team.CreateUser(root.ID, "routing-member", "routing-password", db.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := team.SetProjects(root.ID, member.ID, []int64{project.ID}); err != nil {
		t.Fatal(err)
	}
	user, err := h.store.Users.FindByID(member.ID)
	if err != nil || user == nil {
		t.Fatalf("find member: %v", err)
	}
	token, err := h.c.Auth.IssueLocalToken(user)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + token.Token}
	server := httptest.NewServer(h.handler)
	t.Cleanup(server.Close)
	for _, path := range []string{"/v1/api/admin/groups", "/v1/api/admin/api-keys"} {
		before := h.do(t, reqSpec{Method: http.MethodGet, Path: path, Headers: headers})
		eq(t, "scoped list status", before.Status, http.StatusOK)
		eq(t, "scoped list length", len(before.obj()["data"].([]any)), 1)
		req, err := http.NewRequest(http.MethodHead, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", headers["Authorization"])
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		eq(t, path+" HEAD status", response.StatusCode, before.Status)
		eq(t, path+" HEAD body", string(body), "")
		after := h.do(t, reqSpec{Method: http.MethodGet, Path: path, Headers: headers})
		eq(t, path+" list after HEAD", after.Raw, before.Raw)
	}
	for _, fixture := range []struct {
		name    string
		orderNo string
		groupID int64
		status  int
	}{
		{"own", "OWN/中文%2F", project.ID, http.StatusOK},
		{"foreign", "FOREIGN/中文%2F", h.groupID, http.StatusNotFound},
	} {
		order := &db.PaymentOrder{
			ID: fixture.name, OrderNo: fixture.orderNo, Amount: 100,
			ProductName: fixture.name, PayMethod: "native", Status: db.StatusPending,
			ExpireAt: time.Now().Add(time.Hour), GroupID: &fixture.groupID,
		}
		if err := h.store.Orders.Insert(order); err != nil {
			t.Fatal(err)
		}
		path := "/v1/api/admin/transactions/" + url.PathEscape(fixture.orderNo)
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			r := h.do(t, reqSpec{Method: method, Path: path, Headers: headers})
			eq(t, method+" "+fixture.name+" order", r.Status, fixture.status)
		}
		if fixture.name == "foreign" {
			r := h.do(t, reqSpec{Method: http.MethodDelete, Path: path, Headers: headers})
			eq(t, "foreign order deletion", r.Status, http.StatusNotFound)
			stored, err := h.store.Orders.FindByOrderNo(order.OrderNo, false)
			if err != nil || stored == nil {
				t.Fatalf("foreign order changed: %v", err)
			}
		}
	}
}
