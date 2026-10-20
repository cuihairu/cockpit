package server

// api_oob_test.go 带外动作端点测试：fake agent 应答 oob.wol / oob.probe，
// 断言转发参数、透传数据与审计落库；无真实 UDP 发送与真实探测
// （发送与探测都在 agent 侧 provider，已单测）。

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

// ============ handleOobProbe ============

// TestOobProbeValidation 校验与转发链路的错误分支（镜像 WOL 同款）。
func TestOobProbeValidation(t *testing.T) {
	s := newTestServerWithDB(t)
	oobFakeAgent(t, s, "a1", nil)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `nope`, http.StatusBadRequest},
		{"missing both", `{}`, http.StatusBadRequest},
		{"missing ip", `{"agent_id":"a1"}`, http.StatusBadRequest},
		{"missing agent", `{"ip":"10.0.0.5"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe", strings.NewReader(c.body)))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

func TestOobProbeAgentOffline(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := covRec()
	s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe",
		strings.NewReader(`{"agent_id":"gone","ip":"10.0.0.5"}`)))
	covWantCode(t, "offline", rec, http.StatusServiceUnavailable)
}

func TestOobProbeRequiresOobCapability(t *testing.T) {
	s := newTestServerWithDB(t)
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup"}}, nil) // 无 oob capability
	rec := covRec()
	s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe",
		strings.NewReader(`{"agent_id":"a1","ip":"10.0.0.5"}`)))
	covWantCode(t, "no oob cap", rec, http.StatusNotFound)
}

// TestOobProbeCallAgentError 连接已断 → 502；body 同时带 ports/timeout，
// 覆盖非空可选字段的透传分支。
func TestOobProbeCallAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "oob"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	agent.Close()

	rec := covRec()
	s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe",
		strings.NewReader(`{"agent_id":"a1","ip":"10.0.0.5","ports":[22,80],"timeout":3}`)))
	covWantCode(t, "closed agent", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

// TestOobProbeForwardsRPC 成功路径：ip/ports/timeout 原样转发（可选字段只传
// 非空），agent 报告透传，审计落库记 IP 与端口清单。
func TestOobProbeForwardsRPC(t *testing.T) {
	s := newTestServerWithDB(t)
	var gotMethod string
	var gotParams map[string]interface{}
	oobFakeAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod, gotParams = method, params
		return map[string]interface{}{
			"ip":   "10.0.0.5",
			"arp":  map[string]interface{}{"verdict": "present", "detail": "arp complete aa:bb:cc:dd:ee:ff on br-lan"},
			"icmp": map[string]interface{}{"verdict": "unknown"},
			"tcp":  map[string]interface{}{"verdict": "present", "detail": "tcp 22 connected"},
			"at":   float64(1760000000),
		}, ""
	})

	rec := covRec()
	body := `{"agent_id":"a1","ip":"10.0.0.5","ports":[22,80],"timeout":3}`
	s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe", strings.NewReader(body)))
	covWantCode(t, "probe ok", rec, http.StatusOK)
	if gotMethod != "oob.probe" {
		t.Fatalf("method = %q, want oob.probe", gotMethod)
	}
	if gotParams["ip"] != "10.0.0.5" || gotParams["timeout"] != float64(3) {
		t.Fatalf("params = %v", gotParams)
	}
	ports, ok := gotParams["ports"].([]interface{})
	if !ok || len(ports) != 2 || ports[0] != float64(22) || ports[1] != float64(80) {
		t.Fatalf("ports = %v", gotParams["ports"])
	}
	if !strings.Contains(rec.Body.String(), `"verdict":"present"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobProbe})
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
	if details["ip"] != "10.0.0.5" {
		t.Fatalf("audit details = %v", details)
	}
	ports, ok = details["ports"].([]interface{})
	if !ok || len(ports) != 2 || ports[0] != float64(22) {
		t.Fatalf("audit ports = %v", details["ports"])
	}
}

// TestOobProbeOptionalFieldsOmitted 空 ports/timeout 不透传；探活动作经认证
// 走 mux 时审计 username 取登录用户。
func TestOobProbeOptionalFieldsOmitted(t *testing.T) {
	s := newTestServerWithDB(t)
	var gotParams map[string]interface{}
	oobFakeAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"ip": "10.0.0.5"}, ""
	})

	mux := http.NewServeMux()
	s.registerOobAPI(mux)
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodPost, "/api/oob/probe", strings.NewReader(`{"agent_id":"a1","ip":"10.0.0.5"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "probe minimal", rec, http.StatusOK)
	if _, has := gotParams["ports"]; has {
		t.Fatalf("empty ports must not be forwarded: %v", gotParams)
	}
	if _, has := gotParams["timeout"]; has {
		t.Fatalf("nil timeout must not be forwarded: %v", gotParams)
	}

	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobProbe})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "admin" {
		t.Fatalf("audit = %+v, want username admin", logs)
	}
	var details map[string]interface{}
	if err := json.Unmarshal([]byte(logs[0].Details), &details); err != nil {
		t.Fatal(err)
	}
	if _, has := details["ports"]; has {
		t.Fatalf("empty ports must not be audited: %v", details)
	}
}

// TestOobProbeAgentError agent 侧错误（参数非法等）→ 502 透传 agent 文案，
// 失败的探测不写审计。
func TestOobProbeAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	oobFakeAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "oob.probe: invalid ip \"999.999.1.1\""
	})
	rec := covRec()
	s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe",
		strings.NewReader(`{"agent_id":"a1","ip":"999.999.1.1"}`)))
	covWantCode(t, "agent error", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "invalid ip") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobProbe})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("failed probe must not be audited, got %d", len(logs))
	}
}

// TestOobProbeBadPayload 应答 status 非字符串的 payload，触发
// DecodeRPCResponse 失败分支 → 502。
func TestOobProbeBadPayload(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("bad2", nil)
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
	s.handleOobProbe(rec, covReq(http.MethodPost, "/api/oob/probe",
		strings.NewReader(`{"agent_id":"bad2","ip":"10.0.0.5"}`)))
	covWantCode(t, "bad payload", rec, http.StatusBadGateway)
}
