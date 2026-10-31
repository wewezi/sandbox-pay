package services

import (
	"sync"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

type cashierChannelOperation struct {
	mu    sync.Mutex
	users int
}

// A pending order's payMethod is its current selection, not a completed
// payment. All three methods remain selectable; provider bindings are retained
// for retries, and the first verified payment records its actual source.
func CashierChannelCapabilities(order *db.PaymentOrder) ([]string, string) {
	if order == nil {
		return nil, "订单信息不可用"
	}
	if order.Status != db.StatusPending {
		return nil, ""
	}
	return []string{"native", "alipay", "paypal"}, ""
}

func CashierChannelAllowed(order *db.PaymentOrder, channel string) bool {
	channels, _ := CashierChannelCapabilities(order)
	for _, available := range channels {
		if available == channel {
			return true
		}
	}
	return false
}

// GetOrderForCashierChannel validates before the route contacts a provider.
// SwitchPaymentMethod repeats this validation inside its write transaction.
func (s *PaymentService) GetOrderForCashierChannel(orderNo, channel string) (*db.PaymentOrder, error) {
	order, err := s.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
		return nil, httperr.Conflict("订单状态不允许切换渠道")
	}
	if !CashierChannelAllowed(order, channel) {
		_, reason := CashierChannelCapabilities(order)
		return nil, httperr.Conflict(reason)
	}
	return order, nil
}

// LockCashierChannel serializes channel creation/recovery and wallet
// confirmation for one order. It does not hold a database transaction during
// the provider request, and unrelated orders remain independent.
func (s *PaymentService) LockCashierChannel(orderNo string) func() {
	s.channelMu.Lock()
	if s.channelOperations == nil {
		s.channelOperations = make(map[string]*cashierChannelOperation)
	}
	operation := s.channelOperations[orderNo]
	if operation == nil {
		operation = &cashierChannelOperation{}
		s.channelOperations[orderNo] = operation
	}
	operation.users++
	s.channelMu.Unlock()
	operation.mu.Lock()
	return func() {
		operation.mu.Unlock()
		s.channelMu.Lock()
		operation.users--
		if operation.users == 0 {
			delete(s.channelOperations, orderNo)
		}
		s.channelMu.Unlock()
	}
}

// RestoreCashierChannel is only called when the adapter proves no payment was
// created. The transaction compares the attempted selection and original
// binding; a completed payment or newly created provider binding cannot be
// rolled back, and a pre-existing retry snapshot is preserved.
func (s *PaymentService) RestoreCashierChannel(original *db.PaymentOrder, attempted string) error {
	if original == nil || original.Status != db.StatusPending {
		return nil
	}
	return s.store.Do(func(tx *db.TxScope) error {
		order, err := tx.Orders.FindByOrderNo(original.OrderNo, false)
		if err != nil || order == nil {
			return err
		}
		if order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) ||
			order.PayMethod != attempted || !sameChannelString(order.ProviderOrderID, original.ProviderOrderID) ||
			!sameChannelString(order.ThirdPartyTradeNo, original.ThirdPartyTradeNo) {
			return nil
		}
		return tx.Orders.Update(original.OrderNo, map[string]any{
			"payMethod": original.PayMethod,
		})
	})
}

func sameChannelString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
