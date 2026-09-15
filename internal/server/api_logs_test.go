package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// logsReq 构造对 /api/agents/{id}/logs/{sub} 的请求
func logsReq(method, agentID, sub, body string) *http.Request {
	return httptest.NewRequest(method, "/api/agents/"+agentID+"/logs/"+sub, strings.NewReader(body))
}

func TestLogsStatusAndSourcesForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		if method == "logs.status" {
			return map[string]interface{}{"journalctl": true, "docker": false}, ""
		}
		return map[string]interface{}{
			"systemd": []string{"nginx.service", "ssh.service"},
			"docker":  []string{},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodGet, "a1", "status", ""), "a1/logs/status")
	if rec.Code != http.StatusOK || gotMethod != "logs.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var st struct {
		Journalctl bool `json:"journalctl"`
		Docker     bool `json:"docker"`
	}
	json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.Journalctl || st.Docker {
		t.Fatalf("status = %+v", st)
	}

	rec = httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodGet, "a1", "sources", ""), "a1/logs/sources")
	if rec.Code != http.StatusOK || gotMethod != "logs.sources" {
		t.Fatalf("sources: code=%d method=%s", rec.Code, gotMethod)
	}
	var src struct {
		Systemd []string `json:"systemd"`
	}
	json.Unmarshal(rec.Body.Bytes(), &src)
	if len(src.Systemd) != 2 || src.Systemd[0] != "nginx.service" {
		t.Fatalf("sources = %+v", src)
	}
}

func TestLogsQueryForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		// json roundtrip 归一（贴近真实线路形状）
		b, _ := json.Marshal(params)
		json.Unmarshal(b, &gotParams)
		return map[string]interface{}{"lines": "2026-09-15T10:00:00+08:00 ready\n", "truncated": false}, ""
	})

	body := `{"type":"systemd","source":"nginx.service","tail":500,"since_minutes":60,"grep":"ready"}`
	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodPost, "a1", "query", body), "a1/logs/query")
	if rec.Code != http.StatusOK {
		t.Fatalf("query: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lines     string `json:"lines"`
		Truncated bool   `json:"truncated"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.Contains(resp.Lines, "ready") || resp.Truncated {
		t.Fatalf("resp = %+v", resp)
	}

	// 转发参数形状核对
	q, _ := gotParams["query"].(map[string]interface{})
	if q == nil || q["type"] != "systemd" || q["source"] != "nginx.service" ||
		q["tail"] != float64(500) || q["since_minutes"] != float64(60) || q["grep"] != "ready" {
		t.Fatalf("forwarded params = %v", gotParams)
	}
}

func TestLogsQueryValidatesBeforeForwarding(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{"lines": "", "truncated": false}, ""
	})

	bad := []string{
		`{"type":"file","source":"x"}`,
		`{"type":"systemd","source":"a; rm"}`,
		`{"type":"systemd","source":"../etc"}`,
		`{"type":"systemd","source":"x.service","tail":5000}`,
		`{"type":"systemd","source":"x.service","since_minutes":9999}`,
		`{"type":"systemd","source":"x.service","grep":"multi\nline"}`,
		`{"type":"systemd","source":""}`,
	}
	for _, body := range bad {
		rec := httptest.NewRecorder()
		s.handleAgentLogsAPI(rec, logsReq(http.MethodPost, "a1", "query", body), "a1/logs/query")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s code = %d, want 400", body, rec.Code)
		}
	}
	if dispatched != 0 {
		t.Errorf("invalid queries should not reach agent, dispatched = %d", dispatched)
	}

	// 合法查询默认值：tail=0 → 服务端归一 200（agent 侧同规则兜底）
	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodPost, "a1", "query", `{"type":"docker","source":"web-1"}`), "a1/logs/query")
	if rec.Code != http.StatusOK {
		t.Errorf("valid default query: code = %d, want 200", rec.Code)
	}
}

func TestLogsErrorPassthroughAndOffline(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "logs.query" {
			return nil, "journalctl: permission denied"
		}
		return map[string]interface{}{}, ""
	})

	// agent 错误原样透传（502）
	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodPost, "a1", "query",
		`{"type":"systemd","source":"x.service"}`), "a1/logs/query")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "permission denied") {
		t.Fatalf("code=%d body=%s, want 502 with agent error", rec.Code, rec.Body.String())
	}

	// 离线 agent 503
	rec = httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodGet, "ghost", "status", ""), "ghost/logs/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline: code = %d, want 503", rec.Code)
	}

	// 未知子路径 404
	rec = httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, logsReq(http.MethodGet, "a1", "tail", ""), "a1/logs/tail")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub: code = %d, want 404", rec.Code)
	}
}
