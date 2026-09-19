package server

// cov_final_test.go 覆盖率收尾（最后一轮）：NewServer/Start 启动防御分支、
// registerRoutes 认证分发 switch、TOTP/重置/审计/票据/备份/ACME 的注入
// 错误分支。手段延续 cov_pct 系列：包级 var 注入 + t.Cleanup 恢复、
// panic 哨兵截获 logFatalf、HTTPS fake ACME 端点，全程不出网。

import (
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	lego "github.com/go-acme/lego/v4/lego"
	"github.com/pquerna/otp/totp"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 通用基建 ============

// covFinalFatal logFatalf 哨兵载体
type covFinalFatal struct{ msg string }

// covFinalInterceptFatal 把 logFatalf 换成 panic 哨兵（NewServer/Start 的
// 防御分支以 log.Fatal 终止进程，测试截获后 recover 断言），t.Cleanup 还原
func covFinalInterceptFatal(t *testing.T) {
	t.Helper()
	old := logFatalf
	logFatalf = func(format string, args ...any) {
		panic(covFinalFatal{msg: fmt.Sprintf(format, args...)})
	}
	t.Cleanup(func() { logFatalf = old })
}

// covFinalWantFatal 断言 fn 以 logFatalf panic 退出且消息含 wantSub
func covFinalWantFatal(t *testing.T, wantSub string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		f, ok := r.(covFinalFatal)
		if !ok {
			t.Fatalf("want logFatalf panic, got %#v", r)
		}
		if !strings.Contains(f.msg, wantSub) {
			t.Fatalf("fatal message %q, want substring %q", f.msg, wantSub)
		}
	}()
	fn()
}

// covFinalErrReader 恒错误的熵源
type covFinalErrReader struct{}

func (covFinalErrReader) Read(p []byte) (int, error) { return 0, errors.New("cov-final: no entropy") }

// covFinalPEMBlock 伪造 PEM block（仅作假 Resource 的占位字节，不参与解析）
func covFinalPEMBlock(typ string) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: []byte("cov-final-fake")})
}

// ============ NewServer / Start 防御分支 ============

// TestCovFinalNewServerOpenFail 覆盖 NewServer 存储打开失败：/proc 下路径
// 必然创建失败，logFatalf 被哨兵截获。
func TestCovFinalNewServerOpenFail(t *testing.T) {
	t.Chdir(t.TempDir())
	covFinalInterceptFatal(t)
	covFinalWantFatal(t, "Failed to open database", func() {
		NewServer(&config.Config{Database: &config.DatabaseConfig{Path: "/proc/cov-final-nope/db.db"}})
	})
}

// TestCovFinalNewServerProdKey 覆盖 PRODUCTION=true 下密钥校验的通过与
// 失败两个分支（校验函数注入，不依赖环境）。
func TestCovFinalNewServerProdKey(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PRODUCTION", "true")

	old := storageValidateKey
	defer func() { storageValidateKey = old }()

	// 校验通过 → 正常构造
	storageValidateKey = func() error { return nil }
	if s := NewServer(&config.Config{}); s == nil || s.db == nil {
		t.Fatal("NewServer with valid key: want server")
	}

	// 校验失败 → SECURITY ERROR fatal
	storageValidateKey = func() error { return errors.New("cov-final: weak key") }
	covFinalInterceptFatal(t)
	covFinalWantFatal(t, "SECURITY ERROR", func() { NewServer(&config.Config{}) })
}

// TestCovFinalStartPasswordChecks 覆盖 Start 的管理员密码三道防线：
// 缺失 / 长度不足 / 常见弱密码。
func TestCovFinalStartPasswordChecks(t *testing.T) {
	cases := []struct {
		name    string
		pass    string
		wantSub string
	}{
		{"empty", "", "ADMIN_PASSWORD environment variable is required"},
		{"short", "Sh0rt", "at least 8 characters"},
		{"weak", "password", "too weak"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			s := NewServer(&config.Config{})
			defer s.Shutdown()
			covFinalInterceptFatal(t)
			t.Setenv("ADMIN_PASSWORD", tc.pass)
			covFinalWantFatal(t, tc.wantSub, func() { _ = s.Start() })
		})
	}
}

// TestCovFinalStartFullPath 走通 Start 全路径直到 ListenAndServe：预占端口
// 使监听立即失败返回（Start 在监听前完成 InitAdmin、路由注册、ACME 闭包
// 注入与 overlay 客户端构造）。Start 返回后 acmeDNSConfig 闭包已就位，
// 调用 acmeDNS() 覆盖闭包体。
func TestCovFinalStartFullPath(t *testing.T) {
	t.Chdir(t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	oldValidate := storageValidateKey
	storageValidateKey = func() error { return nil }
	t.Cleanup(func() { storageValidateKey = oldValidate })

	cfg := &config.Config{
		Server: &config.ServerConfig{Host: "127.0.0.1", Port: port},
		JWT:    &config.JWTConfig{Secret: "cov-final-secret", Expiration: time.Hour},
		Overlay: &config.OverlayConfig{
			ZeroTier:  &config.ZeroTierCloudConfig{APIToken: "cov-zt-token"},
			Tailscale: &config.TailscaleCloudConfig{APIToken: "cov-ts-token", Tailnet: "cov-final@example.com"},
		},
	}
	s := NewServer(cfg)
	defer s.Shutdown()

	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "cov-final-strong-pass")
	if err := s.Start(); err == nil {
		t.Fatal("Start: want error (port occupied), got nil")
	}
	if s.overlayZT == nil || s.overlayTS == nil {
		t.Fatal("Start: overlay clients not constructed")
	}
	// Start 注入的 ACME DNS 闭包体执行
	if cfg2 := s.acmeDNS(); cfg2.Provider != "" && cfg2.Provider != "cloudflare" {
		t.Fatalf("acmeDNS provider = %q", cfg2.Provider)
	}
}

// ============ registerRoutes 认证分发 switch ============

// TestCovFinalAuthSwitchCases 覆盖 /api/ 前缀守卫内 switch 的 login /
// refresh / totp verify case 体。这三条路径已注册精确模式，正常请求不会
// 落入前缀分支；利用 Go 1.22+ ServeMux「按 escaped path 路由、handler 收
// 到解码 path」的行为，%2F 转义使请求落入 /api/ 前缀分支后 switch 又按解
// 码后的 path 精确命中 case。
func TestCovFinalAuthSwitchCases(t *testing.T) {
	s := covNewServer(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	cases := []struct {
		desc string
		path string
		body string
		want int
	}{
		{"login case", "/api/auth%2Flogin", `{"username":"ghost","password":"wrong"}`, http.StatusUnauthorized},
		{"refresh case", "/api/auth%2Frefresh", `{"refresh_token":"bogus"}`, http.StatusUnauthorized},
		{"totp verify case", "/api/auth%2Ftotp%2Fverify", `{"code":"123456"}`, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "http://cov-final"+tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		covWantCode(t, tc.desc, rec, tc.want)
	}
}

// TestCovFinalInventoryConsistencyCase 覆盖 serveAPI 分发的
// /inventory/consistency case 体（sync 未启用 → 503 指名引导）。
func TestCovFinalInventoryConsistencyCase(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.serveAPI(rec, covReq(http.MethodGet, "/api/inventory/consistency", nil))
	covWantCode(t, "inventory consistency 503", rec, http.StatusServiceUnavailable)
}

// TestCovFinalInventorySyncManagerErr 覆盖 startInventorySync 的 sync
// manager 构造失败两分支（非严格记日志跳过 / 严格返回错误）。构造失败的
// 唯一入口是 fsnotify.NewWatcher 的 inotify instance 耗尽：先占满
// max_user_instances 配额（预留不足即跳过），被测构造即报错。
func TestCovFinalInventorySyncManagerErr(t *testing.T) {
	var holders []*fsnotify.Watcher
	t.Cleanup(func() {
		for _, w := range holders {
			_ = w.Close()
		}
	})
	for i := 0; i < 4096; i++ {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			break // inotify instance 配额耗尽
		}
		holders = append(holders, w)
	}
	if len(holders) == 0 {
		t.Skip("inotify instance 配额无法耗尽，环境不支持该分支")
	}

	s := covNewServer(t)
	s.cfg = &config.Config{Inventory: &config.InventoryConfig{
		Watch: true, Path: "/tmp/cov-final-inventory.yaml", Strict: false,
	}}
	// 非严格：构造失败记日志后跳过
	if err := s.startInventorySync(); err != nil {
		t.Fatalf("startInventorySync non-strict: want nil, got %v", err)
	}
	// 严格：构造失败原样返回
	s.cfg.Inventory.Strict = true
	if err := s.startInventorySync(); err == nil {
		t.Fatal("startInventorySync strict: want error, got nil")
	}
}

// ============ 注入错误分支 ============

// TestCovFinalTicketRandReadErr 覆盖 GenerateTicket 的熵源失败分支
func TestCovFinalTicketRandReadErr(t *testing.T) {
	old := randRead
	randRead = func(b []byte) (int, error) { return 0, errors.New("cov-final: rand") }
	t.Cleanup(func() { randRead = old })

	tm := NewTicketManager()
	if _, err := tm.GenerateTicket("u1", "u1", nil); err == nil {
		t.Fatal("GenerateTicket: want error, got nil")
	}
}

// TestCovFinalTicketCreateFail 覆盖 handleTicketCreate 的票据生成失败 →
// 500 分支（前置：agent 在线 + 目标放行 + 认证用户，全部真实通过）。
func TestCovFinalTicketCreateFail(t *testing.T) {
	s := covNewServer(t)
	s.cfg = &config.Config{RemoteControl: &config.RemoteControlConfig{AllowArbitraryTarget: true}}
	covFakeAgent(t, s, "covfinal-a", nil, nil)
	user := covSeedUser(t, s, "covfinal-remote", "cov-pass-123", "admin")

	old := randRead
	randRead = func(b []byte) (int, error) { return 0, errors.New("cov-final: rand") }
	t.Cleanup(func() { randRead = old })

	body := `{"agent_id":"covfinal-a","host":"10.0.0.8","port":22,"protocol":"ssh"}`
	req := covAuthReq(http.MethodPost, "/api/remote/ticket", strings.NewReader(body), user.ID, "covfinal-remote", "admin")
	rec := covCallAuth(s, s.handleTicketCreate, req)
	covWantCode(t, "ticket create rand err", rec, http.StatusInternalServerError)
}

// TestCovFinalForgotResetTokenErr 覆盖忘记密码的重置令牌生成失败分支
func TestCovFinalForgotResetTokenErr(t *testing.T) {
	s := covNewServer(t)
	covSeedUser(t, s, "covfinal-forgot", "cov-pass-123", "user")

	old := authGenerateResetToken
	authGenerateResetToken = func(userID, email string) (string, string, error) {
		return "", "", errors.New("cov-final: reset token")
	}
	t.Cleanup(func() { authGenerateResetToken = old })

	rec := covRec()
	s.handleForgotPassword(rec, covReq(http.MethodPost, "/api/auth/forgot-password",
		strings.NewReader(`{"username":"covfinal-forgot"}`)))
	covWantCode(t, "forgot reset token err", rec, http.StatusInternalServerError)
}

// TestCovFinalAuditStatsErr 覆盖审计统计查询失败 → 500 分支
func TestCovFinalAuditStatsErr(t *testing.T) {
	s := covNewServer(t)
	old := dbGetAuditLogStats
	dbGetAuditLogStats = func(*storage.DB) (map[string]interface{}, error) {
		return nil, errors.New("cov-final: stats")
	}
	t.Cleanup(func() { dbGetAuditLogStats = old })

	rec := covRec()
	s.handleAuditLogStats(rec, covReq(http.MethodGet, "/api/admin/audit/stats", nil))
	covWantCode(t, "audit stats err", rec, http.StatusInternalServerError)
}

// TestCovFinalBackupChmodErr 覆盖 server 备份 chmod 失败告警分支（仅记
// 日志，备份本身仍成功）。
func TestCovFinalBackupChmodErr(t *testing.T) {
	dir := t.TempDir()
	s := &Server{db: testServerDB(t), cfg: &config.Config{
		Database: &config.DatabaseConfig{Path: filepath.Join(dir, "cockpit.db")},
	}}

	old := osChmod
	osChmod = func(string, os.FileMode) error { return errors.New("cov-final: chmod") }
	t.Cleanup(func() { osChmod = old })

	name, err := s.runServerBackup()
	if err != nil {
		t.Fatalf("runServerBackup: want nil error, got %v", err)
	}
	if name == "" {
		t.Fatal("runServerBackup: want backup name")
	}
}

// TestCovFinalTOTPGenerateInject 覆盖 TOTP 生成的三个注入失败分支
// （备份码生成 / 密钥加密 / 备份码哈希）。
func TestCovFinalTOTPGenerateInject(t *testing.T) {
	s := covNewServer(t)
	user := covSeedUser(t, s, "covfinal-totp", "cov-pass-123", "user")
	authed := func() *httptest.ResponseRecorder {
		return covCallAuth(s, s.handleTOTPGenerate,
			covAuthReq(http.MethodPost, "/api/auth/totp/generate", nil, user.ID, "covfinal-totp", "user"))
	}

	t.Run("backup codes", func(t *testing.T) {
		old := storageGenerateBackupCodes
		storageGenerateBackupCodes = func() ([]string, error) { return nil, errors.New("cov-final: codes") }
		t.Cleanup(func() { storageGenerateBackupCodes = old })
		covWantCode(t, "gen backup-codes err", authed(), http.StatusInternalServerError)
	})
	t.Run("encrypt", func(t *testing.T) {
		old := storageEncrypt
		storageEncrypt = func(string) (string, error) { return "", errors.New("cov-final: encrypt") }
		t.Cleanup(func() { storageEncrypt = old })
		covWantCode(t, "gen encrypt err", authed(), http.StatusInternalServerError)
	})
	t.Run("hash", func(t *testing.T) {
		old := storageHashBackupCodes
		storageHashBackupCodes = func([]string) ([]string, error) { return nil, errors.New("cov-final: hash") }
		t.Cleanup(func() { storageHashBackupCodes = old })
		covWantCode(t, "gen hash err", authed(), http.StatusInternalServerError)
	})
}

// TestCovFinalTOTPVerifyTokenErr 覆盖 TOTP 验证通过后会话签发失败的 500
// 分支：完整走 generate→enable→login 拿临时令牌，对码后注入签发错误。
func TestCovFinalTOTPVerifyTokenErr(t *testing.T) {
	s := covNewServer(t)
	pass := "cov-pass-123"
	user := covSeedUser(t, s, "covfinal-totp2", pass, "user")

	// generate + enable TOTP
	rec := covCallAuth(s, s.handleTOTPGenerate,
		covAuthReq(http.MethodPost, "/api/auth/totp/generate", nil, user.ID, "covfinal-totp2", "user"))
	covWantCode(t, "generate", rec, http.StatusOK)
	var gen TOTPGenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&gen); err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(gen.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	enableRec := covCallAuth(s, s.handleTOTPEnable,
		covAuthReq(http.MethodPost, "/api/auth/totp/enable",
			strings.NewReader(`{"code":"`+code+`"}`), user.ID, "covfinal-totp2", "user"))
	covWantCode(t, "enable", enableRec, http.StatusOK)

	// 登录拿临时令牌
	loginRec := covRec()
	s.handleLoginWithAudit(loginRec, covReq(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"covfinal-totp2","password":"`+pass+`"}`)))
	var lr struct {
		TmpToken string `json:"tmp_token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &lr); err != nil || lr.TmpToken == "" {
		t.Fatalf("login = %d %q (%v)", loginRec.Code, loginRec.Body.String(), err)
	}

	// 注入会话签发失败 → 验证走到签发一步才 500
	old := serviceGenerateToken
	serviceGenerateToken = func(*auth.Service, string, string, string) (string, error) {
		return "", errors.New("cov-final: token")
	}
	t.Cleanup(func() { serviceGenerateToken = old })

	verifyRec := covRec()
	s.handleTOTPVerify(verifyRec, covReq(http.MethodPost, "/api/auth/totp/verify",
		strings.NewReader(`{"code":"`+code+`","tmp_token":"`+lr.TmpToken+`"}`)))
	covWantCode(t, "verify token err", verifyRec, http.StatusInternalServerError)
}

// ============ ACME 签发链路 ============

// TestCovFinalAcmeDNSUnknownProvider 未知 provider 在 Ready() 处即被拦截
// （Ready 与 dnsProvider 的 provider 集合一致，后者 switch 的 default 为
// 结构性死分支，仅作防御）。
func TestCovFinalAcmeDNSUnknownProvider(t *testing.T) {
	_, err := dnsProvider(AcmeDNSConfig{Provider: "cov-final-unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown ACME DNS provider") {
		t.Fatalf("dnsProvider: want unknown provider error, got %v", err)
	}
}

// TestCovFinalAcmeAccountKeyGenErr 覆盖 ensureAccount 的账户私钥生成失败
// 分支（空库走 ErrAcmeAccountNotFound → 生成注入失败）。
func TestCovFinalAcmeAccountKeyGenErr(t *testing.T) {
	db := testServerDB(t)
	old := generateAccountKeyPEMFn
	generateAccountKeyPEMFn = func() (string, error) { return "", errors.New("cov-final: key") }
	t.Cleanup(func() { generateAccountKeyPEMFn = old })

	l := NewLegoIssuer(db, func() AcmeDNSConfig {
		return AcmeDNSConfig{Provider: "cloudflare", CloudflareToken: "cov-token"}
	}).(*legoIssuer)
	_, err := l.Issue(&storage.AcmeCert{
		Domains:       []string{"cov-final.example.com"},
		PrimaryDomain: "cov-final.example.com",
		CADirectory:   AcmeCADirectoryStaging,
	})
	if err == nil || !strings.Contains(err.Error(), "generate account key") {
		t.Fatalf("Issue: want generate account key error, got %v", err)
	}
}

// TestCovFinalAcmeIssueInject 覆盖 Issue 主链路的注入点：客户端构造失败 /
// DNS provider 挂载失败 / 叶子证书解析失败 / 签发产物组装成功。账户注册
// 打 HTTPS fake 端点，Obtain 注入假 Resource，全程不出网。
func TestCovFinalAcmeIssueInject(t *testing.T) {
	db := testServerDB(t)
	fakeURL := covPctFakeACMEOrderReject(t)

	old := acmeDirectoryURLs[AcmeCADirectoryStaging]
	acmeDirectoryURLs[AcmeCADirectoryStaging] = fakeURL
	t.Cleanup(func() { acmeDirectoryURLs[AcmeCADirectoryStaging] = old })

	l := NewLegoIssuer(db, func() AcmeDNSConfig {
		return AcmeDNSConfig{Provider: "cloudflare", CloudflareToken: "cov-token"}
	}).(*legoIssuer)
	cert := &storage.AcmeCert{
		Domains:       []string{"cov-final.example.com"},
		PrimaryDomain: "cov-final.example.com",
		CADirectory:   AcmeCADirectoryStaging,
	}

	t.Run("lego client", func(t *testing.T) {
		old := legoNewClient
		legoNewClient = func(*lego.Config) (*lego.Client, error) { return nil, errors.New("cov-final: client") }
		t.Cleanup(func() { legoNewClient = old })
		if _, err := l.Issue(cert); err == nil || !strings.Contains(err.Error(), "lego client") {
			t.Fatalf("Issue: want lego client error, got %v", err)
		}
	})

	t.Run("set dns01", func(t *testing.T) {
		old := setDNS01Provider
		setDNS01Provider = func(*lego.Client, challenge.Provider) error { return errors.New("cov-final: dns01") }
		t.Cleanup(func() { setDNS01Provider = old })
		if _, err := l.Issue(cert); err == nil || !strings.Contains(err.Error(), "set dns01") {
			t.Fatalf("Issue: want set dns01 error, got %v", err)
		}
	})

	// 后两个子测试共用「Obtain 返回假 Resource」注入
	oldObtain := obtainCertificate
	obtainCertificate = func(*lego.Client, certificate.ObtainRequest) (*certificate.Resource, error) {
		return &certificate.Resource{
			Certificate:       covFinalPEMBlock("CERTIFICATE"),
			IssuerCertificate: covFinalPEMBlock("CERTIFICATE"),
			PrivateKey:        covFinalPEMBlock("EC PRIVATE KEY"),
		}, nil
	}
	t.Cleanup(func() { obtainCertificate = oldObtain })

	t.Run("leaf parse", func(t *testing.T) {
		old := pemLeafNotAfterFn
		pemLeafNotAfterFn = func([]byte) (time.Time, error) { return time.Time{}, errors.New("cov-final: parse") }
		t.Cleanup(func() { pemLeafNotAfterFn = old })
		if _, err := l.Issue(cert); err == nil || !strings.Contains(err.Error(), "parse issued certificate") {
			t.Fatalf("Issue: want parse error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		old := pemLeafNotAfterFn
		pemLeafNotAfterFn = func([]byte) (time.Time, error) { return expires, nil }
		t.Cleanup(func() { pemLeafNotAfterFn = old })

		res, err := l.Issue(cert)
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if res.CertificatePEM == "" || res.IssuerPEM == "" || res.PrivateKeyPEM == "" {
			t.Fatalf("IssuedResult empty: %+v", res)
		}
		if !res.ExpiresAt.Equal(expires) {
			t.Fatalf("ExpiresAt = %v, want %v", res.ExpiresAt, expires)
		}
	})
}

// TestCovFinalAcmeCryptoRand 锁定 Go 1.26 行为：ecdsa.GenerateKey 对恒
// 错误的熵源 reader 也返回成功（实测），generateAccountKeyPEM 的错误分支
// 因此在该 Go 版本下不可达——此测试仅固化该事实，防回归误判。
func TestCovFinalAcmeCryptoRand(t *testing.T) {
	old := cryptoRandReader
	cryptoRandReader = covFinalErrReader{}
	t.Cleanup(func() { cryptoRandReader = old })

	if _, err := generateAccountKeyPEM(); err != nil {
		t.Fatalf("generateAccountKeyPEM: Go 1.26 ecdsa.GenerateKey should ignore bad reader, got %v", err)
	}
}
