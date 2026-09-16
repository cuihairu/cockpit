package server

// 杂项覆盖率补充测试：splitTarget / sendCloseToAgent / hasPrefix /
// password_reset_handlers / remote_audit（目标白名单与出口策略、远控审计）。

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ splitTarget（api_desktop.go 纯函数）============

func TestCovSplitTarget(t *testing.T) {
	cases := []struct {
		target   string
		wantHost string
		wantPort int
	}{
		{"host.example:22", "host.example", 22},
		{"127.0.0.1:8080", "127.0.0.1", 8080},
		{"no-port", "no-port", 0},
		{"", "", 0},
		{":22", ":22", 0},         // 冒号在开头 → idx=0 → 原样返回
		{"host:abc", "host", 0},   // 非数字端口 → 0
		{"a:b:22", "a:b", 22},     // 多冒号取最后一个
		{"[::1]:22", "[::1]", 22}, // IPv6 字面量
	}
	for _, c := range cases {
		host, port := splitTarget(c.target)
		if host != c.wantHost || port != c.wantPort {
			t.Errorf("splitTarget(%q) = (%q, %d), want (%q, %d)", c.target, host, port, c.wantHost, c.wantPort)
		}
	}
}

// ============ sendCloseToAgent（api_remote.go 纯转发分支）============

func TestCovSendCloseToAgent(t *testing.T) {
	s := covNewServer(t)
	agent := covFakeAgent(t, s, "a1", nil, nil) // respond=nil：无人消费 Send，直接读消息

	s.sendCloseToAgent(&TerminalSession{AgentID: "a1", ConnID: "cov-conn-1"})
	select {
	case msg := <-agent.Send:
		if msg.Type != protocol.MessageTypeProxyClose {
			t.Errorf("msg type = %v, want ProxyClose", msg.Type)
		}
		if msg.Payload["proxyId"] != "terminal-cov-conn-1" || msg.Payload["connId"] != "cov-conn-1" {
			t.Errorf("payload = %v", msg.Payload)
		}
		if msg.Payload["terminal"] != true {
			t.Errorf("terminal flag = %v", msg.Payload["terminal"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no ProxyClose message sent")
	}

	// agent 不在线 → 静默返回（不 panic）
	s.sendCloseToAgent(&TerminalSession{AgentID: "ghost", ConnID: "c2"})
}

// ============ hasPrefix（server.go 纯函数）============

func TestCovHasPrefix(t *testing.T) {
	cases := []struct {
		s, prefix string
		want      bool
	}{
		{"hello", "he", true},
		{"hello", "hello", true},
		{"hello", "hello!", false}, // 前缀比自身长
		{"hello", "x", false},
		{"", "", true},
		{"", "a", false},
		{"abc", "", true},
	}
	for _, c := range cases {
		if got := hasPrefix(c.s, c.prefix); got != c.want {
			t.Errorf("hasPrefix(%q, %q) = %v, want %v", c.s, c.prefix, got, c.want)
		}
	}
}

// ============ password_reset_handlers.go ============

func covSeedResetUser(t *testing.T, s *Server, email string) *storage.User {
	t.Helper()
	hash, err := storage.HashPassword("oldpass123")
	if err != nil {
		t.Fatal(err)
	}
	user := &storage.User{ID: "cov-reset-1", Username: "covreset", Password: hash, Email: email, Role: "user"}
	if err := s.db.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestCovForgotPasswordBranches(t *testing.T) {
	s := covNewServer(t)

	// 非 POST → 405
	rec := covRec()
	s.handleForgotPassword(rec, covReq(http.MethodGet, "/forgot", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// bad json → 400
	rec = covRec()
	s.handleForgotPassword(rec, covReq(http.MethodPost, "/forgot", strings.NewReader("not-json")))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// username 空 → 400
	rec = covRec()
	s.handleForgotPassword(rec, covReq(http.MethodPost, "/forgot", strings.NewReader(`{}`)))
	covWantCode(t, "empty username", rec, http.StatusBadRequest)

	// 用户不存在 → 200 通用消息（不泄露存在性）
	rec = covRec()
	s.handleForgotPassword(rec, covReq(http.MethodPost, "/forgot", strings.NewReader(`{"username":"nobody"}`)))
	covWantCode(t, "unknown user", rec, http.StatusOK)

	// 用户存在但无邮箱 → 200 提示联系管理员
	covSeedResetUser(t, s, "")
	rec = covRec()
	s.handleForgotPassword(rec, covReq(http.MethodPost, "/forgot", strings.NewReader(`{"username":"covreset"}`)))
	covWantCode(t, "no email", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "未绑定邮箱") {
		t.Errorf("no-email body = %s", rec.Body.String())
	}
}

// 有邮箱用户：返回脱敏地址；邮件在 goroutine 内发送（未配置 SMTP → 立即失败，不连网）
func TestCovForgotPasswordWithEmail(t *testing.T) {
	s := covNewServer(t)
	covSeedResetUser(t, s, "covreset@example.invalid")

	rec := covRec()
	s.handleForgotPassword(rec, covReq(http.MethodPost, "/forgot", strings.NewReader(`{"username":"covreset"}`)))
	covWantCode(t, "email sent response", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"masked_email"`) {
		t.Errorf("body missing masked email: %s", rec.Body.String())
	}
	// 等待异步发送 goroutine 结束（未配置 SMTP → 立即返回错误）
	time.Sleep(50 * time.Millisecond)
}

func TestCovResetPasswordBranches(t *testing.T) {
	s := covNewServer(t)
	covSeedResetUser(t, s, "covreset@example.invalid")
	token, _, err := auth.GenerateResetToken("cov-reset-1", "covreset@example.invalid")
	if err != nil {
		t.Fatal(err)
	}

	// 非 POST → 405
	rec := covRec()
	s.handleResetPassword(rec, covReq(http.MethodGet, "/reset", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// bad json → 400
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset", strings.NewReader("not-json")))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 空字段 → 400
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset", strings.NewReader(`{}`)))
	covWantCode(t, "empty fields", rec, http.StatusBadRequest)

	// 短密码 → 400
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"x","new_password":"123"}`)))
	covWantCode(t, "short password", rec, http.StatusBadRequest)

	// 无效 token → 401
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"bogus-token","new_password":"newpass123"}`)))
	covWantCode(t, "invalid token", rec, http.StatusUnauthorized)

	// 有效 token + 错验证码 → 401
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"`+token+`","new_password":"newpass123","code":"000000"}`)))
	covWantCode(t, "invalid code", rec, http.StatusUnauthorized)

	// 有效 token + 超长密码（bcrypt >72 字节）→ HashPassword err → 500
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"`+token+`","new_password":"`+strings.Repeat("x", 100)+`"}`)))
	covWantCode(t, "hash error", rec, http.StatusInternalServerError)

	// 有效 token + closed db → UpdatePassword err → 500
	s2 := covNewServer(t)
	covSeedResetUser(t, s2, "covreset@example.invalid")
	token2, _, err := auth.GenerateResetToken("cov-reset-1", "covreset@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	covCloseDB(t, s2)
	rec = covRec()
	s2.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"`+token2+`","new_password":"newpass123"}`)))
	covWantCode(t, "update error", rec, http.StatusInternalServerError)

	// 成功 → 200（带验证码双因子路径）
	token3, code3, err := auth.GenerateResetToken("cov-reset-1", "covreset@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"`+token3+`","new_password":"brandnew123","code":"`+code3+`"}`)))
	covWantCode(t, "reset ok", rec, http.StatusOK)

	// 已消费的 token 再次使用 → 401（一次性）
	rec = covRec()
	s.handleResetPassword(rec, covReq(http.MethodPost, "/reset",
		strings.NewReader(`{"token":"`+token3+`","new_password":"brandnew123"}`)))
	covWantCode(t, "consumed token", rec, http.StatusUnauthorized)
}

func TestCovVerifyResetCodeBranches(t *testing.T) {
	s := covNewServer(t)

	// 非 POST → 405
	rec := covRec()
	s.handleVerifyResetCode(rec, covReq(http.MethodGet, "/verify", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// bad json → 400
	rec = covRec()
	s.handleVerifyResetCode(rec, covReq(http.MethodPost, "/verify", strings.NewReader("not-json")))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 无效 token/code → valid:false（仍 200）
	rec = covRec()
	s.handleVerifyResetCode(rec, covReq(http.MethodPost, "/verify",
		strings.NewReader(`{"token":"bogus","code":"000000"}`)))
	covWantCode(t, "invalid", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"valid":false`) {
		t.Errorf("invalid body = %s", rec.Body.String())
	}

	// 有效 token + 匹配 code → valid:true
	token, code, err := auth.GenerateResetToken("cov-reset-1", "covreset@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s.handleVerifyResetCode(rec, covReq(http.MethodPost, "/verify",
		strings.NewReader(`{"token":"`+token+`","code":"`+code+`"}`)))
	covWantCode(t, "valid", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"valid":true`) {
		t.Errorf("valid body = %s", rec.Body.String())
	}
}

// ============ remote_audit.go：目标白名单与出口策略 ============

func covRemoteCfg(arbitrary bool, targets []string, egress []*config.RemoteEgressPolicy) *Server {
	return &Server{cfg: &config.Config{
		RemoteControl: &config.RemoteControlConfig{
			AllowArbitraryTarget: arbitrary,
			AllowedTargets:       targets,
			EgressPolicies:       egress,
		},
	}}
}

func TestCovValidateRemoteTarget(t *testing.T) {
	// host 空 → 拒绝
	s := &Server{}
	if allow, msg := s.validateRemoteTarget(""); allow || msg == "" {
		t.Errorf("empty host: (%v, %q)", allow, msg)
	}

	// cfg nil → 任意 host 拒绝
	if allow, _ := s.validateRemoteTarget("host.example"); allow {
		t.Error("nil cfg should deny")
	}

	// AllowArbitraryTarget → 全放行
	sa := covRemoteCfg(true, nil, nil)
	if allow, msg := sa.validateRemoteTarget("anything.example"); !allow || msg != "" {
		t.Errorf("arbitrary: (%v, %q)", allow, msg)
	}

	// AllowedTargets 显式 host / IP / CIDR 命中
	sl := covRemoteCfg(false, []string{"host.example", "10.0.0.0/8", "  192.168.1.5  "}, nil)
	for _, host := range []string{"host.example", "10.1.2.3", "192.168.1.5"} {
		if allow, msg := sl.validateRemoteTarget(host); !allow || msg != "" {
			t.Errorf("allow-list host %q: (%v, %q)", host, allow, msg)
		}
	}
	// 未命中（含带空格 host 不 trim 前不匹配显式项）
	for _, host := range []string{"other.example", "192.168.1.6"} {
		if allow, _ := sl.validateRemoteTarget(host); allow {
			t.Errorf("host %q should be denied", host)
		}
	}
}

func TestCovTargetMatchesAllowEntry(t *testing.T) {
	cases := []struct {
		host, entry string
		want        bool
	}{
		{"host.example", "host.example", true},
		{"host.example", "  host.example  ", true}, // entry 两端空白被 trim
		{"host.example", "", false},                // 空 entry
		{"10.1.2.3", "10.0.0.0/8", true},           // CIDR 命中
		{"11.1.2.3", "10.0.0.0/8", false},          // CIDR 不含
		{"host.example", "10.0.0.0/8", false},      // host 非 IP
		{"host.example", "not-a-cidr", false},      // entry 非 CIDR 且不等
		{"10.1.2.3", "10.1.2.3", true},             // 显式 IP
	}
	for _, c := range cases {
		if got := targetMatchesAllowEntry(c.host, c.entry); got != c.want {
			t.Errorf("targetMatchesAllowEntry(%q, %q) = %v, want %v", c.host, c.entry, got, c.want)
		}
	}
}

func TestCovMatchRemoteEgress(t *testing.T) {
	policy := &config.RemoteEgressPolicy{
		AgentID:        "a1",
		AllowedTargets: []string{"10.0.0.0/8", "db.internal"},
		AllowedPorts:   []int{22, 443, 0}, // 0 端口会被视图过滤
	}

	// 全局白名单校验失败 → nil + errMsg
	s := covRemoteCfg(false, nil, nil)
	if m, msg := s.matchRemoteEgress("a1", "blocked.example", 22); m != nil || msg == "" {
		t.Errorf("blocked host: (%v, %q)", m, msg)
	}

	// 无 egress 策略 → global-allow-list
	s2 := covRemoteCfg(true, nil, nil)
	m, msg := s2.matchRemoteEgress("a1", "any.example", 22)
	if m == nil || m.Mode != "global-allow-list" || msg != "" {
		t.Errorf("global allow: (%+v, %q)", m, msg)
	}
	if m.summary() != "global-allow-list" {
		t.Errorf("summary = %q", m.summary())
	}

	// egress 有策略但不匹配该 agent → nil + "agent not allowed"
	s3 := covRemoteCfg(true, nil, []*config.RemoteEgressPolicy{policy})
	if m, msg := s3.matchRemoteEgress("a2", "10.1.2.3", 22); m != nil || !strings.Contains(msg, "not allowed for remote egress") {
		t.Errorf("agent unmatched: (%v, %q)", m, msg)
	}

	// agent 匹配但 host 不在其 allowed_targets → nil
	if m, msg := s3.matchRemoteEgress("a1", "192.168.1.1", 22); m != nil || !strings.Contains(msg, "not allowed for agent") {
		t.Errorf("host denied: (%v, %q)", m, msg)
	}

	// host 允许但端口不在 allowed_ports → nil
	if m, msg := s3.matchRemoteEgress("a1", "10.1.2.3", 8080); m != nil || !strings.Contains(msg, "port") {
		t.Errorf("port denied: (%v, %q)", m, msg)
	}

	// 全匹配 → agent-egress
	m, msg = s3.matchRemoteEgress("a1", "10.1.2.3", 22)
	if m == nil || msg != "" || m.Mode != "agent-egress" || m.AgentID != "a1" || m.Port != 22 {
		t.Fatalf("agent egress: (%+v, %q)", m, msg)
	}
	if m.summary() != "agent:a1 port:22" {
		t.Errorf("summary = %q", m.summary())
	}

	// validateRemoteTargetForAgent 包装：allowed/denied
	if allow, _ := s3.validateRemoteTargetForAgent("a1", "db.internal", 443); !allow {
		t.Error("db.internal:443 should be allowed")
	}
	if allow, msg := s3.validateRemoteTargetForAgent("a1", "db.internal", 8080); allow || msg == "" {
		t.Errorf("db.internal:8080: (%v, %q)", allow, msg)
	}
}

func TestCovEgressPolicyView(t *testing.T) {
	// buildRemoteEgressPolicyViews：nil 策略跳过、port<=0 过滤、AgentID trim
	views := buildRemoteEgressPolicyViews([]*config.RemoteEgressPolicy{
		nil,
		{AgentID: "  a1  ", AllowedTargets: []string{"h"}, AllowedPorts: []int{22, 0, -1}},
	})
	if len(views) != 1 {
		t.Fatalf("views = %d, want 1", len(views))
	}
	v := views[0]
	if v.AgentID != "a1" {
		t.Errorf("agentID = %q", v.AgentID)
	}
	if len(v.AllowedPorts) != 1 {
		t.Errorf("allowedPorts = %v, want only 22", v.AllowedPorts)
	}
	if len(buildRemoteEgressPolicyViews(nil)) != 0 {
		t.Error("nil policies should build nil views")
	}

	// allowsTarget：nil receiver / 空列表 / 命中
	var nilView *remoteEgressPolicyView
	if nilView.allowsTarget("h") || nilView.allowsPort(22) {
		t.Error("nil view should deny everything")
	}
	if v.allowsTarget("other") {
		t.Error("unlisted host should be denied")
	}
	if !v.allowsTarget("h") {
		t.Error("listed host should be allowed")
	}
	// allowsPort：0/负端口拒绝、未列端口拒绝
	if v.allowsPort(0) || v.allowsPort(-1) || v.allowsPort(80) {
		t.Error("port rules failed")
	}
	if !v.allowsPort(22) {
		t.Error("port 22 should be allowed")
	}
	// 空 ports / 空 targets 视图
	empty := &remoteEgressPolicyView{AgentID: "x"}
	if empty.allowsTarget("h") || empty.allowsPort(22) {
		t.Error("empty view should deny")
	}

	// summary：nil / 未知 mode / agent-egress
	var nilMatch *remoteEgressMatch
	if nilMatch.summary() != "" {
		t.Error("nil summary should be empty")
	}
	if (&remoteEgressMatch{Mode: "bogus"}).summary() != "" {
		t.Error("unknown mode summary should be empty")
	}
	if (&remoteEgressMatch{Mode: "agent-egress", AgentID: "a9", Port: 2222}).summary() != "agent:a9 port:2222" {
		t.Error("agent-egress summary failed")
	}
}

func TestCovRejectTargetDenied(t *testing.T) {
	rec := covRec()
	rejectTargetDenied(rec, "no way")
	covWantCode(t, "reject", rec, http.StatusForbidden)
}

// ============ remote_audit.go：远控审计落库与失败分支 ============

func TestCovRemoteAuditBranches(t *testing.T) {
	details := &audit.RemoteSessionDetails{Protocol: "ssh", AgentID: "a1", Host: "h", Port: 22, Session: "cov-s1"}

	// audit nil → 直接返回
	bare := &Server{}
	bare.auditRemoteStart(audit.ActionRemoteStart, "1", "admin", "127.0.0.1", "ua", details)
	bare.auditRemoteFailure("1", "admin", "127.0.0.1", "ua", details)
	bare.auditRemoteEnd("1", "admin", "127.0.0.1", "ua", details)

	// details nil → 直接返回
	s := covNewServer(t)
	s.auditRemoteStart(audit.ActionRemoteStart, "1", "admin", "127.0.0.1", "ua", nil)
	s.auditRemoteFailure("1", "admin", "127.0.0.1", "ua", nil)
	s.auditRemoteEnd("1", "admin", "127.0.0.1", "ua", nil)

	// 正常落库（start / failure / end）
	s.auditRemoteStart(audit.ActionRemoteStart, "1", "admin", "127.0.0.1", "ua", details)
	s.auditRemoteFailure("1", "admin", "127.0.0.1", "ua", details)
	endDetails := &audit.RemoteSessionDetails{Protocol: "ssh", Session: "cov-s1", Duration: "1m", Reason: "done"}
	s.auditRemoteEnd("1", "admin", "127.0.0.1", "ua", endDetails)

	// closed db → LogRemoteSession err → printf 分支（不 panic 即可）
	s2 := covNewServer(t)
	covCloseDB(t, s2)
	s2.auditRemoteStart(audit.ActionRemoteStart, "1", "admin", "127.0.0.1", "ua", details)
	s2.auditRemoteFailure("1", "admin", "127.0.0.1", "ua", details)
	s2.auditRemoteEnd("1", "admin", "127.0.0.1", "ua", details)
}
