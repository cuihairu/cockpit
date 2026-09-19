package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestDriftConfigAPI(t *testing.T) {
	s := newBackupTestServer(t)

	// 默认值
	rec := httptest.NewRecorder()
	s.handleDriftConfig(rec, httptest.NewRequest(http.MethodGet, "/api/drift/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var cfg struct {
		ScanIntervalSeconds int `json:"scan_interval_seconds"`
		Min                 int `json:"min"`
		Max                 int `json:"max"`
	}
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.ScanIntervalSeconds != 1800 || cfg.Min != 0 || cfg.Max != 86400 {
		t.Fatalf("default config = %+v", cfg)
	}

	// 合法写入（含 0 = 关闭）
	for _, v := range []int{0, 300, 86400} {
		rec = httptest.NewRecorder()
		body := strings.NewReader(`{"scan_interval_seconds":` + strconv.Itoa(v) + `}`)
		s.handleDriftConfig(rec, httptest.NewRequest(http.MethodPut, "/api/drift/config", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %d: code=%d body=%s", v, rec.Code, rec.Body.String())
		}
		if got := s.GetDriftScanInterval(); got != v {
			t.Fatalf("PUT %d then GET = %d", v, got)
		}
	}

	// 越界拒绝
	rec = httptest.NewRecorder()
	s.handleDriftConfig(rec, httptest.NewRequest(http.MethodPut, "/api/drift/config",
		strings.NewReader(`{"scan_interval_seconds":-1}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT -1: code = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleDriftConfig(rec, httptest.NewRequest(http.MethodPut, "/api/drift/config",
		strings.NewReader(`{"scan_interval_seconds":90000}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT 90000: code = %d, want 400", rec.Code)
	}
}

// withFakeDriftAgent 注册带 drift capability 的假 agent 并挂 RPC 应答
func withFakeDriftAgent(t *testing.T, s *Server, agentID string,
	handler func(method string, params map[string]interface{}) (interface{}, string)) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	agent.Capabilities = []protocol.Capability{{Type: "drift"}}
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
}

func TestScanDriftOnceCreatesAlert(t *testing.T) {
	s := newBackupTestServer(t)
	var calls int
	withFakeDriftAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		if method != "drift.check" {
			t.Errorf("unexpected method %s", method)
		}
		return map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"kind": "nginx", "name": "app", "status": "drifted"},
				map[string]interface{}{"kind": "stack", "name": "web/.env", "status": "ok"},
				map[string]interface{}{"kind": "nginx", "name": "gone", "status": "missing"},
			},
		}, ""
	})

	s.scanDriftOnce()
	if calls != 1 {
		t.Fatalf("agent calls = %d, want 1", calls)
	}
	alerts, err := s.db.ListAlerts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1", len(alerts))
	}
	if !strings.Contains(alerts[0].Message, "nginx/app") || !strings.Contains(alerts[0].Message, "missing") {
		t.Fatalf("message = %q", alerts[0].Message)
	}
	if strings.Contains(alerts[0].Message, "web/.env") {
		t.Fatalf("ok item should not appear: %q", alerts[0].Message)
	}

	// 再次扫描：未读告警存在 → 去重，仍只有 1 条
	s.scanDriftOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("alerts after rescan = %d, want 1", len(alerts))
	}
}

func TestScanDriftOnceSkipsErrorsAndAllOK(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeDriftAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"kind": "nginx", "name": "app", "status": "ok"},
				map[string]interface{}{"kind": "nginx", "name": "x", "status": "error"},
			},
		}, ""
	})
	// 全 ok + error 项 → 无告警
	s.scanDriftOnce()
	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("alerts = %d, want 0", len(alerts))
	}

	// agent 返回错误 → 跳过不 panic、无告警
	withFakeDriftAgent(t, s, "bad", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "baseline unreadable"
	})
	s.scanDriftOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("alerts = %d, want 0", len(alerts))
	}

	// 标题含主机名（告警可定位到主机）
	withFakeDriftAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"kind": "cron", "name": "cockpit", "status": "drifted"},
			},
		}, ""
	})
	s.scanDriftOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 1 || alerts[0].Title != "配置漂移：主机 host-a2" {
		t.Fatalf("alerts = %+v", alerts)
	}
}
