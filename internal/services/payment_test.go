package services

import (
	"testing"
	"time"

	"sandbox-pay/internal/db"
)

// createMockPaymentOrder 对应 payment.spec 的 createMockOrder(overrides)：
// 先给默认值，测试再改字段。金额单位是整数分（￥200）。
func createMockPaymentOrder() *db.PaymentOrder {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(1)
	return &db.PaymentOrder{
		ID:             "ord1",
		OrderNo:        "PAY20250101001",
		Amount:         20000,
		RefundedAmount: 0,
		Status:         db.StatusRefunding,
		ProductName:    "Test",
		PayMethod:      "native",
		GroupID:        &groupID,
		UserID:         0,
		ChannelCost:    0,
		Fee:            0,
		SettleAmount:   0,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpireAt:       now.Add(30 * time.Minute),
	}
}

// createPaymentService 事务是真 Store.Do，repo 是真库 —— 只测状态机。
func createPaymentService(store *db.Store) (*PaymentService, *fakeNotify) {
	notify := &fakeNotify{}
	fees := NewFeeCalculator(testFeeConfig())
	return NewPaymentService(store, notify, fees), notify
}

// Port of server/src/services/payment.service.spec.ts
func TestPaymentServiceMarkRefunded(t *testing.T) {
	setup := func(t *testing.T) (*PaymentService, *fakeNotify, *db.Store, *db.PaymentOrder) {
		t.Helper()
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		insertOrder(t, store, order)
		svc, notify := createPaymentService(store)
		return svc, notify, store, order
	}

	t.Run("部分退款落地后回到 Paid，剩余余额还能再退", func(t *testing.T) {
		svc, _, store, order := setup(t)

		// ￥200 的订单退 ￥5
		got, err := svc.MarkRefunded(order.OrderNo, 500, "SP_REFUND_1", nil)
		if err != nil {
			t.Fatalf("MarkRefunded: %v", err)
		}

		if got.RefundedAmount != 500 {
			t.Errorf("refundedAmount = %d, want 500", got.RefundedAmount)
		}
		// 关键断言：不能停在 Refunding，否则查单 409，余额永久退不出来
		if got.Status != db.StatusPaid {
			t.Errorf("status = %d, want Paid(%d)", got.Status, db.StatusPaid)
		}
		row := reloadOrder(t, store, order.OrderNo)
		if row.RefundedAmount != 500 {
			t.Errorf("row.refundedAmount = %d, want 500", row.RefundedAmount)
		}
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d)", row.Status, db.StatusPaid)
		}
	})

	t.Run("退满后置为 Refunded", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.RefundedAmount = 19500
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		got, err := svc.MarkRefunded(order.OrderNo, 500, "SP_REFUND_2", nil)
		if err != nil {
			t.Fatalf("MarkRefunded: %v", err)
		}

		if got.RefundedAmount != 20000 {
			t.Errorf("refundedAmount = %d, want 20000", got.RefundedAmount)
		}
		if got.Status != db.StatusRefunded {
			t.Errorf("status = %d, want Refunded(%d)", got.Status, db.StatusRefunded)
		}
	})

	t.Run("累计超过订单金额时拒绝", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.RefundedAmount = 19500
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		_, err := svc.MarkRefunded(order.OrderNo, 1000, "SP_REFUND_3", nil)
		requireErrStatus(t, err, 400)

		// update 不能落地（事务回滚）
		row := reloadOrder(t, store, order.OrderNo)
		if row.RefundedAmount != 19500 {
			t.Errorf("row.refundedAmount = %d, want 19500 (must not update)", row.RefundedAmount)
		}
	})

	t.Run("同一 refundTradeNo 重放且已退完时是 no-op", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.RefundedAmount = 20000
		order.RefundTradeNo = strPtr("SP_REFUND_4")
		order.Status = db.StatusRefunded
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		got, err := svc.MarkRefunded(order.OrderNo, 500, "SP_REFUND_4", nil)
		if err != nil {
			t.Fatalf("MarkRefunded (replay): %v", err)
		}

		if got.RefundedAmount != 20000 {
			t.Errorf("refundedAmount = %d, want 20000 (replay must not add again)", got.RefundedAmount)
		}
		row := reloadOrder(t, store, order.OrderNo)
		if row.RefundedAmount != 20000 {
			t.Errorf("row.refundedAmount = %d, want 20000", row.RefundedAmount)
		}
		if row.Status != db.StatusRefunded {
			t.Errorf("row.status = %d, want Refunded(%d)", row.Status, db.StatusRefunded)
		}
	})

	t.Run("订单不在 Paid / Refunding 时拒绝", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusPending
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		_, err := svc.MarkRefunded(order.OrderNo, 500, "SP_REFUND_5", nil)
		requireErrStatus(t, err, 409)
	})

	t.Run("groupId 不匹配时当订单不存在（不泄漏他人订单）", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder() // groupID = 1
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		_, err := svc.MarkRefunded(order.OrderNo, 500, "SP_REFUND_6", i64Ptr(999))
		requireErrStatus(t, err, 404)
	})

	t.Run("落地后按状态发对应的通知", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		keyID := seedAPIKey(t, store, 1, "sp_test_key")
		order := createMockPaymentOrder()
		order.NotifyURL = strPtr("https://example.com/notify")
		order.APIKeyID = &keyID
		order.SigningSecret = strPtr("whsec_refund_test")
		order.SignatureVersion = 2
		insertOrder(t, store, order)
		svc, notify := createPaymentService(store)

		if _, err := svc.MarkRefunded(order.OrderNo, 500, "SP_REFUND_7", nil); err != nil {
			t.Fatalf("MarkRefunded: %v", err)
		}

		if len(notify.calls) != 1 {
			t.Fatalf("enqueueNotification called %d times, want 1", len(notify.calls))
		}
		call := notify.calls[0]
		if call.url != "https://example.com/notify" {
			t.Errorf("notify url = %q, want https://example.com/notify", call.url)
		}
		if call.notification.Status != "partial_refunded" {
			t.Errorf("notification.status = %q, want partial_refunded", call.notification.Status)
		}
		if call.notification.Amount != 200 {
			t.Errorf("notification.amount = %v, want 200 (整单 ￥200)", call.notification.Amount)
		}
		if call.signingKey != "whsec_refund_test" {
			t.Error("refund callback must use the independent project signing snapshot")
		}
	})
}

func TestPaymentServiceMarkPaid(t *testing.T) {
	setup := func(t *testing.T) (*PaymentService, *fakeNotify, *db.Store, *db.PaymentOrder) {
		t.Helper()
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusPending
		insertOrder(t, store, order)
		svc, notify := createPaymentService(store)
		return svc, notify, store, order
	}

	t.Run("金额一致时入账并算出手续费", func(t *testing.T) {
		svc, _, _, order := setup(t)

		result, err := svc.MarkPaid(order.OrderNo, strPtr("TRADE1"), 20000, "")
		if err != nil {
			t.Fatalf("MarkPaid: %v", err)
		}

		if result.Status != db.StatusPaid {
			t.Errorf("status = %d, want Paid(%d)", result.Status, db.StatusPaid)
		}
		if result.Fee != 200 {
			t.Errorf("fee = %d, want 200 (20000 * 0.01)", result.Fee)
		}
		if result.SettleAmount != 19800 {
			t.Errorf("settleAmount = %d, want 19800", result.SettleAmount)
		}
		if result.ThirdPartyTradeNo == nil || *result.ThirdPartyTradeNo != "TRADE1" {
			t.Errorf("thirdPartyTradeNo = %v, want TRADE1", result.ThirdPartyTradeNo)
		}
	})

	t.Run("金额被篡改时拒绝入账", func(t *testing.T) {
		svc, _, store, order := setup(t)

		// 回调声称只付了 ￥1，订单是 ￥200
		_, err := svc.MarkPaid(order.OrderNo, strPtr("TRADE2"), 100, "")
		requireErrStatus(t, err, 409)

		row := reloadOrder(t, store, order.OrderNo)
		if row.Status != db.StatusPending {
			t.Errorf("row.status = %d, want Pending(%d) (update must not run)", row.Status, db.StatusPending)
		}
		if row.ThirdPartyTradeNo != nil {
			t.Errorf("row.thirdPartyTradeNo = %v, want nil", row.ThirdPartyTradeNo)
		}
	})

	t.Run("外币订单按 foreignAmount 校验", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusPending
		order.ForeignAmount = i64Ptr(2800)
		order.ForeignCurrency = strPtr("USD")
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		result, err := svc.MarkPaid(order.OrderNo, strPtr("TRADE3"), 2800, "USD")
		if err != nil {
			t.Fatalf("MarkPaid: %v", err)
		}
		if result.Status != db.StatusPaid {
			t.Errorf("status = %d, want Paid(%d)", result.Status, db.StatusPaid)
		}
	})

	t.Run("外币金额对不上时拒绝", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusPending
		order.ForeignAmount = i64Ptr(2800)
		order.ForeignCurrency = strPtr("USD")
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		_, err := svc.MarkPaid(order.OrderNo, strPtr("TRADE4"), 100, "USD")
		requireErrStatus(t, err, 409)
	})

	t.Run("重复回调是 no-op（已 Paid 就跳过）", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusPaid
		order.Fee = 200
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		if _, err := svc.MarkPaid(order.OrderNo, strPtr("TRADE5"), 20000, ""); err != nil {
			t.Fatalf("MarkPaid (duplicate): %v", err)
		}

		// update 没被调用过 —— 跳过分支不带 thirdPartyTradeNo 落库
		row := reloadOrder(t, store, order.OrderNo)
		if row.ThirdPartyTradeNo != nil {
			t.Errorf("row.thirdPartyTradeNo = %v, want nil (update must not run)", row.ThirdPartyTradeNo)
		}
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d)", row.Status, db.StatusPaid)
		}
	})

	t.Run("订单不存在时 404", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		svc, _ := createPaymentService(store)

		_, err := svc.MarkPaid("NOPE", strPtr("TRADE6"), 20000, "")
		requireErrStatus(t, err, 404)
	})
}

func TestPaymentServiceMarkRefundFailed(t *testing.T) {
	t.Run("把 Refunding 退回 Paid，避免永久卡住", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusRefunding
		insertOrder(t, store, order)
		svc, _ := createPaymentService(store)

		reason := "upstream rejected"
		got, err := svc.MarkRefundFailed(order.OrderNo, &reason)
		if err != nil {
			t.Fatalf("MarkRefundFailed: %v", err)
		}
		if got.Status != db.StatusPaid {
			t.Errorf("status = %d, want Paid(%d)", got.Status, db.StatusPaid)
		}
		row := reloadOrder(t, store, order.OrderNo)
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d)", row.Status, db.StatusPaid)
		}
	})

	t.Run("不是 Refunding 时不动", func(t *testing.T) {
		store := newTestStore(t)
		seedGroup1(t, store)
		order := createMockPaymentOrder()
		order.Status = db.StatusPaid
		insertOrder(t, store, order)
		before := reloadOrder(t, store, order.OrderNo).UpdatedAt
		svc, _ := createPaymentService(store)

		// 留出毫秒级间隔：真 update 会把 updatedAt 刷成当前时间
		time.Sleep(10 * time.Millisecond)
		if _, err := svc.MarkRefundFailed(order.OrderNo, nil); err != nil {
			t.Fatalf("MarkRefundFailed: %v", err)
		}

		row := reloadOrder(t, store, order.OrderNo)
		if row.Status != db.StatusPaid {
			t.Errorf("row.status = %d, want Paid(%d)", row.Status, db.StatusPaid)
		}
		if !row.UpdatedAt.Equal(before) {
			t.Errorf("updatedAt changed %v → %v, want untouched (update must not run)", before, row.UpdatedAt)
		}
	})
}
