package storage

import (
	"strings"
	"testing"
	"time"
)

// ============ 审计日志 ============

func TestCovGetAuditLogsQueryError(t *testing.T) {
	db := testDB(t)
	db.Close()

	if _, _, err := db.GetAuditLogs(0, 10, nil); err == nil {
		t.Error("GetAuditLogs() on closed DB should fail")
	}
}

// ============ 备份 ============

func TestCovGetBackupRunNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if _, err := db.GetBackupRun(9999); err == nil {
		t.Error("GetBackupRun() should fail for missing id")
	}
}

func TestCovDeleteBackupConfigClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()

	if err := db.DeleteBackupConfig(1); err == nil {
		t.Error("DeleteBackupConfig() on closed DB should fail")
	}
}

// TestCovDeleteBackupConfigWriteConflict 覆盖事务内删除失败分支：
// 另一连接持有 BEGIN IMMEDIATE 保留锁时，删除 BackupRun 报错。
func TestCovDeleteBackupConfigWriteConflict(t *testing.T) {
	path := t.TempDir() + "/backup-conflict.db"
	db1, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open(db1) error = %v", err)
	}
	defer db1.Close()
	db2, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open(db2) error = %v", err)
	}
	defer db2.Close()

	if err := db1.CreateBackupConfig(&BackupConfig{AgentID: "a", Name: "cfg", Schedule: "manual", Enabled: true, NextRunAt: 1}); err != nil {
		t.Fatalf("CreateBackupConfig() error = %v", err)
	}

	if err := db1.db.Exec("BEGIN IMMEDIATE").Error; err != nil {
		t.Fatalf("BEGIN IMMEDIATE error = %v", err)
	}
	defer db1.db.Exec("ROLLBACK")

	if err := db2.DeleteBackupConfig(1); err == nil {
		t.Error("DeleteBackupConfig() should fail while DB is locked")
	}
}

// ============ 拨测历史 ============

func TestCovListProbeResultsDefaultLimit(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := db.CreateProbeResults([]*ProbeResult{{
			ResourceType: "service",
			ResourceID:   "svc-cov",
			Name:         "svc",
			Status:       "up",
			CheckedAt:    now.Add(time.Duration(-i) * time.Minute),
		}}); err != nil {
			t.Fatalf("CreateProbeResults() error = %v", err)
		}
	}

	// limit<=0 回退为 50
	list, err := db.ListProbeResults("service", "svc-cov", 0)
	if err != nil {
		t.Fatalf("ListProbeResults(0) error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("length = %d, want 3", len(list))
	}
}

// ============ Stack 部署历史 ============

func TestCovListStackDeploymentsDefaultLimit(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	for i := 0; i < 3; i++ {
		if err := db.CreateStackDeployment(&StackDeployment{
			AgentID:   "agent-cov",
			StackName: "web",
			Action:    "deploy",
			Status:    "success",
			TaskID:    strings.Repeat("t", i+1),
			StartedAt: time.Now().Unix(),
		}); err != nil {
			t.Fatalf("CreateStackDeployment() error = %v", err)
		}
	}

	// limit<=0 与 limit>200 都回退为 50
	for _, limit := range []int{0, 201} {
		list, err := db.ListStackDeployments("agent-cov", "web", limit)
		if err != nil {
			t.Fatalf("ListStackDeployments(%d) error = %v", limit, err)
		}
		if len(list) != 3 {
			t.Errorf("limit=%d length = %d, want 3", limit, len(list))
		}
	}
}

// ============ 管理员初始化 ============

func TestCovInitAdminUserPasswordTooLong(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// bcrypt 拒绝超过 72 字节的密码
	err := db.InitAdminUser("cov-admin", strings.Repeat("a", 100))
	if err == nil {
		t.Fatal("InitAdminUser() should fail for >72-byte password")
	}
	if !strings.Contains(err.Error(), "72") {
		t.Errorf("error = %v, want bcrypt password-too-long", err)
	}
	// 用户不应被创建
	if _, err := db.GetUserByUsername("cov-admin"); err != ErrNotFound {
		t.Error("admin user should not exist after failed init")
	}
}
