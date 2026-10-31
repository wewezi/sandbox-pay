package services

import (
	"encoding/json"
	"strings"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/geoip"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/money"
)

type OrderRefundItem struct {
	ID        string  `json:"id"`
	RequestID string  `json:"requestId"`
	Reason    string  `json:"reason"`
	Amount    float64 `json:"amount"`
	TradeNo   *string `json:"tradeNo,omitempty"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type OrderNotificationItem struct {
	NotificationItem
	Body string `json:"body"`
}

// TransactionDetail exposes persisted reconciliation fields to authenticated
// administrators. Signing keys and notification signatures stay on the server.
type TransactionDetail struct {
	TransactionItem
	ReturnURL       *string                 `json:"returnUrl,omitempty"`
	ProviderOrderID *string                 `json:"providerOrderId,omitempty"`
	ExchangeRate    *float64                `json:"exchangeRate,omitempty"`
	ForeignAmount   *float64                `json:"foreignAmount,omitempty"`
	ForeignCurrency *string                 `json:"foreignCurrency,omitempty"`
	RefundTradeNo   *string                 `json:"refundTradeNo,omitempty"`
	RefundAt        *string                 `json:"refundAt,omitempty"`
	Refunds         []OrderRefundItem       `json:"refunds"`
	Notifications   []OrderNotificationItem `json:"notifications"`
}

func transactionItem(order *db.PaymentOrder, meta db.OrderMetadata) TransactionItem {
	item := TransactionItem{
		ID: order.ID, OrderNo: order.OrderNo, Channel: channelOf(order.PayMethod),
		Amount: money.ToYuan(order.Amount), RefundedAmount: money.ToYuan(order.RefundedAmount),
		Fee: money.ToYuan(order.Fee), SettleAmount: money.ToYuan(order.SettleAmount),
		ChannelCost: money.ToYuan(order.ChannelCost), PayMethod: order.PayMethod, ProductName: order.ProductName,
		ExternalOrderNo: order.ExternalOrderNo, ThirdPartyTradeNo: order.ThirdPartyTradeNo,
		Status: statusOf(order.Status), RawStatus: order.Status,
		CreatedAt: JsTime(order.CreatedAt), UpdatedAt: JsTime(order.UpdatedAt), ExpireAt: JsTime(order.ExpireAt), PayAt: jsTimePtr(order.PayAt),
		GroupID: order.GroupID, GroupName: meta.GroupName, APIKeyID: order.APIKeyID, APIKeyName: meta.APIKeyName,
		NotifyURL: order.NotifyURL, NotifyStatus: meta.NotifyStatus, NotifyRetryCount: meta.NotifyRetryCount, NotifyLastError: meta.NotifyLastError,
		ClientIP: order.ClientIP,
	}
	if order.ClientLocation != nil && strings.TrimSpace(*order.ClientLocation) != "null" {
		var location geoip.Location
		if err := json.Unmarshal([]byte(*order.ClientLocation), &location); err == nil {
			item.ClientLocation = &location
		}
	}
	if order.Status == db.StatusRefunding && meta.RefundRequestID != nil && meta.PendingRefundAmount != nil {
		item.RefundRequestID = meta.RefundRequestID
		amount := money.ToYuan(*meta.PendingRefundAmount)
		item.PendingRefundAmount = &amount
	}
	return item
}

// GetTransaction reads the order and its related records from one database
// snapshot so a concurrent payment/refund cannot produce contradictory details.
func (s *AdminService) GetTransaction(orderNo string) (*TransactionDetail, error) {
	var out *TransactionDetail
	err := s.store.Do(func(tx *db.TxScope) error {
		order, err := tx.Orders.FindByOrderNo(orderNo, false)
		if err != nil {
			return err
		}
		if order == nil {
			return httperr.NotFound("订单不存在")
		}
		metadata, err := tx.Orders.MetadataByOrderNos([]string{orderNo})
		if err != nil {
			return err
		}
		refunds, err := tx.Refunds.ListByOrderNo(orderNo)
		if err != nil {
			return err
		}
		notifications, err := tx.Notifies.ListByOrderNo(orderNo)
		if err != nil {
			return err
		}
		out = &TransactionDetail{
			TransactionItem: transactionItem(order, metadata[orderNo]),
			ReturnURL:       order.ReturnURL, ProviderOrderID: order.ProviderOrderID,
			ExchangeRate: order.ExchangeRate, ForeignCurrency: order.ForeignCurrency,
			RefundTradeNo: order.RefundTradeNo, RefundAt: jsTimePtr(order.RefundAt),
			Refunds: make([]OrderRefundItem, 0, len(refunds)), Notifications: make([]OrderNotificationItem, 0, len(notifications)),
		}
		if order.ForeignAmount != nil {
			amount := money.ToYuan(*order.ForeignAmount)
			out.ForeignAmount = &amount
		}
		for _, op := range refunds {
			out.Refunds = append(out.Refunds, OrderRefundItem{
				ID: op.ID, RequestID: op.RequestID, Reason: op.Reason, Amount: money.ToYuan(op.Amount),
				TradeNo: op.TradeNo, Status: op.Status, CreatedAt: JsTime(op.CreatedAt), UpdatedAt: JsTime(op.UpdatedAt),
			})
		}
		for _, n := range notifications {
			out.Notifications = append(out.Notifications, OrderNotificationItem{
				NotificationItem: NotificationItem{
					ID: n.ID, OrderNo: n.OrderNo, URL: n.URL, Status: n.Status, RetryCount: n.RetryCount,
					LastError: n.LastError, LastAttemptAt: jsTimePtr(n.LastAttemptAt), CreatedAt: JsTime(n.CreatedAt),
				}, Body: n.Body,
			})
		}
		return nil
	})
	return out, err
}
