package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type OperationSettings struct {
	UsageRetentionDays   int    `json:"usageRetentionDays"`
	AuditRetentionDays   int    `json:"auditRetentionDays"`
	AttemptRetentionDays int    `json:"attemptRetentionDays"`
	BackupIntervalHours  int    `json:"backupIntervalHours"`
	BackupKeepCount      int    `json:"backupKeepCount"`
	UpdatedAt            string `json:"updatedAt"`
}

func (s *Store) OperationSettings() (*OperationSettings, error) {
	out := &OperationSettings{}
	err := s.db.QueryRow(`SELECT usageRetentionDays,auditRetentionDays,attemptRetentionDays,backupIntervalHours,backupKeepCount,updatedAt FROM operation_settings WHERE id=1`).Scan(&out.UsageRetentionDays, &out.AuditRetentionDays, &out.AttemptRetentionDays, &out.BackupIntervalHours, &out.BackupKeepCount, &out.UpdatedAt)
	if err == nil {
		out.UpdatedAt = fromSQL(out.UpdatedAt).Format(time.RFC3339Nano)
	}
	return out, err
}
func (s *Store) SaveOperationSettings(in OperationSettings) error {
	for _, days := range []int{in.UsageRetentionDays, in.AuditRetentionDays, in.AttemptRetentionDays} {
		if days < 1 || days > 3650 {
			return fmt.Errorf("日志保留天数须为 1 至 3650")
		}
	}
	if in.BackupIntervalHours < 0 || in.BackupIntervalHours > 720 || in.BackupKeepCount < 1 || in.BackupKeepCount > 100 {
		return fmt.Errorf("备份间隔须为 0 至 720 小时，保留份数须为 1 至 100")
	}
	_, err := s.db.Exec(`UPDATE operation_settings SET usageRetentionDays=?,auditRetentionDays=?,attemptRetentionDays=?,backupIntervalHours=?,backupKeepCount=?,updatedAt=? WHERE id=1`, in.UsageRetentionDays, in.AuditRetentionDays, in.AttemptRetentionDays, in.BackupIntervalHours, in.BackupKeepCount, nowSQL())
	return err
}

type CleanupResult struct {
	Usage    int64 `json:"usage"`
	Audit    int64 `json:"audit"`
	Attempts int64 `json:"attempts"`
	Sessions int64 `json:"sessions"`
}

func (s *Store) CleanOperationLogs(now time.Time) (*CleanupResult, error) {
	settings, err := s.OperationSettings()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := &CleanupResult{}
	items := []struct {
		table string
		days  int
		count *int64
	}{
		{"api_usage_logs", settings.UsageRetentionDays, &out.Usage},
		{"admin_audit_logs", settings.AuditRetentionDays, &out.Audit},
		{"notify_attempts", settings.AttemptRetentionDays, &out.Attempts},
	}
	for _, item := range items {
		result, err := tx.Exec(`DELETE FROM `+item.table+` WHERE julianday(createdAt)<julianday(?)`, toSQL(now.AddDate(0, 0, -item.days)))
		if err != nil {
			return nil, err
		}
		*item.count, err = result.RowsAffected()
		if err != nil {
			return nil, err
		}
	}
	result, err := tx.Exec(`DELETE FROM admin_sessions WHERE julianday(expiresAt)<julianday(?) OR (revokedAt IS NOT NULL AND julianday(revokedAt)<julianday(?))`, toSQL(now.AddDate(0, 0, -7)), toSQL(now.AddDate(0, 0, -7)))
	if err != nil {
		return nil, err
	}
	out.Sessions, err = result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

type AuditLog struct {
	ID         int64   `json:"id"`
	UserID     *int64  `json:"userId,omitempty"`
	Username   string  `json:"username"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Action     string  `json:"action"`
	Target     string  `json:"target"`
	ClientIP   string  `json:"clientIp"`
	StatusCode int     `json:"statusCode"`
	DurationMS float64 `json:"durationMs"`
	RequestID  string  `json:"requestId"`
	CreatedAt  string  `json:"createdAt"`
}

func (s *Store) RecordAudit(in AuditLog) error {
	_, err := s.db.Exec(`INSERT INTO admin_audit_logs(userId,username,method,path,action,target,clientIp,statusCode,durationMs,requestId,createdAt) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, bindValue(in.UserID), in.Username, in.Method, in.Path, in.Action, in.Target, in.ClientIP, in.StatusCode, in.DurationMS, in.RequestID, nowSQL())
	return err
}
func (s *Store) AuditLogs(keyword string, limit, offset int64) ([]AuditLog, int64, error) {
	w := &Where{}
	if keyword != "" {
		like := "%" + keyword + "%"
		w.Add(`(username LIKE ? OR action LIKE ? OR target LIKE ? OR clientIp LIKE ? OR requestId LIKE ?)`, like, like, like, like, like)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_audit_logs`+w.SQL(), w.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id,userId,username,method,path,action,target,clientIp,statusCode,durationMs,requestId,createdAt FROM admin_audit_logs`+w.SQL()+` ORDER BY createdAt DESC,id DESC LIMIT ? OFFSET ?`, append(append([]any{}, w.Args()...), limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]AuditLog, 0)
	for rows.Next() {
		var item AuditLog
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.Method, &item.Path, &item.Action, &item.Target, &item.ClientIP, &item.StatusCode, &item.DurationMS, &item.RequestID, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.CreatedAt = fromSQL(item.CreatedAt).Format(time.RFC3339Nano)
		out = append(out, item)
	}
	return out, total, rows.Err()
}

type OperationRun struct {
	Name      string `json:"name"`
	LastRunAt string `json:"lastRunAt"`
	LastError string `json:"lastError"`
}

func (s *Store) RecordOperationRun(name, message string) error {
	_, err := s.db.Exec(`INSERT INTO operation_runs(name,lastRunAt,lastError) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET lastRunAt=excluded.lastRunAt,lastError=excluded.lastError`, name, nowSQL(), message)
	return err
}
func (s *Store) OperationRuns() ([]OperationRun, error) {
	rows, err := s.db.Query(`SELECT name,lastRunAt,lastError FROM operation_runs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OperationRun, 0)
	for rows.Next() {
		var item OperationRun
		if err := rows.Scan(&item.Name, &item.LastRunAt, &item.LastError); err != nil {
			return nil, err
		}
		item.LastRunAt = fromSQL(item.LastRunAt).Format(time.RFC3339Nano)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) DatabasePath() string           { return s.path }
func (s *Store) OperationalCounts() (map[string]int64, int, error) {
	out := map[string]int64{}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return nil, 0, err
	}
	for _, table := range []string{"payment_orders", "api_usage_logs", "admin_audit_logs", "notify_attempts"} {
		var n int64
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			return nil, 0, err
		}
		out[table] = n
	}
	for name, query := range map[string]string{"failedCallbacks": `SELECT COUNT(*) FROM notify_queues WHERE status IN(2,3)`, "pendingCallbacks": `SELECT COUNT(*) FROM notify_queues WHERE status=0`, "pendingRefunds": `SELECT COUNT(*) FROM payment_orders WHERE status=3`} {
		var n int64
		if err := s.db.QueryRow(query).Scan(&n); err != nil {
			return nil, 0, err
		}
		out[name] = n
	}
	return out, version, nil
}
func (s *Store) Snapshot(path string) error {
	if s.path == "" {
		return fmt.Errorf("当前数据库不支持文件备份")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("备份目标已存在")
	} else if !os.IsNotExist(err) {
		return err
	}
	if filepath.Clean(path) == filepath.Clean(s.path) {
		return fmt.Errorf("备份目标不能覆盖当前数据库")
	}
	_, err := s.db.Exec(`VACUUM INTO ?`, path)
	if err == nil {
		err = os.Chmod(path, 0o600)
	}
	return err
}
