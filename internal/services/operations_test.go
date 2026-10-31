package services

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

func TestOperationScheduleRetriesBackupBeforeRetention(t *testing.T) {
	store := newTestStore(t)
	root := filepath.Join(t.TempDir(), "backups")
	if err := os.WriteFile(root, []byte("blocks backup directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewOperationsService(store, config.Map{
		"BACKUP_DIR":  root,
		"CONFIG_FILE": filepath.Join(t.TempDir(), "missing.env"),
	})
	now := time.Now().UTC()
	if err := store.UsageLogs.Insert(&db.UsageLog{RequestID: "before-retention", Method: "GET", Path: "/gateway/query", StatusCode: 200, CreatedAt: now.AddDate(0, 0, -400)}); err != nil {
		t.Fatal(err)
	}
	for _, tick := range []time.Time{now, now.Add(time.Minute)} {
		if err := service.RunSchedule(tick); err == nil {
			t.Fatal("backup failure was not reported")
		}
		_, total, _, err := store.UsageLogs.Search(db.UsageSearchOpts{Limit: 10})
		if err != nil || total != 1 {
			t.Fatalf("logs cleaned before backup succeeded: count=%d err=%v", total, err)
		}
		runs, err := store.OperationRuns()
		if err != nil || len(runs) != 1 || runs[0].Name != "backup" || runs[0].LastError == "" {
			t.Fatalf("backup failure state: %+v err=%v", runs, err)
		}
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSchedule(now.Add(2 * time.Minute)); err != nil {
		t.Fatalf("backup did not recover on next tick: %v", err)
	}
	items, err := service.Backups()
	if err != nil || len(items) != 1 {
		t.Fatalf("recovered backups: %+v err=%v", items, err)
	}
	backup, err := sql.Open("sqlite", filepath.Join(root, items[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var savedLogs int
	if err := backup.QueryRow(`SELECT COUNT(*) FROM api_usage_logs`).Scan(&savedLogs); err != nil || savedLogs != 1 {
		t.Fatalf("backup did not preserve logs before cleanup: count=%d err=%v", savedLogs, err)
	}
	_, total, _, err := store.UsageLogs.Search(db.UsageSearchOpts{Limit: 10})
	if err != nil || total != 0 {
		t.Fatalf("retention did not resume after backup: count=%d err=%v", total, err)
	}
	runs, err := store.OperationRuns()
	if err != nil || len(runs) != 2 {
		t.Fatalf("recovered operation state: %+v err=%v", runs, err)
	}
	for _, run := range runs {
		if run.LastError != "" {
			t.Fatalf("operation failure was not cleared: %+v", run)
		}
		if _, err := time.Parse(time.RFC3339Nano, run.LastRunAt); err != nil {
			t.Fatalf("operation time lacks UTC timezone: %q", run.LastRunAt)
		}
	}
	if err := service.RunSchedule(now.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, err = service.Backups()
	if err != nil || len(items) != 1 {
		t.Fatalf("successful backup was repeated before interval elapsed: %+v err=%v", items, err)
	}
}
