// 完整支付生命周期的 e2e：下单 → 查单 → 收银台 → 核销 → 退款 → 后台列表。
//
// 走 native 通道，不碰支付宝 / PayPal 的真实沙箱。
// 对应原 TS src/routes/payment-flow.e2e.spec.ts：用例有先后依赖（共享 orderNo），
// 和 TS 的 before/after 语义一致 —— 顺序跑，不能并行。
package routes

import (
	"net/http"
	"strings"
	"testing"
)

func TestPaymentFlow(t *testing.T) {
	h := newHarness(t, nil)
	var orderNo string

	t.Run("接入方用 Bearer 下 native 单，拿到收银台 URL", func(t *testing.T) {
		r, no := h.createOrder(t, map[string]any{
			"amount":          1.5,
			"productName":     "e2e widget",
			"payMethod":       "native",
			"externalOrderNo": "EXT-001",
			"notifyUrl":       "https://example.com/notify",
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "data.type", str(t, d["type"], "type"), "url")
		hasSubstr(t, str(t, d["data"], "data"), "/cashier.html?orderNo=", "收银台 URL")
		if !strings.HasPrefix(no, "PAY") {
			t.Fatalf("orderNo %q 不以 PAY 开头", no)
		}
		orderNo = no
	})

	t.Run("同一 externalOrderNo 再下是幂等，金额变了才 400", func(t *testing.T) {
		same := map[string]any{
			"amount":          1.5,
			"productName":     "e2e widget",
			"payMethod":       "native",
			"externalOrderNo": "EXT-001",
		}
		again := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/gateway/pay",
			Body:    same,
			Headers: h.groupAuth,
		})
		eq(t, "again status", again.Status, http.StatusOK)
		eq(t, "again orderNo", cashierOrderNo(t, again), orderNo)

		changed := map[string]any{
			"amount":          9.9,
			"productName":     "e2e widget",
			"payMethod":       "native",
			"externalOrderNo": "EXT-001",
		}
		clash := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/gateway/pay",
			Body:    changed,
			Headers: h.groupAuth,
		})
		eq(t, "clash status", clash.Status, http.StatusBadRequest)
	})

	t.Run("接入方查单能看到 Pending 状态和金额", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=" + orderNo,
			Headers: h.groupAuth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "orderNo", str(t, d["orderNo"], "orderNo"), orderNo)
		eq(t, "amount", num(t, d["amount"], "amount"), 1.5)
		eq(t, "status", num(t, d["status"], "status"), 0) // Pending
		eq(t, "productName", str(t, d["productName"], "productName"), "e2e widget")
		eq(t, "externalOrderNo", str(t, d["externalOrderNo"], "externalOrderNo"), "EXT-001")
	})

	t.Run("收银台公开接口返回待支付信息，不需要登录", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method: "GET",
			Path:   "/v1/api/native-pay/cashier?orderNo=" + orderNo,
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "orderNo", str(t, d["orderNo"], "orderNo"), orderNo)
		eq(t, "amount", num(t, d["amount"], "amount"), 1.5)
		if str(t, d["status"], "status") == "paid" {
			t.Fatal("status 不该是 paid")
		}
	})

	t.Run("管理员核销入账", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method: "POST",
			Path:   "/v1/api/native-pay/confirm",
			Body: map[string]any{
				"orderNo":    orderNo,
				"tradeNo":    "SP_WALLET_E2E",
				"walletUser": "wallet-user",
				"walletPass": "secret1",
			},
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
		})
		eq(t, "confirm status", r.Status, http.StatusOK)

		query := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=" + orderNo,
			Headers: h.groupAuth,
		})
		d := query.data()
		eq(t, "status", num(t, d["status"], "status"), 1) // Paid
		eq(t, "thirdPartyTradeNo", str(t, d["thirdPartyTradeNo"], "thirdPartyTradeNo"), "SP_WALLET_E2E")
	})

	t.Run("已支付订单再核销被拒，原 tradeNo 不动", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method: "POST",
			Path:   "/v1/api/native-pay/confirm",
			Body: map[string]any{
				"orderNo":    orderNo,
				"tradeNo":    "SP_WALLET_E2E_AGAIN",
				"walletUser": "wallet-user",
				"walletPass": "secret1",
			},
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"}),
		})
		// native 层在 Pending 之外直接 400，不会进到 markPaid 的 no-op
		eq(t, "confirm status", r.Status, http.StatusBadRequest)

		query := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=" + orderNo,
			Headers: h.groupAuth,
		})
		eq(t, "thirdPartyTradeNo", str(t, query.data()["thirdPartyTradeNo"], "thirdPartyTradeNo"),
			"SP_WALLET_E2E")
	})

	t.Run("后台列表能看到这笔 Paid 订单", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/admin/transactions",
			Headers: h.auth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "total", num(t, d["total"], "total"), 1)
		items := d["items"].([]any)
		item := items[0].(map[string]any)
		eq(t, "items[0].orderNo", str(t, item["orderNo"], "orderNo"), orderNo)
		eq(t, "items[0].status", str(t, item["status"], "status"), "SUCCESS")
		eq(t, "items[0].amount", num(t, item["amount"], "amount"), 1.5)
	})

	t.Run("统计把这笔算进成功数", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats", Headers: h.auth})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "totalCount", num(t, d["totalCount"], "totalCount"), 1)
		eq(t, "successCount", num(t, d["successCount"], "successCount"), 1)
		eq(t, "amount", num(t, d["amount"], "amount"), 1.5)
		eq(t, "successRate", num(t, d["successRate"], "successRate"), 1)
	})

	t.Run("接入方退全款，订单落到 Refunded", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/gateway/refund",
			Body:    map[string]any{"orderNo": orderNo, "amount": 1.5, "reason": "e2e refund"},
			Headers: h.groupAuth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		d := r.data()
		eq(t, "orderNo", str(t, d["orderNo"], "orderNo"), orderNo)
		eq(t, "refundedAmount", num(t, d["refundedAmount"], "refundedAmount"), 1.5)
		hasSubstr(t, str(t, d["refundTradeNo"], "refundTradeNo"), "SP_REFUND_", "refundTradeNo")

		query := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/gateway/query?orderNo=" + orderNo,
			Headers: h.groupAuth,
		})
		q := query.data()
		eq(t, "status", num(t, q["status"], "status"), 4) // Refunded
		eq(t, "refundedAmount", num(t, q["refundedAmount"], "refundedAmount"), 1.5)
	})

	t.Run("退完再退被拒", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/gateway/refund",
			Body:    map[string]any{"orderNo": orderNo, "amount": 0.01},
			Headers: h.groupAuth,
		})
		eq(t, "status", r.Status, http.StatusConflict)
	})

	t.Run("渠道专属端点 /gateway/native/pay 也能下单，无需传 payMethod", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/gateway/native/pay",
			Body:    map[string]any{"amount": 0.01, "productName": "channel-native"},
			Headers: h.groupAuth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		eq(t, "data.type", str(t, r.data()["type"], "type"), "url")
	})

	t.Run("公开沙箱测试下单无需登录", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method: "POST",
			Path:   "/v1/api/native-pay/public-test-pay",
		})
		eq(t, "status", r.Status, http.StatusOK)
		eq(t, "data.type", str(t, r.data()["type"], "type"), "url")
	})

	t.Run("校验层拒绝未声明字段", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method: "POST",
			Path:   "/v1/api/gateway/pay",
			Body: map[string]any{
				"amount":      1,
				"productName": "x",
				"payMethod":   "native",
				"extra":       true,
			},
			Headers: h.groupAuth,
		})
		eq(t, "status", r.Status, http.StatusBadRequest)
		hasSubstr(t, str(t, r.obj()["message"], "message"),
			"property extra should not exist", "400 message")
	})
}
