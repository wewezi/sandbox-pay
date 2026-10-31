package db

import (
	"database/sql"
	"strings"
)

// OrderMetadata contains presentation metadata without loading signing keys.
type OrderMetadata struct {
	GroupName           *string
	APIKeyName          *string
	NotifyStatus        *int
	NotifyRetryCount    *int64
	NotifyLastError     *string
	RefundRequestID     *string
	PendingRefundAmount *int64
}

// MetadataByOrderNos fetches metadata for a page in one query. Notifications are
// ordered by creation time and id so the latest result remains deterministic.
func (r *OrdersRepo) MetadataByOrderNos(orderNos []string) (map[string]OrderMetadata, error) {
	out := make(map[string]OrderMetadata, len(orderNos))
	if len(orderNos) == 0 {
		return out, nil
	}
	args := make([]any, len(orderNos))
	placeholders := make([]string, len(orderNos))
	for i, orderNo := range orderNos {
		args[i], placeholders[i] = orderNo, "?"
	}
	rows, err := r.ex.Query(`SELECT o."orderNo", g."name", k."name", n."status", n."retryCount", n."lastError", f."requestId", f."amount"
		FROM "payment_orders" o
		LEFT JOIN "groups" g ON g."id" = o."groupId"
		LEFT JOIN "api_keys" k ON k."id" = o."apiKeyId"
		LEFT JOIN "notify_queues" n ON n."id" = (
			SELECT q."id" FROM "notify_queues" q WHERE q."orderNo" = o."orderNo"
			ORDER BY q."createdAt" DESC, q."id" DESC LIMIT 1)
		LEFT JOIN "refund_operations" f ON f."orderNo" = o."orderNo" AND f."status" IN ('processing', 'pending', 'unknown')
		WHERE o."orderNo" IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var orderNo string
		var groupName, keyName, lastError, requestID sql.NullString
		var status, retries, refundAmount sql.NullInt64
		if err := rows.Scan(&orderNo, &groupName, &keyName, &status, &retries, &lastError, &requestID, &refundAmount); err != nil {
			return nil, err
		}
		meta := OrderMetadata{
			GroupName: nullString(groupName), APIKeyName: nullString(keyName),
			NotifyRetryCount: nullInt64(retries), NotifyLastError: nullString(lastError),
			RefundRequestID: nullString(requestID), PendingRefundAmount: nullInt64(refundAmount),
		}
		if status.Valid {
			value := int(status.Int64)
			meta.NotifyStatus = &value
		}
		out[orderNo] = meta
	}
	return out, rows.Err()
}

// ListByOrderNo returns all delivery records belonging to this order only.
func (r *NotifyRepo) ListByOrderNo(orderNo string) ([]*NotifyQueue, error) {
	rows, err := r.ex.Query(`SELECT `+notifyColumns+` FROM "notify_queues" WHERE "orderNo" = ? ORDER BY "createdAt" DESC, "id" DESC`, orderNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*NotifyQueue, 0)
	for rows.Next() {
		n, err := scanNotify(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListByOrderNo returns the persisted refund operations in creation order.
func (r *RefundRepo) ListByOrderNo(orderNo string) ([]*RefundOperation, error) {
	rows, err := r.ex.Query(`SELECT "id", "requestId", "reason", "orderNo", "amount", "tradeNo", "status", "createdAt", "updatedAt"
		FROM "refund_operations" WHERE "orderNo" = ? ORDER BY "createdAt" ASC, "id" ASC`, orderNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*RefundOperation, 0)
	for rows.Next() {
		var op RefundOperation
		var tradeNo sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&op.ID, &op.RequestID, &op.Reason, &op.OrderNo, &op.Amount, &tradeNo, &op.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		op.TradeNo = nullString(tradeNo)
		op.CreatedAt, op.UpdatedAt = fromSQL(createdAt), fromSQL(updatedAt)
		out = append(out, &op)
	}
	return out, rows.Err()
}
