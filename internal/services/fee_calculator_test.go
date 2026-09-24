package services

import (
	"testing"

	"sandbox-pay/internal/config"
)

// Port of server/src/services/fee-calculator.spec.ts
func TestFeeCalculator(t *testing.T) {
	// Config 就一个 Get —— 未命中时回落到 config.Defaults。
	createCalculator := func(env map[string]string) *FeeCalculator {
		return NewFeeCalculator(config.Map(env))
	}

	t.Run("calculates alipay fee at 0.6% when configured", func(t *testing.T) {
		calc := createCalculator(map[string]string{
			"FEE_RATE_ALIPAY":     "0.006",
			"CHANNEL_COST_ALIPAY": "0.006",
			"FEE_MIN_CENTS":       "0",
		})
		result := calc.Calculate("alipay", 10000)

		if result.Fee != 60 {
			t.Errorf("fee = %d, want 60", result.Fee)
		}
		if result.ChannelCost != 60 {
			t.Errorf("channelCost = %d, want 60", result.ChannelCost)
		}
		if result.SettleAmount != 9940 {
			t.Errorf("settleAmount = %d, want 9940", result.SettleAmount)
		}
	})

	t.Run("calculates paypal fee at 4.4% when configured", func(t *testing.T) {
		calc := createCalculator(map[string]string{
			"FEE_RATE_PAYPAL":     "0.044",
			"CHANNEL_COST_PAYPAL": "0.044",
			"FEE_MIN_CENTS":       "0",
		})
		result := calc.Calculate("paypal", 10000)

		if result.Fee != 440 {
			t.Errorf("fee = %d, want 440", result.Fee)
		}
		if result.ChannelCost != 440 {
			t.Errorf("channelCost = %d, want 440", result.ChannelCost)
		}
		if result.SettleAmount != 9560 {
			t.Errorf("settleAmount = %d, want 9560", result.SettleAmount)
		}
	})

	t.Run("returns 0 when no rate configured", func(t *testing.T) {
		calc := createCalculator(map[string]string{})
		result := calc.Calculate("native", 10000)

		if result.Fee != 0 {
			t.Errorf("fee = %d, want 0", result.Fee)
		}
		if result.ChannelCost != 0 {
			t.Errorf("channelCost = %d, want 0", result.ChannelCost)
		}
		if result.SettleAmount != 10000 {
			t.Errorf("settleAmount = %d, want 10000", result.SettleAmount)
		}
	})

	t.Run("applies minimum fee floor", func(t *testing.T) {
		calc := createCalculator(map[string]string{
			"FEE_RATE_NATIVE": "0",
			"FEE_MIN_CENTS":   "50",
		})
		result := calc.Calculate("native", 100)

		if result.Fee != 50 {
			t.Errorf("fee = %d, want 50 (minimum floor)", result.Fee)
		}
		if result.SettleAmount != 50 {
			t.Errorf("settleAmount = %d, want 50", result.SettleAmount)
		}
	})

	t.Run("margin = fee - channelCost represents gross profit", func(t *testing.T) {
		calc := createCalculator(map[string]string{
			"FEE_RATE_PAYPAL":     "0.05",
			"CHANNEL_COST_PAYPAL": "0.03",
			"FEE_MIN_CENTS":       "0",
		})
		result := calc.Calculate("paypal", 10000)

		if result.Fee != 500 {
			t.Errorf("fee = %d, want 500", result.Fee)
		}
		if result.ChannelCost != 300 {
			t.Errorf("channelCost = %d, want 300", result.ChannelCost)
		}
		if result.Fee-result.ChannelCost != 200 {
			t.Errorf("fee - channelCost = %d, want 200", result.Fee-result.ChannelCost)
		}
	})
}
