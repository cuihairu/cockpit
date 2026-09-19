package sync

// cov_round1_test.go 覆盖率：Manager.Validate 的空 inventory 短路与
// regions/zones/agents 缺 ID 的回填分支。
// loadInventory 的 IO 错误族与 watchLoop 事件竞争由既有测试覆盖。

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCovManagerValidateEmpty 空结构合法：Validate 直接通过
func TestCovManagerValidateEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inv.yaml")
	if err := os.WriteFile(path, []byte("version: v1\ndomains: {}\nregions: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	if err := m.Validate(); err != nil {
		t.Fatalf("empty inventory should be valid: %v", err)
	}
}

// TestCovManagerValidateFillsIDs regions/zones/agents 全缺 ID → 回填为
// map key；缺 ID 字段本身合法
func TestCovManagerValidateFillsIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inv.yaml")
	yaml := `version: v1
regions:
  region1:
    zones:
      zone1:
        agents:
          agent1:
            hostname: test
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	if err := m.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
