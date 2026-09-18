package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// fakeAcmeIssuer AcmeIssuer 测试桩：记录调用、可注入失败
type fakeAcmeIssuer struct {
	issueErr   error
	issues     int
	lastExpiry time.Time
}

func (f *fakeAcmeIssuer) Issue(cert *storage.AcmeCert) (*IssuedResult, error) {
	f.issues++
	if f.issueErr != nil {
		return nil, f.issueErr
	}
	expiry := time.Now().AddDate(0, 0, 90)
	f.lastExpiry = expiry
	return &IssuedResult{
		CertificatePEM: "-----CERT-----",
		IssuerPEM:      "-----ISSUER-----",
		PrivateKeyPEM:  "-----KEY-----",
		ExpiresAt:      expiry,
	}, nil
}

func newAcmeTestServer(t *testing.T) (*Server, *fakeAcmeIssuer) {
	t.Helper()
	s := newBackupTestServer(t)
	f := &fakeAcmeIssuer{}
	s.acme = f
	s.cfg = &config.Config{DNS: &config.DNSConfig{Cloudflare: &config.CloudflareDNSConfig{APIToken: "test-token"}}}
	return s, f
}

func mkAcmeCert(t *testing.T, s *Server, mutate func(*storage.AcmeCert)) *storage.AcmeCert {
	t.Helper()
	cert := &storage.AcmeCert{
		Domains:         []string{"home.example.com"},
		PrimaryDomain:   "home.example.com",
		CADirectory:     AcmeCADirectoryStaging,
		Status:          "pending",
		RenewBeforeDays: 30,
		AutoRenew:       true,
		LastStatus:      "never",
	}
	if mutate != nil {
		mutate(cert)
	}
	if err := s.db.CreateAcmeCert(cert); err != nil {
		t.Fatalf("create acme cert: %v", err)
	}
	return cert
}

func TestAcmeCertAPIValidation(t *testing.T) {
	s, _ := newAcmeTestServer(t)

	// domains 空
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs",
		strings.NewReader(`{"caDirectory":"staging","autoRenew":true}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty domains: code = %d", rec.Code)
	}

	// 坏域名
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs",
		strings.NewReader(`{"domains":["bad_domain!"],"caDirectory":"staging"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad domain: code = %d", rec.Code)
	}

	// 泛域名合法、大小写归一
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs",
		strings.NewReader(`{"domains":["*.Example.com"],"caDirectory":"staging","autoRenew":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("wildcard domain: code = %d body = %s", rec.Code, rec.Body.String())
	}

	// CA 白名单
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs",
		strings.NewReader(`{"domains":["a.example.com"],"caDirectory":"zerossl"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown CA: code = %d", rec.Code)
	}

	// renewBeforeDays 越界
	for _, bad := range []int{0, 6, 91} {
		rec = httptest.NewRecorder()
		body := `{"domains":["a.example.com"],"renewBeforeDays":` + strconv.Itoa(bad) + `}`
		s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("renewBeforeDays=%d: code = %d", bad, rec.Code)
		}
	}
}

func TestAcmeCertAPICRUDAndAudit(t *testing.T) {
	s, _ := newAcmeTestServer(t)

	// 创建：默认 CA = staging（D4 防误触生产限频）
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs",
		strings.NewReader(`{"domains":["home.example.com","*.example.com"],"autoRenew":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("create: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created acmeCertView
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == 0 || created.PrimaryDomain != "home.example.com" || created.CADirectory != "staging" || created.Status != "pending" {
		t.Fatalf("created = %+v", created)
	}

	// 列表：无 PEM 字段
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, "/acme/certs", nil))
	body := rec.Body.String()
	if strings.Contains(body, "PEM") || strings.Contains(body, "PRIVATE KEY") {
		t.Error("list response must not contain PEM fields")
	}

	// 更新：域名变更重置 pending
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/certs/"+strconv.Itoa(int(created.ID)),
		strings.NewReader(`{"domains":["other.example.com"],"caDirectory":"production","autoRenew":false,"renewBeforeDays":14}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: code=%d", rec.Code)
	}
	got, _ := s.db.GetAcmeCert(created.ID)
	if got.PrimaryDomain != "other.example.com" || got.CADirectory != "production" || got.AutoRenew || got.RenewBeforeDays != 14 {
		t.Errorf("updated = %+v", got)
	}

	// 删除
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodDelete, "/acme/certs/"+strconv.Itoa(int(created.ID)), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: code=%d", rec.Code)
	}
	list, _ := s.db.ListAcmeCerts()
	if len(list) != 0 {
		t.Errorf("after delete list = %d", len(list))
	}

	// 审计
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	found := map[string]bool{}
	for _, l := range logs {
		found[l.Action] = true
	}
	for _, action := range []string{"acme_create", "acme_update", "acme_delete"} {
		if !found[action] {
			t.Errorf("audit %s missing, logs = %v", action, found)
		}
	}
}

func TestAcmeIssueEndpoint(t *testing.T) {
	s, f := newAcmeTestServer(t)
	cert := mkAcmeCert(t, s, nil)

	// 签发成功 → 状态回写 + 观测表联动
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs/"+strconv.Itoa(int(cert.ID))+"/issue", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("issue: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if f.issues != 1 {
		t.Fatalf("issues = %d", f.issues)
	}
	got, _ := s.db.GetAcmeCert(cert.ID)
	if got.Status != "issued" || got.LastStatus != "ok" || got.ExpiresAt.IsZero() {
		t.Fatalf("after issue = %+v", got)
	}
	if got.CertificatePEM != "-----CERT-----" || got.PrivateKeyPEM != "-----KEY-----" {
		t.Errorf("PEM not persisted: %+v", got)
	}
	// 观测表联动（D6）
	obs, err := s.db.GetCertificate("acme-" + strconv.Itoa(int(cert.ID)))
	if err != nil || obs.DomainName != "home.example.com" || obs.Labels["source"] != "acme" || obs.Status != "valid" {
		t.Errorf("observation = %+v err = %v", obs, err)
	}
	// 响应无 PEM
	if strings.Contains(rec.Body.String(), "-----CERT-----") {
		t.Error("issue response must not contain PEM")
	}

	// token 未配置 → 503（D12）
	s.cfg.DNS.Cloudflare.APIToken = ""
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs/"+strconv.Itoa(int(cert.ID))+"/issue", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no token: code = %d, want 503", rec.Code)
	}
	s.cfg.DNS.Cloudflare.APIToken = "test-token"

	// 签发失败 → 400 + failed 状态 + 告警
	f.issueErr = errors.New("dns propagation timeout")
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs/"+strconv.Itoa(int(cert.ID))+"/issue", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("failed issue: code = %d", rec.Code)
	}
	got, _ = s.db.GetAcmeCert(cert.ID)
	if got.Status != "failed" || !strings.Contains(got.LastError, "propagation") {
		t.Errorf("after failed issue = %+v", got)
	}
	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 1 || !strings.Contains(alerts[0].Title, "home.example.com") {
		t.Fatalf("alerts = %+v", alerts)
	}

	// 重复失败 → 告警真去重（手动 issue 不节流，连续两次都执行）
	s.handleACME(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost,
		"/acme/certs/"+strconv.Itoa(int(cert.ID))+"/issue", nil))
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Errorf("alerts after re-issue = %d, want 1 (dedup)", len(alerts))
	}

	// 审计含 acme_issue
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	foundIssue := false
	for _, l := range logs {
		if l.Action == "acme_issue" {
			foundIssue = true
		}
	}
	if !foundIssue {
		t.Error("audit acme_issue missing")
	}
}

func TestAcmeDownload(t *testing.T) {
	s, _ := newAcmeTestServer(t)
	cert := mkAcmeCert(t, s, func(c *storage.AcmeCert) {
		c.Status = "issued"
		c.CertificatePEM = "CERT-PEM-BODY"
		c.IssuerPEM = "ISSUER-PEM-BODY"
		c.PrivateKeyPEM = "KEY-PEM-BODY"
	})
	idPath := "/acme/certs/" + strconv.Itoa(int(cert.ID)) + "/download"

	// cert / issuer 不记审计
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, idPath+"?part=cert", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "CERT-PEM-BODY" {
		t.Errorf("cert download: code=%d body=%s", rec.Code, rec.Body.String())
	}
	// key 记审计（D9）
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, idPath+"?part=key", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "KEY-PEM-BODY" {
		t.Errorf("key download: code=%d", rec.Code)
	}
	// 非法 part
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, idPath+"?part=nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad part: code = %d", rec.Code)
	}
	// 未签发 → 404
	pending := mkAcmeCert(t, s, nil)
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet,
		"/acme/certs/"+strconv.Itoa(int(pending.ID))+"/download", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("pending download: code = %d", rec.Code)
	}

	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	keyAudits := 0
	for _, l := range logs {
		if l.Action == "acme_download_key" {
			keyAudits++
		}
	}
	if keyAudits != 1 {
		t.Errorf("acme_download_key audits = %d, want 1 (cert download not audited)", keyAudits)
	}
}

func TestAcmeScanRenewal(t *testing.T) {
	s, f := newAcmeTestServer(t)

	// 未临期 → 跳过
	mkAcmeCert(t, s, func(c *storage.AcmeCert) {
		c.Status = "issued"
		c.ExpiresAt = time.Now().AddDate(0, 0, 90)
	})
	s.scanACMEOnce()
	if f.issues != 0 {
		t.Fatalf("far expiry should not renew, issues = %d", f.issues)
	}

	// 临期 issued → 续签
	cert := mkAcmeCert(t, s, func(c *storage.AcmeCert) {
		c.Status = "issued"
		c.ExpiresAt = time.Now().AddDate(0, 0, 10) // 10 天 < 30 天阈值
	})
	s.scanACMEOnce()
	if f.issues != 1 {
		t.Fatalf("near expiry should renew, issues = %d", f.issues)
	}
	got, _ := s.db.GetAcmeCert(cert.ID)
	if got.Status != "issued" || got.LastRenewAt == 0 {
		t.Errorf("after renew = %+v", got)
	}

	// failed → 1h 节流内跳过
	f.issueErr = errors.New("rate limited")
	failed := mkAcmeCert(t, s, func(c *storage.AcmeCert) {
		c.Status = "failed"
		c.LastRenewAt = time.Now().Unix() - 60 // 1 分钟前失败
	})
	s.scanACMEOnce()
	if f.issues != 1 {
		t.Errorf("throttled failed cert should be skipped, issues = %d", f.issues)
	}

	// 节流过期（LastRenewAt 推到 2h 前）→ 重试并转成功
	f.issueErr = nil
	failed.LastRenewAt = time.Now().Add(-2 * time.Hour).Unix()
	if err := s.db.UpdateAcmeCert(failed); err != nil {
		t.Fatal(err)
	}
	s.scanACMEOnce()
	if f.issues != 2 {
		t.Fatalf("throttle expired should retry, issues = %d", f.issues)
	}
	got, _ = s.db.GetAcmeCert(failed.ID)
	if got.Status != "issued" || got.LastError != "" {
		t.Errorf("after retry = %+v", got)
	}

	// pending + AutoRenew 不自动首签
	mkAcmeCert(t, s, nil)
	s.scanACMEOnce()
	if f.issues != 2 {
		t.Errorf("pending should not auto-issue, issues = %d", f.issues)
	}
}

func TestAcmeScanInterval(t *testing.T) {
	s, _ := newAcmeTestServer(t)
	if got := s.GetAcmeScanInterval(); got != 3600 {
		t.Errorf("default interval = %d, want 3600", got)
	}
	for _, v := range []int{0, 300, 86400} {
		if err := s.SetAcmeScanInterval(v); err != nil {
			t.Errorf("Set(%d): %v", v, err)
		}
		if got := s.GetAcmeScanInterval(); got != v {
			t.Errorf("Set(%d) then Get = %d", v, got)
		}
	}
	for _, bad := range []int{-1, 60, 90000} {
		if err := s.SetAcmeScanInterval(bad); err == nil {
			t.Errorf("Set(%d) should fail", bad)
		}
	}
}

func TestAcmeAccountAPI(t *testing.T) {
	s, _ := newAcmeTestServer(t)

	// 未注册视图
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, "/acme/account", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"registered":false`) {
		t.Errorf("empty account: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 设置 email
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/account",
		strings.NewReader(`{"email":"me@example.com"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("set email: code=%d", rec.Code)
	}
	acc, err := s.db.GetAcmeAccount()
	if err != nil || acc.Email != "me@example.com" {
		t.Fatalf("account = %+v err = %v", acc, err)
	}
	// 私钥绝不出响应
	if strings.Contains(rec.Body.String(), "PRIVATE KEY") {
		t.Error("account response must not contain private key")
	}

	// 坏 email 拒绝
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/account",
		strings.NewReader(`{"email":"not-an-email"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad email: code = %d", rec.Code)
	}
}

// TestPemLeafNotAfter pemLeafNotAfter 从自签证书提取 NotAfter
func TestPemLeafNotAfter(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 45),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	got, err := pemLeafNotAfter(certPEM)
	if err != nil {
		t.Fatalf("pemLeafNotAfter: %v", err)
	}
	if got.Unix() != tmpl.NotAfter.Unix() {
		t.Errorf("NotAfter = %v, want %v", got, tmpl.NotAfter)
	}

	// 坏输入
	if _, err := pemLeafNotAfter([]byte("not pem")); err == nil {
		t.Error("non-PEM input should fail")
	}
}
