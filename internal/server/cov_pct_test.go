package server

// cov_pct_test.go 覆盖率补测（第一部分）：ACME 签发/巡检/API 族。
// 手段：
//   - httptest.NewTLSServer 伪造 ACME 端点 + LEGO_CA_CERTIFICATES 环境变量
//     （lego NewConfig 每次构造 HTTPClient 时实时读取该变量建证书池，无缓存）
//     ——lego sender 强制 HTTPS，纯 HTTP 伪端点在拉目录前即被拒
//   - storage.Open 以 mattn DSN 参数 _query_only=1 重开已迁移好的库文件：
//     读正常、写一律失败，覆盖「先读成功、后写失败」的 500 族分支
//   - closed db 覆盖「首个 DB 操作即失败」的分支；closed agent（Close 不
//     Unregister）让 CallAgent 立即报 "agent X is closed"，免 30s 超时

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 通用基建 ============

// covPctQueryOnlyDB 先建并迁移一个可写库（seed 回调在可写阶段塞数据），
// 关闭后以 _query_only=1 重开：读正常、写全部失败。
// 用于「读成功→写失败」分支。
func covPctQueryOnlyDB(t *testing.T, seed func(db *storage.DB)) *Server {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.db")

	// 第一阶段：可写库，跑迁移 + 种子数据
	db, err := storage.Open(storage.Config{Path: path})
	if err != nil {
		t.Fatalf("open writable db: %v", err)
	}
	// 探测种子固定在可写阶段写入（不依赖调用方的 seed）
	if err := db.SetSetting("cov_pct_probe", "1"); err != nil {
		t.Fatalf("seed probe setting: %v", err)
	}
	if seed != nil {
		seed(db)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable db: %v", err)
	}

	// 第二阶段：只读重开（file: 前缀使 mattn 解析 DSN 参数；迁移识别
	// schema 完整而不产生写）
	dbRO, err := storage.Open(storage.Config{Path: "file:" + path + "?_query_only=1"})
	if err != nil {
		t.Fatalf("reopen query-only db: %v", err)
	}
	t.Cleanup(func() { _ = dbRO.Close() })

	// 只读验证：种子读回必须成功、新写必须失败（否则该手法失效，测试直接暴露）
	if v, err := dbRO.GetSetting("cov_pct_probe"); err != nil || v != "1" {
		t.Fatalf("query-only db read broken: %v %q", err, v)
	}
	if err := dbRO.SetSetting("cov_pct_probe2", "1"); err == nil {
		t.Fatalf("query-only db unexpectedly accepted a write")
	}

	return &Server{db: dbRO}
}

// covPctFakeACME 起一个最小 HTTPS ACME 伪造端点，返回目录 URL。
// newAccountStatus 控制 newAccount 端点返回码（0 视为 201）。
// lego sender 强制 HTTPS 且 createDefaultHTTPClient 的证书池只认 CA 证书
// （httptest 自签证书 IsCA=false 无法作为根加入），故自建 CA 签发服务端
// 证书，CA 的 PEM 写文件后经 LEGO_CA_CERTIFICATES 注入——lego 每次
// NewConfig 都实时读该变量建证书池，无缓存。
func covPctFakeACME(t *testing.T, newAccountStatus int) string {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "https://" + r.Host
		switch r.URL.Path {
		case "/": // ACME 目录就在 dirURL 根路径
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"newNonce": %q, "newAccount": %q, "newOrder": %q,
				"revokeCert": %q, "keyChange": %q
			}`, base+"/nonce", base+"/new-account", base+"/new-order",
				base+"/revoke-cert", base+"/key-change")))
		case "/nonce":
			w.Header().Set("Replay-Nonce", "cov-nonce")
			w.WriteHeader(http.StatusNoContent)
		case "/new-account":
			code := newAccountStatus
			if code == 0 {
				code = http.StatusCreated
			}
			w.Header().Set("Location", base+"/account/1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"status":"valid"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	})

	// 自签 CA（IsCA=true，AppendCertsFromPEM 才接受）
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cov-pct-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}

	// 由 CA 签发 SAN 含 127.0.0.1/::1/localhost 的服务端证书
	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen server key: %v", err)
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{srvDER, caDER},
		PrivateKey:  srvKey,
	}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	// CA 的 PEM 写文件，注入 lego 的 CA 证书池环境变量
	caFile := filepath.Join(t.TempDir(), "cov-acme-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("write ca pem: %v", err)
	}
	t.Setenv("LEGO_CA_CERTIFICATES", caFile)
	return srv.URL
}

// covPctIssuer 构造指向注入闭包的 legoIssuer
func covPctIssuer(db *storage.DB) *legoIssuer {
	return NewLegoIssuer(db, func() AcmeDNSConfig { return AcmeDNSConfig{} }).(*legoIssuer)
}

// covPctSeedCert 可写阶段预置一条 AcmeCert（默认 id=1）
func covPctSeedCert(t *testing.T, db *storage.DB) {
	t.Helper()
	if err := db.CreateAcmeCert(&storage.AcmeCert{
		ID:            1,
		Domains:       []string{"cov.example.com"},
		PrimaryDomain: "cov.example.com",
		CADirectory:   AcmeCADirectoryStaging,
		Status:        "issued",
		DeployAgentID: "deploy-closed",
	}); err != nil {
		t.Fatalf("seed acme cert: %v", err)
	}
}

// ============ acme_issuer.go ============

// TestCovPctAcmeDNSClosure 覆盖 acmeDNS 的闭包注入路径（L133-135：
// s.acmeDNSConfig != nil 时直接调用闭包）
func TestCovPctAcmeDNSClosure(t *testing.T) {
	called := false
	s := &Server{acmeDNSConfig: func() AcmeDNSConfig {
		called = true
		return AcmeDNSConfig{Provider: "cloudflare", CloudflareToken: "tok"}
	}}
	got := s.acmeDNS()
	if !called || got.CloudflareToken != "tok" {
		t.Fatalf("acmeDNS closure path not exercised: called=%v got=%+v", called, got)
	}
}

// TestCovPctAcmeIssueRegisterFail 覆盖 ensureAccount 的 Register 错误分支
// （acme_issuer.go L202-204）：HTTPS 伪造端点目录可拉、newAccount 返回 500。
func TestCovPctAcmeIssueRegisterFail(t *testing.T) {
	db, err := storage.Open(storage.Config{Path: filepath.Join(t.TempDir(), "a.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	dirURL := covPctFakeACME(t, http.StatusInternalServerError)
	l := covPctIssuer(db)
	if _, err := l.ensureAccount(AcmeCADirectoryStaging, dirURL); err == nil ||
		!strings.Contains(err.Error(), "acme register") {
		t.Fatalf("expect acme register failure, got %v", err)
	}
}

// TestCovPctAcmeIssueSaveAccountFail 覆盖 ensureAccount 的 SaveAcmeAccount
// 错误分支（acme_issuer.go L210-212）：HTTPS 端点注册成功，但库只读写失败。
func TestCovPctAcmeIssueSaveAccountFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, nil)
	dirURL := covPctFakeACME(t, 0)
	l := covPctIssuer(s.db)
	if _, err := l.ensureAccount(AcmeCADirectoryStaging, dirURL); err == nil {
		t.Fatalf("expect save acme account failure on query-only db")
	}
}

// TestCovPctIssueUnknownDir 覆盖 Issue 的未知 CA 目录分支
// （acme_issuer.go L217-220，单行 if 命中即算）
func TestCovPctIssueUnknownDir(t *testing.T) {
	db, err := storage.Open(storage.Config{Path: filepath.Join(t.TempDir(), "b.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	l := covPctIssuer(db)
	if _, err := l.Issue(&storage.AcmeCert{CADirectory: "nope"}); err == nil ||
		!strings.Contains(err.Error(), "unknown CA directory") {
		t.Fatalf("expect unknown CA directory error, got %v", err)
	}
}

// ============ acme_scan.go ============

// covPctFakeIssuer 测试注入用签发器（实现 AcmeIssuer 接口）
type covPctFakeIssuer struct {
	res *IssuedResult
	err error
}

func (f *covPctFakeIssuer) Issue(cert *storage.AcmeCert) (*IssuedResult, error) {
	return f.res, f.err
}

// TestCovPctRunACMEIssueUpdateFail 覆盖 runACMEIssue 签发成功但回写库失败
// （acme_scan.go L156-158）：伪造 issuer 成功 + closed db。
func TestCovPctRunACMEIssueUpdateFail(t *testing.T) {
	s := covNewServer(t)
	s.acme = &covPctFakeIssuer{res: &IssuedResult{CertificatePEM: "c", ExpiresAt: time.Now()}}
	covCloseDB(t, s)
	cert := &storage.AcmeCert{ID: 1, PrimaryDomain: "x.example.com", AutoRenew: true}
	if _, err := s.runACMEIssue(cert, nil, time.Now()); err == nil {
		t.Fatalf("expect update failure on closed db")
	}
}

// TestCovPctObservationLookupFail 覆盖 upsertAcmeCertObservation 的
// GetCertificate 非 NotFound 错误分支（acme_scan.go L172-175）：closed db。
func TestCovPctObservationLookupFail(t *testing.T) {
	s := covNewServer(t)
	covCloseDB(t, s)
	s.upsertAcmeCertObservation(&storage.AcmeCert{ID: 7, PrimaryDomain: "y.example.com"})
}

// TestCovPctObservationUpsertFail 覆盖 UpsertCertificate 失败分支
// （acme_scan.go L191-193）：只读库读成功（NotFound 走新建）但写失败。
func TestCovPctObservationUpsertFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, nil)
	s.upsertAcmeCertObservation(&storage.AcmeCert{ID: 7, PrimaryDomain: "y.example.com"})
}

// ============ api_acme.go ============

// TestCovPctAcmeCertSubBadID 覆盖 handleAcmeCertSub 的非数字 id 分支
// （api_acme.go L74-77）：PUT /acme/certs/abc → 404。
func TestCovPctAcmeCertSubBadID(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := covRec()
	s.handleAcmeCertSub(rec, covReq(http.MethodPut, "/api/acme/certs/abc", nil), "abc")
	covWantCode(t, "bad id", rec, http.StatusNotFound)
}

// TestCovPctAcmeListDBFail 覆盖 handleAcmeList 的 ListAcmeCerts 失败分支
// （api_acme.go L229-232）：closed db。
func TestCovPctAcmeListDBFail(t *testing.T) {
	s := covNewServer(t)
	covCloseDB(t, s)
	rec := covRec()
	s.handleAcmeList(rec, covReq(http.MethodGet, "/api/acme/certs", nil))
	covWantCode(t, "list 500", rec, http.StatusInternalServerError)
}

// TestCovPctAcmeCreateDBFail 覆盖 handleAcmeCreate 的 CreateAcmeCert 失败
// 分支（api_acme.go L267-270）：合法 body + 只读库（校验全过、写被拒）。
func TestCovPctAcmeCreateDBFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, nil)
	body := strings.NewReader(`{"domains":["cov.example.com"],"caDirectory":"staging"}`)
	rec := covRec()
	s.handleAcmeCreate(rec, covReq(http.MethodPost, "/api/acme/certs", body))
	covWantCode(t, "create 500", rec, http.StatusInternalServerError)
}

// TestCovPctAcmeUpdateValidations 覆盖 handleAcmeUpdate 的三个校验失败
// 分支：坏 JSON（L282-285）、坏域名（L287-290）、坏 deploy（L291-294）。
func TestCovPctAcmeUpdateValidations(t *testing.T) {
	s := newTestServerWithDB(t)
	covPctSeedCert(t, s.db)

	// 坏 JSON：cert 已读到，body 解码失败
	rec := covRec()
	s.handleAcmeUpdate(rec, covReq(http.MethodPut, "/api/acme/certs/1", strings.NewReader(`{`)), 1)
	covWantCode(t, "update bad json", rec, http.StatusBadRequest)

	// 域名列表为空：validateAcmeInput 失败
	rec = covRec()
	s.handleAcmeUpdate(rec, covReq(http.MethodPut, "/api/acme/certs/1", strings.NewReader(`{"domains":[]}`)), 1)
	covWantCode(t, "update bad domains", rec, http.StatusBadRequest)

	// deploy 只给路径不给 agentID：validateAcmeDeploy 失败
	rec = covRec()
	s.handleAcmeUpdate(rec, covReq(http.MethodPut, "/api/acme/certs/1",
		strings.NewReader(`{"domains":["cov.example.com"],"deployCertPath":"/tmp/c.pem"}`)), 1)
	covWantCode(t, "update bad deploy", rec, http.StatusBadRequest)
}

// TestCovPctAcmeUpdateDBFail 覆盖 handleAcmeUpdate 的 UpdateAcmeCert 失败
// 分支（api_acme.go L311-314）：只读库读成功、合法 body、写被拒。
func TestCovPctAcmeUpdateDBFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, func(db *storage.DB) { covPctSeedCert(t, db) })
	body := strings.NewReader(`{"domains":["cov.example.com"],"caDirectory":"staging"}`)
	rec := covRec()
	s.handleAcmeUpdate(rec, covReq(http.MethodPut, "/api/acme/certs/1", body), 1)
	covWantCode(t, "update 500", rec, http.StatusInternalServerError)
}

// TestCovPctAcmeDeleteDBFail 覆盖 handleAcmeDelete 的 DeleteAcmeCert 失败
// 分支（api_acme.go L338-341）：只读库读成功、删除被拒。
func TestCovPctAcmeDeleteDBFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, func(db *storage.DB) { covPctSeedCert(t, db) })
	rec := covRec()
	s.handleAcmeDelete(rec, covReq(http.MethodDelete, "/api/acme/certs/1", nil), 1)
	covWantCode(t, "delete 500", rec, http.StatusInternalServerError)
}

// TestCovPctAcmeDeployWriteReject 覆盖 deployAcmeCert 第一次 file.write 被
// agent 拒绝（api_acme.go L416-418）：fake agent 回 error status。
func TestCovPctAcmeDeployWriteReject(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "deploy-reject", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("denied")
	})
	cert := &storage.AcmeCert{
		ID: 1, PrimaryDomain: "cov.example.com",
		DeployAgentID:  "deploy-reject",
		DeployCertPath: "/tmp/c.pem", DeployKeyPath: "/tmp/k.pem",
		CertificatePEM: "CERT", PrivateKeyPEM: "KEY",
	}
	err := s.deployAcmeCert(cert)
	if err == nil || !strings.Contains(err.Error(), "rejected by agent") {
		t.Fatalf("expect write reject, got %v", err)
	}
	if cert.LastDeployError == "" {
		t.Fatalf("LastDeployError should be recorded")
	}
}

// TestCovPctAcmeDeployKeyReject 覆盖 deployAcmeCert 第二次（key）file.write
// 被拒（api_acme.go L424-426）：cert 路径成功、key 路径回 error。
func TestCovPctAcmeDeployKeyReject(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "deploy-key-reject", nil, func(_ string, params map[string]interface{}) map[string]interface{} {
		path, _ := params["path"].(string)
		if strings.HasSuffix(path, "k.pem") {
			return covErrPayload("key denied")
		}
		return covOKPayload(nil)
	})
	cert := &storage.AcmeCert{
		ID: 1, PrimaryDomain: "cov.example.com",
		DeployAgentID:  "deploy-key-reject",
		DeployCertPath: "/tmp/c.pem", DeployKeyPath: "/tmp/k.pem",
		CertificatePEM: "CERT", PrivateKeyPEM: "KEY",
	}
	err := s.deployAcmeCert(cert)
	if err == nil || !strings.Contains(err.Error(), "rejected by agent") {
		t.Fatalf("expect key write reject, got %v", err)
	}
}

// TestCovPctAcmeDeployFinalUpdateFail 覆盖 deployAcmeCert 末尾
// UpdateAcmeCert 失败分支（api_acme.go L429-431）：两次 write 成功 +
// closed db（写部署状态时失败）。
func TestCovPctAcmeDeployFinalUpdateFail(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "deploy-ok", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covOKPayload(nil)
	})
	cert := &storage.AcmeCert{
		ID: 1, PrimaryDomain: "cov.example.com",
		DeployAgentID:  "deploy-ok",
		DeployCertPath: "/tmp/c.pem", DeployKeyPath: "/tmp/k.pem",
		CertificatePEM: "CERT", PrivateKeyPEM: "KEY",
	}
	covCloseDB(t, s)
	if err := s.deployAcmeCert(cert); err == nil {
		t.Fatalf("expect final update failure on closed db")
	}
}

// TestCovPctAcmeDeployHandler502 覆盖 handleAcmeDeploy 的 deployAcmeCert
// 失败分支（api_acme.go L472-475）：registry 命中但 agent 已 Close（不
// Unregister）→ CallAgent 立即报 "agent X is closed" → 502。
func TestCovPctAcmeDeployHandler502(t *testing.T) {
	s := newTestServerWithDB(t)
	covPctSeedCert(t, s.db)
	closed := NewAgent("deploy-closed", nil)
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close() // 不 Unregister：registry.Get 仍命中，SendWithTimeout 立即失败

	rec := covRec()
	s.handleAcmeDeploy(rec, covReq(http.MethodPost, "/api/acme/certs/1/deploy", nil), "1")
	covWantCode(t, "deploy 502", rec, http.StatusBadGateway)
}

// TestCovPctAcmeAccountSaveFail 覆盖 handleAcmeAccountAPI PUT 的
// SaveAcmeAccount 失败分支（api_acme.go L558-561）：只读库无账户，
// GetAcmeAccount 返回 not found → 走 Save 分支 → 写被拒。
func TestCovPctAcmeAccountSaveFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, nil)
	rec := covRec()
	s.handleAcmeAccountAPI(rec, covReq(http.MethodPut, "/api/acme/account",
		strings.NewReader(`{"email":"cov@example.com"}`)))
	covWantCode(t, "account save 500", rec, http.StatusInternalServerError)
}

// TestCovPctAcmeAccountUpdateFail 覆盖 handleAcmeAccountAPI PUT 的
// UpdateAcmeAccountEmail 失败分支（api_acme.go L562-565）：只读库已有
// 账户，读成功、更新被拒。
func TestCovPctAcmeAccountUpdateFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, func(db *storage.DB) {
		if err := db.SaveAcmeAccount(&storage.AcmeAccount{Email: "old@example.com"}); err != nil {
			t.Fatalf("seed acme account: %v", err)
		}
	})
	rec := covRec()
	s.handleAcmeAccountAPI(rec, covReq(http.MethodPut, "/api/acme/account",
		strings.NewReader(`{"email":"new@example.com"}`)))
	covWantCode(t, "account update 500", rec, http.StatusInternalServerError)
}
