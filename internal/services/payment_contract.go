package services

import "sandbox-pay/internal/httperr"

// QueryByExternalOrderNo supports recovery when the original pay response was
// lost, without exposing another project's order or creating a replacement.
func (s *PaymentService) QueryByExternalOrderNo(groupID int64, externalOrderNo string) (*OrderGroupQueryResult, error) {
	order, err := s.store.Orders.FindLatestByExternalOrderNo(groupID, externalOrderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	return s.QueryByGroup(groupID, order.OrderNo)
}
