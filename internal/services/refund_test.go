package services

import (
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

// createMockRefundOrder 对应 refund.spec 的 createMockOrder(overrides)：
// 默认 ￥100、已支付的支付宝单（￥10000 分），带 notifyUrl 但没挂 apiKeyId。
func createMockRefundOrder() *db.PaymentOrder {
	now := time.Now().UTC()
	groupID := int64(1)
	return &db.PaymentOrder{
		ID:                "test-id",
		OrderNo:           "PAY123",
		Amount:            10000,
		RefundedAmount:    0,
		Status:            db.StatusPaid,
		ProductName:       "Test",
		PayMethod:         "alipay",
		GroupID:           &groupID,
		UserID:            0,
		ChannelCost:       0,
		Fee:               0,
		SettleAmount:      0,
		ThirdPartyTradeNo: strPtr("ALI_TRADE_123"),
		NotifyURL:         strPtr("https://example.com/notify"),
		CreatedAt:         now,
		UpdatedAt:         now,
		ExpireAt:          now,
	}
}

type refundFixture struct {
	service *RefundService
	store   *db.Store
	alipay  *fakeAlipay
	paypal  *fakePayPal
}

// createRefundService 三个假依赖：真 Store + 真 PaymentService + 假网关。
func createRefundService(t *testing.T) *refundFixture {
	t.Helper()
	store := newTestStore(t)
	seedGroup1(t, store)
	alipay := newFakeAlipay("ALI_REFUND_123")
	paypal := newFakePayPal("PAYPAL_REFUND_123")
	notify := &fakeNotify{}
	payments := NewPaymentService(store, notify, NewFeeCalculator(testFeeConfig()))
	svc := NewRefundService(store, payments, alipay, paypal)
	return &refundFixture{service: svc, store: store, alipay: alipay, paypal: paypal}
}

// insert 默认单。
func (f *refundFixture) insert(t *testing.T, mutate func(*db.PaymentOrder)) *db.PaymentOrder {
	t.Helper()
	order := createMockRefundOrder()
	if mutate != nil {
		mutate(order)
	}
	insertOrder(t, f.store, order)
	return order
}

// adminOpts 对应 TS 的 { actor: 'admin' }。
func adminOpts() RefundOptions { return RefundOptions{Actor: "admin"} }

// Port of server/src/services/refund.service.spec.ts
func TestRefundServiceExecute(t *testing.T) {
	t.Run("throws when orderNo is missing", func(t *testing.T) {
		f := createRefundService(t)
		_, err := f.service.Execute(RefundInput{OrderNo: "", Amount: 10}, adminOpts())
		requireErrStatus(t, err, 400)
	})

	t.Run("throws when amount is missing", func(t *testing.T) {
		f := createRefundService(t)
		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 0}, adminOpts())
		requireErrStatus(t, err, 400)
	})

	t.Run("throws when order not found (admin path)", func(t *testing.T) {
		f := createRefundService(t)
		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 10}, adminOpts())
		requireErrStatus(t, err, 404)
	})

	t.Run("throws when order status is not Paid (admin path)", func(t *testing.T) {
		f := createRefundService(t)
		f.insert(t, func(o *db.PaymentOrder) { o.Status = db.StatusPending })

		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 10}, adminOpts())
		requireErrStatus(t, err, 409)
	})

	t.Run("throws when refund amount exceeds refundable balance", func(t *testing.T) {
		f := createRefundService(t)
		order := f.insert(t, func(o *db.PaymentOrder) {
			o.Amount = 1000
			o.RefundedAmount = 800
		})

		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 5}, adminOpts())
		requireErrStatus(t, err, 400)

		// 余额不够就不该动状态 —— MarkRefunding 一旦跑过就会是 Refunding
		row := reloadOrder(t, f.store, order.OrderNo)
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d) (markRefunding must not run)", row.Status, db.StatusPaid)
		}
	})

	t.Run("successfully refunds via alipay", func(t *testing.T) {
		f := createRefundService(t)
		order := f.insert(t, nil)

		result, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50}, adminOpts())
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		if result.OrderNo != "PAY123" {
			t.Errorf("orderNo = %q, want PAY123", result.OrderNo)
		}
		if result.RefundTradeNo != "ALI_REFUND_123" {
			t.Errorf("refundTradeNo = %q, want ALI_REFUND_123", result.RefundTradeNo)
		}
		if result.Currency != "CNY" {
			t.Errorf("currency = %q, want CNY", result.Currency)
		}
		// 网关收到的是订单号 + 整数分 + reason（未传 → 空串）
		if len(f.alipay.calls) != 1 {
			t.Fatalf("alipay.refund called %d times, want 1", len(f.alipay.calls))
		}
		call := f.alipay.calls[0]
		if call.orderNo != order.OrderNo {
			t.Errorf("alipay refund orderNo = %q, want the order's orderNo %q", call.orderNo, order.OrderNo)
		}
		if call.cents != 5000 {
			t.Errorf("alipay refund cents = %d, want 5000", call.cents)
		}
		if call.reason != "" {
			t.Errorf("alipay refund reason = %q, want empty", call.reason)
		}
		// markRefunded 落地：￥50 → 5000 分
		row := reloadOrder(t, f.store, order.OrderNo)
		if row.RefundedAmount != 5000 {
			t.Errorf("row.refundedAmount = %d, want 5000", row.RefundedAmount)
		}
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d) (partial refund lands back on Paid)", row.Status, db.StatusPaid)
		}
		if row.RefundTradeNo == nil || *row.RefundTradeNo != "ALI_REFUND_123" {
			t.Errorf("row.refundTradeNo = %v, want ALI_REFUND_123", row.RefundTradeNo)
		}
	})

	t.Run("successfully refunds via native", func(t *testing.T) {
		f := createRefundService(t)
		f.insert(t, func(o *db.PaymentOrder) {
			o.PayMethod = "native"
			o.ThirdPartyTradeNo = nil
		})

		result, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50}, adminOpts())
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		if result.Currency != "CNY" {
			t.Errorf("currency = %q, want CNY", result.Currency)
		}
		if !strings.Contains(result.RefundTradeNo, "SP_REFUND_") {
			t.Errorf("refundTradeNo = %q, want it to contain SP_REFUND_", result.RefundTradeNo)
		}
	})

	t.Run("pro-rates a paypal refund by the stored fx amount", func(t *testing.T) {
		f := createRefundService(t)
		f.insert(t, func(o *db.PaymentOrder) {
			o.PayMethod = "paypal"
			o.Amount = 10000
			o.ForeignAmount = i64Ptr(1400) // $14.00
			o.ForeignCurrency = strPtr("USD")
			o.ThirdPartyTradeNo = strPtr("CAPTURE_1")
		})

		result, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50}, adminOpts()) // 退一半
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		if len(f.paypal.calls) != 1 {
			t.Fatalf("paypal.refund called %d times, want 1", len(f.paypal.calls))
		}
		call := f.paypal.calls[0]
		if call.captureID != "CAPTURE_1" {
			t.Errorf("paypal refund captureID = %q, want CAPTURE_1", call.captureID)
		}
		if call.cents != 700 {
			t.Errorf("paypal refund cents = %d, want 700 (5000/10000 * 1400)", call.cents)
		}
		if call.currency != "USD" {
			t.Errorf("paypal refund currency = %q, want USD", call.currency)
		}
		if call.reason != "" {
			t.Errorf("paypal refund reason = %q, want empty", call.reason)
		}
		if result.Currency != "USD" {
			t.Errorf("result.currency = %q, want USD", result.Currency)
		}
	})

	t.Run("never pro-rates a paypal refund down to zero", func(t *testing.T) {
		f := createRefundService(t)
		f.insert(t, func(o *db.PaymentOrder) {
			o.PayMethod = "paypal"
			o.Amount = 100000            // ￥1000
			o.ForeignAmount = i64Ptr(14) // $0.14 —— 故意极端，按比例会被 round 抹成 0
			o.ForeignCurrency = strPtr("USD")
			o.ThirdPartyTradeNo = strPtr("CAPTURE_2")
		})

		// 退 ￥0.10 → 10/100000*14 = 0.0014，round 成 0，PayPal 会报
		// CANNOT_BE_ZERO_OR_NEGATIVE，所以必须夹到 1
		if _, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 0.1}, adminOpts()); err != nil {
			t.Fatalf("Execute: %v", err)
		}

		if len(f.paypal.calls) != 1 {
			t.Fatalf("paypal.refund called %d times, want 1", len(f.paypal.calls))
		}
		call := f.paypal.calls[0]
		if call.captureID != "CAPTURE_2" {
			t.Errorf("paypal refund captureID = %q, want CAPTURE_2", call.captureID)
		}
		if call.cents != 1 {
			t.Errorf("paypal refund cents = %d, want at least 1", call.cents)
		}
		if call.currency != "USD" {
			t.Errorf("paypal refund currency = %q, want USD", call.currency)
		}
	})

	t.Run("rejects a paypal refund without a captureId", func(t *testing.T) {
		f := createRefundService(t)
		order := f.insert(t, func(o *db.PaymentOrder) {
			o.PayMethod = "paypal"
			o.ThirdPartyTradeNo = nil
		})

		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50}, adminOpts())
		requireErrStatus(t, err, 400)

		// dispatch 抛错也要回滚，别卡在 Refunding
		row := reloadOrder(t, f.store, order.OrderNo)
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d) (must roll back)", row.Status, db.StatusPaid)
		}
		if len(f.paypal.calls) != 0 {
			t.Errorf("paypal.refund called %d times, want 0", len(f.paypal.calls))
		}
	})

	t.Run("rolls back to Paid when upstream refund fails", func(t *testing.T) {
		// 上游抛错在 Go 网关契约里 = Refund() 返回空串（网关吞错误，不 panic/throw）。
		// TS 的「上游抛错」和「上游返回 null」两个用例在 Go 里走同一个 "" 分支。
		f := createRefundService(t)
		f.alipay.ret = ""
		order := f.insert(t, nil)

		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50}, adminOpts())
		requireErrStatus(t, err, 400)

		if len(f.alipay.calls) != 1 {
			t.Errorf("alipay.refund called %d times, want 1", len(f.alipay.calls))
		}
		row := reloadOrder(t, f.store, order.OrderNo)
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d) (must roll back via markRefundFailed)", row.Status, db.StatusPaid)
		}
		// markRefunded 没被调用过
		if row.RefundedAmount != 0 {
			t.Errorf("row.refundedAmount = %d, want 0", row.RefundedAmount)
		}
		if row.RefundTradeNo != nil {
			t.Errorf("row.refundTradeNo = %v, want nil", row.RefundTradeNo)
		}
	})

	t.Run("throws for unsupported payMethod", func(t *testing.T) {
		f := createRefundService(t)
		f.insert(t, func(o *db.PaymentOrder) { o.PayMethod = "bitcoin" })

		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50}, adminOpts())
		requireErrStatus(t, err, 400)
	})

	t.Run("uses paymentService.getOrderForRefund when groupId is provided", func(t *testing.T) {
		f := createRefundService(t)
		order := f.insert(t, nil)

		// 分组隔离的查单路径：订单属于分组 1，传分组 2 必须 404 ——
		// 如果走了不分组的 admin 查单路径，这里会成功，断言就抓住了。
		_, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50},
			RefundOptions{Actor: "group:1", GroupID: i64Ptr(2)})
		requireErrStatus(t, err, 404)

		result, err := f.service.Execute(RefundInput{OrderNo: "PAY123", Amount: 50},
			RefundOptions{Actor: "group:1", GroupID: i64Ptr(1)})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.OrderNo != "PAY123" {
			t.Errorf("orderNo = %q, want PAY123", result.OrderNo)
		}
		if result.RefundTradeNo != "ALI_REFUND_123" {
			t.Errorf("refundTradeNo = %q, want ALI_REFUND_123", result.RefundTradeNo)
		}
		// 落地金额证明 markRefunded(orderNo, 5000, tradeNo, groupId) 走完了整条链路
		row := reloadOrder(t, f.store, order.OrderNo)
		if row.RefundedAmount != 5000 {
			t.Errorf("row.refundedAmount = %d, want 5000", row.RefundedAmount)
		}
		if row.RefundTradeNo == nil || *row.RefundTradeNo != "ALI_REFUND_123" {
			t.Errorf("row.refundTradeNo = %v, want ALI_REFUND_123", row.RefundTradeNo)
		}
	})
}
