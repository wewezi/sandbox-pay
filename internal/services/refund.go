package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
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

// StructuredRefunder 能区分上游已完成、仍在处理和明确拒绝。
type StructuredRefunder interface {
	RefundWithStatus(orderNo string, refundCents int64, reason, requestID string) (string, string, error)
}

type StructuredPayPalRefunder interface {
	RefundWithStatus(captureID string, refundCents int64, currency, reason, requestID string) (string, string, error)
}

type PayPalRefundQuerier interface {
	QueryRefund(refundID string, expectedCents int64, expectedCurrency string) (string, error)
}

// RefundInput 退款入参。Amount 是元。
type RefundInput struct {
	OrderNo   string
	Amount    float64
	Reason    *string
	RequestID string
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
	Status         string  `json:"status"`
	RequestID      string  `json:"requestId"`
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
	if dto.OrderNo == "" || math.IsNaN(dto.Amount) || math.IsInf(dto.Amount, 0) || dto.Amount < 0.01 || dto.Amount >= float64(math.MaxInt64)/100 {
		return nil, httperr.BadRequest("orderNo 和 amount 为必填项")
	}
	requestID := strings.TrimSpace(dto.RequestID)
	if requestID == "" {
		requestID = "SP_REFUND_" + randomHex(16)
	}
	if len(requestID) > 64 || strings.IndexFunc(requestID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) >= 0 {
		return nil, httperr.BadRequest("requestId 只能包含字母、数字、下划线和连字符，最多64字符")
	}
	refundCents := money.YuanToCents(dto.Amount)
	operationID := refundOperationID(dto.OrderNo, requestID)
	order, op, dispatch, err := s.reserve(dto.OrderNo, refundCents, requestID, operationID, strDeref(dto.Reason, ""), opts.GroupID)
	if err != nil {
		return nil, err
	}
	currency := "CNY"
	if order.PayMethod == "paypal" {
		currency = strDeref(order.ForeignCurrency, "USD")
	}
	if !dispatch {
		return &RefundResult{OrderNo: dto.OrderNo, RefundedAmount: money.ToYuan(op.Amount), RefundTradeNo: strDeref(op.TradeNo, ""), Currency: currency, Status: "completed", RequestID: requestID}, nil
	}
	tradeNo, currency, status, dispatchErr := s.dispatch(order, op)
	if status == "FAILED" {
		if err := s.failOperation(dto.OrderNo, operationID); err != nil {
			return nil, err
		}
		return nil, httperr.BadRequest("上游退款失败: 网关未处理退款请求")
	}
	if dispatchErr != nil || status != "COMPLETED" || tradeNo == "" {
		opStatus := db.RefundPending
		if dispatchErr != nil {
			opStatus = db.RefundUnknown
			s.log.Warn(fmt.Sprintf("Refund %s outcome unknown: %s", requestID, dispatchErr))
		}
		var trade *string
		if tradeNo != "" {
			trade = &tradeNo
		}
		if err := s.store.Refunds.Update(operationID, opStatus, trade); err != nil {
			return nil, err
		}
		return &RefundResult{OrderNo: dto.OrderNo, RefundTradeNo: tradeNo, Currency: currency, Status: "pending", RequestID: requestID}, nil
	}
	if _, err := s.payments.completeRefund(dto.OrderNo, refundCents, tradeNo, opts.GroupID, operationID); err != nil {
		return nil, err
	}
	actor := opts.Actor
	if actor == "" {
		actor = "unknown"
	}
	s.log.Log(fmt.Sprintf("Refund done %s %v %s by %s tradeNo=%s",
		dto.OrderNo, strconv.FormatFloat(dto.Amount, 'f', -1, 64), currency, actor, tradeNo))

	return &RefundResult{
		OrderNo:        dto.OrderNo,
		RefundedAmount: dto.Amount,
		RefundTradeNo:  tradeNo,
		Currency:       currency,
		Status:         "completed",
		RequestID:      requestID,
	}, nil
}

// reserve 在同一事务检查归属、余额和状态，只有抢占成功的请求能派发。
func (s *RefundService) reserve(orderNo string, cents int64, requestID, operationID, reason string, groupID *int64) (*db.PaymentOrder, *db.RefundOperation, bool, error) {
	var order *db.PaymentOrder
	var op *db.RefundOperation
	dispatch := false
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		order, err = tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if order == nil || groupID != nil && (order.GroupID == nil || *order.GroupID != *groupID) {
			return httperr.NotFound("订单不存在")
		}
		op, err = tx.Refunds.FindByID(operationID)
		if err != nil {
			return err
		}
		if op != nil {
			if op.OrderNo != orderNo || op.Amount != cents {
				return httperr.Conflict("requestId 已用于不同退款")
			}
			if op.Status == db.RefundCompleted {
				return nil
			}
			if op.Status == db.RefundFailed {
				return httperr.Conflict("该退款请求已失败，请使用新的 requestId")
			}
			if order.Status != db.StatusRefunding || op.Status == db.RefundProcessing && op.UpdatedAt.Add(2*time.Minute).After(time.Now()) {
				return httperr.Conflict("退款请求正在处理")
			}
			if op.TradeNo == nil && op.CreatedAt.Add(30*time.Minute).Before(time.Now()) {
				return httperr.Conflict("未知退款结果已超过安全重试期限，需要人工核对上游记录")
			}
			if order.PayMethod == "paypal" {
				if _, ok := s.paypal.(StructuredPayPalRefunder); !ok {
					return httperr.Conflict("该渠道不支持重试在途退款")
				}
			} else if order.PayMethod == "alipay" {
				if _, ok := s.alipay.(StructuredRefunder); !ok {
					return httperr.Conflict("该渠道不支持重试在途退款")
				}
			} else {
				return httperr.Conflict("该渠道的在途退款需要人工核对")
			}
			if err := tx.Refunds.Update(operationID, db.RefundProcessing, op.TradeNo); err != nil {
				return err
			}
			dispatch = true
			return nil
		}
		if order.Status != db.StatusPaid {
			return httperr.Conflict(fmt.Sprintf("订单状态不允许退款: %d", order.Status))
		}
		if cents > order.Amount-order.RefundedAmount {
			return httperr.BadRequest("退款金额超过可退余额")
		}
		op = &db.RefundOperation{ID: operationID, RequestID: requestID, Reason: reason, OrderNo: orderNo, Amount: cents, Status: db.RefundProcessing}
		if err := tx.Refunds.Insert(op); err != nil {
			return err
		}
		if err := tx.Orders.Update(orderNo, map[string]any{"status": db.StatusRefunding}); err != nil {
			return err
		}
		order.Status = db.StatusRefunding
		dispatch = true
		return nil
	})
	return order, op, dispatch, err
}

// 上游请求ID限制38字符；按订单隔离调用方标识，避免不同分组的相同requestId互相冲突。
func refundOperationID(orderNo, requestID string) string {
	sum := sha256.Sum256([]byte(orderNo + "\x00" + requestID))
	return "RF_" + hex.EncodeToString(sum[:16])
}

func (s *RefundService) failOperation(orderNo, requestID string) error {
	return s.store.Do(func(tx *db.TxScope) error {
		if err := tx.Refunds.Update(requestID, db.RefundFailed, nil); err != nil {
			return err
		}
		return tx.Orders.Update(orderNo, map[string]any{"status": db.StatusPaid})
	})
}

// dispatch 调各渠道自己的退款接口，返回上游退款单号和退款币种。
// 上游已完成、待确认、明确失败和结果未知分别处理。
func (s *RefundService) dispatch(order *db.PaymentOrder, op *db.RefundOperation) (string, string, string, error) {
	refundCents, reason, requestID := op.Amount, op.Reason, op.ID
	switch order.PayMethod {
	case "alipay":
		if gateway, ok := s.alipay.(StructuredRefunder); ok {
			id, status, err := gateway.RefundWithStatus(order.OrderNo, refundCents, reason, requestID)
			return id, "CNY", status, err
		}
		id := s.alipay.Refund(order.OrderNo, refundCents, reason)
		return id, "CNY", legacyRefundStatus(id), nil
	case "paypal":
		if strDeref(order.ThirdPartyTradeNo, "") == "" {
			return "", "USD", "FAILED", httperr.BadRequest("订单缺少 PayPal captureId，无法退款")
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
		if op.TradeNo != nil {
			if gateway, ok := s.paypal.(PayPalRefundQuerier); ok {
				status, err := gateway.QueryRefund(*op.TradeNo, paypalCents, currency)
				return *op.TradeNo, currency, status, err
			}
		}
		if gateway, ok := s.paypal.(StructuredPayPalRefunder); ok {
			id, status, err := gateway.RefundWithStatus(*order.ThirdPartyTradeNo, paypalCents, currency, reason, requestID)
			return id, currency, status, err
		}
		id := s.paypal.Refund(*order.ThirdPartyTradeNo, paypalCents, currency, reason)
		return id, currency, legacyRefundStatus(id), nil
	case "native":
		// 自有存管渠道：资金由管理员/财务线下对平。
		return "SP_REFUND_" + strings.TrimPrefix(requestID, "RF_"), "CNY", "COMPLETED", nil
	default:
		return "", "", "FAILED", httperr.BadRequestf("不支持的退款渠道: %s", order.PayMethod)
	}
}

func legacyRefundStatus(id string) string {
	if id == "" {
		return "FAILED"
	}
	return "COMPLETED"
}
