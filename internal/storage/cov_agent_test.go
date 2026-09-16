package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// ============ Agent Secret ============

func TestCovUpdateAgentSecret(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertAgent(&Agent{ID: "agent-sec", Hostname: "h"})

	if err := db.UpdateAgentSecret("agent-sec", "hash-value"); err != nil {
		t.Fatalf("UpdateAgentSecret() error = %v", err)
	}
	agent, err := db.GetAgent("agent-sec")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.SecretHash != "hash-value" {
		t.Errorf("SecretHash = %s, want hash-value", agent.SecretHash)
	}
}

func TestCovRegenerateAgentSecret(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertAgent(&Agent{ID: "agent-regen", Hostname: "h"})

	secret, err := db.RegenerateAgentSecret("agent-regen")
	if err != nil {
		t.Fatalf("RegenerateAgentSecret() error = %v", err)
	}
	if len(secret) != 64 { // 32 字节 hex
		t.Errorf("secret length = %d, want 64", len(secret))
	}
	if !VerifyAgentSecret(covAgentHash(t, secret), secret) {
		t.Error("regenerated secret should verify against stored hash")
	}

	// 数据库不可写 -> UpdateAgentSecret 报错
	db2 := testDB(t)
	db2.Close()
	if _, err := db2.RegenerateAgentSecret("agent-x"); err == nil {
		t.Error("RegenerateAgentSecret() on closed DB should fail")
	}
}

func covAgentHash(t *testing.T, secret string) string {
	t.Helper()
	hash, err := HashAgentSecret(secret)
	if err != nil {
		t.Fatalf("HashAgentSecret() error = %v", err)
	}
	return hash
}

// ============ Agent 清理 / 指标 / 过滤 ============

func TestCovCleanupOfflineAgentsQueryError(t *testing.T) {
	db := testDB(t)
	db.Close()

	if _, err := db.CleanupOfflineAgents(time.Hour); err == nil {
		t.Error("CleanupOfflineAgents() on closed DB should fail")
	}
}

func TestCovGetSystemMetricsOffset(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := db.SaveSystemMetric(&SystemMetric{
			AgentID:   "agent-offset",
			Timestamp: now.Add(time.Duration(-i) * time.Hour),
			CPUUsage:  float64(10 * i),
		}); err != nil {
			t.Fatalf("SaveSystemMetric() error = %v", err)
		}
	}

	list, err := db.GetSystemMetrics("agent-offset", 2, 1)
	if err != nil {
		t.Fatalf("GetSystemMetrics() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("length = %d, want 2", len(list))
	}
	if list[0].CPUUsage != 10 {
		t.Errorf("first CPUUsage = %v, want 10", list[0].CPUUsage)
	}
}

func TestCovListComputeInstancesZoneFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertAgent(&Agent{ID: "agent-z", Hostname: "host"})
	for _, inst := range []*ComputeInstance{
		{ID: "iz-1", Name: "one", AgentID: "agent-z", Type: "vm", Zone: "z1"},
		{ID: "iz-2", Name: "two", AgentID: "agent-z", Type: "vm", Zone: "z2"},
	} {
		if err := db.UpsertComputeInstance(inst); err != nil {
			t.Fatalf("UpsertComputeInstance() error = %v", err)
		}
	}

	list, err := db.ListComputeInstances(&ComputeInstanceFilter{Zone: "z1"})
	if err != nil {
		t.Fatalf("ListComputeInstances(zone) error = %v", err)
	}
	if len(list) != 1 || list[0].ID != "iz-1" {
		t.Errorf("zone filter returned %v", list)
	}
}

// ============ 状态更新 ============

func TestCovUpdateDomainStatus(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertDomain(&Domain{ID: "dz-1", Domain: "z.example.com"})
	if err := db.UpdateDomainStatus("dz-1", "error"); err != nil {
		t.Fatalf("UpdateDomainStatus() error = %v", err)
	}
	d, err := db.GetDomain("dz-1")
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if d.Status != "error" {
		t.Errorf("Status = %s, want error", d.Status)
	}
}

func TestCovUpdateCertificateStatus(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	expiry := time.Now().UTC().Add(90 * 24 * time.Hour)
	db.UpsertCertificate(&Certificate{ID: "cz-1", DomainName: "c.example.com", ExpiresAt: expiry})

	// expiresAt 非零：一并更新过期时间
	if err := db.UpdateCertificateStatus("cz-1", "valid", expiry); err != nil {
		t.Fatalf("UpdateCertificateStatus() error = %v", err)
	}
	cert, err := db.GetCertificate("cz-1")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if cert.Status != "valid" {
		t.Errorf("Status = %s, want valid", cert.Status)
	}

	// expiresAt 为零：只更新状态
	if err := db.UpdateCertificateStatus("cz-1", "expired", time.Time{}); err != nil {
		t.Fatalf("UpdateCertificateStatus(zero) error = %v", err)
	}
	cert, err = db.GetCertificate("cz-1")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if cert.Status != "expired" {
		t.Errorf("Status = %s, want expired", cert.Status)
	}
}

func TestCovUpdateServiceStatus(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertService(&Service{ID: "sz-1", Name: "svc", Type: "http"})
	lastCheck := time.Now().UTC()
	if err := db.UpdateServiceStatus("sz-1", "down", 250, lastCheck); err != nil {
		t.Fatalf("UpdateServiceStatus() error = %v", err)
	}
	svc, err := db.GetService("sz-1")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if svc.Status != "down" {
		t.Errorf("Status = %s, want down", svc.Status)
	}
}

// ============ Open / Close 边界 ============

func TestCovOpenDefaultPath(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open(default path) error = %v", err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()
	if _, err := os.Stat("cockpit.db"); err != nil {
		t.Errorf("default db file missing: %v", err)
	}
}

func TestCovOpenMkdirFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(parent, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(Config{Path: filepath.Join(parent, "sub", "test.db")})
	if err == nil {
		t.Fatal("Open() should fail when parent is a file")
	}
	if !strings.Contains(err.Error(), "create directory") {
		t.Errorf("error = %v, want create directory failure", err)
	}
}

func TestCovOpenDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(Config{Path: dir})
	if err == nil {
		t.Fatal("Open() on a directory should fail")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %v, want open database failure", err)
	}
}

func TestCovOpenMigrateFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate.db")
	db, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	// 把 users 表换成同名视图，令下一次 AutoMigrate 建表冲突
	if err := db.db.Exec("DROP TABLE users").Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := db.db.Exec("CREATE VIEW users AS SELECT 'x' AS id").Error; err != nil {
		t.Fatalf("create view: %v", err)
	}
	db.Close()

	if _, err := Open(Config{Path: path}); err == nil {
		t.Fatal("Open() should fail when migration hits a conflicting view")
	} else if !strings.Contains(err.Error(), "migrate") {
		t.Errorf("error = %v, want migrate failure", err)
	}
}

func TestCovCloseNilPool(t *testing.T) {
	// 白盒构造：gorm.DB 没有连接池时 DB() 报错，Close 应透传
	d := &DB{db: &gorm.DB{Config: &gorm.Config{}}}
	if err := d.Close(); err == nil {
		t.Error("Close() should fail when underlying pool is empty")
	}
}

// ============ VacuumInto ============

func TestCovVacuumInto(t *testing.T) {
	dir := t.TempDir()
	db := testDB(t)
	defer db.Close()

	out := filepath.Join(dir, "backup-copy.db")
	if err := db.VacuumInto(out); err != nil {
		t.Fatalf("VacuumInto() error = %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("backup file missing: %v", err)
	}
	// 目标文件已存在时应报错
	if err := db.VacuumInto(out); err == nil {
		t.Error("VacuumInto() should fail when target exists")
	}
}
