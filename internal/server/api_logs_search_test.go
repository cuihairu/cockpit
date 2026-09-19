package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ 跨机日志联邦检索（logs-design.md M3，D11-D16） ============

// withFakeLogsAgent 注册带 logs capability 的假 agent，handler 同步回 RPC 响应
// （withFakeBackupAgent 同款骨架，capability 换 logs）
func withFakeLogsAgent(t *testing.T, s *Server, agentID, hostname string,
	handler func(method string, params map[string]interface{}) (interface{}, string)) {

	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = hostname
	agent.Capabilities = []protocol.Capability{{Type: "logs"}}
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
	t.Cleanup(func() { close(agent.Send) })
}

// newLogsSearchTestServer 最小 Server（search 仅依赖 registry）
func newLogsSearchTestServer(t *testing.T) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Server{
		registry: NewRegistry(),
		audit:    audit.NewLogger(testServerDB(t)),
		notifier: notification.NewService(nil),
		ctx:      ctx,
	}
}

// postLogsSearch 发起检索请求并返回 recorder
func postLogsSearch(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/logs/search", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleLogsSearch(rec, req)
	return rec
}

// TestLogsSearchValidates 校验表驱动：复用 M1 规则 + 扇出收窄 + 405
func TestLogsSearchValidates(t *testing.T) {
	s := newLogsSearchTestServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad type", `{"type":"files","source":"nginx.service"}`, 400},
		{"empty source", `{"type":"systemd","source":""}`, 400},
		{"bad source chars", `{"type":"systemd","source":"a b"}`, 400},
		{"tail over search cap", `{"type":"systemd","source":"nginx.service","tail":501}`, 400},
		{"tail zero ok default", `{"type":"systemd","source":"nginx.service","tail":0}`, 200},
		{"grep newline", `{"type":"systemd","source":"nginx.service","grep":"a\nb"}`, 400},
		{"grep too long", `{"type":"systemd","source":"x","grep":"` + strings.Repeat("g", 257) + `"}`, 400},
		{"agents empty id", `{"type":"systemd","source":"x","agents":["a1",""]}`, 400},
		{"agents wrong shape", `{"type":"systemd","source":"x","agents":"a1"}`, 400},
		{"bad json", `{`, 400},
	}
	for _, tc := range cases {
		rec := postLogsSearch(t, s, tc.body)
		if rec.Code != tc.want {
			t.Errorf("%s: code = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	// GET 拒绝
	req := httptest.NewRequest(http.MethodGet, "/api/logs/search", nil)
	rec := httptest.NewRecorder()
	s.handleLogsSearch(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: code = %d, want 405", rec.Code)
	}
}

// TestLogsSearchFanoutAggregates 扇出聚合：多机结果按 ID 排序透传 + skipped 归因
func TestLogsSearchFanoutAggregates(t *testing.T) {
	s := newLogsSearchTestServer(t)

	withFakeLogsAgent(t, s, "b-2", "host-b", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "logs.query" {
			return nil, "unexpected method " + method
		}
		q, _ := params["query"].(map[string]interface{})
		// 假 agent 收内存 map（int）；真实链路经 JSON 序列化为 float64——两侧都认
		tail, _ := q["tail"].(float64)
		if q["source"] != "nginx.service" || (tail != 100 && q["tail"] != 100) {
			return nil, "unexpected query params"
		}
		return map[string]interface{}{"lines": "B1\nB2\n", "truncated": true}, ""
	})
	withFakeLogsAgent(t, s, "a-1", "host-a", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"lines": "A1\n", "truncated": false}, ""
	})
	// 无 logs capability 的 agent：默认全选时应被跳过
	plain := NewAgent("c-3", nil)
	plain.Hostname = "host-c"
	if err := s.registry.Register(plain); err != nil {
		t.Fatalf("register plain agent: %v", err)
	}

	// agents 指定子集 + 不存在的 id（offline 归因）
	body := `{"type":"systemd","source":"nginx.service","tail":100,"agents":["b-2","a-1","ghost"]}`
	rec := postLogsSearch(t, s, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Results []struct {
			AgentID   string `json:"agentId"`
			Hostname  string `json:"hostname"`
			OK        bool   `json:"ok"`
			Truncated bool   `json:"truncated"`
			Lines     string `json:"lines"`
		} `json:"results"`
		Skipped []struct {
			AgentID string `json:"agentId"`
			Reason  string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(out.Results))
	}
	// 按 agentId 稳定排序
	if out.Results[0].AgentID != "a-1" || out.Results[1].AgentID != "b-2" {
		t.Errorf("results order = %s,%s, want a-1,b-2", out.Results[0].AgentID, out.Results[1].AgentID)
	}
	if out.Results[0].Hostname != "host-a" || out.Results[0].Lines != "A1\n" || out.Results[0].Truncated {
		t.Errorf("a-1 result = %+v", out.Results[0])
	}
	if out.Results[1].Hostname != "host-b" || out.Results[1].Lines != "B1\nB2\n" || !out.Results[1].Truncated {
		t.Errorf("b-2 result = %+v", out.Results[1])
	}
	if len(out.Skipped) != 1 || out.Skipped[0].AgentID != "ghost" || out.Skipped[0].Reason != "offline" {
		t.Errorf("skipped = %+v, want ghost/offline", out.Skipped)
	}
}

// TestLogsSearchSkipsNoLogsCapability 默认全选时跳过无 capability 的 agent（no-logs）
func TestLogsSearchSkipsNoLogsCapability(t *testing.T) {
	s := newLogsSearchTestServer(t)
	withFakeLogsAgent(t, s, "a-1", "host-a", nil)
	plain := NewAgent("c-3", nil)
	plain.Hostname = "host-c"
	if err := s.registry.Register(plain); err != nil {
		t.Fatalf("register plain agent: %v", err)
	}

	rec := postLogsSearch(t, s, `{"type":"systemd","source":"nginx.service"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var out struct {
		Results []struct {
			AgentID string `json:"agentId"`
		} `json:"results"`
		Skipped []struct {
			AgentID string `json:"agentId"`
			Reason  string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].AgentID != "a-1" {
		t.Errorf("results = %+v, want only a-1", out.Results)
	}
	if len(out.Skipped) != 1 || out.Skipped[0].AgentID != "c-3" || out.Skipped[0].Reason != "no-logs" {
		t.Errorf("skipped = %+v, want c-3/no-logs", out.Skipped)
	}

	// 显式指定无 capability 的 id → no-logs 归因
	rec = postLogsSearch(t, s, `{"type":"systemd","source":"nginx.service","agents":["c-3"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 0 || len(out.Skipped) != 1 || out.Skipped[0].Reason != "no-logs" {
		t.Errorf("explicit no-logs: results=%d skipped=%+v", len(out.Results), out.Skipped)
	}
}

// TestLogsSearchAgentFailureDegrades 单机失败降级：ok=false + error，整体仍 200（D13）
func TestLogsSearchAgentFailureDegrades(t *testing.T) {
	s := newLogsSearchTestServer(t)
	withFakeLogsAgent(t, s, "a-1", "host-a", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "journalctl exited 1"
	})
	withFakeLogsAgent(t, s, "b-2", "host-b", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"lines": "B1\n", "truncated": false}, ""
	})

	rec := postLogsSearch(t, s, `{"type":"systemd","source":"nginx.service"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (partial failure must not 5xx)", rec.Code)
	}
	var out struct {
		Results []struct {
			AgentID string `json:"agentId"`
			OK      bool   `json:"ok"`
			Lines   string `json:"lines"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(out.Results))
	}
	byID := map[string]struct {
		OK    bool
		Lines string
		Error string
	}{}
	for _, r := range out.Results {
		byID[r.AgentID] = struct {
			OK    bool
			Lines string
			Error string
		}{r.OK, r.Lines, r.Error}
	}
	if a := byID["a-1"]; a.OK || a.Error == "" {
		t.Errorf("a-1 should degrade, got %+v", a)
	}
	if b := byID["b-2"]; !b.OK || b.Lines != "B1\n" || b.Error != "" {
		t.Errorf("b-2 should succeed, got %+v", b)
	}
}

// TestLogsSearchAllTargetsUnavailable 全部目标不可用 → 200 空结果（D13）
func TestLogsSearchAllTargetsUnavailable(t *testing.T) {
	s := newLogsSearchTestServer(t)
	plain := NewAgent("c-3", nil)
	if err := s.registry.Register(plain); err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := postLogsSearch(t, s, `{"type":"systemd","source":"nginx.service"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 empty", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"results":[]`) {
		t.Errorf("body = %s, want empty results", rec.Body.String())
	}
}

// TestLogsSearchInFlightLimit 在途闸门：两个在途占满后第三个请求 429（D12）。
// 放文件末尾执行：占住的名额随 CallAgent 30s 超时自动释放（goroutine 自灭，
// 本文件其后已无检索测试，不会串扰）
func TestLogsSearchInFlightLimit(t *testing.T) {
	s := newLogsSearchTestServer(t)
	// 不起读 goroutine 的 agent：请求送达但永不响应 → 在途挂起
	for _, id := range []string{"hang1", "hang2"} {
		hanging := NewAgent(id, nil)
		hanging.Capabilities = []protocol.Capability{{Type: "logs"}}
		if err := s.registry.Register(hanging); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	// 两个后台请求各占一个在途名额（CallAgent 30s 超时前一直挂住）
	done := make(chan struct{}, 2)
	for _, agents := range []string{`"agents":["hang1"]`, `"agents":["hang2"]`} {
		go func(agents string) {
			defer func() { done <- struct{}{} }()
			postLogsSearch(t, s, `{"type":"systemd","source":"x",`+agents+`}`)
		}(agents)
	}

	// 等两个请求都进入在途计数
	deadline := time.Now().Add(5 * time.Second)
	for logsSearchInFlight.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if logsSearchInFlight.Load() < 2 {
		t.Fatal("background requests never entered in-flight state")
	}

	rec := postLogsSearch(t, s, `{"type":"systemd","source":"nginx.service"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("code = %d, want 429", rec.Code)
	}
}
