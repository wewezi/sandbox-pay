package services

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/signature"
)

// NotifyEnqueuer 只准备记录；订单状态机负责在事务内入队。
type NotifyEnqueuer interface {
	PrepareNotification(notifyURL string, notification PaymentNotification, signingKey string) (db.InsertNotifyInput, error)
}

// OrderCaller 下单方。
//
// GroupID decides ownership; APIKeyID records the bearer credential used.
// Callback signing uses a separate project secret saved on the order.
type OrderCaller struct {
	GroupID        int64
	APIKeyID       *int64
	ClientIP       *string
	ClientLocation *string
}

// CreateOrderInput 下单入参。Amount 是元，边界上立刻转整数分。
type CreateOrderInput struct {
	Amount          float64
	ProductName     string
	PayMethod       string
	ExpireMinutes   *int
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
	CallbackSignatureVersion int     `json:"callbackSignatureVersion"`
	OrderNo                  string  `json:"orderNo"`
	ExternalOrderNo          *string `json:"externalOrderNo,omitempty"`
	Amount                   float64 `json:"amount"`
	RefundedAmount           float64 `json:"refundedAmount"`
	Fee                      float64 `json:"fee"`
	SettleAmount             float64 `json:"settleAmount"`
	Status                   int     `json:"status"`
	PayMethod                string  `json:"payMethod"`
	ProductName              string  `json:"productName"`
	PayAt                    *string `json:"payAt,omitempty"`
	ThirdPartyTradeNo        *string `json:"thirdPartyTradeNo,omitempty"`
	CreatedAt                string  `json:"createdAt"`
	ExpireAt                 string  `json:"expireAt"`
}

// PaymentService 订单状态机：建单、查单、以及三个 mark* 入账原语。
//
// 状态判定都在 Store.Do 的写事务里完成 —— SQLite 是单写者，写事务之间
// 本来就串行，「读-判状态-写」天然原子，不需要行锁。
// 通知记录与订单一起提交，网络投递由 NotifyService 在事务外执行。
type PaymentService struct {
	store             *db.Store
	notify            NotifyEnqueuer
	fees              *FeeCalculator
	log               *logger.Logger
	channelMu         sync.Mutex
	channelOperations map[string]*cashierChannelOperation
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
	expireMinutes := 30
	if dto.ExpireMinutes != nil {
		expireMinutes = *dto.ExpireMinutes
		if expireMinutes < 1 || expireMinutes > 120 {
			return nil, httperr.BadRequest("expireMinutes 必须是 1–120 的整数分钟")
		}
	}
	if math.IsNaN(dto.Amount) || math.IsInf(dto.Amount, 0) || dto.Amount < 0.01 || dto.Amount >= float64(math.MaxInt64)/100 {
		return nil, httperr.BadRequest("amount 必须是有效的正数金额")
	}
	// 入参是元，边界上转成整数分。
	amountCents := money.YuanToCents(dto.Amount)

	var order *db.PaymentOrder
	err := s.store.Do(func(tx *db.TxScope) error {
		var group *db.Group
		if caller.APIKeyID != nil || strDeref(dto.NotifyURL, "") != "" {
			var err error
			group, err = tx.Groups.FindByID(caller.GroupID)
			if err != nil {
				return err
			}
			if group == nil || !group.IsActive {
				return httperr.BadRequest("项目不存在或已停用")
			}
		}
		if caller.APIKeyID != nil {
			key, err := tx.ApiKeys.FindByID(*caller.APIKeyID)
			if err != nil {
				return err
			}
			if key == nil || !key.IsActive || key.GroupID != caller.GroupID {
				return httperr.Unauthorized("令牌不存在、已停用或不属于当前项目")
			}
		}
		if strDeref(dto.ExternalOrderNo, "") != "" {
			existing, err := tx.Orders.FindLatestByExternalOrderNo(caller.GroupID, *dto.ExternalOrderNo)
			if err != nil {
				return err
			}
			if existing != nil {
				if existing.Amount != amountCents {
					// 同一个幂等键换了金额：调用方的 bug，不是资源不存在，400 更准确。
					return httperr.BadRequestf(
						"externalOrderNo %s already exists with a different amount", *dto.ExternalOrderNo)
				}
				if existing.PayMethod != dto.PayMethod {
					return httperr.Conflict("相同 externalOrderNo 不能更换支付渠道")
				}
				order = existing
				return nil
			}
		}

		now := time.Now()
		groupID := caller.GroupID
		order = &db.PaymentOrder{
			ID:               randomHex(16),
			OrderNo:          signature.CreateOrderNo("PAY", 12),
			Amount:           amountCents,
			ProductName:      dto.ProductName,
			PayMethod:        dto.PayMethod,
			Status:           db.StatusPending,
			CreatedAt:        now,
			UpdatedAt:        now,
			ExpireAt:         now.Add(time.Duration(expireMinutes) * time.Minute),
			ExternalOrderNo:  dto.ExternalOrderNo,
			ReturnURL:        dto.ReturnURL,
			NotifyURL:        dto.NotifyURL,
			GroupID:          &groupID,
			APIKeyID:         caller.APIKeyID,
			ClientIP:         caller.ClientIP,
			ClientLocation:   caller.ClientLocation,
			SignatureVersion: 2,
			// userId: 0 = 未归属（没有终端用户体系）
			UserID:         0,
			RefundedAmount: 0,
			ChannelCost:    0,
			Fee:            0,
			SettleAmount:   0,
		}
		if strDeref(dto.NotifyURL, "") != "" {
			if group.WebhookSecret == "" {
				return httperr.BadRequest("项目未配置回调签名密钥")
			}
			secret := group.WebhookSecret
			order.SigningSecret = &secret
		}
		return tx.Orders.Insert(order)
	})
	if err != nil {
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

// SwitchPaymentMethod changes only a pending order's current selection. A
// PayPal binding and its amount snapshot survive selection changes for retry.
func (s *PaymentService) SwitchPaymentMethod(orderNo, method string) error {
	if method != "alipay" && method != "paypal" && method != "native" {
		return httperr.BadRequest("无效支付渠道")
	}
	return s.store.Do(func(tx *db.TxScope) error {
		order, err := tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if order == nil {
			return httperr.NotFound("订单不存在")
		}
		if order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
			return httperr.Conflict("订单状态不允许切换渠道")
		}
		if order.PayMethod == method {
			return nil
		}
		if !CashierChannelAllowed(order, method) {
			_, reason := CashierChannelCapabilities(order)
			return httperr.Conflict(reason)
		}
		return tx.Orders.Update(orderNo, map[string]any{"payMethod": method})
	})
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
		CallbackSignatureVersion: order.SignatureVersion,
		OrderNo:                  order.OrderNo,
		ExternalOrderNo:          order.ExternalOrderNo,
		Amount:                   money.ToYuan(order.Amount),
		RefundedAmount:           money.ToYuan(order.RefundedAmount),
		Fee:                      money.ToYuan(order.Fee),
		SettleAmount:             money.ToYuan(order.SettleAmount),
		Status:                   order.Status,
		PayMethod:                order.PayMethod,
		ProductName:              order.ProductName,
		PayAt:                    jsTimePtr(order.PayAt),
		ThirdPartyTradeNo:        order.ThirdPartyTradeNo,
		CreatedAt:                JsTime(order.CreatedAt),
		ExpireAt:                 JsTime(order.ExpireAt),
	}, nil
}

// MarkPaid 网关确认收款后入账。
//
// 调用方必须传上游真实回报的金额和币种：拿它和订单原值比对，
// 挡掉伪造/篡改金额的回调。重复回调不改状态，也不重复入队。
func (s *PaymentService) MarkPaid(orderNo string, thirdPartyTradeNo *string, paidAmountCents int64, paidCurrency string) (*db.PaymentOrder, error) {
	return s.markPaid(orderNo, thirdPartyTradeNo, paidAmountCents, paidCurrency, "")
}

// MarkPaidForChannel records the first verified payment's actual source in
// the same transaction as settlement and fee calculation. The payer may have
// changed the pending selection after an earlier provider link was issued.
func (s *PaymentService) MarkPaidForChannel(channel, orderNo string, thirdPartyTradeNo *string, paidAmountCents int64, paidCurrency string) (*db.PaymentOrder, error) {
	if channel != "native" && channel != "alipay" && channel != "paypal" {
		return nil, httperr.BadRequest("无效支付来源渠道")
	}
	return s.markPaid(orderNo, thirdPartyTradeNo, paidAmountCents, paidCurrency, channel)
}

func (s *PaymentService) markPaid(orderNo string, thirdPartyTradeNo *string, paidAmountCents int64, paidCurrency, sourceChannel string) (*db.PaymentOrder, error) {
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

		if sourceChannel != "" {
			current.PayMethod = sourceChannel
		}
		fees := s.fees.Calculate(current.PayMethod, current.Amount)
		payAt := time.Now()
		patch := map[string]any{
			"payMethod": current.PayMethod,
			"fee":       fees.Fee, "channelCost": fees.ChannelCost, "settleAmount": fees.SettleAmount,
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
		return s.enqueueStateNotification(tx, current, "paid", current.ThirdPartyTradeNo)
	})
	if err != nil {
		return nil, err
	}

	return order, nil
}

func (s *PaymentService) enqueueStateNotification(tx *db.TxScope, order *db.PaymentOrder, status string, tradeNo *string) error {
	if order.NotifyURL == nil || *order.NotifyURL == "" {
		return nil
	}
	if order.SigningSecret == nil || *order.SigningSecret == "" {
		if order.SignatureVersion >= 2 {
			return fmt.Errorf("order callback signing secret is missing")
		}
		return nil
	}
	payAt := time.Now()
	if order.PayAt != nil {
		payAt = *order.PayAt
	}
	in, err := s.notify.PrepareNotification(*order.NotifyURL, PaymentNotification{
		OrderNo: order.OrderNo, ExternalOrderNo: order.ExternalOrderNo, Status: status,
		Amount: money.ToYuan(order.Amount), PayMethod: order.PayMethod, PayAt: JsTime(payAt), ThirdPartyTradeNo: tradeNo,
	}, *order.SigningSecret)
	if err != nil {
		return err
	}
	if order.SignatureVersion >= 2 {
		var meta map[string]any
		if err := json.Unmarshal([]byte(in.Signature), &meta); err != nil {
			return err
		}
		if meta == nil {
			return fmt.Errorf("invalid callback signature metadata")
		}
		meta["version"] = order.SignatureVersion
		encoded, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		in.Signature = string(encoded)
	}
	return tx.Notifies.Insert(in)
}

// MarkRefunded 全额或部分退款落地。调用方必须已经在上游执行完退款并拿到 refundTradeNo。
//
// 按 (orderNo, refundTradeNo) 幂等 —— 同一个 tradeNo 重放是 no-op。
// groupIDForCheck 非 nil 时做分组隔离，不匹配一律报「订单不存在」，不泄漏他人订单。
func (s *PaymentService) MarkRefunded(orderNo string, refundedCents int64, refundTradeNo string, groupIDForCheck *int64) (*db.PaymentOrder, error) {
	return s.completeRefund(orderNo, refundedCents, refundTradeNo, groupIDForCheck, "")
}

func (s *PaymentService) completeRefund(orderNo string, refundedCents int64, refundTradeNo string, groupIDForCheck *int64, requestID string) (*db.PaymentOrder, error) {
	if refundedCents <= 0 || refundTradeNo == "" {
		return nil, httperr.BadRequest("退款金额和上游退款单号必须有效")
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
		if groupIDForCheck != nil && (current.GroupID == nil || *current.GroupID != *groupIDForCheck) {
			return httperr.NotFound("订单不存在")
		}
		previous, err := tx.Refunds.FindByTradeNo(orderNo, refundTradeNo)
		if err != nil {
			return err
		}
		if requestID != "" && previous != nil && previous.ID != requestID {
			return httperr.Conflict("上游退款单号已属于另一退款请求，需要人工核对")
		}
		// 兼容升级前仅在订单上保存的最后一笔退款。
		if (previous != nil && previous.Status == db.RefundCompleted) || (requestID == "" && current.RefundTradeNo != nil && *current.RefundTradeNo == refundTradeNo) {
			order = current
			return nil
		}
		if current.Status != db.StatusPaid && current.Status != db.StatusRefunding {
			return httperr.Conflict(fmt.Sprintf("订单状态不允许退款: %d", current.Status))
		}
		newRefunded := current.RefundedAmount + refundedCents
		if refundedCents > current.Amount-current.RefundedAmount {
			return httperr.BadRequest("退款累计金额超过订单金额")
		}
		status := db.StatusPaid
		if newRefunded == current.Amount {
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
		if requestID == "" {
			if err := tx.Refunds.Insert(&db.RefundOperation{ID: "RF_" + randomHex(16), OrderNo: orderNo, Amount: refundedCents, TradeNo: &refundTradeNo, Status: db.RefundCompleted}); err != nil {
				return err
			}
		} else {
			op, err := tx.Refunds.FindByID(requestID)
			if err != nil {
				return err
			}
			if op == nil || op.OrderNo != orderNo || op.Amount != refundedCents || op.Status == db.RefundCompleted || op.Status == db.RefundFailed {
				return httperr.Conflict("退款请求状态与完成记录不匹配")
			}
			if err := tx.Refunds.Update(requestID, db.RefundCompleted, &refundTradeNo); err != nil {
				return err
			}
		}

		s.log.Log(fmt.Sprintf("Order %s refund applied: %d cents (total refunded %d, status %d)",
			orderNo, refundedCents, newRefunded, current.Status))
		order = current
		notifyStatus := "partial_refunded"
		if status == db.StatusRefunded {
			notifyStatus = "refunded"
		}
		return s.enqueueStateNotification(tx, current, notifyStatus, &refundTradeNo)
	})
	if err != nil {
		return nil, err
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
		if current.Status != db.StatusPaid {
			return httperr.Conflict("订单已有在途退款或不处于已支付状态")
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
