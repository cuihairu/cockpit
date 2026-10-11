package storage

import (
	"strings"
	"testing"
)

// cov_remote_credential_vault_test.go 远控凭据 × vault core 迁移
// （vault-design.md V6 第一波/V8）：新写入走 v1 前缀、v0 存量密文零迁移可解。

func TestRemoteCredentialVaultV1Writes(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	in := &RemoteCredentialInput{
		UserID: "u1", AgentID: "a1", Host: "h1", Port: 22, Protocol: "ssh",
		Username: "root", Password: "brand-new-pw",
	}
	if err := db.UpsertRemoteCredential(in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var row RemoteCredential
	if err := db.Gorm().First(&row, "user_id = ?", "u1").Error; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.HasPrefix(row.PasswordEnc, "v1:") {
		t.Fatalf("new writes must be v1-prefixed, got %q", row.PasswordEnc[:6])
	}
	got, err := db.GetRemoteCredential("u1", "a1", "h1", "ssh", 22)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Password != "brand-new-pw" || got.Username != "root" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestRemoteCredentialV0CiphertextCompat(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// v0 存量格式（storage.Encrypt 无前缀，历史 remote_credentials 行）：
	// 手工落库后 vault core 解密——零迁移依据
	legacyEnc, err := Encrypt("legacy-password")
	if err != nil {
		t.Fatalf("legacy Encrypt: %v", err)
	}
	if strings.HasPrefix(legacyEnc, "v1:") {
		t.Fatal("storage.Encrypt must stay v0 (legacy auth-domain rows rely on it)")
	}
	row := &RemoteCredential{
		ID: "legacy-id", UserID: "u0", AgentID: "a0", Host: "h0", Port: 22,
		Protocol: "ssh", Username: "root", PasswordEnc: legacyEnc,
	}
	if err := db.Gorm().Create(row).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	got, err := db.GetRemoteCredential("u0", "a0", "h0", "ssh", 22)
	if err != nil {
		t.Fatalf("Get on v0 row: %v", err)
	}
	if got.Password != "legacy-password" {
		t.Fatalf("v0 decrypt mismatch: %q", got.Password)
	}
}

func TestRemoteCredentialVaultDecryptFailPropagates(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	orig := vaultDecrypt
	vaultDecrypt = func(string) (string, error) { return "", errVaultInjected }
	t.Cleanup(func() { vaultDecrypt = orig })

	row := &RemoteCredential{
		ID: "bad-id", UserID: "u9", AgentID: "a9", Host: "h9", Port: 22,
		Protocol: "ssh", PasswordEnc: "ciphertext",
	}
	if err := db.Gorm().Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.GetRemoteCredential("u9", "a9", "h9", "ssh", 22); err == nil {
		t.Fatal("decrypt failure must propagate")
	}
}

type vaultInjectedError struct{}

func (vaultInjectedError) Error() string { return "injected vault decrypt failure" }

var errVaultInjected = vaultInjectedError{}
