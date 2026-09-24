// 后台接口的 e2e：JWT 守卫、统计、订单列表、分组凭证、通知、沙箱重置。
//
// 对应原 TS src/routes/admin.e2e.spec.ts —— 同一个 harness 顺序跑，
// 和 TS 的 before/after 共享一个临时库语义一致（用例顺序即文件内顺序）。
package routes

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"
)

func TestAdminE2E(t *testing.T) {
	h := newHarness(t, nil)

	t.Run("JWT 守卫：没带 token 一律 401", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats"})
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("JWT 守卫：伪造 token 一律 401", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/admin/stats",
			Headers: map[string]string{"Authorization": "Bearer invalid-token"},
		})
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("GET /admin/stats 空账本 successRate 给 null，不是 0", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats", Headers: h.auth})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "totalCount", num(t, d["totalCount"], "totalCount"), 0)
		eq(t, "successCount", num(t, d["successCount"], "successCount"), 0)
		eq(t, "amount", num(t, d["amount"], "amount"), 0)
		// 空账本上 0% 和「暂无数据」不是一个意思，占位符由前端决定
		if d["successRate"] != nil {
			t.Fatalf("successRate: got %v, want null", d["successRate"])
		}
	})

	t.Run("GET /admin/transactions 返回分页信封", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/transactions", Headers: h.auth})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		if _, ok := d["items"].([]any); !ok {
			t.Fatalf("items 不是数组: %#v", d["items"])
		}
		eq(t, "total", num(t, d["total"], "total"), 0)
		eq(t, "page", num(t, d["page"], "page"), 1)
	})

	t.Run("GET /admin/transactions 接受分页参数", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/admin/transactions?page=2&pageSize=10",
			Headers: h.auth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "page", num(t, d["page"], "page"), 2)
		eq(t, "pageSize", num(t, d["pageSize"], "pageSize"), 10)
	})

	t.Run("GET /admin/groups 返回分组列表，Key 是明文（沙箱后台要给自己抄）", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/groups", Headers: h.auth})
		eq(t, "status", r.Status, http.StatusOK)
		items := r.dataArr()
		eq(t, "分组数", len(items), 1)
		g := items[0].(map[string]any)
		if str(t, g["name"], "name") == "" {
			t.Fatal("name 为空")
		}
		keys, ok := g["keys"].([]any)
		if !ok || len(keys) == 0 {
			t.Fatalf("keys 为空: %#v", g["keys"])
		}
		eq(t, "keys[0].key", str(t, keys[0].(map[string]any)["key"], "key"), testAPIKey)
		// 合并成一个 Key 之后不该再有这两个字段漏出去
		if _, ok := g["appKey"]; ok {
			t.Fatal("appKey 不该出现在响应里")
		}
		if _, ok := g["appSecret"]; ok {
			t.Fatal("appSecret 不该出现在响应里")
		}
	})

	t.Run("新建分组自带第一把 Key", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/admin/groups",
			Body:    map[string]any{"name": "分组B", "keyName": "生产"},
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "name", str(t, d["name"], "name"), "分组B")
		keys, ok := d["keys"].([]any)
		if !ok || len(keys) != 1 {
			t.Fatalf("keys 长度不是 1: %#v", d["keys"])
		}
		k := keys[0].(map[string]any)
		eq(t, "keys[0].name", str(t, k["name"], "name"), "生产")
		if !regexp.MustCompile(`^sp_sandbox_[0-9a-f]{64}$`).MatchString(str(t, k["key"], "key")) {
			t.Fatalf("key 形状不对: %q", k["key"])
		}
	})

	t.Run("给已有分组再签一把 Key", func(t *testing.T) {
		list := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/groups", Headers: h.auth})
		groupID := int64(num(t, list.dataArr()[0].(map[string]any)["id"], "id"))

		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    fmt.Sprintf("/v1/api/admin/groups/%d/keys", groupID),
			Body:    map[string]any{"name": "CI"},
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "name", str(t, d["name"], "name"), "CI")
		eq(t, "groupId", int64(num(t, d["groupId"], "groupId")), groupID)
		if str(t, d["key"], "key") == testAPIKey {
			t.Fatal("新签的 Key 不该和种子 Key 相同")
		}
	})

	t.Run("停用 Key 后入站 401，行还在", func(t *testing.T) {
		issued := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/admin/groups/1/keys",
			Body:    map[string]any{"name": "即将停用"},
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
		})
		key := issued.data()

		off := h.do(t, reqSpec{
			Method:  "POST",
			Path:    fmt.Sprintf("/v1/api/admin/api-keys/%d/toggle", int64(num(t, key["id"], "id"))),
			Headers: h.auth,
		})
		eq(t, "toggle status", off.Status, http.StatusOK)

		pay := h.do(t, reqSpec{
			Method: "POST",
			Path:   "/v1/api/gateway/pay",
			Body:   map[string]any{"amount": 1, "productName": "x", "payMethod": "native"},
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + str(t, key["key"], "key"),
			},
		})
		eq(t, "停用 Key 下单 status", pay.Status, http.StatusUnauthorized)

		// 种子 Key 还活着
		still := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=NOPE",
			Headers: map[string]string{"Authorization": "Bearer " + testAPIKey},
		})
		eq(t, "种子 Key 查单 status", still.Status, http.StatusNotFound)
	})

	t.Run("POST /admin/refund 不存在的订单 404", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/admin/refund",
			Body:    map[string]any{"orderNo": "NOPE", "amount": 10},
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
		})
		eq(t, "status", r.Status, http.StatusNotFound)
	})

	t.Run("GET /admin/notifications 返回分页列表", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/notifications", Headers: h.auth})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		if _, ok := d["items"].([]any); !ok {
			t.Fatalf("items 不是数组: %#v", d["items"])
		}
		eq(t, "total", num(t, d["total"], "total"), 0)
	})

	t.Run("POST /admin/reset-data ENABLE_SANDBOX=true 时能清空订单", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/admin/reset-data",
			Headers: h.auth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		deleted, ok := r.data()["deleted"].(map[string]any)
		if !ok {
			t.Fatalf("deleted 不是对象: %#v", r.data()["deleted"])
		}
		eq(t, "deleted.orders", num(t, deleted["orders"], "orders"), 0)
		eq(t, "deleted.notifications", num(t, deleted["notifications"], "notifications"), 0)
	})
}
