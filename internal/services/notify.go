package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/signature"
)

// PaymentNotification 回调 body 的形状。
//
// 字段序 = 原版对象字面量的书写顺序 —— body 字节要和签名时那份完全一致，
// 换顺序等于换字节，接入方验签就挂了。
type PaymentNotification struct {
	OrderNo           string  `json:"orderNo"`
	ExternalOrderNo   *string `json:"externalOrderNo,omitempty"`
	Status            string  `json:"status"`
	Amount            float64 `json:"amount"`
	PayMethod         string  `json:"payMethod"`
	PayAt             string  `json:"payAt"`
	ThirdPartyTradeNo *string `json:"thirdPartyTradeNo,omitempty"`
}

// notifySignature 写进 signature 列的结构。
//
// 必须是 struct 而不是 map：Go 的 map 按键名排序，{nonce, sign, timestamp}
// 和原版 {timestamp, nonce, sign} 的字节顺序对不上。
type notifySignature struct {
	Timestamp string `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Sign      string `json:"sign"`
	Version   int    `json:"version,omitempty"`
}

// parseSignature 收窄 JSON.parse 的结果。
//
// 非字符串字段一律给空串 —— httpclient.Post 会跳过空值 header，
// 保持和原版「字段缺失时 header 不发」一致：别兜底成一个空签名头。
func parseSignature(raw string) notifySignature {
	var out notifySignature
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return out
	}
	if v, ok := obj["timestamp"].(string); ok {
		out.Timestamp = v
	}
	if v, ok := obj["nonce"].(string); ok {
		out.Nonce = v
	}
	if v, ok := obj["sign"].(string); ok {
		out.Sign = v
	}
	if v, ok := obj["version"].(float64); ok && v == 2 {
		out.Version = 2
	}
	return out
}

// NotifyRepo NotifyService 依赖的队列读写。
//
// 声明成接口（consumer-side，Go 惯例）：*db.NotifyRepo 天然满足，
// 生产行为零变化；测试要塞假的 —— 尤其 FindRetryable 需要能返回
// 真库永远不会返回的行（已 Exhausted 的），来钉住 HandleNotifications
// 里的守卫分支。
type NotifyRepo interface {
	Insert(in db.InsertNotifyInput) error
	FindRetryable(limit int64) ([]*db.NotifyQueue, error)
	Update(id int64, patch map[string]any) error
}

// NotifyService 异步入队 + 定时重试的回调投递。
//
// 退避 5 档：5s / 15s / 60s / 300s / 900s，第 5 次失败置 Exhausted 不再投。
type NotifyService struct {
	log  *logger.Logger
	repo NotifyRepo
	http httpclient.Client

	delays []time.Duration
}

// NewNotifyService 构造。
func NewNotifyService(repo NotifyRepo, http httpclient.Client) *NotifyService {
	return &NotifyService{
		log:    logger.New("NotifyService"),
		repo:   repo,
		http:   http,
		delays: []time.Duration{5 * time.Second, 15 * time.Second, 60 * time.Second, 300 * time.Second, 900 * time.Second},
	}
}

// EnqueueNotification signs the serialized body with the supplied secret.
// Payment orders retain their signing snapshot across API token rotation.
func (s *NotifyService) EnqueueNotification(notifyURL string, notification PaymentNotification, signingKey string) error {
	in, err := s.PrepareNotification(notifyURL, notification, signingKey)
	if err != nil {
		return err
	}
	if err := s.repo.Insert(in); err != nil {
		return err
	}
	s.log.Log(fmt.Sprintf("Notification enqueued for order %s -> %s", notification.OrderNo, notifyURL))
	return nil
}

// PrepareNotification 只序列化和签名，不写库、不发网络请求。
// 支付状态机将返回的记录与订单变更一起写入同一事务。
func (s *NotifyService) PrepareNotification(notifyURL string, notification PaymentNotification, signingKey string) (db.InsertNotifyInput, error) {
	body, err := json.Marshal(notification)
	if err != nil {
		return db.InsertNotifyInput{}, err
	}
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	nonce := nonce8()
	sign := signature.Sign(signature.BuildPayload(string(body), timestamp, nonce), signingKey)

	meta, err := json.Marshal(notifySignature{Timestamp: timestamp, Nonce: nonce, Sign: sign})
	if err != nil {
		return db.InsertNotifyInput{}, err
	}
	orderNo := notification.OrderNo
	return db.InsertNotifyInput{
		OrderNo:   &orderNo,
		URL:       notifyURL,
		Body:      string(body),
		Signature: string(meta),
		Status:    db.NotifyPending,
	}, nil
}

// HandleNotifications 定时器每分钟调一次：把到期的待发/失败通知投出去。
func (s *NotifyService) HandleNotifications() error {
	items, err := s.repo.FindRetryable(50)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Status == db.NotifyExhausted {
			continue
		}
		if item.AvailableAt != nil && item.AvailableAt.After(time.Now()) {
			continue
		}
		if item.AvailableAt == nil && item.LastAttemptAt != nil {
			idx := int(item.RetryCount)
			if idx > len(s.delays)-1 {
				idx = len(s.delays) - 1
			}
			if idx < 0 {
				idx = 0
			}
			nextAttempt := item.LastAttemptAt.Add(s.delays[idx])
			if nextAttempt.After(time.Now()) {
				continue
			}
		}
		if diagnostics, ok := s.repo.(notifyDiagnosticsRepo); ok {
			item, err = diagnostics.ClaimDelivery(item.ID, time.Now())
			if err != nil {
				return err
			}
			if item == nil {
				continue
			}
		}
		if err := s.processNotify(item); err != nil {
			return err
		}
	}
	return nil
}

// processNotify 投一条。lastAttemptAt 先落地，解析/发送的失败都算一次重试。
func (s *NotifyService) processNotify(item *db.NotifyQueue) error {
	started := time.Now()
	patch := map[string]any{"lastAttemptAt": started, "availableAt": nil}
	attempt := db.NotifyAttempt{CreatedAt: started}

	err := func() error {
		// body 必须是合法 JSON 才发得出去 —— 签名是对 enqueue 时那份字符串算的，
		// 校验一遍能提前挡掉存坏的行。
		var payload any
		if err := json.Unmarshal([]byte(item.Body), &payload); err != nil {
			return err
		}
		sigInfo := parseSignature(item.Signature)
		version := sigInfo.Version
		if version == 0 {
			version = 1
		}
		resp, err := s.http.Post(item.URL, []byte(item.Body), map[string]string{
			"X-Sandbox-Pay-Timestamp":         sigInfo.Timestamp,
			"X-Sandbox-Pay-Nonce":             sigInfo.Nonce,
			"X-Sandbox-Pay-Signature":         sigInfo.Sign,
			"X-Sandbox-Pay-Signature-Version": strconv.Itoa(version),
		}, 5*time.Second)
		if resp != nil {
			attempt.HTTPStatus = &resp.Status
			attempt.ResponseSummary = s.responseSummary(resp, item)
		}
		if err != nil {
			return err
		}
		if resp == nil {
			return fmt.Errorf("未收到回调响应")
		}
		if resp.Status >= 200 && resp.Status < 300 {
			patch["status"] = db.NotifySuccess
			patch["lastError"] = nil
			s.log.Log(fmt.Sprintf("Notification success: %s (order: %s)", item.URL, tsOpt(item.OrderNo)))
			return nil
		}
		return fmt.Errorf("HTTP %d", resp.Status)
	}()
	if err != nil {
		attempt.ErrorMessage = sanitizeCallbackText(err.Error(), item)
		s.applyFailure(item, patch, attempt.ErrorMessage)
	}
	attempt.DurationMs = time.Since(started).Milliseconds()
	if diagnostics, ok := s.repo.(notifyDiagnosticsRepo); ok {
		return diagnostics.RecordAttempt(item.ID, attempt, patch)
	}
	return s.repo.Update(item.ID, patch)
}

// applyFailure 记一次失败：补 retryCount/lastError，到顶就置 Exhausted。
func (s *NotifyService) applyFailure(item *db.NotifyQueue, patch map[string]any, message string) {
	retry := item.RetryCount + 1
	patch["retryCount"] = retry
	patch["lastError"] = message
	if retry >= int64(len(s.delays)) {
		patch["status"] = db.NotifyExhausted
		s.log.Error(fmt.Sprintf("Notification exhausted: %s (order: %s), Error: %s", item.URL, tsOpt(item.OrderNo), message))
		return
	}
	patch["status"] = db.NotifyFailed
	idx := int(retry)
	if idx >= len(s.delays) {
		idx = len(s.delays) - 1
	}
	patch["availableAt"] = time.Now().Add(s.delays[idx])
	s.log.Warn(fmt.Sprintf("Notification failed: %s (order: %s), Retry: %d, Error: %s", item.URL, tsOpt(item.OrderNo), retry, message))
}
