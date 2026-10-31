package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

func TestUsageCaptureSnapshotsGuardContextAndIgnoresBodies(t *testing.T) {
	const secret = "sp_sandbox_abcdef0123456789"
	const webhook = "whsec_0123456789abcdef"
	var logged UsageRecord
	options := UsageOptions{Record: func(record UsageRecord) error {
		logged = record
		return nil
	}}
	guard := RequireAPIKey(&usageGuardRepo{hit: &db.ActiveKeyHit{
		Group: &db.Group{ID: 2, Name: "project", WebhookSecret: webhook},
		Key:   &db.APIKey{ID: 3, GroupID: 2, Name: "token", KeyHash: db.HashAPIKey(secret), KeyPrefix: "sp_sandbox_abcd", KeySuffix: "6789"},
	}})
	handler := Correlation(CaptureAPIUsage(options)(guard(H(func(w http.ResponseWriter, r *http.Request) error {
		if Group(r.Context()) == nil || APIKey(r.Context()) == nil {
			t.Fatal("guard metadata absent from handler context")
		}
		BindUsageOrder(r.Context(), "PAY_123456")
		OK(w, map[string]any{"secret": secret, "webhook": webhook})
		return nil
	}))))
	r := httptest.NewRequest("POST", "/v1/api/gateway/pay?token="+secret, strings.NewReader(`{"requestSecret":"`+webhook+`"}`))
	r.RemoteAddr = "[::ffff:8.8.8.8]:1234"
	r.Header.Set("Authorization", "bEaReR "+secret)
	r.Header.Set("X-Correlation-ID", "usage-context-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || logged.StatusCode != http.StatusOK || logged.RequestID != "usage-context-1" {
		t.Fatalf("response/request status metadata mismatch: response=%d log=%+v", rec.Code, logged)
	}
	if logged.APIKeyID == nil || *logged.APIKeyID != 3 || logged.GroupID == nil || *logged.GroupID != 2 || logged.GroupName != "project" || logged.APIKeyName != "token" {
		t.Fatalf("guard's cloned context attribution was lost: %+v", logged)
	}
	if logged.OrderNo != "PAY_123456" || logged.Path != "/v1/api/gateway/pay" || logged.ClientIP != "8.8.8.8" || logged.DurationMS < 0 || logged.CreatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("invalid usage metadata: %+v", logged)
	}
	stored, err := json.Marshal(logged)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, webhook, db.HashAPIKey(secret), "requestSecret", "?token="} {
		if strings.Contains(string(stored), forbidden) {
			t.Fatalf("usage log contains credential/body/query material %q: %s", forbidden, stored)
		}
	}
}

type usageGuardRepo struct{ hit *db.ActiveKeyHit }

func (r *usageGuardRepo) FindActiveByKey(string) (*db.ActiveKeyHit, error) { return r.hit, nil }
func (r *usageGuardRepo) Touch(int64) error                                { return nil }

func TestUsageCaptureRedactsPublicErrorsAndCorrelationIDs(t *testing.T) {
	const token = "custom-opaque-token"
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"public", httperr.BadRequest("bad " + token + " whsec_sensitive sp_sandbox_sensitive " + db.HashAPIKey(token)), "bad [REDACTED] [REDACTED] [REDACTED] [REDACTED]"},
		{"internal", errors.New("private SQL password " + token), "Internal server error"},
		{"explicit 500", httperr.Internal("private provider credentials"), "Internal server error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logged UsageRecord
			capture := CaptureAPIUsage(UsageOptions{Record: func(record UsageRecord) error { logged = record; return nil }})
			handler := Correlation(capture(H(func(w http.ResponseWriter, r *http.Request) error {
				BindUsageOrder(r.Context(), token)
				return test.err
			})))
			r := httptest.NewRequest("POST", "/gateway/pay?password=ignore", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-Correlation-ID", token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if logged.RequestID != "[REDACTED]" || logged.ErrorMessage != test.want || logged.OrderNo != "" {
				t.Fatalf("sensitive error/request metadata survived: %+v", logged)
			}
		})
	}
}

func TestUsageCapturePreservesResponseAndSurvivesStorageFailure(t *testing.T) {
	for _, fail := range []struct {
		name string
		sink func(UsageRecord) error
	}{
		{"error", func(UsageRecord) error { return errors.New("sqlite unavailable") }},
		{"panic", func(UsageRecord) error { panic("storage panic") }},
	} {
		t.Run(fail.name, func(t *testing.T) {
			warned := 0
			handler := CaptureAPIUsage(UsageOptions{Record: fail.sink, WriteFailed: func() { warned++ }})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Test", "preserved")
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("caller response"))
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest("POST", "/gateway/pay", nil))
			if rec.Code != http.StatusAccepted || rec.Body.String() != "caller response" || rec.Header().Get("X-Test") != "preserved" || warned != 1 {
				t.Fatalf("logging failure affected response: status=%d body=%q warnings=%d", rec.Code, rec.Body.String(), warned)
			}
		})
	}
}

func TestUsageCaptureSupportsResponseControllerAndFinalStatus(t *testing.T) {
	var logged UsageRecord
	handler := CaptureAPIUsage(UsageOptions{Record: func(record UsageRecord) error { logged = record; return nil }})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("stream"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("underlying Flusher lost: %v", err)
		}
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/gateway/pay", nil))
	if logged.StatusCode != http.StatusCreated || rec.Code != http.StatusCreated || !rec.Flushed || rec.Body.String() != "stream" {
		t.Fatalf("ResponseWriter behavior changed: log=%+v recorder=%+v", logged, rec)
	}
}

func TestUsageCaptureLogsMiddlewarePanicAndRepanics(t *testing.T) {
	var logged UsageRecord
	handler := CaptureAPIUsage(UsageOptions{Record: func(record UsageRecord) error { logged = record; return nil }})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("private middleware failure")
	}))
	func() {
		defer func() {
			if got := recover(); got != "private middleware failure" {
				t.Fatalf("original panic was changed: %#v", got)
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/gateway/pay", nil))
	}()
	if logged.StatusCode != http.StatusInternalServerError || logged.ErrorMessage != "Internal server error" {
		t.Fatalf("middleware panic did not produce a safe log: %+v", logged)
	}
}
