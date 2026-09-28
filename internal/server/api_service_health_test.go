package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// healthAgentBody 一条合法探针（cloudflared 首用例形态）
func healthAgentBody() map[string]interface{} {
	return map[string]interface{}{
		"probes": []interface{}{map[string]interface{}{
			"id": "cloudflared-ready", "type": "http",
			"target": "http://127.0.0.1:20241/ready",
			"heal":   true, "unit": "cloudflared.service",
		}},
		"whitelist": []interface{}{"cloudflared.service"},
	}
}

// manyProbes n 条最小合法探针（>32 上限校验用）
func manyProbes(n int) map[string]interface{} {
	probes := make([]interface{}, n)
	for i := range probes {
		probes[i] = map[string]interface{}{
			"id": fmt.Sprintf("p%02d", i), "type": "tcp", "target": "h:1"}
	}
	return map[string]interface{}{"probes": probes}
}

// doHealthAPI 经 serveAPI 的分发口径直调 handler（rest 与 path 的 /agents/ 后缀一致）
func doHealthAPI(s *Server, method, rest string, body interface{}) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		req = httptest.NewRequest(method, "/api/agents/"+rest, strings.NewReader(string(raw)))
	} else {
		req = httptest.NewRequest(method, "/api/agents/"+rest, nil)
	}
	rec := httptest.NewRecorder()
	s.handleAgentHealthAPI(rec, req, rest)
	return rec
}

// TestHealthConfigSaveValidation PUT 校验矩阵（D12 + D6 白名单门）
func TestHealthConfigSaveValidation(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"applied": 1}, ""
	})

	cases := []struct {
		name string
		body map[string]interface{}
		want string // 期望错误片段
	}{
		{"空 probes", map[string]interface{}{"probes": []interface{}{}}, "must not be empty"},
		{"非法 id", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "Bad_ID", "type": "tcp", "target": "h:1"}}}, "invalid id"},
		{"未知类型", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "icmp", "target": "h"}}}, "unsupported type"},
		{"http scheme", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "http", "target": "ftp://h/x"}}}, "http://"},
		{"interval 越界", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1", "intervalSec": 1}}}, "intervalSec"},
		{"heal 未进白名单", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "http", "target": "http://h/x",
				"heal": true, "unit": "rogue.service"}}}, "whitelist"},
		{"白名单条目非 unit", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1"}},
			"whitelist": []interface{}{"x.socket"}}, "*.service"},
		{"超 32 条探针", manyProbes(33), "too many probes"},
		{"重复 id", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1"},
			map[string]interface{}{"id": "p", "type": "tcp", "target": "h:2"}}}, "duplicate id"},
		{"expectStatus 越界", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "http", "target": "http://h/x", "expectStatus": 600}}}, "expectStatus"},
		{"tcp 空 target", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "tcp"}}}, "tcp target is required"},
		{"systemd target 非 unit", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "systemd", "target": "x"}}}, "*.service"},
		{"timeoutSec 越界", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1", "timeoutSec": 31}}}, "timeoutSec"},
		{"failThreshold 越界", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1", "failThreshold": 61}}}, "failThreshold"},
		{"heal 缺 unit", map[string]interface{}{"probes": []interface{}{
			map[string]interface{}{"id": "p", "type": "http", "target": "http://h/x", "heal": true}}}, "heal unit is required"},
		{"heal unit 非 unit 形状", map[string]interface{}{
			"probes": []interface{}{
				map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1", "heal": true, "unit": "x"}},
			"whitelist": []interface{}{"x.service"}}, "must be a *.service unit"},
		{"backoffWindowSec 越界", map[string]interface{}{
			"probes": []interface{}{
				map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1", "heal": true,
					"unit": "a.service", "backoffWindowSec": 59}},
			"whitelist": []interface{}{"a.service"}}, "backoffWindowSec"},
		{"maxRestartsInWindow 越界", map[string]interface{}{
			"probes": []interface{}{
				map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1", "heal": true,
					"unit": "a.service", "maxRestartsInWindow": 11}},
			"whitelist": []interface{}{"a.service"}}, "maxRestartsInWindow"},
	}
	for _, tc := range cases {
		rec := doHealthAPI(s, http.MethodPut, "a1/health", tc.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code=%d body=%s", tc.name, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: want %q in %s", tc.name, tc.want, rec.Body.String())
		}
	}
	// heal 未进白名单的文案要指引 heal=false 只告警（D6 中途检查点）
	rec := doHealthAPI(s, http.MethodPut, "a1/health", map[string]interface{}{
		"probes": []interface{}{map[string]interface{}{
			"id": "p", "type": "http", "target": "http://h/x", "heal": true, "unit": "rogue.service"}}})
	if !strings.Contains(rec.Body.String(), "heal=false") {
		t.Errorf("guidance missing: %s", rec.Body.String())
	}
	// 非法 JSON
	req := httptest.NewRequest(http.MethodPut, "/api/agents/a1/health", strings.NewReader("{nope"))
	rec2 := httptest.NewRecorder()
	s.handleAgentHealthAPI(rec2, req, "a1/health")
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("bad json: code=%d", rec2.Code)
	}
}

// TestHealthConfigSaveAndPush 保存落库 + 在线推送 + 审计（D3/D9）
func TestHealthConfigSaveAndPush(t *testing.T) {
	var gotMethod string
	var gotParams map[string]interface{}
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "service.health.config" {
			gotMethod = method
			gotParams = params
		}
		return map[string]interface{}{"applied": 1}, ""
	})

	rec := doHealthAPI(s, http.MethodPut, "a1/health", healthAgentBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("save: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Applied int  `json:"applied"`
		Pushed  bool `json:"pushed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resp: %v", err)
	}
	if resp.Applied != 1 || !resp.Pushed {
		t.Fatalf("resp: %+v", resp)
	}
	if gotMethod != "service.health.config" {
		t.Fatalf("push method = %q", gotMethod)
	}
	if _, ok := gotParams["probes"]; !ok {
		t.Fatalf("push params missing probes: %v", gotParams)
	}
	// 配置落库（含元数据）
	cfgRaw, err := s.db.GetSetting(ServiceHealthConfigPrefix + "a1")
	if err != nil || !strings.Contains(cfgRaw, "cloudflared-ready") {
		t.Fatalf("config stored: %q err=%v", cfgRaw, err)
	}
	// 审计 service_health_config
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionServiceHealthConfig})
	if err != nil || total != 1 {
		t.Fatalf("audit rows: %d err=%v", total, err)
	}
	if logs[0].ResourceID != "a1" {
		t.Fatalf("audit resourceID = %q", logs[0].ResourceID)
	}
}

// TestHealthOverviewOnlineAndOffline 在线实时拉取（drain=false）+ 离线灰态
// 回落（D8）；未注册 agent 的 PUT 保存成功但 pushed=false
func TestHealthOverviewOnlineAndOffline(t *testing.T) {
	s, agent := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "service.health.status" {
			if drain, _ := params["drain"].(bool); drain {
				return nil, "scan loop must not be the only caller of GET overview"
			}
			return map[string]interface{}{
				"states": map[string]interface{}{
					"cloudflared-ready": map[string]interface{}{
						"status": "fail", "consecutiveFails": 3, "lastError": "HTTP 502"}},
			}, ""
		}
		return map[string]interface{}{"applied": 1}, ""
	})

	rec := doHealthAPI(s, http.MethodPut, "a1/health", healthAgentBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	rec = doHealthAPI(s, http.MethodGet, "a1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("overview: %d", rec.Code)
	}
	var resp struct {
		Config *struct {
			Probes []struct{ ID string } `json:"probes"`
		} `json:"config"`
		States map[string]struct {
			Status string `json:"status"`
		} `json:"states"`
		Online bool `json:"online"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("overview body: %v", err)
	}
	if !resp.Online || len(resp.Config.Probes) != 1 || resp.States["cloudflared-ready"].Status != "fail" {
		t.Fatalf("overview: %+v", resp)
	}
	// 状态已入缓存
	if s.loadHealthStateCache("a1") == nil {
		t.Fatal("state cache missing")
	}

	// 离线：连接关闭后灰态回落缓存
	agent.Close()
	s.registry.Unregister("a1")
	rec = doHealthAPI(s, http.MethodGet, "a1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("offline overview: %d", rec.Code)
	}
	resp = struct {
		Config *struct {
			Probes []struct{ ID string } `json:"probes"`
		} `json:"config"`
		States map[string]struct {
			Status string `json:"status"`
		} `json:"states"`
		Online bool `json:"online"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("offline body: %v", err)
	}
	if resp.Online || resp.States["cloudflared-ready"].Status != "fail" {
		t.Fatalf("offline gray state: %+v", resp)
	}
}

// TestHealthProbeCheckForward 立即探测转发：成功透传、agent 错误透传、
// 离线 503（读语义不审计）
func TestHealthProbeCheckForward(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "service.health.now" {
			return nil, "unexpected " + method
		}
		if id, _ := params["id"].(string); id == "nope" {
			return nil, "unknown probe id \"nope\""
		}
		return map[string]interface{}{"probe": "p1", "status": "ok"}, ""
	})
	rec := doHealthAPI(s, http.MethodPost, "a1/health/probes/p1/check", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("check: %d %s", rec.Code, rec.Body.String())
	}
	rec = doHealthAPI(s, http.MethodPost, "a1/health/probes/nope/check", nil)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "unknown probe") {
		t.Fatalf("agent error passthrough: %d %s", rec.Code, rec.Body.String())
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	for _, l := range logs {
		if l.Action == audit.ActionServiceHealthConfig {
			t.Fatal("check is read-like and must not audit")
		}
	}
	// 离线 agent → 503
	rec = doHealthAPI(s, http.MethodPost, "ghost/health/probes/p1/check", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("offline check: %d", rec.Code)
	}
}

// TestHealthRoutePerms RBAC：health 面归 services 权限（D10）
func TestHealthRoutePerms(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         string
	}{
		{http.MethodGet, "/api/agents/a1/health", "services:read"},
		{http.MethodPut, "/api/agents/a1/health", "services:write"},
		{http.MethodPost, "/api/agents/a1/health/probes/p/check", "services:write"},
	} {
		perms, governed := requiredPerms(tc.path, tc.method)
		if !governed || len(perms) != 1 || perms[0] != tc.want {
			t.Errorf("%s %s: perms=%v governed=%v", tc.method, tc.path, perms, governed)
		}
	}
}

// TestHealthRouteDispatchOddShapes 路由分发的边界形状：未知子路径 404、
// 空 probeId 404、无 /health 后缀 404；无配置 agent 的概览返回空态
func TestHealthRouteDispatchOddShapes(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	for _, rest := range []string{"a1/health/wat", "a1/health/probes//check", "a1/health/probes/p/check/x"} {
		rec := doHealthAPI(s, http.MethodGet, rest, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: code=%d want 404", rest, rec.Code)
		}
	}
	// rest 里根本没有 /health（serveAPI 分发前的防御形状）
	rec := doHealthAPI(s, http.MethodGet, "bogus", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("bogus: code=%d want 404", rec.Code)
	}
	// 空 probeId 要按 POST 口径分发才能进 check 分支（GET 落 default 同样 404，
	// 但覆盖的是另一条边）
	rec = doHealthAPI(s, http.MethodPost, "a1/health/probes//check", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST empty probeId: code=%d want 404", rec.Code)
	}
	// 从未配置/未扫描过的 agent：概览 200 空态（online=false、states={}）
	rec = doHealthAPI(s, http.MethodGet, "ghost/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ghost overview: %d", rec.Code)
	}
	var resp struct {
		Online bool                   `json:"online"`
		States map[string]interface{} `json:"states"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ghost body: %v", err)
	}
	if resp.Online || len(resp.States) != 0 {
		t.Fatalf("ghost overview: %+v", resp)
	}
}

// TestHealthOverviewConfigCorrupt 库内配置损坏：概览 500（不静默吞配置错误）；
// 状态缓存损坏则按无缓存处理
func TestHealthOverviewConfigCorrupt(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", "{nope")
	rec := doHealthAPI(s, http.MethodGet, "a1/health", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt config: code=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = s.db.SetSetting(ServiceHealthStatePrefix+"a1", "{nope")
	if s.loadHealthStateCache("a1") != nil {
		t.Fatal("corrupt state cache must read as absent")
	}
}

// TestHealthConfigSaveBodyTooLarge PUT 请求体超限 413
func TestHealthConfigSaveBodyTooLarge(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	rec := doHealthAPI(s, http.MethodPut, "a1/health", map[string]interface{}{
		"probes": []interface{}{}, "filler": strings.Repeat("x", 20<<10)})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: code=%d", rec.Code)
	}
}

// TestHealthOverviewFetchFallbacks 在线但拉取失败的四形态均回落缓存灰态
// （D8）：RPC error、Data 标量、Data 不可解码、agent 通道已关
func TestHealthOverviewFetchFallbacks(t *testing.T) {
	cached := map[string]healthStateEntry{"p1": {Status: "ok"}}
	for _, tc := range []struct {
		name    string
		handler func(method string, params map[string]interface{}) (interface{}, string)
		close   bool
	}{
		{"rpc error", func(_ string, _ map[string]interface{}) (interface{}, string) {
			return nil, "boom"
		}, false},
		{"scalar data", func(_ string, _ map[string]interface{}) (interface{}, string) {
			return 42, ""
		}, false},
		{"undecodable data", func(_ string, _ map[string]interface{}) (interface{}, string) {
			return map[string]interface{}{"x": make(chan int)}, ""
		}, false},
		{"agent closed", func(_ string, _ map[string]interface{}) (interface{}, string) {
			return map[string]interface{}{"states": map[string]interface{}{}}, ""
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, agent := newOverlayServer(t, tc.handler)
			s.cacheHealthState("a1", cached)
			if tc.close {
				agent.Close() // Send 关闭但 LastSeen 新鲜：IsOnline 仍 true
			}
			rec := doHealthAPI(s, http.MethodGet, "a1/health", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("overview: %d %s", rec.Code, rec.Body.String())
			}
			var resp struct {
				Online bool `json:"online"`
				States map[string]struct {
					Status string `json:"status"`
				} `json:"states"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("body: %v", err)
			}
			if !resp.Online || resp.States["p1"].Status != "ok" {
				t.Fatalf("fallback: %+v", resp)
			}
		})
	}
}

// TestHealthMarshalSeamDefense healthJSONMarshal 注入失败：保存 500、概览
// 回落缓存、归集跳过、缓存写入静默放弃（四调用点共用一条注入，行为中性）
func TestHealthMarshalSeamDefense(t *testing.T) {
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{
			"states": map[string]interface{}{"cloudflared-ready": map[string]interface{}{"status": "ok"}}}, ""
	})
	// 缓存灰态先于注入种子（注入后 cacheHealthState 不再落库）
	s.cacheHealthState("a1", map[string]healthStateEntry{"p1": {Status: "ok"}})
	old := healthJSONMarshal
	healthJSONMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { healthJSONMarshal = old }()

	// ① PUT 保存：配置序列化失败 → 500（不落库不推送）
	rec := doHealthAPI(s, http.MethodPut, "a1/health", healthAgentBody())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}

	// ② GET 概览：agent 应答重编码失败 → 回落缓存灰态
	rec = doHealthAPI(s, http.MethodGet, "a1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("overview: %d", rec.Code)
	}
	var resp struct {
		Online bool `json:"online"`
		States map[string]struct {
			Status string `json:"status"`
		} `json:"states"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Online || resp.States["p1"].Status != "ok" {
		t.Fatalf("fallback: %+v", resp)
	}

	// ③ 归集：drain 重编码失败 → 跳过该 agent（缓存不被本轮覆盖）
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))
	s.scanServiceHealthOnce()
	cached := s.loadHealthStateCache("a1")
	if cached == nil || len(cached.States) != 1 {
		t.Fatalf("scan must skip on marshal failure, cache: %+v", cached)
	}

	// ④ cacheHealthState：marshal 失败静默放弃（灰态尽力而为）
	s.cacheHealthState("a2", map[string]healthStateEntry{"p": {Status: "ok"}})
	if s.loadHealthStateCache("a2") != nil {
		t.Fatal("a2 cache must stay absent")
	}
}

// TestHealthConfigSaveAuditUser 带认证上下文的保存：UpdatedBy/审计 username
// 落操作人（无人值守之外的人工动作要留名，D9）
func TestHealthConfigSaveAuditUser(t *testing.T) {
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"applied": 1}, ""
	})
	req := httptest.NewRequest(http.MethodPut, "/api/agents/a1/health",
		strings.NewReader(`{"probes":[{"id":"p","type":"tcp","target":"h:1"}]}`))
	req = req.WithContext(auth.ContextWithUser(req.Context(), "u1", "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleAgentHealthAPI(rec, req, "a1/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	cfgRaw, err := s.db.GetSetting(ServiceHealthConfigPrefix + "a1")
	if err != nil || !strings.Contains(cfgRaw, `"updatedBy":"admin"`) {
		t.Fatalf("updatedBy missing: %q %v", cfgRaw, err)
	}
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionServiceHealthConfig})
	if err != nil || len(logs) != 1 || logs[0].Username != "admin" {
		t.Fatalf("audit username: %+v %v", logs, err)
	}
}

// TestHealthConfigSaveDBFailure 落库失败：500（配置权威在 DB，写失败必须显式失败）
func TestHealthConfigSaveDBFailure(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	_ = s.db.Close()
	rec := doHealthAPI(s, http.MethodPut, "a1/health", healthAgentBody())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("db failure: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHealthConfigTooLargeSerialized 序列化后超 3500 字节：400（Setting.Value
// 4096 上限的余量口径，healthMaxConfigBytes）
func TestHealthConfigTooLargeSerialized(t *testing.T) {
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"applied": 1}, ""
	})
	rec := doHealthAPI(s, http.MethodPut, "a1/health", map[string]interface{}{
		"probes":    []interface{}{map[string]interface{}{"id": "p", "type": "tcp", "target": "h:1"}},
		"whitelist": []interface{}{strings.Repeat("a", 3600) + ".service"},
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "config too large") {
		t.Fatalf("too large: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHealthProbeCheckDecodeFailure check 转发的解码失败 502；status=error 且
// error 为空的形状兜底文案（本地 pump 直发原始 payload，绕过 handler 包装）
func TestHealthProbeCheckDecodeFailure(t *testing.T) {
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"x": make(chan int)}, "" // 解码失败
	})
	rec := doHealthAPI(s, http.MethodPost, "a1/health/probes/p/check", nil)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "invalid response") {
		t.Fatalf("decode failure: %d %s", rec.Code, rec.Body.String())
	}

	// 本地 pump：status=error 但 error 缺席 → 兜底文案
	agent2 := NewAgent("a2", nil)
	if err := s.registry.Register(agent2); err != nil {
		t.Fatalf("register a2: %v", err)
	}
	go func() {
		for reqMsg := range agent2.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": "error"})
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent2.Close() })
	rec = doHealthAPI(s, http.MethodPost, "a2/health/probes/p/check", nil)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "agent rejected the operation") {
		t.Fatalf("empty msg fallback: %d %s", rec.Code, rec.Body.String())
	}
}

// TestPushServiceHealthConfigOffline 保存时离线：配置落库不推送（D3 注册补推）
func TestPushServiceHealthConfigOffline(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	s.registry.Unregister("a1")
	rec := doHealthAPI(s, http.MethodPut, "a1/health", healthAgentBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("offline save: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Pushed bool `json:"pushed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resp: %v", err)
	}
	if resp.Pushed {
		t.Fatal("offline save must report pushed=false")
	}
	if s.pushServiceHealthConfig("a1") {
		t.Fatal("offline push must fail")
	}
	if s.pushServiceHealthConfig("never-configured") {
		t.Fatal("no config → no push")
	}
}

// TestHealthConfigSystemdUnitDefault systemd 探针的 heal unit 缺省退化用
// target 本身（单一事实源：unit 即被探测的 unit）
func TestHealthConfigSystemdUnitDefault(t *testing.T) {
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"applied": 1}, ""
	})
	rec := doHealthAPI(s, http.MethodPut, "a1/health", map[string]interface{}{
		"probes": []interface{}{map[string]interface{}{
			"id": "sshd-active", "type": "systemd", "target": "ssh.service"}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	cfgRaw, err := s.db.GetSetting(ServiceHealthConfigPrefix + "a1")
	if err != nil || !strings.Contains(cfgRaw, `"unit":"ssh.service"`) {
		t.Fatalf("unit default: %q %v", cfgRaw, err)
	}
}

// TestHealthServeAPIRoute 走完整 serveAPI 分派进 /agents/{id}/health 分流行
// （api.go 路由 case；RBAC 归 services:read，见 TestHealthRoutePerms）
func TestHealthServeAPIRoute(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)
	_, req := doAuthenticatedRequest(s, "GET", "/api/agents/ghost/health", nil)
	rec := callWithAuth(s, s.serveAPI, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("serveAPI health route: %d %s", rec.Code, rec.Body.String())
	}
}
