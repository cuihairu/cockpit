package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// newServerBackupTestServer 带临时数据库目录的测试 server
func newServerBackupTestServer(t *testing.T) *Server {
	t.Helper()
	s := newBackupTestServer(t)
	s.cfg = &config.Config{
		Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "test.db")},
	}
	return s
}

// TestVacuumIntoProduct VACUUM INTO 产物是合法 SQLite 且包含业务表
func TestVacuumIntoProduct(t *testing.T) {
	s := newServerBackupTestServer(t)
	// 造一点数据
	if err := s.db.SetSetting("k1", "v1"); err != nil {
		t.Fatal(err)
	}
	name, err := s.runServerBackup()
	if err != nil {
		t.Fatal(err)
	}
	if !isValidServerBackupName(name) {
		t.Fatalf("name = %q", name)
	}
	path := filepath.Join(s.serverBackupDir(), name)
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("backup file: %v size=%d", err, info.Size())
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	// 产物用真实 GORM 打开能读到刚写的数据
	restored, err := storage.Open(storage.Config{Path: path})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer restored.Close()
	if v, err := restored.GetSetting("k1"); err != nil || v != "v1" {
		t.Fatalf("restored setting = %q err=%v, want v1", v, err)
	}
}

// TestServerBackupNameValidation 文件名严格模式：穿越与畸形名拒绝
func TestServerBackupNameValidation(t *testing.T) {
	for _, bad := range []string{
		"../cockpit.db", "cockpit-20260915-000000.db.bak", "evil.db",
		"cockpit-2026-010203.db", "", "cockpit-20260915-000000.sqlite",
	} {
		if isValidServerBackupName(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
	}
	if !isValidServerBackupName("cockpit-20260915-043000.db") {
		t.Fatal("valid name rejected")
	}
}

// TestServerBackupSettings 配置语义：默认/合法/越界回默认
func TestServerBackupSettings(t *testing.T) {
	s := newServerBackupTestServer(t)
	if got := s.serverBackupIntervalHours(); got != 24 {
		t.Fatalf("interval default = %d, want 24", got)
	}
	if got := s.serverBackupRetentionDays(); got != 7 {
		t.Fatalf("retention default = %d, want 7", got)
	}
	s.db.SetSetting(ServerBackupIntervalSettingKey, "0")
	if got := s.serverBackupIntervalHours(); got != 0 {
		t.Fatalf("interval 0 = %d, want 0 (disabled)", got)
	}
	s.db.SetSetting(ServerBackupIntervalSettingKey, "999")
	if got := s.serverBackupIntervalHours(); got != 24 {
		t.Fatalf("interval 999 = %d, want default", got)
	}
	s.db.SetSetting(ServerBackupRetentionSettingKey, "abc")
	if got := s.serverBackupRetentionDays(); got != 7 {
		t.Fatalf("retention abc = %d, want default", got)
	}
}

// TestServerBackupCleanup 保留天数清理：过期删、永久留
func TestServerBackupCleanup(t *testing.T) {
	s := newServerBackupTestServer(t)
	dir := s.serverBackupDir()
	os.MkdirAll(dir, 0700)
	old := filepath.Join(dir, "cockpit-20200101-000000.db")
	os.WriteFile(old, []byte("x"), 0600)
	os.Chtimes(old, time.Now().AddDate(0, 0, -10), time.Now().AddDate(0, 0, -10))
	fresh := filepath.Join(dir, "cockpit-20990101-000000.db")
	os.WriteFile(fresh, []byte("x"), 0600)

	s.cleanupServerBackups()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("expired backup should be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh backup should be kept")
	}

	// 0 = 永久
	os.WriteFile(old, []byte("x"), 0600)
	s.db.SetSetting(ServerBackupRetentionSettingKey, "0")
	s.cleanupServerBackups()
	if _, err := os.Stat(old); err != nil {
		t.Fatal("retention=0 should keep forever")
	}
}

// TestServerBackupsAPI 全路径：列表/配置/立即备份/下载/删除/穿越拒绝/审计
func TestServerBackupsAPI(t *testing.T) {
	s := newServerBackupTestServer(t)

	// 配置读取（默认）+ 写入
	r0 := httptest.NewRequest(http.MethodGet, "/server-backups/config", nil)
	w0 := httptest.NewRecorder()
	s.handleServerBackups(w0, r0)
	if w0.Code != http.StatusOK || !strings.Contains(w0.Body.String(), `"interval_hours":24`) {
		t.Fatalf("config GET = %d %s", w0.Code, w0.Body.String())
	}
	body := strings.NewReader(`{"interval_hours":12,"retention_days":3}`)
	r1 := httptest.NewRequest(http.MethodPut, "/server-backups/config", body)
	w1 := httptest.NewRecorder()
	s.handleServerBackups(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("config PUT = %d %s", w1.Code, w1.Body.String())
	}
	if got := s.serverBackupIntervalHours(); got != 12 {
		t.Fatalf("interval after PUT = %d, want 12", got)
	}
	// 越界 400
	body = strings.NewReader(`{"interval_hours":999}`)
	r1b := httptest.NewRequest(http.MethodPut, "/server-backups/config", body)
	w1b := httptest.NewRecorder()
	s.handleServerBackups(w1b, r1b)
	if w1b.Code != http.StatusBadRequest {
		t.Fatalf("config PUT out-of-range = %d, want 400", w1b.Code)
	}

	// 立即备份
	r2 := httptest.NewRequest(http.MethodPost, "/server-backups/run", nil)
	w2 := httptest.NewRecorder()
	s.handleServerBackups(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("run = %d %s", w2.Code, w2.Body.String())
	}
	var runResp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("run resp: %v", err)
	}
	name := runResp.Name

	// 列表
	r3 := httptest.NewRequest(http.MethodGet, "/server-backups", nil)
	w3 := httptest.NewRecorder()
	s.handleServerBackups(w3, r3)
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), name) {
		t.Fatalf("list = %d %s", w3.Code, w3.Body.String())
	}

	// 下载
	r4 := httptest.NewRequest(http.MethodGet, "/server-backups/"+name+"/download", nil)
	w4 := httptest.NewRecorder()
	s.handleServerBackups(w4, r4)
	if w4.Code != http.StatusOK || w4.Body.Len() == 0 {
		t.Fatalf("download = %d len=%d", w4.Code, w4.Body.Len())
	}

	// 穿越与畸形名 400/404
	r5 := httptest.NewRequest(http.MethodGet, "/server-backups/..%2Fcockpit.db/download", nil)
	w5 := httptest.NewRecorder()
	s.handleServerBackups(w5, r5)
	if w5.Code != http.StatusBadRequest && w5.Code != http.StatusNotFound {
		t.Fatalf("traversal = %d", w5.Code)
	}
	r5b := httptest.NewRequest(http.MethodDelete, "/server-backups/evil.db", nil)
	w5b := httptest.NewRecorder()
	s.handleServerBackups(w5b, r5b)
	if w5b.Code != http.StatusNotFound {
		t.Fatalf("invalid delete = %d, want 404", w5b.Code)
	}

	// 删除
	r6 := httptest.NewRequest(http.MethodDelete, "/server-backups/"+name, nil)
	w6 := httptest.NewRecorder()
	s.handleServerBackups(w6, r6)
	if w6.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", w6.Code, w6.Body.String())
	}

	// 审计：立即备份(create)/配置(update)/下载(export)/删除(delete)
	logs, _, err := s.db.GetAuditLogs(0, 30, map[string]interface{}{"resource": "server_backup"})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, l := range logs {
		actions[l.Action] = true
	}
	if !actions["create"] || !actions["update"] || !actions["export"] || !actions["delete"] {
		t.Fatalf("audit actions = %v", actions)
	}
}

// TestLatestServerBackupAt 间隔判断依据：目录最新文件名解析
func TestLatestServerBackupAt(t *testing.T) {
	s := newServerBackupTestServer(t)
	if !s.latestServerBackupAt().IsZero() {
		t.Fatal("empty dir should be zero")
	}
	dir := s.serverBackupDir()
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "cockpit-20250101-120000.db"), []byte("x"), 0600)
	at := s.latestServerBackupAt()
	if at.IsZero() || at.Year() != 2025 || at.Hour() != 12 {
		t.Fatalf("latest = %v", at)
	}
}
