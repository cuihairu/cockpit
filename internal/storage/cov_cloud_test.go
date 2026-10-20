package storage

import (
	"errors"
	"testing"
)

// cloud.go 错误分支确定性覆盖：入参校验、randRead 注入点（ID 生成失败）、
// 闭库直调（First 非 NotFound / Delete res.Error 上抛）。注入点与闭库是本包
// 既有惯例（cov_inject_test.go / cov_paths_test.go）。

func stubRandReadError(t *testing.T) {
	t.Helper()
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }
}

func TestCovCloudAccountErrorBranches(t *testing.T) {
	db := testDB(t)

	// 空 vendor/alias：入参校验分支
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "", Alias: "x", CredentialRef: "${A}"}); err == nil {
		t.Error("empty vendor should be rejected")
	}
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "aliyun", Alias: "", CredentialRef: "${A}"}); err == nil {
		t.Error("empty alias should be rejected")
	}

	// 新建路径 ID 生成失败：randRead 注入点（First 走 NotFound 后）
	stubRandReadError(t)
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${A}"}); err == nil {
		t.Error("rand failure should propagate")
	}

	db.Close()

	// 闭库：First 返回非 NotFound 错误上抛
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${A}"}); err == nil {
		t.Error("closed-db upsert should fail")
	}
	// 闭库：Delete 的 res.Error 上抛（先于 RowsAffected 检查）
	if err := db.DeleteCloudAccount("whatever"); err == nil {
		t.Error("closed-db delete should fail")
	}
}

func TestCovCloudInstanceErrorBranches(t *testing.T) {
	db := testDB(t)

	// 空 account_id/native_id：入参校验分支
	if err := db.UpsertCloudInstance(&CloudInstanceInput{AccountID: "", NativeID: "i-1"}); err == nil {
		t.Error("empty account id should be rejected")
	}
	if err := db.UpsertCloudInstance(&CloudInstanceInput{AccountID: "a1", NativeID: ""}); err == nil {
		t.Error("empty native id should be rejected")
	}

	// 新建路径 ID 生成失败：先落一个账号使 First 走 NotFound，再注入 rand 失败
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${A}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, err := db.ListCloudAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	stubRandReadError(t)
	if err := db.UpsertCloudInstance(&CloudInstanceInput{AccountID: accounts[0].ID, NativeID: "i-1"}); err == nil {
		t.Error("rand failure should propagate")
	}

	db.Close()

	// 闭库：First 非 NotFound 错误上抛
	if err := db.UpsertCloudInstance(&CloudInstanceInput{AccountID: "a", NativeID: "n"}); err == nil {
		t.Error("closed-db upsert should fail")
	}
	// 闭库：Delete 的 res.Error 上抛
	if err := db.DeleteCloudInstance("whatever"); err == nil {
		t.Error("closed-db delete should fail")
	}
}
