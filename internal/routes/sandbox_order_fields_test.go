package routes

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/services"
)

func sandboxOrderFieldReceipt(t *testing.T, h *harness, response *resp, method string, minutes int) *db.PaymentOrder {
	t.Helper()
	eq(t, "creation HTTP status", response.Status, http.StatusOK)
	body := response.data()
	orderNo := str(t, body["orderNo"], "orderNo")
	stored, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || stored == nil {
		t.Fatalf("created order missing: %v", err)
	}
	eq(t, "stored payment method", stored.PayMethod, method)
	eq[any](t, "receipt payment method", body["payMethod"], method)
	eq[any](t, "receipt stored status", body["status"], float64(stored.Status))
	eq[any](t, "receipt creation time", body["createdAt"], services.JsTime(stored.CreatedAt))
	eq[any](t, "receipt expiration time", body["expireAt"], services.JsTime(stored.ExpireAt))
	createdAt, err := time.Parse(time.RFC3339Nano, str(t, body["createdAt"], "createdAt"))
	if err != nil {
		t.Fatalf("creation time must be ISO format: %v", err)
	}
	expireAt, err := time.Parse(time.RFC3339Nano, str(t, body["expireAt"], "expireAt"))
	if err != nil {
		t.Fatalf("expiration time must be ISO format: %v", err)
	}
	deviation := expireAt.Sub(createdAt) - time.Duration(minutes)*time.Minute
	if deviation < -time.Second || deviation > time.Second {
		t.Fatalf("actual deadline does not match requested %d minutes: duration=%s", minutes, expireAt.Sub(createdAt))
	}
	if _, exists := body["expireMinutes"]; exists {
		t.Fatal("receipt must report actual timestamps rather than repeat request TTL")
	}
	if stored.ProviderOrderID != nil || stored.ThirdPartyTradeNo != nil || stored.ForeignAmount != nil || stored.ExchangeRate != nil {
		t.Fatal("local order creation dispatched a provider payment or quote")
	}
	for _, field := range []string{"data", "paymentUrl"} {
		if !strings.Contains(str(t, body[field], field), "/cashier?orderNo="+orderNo) {
			t.Fatalf("%s must open the local cashier", field)
		}
	}
	return stored
}

func TestSandboxOrderFieldsDefaultsRemainCompatible(t *testing.T) {
	for _, explicitNull := range []bool{false, true} {
		t.Run(fmt.Sprintf("optional null=%v", explicitNull), func(t *testing.T) {
			h := newHarness(t, nil)
			body := map[string]any{"amount": 0.01}
			if explicitNull {
				body["payMethod"], body["expireMinutes"] = nil, nil
			}
			response := testPayPost(t, h, body)
			order := sandboxOrderFieldReceipt(t, h, response, "native", 30)
			eq(t, "default product", order.ProductName, "Mintweft Test Order")
			eq(t, "default CNY cents", order.Amount, int64(1))
			if order.ExternalOrderNo != nil || order.ReturnURL != nil || order.NotifyURL != nil || order.APIKeyID != nil {
				t.Fatal("default creation acquired unsupported optional context")
			}
		})
	}
}

func TestSandboxOrderFieldsThreeMethodsAndTTLBoundariesStayLocal(t *testing.T) {
	for _, method := range []string{"native", "alipay", "paypal"} {
		for _, minutes := range []int{1, 120} {
			t.Run(fmt.Sprintf("%s/%d", method, minutes), func(t *testing.T) {
				// Credentials are deliberately absent. Dispatching either SDK
				// would fail rather than create the expected local cashier order.
				h := newHarness(t, nil)
				external := fmt.Sprintf("FULL-FORM-%s-%d", method, minutes)
				response := testPayPost(t, h, map[string]any{
					"amount": 12.99, "productName": "Full form order", "payMethod": method,
					"expireMinutes": minutes, "externalOrderNo": external,
					"returnUrl": "https://merchant.example/complete", "notifyUrl": "https://merchant.example/notify",
				})
				order := sandboxOrderFieldReceipt(t, h, response, method, minutes)
				if order.ExternalOrderNo == nil || order.ReturnURL == nil || order.NotifyURL == nil {
					t.Fatal("explicit order context was not saved")
				}
				eq(t, "stored amount", order.Amount, int64(1299))
				eq(t, "stored external order", *order.ExternalOrderNo, external)
				eq(t, "stored return URL", *order.ReturnURL, "https://merchant.example/complete")
				eq(t, "stored notification URL", *order.NotifyURL, "https://merchant.example/notify")
				callbacks, total, err := h.store.Notifies.Search(db.NotifySearchOpts{Limit: 10})
				if err != nil || total != 0 || len(callbacks) != 0 {
					t.Fatal("creating a local order sent a payment notification")
				}
			})
		}
	}
}

func TestSandboxOrderFieldsRejectInvalidTTLWithoutCreatingOrders(t *testing.T) {
	for _, invalid := range []any{0, -1, 121, 1.5, 119.5, "bad-number", "", "NaN", "Infinity", "-Infinity", 1e20} {
		t.Run(fmt.Sprintf("%T=%v", invalid, invalid), func(t *testing.T) {
			h := newHarness(t, nil)
			response := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "paypal", "expireMinutes": invalid})
			eq(t, "invalid TTL status", response.Status, http.StatusBadRequest)
			count, err := h.store.Orders.Count()
			if err != nil || count != 0 {
				t.Fatal("invalid TTL created an order")
			}
		})
	}
	for _, literal := range []string{"NaN", "Infinity", "-Infinity"} {
		t.Run("invalid JSON "+literal, func(t *testing.T) {
			h := newHarness(t, nil)
			response := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/admin/test-pay",
				Body:    `{"amount":1,"expireMinutes":` + literal + `}`,
				Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
			})
			eq(t, "non-JSON numeric value status", response.Status, http.StatusBadRequest)
			count, err := h.store.Orders.Count()
			if err != nil || count != 0 {
				t.Fatal("invalid JSON TTL created an order")
			}
		})
	}
}

func TestSandboxOrderFieldsRetryPreservesDeadlineAndOriginalContext(t *testing.T) {
	h := newHarness(t, nil)
	project, err := h.c.Groups.CreateGroup("Full form project", "Original token")
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := h.c.Groups.IssueKey(project.ID, "Retry token")
	if err != nil {
		t.Fatal(err)
	}
	first := testPayPost(t, h, map[string]any{
		"amount": 11.99, "productName": "Original product", "payMethod": "paypal", "expireMinutes": 1,
		"externalOrderNo": "SAME-FORM-ORDER", "apiKeyId": project.Keys[0].ID,
		"returnUrl": "https://original.example/complete", "notifyUrl": "https://original.example/notify",
	})
	original := sandboxOrderFieldReceipt(t, h, first, "paypal", 1)
	retry := testPayPost(t, h, map[string]any{
		"amount": 11.99, "productName": "Changed product", "payMethod": "paypal", "expireMinutes": 120,
		"externalOrderNo": "SAME-FORM-ORDER", "apiKeyId": secondKey.ID,
		"returnUrl": "https://changed.example/complete", "notifyUrl": "https://changed.example/notify",
	})
	eq(t, "retry HTTP status", retry.Status, http.StatusOK)
	for _, field := range []string{"orderNo", "amount", "productName", "payMethod", "createdAt", "expireAt", "externalOrderNo", "apiKeyId", "apiKeyName", "groupName", "returnUrl", "notifyUrl", "status"} {
		eq(t, "retry original "+field, retry.data()[field], first.data()[field])
	}
	stored, err := h.store.Orders.FindByOrderNo(original.OrderNo, false)
	if err != nil || stored == nil || !stored.ExpireAt.Equal(original.ExpireAt) || !stored.CreatedAt.Equal(original.CreatedAt) ||
		stored.ProductName != original.ProductName || stored.APIKeyID == nil || *stored.APIKeyID != *original.APIKeyID {
		t.Fatal("idempotent retry changed the stored deadline or original context")
	}
	changedMethod := testPayPost(t, h, map[string]any{
		"amount": 11.99, "externalOrderNo": "SAME-FORM-ORDER", "apiKeyId": project.Keys[0].ID,
		"payMethod": "alipay", "expireMinutes": 120,
	})
	eq(t, "different method conflict", changedMethod.Status, http.StatusConflict)
	count, err := h.store.Orders.Count()
	if err != nil || count != 1 {
		t.Fatal("retry or conflicting method created another order")
	}
}

func TestSandboxOrderFieldsTerminalRetriesReportActualStatus(t *testing.T) {
	for _, state := range []struct {
		name       string
		stored     int
		deadline   bool
		wantStatus int
	}{
		{name: "paid", stored: db.StatusPaid, wantStatus: db.StatusPaid},
		{name: "failed", stored: db.StatusFailed, wantStatus: db.StatusFailed},
		{name: "expired", stored: db.StatusExpired, wantStatus: db.StatusExpired},
		{name: "refunding", stored: db.StatusRefunding, wantStatus: db.StatusRefunding},
		{name: "refunded", stored: db.StatusRefunded, wantStatus: db.StatusRefunded},
		{name: "deadline passed", stored: db.StatusPending, deadline: true, wantStatus: db.StatusExpired},
	} {
		t.Run(state.name, func(t *testing.T) {
			h := newHarness(t, nil)
			body := map[string]any{"amount": 1, "payMethod": "alipay", "expireMinutes": 30, "externalOrderNo": "TERMINAL-FORM-ORDER"}
			first := testPayPost(t, h, body)
			order := sandboxOrderFieldReceipt(t, h, first, "alipay", 30)
			patch := map[string]any{"status": state.stored}
			if state.deadline {
				patch["createdAt"], patch["expireAt"] = time.Now().Add(-time.Hour), time.Now().Add(-time.Minute)
			}
			if err := h.store.Orders.Update(order.OrderNo, patch); err != nil {
				t.Fatal(err)
			}
			stored, err := h.store.Orders.FindByOrderNo(order.OrderNo, false)
			if err != nil || stored == nil {
				t.Fatal("terminal fixture missing")
			}
			body["expireMinutes"] = 120
			retry := testPayPost(t, h, body)
			eq(t, "terminal retry HTTP status", retry.Status, http.StatusOK)
			eq[any](t, "actual status", retry.data()["status"], float64(state.wantStatus))
			eq(t, "actual method", retry.data()["payMethod"], "alipay")
			eq[any](t, "original order", retry.data()["orderNo"], order.OrderNo)
			eq[any](t, "stored creation time", retry.data()["createdAt"], services.JsTime(stored.CreatedAt))
			eq[any](t, "stored deadline", retry.data()["expireAt"], services.JsTime(stored.ExpireAt))
			if !strings.Contains(str(t, retry.data()["paymentUrl"], "paymentUrl"), "/cashier?orderNo="+order.OrderNo) {
				t.Fatal("terminal retry attempted to open a payment provider")
			}
		})
	}
}

func TestSandboxOrderFieldsProjectPermissionsAndUnknownFieldsRemainEnforced(t *testing.T) {
	t.Run("project scope", func(t *testing.T) {
		h := newHarness(t, nil)
		root, err := h.store.Users.FindByUsername(testAdmin)
		if err != nil || root == nil {
			t.Fatal("root user fixture missing")
		}
		if err := h.store.Users.Insert("form-member", root.Password, db.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		member, err := h.store.Users.FindByUsername("form-member")
		if err != nil || member == nil {
			t.Fatal("member fixture missing")
		}
		owned, err := h.c.Groups.CreateGroup("Owned form project", "Owned token")
		if err != nil {
			t.Fatal(err)
		}
		foreign, err := h.c.Groups.CreateGroup("Foreign form project", "Foreign token")
		if err != nil {
			t.Fatal(err)
		}
		team := services.NewTeamService(h.store)
		if err := team.SetProjects(root.ID, member.ID, []int64{owned.ID}); err != nil {
			t.Fatal(err)
		}
		token, err := h.c.Auth.IssueLocalToken(member)
		if err != nil {
			t.Fatal(err)
		}
		h.auth = map[string]string{"Authorization": "Bearer " + token.Token}
		allowed := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "paypal", "expireMinutes": 7, "apiKeyId": owned.Keys[0].ID})
		sandboxOrderFieldReceipt(t, h, allowed, "paypal", 7)
		denied := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "alipay", "expireMinutes": 120, "apiKeyId": foreign.Keys[0].ID})
		eq(t, "foreign project concealed", denied.Status, http.StatusNotFound)
		defaultDenied := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "native", "expireMinutes": 60})
		eq(t, "unassigned default project denied", defaultDenied.Status, http.StatusForbidden)
		if err := team.SetProjects(root.ID, member.ID, nil); err != nil {
			t.Fatal(err)
		}
		revoked := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "paypal", "expireMinutes": 7, "apiKeyId": owned.Keys[0].ID})
		eq(t, "revoked project concealed", revoked.Status, http.StatusNotFound)
		count, err := h.store.Orders.Count()
		if err != nil || count != 1 {
			t.Fatal("project authorization failure created an order")
		}
	})
	for _, field := range []string{"groupId", "accountNo"} {
		t.Run("unknown "+field, func(t *testing.T) {
			h := newHarness(t, nil)
			response := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "paypal", "expireMinutes": 60, field: "unsupported"})
			eq(t, "unknown field rejected", response.Status, http.StatusBadRequest)
			count, err := h.store.Orders.Count()
			if err != nil || count != 0 {
				t.Fatal("unsupported field created an order")
			}
		})
	}
	for _, method := range []string{"bank", "", "stripe", "PayPal"} {
		t.Run("invalid method "+method, func(t *testing.T) {
			h := newHarness(t, nil)
			response := testPayPost(t, h, map[string]any{"amount": 1, "payMethod": method, "expireMinutes": 60})
			eq(t, "invalid method rejected", response.Status, http.StatusBadRequest)
		})
	}
	for _, disabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("sandbox disabled=%v", disabled), func(t *testing.T) {
			settings := map[string]string{}
			if disabled {
				settings["ENABLE_SANDBOX"] = "false"
			}
			h := newHarness(t, settings)
			var response *resp
			want := http.StatusForbidden
			if disabled {
				response = testPayPost(t, h, map[string]any{"amount": 1, "payMethod": "paypal", "expireMinutes": 120})
			} else {
				want = http.StatusUnauthorized
				response = h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/test-pay", Body: map[string]any{"amount": 1, "payMethod": "alipay", "expireMinutes": 120}})
			}
			eq(t, "creation guard", response.Status, want)
			count, err := h.store.Orders.Count()
			if err != nil || count != 0 {
				t.Fatal("sandbox or authentication guard failure created an order")
			}
		})
	}
}
