package server

// cov_round5_handlers_test.go 第五轮覆盖率：serveAPI 分发分支、ACME
// 下载/账户/自动部署、nas/smart/overlay 三族 agent API 的错误分支、
// service 单元文件限额、日志尾随的注册表分发与 streaming 探测。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// TestCovServeAPIRouting serveAPI 各分发分支命中（handler 内部行为已由
// 各族直测覆盖，这里只验路由落点）
func TestCovServeAPIRouting(t *testing.T) {
	s := covNewServer(t)
	get := func(target string) int {
		rec := covRec()
		s.serveAPI(rec, covReq(http.MethodGet, target, nil))
		return rec.Code
	}
	if c := get("/api/smart/config"); c != http.StatusOK {
		t.Errorf("smart config: %d", c)
	}
	if c := get("/api/nas/config"); c != http.StatusOK {
		t.Errorf("nas config: %d", c)
	}
	// 生产 mux 挂 StripPrefix("/api") 之下，serveAPI 收到的 URL 无 /api 前缀；
	// ddns/acme 的子分发从 r.URL.Path 二次剥前缀（剥掉 "/ddns" / "/acme"），
	// 直调须同形态
	if c := get("/ddns"); c != http.StatusOK {
		t.Errorf("ddns list: %d", c)
	}
	if c := get("/acme/certs"); c != http.StatusOK {
		t.Errorf("acme list: %d", c)
	}
	rec := covRec()
	s.serveAPI(rec, covReq(http.MethodPost, "/logs/search",
		strings.NewReader(`{"type":"systemd","source":"x.service"}`)))
	if rec.Code != http.StatusOK {
		t.Errorf("logs search: %d (%s)", rec.Code, rec.Body.String())
	}
	// /agents/ 下的四个子分发（services/nas/overlay/smart）
	if c := get("/agents/ghost/services/"); c != http.StatusServiceUnavailable {
		t.Errorf("agents services: %d", c)
	}
	if c := get("/agents/ghost/nas/"); c != http.StatusNotFound {
		t.Errorf("agents nas: %d", c)
	}
	if c := get("/agents/ghost/overlay/"); c != http.StatusNotFound {
		t.Errorf("agents overlay: %d", c)
	}
	if c := get("/agents/ghost/smart/"); c != http.StatusNotFound {
		t.Errorf("agents smart: %d", c)
	}
}

// TestCovAcmeDownloadParts 证书下载：part 三分支 + 空产物 404 + 方法/id/缺失
func TestCovAcmeDownloadParts(t *testing.T) {
	s := covNewServer(t)
	cert := &storage.AcmeCert{
		Domains: []string{"d.test"}, PrimaryDomain: "d.test", CADirectory: "staging",
		Status: "issued", CertificatePEM: "CERT", IssuerPEM: "ISS", PrivateKeyPEM: "KEY",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.db.CreateAcmeCert(cert); err != nil {
		t.Fatal(err)
	}
	empty := &storage.AcmeCert{Domains: []string{"e.test"}, PrimaryDomain: "e.test", CADirectory: "staging", Status: "pending"}
	if err := s.db.CreateAcmeCert(empty); err != nil {
		t.Fatal(err)
	}

	dl := func(id, part string) string {
		t.Helper()
		rec := covRec()
		s.handleAcmeDownload(rec, covReq(http.MethodGet, "/api/acme/certs/"+id+"/download?part="+part, nil), id)
		if rec.Code != http.StatusOK {
			t.Fatalf("download %s/%s: %d (%s)", id, part, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	if b := dl("1", "cert"); b != "CERT" {
		t.Errorf("cert part: %q", b)
	}
	if b := dl("1", "issuer"); b != "ISS" {
		t.Errorf("issuer part: %q", b)
	}
	if b := dl("1", "key"); b != "KEY" {
		t.Errorf("key part: %q", b)
	}
	// part 非法 / 未签发 / 方法 / 非法 id / 记录缺失
	for _, tc := range []struct {
		name, method, id, part string
		want                   int
	}{
		{"bad part", http.MethodGet, "1", "bogus", http.StatusBadRequest},
		{"not issued", http.MethodGet, "2", "cert", http.StatusNotFound},
		{"method", http.MethodPost, "1", "cert", http.StatusMethodNotAllowed},
		{"bad id", http.MethodGet, "abc", "cert", http.StatusNotFound},
		{"missing", http.MethodGet, "999", "cert", http.StatusNotFound},
	} {
		rec := covRec()
		s.handleAcmeDownload(rec, covReq(tc.method, "/api/acme/certs/"+tc.id+"/download?part="+tc.part, nil), tc.id)
		if rec.Code != tc.want {
			t.Errorf("%s: code = %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}

// TestCovAcmeAccountAPI 账户：未注册 GET / 首次 PUT 走 Save / 二次 PUT 走
// Update / 非法 email / bad json / 405
func TestCovAcmeAccountAPI(t *testing.T) {
	s := covNewServer(t)
	call := func(method, body string) *int {
		rec := covRec()
		req := covReq(method, "/api/acme/account", nil)
		if body != "" {
			req = covReq(method, "/api/acme/account", strings.NewReader(body))
		}
		s.handleAcmeAccountAPI(rec, req)
		return &rec.Code
	}
	if c := *call(http.MethodGet, ""); c != http.StatusOK {
		t.Fatalf("get unregistered: %d", c)
	}
	if c := *call(http.MethodPut, `{"email":"admin@example.com"}`); c != http.StatusOK {
		t.Fatalf("put first: %d", c)
	}
	if c := *call(http.MethodPut, `{"email":"root@example.com"}`); c != http.StatusOK {
		t.Fatalf("put second: %d", c)
	}
	rec := covRec()
	s.handleAcmeAccountAPI(rec, covReq(http.MethodGet, "/api/acme/account", nil))
	if !strings.Contains(rec.Body.String(), `"registered":true`) ||
		!strings.Contains(rec.Body.String(), "root@example.com") {
		t.Fatalf("get registered: %s", rec.Body.String())
	}
	if c := *call(http.MethodPut, `{"email":"not-an-email"}`); c != http.StatusBadRequest {
		t.Errorf("invalid email: %d", c)
	}
	if c := *call(http.MethodPut, `{`); c != http.StatusBadRequest {
		t.Errorf("bad json: %d", c)
	}
	if c := *call(http.MethodPatch, ""); c != http.StatusMethodNotAllowed {
		t.Errorf("405: %d", c)
	}
}

// TestCovAcmeDispatchMisc acme 分发 405/404 分支与无鉴权上下文的审计
func TestCovAcmeDispatchMisc(t *testing.T) {
	s := covNewServer(t)
	check := func(name, method, target string, want int) {
		t.Helper()
		rec := covRec()
		s.handleACME(rec, covReq(method, target, nil))
		if rec.Code != want {
			t.Errorf("%s: code = %d, want %d", name, rec.Code, want)
		}
	}
	check("certs 405", http.MethodPatch, "/acme/certs", http.StatusMethodNotAllowed)
	check("unknown 404", http.MethodGet, "/acme/bogus", http.StatusNotFound)
	check("cert sub 405", http.MethodPatch, "/acme/certs/1", http.StatusMethodNotAllowed)
	// 无用户上下文调用审计（username 空）；再经 Middleware 注入用户
	// 走 username 赋值分支（借一个合法 create 请求触发审计）
	s.auditAcme(covReq(http.MethodPost, "/acme/x", nil), "acme_test", "res", nil)
	rec := covCallAuth(s, s.handleACME, covAuthReq(http.MethodPost, "/acme/certs",
		strings.NewReader(`{"domains":["audit.test"],"caDirectory":"staging"}`), "u1", "admin", "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("authed create: %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestCovAcmeAutoDeploy 签发后自动部署：无目标直返 / 目标离线失败只记日志 /
// 在线 file-manager agent 两次 file.write 成功后清 LastDeployError
func TestCovAcmeAutoDeploy(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "dep-a", []string{"file-manager"}, func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success", "data": map[string]interface{}{}}
	})
	mk := func(id uint, agent string) *storage.AcmeCert {
		return &storage.AcmeCert{
			ID: id, Domains: []string{"d.test"}, PrimaryDomain: "d.test", CADirectory: "staging",
			Status: "issued", CertificatePEM: "CERT", PrivateKeyPEM: "KEY",
			DeployAgentID: agent, DeployCertPath: "/etc/d/cert.pem", DeployKeyPath: "/etc/d/key.pem",
		}
	}
	// 离线目标：deployAcmeCert 失败 → maybeDeploy 只记日志（LastDeployError 回写）
	ghost := mk(11, "ghost-dep")
	s.maybeDeployAcmeCert(ghost)
	if ghost.LastDeployError == "" {
		t.Fatal("ghost deploy should record LastDeployError")
	}
	// 在线目标：两次 write 成功
	okCert := mk(12, "dep-a")
	s.maybeDeployAcmeCert(okCert)
	if okCert.LastDeployError != "" {
		t.Fatalf("deploy should succeed: %q", okCert.LastDeployError)
	}
}

// TestCovAgentAPIFamilies nas/smart/overlay 三族共同错误分支：错误消息为空
// 的兜底、成功无 data 的 {} 回退、错误子路径与方法
func TestCovAgentAPIFamilies(t *testing.T) {
	s := covNewServer(t)
	okEmpty := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success"}
	}
	errEmpty := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error"}
	}
	covFakeAgent(t, s, "fam-nas", []string{"nas"}, errEmpty)
	covFakeAgent(t, s, "fam-smart", []string{"hardware-monitor"}, errEmpty)
	covFakeAgent(t, s, "fam-overlay", []string{"overlay"}, errEmpty)
	covFakeAgent(t, s, "fam-nas-ok", []string{"nas"}, okEmpty)
	covFakeAgent(t, s, "fam-smart-ok", []string{"hardware-monitor"}, okEmpty)
	covFakeAgent(t, s, "fam-overlay-ok", []string{"overlay"}, okEmpty)

	// nas：错误消息为空兜底 + 成功无 data
	rec := covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/fam-nas/nas/status", nil), "fam-nas/nas/status")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "agent rejected the operation") {
		t.Errorf("nas empty msg: %d %s", rec.Code, rec.Body.String())
	}
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/fam-nas-ok/nas/status", nil), "fam-nas-ok/nas/status")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Errorf("nas nil data: %d %s", rec.Code, rec.Body.String())
	}
	// nas：错误子路径 / 错误方法 / 离线 agent
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/fam-nas/nas/bogus", nil), "fam-nas/nas/bogus")
	if rec.Code != http.StatusNotFound {
		t.Errorf("nas bad sub: %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodPost, "/api/agents/fam-nas/nas/status", nil), "fam-nas/nas/status")
	if rec.Code != http.StatusNotFound {
		t.Errorf("nas bad method: %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/ghost/nas/status", nil), "ghost/nas/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("nas offline: %d", rec.Code)
	}

	// smart：同款三分支
	rec = covRec()
	s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/fam-smart/smart/status", nil), "fam-smart/smart/status")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "agent rejected the operation") {
		t.Errorf("smart empty msg: %d %s", rec.Code, rec.Body.String())
	}
	rec = covRec()
	s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/fam-smart-ok/smart/status", nil), "fam-smart-ok/smart/status")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Errorf("smart nil data: %d %s", rec.Code, rec.Body.String())
	}
	rec = covRec()
	s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/ghost/smart/status", nil), "ghost/smart/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("smart offline: %d", rec.Code)
	}

	// overlay：同款三分支
	rec = covRec()
	s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/fam-overlay/overlay/status", nil), "fam-overlay/overlay/status")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "agent rejected the operation") {
		t.Errorf("overlay empty msg: %d %s", rec.Code, rec.Body.String())
	}
	rec = covRec()
	s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/fam-overlay-ok/overlay/status", nil), "fam-overlay-ok/overlay/status")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Errorf("overlay nil data: %d %s", rec.Code, rec.Body.String())
	}
	rec = covRec()
	s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/ghost/overlay/status", nil), "ghost/overlay/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("overlay offline: %d", rec.Code)
	}

	// 全局配置端点：smart bad json / 405、nas bad json
	rec = covRec()
	s.handleSmartConfig(rec, covReq(http.MethodPut, "/api/smart/config", strings.NewReader(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("smart config bad json: %d", rec.Code)
	}
	rec = covRec()
	s.handleSmartConfig(rec, covReq(http.MethodPatch, "/api/smart/config", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("smart config 405: %d", rec.Code)
	}
	rec = covRec()
	s.handleNASConfig(rec, covReq(http.MethodPut, "/api/nas/config", strings.NewReader(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("nas config bad json: %d", rec.Code)
	}
}

// TestCovServiceUnitFileLimits service 单元文件：超限 413、非法名 400、
// 多段 404、成功保存与 data 为空的列表回退
func TestCovServiceUnitFileLimits(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "svc-b", []string{"services"}, func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success"}
	})

	// 列表：data 为空 → {}
	rec := covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodGet, "/api/agents/svc-b/services", nil), "svc-b/services")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Errorf("list nil data: %d %s", rec.Code, rec.Body.String())
	}

	// 超限内容 → 413
	big := fmt.Sprintf(`{"content":"%s"}`, strings.Repeat("a", unitFileBodyLimit+10))
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/svc-b/services/big.service/file", strings.NewReader(big)), "svc-b/services/big.service/file")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: %d", rec.Code)
	}

	// 非法 unit 名 / 多段路径 → 400/404
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/svc-b/services/x/file", strings.NewReader(`{"content":"y"}`)), "svc-b/services/bad!name/file")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad unit name: %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodGet, "/api/agents/svc-b/services/x/file", nil), "svc-b/services/a/b/file")
	if rec.Code != http.StatusNotFound {
		t.Errorf("multi segment: %d", rec.Code)
	}

	// 合法保存（service.unitsave 转发，data 为空 → {}）
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/svc-b/services/ok.service/file", strings.NewReader(`{"content":"[Unit]\n"}`)), "svc-b/services/ok.service/file")
	if rec.Code != http.StatusOK {
		t.Errorf("save: %d %s", rec.Code, rec.Body.String())
	}
}

// notFlusherW 只暴露 ResponseWriter 的包装（ResponseRecorder 自带 Flush，
// 需剥离才能触发 handler 的 streaming 探测分支）
type notFlusherW struct{ http.ResponseWriter }

// TestCovLogsFollowRegistryAndHandler 尾随注册表分发的静默丢弃分支与
// handler 的参数校验 / agent 不可达 / streaming 探测 / 真实推流与断开
func TestCovLogsFollowRegistryAndHandler(t *testing.T) {
	s := covNewServer(t)
	// 未知 followId：data/close 静默丢弃
	s.HandleLogsFollowData("logs:nope", []byte("x"))
	s.HandleLogsFollowClose("logs:nope", "done")

	// bad json / 非法参数
	rec := covRec()
	s.handleAgentLogsFollow(rec, covReq(http.MethodPost, "/api/agents/ghost/logs/follow", strings.NewReader(`{`)), "ghost")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentLogsFollow(rec, covReq(http.MethodPost, "/api/agents/ghost/logs/follow",
		strings.NewReader(`{"type":"systemd","source":"bad!name"}`)), "ghost")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid query: %d", rec.Code)
	}
	// agent 不可达 → 502 且注册表清理
	rec = covRec()
	s.handleAgentLogsFollow(rec, covReq(http.MethodPost, "/api/agents/ghost/logs/follow",
		strings.NewReader(`{"type":"systemd","source":"x.service","tail":50}`)), "ghost")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("ghost follow: %d", rec.Code)
	}

	okResp := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success", "data": map[string]interface{}{}}
	}
	// agent 应答成功但 writer 非 Flusher → 500 streaming unsupported
	covFakeAgent(t, s, "flw", []string{"logs"}, okResp)
	rec = covRec()
	s.handleAgentLogsFollow(&notFlusherW{rec}, covReq(http.MethodPost, "/api/agents/flw/logs/follow",
		strings.NewReader(`{"type":"systemd","source":"x.service","tail":50}`)), "flw")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "streaming unsupported") {
		t.Errorf("streaming: %d %s", rec.Code, rec.Body.String())
	}

	// 真实流式：agent 应答成功 → 推一行 → 客户端断开（ctx 取消）收尾
	covFakeAgent(t, s, "flw2", []string{"logs"}, okResp)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/agents/flw2/logs/follow",
		strings.NewReader(`{"type":"systemd","source":"x.service","tail":50}`)).WithContext(ctx)
	rec = covRec()
	done := make(chan struct{})
	go func() {
		s.handleAgentLogsFollow(rec, req, "flw2")
		close(done)
	}()
	var f *logsFollower
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logsFollowersMu.Lock()
		for _, v := range logsFollowers {
			if v.agentID == "flw2" {
				f = v
			}
		}
		logsFollowersMu.Unlock()
		if f != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f == nil {
		t.Fatal("follower not registered")
	}
	f.ch <- "hello-follow-line"
	time.Sleep(50 * time.Millisecond) // 等 pump 写帧
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after ctx cancel")
	}
	if !strings.Contains(rec.Body.String(), "hello-follow-line") {
		t.Errorf("streamed body missing line: %q", rec.Body.String())
	}
}
