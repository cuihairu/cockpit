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

func TestSmartConfigAPI(t *testing.T) {
	s := newBackupTestServer(t)

	// 默认值
	rec := httptest.NewRecorder()
	s.handleSmartConfig(rec, httptest.NewRequest(http.MethodGet, "/api/smart/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var cfg struct {
		ScanIntervalSeconds int `json:"scan_interval_seconds"`
		Min                 int `json:"min"`
		Max                 int `json:"max"`
		Default             int `json:"default"`
	}
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.ScanIntervalSeconds != 3600 || cfg.Min != 300 || cfg.Max != 86400 || cfg.Default != 3600 {
		t.Fatalf("default config = %+v", cfg)
	}

	// 合法写入（含 0 = 关闭）
	for _, v := range []int{0, 300, 86400} {
		rec = httptest.NewRecorder()
		body := strings.NewReader(`{"scan_interval_seconds":` + strconv.Itoa(v) + `}`)
		s.handleSmartConfig(rec, httptest.NewRequest(http.MethodPut, "/api/smart/config", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %d: code=%d body=%s", v, rec.Code, rec.Body.String())
		}
		if got := s.GetSmartScanInterval(); got != v {
			t.Fatalf("PUT %d then GET = %d", v, got)
		}
	}

	// 越界拒绝
	for _, bad := range []string{"-1", "299", "90000"} {
		rec = httptest.NewRecorder()
		s.handleSmartConfig(rec, httptest.NewRequest(http.MethodPut, "/api/smart/config",
			strings.NewReader(`{"scan_interval_seconds":`+bad+`}`)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: code = %d, want 400", bad, rec.Code)
		}
	}
}

// withFakeSmartAgent 注册带 hardware-monitor（metadata.smart 可控）的假 agent
func withFakeSmartAgent(t *testing.T, s *Server, agentID string, smart bool,
	handler func(method string, params map[string]interface{}) (interface{}, string)) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	cap := protocol.Capability{Type: "hardware-monitor"}
	if smart {
		cap.Metadata = map[string]interface{}{"smart": true}
	}
	agent.Capabilities = []protocol.Capability{cap}
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

// smartDevicesPayload 构造 smart.status 应答
func smartDevicesPayload(devs ...map[string]interface{}) interface{} {
	list := make([]interface{}, 0, len(devs))
	for _, d := range devs {
		list = append(list, d)
	}
	return map[string]interface{}{"available": true, "devices": list}
}

func TestScanSmartOnceCreatesAlert(t *testing.T) {
	s := newBackupTestServer(t)
	var calls int
	withFakeSmartAgent(t, s, "a1", true, func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		if method != "hardware-monitor.status" {
			t.Errorf("unexpected method %s", method)
		}
		return smartDevicesPayload(
			map[string]interface{}{"name": "/dev/sda", "health": "failed"},
			map[string]interface{}{"name": "/dev/sdb", "health": "passed",
				"reallocatedSectors": 48, "pendingSectors": 8},
			map[string]interface{}{"name": "/dev/sdc", "health": "passed"},
		), ""
	})

	s.scanSmartOnce()
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
	// 有 FAILED 盘 → error 级 + 专属标题
	if alerts[0].Type != "error" || alerts[0].Title != "磁盘健康告警：主机 host-a1" {
		t.Fatalf("alert = %q (%s)", alerts[0].Title, alerts[0].Type)
	}
	if !strings.Contains(alerts[0].Message, "/dev/sda") ||
		!strings.Contains(alerts[0].Message, "重映射扇区 48") {
		t.Fatalf("message = %q", alerts[0].Message)
	}
	if strings.Contains(alerts[0].Message, "/dev/sdc") {
		t.Fatalf("healthy disk should not appear: %q", alerts[0].Message)
	}

	// 再次扫描：未读告警存在 → 去重，仍只有 1 条
	s.scanSmartOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("alerts after rescan = %d, want 1", len(alerts))
	}
}

func TestScanSmartOnceWarningLevel(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeSmartAgent(t, s, "a1", true, func(method string, params map[string]interface{}) (interface{}, string) {
		// 只有扇区异常、无 FAILED → warning 级
		return smartDevicesPayload(
			map[string]interface{}{"name": "/dev/sdb", "health": "passed", "pendingSectors": 8},
		), ""
	})
	s.scanSmartOnce()
	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1", len(alerts))
	}
	if alerts[0].Type != "warning" || alerts[0].Title != "磁盘健康提醒：主机 host-a1" {
		t.Fatalf("alert = %q (%s)", alerts[0].Title, alerts[0].Type)
	}
}

func TestScanSmartOnceFilters(t *testing.T) {
	s := newBackupTestServer(t)
	// agent A：有 smart 标志，全健康 + unknown 盘（不告警）
	withFakeSmartAgent(t, s, "a1", true, func(method string, params map[string]interface{}) (interface{}, string) {
		return smartDevicesPayload(
			map[string]interface{}{"name": "/dev/sda", "health": "passed"},
			map[string]interface{}{"name": "/dev/sdz", "health": "unknown", "error": "Permission denied"},
		), ""
	})
	// agent B：hardware-monitor 但无 smart 标志 → 跳过（D1）
	var bCalls int
	withFakeSmartAgent(t, s, "a2", false, func(method string, params map[string]interface{}) (interface{}, string) {
		bCalls++
		return smartDevicesPayload(map[string]interface{}{"name": "/dev/sda", "health": "failed"}), ""
	})
	// agent C：RPC 返回错误 → 跳过不 panic
	withFakeSmartAgent(t, s, "a3", true, func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "smartctl exploded"
	})

	s.scanSmartOnce()

	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("alerts = %d, want 0", len(alerts))
	}
	if bCalls != 0 {
		t.Errorf("agent without smart flag should be skipped, calls = %d", bCalls)
	}
}

func TestScanSmartOnceUnavailableSkipped(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeSmartAgent(t, s, "a1", true, func(method string, params map[string]interface{}) (interface{}, string) {
		// smartctl 缺失：available=false → 视为不支持，跳过
		return map[string]interface{}{"available": false, "devices": []interface{}{}}, ""
	})
	s.scanSmartOnce()
	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("alerts = %d, want 0", len(alerts))
	}
}

func TestAgentHasSmart(t *testing.T) {
	a := NewAgent("x", nil)
	if agentHasSmart(a) {
		t.Error("no capability should be false")
	}
	a.Capabilities = []protocol.Capability{{Type: "hardware-monitor"}}
	if agentHasSmart(a) {
		t.Error("capability without metadata should be false")
	}
	a.Capabilities = []protocol.Capability{
		{Type: "hardware-monitor", Metadata: map[string]interface{}{"smart": true}},
	}
	if !agentHasSmart(a) {
		t.Error("metadata.smart=true should be true")
	}
	a.Capabilities = []protocol.Capability{
		{Type: "hardware-monitor", Metadata: map[string]interface{}{"smart": "yes"}},
	}
	if agentHasSmart(a) {
		t.Error("non-bool metadata should be false")
	}
}
