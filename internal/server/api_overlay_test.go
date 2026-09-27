package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestOverlayStatusForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{"tool": "zerotier", "status": "ok", "version": "1.14.2"},
				map[string]interface{}{"tool": "frp", "status": "unavailable"},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/status", nil), "a1/overlay/status")
	if rec.Code != http.StatusOK || gotMethod != "overlay.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var resp struct {
		Tools []struct {
			Tool    string `json:"tool"`
			Status  string `json:"status"`
			Version string `json:"version"`
		} `json:"tools"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Tools) != 2 || resp.Tools[0].Tool != "zerotier" || resp.Tools[1].Status != "unavailable" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestOverlayRouting(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	// 非 GET 拒绝
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/status", nil), "a1/overlay/status")
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST status: code = %d, want 404", rec.Code)
	}
	// 未知子路径
	rec = httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/other", nil), "a1/overlay/other")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub: code = %d, want 404", rec.Code)
	}
	// 畸形 rest
	rec = httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay", nil), "a1/overlay")
	if rec.Code != http.StatusNotFound {
		t.Errorf("malformed rest: code = %d, want 404", rec.Code)
	}
	// agent 离线
	rec = httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/ghost/overlay/status", nil), "ghost/overlay/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline agent: code = %d, want 503", rec.Code)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0", dispatched)
	}
}

func TestOverlayAgentErrorPassthrough(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "agent side boom"
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/status", nil), "a1/overlay/status")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", rec.Code)
	}
	if body := rec.Body.String(); body == "" || (json.Unmarshal([]byte(body), &map[string]interface{}{}) != nil && true) {
		t.Errorf("body should be json error, got %q", body)
	}
}

// ============ M3：网络加入/离开 + daemon/服务管理（overlay-design.md D21-D30） ============

// newOverlayServer 注册带 overlay capability 的假 agent（含 identity 初值，
// 供 D26 registry 就地更新断言用）
func newOverlayServer(t *testing.T, handler func(method string, params map[string]interface{}) (interface{}, string)) (*Server, *Agent) {
	t.Helper()
	s := newBackupTestServer(t)
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{
		Type: "overlay",
		Metadata: map[string]interface{}{
			"identity": map[string]interface{}{"zerotier": map[string]interface{}{"networks": []interface{}{}}},
			"keepme":   "untouched",
		},
	}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			method, _ := reqMsg.Payload["method"].(string)
			params, _ := reqMsg.Payload["params"].(map[string]interface{})
			var data interface{}
			var rpcErr string
			if handler != nil {
				data, rpcErr = handler(method, params)
			}
			payload := map[string]interface{}{"status": "success", "data": data}
			if rpcErr != "" {
				payload = map[string]interface{}{"status": "error", "error": rpcErr}
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, payload)
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent.Close() })
	return s, agent
}

func TestOverlayDaemonForward(t *testing.T) {
	var gotMethod string
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{"tool": "zerotier", "installed": true, "unitExists": true, "active": true},
				map[string]interface{}{"tool": "tailscale", "installed": false, "missingGuide": "install me"},
			},
		}, ""
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/daemon", nil), "a1/overlay/daemon")
	if rec.Code != http.StatusOK || gotMethod != "overlay.daemon" {
		t.Fatalf("daemon: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var resp struct {
		Tools []struct {
			Tool      string `json:"tool"`
			Installed bool   `json:"installed"`
			Unit      string `json:"unit"`
		} `json:"tools"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Tools) != 2 || !resp.Tools[0].Installed || resp.Tools[1].Installed {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestOverlayJoinUpdatesRegistryAndAudit(t *testing.T) {
	const ztNet = "8056c2e21c000001"
	var gotMethod string
	var gotParams map[string]interface{}
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod, gotParams = method, params
		return map[string]interface{}{
			"status":   map[string]interface{}{"tools": []interface{}{}},
			"identity": map[string]interface{}{"zerotier": map[string]interface{}{"networks": []interface{}{ztNet}}},
		}, ""
	})

	body := `{"tool":"zerotier"}`
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/networks/"+ztNet+"/join", strings.NewReader(body)), "a1/overlay/networks/"+ztNet+"/join")
	if rec.Code != http.StatusOK {
		t.Fatalf("join code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotMethod != "overlay.join" {
		t.Errorf("method = %q, want overlay.join", gotMethod)
	}
	if gotParams["tool"] != "zerotier" || gotParams["networkId"] != ztNet {
		t.Errorf("params = %v", gotParams)
	}

	// D26：identity 载荷 → registry 内该 agent 的 metadata.identity 就地更新，
	// 其余 metadata 键保留
	agent, ok := s.registry.Get("a1")
	if !ok {
		t.Fatal("agent gone from registry")
	}
	cap := agent.GetCapability("overlay")
	identity, _ := cap.Metadata["identity"].(map[string]interface{})
	zt, _ := identity["zerotier"].(map[string]interface{})
	networks, _ := zt["networks"].([]interface{})
	if len(networks) != 1 || networks[0] != ztNet {
		t.Errorf("registry identity not updated: %v", cap.Metadata)
	}
	if cap.Metadata["keepme"] != "untouched" {
		t.Errorf("metadata 既有键应保留: %v", cap.Metadata)
	}

	// 审计 overlay_join（D29）
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "overlay_join"})
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit logs = %d, %v", len(logs), err)
	}
	if logs[0].Resource != "overlay_network" || logs[0].ResourceID != "a1/zerotier/"+ztNet || logs[0].Status != "success" {
		t.Errorf("audit row = %+v", logs[0])
	}
}

func TestOverlayLeaveNoIdentityKeepsRegistry(t *testing.T) {
	var gotMethod string
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		// 不携带 identity——server 应跳过 registry 更新（D26 口径）
		return map[string]interface{}{"status": map[string]interface{}{}}, ""
	})
	s.handleAgentOverlayAPI(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/networks/tailnet-name.ts.net/leave", strings.NewReader(`{"tool":"tailscale"}`)),
		"a1/overlay/networks/tailnet-name.ts.net/leave")
	if gotMethod != "overlay.leave" {
		t.Fatalf("method = %q, want overlay.leave", gotMethod)
	}
	agent, _ := s.registry.Get("a1")
	cap := agent.GetCapability("overlay")
	identity := cap.Metadata["identity"].(map[string]interface{})
	if _, hasTS := identity["tailscale"]; hasTS {
		t.Errorf("identity 载荷缺席时不得改写 registry: %v", cap.Metadata)
	}
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "overlay_leave"})
	if err != nil || len(logs) != 1 || logs[0].ResourceID != "a1/tailscale/tailnet-name.ts.net" {
		t.Fatalf("audit = %d, %v", len(logs), err)
	}
}

func TestOverlayNetworkValidationMatrix(t *testing.T) {
	dispatched := 0
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})
	const ztNet = "8056c2e21c000001"
	cases := []struct {
		name, netID, verb, body string
		wantCode                int
	}{
		{"unknown tool", ztNet, "join", `{"tool":"wireguard"}`, http.StatusBadRequest},
		{"empty tool", ztNet, "join", `{}`, http.StatusBadRequest},
		{"zt id not hex", "xyz", "join", `{"tool":"zerotier"}`, http.StatusBadRequest},
		{"zt id too short", "8056c2e21c00000", "join", `{"tool":"zerotier"}`, http.StatusBadRequest},
		{"zt id uppercase", "8056C2E21C000001", "leave", `{"tool":"zerotier"}`, http.StatusBadRequest},
		{"ts bad tailnet", "My Tailnet", "join", `{"tool":"tailscale"}`, http.StatusBadRequest},
		{"ts single label", "mytailnet", "join", `{"tool":"tailscale"}`, http.StatusBadRequest},
		{"bad verb", ztNet, "restart", `{"tool":"zerotier"}`, http.StatusNotFound},
		{"extra path segments", ztNet + "/extra", "join", `{"tool":"zerotier"}`, http.StatusNotFound},
		{"malformed body", ztNet, "join", `{bad json`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		// 真实链路里 mux 传入的 rest 已 URL 解码（空格等字符以原形到达
		// handler），测试用 PathEscape 构造合法请求、rest 传原串
		segs := strings.Split(c.netID, "/")
		for i := range segs {
			segs[i] = url.PathEscape(segs[i])
		}
		path := "/api/agents/a1/overlay/networks/" + strings.Join(segs, "/") + "/" + c.verb
		s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(c.body)), "a1/overlay/networks/"+c.netID+"/"+c.verb)
		if rec.Code != c.wantCode {
			t.Errorf("%s: code = %d, want %d (body=%s)", c.name, rec.Code, c.wantCode, rec.Body.String())
		}
	}
	if dispatched != 0 {
		t.Errorf("校验失败不得转发到 agent, dispatched = %d", dispatched)
	}
}

func TestOverlayNetworkAgentErrorNoAudit(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "zerotier-cli join: cannot connect to daemon"
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/networks/8056c2e21c000001/join", strings.NewReader(`{"tool":"zerotier"}`)), "a1/overlay/networks/8056c2e21c000001/join")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "cannot connect to daemon") {
		t.Errorf("agent 错误应透传给前端, body = %s", body)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, map[string]interface{}{"resource": "overlay_network"})
	if len(logs) != 0 {
		t.Errorf("失败的 join 不应记成功审计, logs = %v", logs)
	}
}

func TestOverlayServiceEndpoint(t *testing.T) {
	var gotMethod string
	var gotParams map[string]interface{}
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod, gotParams = method, params
		return map[string]interface{}{"tool": "tailscale", "unit": "tailscaled.service", "action": "stop"}, ""
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/service", strings.NewReader(`{"tool":"tailscale","action":"stop"}`)), "a1/overlay/service")
	if rec.Code != http.StatusOK || gotMethod != "overlay.service" {
		t.Fatalf("service: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	if gotParams["tool"] != "tailscale" || gotParams["action"] != "stop" {
		t.Errorf("params = %v", gotParams)
	}
	// 审计 resourceID 用 agent 回带的 unit 名（白名单单一事实源在 agent 端）
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "service_toggle"})
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit = %d, %v", len(logs), err)
	}
	if logs[0].ResourceID != "a1/tailscaled.service" {
		t.Errorf("audit resourceID = %q, want a1/tailscaled.service", logs[0].ResourceID)
	}
}

func TestOverlayServiceValidation(t *testing.T) {
	dispatched := 0
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})
	cases := []struct {
		name, body string
		wantCode   int
	}{
		{"unknown tool", `{"tool":"wireguard","action":"start"}`, http.StatusBadRequest},
		{"unknown action", `{"tool":"zerotier","action":"restart"}`, http.StatusBadRequest},
		{"empty", `{}`, http.StatusBadRequest},
		{"malformed", `{`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/service", strings.NewReader(c.body)), "a1/overlay/service")
		if rec.Code != c.wantCode {
			t.Errorf("%s: code = %d, want %d", c.name, rec.Code, c.wantCode)
		}
	}
	if dispatched != 0 {
		t.Errorf("校验失败不得转发到 agent, dispatched = %d", dispatched)
	}
}

// ============ 转发失败与审计退路（覆盖率巡检补齐） ============

func TestOverlayDaemonAgentErrorNoWrite(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "agent daemon probe failed"
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/daemon", nil), "a1/overlay/daemon")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("daemon 转发失败: code = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "agent daemon probe failed") {
		t.Errorf("应透出 agent 错误, body = %s", rec.Body.String())
	}
}

func TestOverlayServiceAgentErrorNoAudit(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "systemctl failed"
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/service", strings.NewReader(`{"tool":"tailscale","action":"stop"}`)), "a1/overlay/service")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("service 转发失败: code = %d, want 502", rec.Code)
	}
	// 未到成功不得记审计（写动作审计口径）
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "service_toggle"})
	if err != nil || len(logs) != 0 {
		t.Errorf("转发失败不得记审计, logs = %d, %v", len(logs), err)
	}
}

func TestOverlayServiceUnitFallbackAndAuditUser(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		// 响应不带 unit——审计 resourceID 退化用工具名（单一事实源缺席的退路）
		return map[string]interface{}{"tool": "zerotier", "action": "start"}, ""
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/service", strings.NewReader(`{"tool":"zerotier","action":"start"}`))
	req = req.WithContext(auth.ContextWithUser(req.Context(), "u1", "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, req, "a1/overlay/service")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "service_toggle"})
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit = %d, %v", len(logs), err)
	}
	if logs[0].ResourceID != "a1/zerotier" {
		t.Errorf("unit 缺席时 resourceID 应退化为工具名: %q", logs[0].ResourceID)
	}
	if logs[0].Username != "admin" {
		t.Errorf("带身份请求应记审计用户: %q", logs[0].Username)
	}
}

// TestAgentUpdateOverlayIdentity D26 的 registry 侧单测：copy-on-write
// 语义（旧 slice/map 不被改写）与无 overlay capability 时的静默忽略。
func TestAgentUpdateOverlayIdentity(t *testing.T) {
	agent := NewAgent("a2", nil)
	agent.Capabilities = []protocol.Capability{
		{Type: "backup"},
		{Type: "overlay", Metadata: map[string]interface{}{"keepme": "yes"}},
	}
	oldCaps := agent.GetCapabilities()

	newIdentity := map[string]interface{}{"tailscale": map[string]interface{}{"tailnet": "ts.net"}}
	agent.UpdateOverlayIdentity(newIdentity)

	caps := agent.GetCapabilities()
	if len(caps) != 2 {
		t.Fatalf("len(caps) = %d, want 2", len(caps))
	}
	overlay := caps[1]
	if overlay.Metadata["keepme"] != "yes" {
		t.Errorf("既有 metadata 键应保留: %v", overlay.Metadata)
	}
	if _, ok := overlay.Metadata["identity"]; !ok {
		t.Errorf("identity 未写入: %v", overlay.Metadata)
	}
	// copy-on-write：先前 GetCapabilities 交出的旧 slice 不得被改写
	if _, mutated := oldCaps[1].Metadata["identity"]; mutated {
		t.Error("旧 capability slice 的 metadata 被原地改写，存在 data race 风险")
	}
	if caps[0].Metadata != nil {
		t.Errorf("非 overlay capability 不应被动: %v", caps[0].Metadata)
	}

	// 无 overlay capability → 静默忽略（不新建 capability 项）
	bare := NewAgent("a3", nil)
	bare.Capabilities = []protocol.Capability{{Type: "backup"}}
	bare.UpdateOverlayIdentity(newIdentity)
	if got := bare.GetCapabilities(); len(got) != 1 || got[0].Type != "backup" {
		t.Errorf("无 overlay capability 时应忽略, caps = %v", got)
	}
}
