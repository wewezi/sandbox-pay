package services

import (
	"fmt"
	"strings"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/signature"
)

// NotifyEnqueuer PaymentService 只用到入队这一件事，声明成接口方便塞假的。
type NotifyEnqueuer interface {
	EnqueueNotification(notifyURL string, notification PaymentNotification, signingKey string) error
}

// OrderCaller 下单方。
//
// GroupID 决定订单归属和数据隔离；APIKeyID 记下用的是哪把 Key，
// 回调签名认它。后台/公开测试下单没有 Key 上下文，APIKeyID 是空的 ——
// 那些单也不带 notifyUrl，走不到签名。
type OrderCaller struct {
	GroupID  int64
	APIKeyID *int64
}

// CreateOrderInput 下单入参。Amount 是元，边界上立刻转整数分。
type CreateOrderInput struct {
	Amount          float64
	ProductName     string
	PayMethod       string
	ExternalOrderNo *string
	ReturnURL       *string
	NotifyURL       *string
}

// OrderResult 下单/查单的最小回执。
type OrderResult struct {
	OrderNo  string  `json:"orderNo"`
	Amount   float64 `json:"amount"`
	ExpireAt string  `json:"expireAt"`
}

// OrderQueryResult 订单详情（网关侧查单）。
type OrderQueryResult struct {
	OrderNo           string  `json:"orderNo"`
	ExternalOrderNo   *string `json:"externalOrderNo,omitempty"`
	Amount            float64 `json:"amount"`
	Status            int     `json:"status"`
	PayMethod         string  `json:"payMethod"`
	ProductName       string  `json:"productName"`
	ReturnURL         *string `json:"returnUrl,omitempty"`
	PayAt             *string `json:"payAt,omitempty"`
	ThirdPartyTradeNo *string `json:"thirdPartyTradeNo,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	ExpireAt          string  `json:"expireAt"`
}

// OrderGroupQueryResult 订单详情（带结算金额，给分组隔离的查单用）。
type OrderGroupQueryResult struct {
	OrderNo           string  `json:"orderNo"`
	ExternalOrderNo   *string `json:"externalOrderNo,omitempty"`
	Amount            float64 `json:"amount"`
	RefundedAmount    float64 `json:"refundedAmount"`
	Fee               float64 `json:"fee"`
	SettleAmount      float64 `json:"settleAmount"`
	Status            int     `json:"status"`
	PayMethod         string  `json:"payMethod"`
	ProductName       string  `json:"productName"`
	PayAt             *string `json:"payAt,omitempty"`
	ThirdPartyTradeNo *string `json:"thirdPartyTradeNo,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	ExpireAt          string  `json:"expireAt"`
}

// PaymentService 订单状态机：建单、查单、以及三个 mark* 入账原语。
//
// 状态判定都在 Store.Do 的写事务里完成 —— SQLite 是单写者，写事务之间
// 本来就串行，「读-判状态-写」天然原子，不需要行锁。
// 通知入队这类动作一律放到事务提交之后。
type PaymentService struct {
	store  *db.Store
	notify NotifyEnqueuer
	fees   *FeeCalculator
	log    *logger.Logger
}

// NewPaymentService 构造。
func NewPaymentService(store *db.Store, notify NotifyEnqueuer, fees *FeeCalculator) *PaymentService {
	return &PaymentService{
		store:  store,
		notify: notify,
		fees:   fees,
		log:    logger.New("PaymentService"),
	}
}

// CreateOrder 建单。
//
// 幂等：只按 (groupID, externalOrderNo) 去重。同分组换把 Key 重下
// 同一个 externalOrderNo 仍然是同一笔，不按 Key 分桶。
func (s *PaymentService) CreateOrder(caller OrderCaller, dto CreateOrderInput) (*OrderResult, error) {
	// 入参是元，边界上转成整数分。
	amountCents := money.YuanToCents(dto.Amount)

	if strDeref(dto.ExternalOrderNo, "") != "" {
		existing, err := s.store.Orders.FindLatestByExternalOrderNo(caller.GroupID, *dto.ExternalOrderNo)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if existing.Amount != amountCents {
				// 同一个幂等键换了金额：调用方的 bug，不是资源不存在，400 更准确。
				return nil, httperr.BadRequestf(
					"externalOrderNo %s already exists with a different amount", *dto.ExternalOrderNo)
			}
			if existing.Status == db.StatusPending && existing.ExpireAt.After(time.Now()) &&
				existing.PayMethod != dto.PayMethod {
				if err := s.store.Orders.Update(existing.OrderNo, map[string]any{"payMethod": dto.PayMethod}); err != nil {
					return nil, err
				}
			}
			return buildOrderResult(existing), nil
		}
	}

	now := time.Now()
	groupID := caller.GroupID
	order := &db.PaymentOrder{
		ID:              randomHex(16),
		OrderNo:         signature.CreateOrderNo("PAY", 12),
		Amount:          amountCents,
		ProductName:     dto.ProductName,
		PayMethod:       dto.PayMethod,
		Status:          db.StatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpireAt:        now.Add(30 * time.Minute),
		ExternalOrderNo: dto.ExternalOrderNo,
		ReturnURL:       dto.ReturnURL,
		NotifyURL:       dto.NotifyURL,
		GroupID:         &groupID,
		APIKeyID:        caller.APIKeyID,
		// userId: 0 = 未归属（没有终端用户体系）
		UserID:         0,
		RefundedAmount: 0,
		ChannelCost:    0,
		Fee:            0,
		SettleAmount:   0,
	}
	if err := s.store.Orders.Insert(order); err != nil {
		return nil, err
	}
	return buildOrderResult(order), nil
}

// QueryOrder 查单（不限分组）。
func (s *PaymentService) QueryOrder(orderNo string) (*OrderQueryResult, error) {
	order, err := s.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	return &OrderQueryResult{
		OrderNo:           order.OrderNo,
		ExternalOrderNo:   order.ExternalOrderNo,
		Amount:            money.ToYuan(order.Amount),
		Status:            order.Status,
		PayMethod:         order.PayMethod,
		ProductName:       order.ProductName,
		ReturnURL:         order.ReturnURL,
		PayAt:             jsTimePtr(order.PayAt),
		ThirdPartyTradeNo: order.ThirdPartyTradeNo,
		CreatedAt:         JsTime(order.CreatedAt),
		ExpireAt:          JsTime(order.ExpireAt),
	}, nil
}

// QueryByGroup 查单，限定在某个分组内（别的分组查不到这一单）。
func (s *PaymentService) QueryByGroup(groupID int64, orderNo string) (*OrderGroupQueryResult, error) {
	order, err := s.store.Orders.FindByOrderNoAndGroup(orderNo, groupID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	return &OrderGroupQueryResult{
		OrderNo:           order.OrderNo,
		ExternalOrderNo:   order.ExternalOrderNo,
		Amount:            money.ToYuan(order.Amount),
		RefundedAmount:    money.ToYuan(order.RefundedAmount),
		Fee:               money.ToYuan(order.Fee),
		SettleAmount:      money.ToYuan(order.SettleAmount),
		Status:            order.Status,
		PayMethod:         order.PayMethod,
		ProductName:       order.ProductName,
		PayAt:             jsTimePtr(order.PayAt),
		ThirdPartyTradeNo: order.ThirdPartyTradeNo,
		CreatedAt:         JsTime(order.CreatedAt),
		ExpireAt:          JsTime(order.ExpireAt),
	}, nil
}

// MarkPaid 网关确认收款后入账。
//
// 调用方必须传上游真实回报的金额和币种：拿它和订单原值比对，
// 挡掉伪造/篡改金额的回调。已支付的订单重复调用会再次入队通知 ——
// 原版就是这个 quirk，接入方自己按 orderNo 去重。
func (s *PaymentService) MarkPaid(orderNo string, thirdPartyTradeNo *string, paidAmountCents int64, paidCurrency string) (*db.PaymentOrder, error) {
	if paidCurrency == "" {
		paidCurrency = "CNY"
	}

	var order *db.PaymentOrder
	err := s.store.Do(func(tx *db.TxScope) error {
		current, err := tx.Orders.FindByOrderNo(orderNo, true)
		if err != nil {
			return err
		}
		if current == nil {
			return httperr.NotFound("订单不存在")
		}
		if current.Status != db.StatusPending {
			s.log.Warn(fmt.Sprintf("Order %s already in status %d, skip", orderNo, current.Status))
			order = current
			return nil
		}

		expectedCents := current.Amount
		tolerance := int64(1)
		currency := strings.ToUpper(paidCurrency)

		var amountValid bool
		switch {
		case currency == "CNY":
			amountValid = abs64(paidAmountCents-expectedCents) <= tolerance
		case current.ForeignAmount != nil && currency == strings.ToUpper(strDeref(current.ForeignCurrency, "")):
			amountValid = abs64(paidAmountCents-*current.ForeignAmount) <= tolerance
		}
		if !amountValid {
			s.log.Error(fmt.Sprintf(
				"Amount mismatch on order %s: gateway reported %d %s (cents), expected %d CNY / %s %s",
				orderNo, paidAmountCents, currency, expectedCents,
				tsOptInt(current.ForeignAmount), strDeref(current.ForeignCurrency, "")))
			return httperr.Conflict(fmt.Sprintf("订单 %s 金额不匹配，拒绝入账", orderNo))
		}

		fees := s.fees.Calculate(current.PayMethod, current.Amount)
		payAt := time.Now()
		patch := map[string]any{
			"fee": fees.Fee, "channelCost": fees.ChannelCost, "settleAmount": fees.SettleAmount,
			"status": db.StatusPaid, "payAt": payAt, "thirdPartyTradeNo": thirdPartyTradeNo,
		}
		if err := tx.Orders.Update(orderNo, patch); err != nil {
			return err
		}
		current.Fee = fees.Fee
		current.ChannelCost = fees.ChannelCost
		current.SettleAmount = fees.SettleAmount
		current.Status = db.StatusPaid
		current.PayAt = &payAt
		current.ThirdPartyTradeNo = thirdPartyTradeNo

		s.log.Log(fmt.Sprintf("Order %s marked as paid (%d %s cents); fee=%d cost=%d settle=%d",
			orderNo, paidAmountCents, paidCurrency, fees.Fee, fees.ChannelCost, fees.SettleAmount))
		order = current
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 事务提交后再入队（前端状态走轮询，不推 WebSocket）。
	// 判 status==Paid 是为了挡住上面的「重复调用跳过」分支之外的路径 ——
	// 但原版只看 status，所以已支付的重复回调确实会再入队一次，照抄。
	if order.Status == db.StatusPaid && order.NotifyURL != nil && *order.NotifyURL != "" && order.APIKey != nil {
		payAt := time.Now()
		if order.PayAt != nil {
			payAt = *order.PayAt
		}
		if err := s.notify.EnqueueNotification(*order.NotifyURL, PaymentNotification{
			OrderNo:           order.OrderNo,
			ExternalOrderNo:   order.ExternalOrderNo,
			Status:            "paid",
			Amount:            money.ToYuan(order.Amount),
			PayMethod:         order.PayMethod,
			PayAt:             JsTime(payAt),
			ThirdPartyTradeNo: thirdPartyTradeNo,
		}, order.APIKey.Key); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// MarkRefunded 全额或部分退款落地。调用方必须已经在上游执行完退款并拿到 refundTradeNo。
//
// 按 (orderNo, refundTradeNo) 幂等 —— 同一个 tradeNo 重放是 no-op。
// groupIDForCheck 非 nil 时做分组隔离，不匹配一律报「订单不存在」，不泄漏他人订单。
func (s *PaymentService) MarkRefunded(orderNo string, refundedCents int64, refundTradeNo string, groupIDForCheck *int64) (*db.PaymentOrder, error) {
	var order *db.PaymentOrder
	err := s.store.Do(func(tx *db.TxScope) error {
		current, err := tx.Orders.FindByOrderNo(orderNo, true)
		if err != nil {
			return err
		}
		if current == nil {
			return httperr.NotFound("订单不存在")
		}
		if groupIDForCheck != nil && (current.GroupID == nil || *current.GroupID != *groupIDForCheck) {
			return httperr.NotFound("订单不存在")
		}
		// 幂等：同一个退款 tradeNo 重放，已经退完了就直接返回。
		if current.RefundTradeNo != nil && *current.RefundTradeNo == refundTradeNo && current.Status == db.StatusRefunded {
			order = current
			return nil
		}
		if current.Status != db.StatusPaid && current.Status != db.StatusRefunding {
			return httperr.Conflict(fmt.Sprintf("订单状态不允许退款: %d", current.Status))
		}
		newRefunded := current.RefundedAmount + refundedCents
		if newRefunded-current.Amount > 1 {
			return httperr.BadRequest("退款累计金额超过订单金额")
		}
		status := db.StatusPaid
		if abs64(newRefunded-current.Amount) <= 1 {
			status = db.StatusRefunded
		} else {
			// 部分退款落地：Refunding 只表示「退款在途」，退成了就得回到 Paid，
			// 否则剩余余额再也退不出来（lookupOrder 只放行 Paid）。
		}
		refundAt := time.Now()
		patch := map[string]any{
			"refundedAmount": newRefunded,
			"refundTradeNo":  refundTradeNo,
			"refundAt":       refundAt,
			"status":         status,
		}
		if err := tx.Orders.Update(orderNo, patch); err != nil {
			return err
		}
		current.RefundedAmount = newRefunded
		current.RefundTradeNo = &refundTradeNo
		current.RefundAt = &refundAt
		current.Status = status

		s.log.Log(fmt.Sprintf("Order %s refund applied: %d cents (total refunded %d, status %d)",
			orderNo, refundedCents, newRefunded, current.Status))
		order = current
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 事务提交后再入队。原版这里只判 notifyUrl && apiKey（没有 status 判断），
	// 幂等重放也会再入队一次，照抄。
	if order.NotifyURL != nil && *order.NotifyURL != "" && order.APIKey != nil {
		status := "partial_refunded"
		if order.Status == db.StatusRefunded {
			status = "refunded"
		}
		payAt := time.Now()
		if order.PayAt != nil {
			payAt = *order.PayAt
		}
		if err := s.notify.EnqueueNotification(*order.NotifyURL, PaymentNotification{
			OrderNo:           order.OrderNo,
			ExternalOrderNo:   order.ExternalOrderNo,
			Status:            status,
			Amount:            money.ToYuan(order.Amount),
			PayMethod:         order.PayMethod,
			PayAt:             JsTime(payAt),
			ThirdPartyTradeNo: &refundTradeNo,
		}, order.APIKey.Key); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// MarkRefunding 标记退款在途（上游已受理，终态未定）。
func (s *PaymentService) MarkRefunding(orderNo string) (*db.PaymentOrder, error) {
	var order *db.PaymentOrder
	err := s.store.Do(func(tx *db.TxScope) error {
		current, err := tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if current == nil {
			return httperr.NotFound("订单不存在")
		}
		if current.Status == db.StatusPaid {
			if err := tx.Orders.Update(orderNo, map[string]any{"status": db.StatusRefunding}); err != nil {
				return err
			}
			current.Status = db.StatusRefunding
		}
		order = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// GetOrderForRefund 退款前的定位：分组隔离 + 只放行已支付的单。
func (s *PaymentService) GetOrderForRefund(orderNo string, groupID int64) (*db.PaymentOrder, error) {
	order, err := s.store.Orders.FindByOrderNoAndGroup(orderNo, groupID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if order.Status != db.StatusPaid {
		return nil, httperr.Conflict(fmt.Sprintf("订单状态不允许退款: %d", order.Status))
	}
	return order, nil
}

// MarkFailed 支付失败落地。非 Pending 的单不动，直接返回当前状态。
func (s *PaymentService) MarkFailed(orderNo string, reason *string) (*db.PaymentOrder, error) {
	var order *db.PaymentOrder
	err := s.store.Do(func(tx *db.TxScope) error {
		current, err := tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if current == nil {
			return httperr.NotFound("订单不存在")
		}
		if current.Status != db.StatusPending {
			order = current
			return nil
		}
		if err := tx.Orders.Update(orderNo, map[string]any{"status": db.StatusFailed}); err != nil {
			return err
		}
		current.Status = db.StatusFailed
		order = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Order %s marked as failed: %s", orderNo, tsOpt(reason)))
	return order, nil
}

// MarkRefundFailed 上游拒绝或处理失败时把 Refunding 退回 Paid，
// 避免订单永久卡在 Refunding。非 Refunding 的单不动。
func (s *PaymentService) MarkRefundFailed(orderNo string, reason *string) (*db.PaymentOrder, error) {
	var order *db.PaymentOrder
	err := s.store.Do(func(tx *db.TxScope) error {
		current, err := tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if current == nil {
			return httperr.NotFound("订单不存在")
		}
		if current.Status != db.StatusRefunding {
			order = current
			return nil
		}
		if err := tx.Orders.Update(orderNo, map[string]any{"status": db.StatusPaid}); err != nil {
			return err
		}
		current.Status = db.StatusPaid
		order = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Warn(fmt.Sprintf("Order %s refund failed, rolled back to Paid: %s", orderNo, tsOpt(reason)))
	return order, nil
}

func buildOrderResult(order *db.PaymentOrder) *OrderResult {
	return &OrderResult{
		OrderNo:  order.OrderNo,
		Amount:   money.ToYuan(order.Amount),
		ExpireAt: JsTime(order.ExpireAt),
	}
}
