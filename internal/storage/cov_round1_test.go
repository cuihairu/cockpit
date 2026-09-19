package storage

// cov_round1_test.go 覆盖率补零：acme 表的缺账户错误、PrimaryDomain
// 回填与缺失读取；Decrypt 的 base64 非法、密文过短、篡改失败。
// NewCipher/NewGCM 错误（密钥恒为 sha256 派生 32 字节）与
// GenerateBackupCodes 的随机数失败不可达，不在目标内。

import (
	"encoding/base64"
	"errors"
	"testing"
)

// TestCovAcmeDBMissingAndDefaults 缺账户错误、PrimaryDomain 回填、
// 读取不存在的证书
func TestCovAcmeDBMissingAndDefaults(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 无账户时改 email → ErrAcmeAccountNotFound
	if err := db.UpdateAcmeAccountEmail("x@example.com"); !errors.Is(err, ErrAcmeAccountNotFound) {
		t.Fatalf("update email without account: %v", err)
	}

	// CreateAcmeCert：PrimaryDomain 缺省取 Domains[0]
	cert := &AcmeCert{Domains: []string{"a.example.com", "b.example.com"}, CADirectory: "staging"}
	if err := db.CreateAcmeCert(cert); err != nil {
		t.Fatal(err)
	}
	if cert.PrimaryDomain != "a.example.com" {
		t.Fatalf("primary domain fill: %q", cert.PrimaryDomain)
	}

	// UpdateAcmeCert：同样回填（覆盖保存场景）
	cert.Domains = []string{"c.example.com"}
	cert.PrimaryDomain = ""
	if err := db.UpdateAcmeCert(cert); err != nil {
		t.Fatal(err)
	}

	// 读取不存在的证书
	if _, err := db.GetAcmeCert(999999); err == nil {
		t.Fatal("expect error for missing cert")
	}
}

// TestCovDecryptFailurePaths Decrypt：base64 非法、密文过短、篡改后
// 认证失败
func TestCovDecryptFailurePaths(t *testing.T) {
	if _, err := Decrypt("!!!not-base64!!!"); err == nil {
		t.Fatal("expect base64 error")
	}
	if _, err := Decrypt(base64.StdEncoding.EncodeToString([]byte("ab"))); err == nil {
		t.Fatal("expect too-short error")
	}

	sealed, err := Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF // 翻转密文尾字节破坏 GCM 认证
	if _, err := Decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("expect tamper error")
	}
}
