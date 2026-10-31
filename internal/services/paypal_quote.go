package services

import (
	"math"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/money"
)

// Amount is in USD major units. A quote is payable only when Available is true;
// null amounts avoid presenting a rounded-up minimum as a payable price.
type PayPalQuote struct {
	Amount       *float64 `json:"amount"`
	Currency     string   `json:"currency"`
	ExchangeRate *float64 `json:"exchangeRate"`
	Available    bool     `json:"available"`
	Reason       string   `json:"reason,omitempty"`
}

const paypalMinimumReason = "订单换算后不足 USD 0.01，无法使用 PayPal，请选择 Mintweft 或支付宝"

func paypalConvertedCents(cnyCents int64, rate float64) (int64, string) {
	if cnyCents <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0, "PayPal 汇率或订单金额无效，请选择其他支付方式"
	}
	converted := float64(cnyCents) * rate
	if math.IsNaN(converted) || math.IsInf(converted, 0) || converted >= float64(math.MaxInt64) {
		return 0, "PayPal 换算金额超出支持范围，请选择其他支付方式"
	}
	if converted < 1 {
		return 0, paypalMinimumReason
	}
	return int64(math.Round(converted)), ""
}

// PayPalQuoteForOrder validates a persisted pending quote, including legacy
// snapshots that previously forced a below-minimum amount up to one USD cent.
// It never changes the order or applies a new market rate.
func PayPalQuoteForOrder(order *db.PaymentOrder) *PayPalQuote {
	quote := &PayPalQuote{Currency: "USD"}
	if order == nil {
		quote.Reason = "PayPal 订单信息不可用"
		return quote
	}
	if order.ExchangeRate != nil && !math.IsNaN(*order.ExchangeRate) && !math.IsInf(*order.ExchangeRate, 0) && *order.ExchangeRate > 0 {
		rate := *order.ExchangeRate
		quote.ExchangeRate = &rate
	}
	if order.ExchangeRate == nil || order.ForeignAmount == nil || order.ForeignCurrency == nil || *order.ForeignCurrency != "USD" {
		quote.Reason = "PayPal 报价信息不完整，请选择其他支付方式"
		return quote
	}
	cents, reason := paypalConvertedCents(order.Amount, *order.ExchangeRate)
	if reason != "" {
		quote.Reason = reason
		return quote
	}
	if cents != *order.ForeignAmount {
		quote.Reason = "PayPal 金额与锁定汇率不一致，请选择其他支付方式"
		return quote
	}
	amount := money.ToYuan(cents)
	quote.Amount = &amount
	quote.Available = true
	return quote
}

func hasPayPalSnapshot(order *db.PaymentOrder) bool {
	return order.ExchangeRate != nil || order.ForeignAmount != nil || order.ForeignCurrency != nil || order.ProviderOrderID != nil
}

// EnsurePayPalQuote locks an eligible quote locally before it is displayed or
// sent to the provider. Concurrent GET/create requests reuse the same snapshot,
// without changing the selected method or the merchant's original CNY amount.
// Rate retrieval occurs outside the write transaction and no provider payment
// is created here. Completed orders are never modified.
func (s *PaymentService) EnsurePayPalQuote(orderNo string, rates *ExchangeRateService) (*PayPalQuote, error) {
	order, err := s.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if hasPayPalSnapshot(order) {
		return PayPalQuoteForOrder(order), nil
	}
	if order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
		return &PayPalQuote{Currency: "USD", Reason: "订单状态不允许生成 PayPal 报价"}, nil
	}
	if rates == nil {
		return &PayPalQuote{Currency: "USD", Reason: "PayPal 汇率暂不可用，请选择其他支付方式"}, nil
	}
	rate := rates.GetCnyToUsdRate()
	var quote *PayPalQuote
	err = s.store.Do(func(tx *db.TxScope) error {
		current, err := tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if current == nil {
			return httperr.NotFound("订单不存在")
		}
		if hasPayPalSnapshot(current) {
			quote = PayPalQuoteForOrder(current)
			return nil
		}
		quote = &PayPalQuote{Currency: "USD"}
		if current.Status != db.StatusPending || !current.ExpireAt.After(time.Now()) {
			quote.Reason = "订单状态不允许生成 PayPal 报价"
			return nil
		}
		cents, reason := paypalConvertedCents(current.Amount, rate)
		if !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate > 0 {
			quote.ExchangeRate = &rate
		}
		if reason != "" {
			quote.Reason = reason
			return nil
		}
		if err := tx.Orders.Update(orderNo, map[string]any{"exchangeRate": rate, "foreignAmount": cents, "foreignCurrency": "USD"}); err != nil {
			return err
		}
		currency := "USD"
		current.ExchangeRate, current.ForeignAmount, current.ForeignCurrency = &rate, &cents, &currency
		quote = PayPalQuoteForOrder(current)
		return nil
	})
	return quote, err
}
