package server

// cov_round11_test.go 覆盖率：ACME issuer 的配置快照与账户注册错误路径
// （死地址触发 client 构造失败；合法 staging 目录在有外网时走通注册）；
// closed DB 触发 handlers 的 500 族；unit 文件保存超限 413；离线 agent
// 转发 502。log.Fatal 族与真实签发流程不在目标内。

import (
	"bytes"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// TestCovAcmeDNSConfigSnapshot newAcmeDNSConfig 对 nil 与全量配置的快照，
// 以及直构 Server{} 未走 Start 时 acmeDNS 的 cfg 兜底
func TestCovAcmeDNSConfigSnapshot(t *testing.T) {
	if got := newAcmeDNSConfig(nil); got.Provider != "" || got.CloudflareToken != "" {
		t.Fatalf("nil config should snapshot empty, got %+v", got)
	}

	cfg := &config.Config{DNS: &config.DNSConfig{
		Provider:   "dnspod",
		Cloudflare: &config.CloudflareDNSConfig{APIToken: "cf"},
		DNSPod:     &config.DNSPodConfig{LoginToken: "dp"},
		AliDNS:     &config.AliDNSConfig{AccessKey: "ak", SecretKey: "sk"},
	}}
	got := newAcmeDNSConfig(cfg)
	if got.Provider != "dnspod" || got.CloudflareToken != "cf" ||
		got.DNSPodToken != "dp" || got.AliAccessKey != "ak" || got.AliSecretKey != "sk" {
		t.Fatalf("snapshot mismatch: %+v", got)
	}

	// acmeDNSConfig 闭包未注入 → 从 cfg 即时快照
	s := &Server{}
	if out := s.acmeDNS(); out.Provider != "" {
		t.Fatalf("fallback acmeDNS: %+v", out)
	}
}

// TestCovAcmeIssuerEnsureAccountPaths ensureAccount：无账户生成 key 后
// Register 失败、存量账户坏 PEM、closed DB 的账户读取失败
func TestCovAcmeIssuerEnsureAccountPaths(t *testing.T) {
	// 无账户 → 生成 key → 目录可拉但注册端点 500 → Register 失败
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	l := NewLegoIssuer(db, func() AcmeDNSConfig { return AcmeDNSConfig{} }).(*legoIssuer)
	// 此 lego 版本 NewClient 即拉目录：死地址 → client 构造失败
	if _, err := l.ensureAccount("staging", "https://127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "lego client") {
		t.Fatalf("expect client failure, got %v", err)
	}

	// 存量账户但 PEM 损坏 → loadAccountKey 失败
	if err := db.SaveAcmeAccount(&storage.AcmeAccount{Email: "a@example.com", PrivateKeyPEM: "junk"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ensureAccount("staging", "https://127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "load account key") {
		t.Fatalf("expect load key failure, got %v", err)
	}
	db.Close()

	// closed DB → GetAcmeAccount 非 NotFound 错误原样返回
	if _, err := l.ensureAccount("staging", "https://127.0.0.1:1"); err == nil || strings.Contains(err.Error(), "load account key") {
		t.Fatalf("expect raw db error, got %v", err)
	}
}

// TestCovAcmeIssueEntryFailures Issue 入口：未知 CA 目录、closed DB，
// 以及合法目录下 ensureAccount 之后透出的错误（有外网时走到 dnsProvider
// 凭据校验，无外网时止步于 Register）
func TestCovAcmeIssueEntryFailures(t *testing.T) {
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	l := NewLegoIssuer(db, func() AcmeDNSConfig { return AcmeDNSConfig{} }).(*legoIssuer)

	if _, err := l.Issue(&storage.AcmeCert{CADirectory: "bogus"}); err == nil || !strings.Contains(err.Error(), "unknown CA directory") {
		t.Fatalf("expect unknown dir, got %v", err)
	}
	_, err = l.Issue(&storage.AcmeCert{CADirectory: "staging", Domains: []string{"a.example.com"}})
	if err == nil || (!strings.Contains(err.Error(), "acme register") && !strings.Contains(err.Error(), "token not configured")) {
		t.Fatalf("expect register or dns-provider failure, got %v", err)
	}
	db.Close()
	if _, err := l.Issue(&storage.AcmeCert{CADirectory: "staging"}); err == nil {
		t.Fatal("expect db error after close")
	}
}

// TestCovPemLeafNotAfterBadCert 块存在但 DER 无法解析
func TestCovPemLeafNotAfterBadCert(t *testing.T) {
	buf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-der")})
	if _, err := pemLeafNotAfter(buf); err == nil {
		t.Fatal("expect parse error")
	}
}

// TestCovDDNSHandlersClosedDB closed DB 下列表与创建的 500 分支
func TestCovDDNSHandlersClosedDB(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.Close() // t.Cleanup 里的二次 Close 返回错误，无副作用

	rec := httptest.NewRecorder()
	s.handleDDNSList(rec, httptest.NewRequest(http.MethodGet, "/api/network/ddns", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("list closed db: %d %s", rec.Code, rec.Body.String())
	}

	body := strings.NewReader(`{"agentId":"agent1","zoneId":"z","zoneName":"example.com","recordName":"www","type":"A","enabled":true}`)
	rec = httptest.NewRecorder()
	s.handleDDNSCreate(rec, httptest.NewRequest(http.MethodPost, "/api/network/ddns", body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("create closed db: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovUnitFileBodyTooLarge 保存内容超上限 → 413
func TestCovUnitFileBodyTooLarge(t *testing.T) {
	s := newTestServerWithDB(t)
	big := bytes.Repeat([]byte("a"), (unitFileBodyLimit+1)+64)
	body := append([]byte(`{"content":"`), big...)
	body = append(body, []byte(`"}`)...)

	rec := httptest.NewRecorder()
	s.handleUnitFile(rec, httptest.NewRequest(http.MethodPut, "/api/agents/a/services/nginx.service/file", bytes.NewReader(body)),
		"a", "nginx.service/file", true)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expect 413, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovForwardServiceRPCOffline agent 未注册 → CallAgent 失败 → 502
func TestCovForwardServiceRPCOffline(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := httptest.NewRecorder()
	s.forwardServiceRPC(rec, httptest.NewRequest(http.MethodGet, "/x", nil), "nobody", "service.list", nil, "", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expect 502, got %d %s", rec.Code, rec.Body.String())
	}
}
