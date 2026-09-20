package cli

// cov_cli_test.go 覆盖 init/status/sync 的参数解析与错误分支。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/inventory"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ init ============

func TestCovInitMkdirError(t *testing.T) {
	// 父路径是普通文件：MkdirAll 失败
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	cmd := &InitCmd{Dir: filepath.Join(blocker, "sub")}
	if err := cmd.Run(); err == nil {
		t.Fatal("mkdir under a file should error")
	}
}

func TestCovInitWriteConfigError(t *testing.T) {
	// config 目标放在只读目录：建目录成功、写文件失败
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(roDir, 0755); err != nil {
		t.Fatalf("mkdir ro: %v", err)
	}
	if err := os.Chmod(roDir, 0555); err != nil {
		t.Fatalf("chmod ro: %v", err)
	}
	cmd := &InitCmd{Dir: dir, Config: filepath.Join(roDir, "cockpit.yaml")}
	err := cmd.Run()
	if err == nil || !strings.Contains(err.Error(), "write config file") {
		t.Fatalf("err = %v, want write config file error", err)
	}
}

func TestCovInitWriteExampleInventoryError(t *testing.T) {
	// inventory 目录只读：目录已存在（MkdirAll 通过），写示例文件失败
	dir := t.TempDir()
	invDir := filepath.Join(dir, "inventory")
	if err := os.MkdirAll(invDir, 0755); err != nil {
		t.Fatalf("mkdir inventory: %v", err)
	}
	if err := os.Chmod(invDir, 0555); err != nil {
		t.Fatalf("chmod inventory: %v", err)
	}
	cmd := &InitCmd{Dir: dir, Example: true}
	err := cmd.Run()
	if err == nil || !strings.Contains(err.Error(), "write example inventory") {
		t.Fatalf("err = %v, want write example inventory error", err)
	}
}

// ============ status ============

func TestCovStatusCountsPopulatedDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cockpit.db")
	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// up 状态服务：覆盖 up 计数
	if err := db.UpsertService(&storage.Service{ID: "svc-up", Name: "up-svc", Type: "http", Status: "up"}); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}
	// 30 天内到期证书：覆盖 expiring 计数
	if err := db.UpsertCertificate(&storage.Certificate{
		ID:         "cert-expiring",
		DomainName: "expiring.example.com",
		Status:     "valid",
		ExpiresAt:  time.Now().UTC().Add(10 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}
	// 已启用的代理：覆盖 enabled 计数
	if err := db.CreateProxy(&storage.Proxy{
		ID:         "px-1",
		Name:       "px",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 19000,
		Target:     "127.0.0.1:80",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("CreateProxy: %v", err)
	}

	cmd := &StatusCmd{DBPath: dbPath}
	if err := cmd.Run(); err != nil {
		t.Fatalf("StatusCmd.Run: %v", err)
	}
}

// ============ sync ============

func TestCovSyncDBPathFromConfig(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	minimalInventoryYAML(t, invPath)

	cfgPath := filepath.Join(dir, "cockpit.yaml")
	cfgYAML := "database:\n  path: " + filepath.Join(dir, "from-config.db") +
		"\ninventory:\n  path: " + invPath + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := &SyncCmd{Config: cfgPath, Inventory: invPath}
	if err := cmd.Run(); err != nil {
		t.Fatalf("SyncCmd.Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "from-config.db")); err != nil {
		t.Errorf("db from config not used: %v", err)
	}
}

func TestCovSyncOpenDBError(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	minimalInventoryYAML(t, invPath)

	// DBPath 指向目录：sqlite 打开失败
	cmd := &SyncCmd{Inventory: invPath, DBPath: dir}
	if err := cmd.Run(); err == nil {
		t.Fatal("db path pointing at a directory should error")
	}
}

func TestCovPrintResultVariants(t *testing.T) {
	printResult("nil", nil)                                                   // nil 直接返回
	printResult("created", &inventory.ResourceResult{Created: 2})             // +2
	printResult("deleted", &inventory.ResourceResult{Created: 1, Deleted: 1}) // +1 -1
	printResult("errors", &inventory.ResourceResult{Updated: 1, Errors: 2})   // ~1 (2 errors)
}
