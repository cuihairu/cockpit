package storage

// cov_remote_credential_gaps_test.go 补 remote_credential.go 的错误分支：
// ID 生成/加解密失败（randRead、cryptoRand 注入点，见 crypto.go 与
// password.go 注释）、closed DB 的查询/删除错误、密文损坏的解密错误；
// 另覆盖 agentUpdateFields 的 LocalIPs/Metadata JSON 序列化块。

import (
	"errors"
	"strings"
	"testing"
)

func TestCovRemoteCredentialIDRandFail(t *testing.T) {
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }
	t.Cleanup(func() { randRead = orig })

	if _, err := newRemoteCredentialID(); err == nil {
		t.Error("newRemoteCredentialID with failing rand should fail")
	}
}

func TestCovUpsertRemoteCredentialEncryptFail(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	orig := cryptoRand
	cryptoRand = failingReader{}
	t.Cleanup(func() { cryptoRand = orig })

	// 口令加密失败（私钥为空走不到第二个 encryptOptional）
	in := &RemoteCredentialInput{UserID: "u", AgentID: "a", Host: "h", Port: 1, Protocol: "ssh", Password: "p"}
	if err := db.UpsertRemoteCredential(in); err == nil {
		t.Error("UpsertRemoteCredential with failing rand (password) should fail")
	}

	// 口令为空跳过、私钥加密失败
	in2 := &RemoteCredentialInput{UserID: "u", AgentID: "a", Host: "h2", Port: 1, Protocol: "ssh", PrivateKey: "k"}
	if err := db.UpsertRemoteCredential(in2); err == nil {
		t.Error("UpsertRemoteCredential with failing rand (private key) should fail")
	}
}

func TestCovUpsertRemoteCredentialIDFail(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 口令/私钥皆空 → 不经加密；新目标 → 走 ID 生成即 randRead 注入失败
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }
	t.Cleanup(func() { randRead = orig })

	in := &RemoteCredentialInput{UserID: "u", AgentID: "a", Host: "h", Port: 1, Protocol: "ssh", Username: "n"}
	if err := db.UpsertRemoteCredential(in); err == nil {
		t.Error("UpsertRemoteCredential with failing ID rand should fail")
	}
}

func TestCovRemoteCredentialClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()

	in := &RemoteCredentialInput{UserID: "u", AgentID: "a", Host: "h", Port: 1, Protocol: "ssh", Password: "p"}
	if err := db.UpsertRemoteCredential(in); err == nil {
		t.Error("UpsertRemoteCredential on closed DB should fail")
	}
	if _, err := db.GetRemoteCredential("u", "a", "h", "ssh", 1); err == nil {
		t.Error("GetRemoteCredential on closed DB should fail")
	}
	if _, err := db.ListRemoteCredentials("u"); err == nil {
		t.Error("ListRemoteCredentials on closed DB should fail")
	}
	if _, err := db.DeleteRemoteCredential("u", "id"); err == nil {
		t.Error("DeleteRemoteCredential on closed DB should fail")
	}
}

func TestCovRemoteCredentialCorruptCiphertext(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	validEnc, err := Encrypt("real-pass")
	if err != nil {
		t.Fatal(err)
	}

	// 口令密文损坏（base64 解不开）
	if err := db.db.Create(&RemoteCredential{
		ID: "rc1", UserID: "u", AgentID: "a", Host: "h", Port: 1, Protocol: "ssh",
		Username: "n", PasswordEnc: "!!!not-base64!!!",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRemoteCredential("u", "a", "h", "ssh", 1); err == nil {
		t.Error("GetRemoteCredential with corrupt password ciphertext should fail")
	}

	// 口令密文完好、私钥密文损坏
	if err := db.db.Create(&RemoteCredential{
		ID: "rc2", UserID: "u2", AgentID: "a", Host: "h", Port: 1, Protocol: "ssh",
		Username: "n", PasswordEnc: validEnc, PrivateKeyEnc: "!!!not-base64!!!",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRemoteCredential("u2", "a", "h", "ssh", 1); err == nil {
		t.Error("GetRemoteCredential with corrupt private key ciphertext should fail")
	}
}

func TestCovAgentUpdateFieldsJSONColumns(t *testing.T) {
	u := agentUpdateFields(&Agent{
		LocalIPs: []string{"10.0.0.2", "10.0.0.3"},
		Metadata: map[string]interface{}{"bootMode": "uefi"},
	})
	ips, ok := u["local_ips"].(string)
	if !ok || !strings.Contains(ips, "10.0.0.3") {
		t.Errorf("local_ips = %v, want JSON array string", u["local_ips"])
	}
	meta, ok := u["metadata"].(string)
	if !ok || !strings.Contains(meta, "uefi") {
		t.Errorf("metadata = %v, want JSON object string", u["metadata"])
	}
}
