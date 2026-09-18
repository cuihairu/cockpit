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

func TestNASConfigAPI(t *testing.T) {
	s := newBackupTestServer(t)

	// 默认值
	rec := httptest.NewRecorder()
	s.handleNASConfig(rec, httptest.NewRequest(http.MethodGet, "/api/nas/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var cfg struct {
		ScanIntervalSeconds int `json:"scan_interval_seconds"`
		UsageWarnPercent    int `json:"usage_warn_percent"`
	}
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.ScanIntervalSeconds != 1800 || cfg.UsageWarnPercent != 80 {
		t.Fatalf("default config = %+v", cfg)
	}

	// 合法写入（间隔含 0 = 关闭；阈值只写一项时间隔不变）
	for _, v := range []int{0, 300, 86400} {
		rec = httptest.NewRecorder()
		s.handleNASConfig(rec, httptest.NewRequest(http.MethodPut, "/api/nas/config",
			strings.NewReader(`{"scan_interval_seconds":`+strconv.Itoa(v)+`}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %d: code=%d", v, rec.Code)
		}
		if got := s.GetNASScanInterval(); got != v {
			t.Fatalf("PUT %d then GET = %d", v, got)
		}
	}
	rec = httptest.NewRecorder()
	s.handleNASConfig(rec, httptest.NewRequest(http.MethodPut, "/api/nas/config",
		strings.NewReader(`{"scan_interval_seconds":1800,"usage_warn_percent":90}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT usage: code=%d", rec.Code)
	}
	if s.GetNASUsageWarnPercent() != 90 {
		t.Fatalf("usage = %d, want 90", s.GetNASUsageWarnPercent())
	}

	// 越界拒绝
	for _, body := range []string{
		`{"scan_interval_seconds":-1}`,
		`{"scan_interval_seconds":299}`,
		`{"scan_interval_seconds":90000}`,
		`{"usage_warn_percent":49}`,
		`{"usage_warn_percent":100}`,
	} {
		rec = httptest.NewRecorder()
		s.handleNASConfig(rec, httptest.NewRequest(http.MethodPut, "/api/nas/config", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: code = %d, want 400", body, rec.Code)
		}
	}
}

// withFakeNASAgent 注册带 nas capability 的假 agent
func withFakeNASAgent(t *testing.T, s *Server, agentID string,
	handler func(method string, params map[string]interface{}) (interface{}, string)) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	agent.Capabilities = []protocol.Capability{{Type: "nas"}}
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

// nasStatusPayload 构造 nas.status 应答
func nasStatusPayload(pools, mounts, shares []map[string]interface{}) interface{} {
	return map[string]interface{}{
		"available": true, "source": "linux",
		"pools": pools, "mounts": mounts, "shares": shares,
	}
}

func TestNasStatusForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeNASAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return nasStatusPayload(
			[]map[string]interface{}{{"name": "md0", "kind": "mdadm", "state": "healthy", "totalGB": 100.0}},
			[]map[string]interface{}{{"device": "/dev/sdb1", "mountPath": "/mnt/data", "totalGB": 1953.0, "usedGB": 1000.0}},
			[]map[string]interface{}{{"protocol": "nfs", "path": "/srv/media", "hosts": "192.168.1.0/24"}},
		), ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentNASAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/nas/status", nil), "a1/nas/status")
	if rec.Code != http.StatusOK || gotMethod != "nas.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"mountPath":"/mnt/data"`) {
		t.Errorf("body = %s", rec.Body.String())
	}

	// 离线 → 503；浏览不审计
	rec = httptest.NewRecorder()
	s.handleAgentNASAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/ghost/nas/status", nil), "ghost/nas/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline: code = %d, want 503", rec.Code)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	if len(logs) != 0 {
		t.Errorf("browse must not audit, logs = %v", logs)
	}
}

func TestNasScanAlerts(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeNASAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nasStatusPayload(
			[]map[string]interface{}{
				{"name": "md0", "kind": "mdadm", "state": "failed", "detail": "inactive"},
				{"name": "tank", "kind": "zfs", "state": "degraded", "detail": "DEGRADED"},
				{"name": "vg0", "kind": "lvm", "state": "unknown"},
			},
			[]map[string]interface{}{
				{"device": "/dev/sdb1", "mountPath": "/mnt/data", "totalGB": 1953.0, "usedGB": 1860.0}, // 95% ≥ 80
				{"device": "/dev/sda1", "mountPath": "/", "totalGB": 40.0, "usedGB": 12.0},             // 30% < 80
			},
			nil,
		), ""
	})

	s.scanNASOnce()
	alerts, _ := s.db.ListAlerts(10)
	// failed 池 + degraded 池 + 超阈值挂载 = 3 条（unknown/低用量不告警）
	if len(alerts) != 3 {
		t.Fatalf("alerts = %d, want 3: %+v", len(alerts), alerts)
	}
	titles := map[string]string{}
	for _, a := range alerts {
		titles[a.Title] = a.Type
	}
	if titles["存储池故障：md0（主机 host-a1）"] != "error" {
		t.Errorf("failed pool alert missing or wrong level: %v", titles)
	}
	if titles["存储池异常：tank（主机 host-a1）"] != "warning" {
		t.Errorf("degraded pool alert missing: %v", titles)
	}
	usageFound := false
	for title := range titles {
		if strings.Contains(title, "/mnt/data") && strings.Contains(title, "95%") {
			usageFound = true
		}
	}
	if !usageFound {
		t.Errorf("usage alert missing: %v", titles)
	}

	// 再次扫描：未读告警真去重
	s.scanNASOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 3 {
		t.Fatalf("alerts after rescan = %d, want 3", len(alerts))
	}
}

func TestNasScanHealthySkips(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeNASAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nasStatusPayload(
			[]map[string]interface{}{{"name": "md0", "kind": "mdadm", "state": "healthy"}},
			[]map[string]interface{}{{"device": "/dev/sda1", "mountPath": "/", "totalGB": 40.0, "usedGB": 12.0}},
			nil,
		), ""
	})
	s.scanNASOnce()
	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("healthy snapshot should not alert, alerts = %+v", alerts)
	}

	// available=false：观测缺失≠故障，跳过不告警
	withFakeNASAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"available": false}, ""
	})
	s.scanNASOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("unavailable agent should not alert, alerts = %+v", alerts)
	}
}
