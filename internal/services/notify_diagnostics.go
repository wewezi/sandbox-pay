package services

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/httperr"
)

type notifyDiagnosticsRepo interface {
	FindByID(int64) (*db.NotifyQueue, error)
	ClaimDelivery(int64, time.Time) (*db.NotifyQueue, error)
	RecordAttempt(int64, db.NotifyAttempt, map[string]any) error
	ListAttempts(int64) ([]db.NotifyAttempt, error)
	SimulationMeta(int64) (db.NotifySimulationMeta, error)
	CloneSimulation(int64, string, time.Time) (*db.NotifyQueue, error)
}

type CallbackAttemptItem struct {
	ID              int64  `json:"id"`
	AttemptNo       int64  `json:"attemptNo"`
	HTTPStatus      *int   `json:"httpStatus,omitempty"`
	ResponseSummary string `json:"responseSummary"`
	DurationMs      int64  `json:"durationMs"`
	ErrorMessage    string `json:"errorMessage"`
	CreatedAt       string `json:"createdAt"`
}

type NotificationDetail struct {
	NotificationItem
	SimulationEnabled    bool                  `json:"simulationEnabled"`
	Body                 string                `json:"body"`
	Attempts             []CallbackAttemptItem `json:"attempts"`
	NextRetryAt          *string               `json:"nextRetryAt,omitempty"`
	AvailableAt          *string               `json:"availableAt,omitempty"`
	SourceNotificationID *int64                `json:"sourceNotificationId,omitempty"`
	SimulationMode       string                `json:"simulationMode,omitempty"`
}

type CallbackSimulationResult struct {
	ID                   int64   `json:"id"`
	SourceNotificationID int64   `json:"sourceNotificationId"`
	Mode                 string  `json:"mode"`
	AvailableAt          string  `json:"availableAt"`
	OrderNo              *string `json:"orderNo,omitempty"`
}

func (s *NotifyService) GetNotificationDetail(id int64) (*NotificationDetail, error) {
	repo, ok := s.repo.(notifyDiagnosticsRepo)
	if !ok {
		return nil, httperr.Internal("回调诊断不可用")
	}
	item, err := repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, httperr.NotFound("通知记录不存在")
	}
	attempts, err := repo.ListAttempts(id)
	if err != nil {
		return nil, err
	}
	meta, err := repo.SimulationMeta(id)
	if err != nil {
		return nil, err
	}
	out := &NotificationDetail{NotificationItem: NotificationItem{ID: item.ID, OrderNo: item.OrderNo, URL: sanitizeCallbackURL(item.URL), Status: item.Status, RetryCount: item.RetryCount, LastError: item.LastError, LastAttemptAt: jsTimePtr(item.LastAttemptAt), CreatedAt: JsTime(item.CreatedAt)}, Body: sanitizeCallbackBody(item.Body, item), Attempts: make([]CallbackAttemptItem, 0, len(attempts)), AvailableAt: jsTimePtr(item.AvailableAt), SourceNotificationID: meta.SourceNotificationID, SimulationMode: meta.Mode}
	if item.Status == db.NotifyPending || item.Status == db.NotifyFailed {
		next := time.Now()
		if item.AvailableAt != nil {
			next = *item.AvailableAt
		} else if item.LastAttemptAt != nil {
			index := int(item.RetryCount)
			if index < 0 {
				index = 0
			}
			if index >= len(s.delays) {
				index = len(s.delays) - 1
			}
			next = item.LastAttemptAt.Add(s.delays[index])
		}
		out.NextRetryAt = jsTimePtr(&next)
	}
	if out.LastError != nil {
		value := sanitizeCallbackText(*out.LastError, item)
		out.LastError = &value
	}
	for _, attempt := range attempts {
		out.Attempts = append(out.Attempts, CallbackAttemptItem{ID: attempt.ID, AttemptNo: attempt.AttemptNo, HTTPStatus: attempt.HTTPStatus, ResponseSummary: sanitizeCallbackText(attempt.ResponseSummary, item), DurationMs: attempt.DurationMs, ErrorMessage: sanitizeCallbackText(attempt.ErrorMessage, item), CreatedAt: JsTime(attempt.CreatedAt)})
	}
	return out, nil
}

func (s *NotifyService) SimulateNotification(id int64, mode string, delaySeconds int) (*CallbackSimulationResult, error) {
	if mode != "duplicate" && mode != "delayed" {
		return nil, httperr.BadRequest("回调模拟类型无效")
	}
	if mode == "delayed" && (delaySeconds < 1 || delaySeconds > 3600) {
		return nil, httperr.BadRequest("延迟时间必须为 1 至 3600 秒")
	}
	if mode == "duplicate" && delaySeconds != 0 {
		return nil, httperr.BadRequest("重复回调不需要延迟时间")
	}
	repo, ok := s.repo.(notifyDiagnosticsRepo)
	if !ok {
		return nil, httperr.Internal("回调诊断不可用")
	}
	available := time.Now().Add(time.Duration(delaySeconds) * time.Second)
	item, err := repo.CloneSimulation(id, mode, available)
	if err == sql.ErrNoRows {
		return nil, httperr.NotFound("通知记录不存在")
	}
	if err != nil {
		return nil, err
	}
	if mode == "duplicate" {
		claimed, err := repo.ClaimDelivery(item.ID, time.Now())
		if err != nil {
			return nil, err
		}
		if claimed != nil {
			if err := s.processNotify(claimed); err != nil {
				return nil, err
			}
		}
	}
	return &CallbackSimulationResult{ID: item.ID, SourceNotificationID: id, Mode: mode, AvailableAt: JsTime(available), OrderNo: item.OrderNo}, nil
}

func sensitiveCallbackKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(key))
	return strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "password") || strings.Contains(key, "signature") || strings.Contains(key, "apikey") || key == "authorization" || key == "auth" || key == "signingkey" || key == "privatekey" || key == "credential" || key == "credentials" || key == "key" || key == "sign" || key == "nonce" || key == "timestamp"
}

func redactCallbackValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if sensitiveCallbackKey(key) {
				out[key] = "[已隐藏]"
			} else {
				out[key] = redactCallbackValue(value)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = redactCallbackValue(value)
		}
		return out
	default:
		return value
	}
}

func sanitizeCallbackURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[回调地址]"
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveCallbackKey(key) {
			query.Set(key, "[已隐藏]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

var callbackURLPattern = regexp.MustCompile(`https?://[^\s"<>]+`)
var callbackCredentialPattern = regexp.MustCompile(`(?i)([a-z0-9_-]*(?:secret|token|password|signature|authorization|credential)[a-z0-9_-]*|api[-_ ]?key|(?:x-sandbox-pay-)?(?:nonce|timestamp|sign)|signing[-_]?key|private[-_]?key|auth)["']?\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;&}]+)`)
var callbackBearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)

func sanitizeCallbackText(raw string, item *db.NotifyQueue) string {
	raw = callbackURLPattern.ReplaceAllStringFunc(raw, sanitizeCallbackURL)
	raw = callbackCredentialPattern.ReplaceAllString(raw, "$1=[已隐藏]")
	raw = callbackBearerPattern.ReplaceAllString(raw, "Bearer [已隐藏]")
	meta := parseSignature(item.Signature)
	for _, value := range []string{meta.Sign, meta.Nonce, meta.Timestamp} {
		if len(value) >= 3 {
			raw = strings.ReplaceAll(raw, value, "[已隐藏]")
		}
	}
	raw = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, raw)
	if len(raw) > 2048 {
		raw = strings.ToValidUTF8(raw[:2048], "") + "…"
	}
	return strings.TrimSpace(raw)
}

func sanitizeCallbackBody(raw string, item *db.NotifyQueue) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) == nil {
		data, err := json.MarshalIndent(redactCallbackValue(value), "", "  ")
		if err == nil {
			return sanitizeCallbackText(string(data), item)
		}
	}
	return sanitizeCallbackText(raw, item)
}

func (s *NotifyService) responseSummary(response *httpclient.Response, item *db.NotifyQueue) string {
	if response.Data != nil {
		if data, err := json.Marshal(redactCallbackValue(response.Data)); err == nil {
			return sanitizeCallbackText(string(data), item)
		}
	}
	return sanitizeCallbackText(response.BodySummary, item)
}
