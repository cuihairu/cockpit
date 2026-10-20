package server

// api_oob_test.go 带外动作端点测试：fake agent 应答 oob.wol，断言转发参数、
// 透传数据与审计落库；无真实 UDP 发送（发送在 agent 侧 provider，已单测）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// oobFakeAgent 注册一个带 oob capability 的 fake agent。
func oobFakeAgent(t *testing.T, s *Server, agentID string, handler func(string, map[string]interface{}) (interface{}, string)) {
	t.Helper()
	withFakeBackupAgentCap(t, s, agentID,
		[]protocol.Capability{{Type: "oob", Version: "1"}}, handler)
}

func oobWakeBody(extra string) string {
	return `{"agent_id":"a1","mac":"00:11:22:aa:bb:cc"` + extra + `}`
}

// ============ registerOobAPI / handleOobAPI 路由 ============

func TestCovRegisterOobAPI(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerOobAPI(mux)

	// 无认证 → 401（auth 中间件拦截）
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodPost, "/api/oob/wol", strings.NewReader(`{}`)))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	authReq := func(body string) *http.Request {
		req := covReq(http.MethodPost, "/api/oob/wol", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	// 带认证 → 正常分发（坏 body → 400）
	rec = covRec()
	mux.ServeHTTP(rec, authReq(`not-json`))
	covWantCode(t, "with auth bad body", rec, http.StatusBadRequest)

	// 带认证的成功路径：审计 username 取登录用户（非 unknown）
	oobFakeAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"sent": 102, "target": "192.168.1.255:9"}, ""
	})
	rec = covRec()
	mux.ServeHTTP(rec, authReq(oobWakeBody(`}`)))
	covWantCode(t, "with auth wake", rec, http.StatusOK)
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobWol})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "admin" {
		t.Fatalf("audit = %+v, want username admin", logs)
	}
}

// TestOobAPIDispatch 未知子路径与非 POST 一律 404。
func TestOobAPIDispatch(t *testing.T) {
	s := newTestServerWithDB(t)
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"unknown action", http.MethodPost, "/api/oob/ping"},
		{"get wol", http.MethodGet, "/api/oob/wol"},
		{"deep path", http.MethodPost, "/api/oob/wol/extra"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleOobAPI(rec, covReq(c.method, c.path, strings.NewReader(`{}`)))
			covWantCode(t, c.name, rec, http.StatusNotFound)
		})
	}
}

// ============ handleOobWake ============

func TestOobWakeValidation(t *testing.T) {
	s := newTestServerWithDB(t)
	oobFakeAgent(t, s, "a1", nil)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `nope`, http.StatusBadRequest},
		{"missing both", `{}`, http.StatusBadRequest},
		{"missing mac", `{"agent_id":"a1"}`, http.StatusBadRequest},
		{"missing agent", `{"mac":"00:11:22:aa:bb:cc"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol", strings.NewReader(c.body)))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

func TestOobWakeAgentOffline(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := covRec()
	s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol",
		strings.NewReader(oobWakeBody(`}`))))
	covWantCode(t, "offline", rec, http.StatusServiceUnavailable)
}

// TestOobWakeCallAgentError agent 注册仍在但连接已断 → CallAgent 失败 → 502；
// body 同时带 target，覆盖非空可选字段的透传分支。
func TestOobWakeCallAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "oob"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	agent.Close()

	rec := covRec()
	s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol",
		strings.NewReader(oobWakeBody(`,"target":"255.255.255.255:9"}`))))
	covWantCode(t, "closed agent", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestOobWakeRequiresOobCapability(t *testing.T) {
	s := newTestServerWithDB(t)
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup"}}, nil) // 无 oob capability
	rec := covRec()
	s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol",
		strings.NewReader(oobWakeBody(`}`))))
	covWantCode(t, "no oob cap", rec, http.StatusNotFound)
}

// TestOobWakeForwardsRPC 成功路径：参数原样转发（可选字段只传非空），
// agent 数据透传，审计落库记 MAC 与 agent 回填的实际发送目标。
func TestOobWakeForwardsRPC(t *testing.T) {
	s := newTestServerWithDB(t)
	var gotMethod string
	var gotParams map[string]interface{}
	oobFakeAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod, gotParams = method, params
		return map[string]interface{}{"sent": 102, "target": "192.168.1.255:9", "mac": "00:11:22:aa:bb:cc"}, ""
	})

	rec := covRec()
	body := oobWakeBody(`,"iface":"br-lan","port":9}`)
	s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol", strings.NewReader(body)))
	covWantCode(t, "wake ok", rec, http.StatusOK)
	if gotMethod != "oob.wol" {
		t.Fatalf("method = %q, want oob.wol", gotMethod)
	}
	if gotParams["mac"] != "00:11:22:aa:bb:cc" || gotParams["iface"] != "br-lan" || gotParams["port"] != float64(9) {
		t.Fatalf("params = %v", gotParams)
	}
	if _, has := gotParams["target"]; has {
		t.Fatalf("empty target must not be forwarded: %v", gotParams)
	}
	if !strings.Contains(rec.Body.String(), `"target":"192.168.1.255:9"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobWol})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(logs))
	}
	if logs[0].ResourceID != "a1" || logs[0].Resource != audit.ResourceOob {
		t.Fatalf("audit = %+v", logs[0])
	}
	var details map[string]interface{}
	if err := json.Unmarshal([]byte(logs[0].Details), &details); err != nil {
		t.Fatalf("audit details not json: %v (%s)", err, logs[0].Details)
	}
	if details["mac"] != "00:11:22:aa:bb:cc" || details["target"] != "192.168.1.255:9" || details["sent"] != float64(102) {
		t.Fatalf("audit details = %v", details)
	}
}

// TestOobWakeAgentError agent 侧错误（参数非法等）→ 502 透传 agent 文案，
// 失败的唤醒不写审计。
func TestOobWakeAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	oobFakeAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "oob.wol: invalid mac address \"zz\""
	})
	rec := covRec()
	s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol",
		strings.NewReader(`{"agent_id":"a1","mac":"zz"}`)))
	covWantCode(t, "agent error", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "invalid mac") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobWol})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("failed wake must not be audited, got %d", len(logs))
	}
}

// TestOobWakeBadPayload 应答 status 非字符串的 payload，触发
// DecodeRPCResponse 失败分支 → 502。
func TestOobWakeBadPayload(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("bad1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "oob"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	go func() {
		for reqMsg := range agent.Send {
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": 123})
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent.Close() })

	rec := covRec()
	s.handleOobWake(rec, covReq(http.MethodPost, "/api/oob/wol",
		strings.NewReader(`{"agent_id":"bad1","mac":"00:11:22:aa:bb:cc"}`)))
	covWantCode(t, "bad payload", rec, http.StatusBadGateway)
}
