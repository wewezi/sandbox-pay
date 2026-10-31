package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
)

type OperationsService struct {
	store *db.Store
	cfg   config.Config
	mutex sync.Mutex
}

func NewOperationsService(store *db.Store, cfg config.Config) *OperationsService {
	return &OperationsService{store: store, cfg: cfg}
}

type BackupInfo struct {
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	CreatedAt      string `json:"createdAt"`
	ConfigIncluded bool   `json:"configIncluded"`
}

var backupNamePattern = regexp.MustCompile(`^sandbox-pay-[0-9TZ.]+-[a-f0-9]{12}\.db$`)

func (s *OperationsService) backupDir() (string, error) {
	raw := strings.TrimSpace(s.cfg.Get("BACKUP_DIR"))
	if raw == "" {
		path := s.store.DatabasePath()
		if path == "" {
			return "", fmt.Errorf("当前数据库不支持文件备份")
		}
		raw = filepath.Join(filepath.Dir(path), "backups")
	}
	return filepath.Abs(raw)
}
func (s *OperationsService) Backups() ([]BackupInfo, error) {
	root, err := s.backupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]BackupInfo, 0)
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !backupNamePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		_, cfgErr := os.Stat(filepath.Join(root, entry.Name()+".env"))
		out = append(out, BackupInfo{Name: entry.Name(), Size: info.Size(), CreatedAt: JsTime(info.ModTime()), ConfigIncluded: cfgErr == nil})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}
func (s *OperationsService) Backup() (result *BackupInfo, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer func() {
		message := ""
		if err != nil {
			message = "备份失败，请检查目录权限和磁盘空间"
		}
		_ = s.store.RecordOperationRun("backup", message)
	}()
	root, err := s.backupDir()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	random := make([]byte, 6)
	if _, err = rand.Read(random); err != nil {
		return nil, err
	}
	name := "sandbox-pay-" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random) + ".db"
	path := filepath.Join(root, name)
	if err = s.store.Snapshot(path); err != nil {
		return nil, err
	}
	configFile := strings.TrimSpace(s.cfg.Get("CONFIG_FILE"))
	if configFile == "" {
		configFile = ".env"
	}
	if content, readErr := os.ReadFile(configFile); readErr == nil {
		if err = os.WriteFile(path+".env", content, 0o600); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(readErr) {
		return nil, readErr
	}
	settings, err := s.store.OperationSettings()
	if err != nil {
		return nil, err
	}
	items, err := s.Backups()
	if err != nil {
		return nil, err
	}
	for i := settings.BackupKeepCount; i < len(items); i++ {
		target := filepath.Join(root, items[i].Name)
		// Only generated files in this exact directory can be pruned.
		if filepath.Dir(target) != root || !backupNamePattern.MatchString(filepath.Base(target)) {
			return nil, fmt.Errorf("备份路径无效")
		}
		if err = os.Remove(target); err != nil {
			return nil, err
		}
		if removeErr := os.Remove(target + ".env"); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, removeErr
		}
	}
	for _, item := range items {
		if item.Name == name {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("备份文件未找到")
}
func (s *OperationsService) BackupPath(name string) (string, error) {
	if !backupNamePattern.MatchString(name) {
		return "", fmt.Errorf("备份名称无效")
	}
	root, err := s.backupDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("备份不是普通文件")
	}
	return path, nil
}
func (s *OperationsService) RunSchedule(now time.Time) error {
	settings, err := s.store.OperationSettings()
	if err != nil {
		return err
	}
	runs, err := s.store.OperationRuns()
	if err != nil {
		return err
	}
	last := map[string]time.Time{}
	failed := map[string]bool{}
	for _, run := range runs {
		last[run.Name] = ParseOperationTime(run.LastRunAt)
		failed[run.Name] = run.LastError != ""
	}
	// A failed attempt must not postpone the next backup or permit retention
	// cleanup before the pending backup has succeeded.
	if settings.BackupIntervalHours > 0 && (failed["backup"] || now.Sub(last["backup"]) >= time.Duration(settings.BackupIntervalHours)*time.Hour) {
		if _, err := s.Backup(); err != nil {
			return err
		}
	}
	if failed["cleanup"] || now.Sub(last["cleanup"]) >= 24*time.Hour {
		_, err := s.store.CleanOperationLogs(now)
		message := ""
		if err != nil {
			message = "日志清理失败"
		}
		_ = s.store.RecordOperationRun("cleanup", message)
		return err
	}
	return nil
}
func ParseOperationTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

type ChannelState struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Reachable  *bool  `json:"reachable,omitempty"`
	Message    string `json:"message"`
}

func (s *OperationsService) Channels(check bool) []ChannelState {
	out := []ChannelState{{ID: "native", Name: "本地模拟", Configured: true, Message: "可用"}, {ID: "alipay", Name: "支付宝沙箱", Configured: strings.TrimSpace(s.cfg.Get("ALIPAY_APP_ID")) != "" && strings.TrimSpace(s.cfg.Get("ALIPAY_PRIVATE_KEY")) != "" && strings.TrimSpace(s.cfg.Get("ALIPAY_PUBLIC_KEY")) != "", Message: "凭证未配置"}, {ID: "paypal", Name: "PayPal 沙箱", Configured: strings.TrimSpace(s.cfg.Get("PAYPAL_CLIENT_ID")) != "" && strings.TrimSpace(s.cfg.Get("PAYPAL_CLIENT_SECRET")) != "", Message: "凭证未配置"}}
	endpoints := map[string]string{"alipay": config.String(s.cfg, "ALIPAY_SERVER_URL"), "paypal": "https://api-m.sandbox.paypal.com"}
	var wait sync.WaitGroup
	for i := 1; i < len(out); i++ {
		if !out[i].Configured {
			continue
		}
		out[i].Message = "凭证已配置"
		if !check {
			continue
		}
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			client := httpclient.New()
			response, err := client.Get(endpoints[out[index].ID], 3*time.Second)
			reachable := err == nil && response != nil
			out[index].Reachable = &reachable
			if reachable {
				out[index].Message = fmt.Sprintf("网络可达（HTTP %d）；此检查不验证凭证", response.Status)
			} else {
				out[index].Message = "网络连接失败，请检查代理、DNS或网络"
			}
		}(i)
	}
	wait.Wait()
	return out
}

type OperationsStatus struct {
	Database      string                `json:"database"`
	SchemaVersion int                   `json:"schemaVersion"`
	Counts        map[string]int64      `json:"counts"`
	Runs          []db.OperationRun     `json:"runs"`
	Channels      []ChannelState        `json:"channels"`
	Settings      *db.OperationSettings `json:"settings"`
}

func (s *OperationsService) Status() (*OperationsStatus, error) {
	counts, version, err := s.store.OperationalCounts()
	if err != nil {
		return nil, err
	}
	settings, err := s.store.OperationSettings()
	if err != nil {
		return nil, err
	}
	runs, err := s.store.OperationRuns()
	if err != nil {
		return nil, err
	}
	return &OperationsStatus{Database: "ok", SchemaVersion: version, Counts: counts, Runs: runs, Channels: s.Channels(false), Settings: settings}, nil
}
