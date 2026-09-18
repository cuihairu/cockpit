package storage

import (
	"errors"
	"testing"
	"time"
)

func TestAcmeAccountSingleRow(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 未注册 → NotFound
	if _, err := db.GetAcmeAccount(); !errors.Is(err, ErrAcmeAccountNotFound) {
		t.Fatalf("empty account err = %v, want ErrAcmeAccountNotFound", err)
	}

	acc := &AcmeAccount{Email: "me@example.com", PrivateKeyPEM: "-----BEGIN KEY-----", CADirectory: "staging"}
	if err := db.SaveAcmeAccount(acc); err != nil {
		t.Fatalf("SaveAcmeAccount: %v", err)
	}
	if acc.ID != 1 {
		t.Fatalf("account ID = %d, want fixed 1", acc.ID)
	}

	// 覆盖保存（email 变更）
	acc.Email = "new@example.com"
	if err := db.SaveAcmeAccount(acc); err != nil {
		t.Fatalf("resave: %v", err)
	}
	got, err := db.GetAcmeAccount()
	if err != nil {
		t.Fatalf("GetAcmeAccount: %v", err)
	}
	if got.Email != "new@example.com" || got.PrivateKeyPEM != "-----BEGIN KEY-----" || got.CADirectory != "staging" {
		t.Errorf("account = %+v", got)
	}

	// 只改 email
	if err := db.UpdateAcmeAccountEmail("third@example.com"); err != nil {
		t.Fatalf("UpdateAcmeAccountEmail: %v", err)
	}
	got, _ = db.GetAcmeAccount()
	if got.Email != "third@example.com" {
		t.Errorf("email = %s", got.Email)
	}

	// 删除后回到 NotFound
	if err := db.DeleteAcmeAccount(); err != nil {
		t.Fatalf("DeleteAcmeAccount: %v", err)
	}
	if _, err := db.GetAcmeAccount(); !errors.Is(err, ErrAcmeAccountNotFound) {
		t.Errorf("after delete err = %v", err)
	}
}

func TestAcmeCertCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cert := &AcmeCert{
		Domains:         []string{"home.example.com", "*.example.com"},
		CADirectory:     "staging",
		Status:          "pending",
		RenewBeforeDays: 30,
		AutoRenew:       true,
		LastStatus:      "never",
	}
	if err := db.CreateAcmeCert(cert); err != nil {
		t.Fatalf("CreateAcmeCert: %v", err)
	}
	if cert.ID == 0 {
		t.Fatal("CreateAcmeCert should set ID")
	}
	// PrimaryDomain 自动取 domains[0]
	if cert.PrimaryDomain != "home.example.com" {
		t.Errorf("PrimaryDomain = %s", cert.PrimaryDomain)
	}

	got, err := db.GetAcmeCert(cert.ID)
	if err != nil {
		t.Fatalf("GetAcmeCert: %v", err)
	}
	if len(got.Domains) != 2 || got.Domains[1] != "*.example.com" {
		t.Errorf("domains round-trip = %v", got.Domains)
	}

	// 签发成功回写（PEM 持久化——重启后证书仍在，D5）
	expires := time.Now().AddDate(0, 0, 90).UTC().Truncate(time.Second)
	got.Status = "issued"
	got.CertificatePEM = "-----BEGIN CERTIFICATE-----"
	got.IssuerPEM = "-----BEGIN CERT-----issuer"
	got.PrivateKeyPEM = "-----BEGIN PRIVATE KEY-----"
	got.ExpiresAt = expires
	got.LastRenewAt = 1726500000
	got.LastStatus = "ok"
	if err := db.UpdateAcmeCert(got); err != nil {
		t.Fatalf("UpdateAcmeCert: %v", err)
	}
	re, _ := db.GetAcmeCert(cert.ID)
	if re.Status != "issued" || re.CertificatePEM == "" || re.PrivateKeyPEM == "" || re.IssuerPEM == "" {
		t.Errorf("PEM fields must persist, got %+v", re)
	}
	if !re.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", re.ExpiresAt, expires)
	}

	// 列表与删除
	list, err := db.ListAcmeCerts()
	if err != nil {
		t.Fatalf("ListAcmeCerts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}
	if err := db.DeleteAcmeCert(cert.ID); err != nil {
		t.Fatalf("DeleteAcmeCert: %v", err)
	}
	if list, _ = db.ListAcmeCerts(); len(list) != 0 {
		t.Errorf("after delete list = %d", len(list))
	}
}
