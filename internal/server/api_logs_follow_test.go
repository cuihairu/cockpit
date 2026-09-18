package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 日志实时尾随 server 侧测试（logs-design.md F4/F5 测试段）：
// 校验 400、上限 429、NDJSON 帧序列、客户端断开触发 follow.stop、
// ProxyClose 转 eof 收尾。RPC 往返用 goroutine 读 agent.Send 模式。

func newLogsFollowTestServer(t *testing.T) (*Server, *Agent) {
	t.Helper()
	s := newBackupTestServer(t)
	agent := NewAgent("a1", nil)
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	t.Cleanup(func() {
		logsFollowersMu.Lock()
		logsFollowers = map[string]*logsFollower{}
		logsFollowsPerAgent = map[string]int{}
		logsFollowersMu.Unlock()
	})
	return s, agent
}

func followBody() *strings.Reader {
	return strings.NewReader(`{"type":"systemd","source":"nginx.service","tail":10,"grep":"err"}`)
}

func TestLogsFollowAPIValidation(t *testing.T) {
	s, _ := newLogsFollowTestServer(t)

	// 坏 JSON → 400
	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`)), "a1/logs/follow")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: code = %d", rec.Code)
	}

	// 非法 type → 400（同 query 校验规则）
	rec = httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"type":"foo","source":"a"}`)), "a1/logs/follow")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad type: code = %d", rec.Code)
	}

	// GET → 404（只收 POST）
	rec = httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodGet, "/x", nil), "a1/logs/follow")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET: code = %d", rec.Code)
	}
}

func TestLogsFollowLimits(t *testing.T) {
	s, _ := newLogsFollowTestServer(t)

	// per-agent 上限
	logsFollowersMu.Lock()
	logsFollowsPerAgent["a1"] = logsFollowMaxPerAgent
	logsFollowersMu.Unlock()
	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodPost, "/x", followBody()), "a1/logs/follow")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("per-agent limit: code = %d", rec.Code)
	}

	// 全局上限
	logsFollowersMu.Lock()
	delete(logsFollowsPerAgent, "a1")
	for i := 0; i < logsFollowMaxGlobal; i++ {
		logsFollowers[strings.Repeat("f", i+1)] = &logsFollower{agentID: "other"}
	}
	logsFollowersMu.Unlock()
	rec = httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodPost, "/x", followBody()), "a1/logs/follow")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("global limit: code = %d", rec.Code)
	}
}

func TestLogsFollowAgentRejects(t *testing.T) {
	s, agent := newLogsFollowTestServer(t)
	go func() {
		reqMsg := <-agent.Send
		if m := reqMsg.Payload["method"]; m != "logs.follow.start" {
			t.Errorf("method = %v, want logs.follow.start", m)
		}
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "error",
			"error":  "journalctl denied",
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}()

	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodPost, "/x", followBody()), "a1/logs/follow")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "journalctl denied") {
		t.Fatalf("agent reject: code = %d body = %s", rec.Code, rec.Body.String())
	}
	// 失败路径注册表必须清空，不占名额
	logsFollowersMu.Lock()
	n := len(logsFollowers)
	logsFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked after rejection: %d", n)
	}
}

func TestLogsFollowStreamFrames(t *testing.T) {
	s, agent := newLogsFollowTestServer(t)
	go func() {
		reqMsg := <-agent.Send
		params := reqMsg.Payload["params"].(map[string]interface{})
		if params["type"] != "systemd" || params["source"] != "nginx.service" ||
			params["tail"] != 10 || params["grep"] != "err" {
			t.Errorf("start params = %v", params)
		}
		followID, _ := params["followId"].(string)
		if followID == "" {
			t.Error("followId missing in start params")
			return
		}
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "success",
			"data":   map[string]interface{}{"followId": followID},
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)

		// 数据走真实 proxy_data 分派（proxyId="logs:<followId>"）
		s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
			"proxyId": "logs:" + followID,
			"connId":  "logs:" + followID,
			"data":    []byte("line1\nERROR line2\n"),
		}))
		// agent 进程退出 → proxy_close → eof 帧
		s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
			"proxyId": "logs:" + followID,
			"connId":  "logs:" + followID,
			"reason":  "exited",
		}))
	}()

	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, httptest.NewRequest(http.MethodPost, "/x", followBody()), "a1/logs/follow")
	if rec.Code != http.StatusOK {
		t.Fatalf("follow: code = %d body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("content-type = %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `{"data":"line1\nERROR line2\n"}`) {
		t.Errorf("data frame missing: %s", body)
	}
	if !strings.Contains(body, `{"eof":true,"reason":"exited"}`) {
		t.Errorf("eof frame missing: %s", body)
	}
	// eof 收尾后注册表清空；agent 侧已自行退出，不应再有 stop RPC
	logsFollowersMu.Lock()
	n := len(logsFollowers)
	logsFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked after eof: %d", n)
	}
	select {
	case extra := <-agent.Send:
		t.Errorf("unexpected RPC after eof: %v", extra.Payload["method"])
	default:
	}
}

func TestLogsFollowClientAbortSendsStop(t *testing.T) {
	s, agent := newLogsFollowTestServer(t)
	go func() {
		reqMsg := <-agent.Send
		followID, _ := reqMsg.Payload["params"].(map[string]interface{})["followId"].(string)
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "success",
			"data":   map[string]interface{}{"followId": followID},
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)

		// 客户端断开 → server 补发 logs.follow.stop
		stopMsg := <-agent.Send
		if m := stopMsg.Payload["method"]; m != "logs.follow.stop" {
			t.Errorf("method = %v, want logs.follow.stop", m)
		}
		stopParams := stopMsg.Payload["params"].(map[string]interface{})
		if stopParams["followId"] != followID {
			t.Errorf("stop followId = %v, want %s", stopParams["followId"], followID)
		}
		stopResp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "success",
			"data":   map[string]interface{}{},
		})
		stopResp.ID = stopMsg.ID
		s.handleRPCResponse(stopResp)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/x", followBody()).WithContext(ctx)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel() // 模拟 fetch abort → 连接断开
	}()
	rec := httptest.NewRecorder()
	s.handleAgentLogsAPI(rec, req, "a1/logs/follow")

	// handler 返回后：注册表清空 + stop RPC 已发出（goroutine 内断言）
	logsFollowersMu.Lock()
	n := len(logsFollowers)
	logsFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked after abort: %d", n)
	}
}
