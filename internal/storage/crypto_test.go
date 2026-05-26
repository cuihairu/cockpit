package storage

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// 设置测试密钥 (32 字节用于 AES-256)
	originalKey := encryptionKey
	encryptionKey = []byte("exactly32byteslongtestencryptkey")
	defer func() { encryptionKey = originalKey }()

	plaintext := "secret-totp-key"

	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("Encrypted text should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted text mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestGenerateBackupCodes(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes failed: %v", err)
	}

	if len(codes) != 10 {
		t.Errorf("Got %d codes, want 10", len(codes))
	}

	for _, code := range codes {
		if len(code) != 14 { // xxxx-xxxx-xxxx
			t.Errorf("Code format wrong: %s", code)
		}
	}
}

func TestHashBackupCodes(t *testing.T) {
	codes := []string{"code1", "code2", "code3"}
	hashed, err := HashBackupCodes(codes)
	if err != nil {
		t.Fatalf("HashBackupCodes failed: %v", err)
	}
	if len(hashed) != 3 {
		t.Errorf("Got %d hashes, want 3", len(hashed))
	}
	for _, h := range hashed {
		if len(h) != 64 { // SHA256 hex length
			t.Errorf("Hash length wrong: %s", h)
		}
	}
}

func TestHashSingleBackupCode(t *testing.T) {
	code := "test-code-123"
	hash := HashSingleBackupCode(code)
	if len(hash) != 64 {
		t.Errorf("Hash length wrong: got %d, want 64", len(hash))
	}
	// 验证一致性
	hash2 := HashSingleBackupCode(code)
	if hash != hash2 {
		t.Error("Hash should be consistent")
	}
}

func TestDecryptErrors(t *testing.T) {
	// 测试无效的 base64
	_, err := Decrypt("invalid-base64!!!")
	if err == nil {
		t.Error("Should return error for invalid base64")
	}

	// 测试过短的数据
	short := "YWJj" // base64 of "abc"
	_, err = Decrypt(short)
	if err == nil {
		t.Error("Should return error for short ciphertext")
	}
}
