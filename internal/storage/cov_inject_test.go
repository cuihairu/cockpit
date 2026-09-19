package storage

import (
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"io"
	"testing"

	"gorm.io/gorm"
)

// 底层原语注入点的收口测试：生产路径永不失败的防御分支经注入覆盖。

// failingCipherBlock 最小 cipher.Block 假实现（NewGCM 前即失败，方法不会被调用）。
type failingCipherBlock struct{}

func (failingCipherBlock) BlockSize() int          { return 0 }
func (failingCipherBlock) Encrypt(dst, src []byte) {}
func (failingCipherBlock) Decrypt(dst, src []byte) {}

// TestCovEncryptInjectErrors 覆盖 Encrypt 的 NewCipher / NewGCM /
// 随机数读取三个错误分支。
func TestCovEncryptInjectErrors(t *testing.T) {
	origCipher, origGCM, origRand := newCipher, newGCM, cryptoRand
	t.Cleanup(func() { newCipher, newGCM, cryptoRand = origCipher, origGCM, origRand })

	newCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("boom cipher") }
	if _, err := Encrypt("x"); err == nil || err.Error() != "boom cipher" {
		t.Errorf("Encrypt with failing cipher = %v", err)
	}

	newCipher = func([]byte) (cipher.Block, error) { return failingCipherBlock{}, nil }
	newGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("boom gcm") }
	if _, err := Encrypt("x"); err == nil || err.Error() != "boom gcm" {
		t.Errorf("Encrypt with failing gcm = %v", err)
	}

	newCipher, newGCM = origCipher, origGCM
	cryptoRand = failingReader{}
	if _, err := Encrypt("x"); err == nil {
		t.Error("Encrypt with failing rand should fail")
	}
}

// TestCovDecryptInjectErrors 覆盖 Decrypt 的 NewCipher / NewGCM 错误分支
// （base64 解码失败由 TestCovDecryptInvalidBase64 覆盖）。
// 输入须是合法 base64（解码先于解密），注入后才轮到 cipher 分支。
func TestCovDecryptInjectErrors(t *testing.T) {
	origCipher, origGCM := newCipher, newGCM
	t.Cleanup(func() { newCipher, newGCM = origCipher, origGCM })

	validB64 := base64.StdEncoding.EncodeToString([]byte("ciphertext"))

	newCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("boom cipher") }
	if _, err := Decrypt(validB64); err == nil || err.Error() != "boom cipher" {
		t.Errorf("Decrypt with failing cipher = %v", err)
	}

	newCipher = func([]byte) (cipher.Block, error) { return failingCipherBlock{}, nil }
	newGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("boom gcm") }
	if _, err := Decrypt(validB64); err == nil || err.Error() != "boom gcm" {
		t.Errorf("Decrypt with failing gcm = %v", err)
	}
}

// TestCovDecryptInvalidBase64 覆盖 Decrypt 的 base64 解码错误分支。
func TestCovDecryptInvalidBase64(t *testing.T) {
	if _, err := Decrypt("!!!not-base64!!!"); err == nil {
		t.Error("Decrypt(invalid base64) should fail")
	}
}

// TestCovGenerateBackupCodesRandFail 覆盖备份码生成中的随机数读取错误分支。
func TestCovGenerateBackupCodesRandFail(t *testing.T) {
	orig := cryptoRand
	t.Cleanup(func() { cryptoRand = orig })

	cryptoRand = failingReader{}
	if _, err := GenerateBackupCodes(); err == nil {
		t.Error("GenerateBackupCodes with failing rand should fail")
	}
}

// TestCovGenerateAgentSecretRandFail 覆盖 Agent 密钥生成中的随机数错误分支。
func TestCovGenerateAgentSecretRandFail(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })

	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }
	if _, err := GenerateAgentSecret(); err == nil {
		t.Error("GenerateAgentSecret with failing rand should fail")
	}
}

// TestCovRegenerateAgentSecretErrors 覆盖 RegenerateAgentSecret 的
// 随机数失败与哈希失败两个错误分支。
func TestCovRegenerateAgentSecretErrors(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.UpsertAgent(&Agent{ID: "a1", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}

	origRand, origHash := randRead, hashAgentSecret
	t.Cleanup(func() { randRead, hashAgentSecret = origRand, origHash })

	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }
	if _, err := db.RegenerateAgentSecret("a1"); err == nil {
		t.Error("RegenerateAgentSecret with failing rand should fail")
	}

	randRead = origRand
	hashAgentSecret = func(string) (string, error) { return "", errors.New("boom hash") }
	if _, err := db.RegenerateAgentSecret("a1"); err == nil {
		t.Error("RegenerateAgentSecret with failing hash should fail")
	}
}

// TestCovEnableTOTPAndRegenerateBackupCodesMarshalFail 覆盖 EnableTOTP 与
// RegenerateBackupCodes 的序列化错误分支。
func TestCovEnableTOTPAndRegenerateBackupCodesMarshalFail(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.CreateUser(&User{ID: "u1", Username: "u", Role: "admin"}); err != nil {
		t.Fatal(err)
	}

	orig := jsonMarshal
	t.Cleanup(func() { jsonMarshal = orig })
	jsonMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("boom json") }

	if err := db.EnableTOTP("u1", "enc", []string{"b1"}); err == nil {
		t.Error("EnableTOTP with failing marshal should fail")
	}
	if err := db.RegenerateBackupCodes("u1", []string{"b1"}); err == nil {
		t.Error("RegenerateBackupCodes with failing marshal should fail")
	}
}

// TestCovGetAuditLogsFindError 注入 Find 失败覆盖分页查询错误分支。
func TestCovGetAuditLogsFindError(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	orig := auditLogFind
	t.Cleanup(func() { auditLogFind = orig })
	auditLogFind = func(*gorm.DB, interface{}) error { return errors.New("boom find") }

	if _, _, err := db.GetAuditLogs(0, 10, nil); err == nil {
		t.Error("GetAuditLogs with failing find should fail")
	}
}

// TestCovListAcmeCertsClosedDB closed db 上 Find 报错，覆盖 ListAcmeCerts 错误分支。
func TestCovListAcmeCertsClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()

	if _, err := db.ListAcmeCerts(); err == nil {
		t.Error("ListAcmeCerts() on closed DB should fail")
	}
}

// failingReader 恒返回错误的 io.Reader。
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrNoProgress }
