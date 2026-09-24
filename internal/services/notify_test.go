package services

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/signature"
)

// ---------------------------------------------------------------------------
// 假 NotifyRepo —— 对应 TS createService 里的 fn() 桩
// ---------------------------------------------------------------------------

type notifyUpdateCall struct {
	id    int64
	patch map[string]any
}

type fakeNotifyRepo struct {
	insertCalls []db.InsertNotifyInput
	retryable   []*db.NotifyQueue
	updateCalls []notifyUpdateCall
}

func (f *fakeNotifyRepo) Insert(in db.InsertNotifyInput) error {
	f.insertCalls = append(f.insertCalls, in)
	return nil
}

func (f *fakeNotifyRepo) FindRetryable(int64) ([]*db.NotifyQueue, error) {
	return f.retryable, nil
}

func (f *fakeNotifyRepo) Update(id int64, patch map[string]any) error {
	f.updateCalls = append(f.updateCalls, notifyUpdateCall{id: id, patch: patch})
	return nil
}

// patchOf 取 update(id, patch) 里的第 i 个 patch。
func (f *fakeNotifyRepo) patchOf(i int) map[string]any {
	if i >= len(f.updateCalls) {
		return nil
	}
	return f.updateCalls[i].patch
}

// mockNotifyQueue 对应 TS 的 createMockNotify(overrides)。
func mockNotifyQueue(mutate func(*db.NotifyQueue)) *db.NotifyQueue {
	orderNo := "PAY123"
	n := &db.NotifyQueue{
		ID:         1,
		OrderNo:    &orderNo,
		URL:        "https://example.com/notify",
		Body:       `{"orderNo":"PAY123","status":"paid","amount":100}`,
		Signature:  `{"timestamp":"1234567890","nonce":"abc123","sign":"sig"}`,
		Status:     db.NotifyPending,
		RetryCount: 0,
		CreatedAt:  time.Now(),
	}
	if mutate != nil {
		mutate(n)
	}
	return n
}

func createNotifyService() (*NotifyService, *fakeNotifyRepo, *fakeHTTP) {
	repo := &fakeNotifyRepo{}
	http := newFakeHTTP()
	return NewNotifyService(repo, http), repo, http
}

// Port of server/src/services/notify.service.spec.ts
func TestNotifyService(t *testing.T) {
	t.Run("enqueueNotification/inserts a pending notification with a verifiable signature", func(t *testing.T) {
		service, repo, _ := createNotifyService()

		err := service.EnqueueNotification("https://example.com/notify", PaymentNotification{
			OrderNo:   "PAY123",
			Status:    "paid",
			Amount:    100,
			PayMethod: "alipay",
			PayAt:     time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		}, "secret")
		if err != nil {
			t.Fatalf("EnqueueNotification: %v", err)
		}

		if len(repo.insertCalls) != 1 {
			t.Fatalf("insert called %d times, want 1", len(repo.insertCalls))
		}
		row := repo.insertCalls[0]
		if row.OrderNo == nil || *row.OrderNo != "PAY123" {
			t.Errorf("orderNo = %v, want PAY123", row.OrderNo)
		}
		if row.URL != "https://example.com/notify" {
			t.Errorf("url = %q, want https://example.com/notify", row.URL)
		}
		if row.Status != db.NotifyPending {
			t.Errorf("status = %d, want Pending(%d)", row.Status, db.NotifyPending)
		}

		// 签名函数是纯函数、没法 spy，所以直接验签 ——
		// 真拿 secret 校一遍：对的密钥过、错的密钥不过。
		var meta struct {
			Timestamp string `json:"timestamp"`
			Nonce     string `json:"nonce"`
			Sign      string `json:"sign"`
		}
		if err := json.Unmarshal([]byte(row.Signature), &meta); err != nil {
			t.Fatalf("parse signature meta: %v", err)
		}
		payload := signature.BuildPayload(row.Body, meta.Timestamp, meta.Nonce)
		if !signature.Verify(payload, "secret", meta.Sign) {
			t.Error("signature must verify with the signing key")
		}
		if signature.Verify(payload, "wrong", meta.Sign) {
			t.Error("signature must not verify with a wrong key")
		}
	})

	t.Run("handleNotifications/skips items not yet due for retry", func(t *testing.T) {
		service, repo, http := createNotifyService()
		repo.retryable = []*db.NotifyQueue{mockNotifyQueue(func(n *db.NotifyQueue) {
			n.Status = db.NotifyFailed
			n.RetryCount = 1
			at := time.Now().Add(-time.Second) // 1 秒前，退避是 15 秒
			n.LastAttemptAt = &at
		})}

		if err := service.HandleNotifications(); err != nil {
			t.Fatalf("HandleNotifications: %v", err)
		}

		if http.postCalls != 0 {
			t.Errorf("http.post called %d times, want 0", http.postCalls)
		}
		if len(repo.updateCalls) != 0 {
			t.Errorf("update called %d times, want 0", len(repo.updateCalls))
		}
	})

	t.Run("handleNotifications/processes items that are due for retry", func(t *testing.T) {
		service, repo, http := createNotifyService()
		repo.retryable = []*db.NotifyQueue{mockNotifyQueue(func(n *db.NotifyQueue) {
			n.Status = db.NotifyFailed
			n.RetryCount = 1
			at := time.Now().Add(-time.Minute) // 60 秒前
			n.LastAttemptAt = &at
		})}

		if err := service.HandleNotifications(); err != nil {
			t.Fatalf("HandleNotifications: %v", err)
		}

		if http.postCalls == 0 {
			t.Fatal("http.post was not called, want a delivery attempt")
		}
		patch := repo.patchOf(0)
		if patch == nil {
			t.Fatal("update was not called")
		}
		if patch["status"] != db.NotifySuccess {
			t.Errorf("patch.status = %v, want Success(%d)", patch["status"], db.NotifySuccess)
		}
	})

	t.Run("handleNotifications/forwards the stored signature headers", func(t *testing.T) {
		service, repo, http := createNotifyService()
		repo.retryable = []*db.NotifyQueue{mockNotifyQueue(nil)}

		if err := service.HandleNotifications(); err != nil {
			t.Fatalf("HandleNotifications: %v", err)
		}
		if http.postCalls == 0 {
			t.Fatal("http.post was not called")
		}

		if http.lastPostURL != "https://example.com/notify" {
			t.Errorf("post url = %q, want https://example.com/notify", http.lastPostURL)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(toBodyString(http.lastPostBody)), &body); err != nil {
			t.Fatalf("post body is not JSON: %v (body=%q)", err, http.lastPostBody)
		}
		if body["orderNo"] != "PAY123" {
			t.Errorf("body.orderNo = %v, want PAY123", body["orderNo"])
		}
		if body["status"] != "paid" {
			t.Errorf("body.status = %v, want paid", body["status"])
		}
		wantHeaders := map[string]string{
			"X-Sandbox-Pay-Timestamp": "1234567890",
			"X-Sandbox-Pay-Nonce":     "abc123",
			"X-Sandbox-Pay-Signature": "sig",
		}
		for k, want := range wantHeaders {
			if http.lastPostHeaders[k] != want {
				t.Errorf("header %s = %q, want %q", k, http.lastPostHeaders[k], want)
			}
		}
	})

	t.Run("handleNotifications/counts a non-2xx response as a failure", func(t *testing.T) {
		service, repo, http := createNotifyService()
		repo.retryable = []*db.NotifyQueue{mockNotifyQueue(nil)}
		http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
			return &httpclient.Response{Status: 500, Data: nil}, nil
		}

		if err := service.HandleNotifications(); err != nil {
			t.Fatalf("HandleNotifications: %v", err)
		}

		patch := repo.patchOf(0)
		if patch == nil {
			t.Fatal("update was not called")
		}
		if patch["status"] != db.NotifyFailed {
			t.Errorf("patch.status = %v, want Failed(%d)", patch["status"], db.NotifyFailed)
		}
		if patch["retryCount"] != int64(1) {
			t.Errorf("patch.retryCount = %v, want int64(1)", patch["retryCount"])
		}
		if patch["lastError"] != "HTTP 500" {
			t.Errorf("patch.lastError = %v, want %q", patch["lastError"], "HTTP 500")
		}
	})

	t.Run("handleNotifications/marks notification as exhausted after max retries", func(t *testing.T) {
		service, repo, http := createNotifyService()
		repo.retryable = []*db.NotifyQueue{mockNotifyQueue(func(n *db.NotifyQueue) {
			n.Status = db.NotifyFailed
			n.RetryCount = 4 // 第 5 次失败就到顶（delays 有 5 档）
			at := time.Unix(0, 0)
			n.LastAttemptAt = &at
		})}
		http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
			return nil, errors.New("timeout")
		}

		if err := service.HandleNotifications(); err != nil {
			t.Fatalf("HandleNotifications: %v", err)
		}

		patch := repo.patchOf(0)
		if patch == nil {
			t.Fatal("update was not called")
		}
		if patch["status"] != db.NotifyExhausted {
			t.Errorf("patch.status = %v, want Exhausted(%d)", patch["status"], db.NotifyExhausted)
		}
		if patch["retryCount"] != int64(5) {
			t.Errorf("patch.retryCount = %v, want int64(5)", patch["retryCount"])
		}
		if patch["lastError"] != "timeout" {
			t.Errorf("patch.lastError = %v, want %q", patch["lastError"], "timeout")
		}
	})

	t.Run("handleNotifications/never touches an already-exhausted item", func(t *testing.T) {
		// 真库的 FindRetryable 永远不会返回 Exhausted 行 —— 假 repo 故意返回，
		// 钉住 HandleNotifications 里那道守卫。
		service, repo, http := createNotifyService()
		repo.retryable = []*db.NotifyQueue{mockNotifyQueue(func(n *db.NotifyQueue) {
			n.Status = db.NotifyExhausted
			n.RetryCount = 5
		})}

		if err := service.HandleNotifications(); err != nil {
			t.Fatalf("HandleNotifications: %v", err)
		}

		if http.postCalls != 0 {
			t.Errorf("http.post called %d times, want 0", http.postCalls)
		}
		if len(repo.updateCalls) != 0 {
			t.Errorf("update called %d times, want 0", len(repo.updateCalls))
		}
	})
}

// toBodyString Post 的 body 是 enqueue 时那份原始 []byte。
func toBodyString(body any) string {
	if b, ok := body.([]byte); ok {
		return string(b)
	}
	if s, ok := body.(string); ok {
		return s
	}
	return ""
}
