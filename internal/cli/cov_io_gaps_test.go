package cli

// cov_io_gaps_test.go 覆盖 StatusCmd/SyncCmd 的 I/O 错误分支。手法：
// 经底层 sqlite 驱动对已迁移的库做定点破坏——给 agents 挂 BEFORE INSERT
// 触发器（RAISE ABORT）使写入确定性失败（AutoMigrate 只补缺失的表、
// 不动触发器，库仍可正常打开）→ Sync 的 "sync had errors"；向指定表
// 插入一行"毒数据"（serializer:json 列写成非法 JSON）→ gorm Find 扫描
// 该行报错 → ListAgents/ListDomains/ListCertificates 的错误返回。

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"

	_ "gorm.io/driver/sqlite" // 注册底层 sqlite3 驱动，供测试直接执行 DDL/DML
)

// covMigratedDB 建一个已完成迁移的库并返回其路径
func covMigratedDB(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cov.db")
	db, err := storage.Open(storage.Config{Path: p})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return p
}

// covRawExec 在库上直接执行一条 SQL（DDL/DML）
func covRawExec(t *testing.T, dbPath, q string) {
	t.Helper()
	c, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer c.Close()
	if _, err := c.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// covPoisonRow 向表插入一行"毒数据"：serializer:json 列写成非法 JSON，
// gorm Find 扫描该行时反序列化确定性报错（时间/数值列驱动会静默容错，
// 只有 JSON 列必然失败）
func covPoisonRow(t *testing.T, dbPath, insert string) {
	t.Helper()
	covRawExec(t, dbPath, insert)
}

func TestCovStatusCmdListErrors(t *testing.T) {
	for _, tc := range []struct{ name, insert, want string }{
		{"agents fail first",
			"INSERT INTO agents (id, capabilities) VALUES ('cov-poison', 'zz-not-json')",
			"list agents"},
		{"domains fail after agents ok",
			"INSERT INTO domains (id, domain, tags) VALUES ('cov-poison', 'cov.example', 'zz-not-json')",
			"list domains"},
		{"certificates fail last",
			"INSERT INTO certificates (id, domain_name, tags) VALUES ('cov-poison', 'cov.example', 'zz-not-json')",
			"list certificates"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := covMigratedDB(t)
			covPoisonRow(t, p, tc.insert)
			err := (&StatusCmd{DBPath: p}).Run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Run err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestCovSyncCmdWriteError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cov.db")
	db, err := storage.Open(storage.Config{Path: p})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	// agents 表写入确定性失败 → result.Errors > 0 → "sync had errors"
	covRawExec(t, p,
		"CREATE TRIGGER cov_abort_insert BEFORE INSERT ON agents "+
			"BEGIN SELECT RAISE(ABORT, 'cov: insert blocked'); END;")

	invPath := filepath.Join(dir, "inventory.yaml")
	body := `version: v1
regions:
  r1:
    name: R
    zones:
      z1:
        name: Z
        agents:
          a1:
            hostname: cov-host
`
	if err := os.WriteFile(invPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	err = (&SyncCmd{DBPath: p, Inventory: invPath}).Run()
	if err == nil || !strings.Contains(err.Error(), "sync had errors") {
		t.Errorf("Run err = %v, want sync had errors", err)
	}
}

func TestCovSyncCmdParseInventoryError(t *testing.T) {
	cmd := &SyncCmd{
		DBPath:    filepath.Join(t.TempDir(), "cov.db"),
		Inventory: filepath.Join(t.TempDir(), "missing.yaml"),
	}
	if err := cmd.Run(); err == nil || !strings.Contains(err.Error(), "parse inventory") {
		t.Errorf("Run err = %v, want parse inventory", err)
	}
}
