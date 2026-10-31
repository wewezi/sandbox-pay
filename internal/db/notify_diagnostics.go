package db

import (
	"database/sql"
	"fmt"
	"time"
)

type NotifyAttempt struct {
	ID              int64
	AttemptNo       int64
	HTTPStatus      *int
	ResponseSummary string
	DurationMs      int64
	ErrorMessage    string
	CreatedAt       time.Time
}

type NotifySimulationMeta struct {
	SourceNotificationID *int64
	Mode                 string
}

// withTransaction also works for repositories already bound to a payment transaction.
func (r *NotifyRepo) withTransaction(fn func(*NotifyRepo) error) error {
	if conn, ok := r.ex.(*sql.DB); ok {
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := fn(&NotifyRepo{ex: tx}); err != nil {
			return err
		}
		return tx.Commit()
	}
	return fn(r)
}

// ClaimDelivery prevents a scheduled worker and an immediate simulation from
// sending the same queued row concurrently. The short lease survives restart.
func (r *NotifyRepo) ClaimDelivery(id int64, now time.Time) (*NotifyQueue, error) {
	var item *NotifyQueue
	err := r.withTransaction(func(tx *NotifyRepo) error {
		result, err := tx.ex.Exec(`UPDATE "notify_queues" SET "leaseUntil" = ? WHERE "id" = ? AND "status" IN (?, ?) AND ("availableAt" IS NULL OR "availableAt" <= ?) AND ("leaseUntil" IS NULL OR "leaseUntil" <= ?)`, toSQL(now.Add(30*time.Second)), id, NotifyPending, NotifyFailed, toSQL(now), toSQL(now))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count == 0 {
			return err
		}
		item, err = tx.FindByID(id)
		return err
	})
	return item, err
}

// RecordAttempt commits successful attempts as well as failed attempts together
// with queue state, so detail pages never show a result without its history.
func (r *NotifyRepo) RecordAttempt(id int64, attempt NotifyAttempt, patch map[string]any) error {
	return r.withTransaction(func(tx *NotifyRepo) error {
		_, err := tx.ex.Exec(`INSERT INTO "notify_attempts" ("notifyId", "attemptNo", "httpStatus", "responseSummary", "durationMs", "errorMessage", "createdAt") VALUES (?, (SELECT COALESCE(MAX("attemptNo"), 0) + 1 FROM "notify_attempts" WHERE "notifyId" = ?), ?, ?, ?, ?, ?)`, id, id, attempt.HTTPStatus, attempt.ResponseSummary, attempt.DurationMs, attempt.ErrorMessage, toSQL(attempt.CreatedAt))
		if err != nil {
			return err
		}
		patch["leaseUntil"] = nil
		return tx.Update(id, patch)
	})
}

func (r *NotifyRepo) ListAttempts(id int64) ([]NotifyAttempt, error) {
	rows, err := r.ex.Query(`SELECT "id", "attemptNo", "httpStatus", "responseSummary", "durationMs", "errorMessage", "createdAt" FROM "notify_attempts" WHERE "notifyId" = ? ORDER BY "attemptNo" ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NotifyAttempt, 0)
	for rows.Next() {
		var item NotifyAttempt
		var status sql.NullInt64
		var created string
		if err := rows.Scan(&item.ID, &item.AttemptNo, &status, &item.ResponseSummary, &item.DurationMs, &item.ErrorMessage, &created); err != nil {
			return nil, err
		}
		if status.Valid {
			value := int(status.Int64)
			item.HTTPStatus = &value
		}
		item.CreatedAt = fromSQL(created)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *NotifyRepo) SimulationMeta(id int64) (NotifySimulationMeta, error) {
	var source sql.NullInt64
	var meta NotifySimulationMeta
	err := r.ex.QueryRow(`SELECT "sourceNotificationId", "simulationMode" FROM "notify_queues" WHERE "id" = ?`, id).Scan(&source, &meta.Mode)
	if source.Valid {
		meta.SourceNotificationID = &source.Int64
	}
	return meta, err
}

// CloneSimulation copies the signed bytes verbatim; re-signing would produce a
// new event instead of testing a receiver's duplicate handling.
func (r *NotifyRepo) CloneSimulation(id int64, mode string, availableAt time.Time) (*NotifyQueue, error) {
	var clone *NotifyQueue
	err := r.withTransaction(func(tx *NotifyRepo) error {
		source, err := tx.FindByID(id)
		if err != nil {
			return err
		}
		if source == nil {
			return sql.ErrNoRows
		}
		newID, err := tx.InsertReturningID(InsertNotifyInput{OrderNo: source.OrderNo, URL: source.URL, Body: source.Body, Signature: source.Signature, Status: NotifyPending, AvailableAt: &availableAt})
		if err != nil {
			return err
		}
		if err := tx.Update(newID, map[string]any{"sourceNotificationId": id, "simulationMode": mode}); err != nil {
			return err
		}
		clone, err = tx.FindByID(newID)
		return err
	})
	return clone, err
}

// Requeue preserves diagnostic history but refuses to overwrite an active delivery.
func (r *NotifyRepo) Requeue(id int64) (bool, error) {
	result, err := r.ex.Exec(`UPDATE "notify_queues" SET "status" = ?, "retryCount" = 0, "lastError" = NULL, "lastAttemptAt" = NULL, "availableAt" = NULL WHERE "id" = ? AND ("leaseUntil" IS NULL OR "leaseUntil" <= ?)`, NotifyPending, id, toSQL(time.Now()))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("通知重新入队: %w", err)
	}
	return count > 0, nil
}
