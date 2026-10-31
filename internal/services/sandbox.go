package services

import (
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/money"
)

type TestOrderInfo struct {
	OrderNo         string  `json:"orderNo"`
	Amount          float64 `json:"amount"`
	ExpireAt        string  `json:"expireAt"`
	CreatedAt       string  `json:"createdAt"`
	PayMethod       string  `json:"payMethod"`
	ProductName     string  `json:"productName"`
	ExternalOrderNo *string `json:"externalOrderNo,omitempty"`
	NotifyURL       *string `json:"notifyUrl,omitempty"`
	ReturnURL       *string `json:"returnUrl,omitempty"`
	GroupName       *string `json:"groupName,omitempty"`
	APIKeyID        *int64  `json:"apiKeyId,omitempty"`
	APIKeyName      *string `json:"apiKeyName,omitempty"`
}

// TestOrderInfo returns stored metadata so an idempotent retry does not claim
// that changed product names, callback destinations, or Key context were saved.
func (s *GroupService) TestOrderInfo(orderNo string) (*TestOrderInfo, error) {
	order, err := s.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	out := &TestOrderInfo{
		OrderNo: order.OrderNo, Amount: money.ToYuan(order.Amount), ExpireAt: JsTime(order.ExpireAt),
		CreatedAt: JsTime(order.CreatedAt), PayMethod: order.PayMethod,
		ProductName: order.ProductName, ExternalOrderNo: order.ExternalOrderNo,
		NotifyURL: order.NotifyURL, ReturnURL: order.ReturnURL, APIKeyID: order.APIKeyID,
	}
	if order.GroupID != nil {
		group, err := s.store.Groups.FindByID(*order.GroupID)
		if err != nil {
			return nil, err
		}
		if group != nil {
			out.GroupName = &group.Name
		}
	}
	if order.APIKeyID != nil {
		key, err := s.store.ApiKeys.FindByID(*order.APIKeyID)
		if err != nil {
			return nil, err
		}
		if key != nil {
			out.APIKeyName = &key.Name
		}
	}
	return out, nil
}

// TestOrderCaller lets an administrator choose project and token attribution.
// The project's webhook secret signs callbacks, including tests with no token.
func (s *GroupService) TestOrderCaller(apiKeyID *int64) (OrderCaller, error) {
	if apiKeyID == nil {
		group, err := s.DefaultGroup()
		if err != nil {
			return OrderCaller{}, err
		}
		return OrderCaller{GroupID: group.ID}, nil
	}
	key, err := s.store.ApiKeys.FindByID(*apiKeyID)
	if err != nil {
		return OrderCaller{}, err
	}
	if key == nil || !key.IsActive {
		return OrderCaller{}, httperr.BadRequest("所选 API Key 不存在或已停用")
	}
	group, err := s.store.Groups.FindByID(key.GroupID)
	if err != nil {
		return OrderCaller{}, err
	}
	if group == nil || !group.IsActive {
		return OrderCaller{}, httperr.BadRequest("所选 API Key 的分组已停用")
	}
	return OrderCaller{GroupID: group.ID, APIKeyID: &key.ID}, nil
}
