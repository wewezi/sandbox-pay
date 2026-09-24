package services

import (
	"math"

	"sandbox-pay/internal/config"
)

// FeeResult 一笔订单入账后的三段金额，全是整数分。
type FeeResult struct {
	// Fee Sandbox Pay 向接入方收的手续费。
	Fee int64 `json:"fee"`
	// ChannelCost 上游渠道（支付宝/PayPal）收的成本。
	ChannelCost int64 `json:"channelCost"`
	// SettleAmount 扣掉 fee 后应结给接入方的金额。
	SettleAmount int64 `json:"settleAmount"`
}

// FeeCalculator 按渠道费率算手续费。费率从 .env 读，默认值见 config.Defaults。
type FeeCalculator struct {
	cfg config.Config
}

// NewFeeCalculator 构造。
func NewFeeCalculator(cfg config.Config) *FeeCalculator {
	return &FeeCalculator{cfg: cfg}
}

// Calculate 算 fee / channelCost / settleAmount。
//
// fee   = max(round(amount * 费率), round(下限))
// cost  = round(amount * 成本率)
// settle= max(0, amount - fee)
func (f *FeeCalculator) Calculate(payMethod string, amountCents int64) FeeResult {
	method := payMethod
	feeRate := config.Rate(f.cfg, "FEE_RATE", method)
	costRate := config.Rate(f.cfg, "CHANNEL_COST", method)
	floor := config.Number(f.cfg, "FEE_MIN_CENTS")

	fee := int64(math.Max(
		math.Round(float64(amountCents)*feeRate),
		math.Round(floor),
	))
	channelCost := int64(math.Round(float64(amountCents) * costRate))
	settle := amountCents - fee
	if settle < 0 {
		settle = 0
	}
	return FeeResult{Fee: fee, ChannelCost: channelCost, SettleAmount: settle}
}
