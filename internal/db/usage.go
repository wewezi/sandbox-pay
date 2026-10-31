package db

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

type UsageRepo struct{ ex executor }

const usageColumns = `"id", "requestId", "method", "path", "statusCode", "durationMs",
"groupId", "groupName", "apiKeyId", "apiKeyName", "keyPrefix", "keySuffix", "clientIp",
"clientLocation", "orderNo", "errorMessage", "createdAt"`

// Insert appends a completed API request and fills its persisted ID/time.
func (r *UsageRepo) Insert(log *UsageLog) error {
	if log == nil {
		return fmt.Errorf("使用日志为空")
	}
	if log.StatusCode < 100 || log.StatusCode > 599 || log.DurationMs < 0 || math.IsNaN(log.DurationMs) || math.IsInf(log.DurationMs, 0) {
		return fmt.Errorf("使用日志的响应状态或耗时无效")
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	res, err := r.ex.Exec(`INSERT INTO "api_usage_logs" (
"requestId", "method", "path", "statusCode", "durationMs", "groupId", "groupName",
"apiKeyId", "apiKeyName", "keyPrefix", "keySuffix", "clientIp", "clientLocation",
"orderNo", "errorMessage", "createdAt") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.RequestID, log.Method, log.Path, log.StatusCode, log.DurationMs, bindValue(log.GroupID), log.GroupName,
		bindValue(log.APIKeyID), log.APIKeyName, log.KeyPrefix, log.KeySuffix, log.ClientIP, log.ClientLocation,
		log.OrderNo, log.ErrorMessage, toSQL(log.CreatedAt))
	if err != nil {
		return err
	}
	log.ID, err = res.LastInsertId()
	return err
}

type UsageSearchOpts struct {
	GroupID    *int64
	ProjectIDs []int64
	APIKeyID   *int64
	Method     string
	Status     string
	StatusCode *int
	Keyword    string
	DateFrom   string
	DateTo     string
	UTCOffset  int
	Offset     int64
	Limit      int64
}

type UsageSummary struct {
	TotalCount        int64   `json:"totalCount"`
	SuccessCount      int64   `json:"successCount"`
	FailureCount      int64   `json:"failureCount"`
	AverageDurationMs float64 `json:"averageDurationMs"`
}

func (o UsageSearchOpts) where() *Where {
	w := &Where{}
	w.AddProjectScope(`"groupId"`, o.ProjectIDs)
	if o.GroupID != nil {
		w.Add(`"groupId" = ?`, *o.GroupID)
	}
	if o.APIKeyID != nil {
		w.Add(`"apiKeyId" = ?`, *o.APIKeyID)
	}
	if o.Method != "" {
		w.Add(`"method" = ?`, strings.ToUpper(o.Method))
	}
	if o.StatusCode != nil {
		w.Add(`"statusCode" = ?`, *o.StatusCode)
	}
	switch o.Status {
	case "success":
		w.Add(`"statusCode" >= 200 AND "statusCode" < 400`)
	case "failure":
		w.Add(`("statusCode" < 200 OR "statusCode" >= 400)`)
	}
	if o.Keyword != "" {
		// INSTR treats '%' and '_' literally instead of turning pasted identifiers
		// into unintentional wildcard queries.
		w.Add(`(INSTR(LOWER("clientIp"), LOWER(?)) > 0 OR INSTR(LOWER("path"), LOWER(?)) > 0
OR INSTR(LOWER("requestId"), LOWER(?)) > 0 OR INSTR(LOWER("orderNo"), LOWER(?)) > 0)`,
			o.Keyword, o.Keyword, o.Keyword, o.Keyword)
	}
	addDateRange(w, `"createdAt"`, o.DateFrom, o.DateTo, o.UTCOffset)
	return w
}

// Search returns a page and a summary over every matching request. Call it on a
// TxScope when the page and summary must represent the same database snapshot.
func (r *UsageRepo) Search(opts UsageSearchOpts) ([]*UsageLog, int64, UsageSummary, error) {
	w := opts.where()
	var summary UsageSummary
	err := r.ex.QueryRow(`SELECT COUNT(*),
COALESCE(SUM(CASE WHEN "statusCode" >= 200 AND "statusCode" < 400 THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN "statusCode" < 200 OR "statusCode" >= 400 THEN 1 ELSE 0 END), 0),
COALESCE(AVG("durationMs"), 0) FROM "api_usage_logs"`+w.SQL(), w.Args()...).Scan(
		&summary.TotalCount, &summary.SuccessCount, &summary.FailureCount, &summary.AverageDurationMs)
	if err != nil {
		return nil, 0, summary, err
	}
	args := append(append([]any{}, w.Args()...), opts.Limit, opts.Offset)
	rows, err := r.ex.Query(`SELECT `+usageColumns+` FROM "api_usage_logs"`+w.SQL()+
		` ORDER BY "createdAt" DESC, "id" DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, summary, err
	}
	defer rows.Close()
	items := make([]*UsageLog, 0)
	for rows.Next() {
		item, err := scanUsageLog(rows)
		if err != nil {
			return nil, 0, summary, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, summary, err
	}
	return items, summary.TotalCount, summary, nil
}

func (r *UsageRepo) FindByID(id int64) (*UsageLog, error) {
	item, err := scanUsageLog(r.ex.QueryRow(`SELECT `+usageColumns+` FROM "api_usage_logs" WHERE "id" = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func scanUsageLog(row scanner) (*UsageLog, error) {
	var item UsageLog
	var groupID, keyID sql.NullInt64
	var createdAt string
	err := row.Scan(&item.ID, &item.RequestID, &item.Method, &item.Path, &item.StatusCode, &item.DurationMs,
		&groupID, &item.GroupName, &keyID, &item.APIKeyName, &item.KeyPrefix, &item.KeySuffix,
		&item.ClientIP, &item.ClientLocation, &item.OrderNo, &item.ErrorMessage, &createdAt)
	if err != nil {
		return nil, err
	}
	item.GroupID, item.APIKeyID = nullInt64(groupID), nullInt64(keyID)
	item.CreatedAt = fromSQL(createdAt)
	return &item, nil
}
