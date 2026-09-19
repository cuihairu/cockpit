package server

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
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

// TestAcmeDNSConfigReady Ready 按 provider 分派判定（D13）：
// 三家各自校验自己的键，错误文案报缺哪个
func TestAcmeDNSConfigReady(t *testing.T) {
	cases := []struct {
		name    string
		cfg     AcmeDNSConfig
		wantOK  bool
		wantMsg string // 非空时断言文案包含
	}{
		{"cloudflare ready", AcmeDNSConfig{CloudflareToken: "tok"}, true, ""},
		{"cloudflare default provider", AcmeDNSConfig{Provider: "cloudflare", CloudflareToken: "tok"}, true, ""},
		{"cloudflare missing token", AcmeDNSConfig{}, false, "Cloudflare token not configured"},
		{"dnspod ready", AcmeDNSConfig{Provider: "dnspod", DNSPodToken: "id,tok"}, true, ""},
		{"dnspod missing token", AcmeDNSConfig{Provider: "dnspod"}, false, "DNSPod token not configured"},
		{"alidns ready", AcmeDNSConfig{Provider: "alidns", AliAccessKey: "ak", AliSecretKey: "sk"}, true, ""},
		{"alidns missing secret", AcmeDNSConfig{Provider: "alidns", AliAccessKey: "ak"}, false, "AliDNS credentials not configured"},
		{"unknown provider", AcmeDNSConfig{Provider: "route53"}, false, "unknown ACME DNS provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, msg := tc.cfg.Ready()
			if ok != tc.wantOK {
				t.Fatalf("Ready() ok = %v msg = %q", ok, msg)
			}
			if tc.wantMsg != "" && !strings.Contains(msg, tc.wantMsg) {
				t.Errorf("msg = %q, want contains %q", msg, tc.wantMsg)
			}
		})
	}
}

// TestDNSProviderFactory 工厂按 provider 构造 lego provider；
// 凭据缺失/未知 provider 各报各的错（构造不做网络请求）
func TestDNSProviderFactory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cfg       AcmeDNSConfig
		wantPType string
	}{
		{"cloudflare", AcmeDNSConfig{CloudflareToken: "tok"}, "*cloudflare.DNSProvider"},
		{"dnspod", AcmeDNSConfig{Provider: "dnspod", DNSPodToken: "id,tok"}, "*dnspod.DNSProvider"},
		{"alidns", AcmeDNSConfig{Provider: "alidns", AliAccessKey: "ak", AliSecretKey: "sk"}, "*alidns.DNSProvider"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := dnsProvider(tc.cfg)
			if err != nil {
				t.Fatalf("dnsProvider() error = %v", err)
			}
			if got := fmt.Sprintf("%T", p); got != tc.wantPType {
				t.Errorf("provider type = %s, want %s", got, tc.wantPType)
			}
		})
	}

	// 缺凭据 → Ready 的文案
	if _, err := dnsProvider(AcmeDNSConfig{Provider: "dnspod"}); err == nil || !strings.Contains(err.Error(), "DNSPod token not configured") {
		t.Errorf("dnspod missing token error = %v", err)
	}
	// 未知 provider
	if _, err := dnsProvider(AcmeDNSConfig{Provider: "route53", CloudflareToken: "tok"}); err == nil || !strings.Contains(err.Error(), "unknown ACME DNS provider") {
		t.Errorf("unknown provider error = %v", err)
	}
}

// TestAcmeConfigDNSField GET /acme/config 响应含 ACME 视角的 dns 字段；
// 非 cloudflare provider 未配凭据时 issue 503 文案按 provider 报缺失键（D13）
func TestAcmeConfigDNSField(t *testing.T) {
	s, _ := newAcmeTestServer(t)

	// 默认 provider（空 = cloudflare）+ token 已配 → configured
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, "/acme/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config: code = %d", rec.Code)
	}
	var body struct {
		DNS struct {
			Provider   string `json:"provider"`
			Configured bool   `json:"configured"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if body.DNS.Provider != "cloudflare" || !body.DNS.Configured {
		t.Errorf("dns field = %+v, want cloudflare/configured", body.DNS)
	}

	// 切 dnspod 未配凭据：config 报未就绪；issue 503 文案报 DNSPod（不再检查 cloudflare token）
	s.cfg.DNS.Provider = "dnspod"
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, "/acme/config", nil))
	body.DNS = struct {
		Provider   string `json:"provider"`
		Configured bool   `json:"configured"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if body.DNS.Provider != "dnspod" || body.DNS.Configured {
		t.Errorf("dns field = %+v, want dnspod/not configured", body.DNS)
	}

	cert := mkAcmeCert(t, s, nil)
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs/"+strconv.Itoa(int(cert.ID))+"/issue", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("issue under dnspod: code = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DNSPod token not configured") {
		t.Errorf("503 body = %s, want DNSPod hint", rec.Body.String())
	}
}

// TestAcmeDeployValidation 部署目标校验（D14）：三字段要么全空，
// 要么 agentID + 两绝对路径齐全
func TestAcmeDeployValidation(t *testing.T) {
	s, _ := newAcmeTestServer(t)
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"no deploy fields", `{"domains":["a.example.com"],"autoRenew":true}`, true},
		{"full target", `{"domains":["a.example.com"],"deployAgentId":"a1","deployCertPath":"/c.pem","deployKeyPath":"/k.pem"}`, true},
		{"missing agent", `{"domains":["a.example.com"],"deployCertPath":"/c.pem","deployKeyPath":"/k.pem"}`, false},
		{"relative path", `{"domains":["a.example.com"],"deployAgentId":"a1","deployCertPath":"c.pem","deployKeyPath":"/k.pem"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleACME(rec, httptest.NewRequest(http.MethodPost, "/acme/certs", strings.NewReader(tc.body)))
			if tc.ok && rec.Code != http.StatusOK {
				t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
			}
			if !tc.ok && rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, want 400", rec.Code)
			}
		})
	}
}

// TestAcmeDeployAPI 手动部署端点（D14）：未绑定 400 / agent 离线 503 /
// 成功两次 RPC 往返（cert 0644、key 0600、truncate 覆盖）+ 状态回写 + 审计
func TestAcmeDeployAPI(t *testing.T) {
	s, _ := newAcmeTestServer(t)
	cert := mkAcmeCert(t, s, func(c *storage.AcmeCert) {
		c.Status = "issued"
		c.CertificatePEM = "CERT-PEM"
		c.PrivateKeyPEM = "KEY-PEM"
		c.DeployAgentID = "a1"
		c.DeployCertPath = "/etc/cockpit/certs/home.example.com.crt.pem"
		c.DeployKeyPath = "/etc/cockpit/certs/home.example.com.key.pem"
	})
	idPath := "/acme/certs/" + strconv.Itoa(int(cert.ID)) + "/deploy"

	// 未绑定 → 400
	cert2 := mkAcmeCert(t, s, nil)
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost,
		"/acme/certs/"+strconv.Itoa(int(cert2.ID))+"/deploy", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no target: code = %d body = %s", rec.Code, rec.Body.String())
	}

	// agent 离线 → 503
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, idPath, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("agent offline: code = %d body = %s", rec.Code, rec.Body.String())
	}

	// 成功：两次 file.write 往返，参数逐项断言
	agent := NewAgent("a1", nil)
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		want := []struct {
			path string
			mode float64
			data string
		}{
			{cert.DeployCertPath, 0o644, "CERT-PEM"},
			{cert.DeployKeyPath, 0o600, "KEY-PEM"},
		}
		for i, w := range want {
			reqMsg := <-agent.Send
			params := reqMsg.Payload["params"].(map[string]interface{})
			if params["path"] != w.path {
				t.Errorf("write[%d] path = %v, want %s", i, params["path"], w.path)
			}
			if params["mode"] != w.mode {
				t.Errorf("write[%d] mode = %v, want %v", i, params["mode"], w.mode)
			}
			if params["truncate"] != true {
				t.Errorf("write[%d] truncate = %v, want true", i, params["truncate"])
			}
			if data, err := base64.StdEncoding.DecodeString(params["data"].(string)); err != nil || string(data) != w.data {
				t.Errorf("write[%d] data = %q (%v), want %q", i, data, err, w.data)
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
				"status": "success",
				"data":   map[string]interface{}{"size": 10.0},
			})
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost, idPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("deploy: code = %d body = %s", rec.Code, rec.Body.String())
	}
	got, _ := s.db.GetAcmeCert(cert.ID)
	if got.LastDeployAt == 0 || got.LastDeployError != "" {
		t.Fatalf("after deploy = lastDeployAt %d err %q", got.LastDeployAt, got.LastDeployError)
	}
	// 响应视图无 PEM
	if strings.Contains(rec.Body.String(), "CERT-PEM") {
		t.Error("deploy response must not contain PEM")
	}
	// 审计 acme_deploy
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	found := false
	for _, l := range logs {
		if l.Action == "acme_deploy" && l.ResourceID == cert.PrimaryDomain {
			found = true
		}
	}
	if !found {
		t.Error("audit acme_deploy missing")
	}
}

// TestAcmeIssueAutoDeploy 签发成功后自动推送（D14）：绑定 + agent 在线
// 时 issue 200 且部署状态回写成功；巡检续期共用 runACMEIssue 同路径
func TestAcmeIssueAutoDeploy(t *testing.T) {
	s, _ := newAcmeTestServer(t)
	agent := NewAgent("a1", nil)
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	cert := mkAcmeCert(t, s, func(c *storage.AcmeCert) {
		c.DeployAgentID = "a1"
		c.DeployCertPath = "/etc/cockpit/certs/home.example.com.crt.pem"
		c.DeployKeyPath = "/etc/cockpit/certs/home.example.com.key.pem"
	})
	go func() {
		for i := 0; i < 2; i++ {
			reqMsg := <-agent.Send
			if m := reqMsg.Payload["method"]; m != "file.write" {
				t.Errorf("write[%d] method = %v, want file.write", i, m)
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
				"status": "success",
				"data":   map[string]interface{}{"size": 10.0},
			})
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPost,
		"/acme/certs/"+strconv.Itoa(int(cert.ID))+"/issue", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("issue: code = %d body = %s", rec.Code, rec.Body.String())
	}
	got, _ := s.db.GetAcmeCert(cert.ID)
	if got.Status != "issued" || got.LastDeployAt == 0 || got.LastDeployError != "" {
		t.Fatalf("after issue+deploy = status %s lastDeployAt %d err %q",
			got.Status, got.LastDeployAt, got.LastDeployError)
	}
}

// TestEqualDomains 域名列表相等判定（更新去重用，与顺序无关长度敏感）
func TestEqualDomains(t *testing.T) {
	if !equalDomains(nil, nil) || !equalDomains([]string{}, []string{}) {
		t.Error("empty lists should be equal")
	}
	if !equalDomains([]string{"a.com", "b.com"}, []string{"a.com", "b.com"}) {
		t.Error("identical lists should be equal")
	}
	if equalDomains([]string{"a.com"}, []string{"a.com", "b.com"}) {
		t.Error("length mismatch should not be equal")
	}
	if equalDomains([]string{"a.com", "b.com"}, []string{"a.com", "c.com"}) {
		t.Error("element mismatch should not be equal")
	}
	if equalDomains([]string{"a.com", "b.com"}, []string{"b.com", "a.com"}) {
		t.Error("order matters (sequential compare)")
	}
}

// TestAccountKeyPEMRoundtrip 账户私钥生成与回读（ensureAccount 的纯加密部分）：
// PEM 可解析、曲线 P-256、跨 DER 序列化一致；垃圾输入报错
func TestAccountKeyPEMRoundtrip(t *testing.T) {
	pemStr, err := generateAccountKeyPEM()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(pemStr, "-----BEGIN EC PRIVATE KEY-----") {
		t.Errorf("pem = %q", pemStr[:40])
	}
	key, err := loadAccountKey(pemStr)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok || ecKey.Curve != elliptic.P256() {
		t.Fatalf("key type/curve = %T %v", key, ecKey.Curve)
	}
	// 重解 DER 与原 key 一致
	der, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(pemStr))
	if string(der) != string(block.Bytes) {
		t.Error("re-marshaled DER should match PEM payload")
	}

	// 垃圾输入
	if _, err := loadAccountKey("not a pem"); err == nil {
		t.Error("garbage input should fail")
	}
}

// TestLegoUserAccessors lego registration.User 三访问器透传字段
func TestLegoUserAccessors(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	u := &legoUser{email: "a@b.com", key: key}
	if u.GetEmail() != "a@b.com" {
		t.Errorf("GetEmail = %q", u.GetEmail())
	}
	if u.GetRegistration() != nil {
		t.Error("fresh user should have nil registration")
	}
	if u.GetPrivateKey() != crypto.PrivateKey(key) {
		t.Error("GetPrivateKey should return the same key")
	}
}

// TestAcmeAccountRegisteredViewAndUpdate 已注册账户的 GET 视图与 PUT 更新分支：
// Update 路径（区别于首次 Save）、空 email 合法（清除）、坏 body 400、
// 非 PUT/GET 405
func TestAcmeAccountRegisteredViewAndUpdate(t *testing.T) {
	s, _ := newAcmeTestServer(t)
	if err := s.db.SaveAcmeAccount(&storage.AcmeAccount{
		Email: "old@example.com", CADirectory: "staging", RegistrationURI: "https://acme.example/acct/1",
	}); err != nil {
		t.Fatal(err)
	}

	// GET：registered=true 视图带 email/caDirectory/registrationURI
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, "/acme/account", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: code=%d", rec.Code)
	}
	var view struct {
		Registered      bool   `json:"registered"`
		Email           string `json:"email"`
		CADirectory     string `json:"caDirectory"`
		RegistrationURI string `json:"registrationURI"`
	}
	json.Unmarshal(rec.Body.Bytes(), &view)
	if !view.Registered || view.Email != "old@example.com" ||
		view.CADirectory != "staging" || view.RegistrationURI != "https://acme.example/acct/1" {
		t.Errorf("view = %+v", view)
	}

	// PUT：走 UpdateAcmeAccountEmail 分支（账户已存在）
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/account",
		strings.NewReader(`{"email":"new@example.com"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT update: code=%d body=%s", rec.Code, rec.Body.String())
	}
	acc, _ := s.db.GetAcmeAccount()
	if acc.Email != "new@example.com" {
		t.Errorf("email = %q", acc.Email)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "acme_update"})
	if len(logs) != 1 {
		t.Errorf("acme_update audit = %d, want 1", len(logs))
	}

	// PUT：空 email 合法（允许清除，跳过正则）
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/account",
		strings.NewReader(`{"email":"  "}`)))
	if rec.Code != http.StatusOK {
		t.Errorf("PUT empty email: code=%d", rec.Code)
	}
	if acc, _ = s.db.GetAcmeAccount(); acc.Email != "" {
		t.Errorf("email after clear = %q", acc.Email)
	}

	// PUT：坏 body
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/account", strings.NewReader(`{bad`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: code=%d", rec.Code)
	}

	// 其他方法 405
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodDelete, "/acme/account", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: code=%d", rec.Code)
	}
}

// TestAcmeConfigIntervalAPI 巡检间隔端点（与 /api/ddns/config 同构）：
// GET 全形 / PUT 合法写入回读 / 0 关闭 / 越界与坏 body 400 / 其他方法 405
func TestAcmeConfigIntervalAPI(t *testing.T) {
	s, _ := newAcmeTestServer(t)

	// GET：默认值 + 范围字段
	rec := httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodGet, "/acme/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config: code=%d", rec.Code)
	}
	var cfg struct {
		ScanIntervalSeconds int `json:"scan_interval_seconds"`
		Min                 int `json:"min"`
		Max                 int `json:"max"`
		Default             int `json:"default"`
	}
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.ScanIntervalSeconds != acmeDefaultInterval || cfg.Min != acmeMinIntervalSeconds ||
		cfg.Max != acmeMaxIntervalSeconds || cfg.Default != acmeDefaultInterval {
		t.Fatalf("GET config = %+v", cfg)
	}

	// PUT：合法值写入并回读生效
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/config",
		strings.NewReader(`{"scan_interval_seconds":7200}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT config: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := s.GetAcmeScanInterval(); got != 7200 {
		t.Fatalf("after PUT interval = %d, want 7200", got)
	}

	// PUT：0 = 关闭，合法
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/config",
		strings.NewReader(`{"scan_interval_seconds":0}`)))
	if rec.Code != http.StatusOK {
		t.Errorf("PUT 0: code=%d", rec.Code)
	}

	// PUT：越界与坏 body
	for _, body := range []string{
		`{"scan_interval_seconds":299}`,
		`{"scan_interval_seconds":90000}`,
		`{"scan_interval_seconds":-5}`,
		`{bad`,
	} {
		rec = httptest.NewRecorder()
		s.handleACME(rec, httptest.NewRequest(http.MethodPut, "/acme/config", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: code = %d, want 400", body, rec.Code)
		}
	}

	// 其他方法 405
	rec = httptest.NewRecorder()
	s.handleACME(rec, httptest.NewRequest(http.MethodDelete, "/acme/config", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE config: code=%d", rec.Code)
	}
}
