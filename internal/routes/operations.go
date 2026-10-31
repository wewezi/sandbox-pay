package routes

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
)

type auditResponse struct {
	http.ResponseWriter
	status int
}

func (w *auditResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *auditResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
func (w *auditResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (a *App) captureAdminAudit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			monitored := r.Method != http.MethodGet || strings.Contains(r.URL.Path, "webhook-secret") || strings.Contains(r.URL.Path, "backups/") || strings.Contains(r.URL.Path, "/exports/")
			if !monitored {
				next.ServeHTTP(w, r)
				return
			}
			started := time.Now()
			recorded := &auditResponse{ResponseWriter: w}
			next.ServeHTTP(recorded, r)
			status := recorded.status
			if status == 0 {
				status = 200
			}
			actor := "未登录"
			var userID *int64
			if user := httpx.User(r.Context()); user != nil {
				actor = user.Username
				if id, err := strconv.ParseInt(fmt.Sprint(user.UserID), 10, 64); err == nil {
					userID = &id
				}
			}
			path := r.URL.Path
			parts := strings.Split(strings.Trim(path, "/"), "/")
			target := ""
			if len(parts) > 0 {
				target = parts[len(parts)-1]
			}
			redact := auditRedactor(r)
			err := a.c.Store.RecordAudit(db.AuditLog{UserID: userID, Username: redact(actor, 200), Method: redact(r.Method, 16), Path: redact(path, 300), Action: redact(auditAction(r), 100), Target: redact(target, 128), ClientIP: redact(httpx.ClientIP(r), 64), StatusCode: status, DurationMS: float64(time.Since(started).Microseconds()) / 1000, RequestID: redact(httpx.CorrelationID(r.Context()), 128)})
			if err != nil {
				a.log.Error("管理操作审计保存失败")
			}
		})
	}
}

var auditSecretPattern = regexp.MustCompile(`(?i)(?:sp_sandbox_[A-Za-z0-9_-]*|whsec_[A-Za-z0-9_-]*|sha256:[a-f0-9]{64})`)

func auditRedactor(r *http.Request) func(string, int) string {
	credential := ""
	if parts := strings.Fields(r.Header.Get("Authorization")); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		credential = parts[1]
	}
	return func(value string, limit int) string {
		if credential != "" {
			value = strings.ReplaceAll(value, credential, "[REDACTED]")
			value = strings.ReplaceAll(value, db.HashAPIKey(credential), "[REDACTED]")
		}
		value = auditSecretPattern.ReplaceAllString(value, "[REDACTED]")
		value = strings.Map(func(character rune) rune {
			if unicode.IsControl(character) {
				return ' '
			}
			return character
		}, value)
		characters := []rune(strings.TrimSpace(value))
		if len(characters) > limit {
			characters = characters[:limit]
		}
		return string(characters)
	}
}
func auditAction(r *http.Request) string {
	path := r.URL.Path
	for _, item := range []struct{ pattern, label string }{{"reset-data", "重置沙箱数据"}, {"/refund", "退款"}, {"/confirm", "确认收款"}, {"/replay", "重发回调"}, {"/simulate", "模拟回调"}, {"/scenarios", "模拟支付场景"}, {"webhook-secret", "读取回调密钥"}, {"/api-keys", "管理API令牌"}, {"/team/", "管理团队"}, {"/password", "修改密码"}, {"/sessions", "撤销会话"}, {"/backups", "数据库备份"}, {"/settings", "修改运行设置"}, {"/cleanup", "清理日志"}, {"/channels/check", "检查渠道连接"}, {"/exports/", "导出数据"}, {"/groups", "管理项目"}, {"/projects", "创建项目"}, {"/transactions/", "删除订单"}, {"/test-pay", "测试下单"}} {
		if strings.Contains(path, item.pattern) {
			return item.label
		}
	}
	return r.Method + " " + path
}

type operationSettingsInput struct {
	UsageRetentionDays   *int `json:"usageRetentionDays"`
	AuditRetentionDays   *int `json:"auditRetentionDays"`
	AttemptRetentionDays *int `json:"attemptRetentionDays"`
	BackupIntervalHours  *int `json:"backupIntervalHours"`
	BackupKeepCount      *int `json:"backupKeepCount"`
}

func (a *App) registerOperations(r chi.Router) {
	r.Get("/operations/status", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Operations.Status()
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
	r.Post("/operations/settings", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		var in operationSettingsInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			return httperr.BadRequest("运行设置格式不正确")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return httperr.BadRequest("运行设置包含多余内容")
		}
		if in.UsageRetentionDays == nil || in.AuditRetentionDays == nil || in.AttemptRetentionDays == nil || in.BackupIntervalHours == nil || in.BackupKeepCount == nil {
			return httperr.BadRequest("运行设置缺少必填字段")
		}
		settings := db.OperationSettings{UsageRetentionDays: *in.UsageRetentionDays, AuditRetentionDays: *in.AuditRetentionDays, AttemptRetentionDays: *in.AttemptRetentionDays, BackupIntervalHours: *in.BackupIntervalHours, BackupKeepCount: *in.BackupKeepCount}
		if err := a.c.Store.SaveOperationSettings(settings); err != nil {
			return httperr.BadRequest(err.Error())
		}
		out, err := a.c.Store.OperationSettings()
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
	r.Post("/operations/cleanup", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Store.CleanOperationLogs(time.Now())
		if err != nil {
			return err
		}
		_ = a.c.Store.RecordOperationRun("cleanup", "")
		httpx.OK(w, out)
		return nil
	}))
	r.Post("/operations/channels/check", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		httpx.OK(w, a.c.Operations.Channels(true))
		return nil
	}))
	r.Get("/operations/backups", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Operations.Backups()
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
	r.Post("/operations/backups", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Operations.Backup()
		if err != nil {
			return httperr.BadRequest("备份失败，请检查目录权限和磁盘空间")
		}
		httpx.OK(w, out)
		return nil
	}))
	r.Get("/operations/backups/{name}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		path, err := a.c.Operations.BackupPath(pathParam(r, "name"))
		if err != nil {
			return httperr.NotFound("备份不存在")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+pathParam(r, "name")+`"`)
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, path)
		return nil
	}))
	r.Get("/audit-logs", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		page, size := int64(1), int64(50)
		if v, err := strconv.ParseInt(queryParam(r, "page"), 10, 64); err == nil && v > 0 {
			page = v
		}
		if v, err := strconv.ParseInt(queryParam(r, "pageSize"), 10, 64); err == nil && v > 0 && v <= 200 {
			size = v
		}
		if page > 1000000 {
			return httperr.BadRequest("页码过大")
		}
		items, total, err := a.c.Store.AuditLogs(queryParam(r, "keyword"), size, (page-1)*size)
		if err != nil {
			return err
		}
		httpx.OK(w, services.PageResult[db.AuditLog]{Items: items, Total: total, Page: page, PageSize: size})
		return nil
	}))
	r.Get("/operations/exports/orders.csv", httpx.H(a.exportOrders()))
	r.Get("/operations/exports/usage.csv", httpx.H(a.exportUsage()))
}
func csvCell(value string) string {
	if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") || strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") {
		return "'" + value
	}
	return value
}
func csvWriter(w http.ResponseWriter, name string) (*csv.Writer, error) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return nil, err
	}
	return csv.NewWriter(w), nil
}
func (a *App) exportOrders() httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		offset, err := parseUTCOffset(queryParam(r, "utcOffset"))
		if err != nil {
			return err
		}
		opts := services.TransactionOpts{ProjectIDs: httpx.ProjectScope(r.Context()), GroupID: optInt64(r, "groupId"), Status: queryParam(r, "status"), PayMethod: queryParam(r, "payMethod"), Keyword: queryParam(r, "keyword"), DateFrom: queryParam(r, "dateFrom"), DateTo: queryParam(r, "dateTo"), UTCOffset: offset, Page: "1", PageSize: "200"}
		result, err := a.c.Admin.GetTransactions(opts)
		if err != nil {
			return err
		}
		writer, err := csvWriter(w, "sandbox-orders.csv")
		if err != nil {
			return err
		}
		defer writer.Flush()
		if err = writer.Write([]string{"订单号", "外部订单号", "商品", "项目", "渠道", "状态", "金额", "累计退款", "手续费", "创建时间", "来源IP"}); err != nil {
			return err
		}
		for page := 1; ; page++ {
			if r.Context().Err() != nil {
				return r.Context().Err()
			}
			if page > 1 {
				opts.Page = strconv.Itoa(page)
				result, err = a.c.Admin.GetTransactions(opts)
				if err != nil {
					return err
				}
			}
			for _, item := range result.Items {
				external, ip, group := "", "", ""
				if item.ExternalOrderNo != nil {
					external = *item.ExternalOrderNo
				}
				if item.ClientIP != nil {
					ip = *item.ClientIP
				}
				if item.GroupName != nil {
					group = *item.GroupName
				}
				row := []string{item.OrderNo, external, item.ProductName, group, item.PayMethod, item.Status, fmt.Sprintf("%.2f", item.Amount), fmt.Sprintf("%.2f", item.RefundedAmount), fmt.Sprintf("%.2f", item.Fee), item.CreatedAt, ip}
				for i := range row {
					row[i] = csvCell(row[i])
				}
				if err = writer.Write(row); err != nil {
					return err
				}
			}
			if int64(page)*result.PageSize >= result.Total || len(result.Items) == 0 {
				break
			}
		}
		writer.Flush()
		return writer.Error()
	}
}
func (a *App) exportUsage() httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		offset, err := parseUTCOffset(queryParam(r, "utcOffset"))
		if err != nil {
			return err
		}
		opts := services.UsageLogOpts{ProjectIDs: httpx.ProjectScope(r.Context()), GroupID: optInt64(r, "groupId"), APIKeyID: optInt64(r, "apiKeyId"), Keyword: queryParam(r, "keyword"), Method: queryParam(r, "method"), Status: queryParam(r, "status"), DateFrom: queryParam(r, "dateFrom"), DateTo: queryParam(r, "dateTo"), UTCOffset: offset, Page: "1", PageSize: "200"}
		result, err := a.c.Usage.GetLogs(opts)
		if err != nil {
			return err
		}
		writer, err := csvWriter(w, "sandbox-usage.csv")
		if err != nil {
			return err
		}
		defer writer.Flush()
		if err = writer.Write([]string{"请求ID", "方法", "路径", "HTTP状态", "耗时ms", "项目", "令牌备注", "订单号", "来源IP", "错误", "时间"}); err != nil {
			return err
		}
		for page := 1; ; page++ {
			if r.Context().Err() != nil {
				return r.Context().Err()
			}
			if page > 1 {
				opts.Page = strconv.Itoa(page)
				result, err = a.c.Usage.GetLogs(opts)
				if err != nil {
					return err
				}
			}
			for _, item := range result.Items {
				row := []string{item.RequestID, item.Method, item.Path, strconv.Itoa(item.StatusCode), fmt.Sprintf("%.3f", item.DurationMs), item.GroupName, item.APIKeyName, item.OrderNo, item.ClientIP, item.ErrorMessage, item.CreatedAt}
				for i := range row {
					row[i] = csvCell(row[i])
				}
				if err = writer.Write(row); err != nil {
					return err
				}
			}
			if int64(page)*result.PageSize >= result.Total || len(result.Items) == 0 {
				break
			}
		}
		writer.Flush()
		return writer.Error()
	}
}
