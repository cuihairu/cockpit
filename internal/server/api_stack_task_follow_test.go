package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 部署任务日志流式推送 server 侧测试（P2 ②，同 logs-follow 测试口径）：
// 路由 405/400、上限 429、agent 拒绝映射（not found → 404，其余 502）、
// NDJSON 帧序列（proxy_data 分派 + proxy_close eof）、客户端断开补发 follow.stop。

func newStackTaskFollowTestServer(t *testing.T) (*Server, *Agent) {
	t.Helper()
	s := newBackupTestServer(t)
	agent := NewAgent("a1", nil)
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	t.Cleanup(func() {
		stackTaskFollowersMu.Lock()
		stackTaskFollowers = map[string]*stackTaskFollower{}
		stackTaskFollowsPerAgent = map[string]int{}
		stackTaskFollowersMu.Unlock()
	})
	return s, agent
}

// startTaskFollowRPC 应答 follow start 成功并回传 followId（测试 goroutine 里调）
func startTaskFollowRPC(s *Server, reqMsg *protocol.Message) string {
	followID, _ := reqMsg.Payload["params"].(map[string]interface{})["followId"].(string)
	resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
		"status": "success",
		"data":   map[string]interface{}{"followId": followID},
	})
	resp.ID = reqMsg.ID
	s.handleRPCResponse(resp)
	return followID
}

func TestStackTaskFollowRouting(t *testing.T) {
	s := &Server{registry: NewRegistry(), db: testServerDB(t)}

	// GET follow → 405（只收 POST）
	rec := httptest.NewRecorder()
	s.handleStacks(rec, httptest.NewRequest(http.MethodGet, "/api/stacks/agents/a1/tasks/t1/follow", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET follow: code = %d", rec.Code)
	}
	// POST follow + 坏 taskId → 400（httptest.NewRequest 拒绝 %zz，手工构造）
	rec = httptest.NewRecorder()
	s.handleStacks(rec, &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/api/stacks/agents/a1/tasks/%zz/follow"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad task id: code = %d", rec.Code)
	}
}

func TestStackTaskFollowLimits(t *testing.T) {
	s, _ := newStackTaskFollowTestServer(t)

	stackTaskFollowersMu.Lock()
	stackTaskFollowsPerAgent["a1"] = stackTaskFollowMaxPerAgent
	stackTaskFollowersMu.Unlock()
	rec := httptest.NewRecorder()
	s.handleStackTaskFollow(rec, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("per-agent limit: code = %d", rec.Code)
	}

	stackTaskFollowersMu.Lock()
	delete(stackTaskFollowsPerAgent, "a1")
	for i := 0; i < stackTaskFollowMaxGlobal; i++ {
		stackTaskFollowers[strings.Repeat("f", i+1)] = &stackTaskFollower{agentID: "other"}
	}
	stackTaskFollowersMu.Unlock()
	rec = httptest.NewRecorder()
	s.handleStackTaskFollow(rec, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("global limit: code = %d", rec.Code)
	}
}

func TestStackTaskFollowAgentRejects(t *testing.T) {
	s, agent := newStackTaskFollowTestServer(t)

	// 任务不存在 → 404
	go func() {
		reqMsg := <-agent.Send
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "error", "error": "task not found: t-9",
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}()
	rec := httptest.NewRecorder()
	s.handleStackTaskFollow(rec, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-9")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("not found: code = %d body = %s", rec.Code, rec.Body.String())
	}

	// agent 侧其他拒绝 → 502
	go func() {
		reqMsg := <-agent.Send
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "error", "error": "stack task follow not available",
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}()
	rec = httptest.NewRecorder()
	s.handleStackTaskFollow(rec, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-1")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("reject: code = %d body = %s", rec.Code, rec.Body.String())
	}
	// 失败路径注册表清空，不占名额
	stackTaskFollowersMu.Lock()
	n := len(stackTaskFollowers)
	stackTaskFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked after rejection: %d", n)
	}
}

func TestStackTaskFollowStreamFrames(t *testing.T) {
	s, agent := newStackTaskFollowTestServer(t)
	go func() {
		reqMsg := <-agent.Send
		params := reqMsg.Payload["params"].(map[string]interface{})
		if params["taskId"] != "t-1" {
			t.Errorf("taskId = %v", params["taskId"])
		}
		followID := startTaskFollowRPC(s, reqMsg)

		// 日志增量走真实 proxy_data 分派（proxyId="stacktask:<followId>"）
		s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
			"proxyId": "stacktask:" + followID,
			"connId":  "stacktask:" + followID,
			"data":    []byte("pulling...\nup -d web\n"),
		}))
		// 任务终态 → agent proxy_close → eof 帧（reason=终态）
		s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
			"proxyId": "stacktask:" + followID,
			"connId":  "stacktask:" + followID,
			"reason":  "success",
		}))
	}()

	// 走 handleStacks 完整路由进流式 handler（覆盖 /tasks/{id}/follow 分派）
	rec := httptest.NewRecorder()
	s.handleStacks(rec, httptest.NewRequest(http.MethodPost, "/api/stacks/agents/a1/tasks/t-1/follow", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("follow: code = %d body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("content-type = %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `{"data":"pulling...\nup -d web\n"}`) {
		t.Errorf("data frame missing: %s", body)
	}
	if !strings.Contains(body, `{"eof":true,"reason":"success"}`) {
		t.Errorf("eof frame missing: %s", body)
	}
	// eof 收尾后注册表清空；agent 侧已自行退出，不应再有 stop RPC
	stackTaskFollowersMu.Lock()
	n := len(stackTaskFollowers)
	stackTaskFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked after eof: %d", n)
	}
	select {
	case extra := <-agent.Send:
		t.Errorf("unexpected RPC after eof: %v", extra.Payload["method"])
	default:
	}
}

func TestStackTaskFollowClientAbortSendsStop(t *testing.T) {
	s, agent := newStackTaskFollowTestServer(t)
	go func() {
		reqMsg := <-agent.Send
		followID := startTaskFollowRPC(s, reqMsg)

		// 客户端断开 → server 补发 stack.task.follow.stop
		stopMsg := <-agent.Send
		if m := stopMsg.Payload["method"]; m != "stack.task.follow.stop" {
			t.Errorf("method = %v, want stack.task.follow.stop", m)
		}
		if p := stopMsg.Payload["params"].(map[string]interface{})["followId"]; p != followID {
			t.Errorf("stop followId = %v, want %s", p, followID)
		}
		stopResp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "success", "data": map[string]interface{}{},
		})
		stopResp.ID = stopMsg.ID
		s.handleRPCResponse(stopResp)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/x", nil).WithContext(ctx)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel() // 模拟 fetch abort → 连接断开
	}()
	rec := httptest.NewRecorder()
	s.handleStackTaskFollow(rec, req, "a1", "t-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("abort: code = %d", rec.Code)
	}
	stackTaskFollowersMu.Lock()
	n := len(stackTaskFollowers)
	stackTaskFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked after abort: %d", n)
	}
}

// noFlushWriter 不实现 Flusher：streaming 探测失败的 500 分支
// （嵌接口类型不提升 Flush，底层 recorder 保 Write 可用）
type noFlushWriter struct{ http.ResponseWriter }

func TestStackTaskFollowNoFlusher(t *testing.T) {
	s, agent := newStackTaskFollowTestServer(t)
	go func() {
		reqMsg := <-agent.Send
		startTaskFollowRPC(s, reqMsg)
	}()
	rec := httptest.NewRecorder()
	s.handleStackTaskFollow(&noFlushWriter{rec}, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-1")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "streaming unsupported") {
		t.Fatalf("no flusher: code = %d body = %s", rec.Code, rec.Body.String())
	}
	stackTaskFollowersMu.Lock()
	n := len(stackTaskFollowers)
	stackTaskFollowersMu.Unlock()
	if n != 0 {
		t.Fatalf("follower leaked: %d", n)
	}
}

// TestStackTaskFollowEdgePaths 代理分派边界 + agent 错误形态映射
func TestStackTaskFollowEdgePaths(t *testing.T) {
	s, agent := newStackTaskFollowTestServer(t)

	// proxy_data / proxy_close 对不存在（已注销）follow：静默丢弃
	s.HandleStackTaskFollowData("stacktask:ghost", []byte("x"))
	s.HandleStackTaskFollowClose("stacktask:ghost", "late")

	// agent 不在注册表（离线/未知）：CallAgent 失败 → 502 failed to reach agent
	rec := httptest.NewRecorder()
	s.handleStackTaskFollow(rec, httptest.NewRequest(http.MethodPost, "/x", nil), "ghost", "t-1")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "failed to reach agent") {
		t.Fatalf("unreachable: code = %d body = %s", rec.Code, rec.Body.String())
	}

	// agent 应答 error 且无 error 字段 → 兜底文案
	go func() {
		reqMsg := <-agent.Send
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "error",
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}()
	rec = httptest.NewRecorder()
	s.handleStackTaskFollow(rec, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-1")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "agent rejected the follow") {
		t.Fatalf("empty error: code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// failAfter 前 n 次 Write 成功、之后失败（嵌具体 recorder 保留 Flush）：
// 让首帧数据写成功、handler 重新进 select 后再断，构造「done 与剩余
// 数据帧同时就绪」的排干场景
type failAfter struct {
	*httptest.ResponseRecorder
	n int
}

func (w *failAfter) Write(b []byte) (int, error) {
	if w.n <= 0 {
		return 0, errors.New("client gone")
	}
	w.n--
	return w.ResponseRecorder.Write(b)
}

// TestStackTaskFollowWriteFail 客户端断开的写帧失败路径：主 select 数据帧
// 失败返回，以及 done 排干循环里对剩余数据帧写失败返回。后者要求 done 与
// 剩余 chunk 同时就绪（select 随机），跑 16 次覆盖两分支。
func TestStackTaskFollowWriteFail(t *testing.T) {
	s, agent := newStackTaskFollowTestServer(t)
	go func() {
		for reqMsg := range agent.Send {
			startTaskFollowRPC(s, reqMsg)
		}
	}()

	waitFollower := func() (string, *stackTaskFollower) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			stackTaskFollowersMu.Lock()
			for id, v := range stackTaskFollowers {
				if v.agentID == "a1" {
					stackTaskFollowersMu.Unlock()
					return id, v
				}
			}
			stackTaskFollowersMu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
		return "", nil
	}
	runSession := func() {
		t.Helper()
		rec := httptest.NewRecorder()
		fin := make(chan struct{})
		go func() {
			s.handleStackTaskFollow(&failAfter{ResponseRecorder: rec, n: 1}, httptest.NewRequest(http.MethodPost, "/x", nil), "a1", "t-1")
			close(fin)
		}()
		followID, f := waitFollower()
		if f == nil {
			select {
			case <-fin:
				t.Fatalf("handler exited early with code %d body %s", rec.Code, rec.Body.String())
			case <-time.After(300 * time.Millisecond):
				t.Fatal("follower never appeared and handler still running")
			}
		}
		// 首帧（写成功，n 归零）+ 第二帧与 done 紧随其后：main select 随机
		// 选中 done 时排干循环对第二帧写失败返回（191-193）
		f.ch <- "first line"
		f.ch <- "second line"
		s.HandleStackTaskFollowClose("stacktask:"+followID, "success")
		select {
		case <-fin:
		case <-time.After(3 * time.Second):
			t.Fatal("handler did not return after write failure")
		}
	}
	for i := 0; i < 16; i++ {
		runSession()
	}
}

// TestStackGitPullConflictMapping git pull 遇本地改动拒绝合并 → 409（前端引导先处理 drift）
func TestStackGitPullConflictMapping(t *testing.T) {
	s := newBackupTestServer(t)
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "docker-api"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	go func() {
		reqMsg := <-agent.Send
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "error", "error": "merge conflict, resolve local changes first: x",
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}()
	rec := httptest.NewRecorder()
	s.handleStacks(rec, httptest.NewRequest(http.MethodPost, "/api/stacks/agents/a1/git/pull", strings.NewReader(`{}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflict mapping: code = %d body = %s", rec.Code, rec.Body.String())
	}
}
