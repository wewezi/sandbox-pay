// 接入网关 API Key 守卫的 e2e。
//
// 支付本身放到 payment_flow_test.go，这里只打守卫和分组隔离。
package routes

import (
	"net/http"
	"testing"
)

func TestGatewayAPIKeyGuard(t *testing.T) {
	h := newHarness(t, nil)

	payBody := map[string]any{"amount": 1, "productName": "Test", "payMethod": "native"}

	pay := func(t *testing.T, headers map[string]string) *resp {
		t.Helper()
		return h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/gateway/pay",
			Body:    payBody,
			Headers: headers,
		})
	}
	jsonCT := map[string]string{"Content-Type": "application/json"}

	t.Run("没带 Authorization 返回 401", func(t *testing.T) {
		eq(t, "status", pay(t, jsonCT).Status, http.StatusUnauthorized)
	})

	t.Run("Key 不对返回 401", func(t *testing.T) {
		r := pay(t, withHeaders(jsonCT, map[string]string{
			"Authorization": "Bearer sp_wrong_key",
		}))
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("少了 Bearer 前缀返回 401", func(t *testing.T) {
		r := pay(t, withHeaders(jsonCT, map[string]string{
			"Authorization": testAPIKey,
		}))
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("Bearer 后面为空返回 401", func(t *testing.T) {
		r := pay(t, withHeaders(jsonCT, map[string]string{
			"Authorization": "Bearer ",
		}))
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("停用的分组返回 401", func(t *testing.T) {
		// 直接把种子分组停掉，再调一次。守卫 JOIN 时查 m.isActive = 1，
		// 这条用例是唯一能覆盖到那个条件的地方。
		if _, err := h.c.Groups.ToggleGroup(h.groupID); err != nil {
			t.Fatalf("停用分组失败: %v", err)
		}
		defer func() {
			if _, err := h.c.Groups.ToggleGroup(h.groupID); err != nil {
				t.Errorf("恢复分组失败: %v", err)
			}
		}()
		eq(t, "status", pay(t, h.groupAuth).Status, http.StatusUnauthorized)
	})

	t.Run("Key 对得上就放行到业务层", func(t *testing.T) {
		r := pay(t, h.groupAuth)
		eq(t, "status", r.Status, http.StatusOK)
		eq(t, "code", num(t, r.obj()["code"], "code"), 200)
		eq(t, "data.type", str(t, r.data()["type"], "type"), "url")
	})

	t.Run("GET 查询同样只需要 Bearer，不用签 query", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=NOPE",
			Headers: map[string]string{"Authorization": "Bearer " + testAPIKey},
		})
		// 过了守卫才会到业务层；订单不存在是 404，Key 不对是 401
		eq(t, "status", r.Status, http.StatusNotFound)
	})

	t.Run("同一分组的第二把 Key 也能下单", func(t *testing.T) {
		issued, err := h.c.Groups.IssueKey(h.groupID, "第二把")
		if err != nil {
			t.Fatalf("IssueKey 失败: %v", err)
		}
		r := pay(t, withHeaders(jsonCT, map[string]string{
			"Authorization": "Bearer " + issued.Key,
		}))
		eq(t, "status", r.Status, http.StatusOK)
	})

	t.Run("分组 B 查不到分组 A 的订单", func(t *testing.T) {
		created := pay(t, h.groupAuth)
		eq(t, "下单 status", created.Status, http.StatusOK)
		orderNo := cashierOrderNo(t, created)

		other, err := h.c.Groups.CreateGroup("隔离测", "")
		if err != nil {
			t.Fatalf("CreateGroup 失败: %v", err)
		}
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=" + orderNo,
			Headers: map[string]string{"Authorization": "Bearer " + other.Keys[0].Key},
		})
		// 守卫过了，业务层按 groupId 收窄，查不到就当不存在
		eq(t, "status", r.Status, http.StatusNotFound)
	})
}
