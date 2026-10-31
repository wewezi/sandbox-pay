package services

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
)

func seedDiagnosticNotify(t *testing.T, store *db.Store) int64 {
	t.Helper()
	orderNo := "PAY_DIAGNOSTIC"
	id, err := store.Notifies.InsertReturningID(db.InsertNotifyInput{OrderNo: &orderNo, URL: "https://callback.example/notify", Body: `{ "orderNo":"PAY_DIAGNOSTIC", "status":"paid", "amount":12.34 }`, Signature: `{"timestamp":"1793412345678","nonce":"diagnostic-nonce","sign":"diagnostic-signature"}`, Status: db.NotifyPending})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCallbackDiagnosticsHistoryAndRequeue(t *testing.T) {
	store := newTestStore(t)
	id := seedDiagnosticNotify(t, store)
	http := newFakeHTTP()
	http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
		return &httpclient.Response{Status: 503, BodySummary: "temporarily unavailable"}, nil
	}
	service := NewNotifyService(store.Notifies, http)
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetNotificationDetail(id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != db.NotifyFailed || len(detail.Attempts) != 1 || detail.Attempts[0].HTTPStatus == nil || *detail.Attempts[0].HTTPStatus != 503 || detail.Attempts[0].ErrorMessage != "HTTP 503" || detail.Attempts[0].ResponseSummary != "temporarily unavailable" || detail.NextRetryAt == nil {
		t.Fatalf("failed detail = %+v", detail)
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	if http.postCalls != 1 {
		t.Fatal("retry was sent before its availability time")
	}
	if queued, err := store.Notifies.Requeue(id); err != nil || !queued {
		t.Fatalf("Requeue = %v, %v", queued, err)
	}
	http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
		return &httpclient.Response{Status: 204}, nil
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	detail, err = service.GetNotificationDetail(id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != db.NotifySuccess || detail.LastError != nil || detail.NextRetryAt != nil || len(detail.Attempts) != 2 || detail.Attempts[1].AttemptNo != 2 || detail.Attempts[1].ErrorMessage != "" {
		t.Fatalf("success detail = %+v", detail)
	}
	if err := store.Notifies.Clear(); err != nil {
		t.Fatal(err)
	}
	attempts, err := store.Notifies.ListAttempts(id)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("clear attempts=%v err=%v", attempts, err)
	}
}

func TestCallbackSimulationPreservesSignedBytesAndSchedulesAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	store, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	id := seedDiagnosticNotify(t, store)
	source, _ := store.Notifies.FindByID(id)
	if err := store.Notifies.Update(id, map[string]any{"status": db.NotifySuccess}); err != nil {
		t.Fatal(err)
	}
	http := newFakeHTTP()
	service := NewNotifyService(store.Notifies, http)
	duplicate, err := service.SimulateNotification(id, "duplicate", 0)
	if err != nil {
		t.Fatal(err)
	}
	copy, _ := store.Notifies.FindByID(duplicate.ID)
	if copy.Body != source.Body || copy.Signature != source.Signature || copy.Status != db.NotifySuccess || toBodyString(http.lastPostBody) != source.Body || http.lastPostHeaders["X-Sandbox-Pay-Signature"] != "diagnostic-signature" {
		t.Fatalf("duplicate did not preserve delivery: %+v", copy)
	}
	if http.postCalls != 1 {
		t.Fatalf("duplicate deliveries=%d", http.postCalls)
	}
	delayed, err := service.SimulateNotification(id, "delayed", 300)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	service = NewNotifyService(store.Notifies, http)
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	if http.postCalls != 1 {
		t.Fatal("delayed callback sent early after restart")
	}
	detail, err := service.GetNotificationDetail(delayed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.AvailableAt == nil || detail.SourceNotificationID == nil || *detail.SourceNotificationID != id || detail.SimulationMode != "delayed" || len(detail.Attempts) != 0 {
		t.Fatalf("delayed detail=%+v", detail)
	}
	if err := store.Notifies.Update(delayed.ID, map[string]any{"availableAt": time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	if http.postCalls != 2 {
		t.Fatalf("due callback deliveries=%d", http.postCalls)
	}
	detail, _ = service.GetNotificationDetail(delayed.ID)
	if detail.Status != db.NotifySuccess || len(detail.Attempts) != 1 {
		t.Fatalf("delayed result=%+v", detail)
	}
}

func TestCallbackDeliveryLeasePreventsConcurrentDeliveryAndRequeue(t *testing.T) {
	store := newTestStore(t)
	id := seedDiagnosticNotify(t, store)
	claimed, err := store.Notifies.ClaimDelivery(id, time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	second, err := store.Notifies.ClaimDelivery(id, time.Now())
	if err != nil || second != nil {
		t.Fatalf("second claim=%v err=%v", second, err)
	}
	if queued, err := store.Notifies.Requeue(id); err != nil || queued {
		t.Fatalf("active delivery requeued=%v err=%v", queued, err)
	}
	http := newFakeHTTP()
	service := NewNotifyService(store.Notifies, http)
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	if http.postCalls != 0 {
		t.Fatal("leased callback was delivered twice")
	}
	if err := store.Notifies.Update(id, map[string]any{"leaseUntil": time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	if http.postCalls != 1 {
		t.Fatal("expired lease did not recover")
	}
}

func TestCallbackDiagnosticsRedactsCredentialsAndBoundsResponses(t *testing.T) {
	store := newTestStore(t)
	id := seedDiagnosticNotify(t, store)
	http := newFakeHTTP()
	http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
		return &httpclient.Response{Status: 200, Data: map[string]any{"authorization": "Bearer credential-value", "nested": []any{map[string]any{"api_key": "sensitive-api-value"}}, "message": "diagnostic-signature diagnostic-nonce 1793412345678", "large": strings.Repeat("x", 4000)}}, nil
	}
	service := NewNotifyService(store.Notifies, http)
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetNotificationDetail(id)
	if err != nil {
		t.Fatal(err)
	}
	summary := detail.Attempts[0].ResponseSummary
	for _, secret := range []string{"credential-value", "sensitive-api-value", "diagnostic-signature", "diagnostic-nonce", "1793412345678"} {
		if strings.Contains(summary, secret) {
			t.Fatalf("summary leaks %s: %s", secret, summary)
		}
	}
	if len(summary) > 2051 {
		t.Fatalf("unbounded summary: %d bytes", len(summary))
	}
	if queued, err := store.Notifies.Requeue(id); err != nil || !queued {
		t.Fatal(err)
	}
	http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
		return nil, errors.New("Post https://user:password@example.test/hook?api_key=error-secret: timeout diagnostic-signature")
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	detail, _ = service.GetNotificationDetail(id)
	message := detail.Attempts[1].ErrorMessage
	for _, secret := range []string{"user:password", "error-secret", "diagnostic-signature"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error leaks %s: %s", secret, message)
		}
	}
	if queued, err := store.Notifies.Requeue(id); err != nil || !queued {
		t.Fatal(err)
	}
	http.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
		return &httpclient.Response{Status: 502, BodySummary: "client_secret=plain-secret signing_key=plain-signing-token X-Sandbox-Pay-Nonce=diagnostic-nonce"}, errors.New("response read failed")
	}
	if err := service.HandleNotifications(); err != nil {
		t.Fatal(err)
	}
	detail, _ = service.GetNotificationDetail(id)
	last := detail.Attempts[2]
	if last.HTTPStatus == nil || *last.HTTPStatus != 502 {
		t.Fatal("HTTP status lost on response read failure")
	}
	for _, secret := range []string{"plain-secret", "plain-signing-token", "diagnostic-nonce"} {
		if strings.Contains(last.ResponseSummary, secret) {
			t.Fatalf("plain text leaks %s: %s", secret, last.ResponseSummary)
		}
	}
}

func TestCallbackSimulationRejectsInvalidInput(t *testing.T) {
	store := newTestStore(t)
	id := seedDiagnosticNotify(t, store)
	service := NewNotifyService(store.Notifies, newFakeHTTP())
	for _, test := range []struct {
		mode  string
		delay int
	}{{"unknown", 0}, {"delayed", 0}, {"delayed", 3601}, {"duplicate", 1}} {
		_, err := service.SimulateNotification(id, test.mode, test.delay)
		requireErrStatus(t, err, 400)
	}
	_, err := service.SimulateNotification(99999, "duplicate", 0)
	requireErrStatus(t, err, 404)
}
