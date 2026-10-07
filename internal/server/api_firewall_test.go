package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestFirewallStatusForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{
			"available":      true,
			"backend":        "nftables",
			"totalRules":     float64(3),
			"truncated":      false,
			"backendVersion": "nft 1.0.9",
			"tables": []interface{}{
				map[string]interface{}{
					"family": "inet",
					"name":   "filter",
					"chains": []interface{}{
						map[string]interface{}{
							"name":   "input",
							"policy": "accept",
							"rules": []interface{}{
								map[string]interface{}{
									"handle":         float64(12),
									"text":           "tcp dport 9000 accept",
									"packets":        float64(123),
									"bytes":          float64(4567),
									"ownedByCockpit": false,
								},
							},
						},
					},
				},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentFirewallAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/firewall/status", nil), "a1/firewall/status")
	if rec.Code != http.StatusOK || gotMethod != "firewall.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var resp struct {
		Available  bool   `json:"available"`
		Backend    string `json:"backend"`
		TotalRules int    `json:"totalRules"`
		Tables     []struct {
			Family string `json:"family"`
			Name   string `json:"name"`
			Chains []struct {
				Name   string `json:"name"`
				Policy string `json:"policy"`
				Rules  []struct {
					Handle int    `json:"handle"`
					Text   string `json:"text"`
				} `json:"rules"`
			} `json:"chains"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if !resp.Available || resp.Backend != "nftables" || resp.TotalRules != 3 {
		t.Fatalf("meta = %+v", resp)
	}
	if len(resp.Tables) != 1 || resp.Tables[0].Family != "inet" || resp.Tables[0].Name != "filter" {
		t.Fatalf("tables = %+v", resp.Tables)
	}
	ch := resp.Tables[0].Chains[0]
	if ch.Name != "input" || ch.Policy != "accept" || len(ch.Rules) != 1 ||
		ch.Rules[0].Handle != 12 || ch.Rules[0].Text != "tcp dport 9000 accept" {
		t.Fatalf("chain = %+v", ch)
	}
}

func TestFirewallRouting(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	// 非 GET 拒绝
	rec := httptest.NewRecorder()
	s.handleAgentFirewallAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/firewall/status", nil), "a1/firewall/status")
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST status: code = %d, want 404", rec.Code)
	}
	// 未知子路径
	rec = httptest.NewRecorder()
	s.handleAgentFirewallAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/firewall/other", nil), "a1/firewall/other")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub: code = %d, want 404", rec.Code)
	}
	// 畸形 rest（无 /firewall/ 切分点）
	rec = httptest.NewRecorder()
	s.handleAgentFirewallAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/firewall", nil), "a1/firewall")
	if rec.Code != http.StatusNotFound {
		t.Errorf("malformed rest: code = %d, want 404", rec.Code)
	}
	// agent 离线
	rec = httptest.NewRecorder()
	s.handleAgentFirewallAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/ghost/firewall/status", nil), "ghost/firewall/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline agent: code = %d, want 503", rec.Code)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0", dispatched)
	}
}

func TestFirewallAgentErrorPassthrough(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "nft: Permission denied (you must be root)"
	})
	rec := httptest.NewRecorder()
	s.handleAgentFirewallAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/firewall/status", nil), "a1/firewall/status")
	if rec.Code != http.StatusBadGateway || rec.Body.String() == "" {
		t.Fatalf("error passthrough: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFirewallErrorFamilies 防火墙 API 的错误/兜底分支族（手法对齐
// TestCovAgentStatusAPIErrorFamilies / TestCovPctSmartAgentClosed）：
// 空 agentID/子路径守卫、transport 失败（agent 已 Close）、RPC 解码失败、
// 空错误消息兜底、success 无 data 的 {} 回退，外加 api.go 分发行的全路由命中
func TestFirewallErrorFamilies(t *testing.T) {
	s := covNewServer(t)
	okEmpty := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success"}
	}
	errEmpty := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error"}
	}
	badResp := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	}
	covFakeAgent(t, s, "fw-empty", []string{"firewall"}, errEmpty)
	covFakeAgent(t, s, "fw-bad", []string{"firewall"}, badResp)
	covFakeAgent(t, s, "fw-ok", []string{"firewall"}, okEmpty)

	// 路由守卫：切分后 agentID 或子路径为空 → 404
	rec := covRec()
	s.handleAgentFirewallAPI(rec, covReq(http.MethodGet, "/api/agents//firewall/status", nil), "/firewall/status")
	covWantCode(t, "empty agentID", rec, http.StatusNotFound)
	rec = covRec()
	s.handleAgentFirewallAPI(rec, covReq(http.MethodGet, "/api/agents/fw-ok/firewall/", nil), "fw-ok/firewall/")
	covWantCode(t, "empty sub", rec, http.StatusNotFound)

	// agent 已关闭 → CallAgent transport 失败 → 502 failed to reach agent
	closed := NewAgent("fw-closed", nil)
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	rec = covRec()
	s.handleAgentFirewallAPI(rec, covReq(http.MethodGet, "/api/agents/fw-closed/firewall/status", nil), "fw-closed/firewall/status")
	covWantCode(t, "closed agent", rec, http.StatusBadGateway)

	// RPC 响应形态非法 → 502 invalid response
	rec = covRec()
	s.handleAgentFirewallAPI(rec, covReq(http.MethodGet, "/api/agents/fw-bad/firewall/status", nil), "fw-bad/firewall/status")
	covWantCode(t, "bad rpc payload", rec, http.StatusBadGateway)

	// 错误消息为空 → "agent rejected the operation" 兜底
	rec = covRec()
	s.handleAgentFirewallAPI(rec, covReq(http.MethodGet, "/api/agents/fw-empty/firewall/status", nil), "fw-empty/firewall/status")
	covWantCode(t, "empty error msg", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "agent rejected the operation") {
		t.Errorf("fallback msg missing: %s", rec.Body.String())
	}

	// success 无 data → {} 回退
	rec = covRec()
	s.handleAgentFirewallAPI(rec, covReq(http.MethodGet, "/api/agents/fw-ok/firewall/status", nil), "fw-ok/firewall/status")
	covWantCode(t, "nil data", rec, http.StatusOK)
	if strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Errorf("nil data fallback = %s, want {}", rec.Body.String())
	}
}

// TestFirewallDispatchRoute api.go 分发行：全路由经认证中间件命中
// /firewall/ 子资源分发（registerRoutes 完整装配）
func TestFirewallDispatchRoute(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "fw-route", []string{"firewall"}, func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success", "data": map[string]interface{}{"available": true}}
	})
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodGet, "/api/agents/fw-route/firewall/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "dispatched firewall status", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "available") {
		t.Errorf("body = %s, want firewall status payload", rec.Body.String())
	}
}

func TestFirewallRBACPrefix(t *testing.T) {
	// firewall/ 前缀 → firewall 资源，GET 推导 firewall:read（D10）
	perms, governed := requiredPerms("/api/agents/a1/firewall/status", http.MethodGet)
	if !governed || len(perms) != 1 || perms[0] != "firewall:read" {
		t.Fatalf("requiredPerms = %v/%v, want [firewall:read]/true", perms, governed)
	}
	// 权限点必须落在 storage 合法集合内（roles UI 可授予）
	if !storage.PermissionValid("firewall:read") {
		t.Error("firewall:read not in storage permission set")
	}
}
