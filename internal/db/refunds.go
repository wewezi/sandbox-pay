package db

import "database/sql"

// RefundRepo 退款操作记录用于在途抢占和历史去重。
type RefundRepo struct{ ex executor }

func (r *RefundRepo) find(where string, args ...any) (*RefundOperation, error) {
	var op RefundOperation
	var tradeNo sql.NullString
	var created, updated string
	err := r.ex.QueryRow(`SELECT "id", "requestId", "reason", "orderNo", "amount", "tradeNo", "status", "createdAt", "updatedAt" FROM "refund_operations" WHERE `+where, args...).Scan(
		&op.ID, &op.RequestID, &op.Reason, &op.OrderNo, &op.Amount, &tradeNo, &op.Status, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	op.TradeNo = nullString(tradeNo)
	op.CreatedAt, op.UpdatedAt = fromSQL(created), fromSQL(updated)
	return &op, nil
}

func (r *RefundRepo) FindByID(id string) (*RefundOperation, error) {
	return r.find(`"id" = ?`, id)
}

func (r *RefundRepo) FindByTradeNo(orderNo, tradeNo string) (*RefundOperation, error) {
	return r.find(`"orderNo" = ? AND "tradeNo" = ?`, orderNo, tradeNo)
}

func (r *RefundRepo) FindActiveByOrderNo(orderNo string) (*RefundOperation, error) {
	return r.find(`"orderNo" = ? AND "status" IN ('processing', 'pending', 'unknown')`, orderNo)
}

func (r *RefundRepo) CountActive() (int64, error) {
	var count int64
	err := r.ex.QueryRow(`SELECT COUNT(*) FROM "refund_operations" WHERE "status" IN ('processing', 'pending', 'unknown')`).Scan(&count)
	return count, err
}

func (r *RefundRepo) Insert(op *RefundOperation) error {
	_, err := r.ex.Exec(`INSERT INTO "refund_operations" ("id", "requestId", "reason", "orderNo", "amount", "tradeNo", "status", "createdAt", "updatedAt") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.RequestID, op.Reason, op.OrderNo, op.Amount, op.TradeNo, op.Status, nowSQL(), nowSQL())
	return err
}

func (r *RefundRepo) Update(id, status string, tradeNo *string) error {
	_, err := r.ex.Exec(`UPDATE "refund_operations" SET "status" = ?, "tradeNo" = ?, "updatedAt" = ? WHERE "id" = ?`, status, tradeNo, nowSQL(), id)
	return err
}
