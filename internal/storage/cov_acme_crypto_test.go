package storage

import (
	"errors"
	"strings"
	"testing"
)

const acmeTestAccountPEM = "-----BEGIN EC PRIVATE KEY--\nacct-key-9q\n-----END EC PRIVATE KEY--"
const acmeTestCertPEM = "-----BEGIN EC PRIVATE KEY--\ncert-key-XYZ\n-----END EC PRIVATE KEY--"

// 私钥列落库加密（vault-design.md V6 第二波②）：写走 v1 密文、读回原明文、
// 公开物料（证书/签发者 PEM）不加密。
func TestAcmePrivateKeyEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.SaveAcmeAccount(&AcmeAccount{Email: "me@example.com", PrivateKeyPEM: acmeTestAccountPEM}); err != nil {
		t.Fatalf("SaveAcmeAccount: %v", err)
	}
	var acctRow struct {
		PrivateKeyPEM string
	}
	if err := db.Gorm().Table("acme_accounts").Select("private_key_pem").Scan(&acctRow).Error; err != nil {
		t.Fatalf("raw account fetch: %v", err)
	}
	if !strings.HasPrefix(acctRow.PrivateKeyPEM, "v1:") {
		t.Fatalf("account key must be v1 ciphertext at rest, got %.8q", acctRow.PrivateKeyPEM)
	}
	if strings.Contains(acctRow.PrivateKeyPEM, "acct-key-9q") {
		t.Fatal("plaintext account key must not be at rest")
	}
	acc, err := db.GetAcmeAccount()
	if err != nil || acc.PrivateKeyPEM != acmeTestAccountPEM {
		t.Fatalf("read back = %q, %v", acc.PrivateKeyPEM, err)
	}

	cert := &AcmeCert{
		Domains: []string{"x.example.com"}, PrimaryDomain: "x.example.com",
		CertificatePEM: "-----BEGIN CERTIFICATE--\npublic-material\n-----END CERTIFICATE--",
		PrivateKeyPEM:  acmeTestCertPEM,
	}
	if err := db.CreateAcmeCert(cert); err != nil {
		t.Fatalf("CreateAcmeCert: %v", err)
	}
	var certRow struct {
		CertificatePEM string
		PrivateKeyPEM  string
	}
	if err := db.Gorm().Table("acme_certs").Select("certificate_pem, private_key_pem").Scan(&certRow).Error; err != nil {
		t.Fatalf("raw cert fetch: %v", err)
	}
	if !strings.HasPrefix(certRow.PrivateKeyPEM, "v1:") || strings.Contains(certRow.PrivateKeyPEM, "cert-key-XYZ") {
		t.Fatalf("cert key must be encrypted at rest, got %.8q", certRow.PrivateKeyPEM)
	}
	if !strings.HasPrefix(certRow.CertificatePEM, "-----BEGIN") {
		t.Fatal("certificate PEM is public material and must stay plaintext")
	}
	got, err := db.GetAcmeCert(cert.ID)
	if err != nil || got.PrivateKeyPEM != acmeTestCertPEM {
		t.Fatalf("cert read back = %q, %v", got.PrivateKeyPEM, err)
	}
	list, err := db.ListAcmeCerts()
	if err != nil || len(list) != 1 || list[0].PrivateKeyPEM != acmeTestCertPEM {
		t.Fatalf("list read back = %+v, %v", list, err)
	}
}

// v0 兼容：存量明文 PEM 行原样读出；再保存起转为密文。
func TestAcmePrivateKeyLegacyPlaintextCompat(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.Gorm().Exec(
		`INSERT INTO acme_certs (domains, primary_domain, private_key_pem) VALUES ('["y.example.com"]','y.example.com',?)`,
		acmeTestCertPEM).Error; err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	got, err := db.GetAcmeCert(1)
	if err != nil || got.PrivateKeyPEM != acmeTestCertPEM {
		t.Fatalf("legacy plaintext read = %q, %v", got.PrivateKeyPEM, err)
	}
	// 保存（含私钥）→ 落库转为密文
	got.Domains = []string{"y.example.com"}
	if err := db.UpdateAcmeCert(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	var row struct {
		PrivateKeyPEM string
	}
	if err := db.Gorm().Table("acme_certs").Select("private_key_pem").Scan(&row).Error; err != nil {
		t.Fatalf("raw fetch: %v", err)
	}
	if !strings.HasPrefix(row.PrivateKeyPEM, "v1:") {
		t.Fatalf("save must re-encrypt legacy plaintext, got %.8q", row.PrivateKeyPEM)
	}
}

// 列内容既非明文 PEM 也非 v1 密文 → 读取报错（不静默当明文）。
func TestAcmePrivateKeyCorruptColumn(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.Gorm().Exec(
		`INSERT INTO acme_certs (domains, primary_domain, private_key_pem) VALUES ('["z.example.com"]','z.example.com','garbage-not-pem')`).Error; err != nil {
		t.Fatalf("corrupt insert: %v", err)
	}
	if _, err := db.GetAcmeCert(1); err == nil || !strings.Contains(err.Error(), "neither PEM nor encrypted") {
		t.Fatalf("corrupt column err = %v", err)
	}
}

// 加密失败注入：写路径上抛、不落半截明文。
func TestAcmePrivateKeyEncryptFailureInjection(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	orig := vaultEncrypt
	vaultEncrypt = func(string) (string, error) { return "", errors.New("boom acme encrypt") }
	t.Cleanup(func() { vaultEncrypt = orig })

	if err := db.SaveAcmeAccount(&AcmeAccount{Email: "a@b.c", PrivateKeyPEM: acmeTestAccountPEM}); err == nil {
		t.Fatal("encrypt failure must propagate")
	}
	var row struct {
		PrivateKeyPEM string
	}
	if err := db.Gorm().Table("acme_accounts").Select("private_key_pem").Scan(&row).Error; err != nil {
		t.Fatalf("raw fetch: %v", err)
	}
	if strings.Contains(row.PrivateKeyPEM, "acct-key-9q") {
		t.Fatal("failed write must not leave plaintext")
	}
}

// 解密失败注入：读路径上抛（错误不带明文）。
func TestAcmePrivateKeyDecryptFailureInjection(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	orig := vaultDecrypt
	vaultDecrypt = func(string) (string, error) { return "", errors.New("boom acme decrypt") }
	t.Cleanup(func() { vaultDecrypt = orig })

	if err := db.SaveAcmeAccount(&AcmeAccount{Email: "a@b.c", PrivateKeyPEM: acmeTestAccountPEM}); err != nil {
		t.Fatalf("save with real encrypt: %v", err)
	}
	vaultDecrypt = func(c string) (string, error) {
		if strings.HasPrefix(c, "v1:") {
			return "", errors.New("boom acme decrypt")
		}
		return orig(c)
	}
	_, err := db.GetAcmeAccount()
	if err == nil || !strings.Contains(err.Error(), "boom acme decrypt") {
		t.Fatalf("decrypt failure must propagate, got %v", err)
	}
}
