package services

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
)

// Refunder 支付宝退款。返回上游退款单号，空串 = 失败（网关自己吞错误，不抛）。
type Refunder interface {
	Refund(orderNo string, refundCents int64, reason string) string
}

// PayPalRefunder PayPal 退款。返回上游退款单号，空串 = 失败。
type PayPalRefunder interface {
	Refund(captureID string, refundCents int64, currency string, reason string) string
}

// RefundInput 退款入参。Amount 是元。
type RefundInput struct {
	OrderNo string
	Amount  float64
	Reason  *string
}

// RefundOptions 两个入口（接入方 Key / 后台 JWT）的差异都在这里。
type RefundOptions struct {
	// GroupID 传了就把订单查询限定到这个分组（网关调用路径）。
	GroupID *int64
	// Actor 谁发起的，只用于日志。
	Actor string
	// IP 客户端 IP，只用于日志。
	IP *string
}

// RefundResult 退款回执。
type RefundResult struct {
	OrderNo        string  `json:"orderNo"`
	RefundedAmount float64 `json:"refundedAmount"`
	RefundTradeNo  string  `json:"refundTradeNo"`
	Currency       string  `json:"currency"`
}

// RefundService 与渠道无关的退款编排。
//
// `/gateway/refund`（接入方 Key）和 `/admin/refund`（JWT）共用这条流程：
// 校验余额 → 标记退款中 → 派发到上游网关 → 落地退款 → 记日志。
type RefundService struct {
	log      *logger.Logger
	store    *db.Store
	payments *PaymentService
	alipay   Refunder
	paypal   PayPalRefunder
}

// NewRefundService 构造。
func NewRefundService(store *db.Store, payments *PaymentService, alipay Refunder, paypal PayPalRefunder) *RefundService {
	return &RefundService{
		log:      logger.New("RefundService"),
		store:    store,
		payments: payments,
		alipay:   alipay,
		paypal:   paypal,
	}
}

// Execute 走完整退款流程。
func (s *RefundService) Execute(dto RefundInput, opts RefundOptions) (*RefundResult, error) {
	if dto.OrderNo == "" || dto.Amount == 0 {
		return nil, httperr.BadRequest("orderNo 和 amount 为必填项")
	}

	order, err := s.lookupOrder(dto.OrderNo, opts.GroupID)
	if err != nil {
		return nil, err
	}

	refundCents := money.YuanToCents(dto.Amount)
	remainingCents := order.Amount - order.RefundedAmount
	if refundCents-remainingCents > 1 {
		return nil, httperr.BadRequest(fmt.Sprintf(
			"Refund amount %v exceeds refundable balance %.2f",
			strconv.FormatFloat(dto.Amount, 'f', -1, 64),
			float64(remainingCents)/100))
	}

	if _, err := s.payments.MarkRefunding(dto.OrderNo); err != nil {
		return nil, err
	}

	// 上游退款失败就回滚成 Paid，别把订单永久卡在 Refunding。
	refundTradeNo, currency, dispatchErr := s.dispatch(order, refundCents, strDeref(dto.Reason, ""))
	if dispatchErr != nil {
		reason := dispatchErr.Error()
		s.log.Error(fmt.Sprintf("Upstream refund failed for %s: %s", dto.OrderNo, reason))
		s.payments.MarkRefundFailed(dto.OrderNo, &reason)
		return nil, httperr.BadRequest("上游退款失败: " + reason)
	}
	if refundTradeNo == "" {
		reason := "Upstream gateway returned null (refund not processed)"
		s.log.Error(fmt.Sprintf("Refund returned null for %s", dto.OrderNo))
		s.payments.MarkRefundFailed(dto.OrderNo, &reason)
		return nil, httperr.BadRequest("上游退款失败: 网关未处理退款请求")
	}

	if _, err := s.payments.MarkRefunded(dto.OrderNo, refundCents, refundTradeNo, opts.GroupID); err != nil {
		return nil, err
	}

	actor := opts.Actor
	if actor == "" {
		actor = "unknown"
	}
	s.log.Log(fmt.Sprintf("Refund done %s %v %s by %s tradeNo=%s",
		dto.OrderNo, strconv.FormatFloat(dto.Amount, 'f', -1, 64), currency, actor, refundTradeNo))

	return &RefundResult{
		OrderNo:        dto.OrderNo,
		RefundedAmount: dto.Amount,
		RefundTradeNo:  refundTradeNo,
		Currency:       currency,
	}, nil
}

// lookupOrder 定位要退的订单。经接入网关调用时限定 groupID；
// 后台路径不限定，方便面板退任意分组的订单。
func (s *RefundService) lookupOrder(orderNo string, groupID *int64) (*db.PaymentOrder, error) {
	if groupID != nil {
		return s.payments.GetOrderForRefund(orderNo, *groupID)
	}
	order, err := s.store.Orders.FindByOrderNo(orderNo, false)
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

// dispatch 调各渠道自己的退款接口，返回上游退款单号和退款币种。
//
// 只有两种情况会返回 error：缺 captureId、渠道不认。网关自身失败走空串分支。
func (s *RefundService) dispatch(order *db.PaymentOrder, refundCents int64, reason string) (string, string, error) {
	switch order.PayMethod {
	case "alipay":
		return s.alipay.Refund(order.OrderNo, refundCents, reason), "CNY", nil
	case "paypal":
		if strDeref(order.ThirdPartyTradeNo, "") == "" {
			return "", "", httperr.BadRequest("订单缺少 PayPal captureId，无法退款")
		}
		currency := strDeref(order.ForeignCurrency, "")
		if currency == "" {
			currency = "USD"
		}
		// PayPal 按外币（美分）退。部分退款时 dto.amount 是原始人民币元，
		// 按下单时记下的汇率折算成美分。
		// 至少 1 分：小额按比例折算会被 round 抹成 0，PayPal 直接报
		// CANNOT_BE_ZERO_OR_NEGATIVE。
		paypalCents := refundCents
		if order.ForeignAmount != nil && *order.ForeignAmount != 0 && order.Amount != 0 {
			proRated := int64(math.Round(float64(refundCents) / float64(order.Amount) * float64(*order.ForeignAmount)))
			if proRated < 1 {
				proRated = 1
			}
			paypalCents = proRated
		}
		return s.paypal.Refund(*order.ThirdPartyTradeNo, paypalCents, currency, reason), currency, nil
	case "native":
		// 自有存管渠道：资金由管理员/财务线下对平。
		return fmt.Sprintf("SP_REFUND_%d", time.Now().UnixMilli()), "CNY", nil
	default:
		return "", "", httperr.BadRequestf("不支持的退款渠道: %s", order.PayMethod)
	}
}
