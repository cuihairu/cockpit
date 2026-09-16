package server

// cov_* 系列测试为覆盖率补充测试（HTTP handler 部分），
// 与既有测试文件相互独立；helper 一律带 cov 前缀避免冲突。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// covNewServer 构造 db/audit/registry/ctx 齐全的测试 Server（cfg 为 nil，需要时自行注入）
func covNewServer(t *testing.T) *Server {
	t.Helper()
	db := testServerDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Server{
		registry:       NewRegistry(),
		db:             db,
		audit:          audit.NewLogger(db),
		notifier:       notification.NewService(nil),
		remoteSessions: NewRemoteSessionManager(),
		ticketMgr:      NewTicketManager(),
		ctx:            ctx,
	}
}

// covCloseDB 关闭底层数据库以触发各 handler 的 db 错误分支
// （storage 关闭幂等，t.Cleanup 的二次 Close 安全）
func covCloseDB(t *testing.T, s *Server) {
	t.Helper()
	if err := s.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
}

// covReq 构造普通请求
func covReq(method, target string, body io.Reader) *http.Request {
	return httptest.NewRequest(method, target, body)
}

// covRec 返回新 recorder
func covRec() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

// covAuthReq 构造带 Bearer token 的请求（配合 auth.Middleware 注入用户上下文）
func covAuthReq(method, target string, body io.Reader, userID, username, role string) *http.Request {
	req := httptest.NewRequest(method, target, body)
	token, err := auth.GenerateToken(userID, username, role)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// covCallAuth 用 auth.Middleware 包裹后调用（为 handler 注入用户上下文）
func covCallAuth(s *Server, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	auth.Middleware(h)(rec, r)
	return rec
}

// covFakeAgent 注册一个应答 RPC 的假 agent。
// respond 返回响应 payload（nil 表示不应答，用于超时场景）。
func covFakeAgent(t *testing.T, s *Server, agentID string, caps []string, respond func(method string, params map[string]interface{}) map[string]interface{}) *Agent {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	for _, c := range caps {
		agent.Capabilities = append(agent.Capabilities, protocol.Capability{Type: c})
	}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if respond != nil {
		go func() {
			for reqMsg := range agent.Send {
				if reqMsg.Type != protocol.MessageTypeRPCRequest {
					continue
				}
				method, _ := reqMsg.Payload["method"].(string)
				params, _ := reqMsg.Payload["params"].(map[string]interface{})
				payload := respond(method, params)
				if payload == nil {
					continue
				}
				resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, payload)
				resp.ID = reqMsg.ID
				s.handleRPCResponse(resp)
			}
		}()
	}
	t.Cleanup(func() {
		s.registry.Unregister(agentID)
	})
	return agent
}

// covOKPayload 成功响应 payload
func covOKPayload(data interface{}) map[string]interface{} {
	return map[string]interface{}{"status": "success", "data": data}
}

// covErrPayload 错误响应 payload
func covErrPayload(msg string) map[string]interface{} {
	return map[string]interface{}{"status": "error", "error": msg}
}

// covWantCode 断言状态码
func covWantCode(t *testing.T, desc string, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("%s: code = %d, want %d (body: %s)", desc, rec.Code, want, rec.Body.String())
	}
}
