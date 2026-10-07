package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
