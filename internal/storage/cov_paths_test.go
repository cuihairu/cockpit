package storage

import (
	"errors"
	"strings"
	"testing"
)

// Open / DDNS / ACME 错误路径的收口测试（覆盖率补齐）。

func TestCovOpenDirectoryPathFails(t *testing.T) {
	// 目标路径是目录 → sqlite 无法打开为数据库文件
	if _, err := Open(Config{Path: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "open database") {
		t.Errorf("Open(directory) error = %v", err)
	}
}

func TestCovGetDDNSConfigMissing(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if _, err := db.GetDDNSConfig(9999); err == nil {
		t.Error("GetDDNSConfig(9999) should fail for missing id")
	}
}

func TestCovListDDNSConfigsClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()

	if _, err := db.ListDDNSConfigs(); err == nil {
		t.Error("ListDDNSConfigs() on closed DB should fail")
	}
}

// TestCovGetAcmeAccountClosedDB closed db 的错误不是 ErrRecordNotFound，
// 应绕过 not-found 归一原样上抛
func TestCovGetAcmeAccountClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()

	_, err := db.GetAcmeAccount()
	if err == nil || errors.Is(err, ErrAcmeAccountNotFound) {
		t.Errorf("GetAcmeAccount() on closed DB = %v, want raw error", err)
	}
}
