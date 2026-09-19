package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cuihairu/cockpit/internal/inventory"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Manager.Consistency / Validate / applyInventory 错误分支的收口测试
// （drift-design.md M6 D28 比对薄封装 + watcher 覆盖率补齐）。

func writeConsistencyInventory(t *testing.T, path string) {
	t.Helper()
	content := "version: v1\nregions:\n  home:\n    zones:\n      rack:\n        agents:\n          agent1:\n            hostname: nas-home\n            ip: 10.0.0.2\n          ghost:\n            hostname: gone\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManagerConsistency(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeConsistencyInventory(t, invPath)

	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertAgent(&storage.Agent{ID: "agent1", Hostname: "NAS-Home", IP: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAgent(&storage.Agent{ID: "stray", Hostname: "stray", IP: "10.0.0.9"}); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(invPath, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)

	report, err := m.Consistency()
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 3 || report.Summary.OK != 1 ||
		report.Summary.Unregistered != 1 || report.Summary.Undeclared != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
}

func TestManagerConsistencyReadError(t *testing.T) {
	m, err := NewManager(filepath.Join(t.TempDir(), "missing.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if _, err := m.Consistency(); err == nil {
		t.Fatal("missing inventory should error")
	}
}

func TestManagerConsistencyDBError(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeConsistencyInventory(t, invPath)

	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(invPath, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	db.Close() // ListAgents 报错路径

	if _, err := m.Consistency(); err == nil {
		t.Fatal("closed db should error")
	}
}

func TestManagerValidateFillsIDs(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	// 全部省略显式 id：Validate 应按 map 键回填（不报错、不改动语义）
	content := "version: v1\nregions:\n  home:\n    zones:\n      rack:\n        agents:\n          a1:\n            hostname: h1\n"
	if err := os.WriteFile(invPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(invPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}

	inv, err := m.watcher.GetInventory()
	if err != nil {
		t.Fatal(err)
	}
	region := inv.Regions["home"]
	if region.ID != "home" {
		t.Errorf("region.ID = %q", region.ID)
	}
	zone := region.Zones["rack"]
	if zone.ID != "rack" {
		t.Errorf("zone.ID = %q", zone.ID)
	}
	if agent := zone.Agents["a1"]; agent.ID != "a1" {
		t.Errorf("agent.ID = %q", agent.ID)
	}
}

func TestManagerValidateEmpty(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	if err := os.WriteFile(invPath, []byte("version: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(invPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if err := m.Validate(); err != nil {
		t.Fatalf("empty inventory should be valid, got %v", err)
	}
}

// applyInventory 的 no-op 分支：db/inv 为 nil 直接跳过。Sync 的 error
// 返回分支实际不可达——syncer 对单条落库失败只计数（result.Errors++）
// 不上抛，此处验证 no-op 契约即可。
func TestApplyInventoryBranches(t *testing.T) {
	w := &Watcher{}
	if err := w.applyInventory(&inventory.Inventory{}); err != nil {
		t.Errorf("nil db should no-op, got %v", err)
	}
	if err := w.applyInventory(nil); err != nil {
		t.Errorf("nil inv should no-op, got %v", err)
	}
}

// loadInventory 中 applyInventory 失败只记日志不上抛：ForceReload 成功
// 返回且刷新 lastModTime（行为契约：apply 异常不阻塞 watcher）。
func TestLoadInventoryApplyErrorIsLogged(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeConsistencyInventory(t, invPath)

	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(invPath, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	db.Close()

	if err := m.Reload(); err != nil {
		t.Fatalf("apply error must be logged not returned, got %v", err)
	}
	if m.watcher.GetLastModTime().IsZero() {
		t.Error("lastModTime should be refreshed even when apply fails")
	}
}
