package routes

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"sandbox-pay/internal/db"
)

func TestOperationsBackupExportAuditAndSettings(t *testing.T) {
	h := newHarness(t, nil)
	fakeConfig := filepath.Join(t.TempDir(), "config.env")
	if err := os.WriteFile(fakeConfig, []byte("EXAMPLE_ONLY=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.cfg["CONFIG_FILE"] = fakeConfig
	settings := map[string]any{"usageRetentionDays": 90, "auditRetentionDays": 180, "attemptRetentionDays": 90, "backupIntervalHours": 24, "backupKeepCount": 2}
	response := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/operations/settings", Headers: h.auth, Body: settings})
	if response.Status != 200 {
		t.Fatalf("settings %d: %s", response.Status, response.Raw)
	}
	var settingsReply struct {
		Data db.OperationSettings `json:"data"`
	}
	if err := json.Unmarshal([]byte(response.Raw), &settingsReply); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339Nano, settingsReply.Data.UpdatedAt); err != nil {
		t.Fatalf("settings time lacks UTC timezone: %q", settingsReply.Data.UpdatedAt)
	}
	bad := map[string]any{"usageRetentionDays": 0, "auditRetentionDays": 180, "attemptRetentionDays": 90, "backupIntervalHours": 24, "backupKeepCount": 2}
	if r := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/operations/settings", Headers: h.auth, Body: bad}); r.Status != 400 {
		t.Fatalf("invalid settings accepted: %d", r.Status)
	}
	paid := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/gateway/pay", Headers: h.groupAuth, Body: map[string]any{"amount": 1, "productName": "=dangerous-formula", "payMethod": "native", "externalOrderNo": "ops_export_order"}})
	if paid.Status != 200 {
		t.Fatal(paid.Raw)
	}
	for i := 0; i < 3; i++ {
		r := h.do(t, reqSpec{Method: "POST", Path: "/v1/api/admin/operations/backups", Headers: h.auth})
		if r.Status != 200 {
			t.Fatalf("backup: %d %s", r.Status, r.Raw)
		}
	}
	backups, err := h.c.Operations.Backups()
	if err != nil || len(backups) != 2 {
		t.Fatalf("backup retention: %d %v", len(backups), err)
	}
	if !backups[0].ConfigIncluded {
		t.Fatal("configuration was not copied")
	}
	path, err := h.c.Operations.BackupPath(backups[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var integrity string
	if err = conn.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("backup integrity %s %v", integrity, err)
	}
	var count int
	if err = conn.QueryRow(`SELECT COUNT(*) FROM payment_orders WHERE externalOrderNo='ops_export_order'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("backup lost order: %d %v", count, err)
	}
	req := httptest.NewRequest("GET", "/v1/api/admin/operations/exports/orders.csv", nil)
	req.Header.Set("Authorization", h.auth["Authorization"])
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, req)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "'=dangerous-formula") || !strings.Contains(recorder.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export not safe or unavailable: %d %s", recorder.Code, recorder.Body.String())
	}
	req = httptest.NewRequest("GET", "/v1/api/admin/operations/backups/"+backups[0].Name, nil)
	req.Header.Set("Authorization", h.auth["Authorization"])
	recorder = httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, req)
	if recorder.Code != 200 || !strings.HasPrefix(recorder.Body.String(), "SQLite format 3") {
		t.Fatalf("backup download invalid: %d", recorder.Code)
	}
	if _, err = h.c.Operations.BackupPath("../config.env"); err == nil {
		t.Fatal("backup traversal accepted")
	}
	audit, total, err := h.store.AuditLogs("", 100, 0)
	if err != nil || total < 5 {
		t.Fatalf("audit missing: %d %v", total, err)
	}
	seen := false
	for _, item := range audit {
		if _, err := time.Parse(time.RFC3339Nano, item.CreatedAt); err != nil {
			t.Fatalf("audit time lacks UTC timezone: %q", item.CreatedAt)
		}
		if item.Action == "修改运行设置" && item.StatusCode == 200 {
			seen = true
		}
		if item.Username != testAdmin {
			t.Fatalf("audit actor missing: %q", item.Username)
		}
	}
	if !seen {
		t.Fatal("settings action not audited")
	}
	text, _ := json.Marshal(audit)
	if strings.Contains(string(text), h.auth["Authorization"]) || strings.Contains(string(text), fakeConfig) {
		t.Fatal("audit contains credential/config data")
	}
}

func TestOperationsSettingsRejectUnknownOrTrailingJSON(t *testing.T) {
	h := newHarness(t, nil)
	valid := `{"usageRetentionDays":90,"auditRetentionDays":180,"attemptRetentionDays":90,"backupIntervalHours":24,"backupKeepCount":7}`
	for _, body := range []string{
		strings.TrimSuffix(valid, "}") + `,"ignoredSetting":true}`,
		strings.TrimSuffix(valid, "}") + `,"updatedAt":"2026-10-31T00:00:00Z"}`,
		strings.Replace(valid, `"backupIntervalHours":24,`, "", 1),
		strings.Replace(valid, `"backupIntervalHours":24`, `"backupIntervalHours":null`, 1),
		valid + ` {}`,
		valid + ` garbage`,
		valid + strings.Repeat(" ", 4096),
	} {
		req := httptest.NewRequest("POST", "/v1/api/admin/operations/settings", strings.NewReader(body))
		req.Header.Set("Authorization", h.auth["Authorization"])
		recorder := httptest.NewRecorder()
		h.handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid settings accepted: %d %s", recorder.Code, recorder.Body.String())
		}
	}
}

type failingCSVResponse struct {
	*httptest.ResponseRecorder
	failOn int
	writes int
	err    error
}

func (w *failingCSVResponse) Write(body []byte) (int, error) {
	w.writes++
	if w.writes == w.failOn {
		return 0, w.err
	}
	return w.ResponseRecorder.Write(body)
}

func TestCSVExportsReportBufferedAndBOMWriteFailures(t *testing.T) {
	h := newHarness(t, nil)
	app := &App{c: h.c}
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request) error{"orders": app.exportOrders(), "usage": app.exportUsage()} {
		for _, failOn := range []int{1, 2} {
			t.Run(name+string(rune('0'+failOn)), func(t *testing.T) {
				expected := errors.New("connection closed")
				writer := &failingCSVResponse{ResponseRecorder: httptest.NewRecorder(), failOn: failOn, err: expected}
				request := httptest.NewRequest("GET", "/export.csv", nil)
				if err := handler(writer, request); !errors.Is(err, expected) {
					t.Fatalf("CSV write failure was ignored: %v", err)
				}
			})
		}
	}
}

func TestAuditMetadataRedactsCredentialsAndBoundsText(t *testing.T) {
	h := newHarness(t, nil)
	requestID := strings.TrimPrefix(h.auth["Authorization"], "Bearer ")
	req := httptest.NewRequest("POST", "/v1/api/admin/operations/settings?secret=query-only", strings.NewReader(`{}`))
	req.Header.Set("Authorization", h.auth["Authorization"])
	req.Header.Set("X-Correlation-ID", requestID)
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, req)
	items, _, err := h.store.AuditLogs("", 10, 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("audit missing: %+v err=%v", items, err)
	}
	if items[0].RequestID != "[REDACTED]" || strings.Contains(items[0].Path, "query-only") {
		t.Fatal("audit recorded bearer credential or query string")
	}
	redact := auditRedactor(req)
	value := redact("whsec_sensitive\n"+strings.Repeat("界", 300), 128)
	if strings.Contains(value, "whsec_sensitive") || strings.Contains(value, "\n") || utf8.RuneCountInString(value) > 128 || !utf8.ValidString(value) {
		t.Fatalf("audit metadata was not sanitized: %q", value)
	}
}

func TestRetentionPreservesOrdersAndRecentLogs(t *testing.T) {
	h := newHarness(t, nil)
	old := time.Now().AddDate(0, 0, -400)
	recent := time.Now()
	for _, stamp := range []time.Time{old, recent} {
		if err := h.store.UsageLogs.Insert(&db.UsageLog{RequestID: "retention-test", Method: "GET", Path: "/gateway/query", StatusCode: 200, CreatedAt: stamp}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := h.store.CleanOperationLogs(recent)
	if err != nil || result.Usage != 1 {
		t.Fatalf("cleanup %+v %v", result, err)
	}
	logs, total, _, err := h.store.UsageLogs.Search(db.UsageSearchOpts{Limit: 20})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("recent log was lost: %d %v", total, err)
	}
}

func TestDeveloperDocumentExampleAndDatabaseProbe(t *testing.T) {
	h := newHarness(t, nil)
	response := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/developer", Headers: h.auth})
	if response.Status != 200 {
		t.Fatal(response.Raw)
	}
	var spec map[string]any
	if err := json.Unmarshal(openAPIDocument, &spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/gateway/pay", "/gateway/query", "/admin/sandbox/scenarios", "/admin/notifications/{id}/detail", "/admin/operations/backups", "/admin/team/users"} {
		if paths[path] == nil {
			t.Fatalf("OpenAPI route missing: %s", path)
		}
	}
	req := httptest.NewRequest("GET", "/v1/api/developer/example", nil)
	r := httptest.NewRecorder()
	h.handler.ServeHTTP(r, req)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "timingSafeEqual") || !strings.Contains(r.Body.String(), "rename(stateFile") {
		t.Fatal("runnable callback example missing")
	}
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("GET", "/health", nil)
	r = httptest.NewRecorder()
	h.handler.ServeHTTP(r, req)
	if r.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed database health: %d", r.Code)
	}
}
