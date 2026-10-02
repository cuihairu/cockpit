package storage

import (
	"errors"
	"testing"
)

func TestUpsertAndGetRemoteCredential(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	in := &RemoteCredentialInput{
		UserID: "u1", AgentID: "agent-1", Host: "192.168.5.188", Port: 22, Protocol: "ssh",
		Username: "cui", Password: "secret-pass", PrivateKey: "-----BEGIN KEY-----",
	}
	if err := db.UpsertRemoteCredential(in); err != nil {
		t.Fatalf("UpsertRemoteCredential: %v", err)
	}

	got, err := db.GetRemoteCredential("u1", "agent-1", "192.168.5.188", "ssh", 22)
	if err != nil {
		t.Fatalf("GetRemoteCredential: %v", err)
	}
	if got.Username != "cui" || got.Password != "secret-pass" || got.PrivateKey != "-----BEGIN KEY-----" {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// 同目标再次保存 → 更新而非新增
	in.Password = "new-pass"
	if err := db.UpsertRemoteCredential(in); err != nil {
		t.Fatalf("second UpsertRemoteCredential: %v", err)
	}
	got, err = db.GetRemoteCredential("u1", "agent-1", "192.168.5.188", "ssh", 22)
	if err != nil {
		t.Fatalf("GetRemoteCredential after upsert: %v", err)
	}
	if got.Password != "new-pass" {
		t.Errorf("password = %q, want updated value", got.Password)
	}
}

func TestRemoteCredentialEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	in := &RemoteCredentialInput{
		UserID: "u1", AgentID: "agent-1", Host: "h", Port: 3389, Protocol: "rdp",
		Username: "admin", Password: "plaintext-pass",
	}
	if err := db.UpsertRemoteCredential(in); err != nil {
		t.Fatalf("UpsertRemoteCredential: %v", err)
	}

	// 直接查原始行：落库必须是密文
	var row RemoteCredential
	if err := db.db.First(&row).Error; err != nil {
		t.Fatalf("raw query: %v", err)
	}
	if row.PasswordEnc == "" || row.PasswordEnc == "plaintext-pass" {
		t.Errorf("password stored as %q, want ciphertext", row.PasswordEnc)
	}
	if row.PasswordEnc == "plaintext-pass" {
		t.Error("ciphertext equals plaintext")
	}

	// 解密回原值
	got, err := db.GetRemoteCredential("u1", "agent-1", "h", "rdp", 3389)
	if err != nil {
		t.Fatalf("GetRemoteCredential: %v", err)
	}
	if got.Password != "plaintext-pass" {
		t.Errorf("decrypt = %q, want plaintext-pass", got.Password)
	}
}

func TestRemoteCredentialEmptyFieldsStoredEmpty(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 只存用户名（VNC 场景只有密码、SSH 场景可能只有私钥——空字段存空串）
	if err := db.UpsertRemoteCredential(&RemoteCredentialInput{
		UserID: "u1", AgentID: "a", Host: "h", Port: 5900, Protocol: "vnc",
		Username: "vnc-user",
	}); err != nil {
		t.Fatalf("UpsertRemoteCredential: %v", err)
	}
	got, err := db.GetRemoteCredential("u1", "a", "h", "vnc", 5900)
	if err != nil {
		t.Fatalf("GetRemoteCredential: %v", err)
	}
	if got.Password != "" || got.PrivateKey != "" {
		t.Errorf("empty fields should stay empty, got %+v", got)
	}
}

func TestListRemoteCredentialsMetaOnly(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	for _, in := range []*RemoteCredentialInput{
		{UserID: "u1", AgentID: "a1", Host: "h1", Port: 22, Protocol: "ssh", Username: "root", Password: "p1"},
		{UserID: "u1", AgentID: "a1", Host: "h2", Port: 5900, Protocol: "vnc", Username: "v", PrivateKey: "k"},
		{UserID: "u2", AgentID: "a1", Host: "h3", Port: 22, Protocol: "ssh", Username: "other"},
	} {
		if err := db.UpsertRemoteCredential(in); err != nil {
			t.Fatalf("UpsertRemoteCredential(%s): %v", in.Host, err)
		}
	}

	list, err := db.ListRemoteCredentials("u1")
	if err != nil {
		t.Fatalf("ListRemoteCredentials: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2 (per-user isolation)", len(list))
	}
	for _, m := range list {
		if m.HasPassword && m.HasPrivKey {
			t.Errorf("%s: both flags set, expected either-or", m.Host)
		}
	}
	// 更新时间倒序：h2 后插入应排在前
	if list[0].Host != "h2" {
		t.Errorf("list[0].Host = %s, want h2 (most recent first)", list[0].Host)
	}
}

func TestDeleteRemoteCredentialScopedToUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.UpsertRemoteCredential(&RemoteCredentialInput{
		UserID: "u1", AgentID: "a", Host: "h", Port: 22, Protocol: "ssh", Username: "root",
	}); err != nil {
		t.Fatalf("UpsertRemoteCredential: %v", err)
	}
	var row RemoteCredential
	if err := db.db.First(&row).Error; err != nil {
		t.Fatalf("raw query: %v", err)
	}

	// 别的用户删不动
	deleted, err := db.DeleteRemoteCredential("u2", row.ID)
	if err != nil || deleted {
		t.Errorf("cross-user delete = %v, %v; want false, nil", deleted, err)
	}

	deleted, err = db.DeleteRemoteCredential("u1", row.ID)
	if err != nil || !deleted {
		t.Fatalf("owner delete = %v, %v; want true, nil", deleted, err)
	}

	// 再删一次 → 不存在
	deleted, err = db.DeleteRemoteCredential("u1", row.ID)
	if err != nil || deleted {
		t.Errorf("second delete = %v, %v; want false, nil", deleted, err)
	}

	if _, err := db.GetRemoteCredential("u1", "a", "h", "ssh", 22); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestGetRemoteCredentialNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if _, err := db.GetRemoteCredential("u1", "a", "h", "ssh", 22); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
