package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
)

// stacks webhook 触发部署 server 侧测试（P2 ②）：
// 默认关闭（404）、token 校验（401）、agent 校验（404）、
// 异步扇出（200 triggered）+ 审计与部署历史落库、remote 参数拒收。

func newStackWebhookTestServer(t *testing.T, token string) *Server {
	t.Helper()
	s := newBackupTestServer(t)
	s.cfg = &config.Config{Stacks: &config.StacksConfig{WebhookToken: token}}
	return s
}

func webhookReq(body string, token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/webhook", strings.NewReader(body))
	if token != "" {
		req.Header.Set("X-Cockpit-Token", token)
	}
	return req
}

func TestStackWebhookDisabledAndAuth(t *testing.T) {
	s := newBackupTestServer(t) // 未配置 stacks.webhook_token

	// 令牌空 = 端点关闭（默认关红线）
	rec := httptest.NewRecorder()
	s.handleStackWebhook(rec, webhookReq(`{}`, "any"))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "disabled") {
		t.Fatalf("disabled: code = %d body = %s", rec.Code, rec.Body.String())
	}

	s = newStackWebhookTestServer(t, "sekrit")
	// 错 token → 401
	rec = httptest.NewRecorder()
	s.handleStackWebhook(rec, webhookReq(`{}`, "wrong"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: code = %d", rec.Code)
	}
	// 缺头 → 401
	rec = httptest.NewRecorder()
	s.handleStackWebhook(rec, httptest.NewRequest(http.MethodPost, "/api/stacks/webhook", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no header: code = %d", rec.Code)
	}
	// GET → 405
	rec = httptest.NewRecorder()
	s.handleStackWebhook(rec, httptest.NewRequest(http.MethodGet, "/api/stacks/webhook", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: code = %d", rec.Code)
	}
}

func TestStackWebhookUnknownAgent(t *testing.T) {
	s := newStackWebhookTestServer(t, "sekrit")
	rec := httptest.NewRecorder()
	s.handleStackWebhook(rec, webhookReq(`{"agentId":"nope"}`, "sekrit"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown agent: code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// TestStackWebhookDeploy 指定 agent：RPC 下发（remote 剥离）、200 受理、
// 异步落审计 + 部署历史（webhook 动作映射 up）
func TestStackWebhookDeploy(t *testing.T) {
	s := newStackWebhookTestServer(t, "sekrit")
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "stack.git.deploy" {
			return nil, "unexpected method " + method
		}
		if _, ok := params["remote"]; ok {
			return nil, "remote param must be stripped"
		}
		if params["branch"] != "main" {
			return nil, "branch not forwarded"
		}
		return map[string]interface{}{
			"head": "abc1234",
			"deployed": []interface{}{
				map[string]interface{}{"stack": "web", "taskId": "t-9"},
			},
			"skipped": []interface{}{
				map[string]interface{}{"stack": "db", "reason": "busy"},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleStackWebhook(rec, webhookReq(`{"agentId":"a1","branch":"main","remote":"evil://x"}`, "sekrit"))
	if rec.Code != http.StatusOK {
		t.Fatalf("trigger: code = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"triggered":["a1"]`) {
		t.Fatalf("triggered missing: %s", rec.Body.String())
	}

	// 异步执行：轮询等审计行 + 部署历史行
	deadline := time.Now().Add(3 * time.Second)
	var auditRows int64
	for time.Now().Before(deadline) {
		_, n, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "stack_webhook_deploy"})
		if err != nil {
			t.Fatalf("list audit: %v", err)
		}
		auditRows = n
		if deps, _ := s.db.ListStackDeployments("a1", "web", 10); auditRows > 0 && len(deps) > 0 {
			d := deps[0]
			if d.Action != "up" || d.Status != "running" || d.TaskID != "t-9" {
				t.Fatalf("deployment row = %+v", d)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("webhook deploy not recorded (audit rows=%d)", auditRows)
}

// TestStackWebhookFanOut 无 agentId：扇出全部在线 agent，离线的不触发
func TestStackWebhookFanOut(t *testing.T) {
	s := newStackWebhookTestServer(t, "sekrit")
	withFakeBackupAgent(t, s, "a1", nil)
	withFakeBackupAgent(t, s, "a2", nil)
	offline := NewAgent("a3", nil)
	offline.LastSeen = time.Now().Add(-10 * time.Minute)
	if err := s.registry.Register(offline); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	s.handleStackWebhook(rec, webhookReq(`{}`, "sekrit"))
	if rec.Code != http.StatusOK {
		t.Fatalf("fan-out: code = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"a1"`) || !strings.Contains(body, `"a2"`) || strings.Contains(body, `"a3"`) {
		t.Fatalf("fan-out targets wrong: %s", body)
	}
}

// stackDeployedEntries 对畸形 deployed 条目（非对象）静默跳过
func TestStackDeployedEntriesMalformed(t *testing.T) {
	got := stackDeployedEntries(map[string]interface{}{
		"deployed": []interface{}{
			"not-a-map",
			map[string]interface{}{"stack": "web"}, // 缺 taskId
			map[string]interface{}{"stack": "web", "taskId": "t-1"}, // 有效
		},
	})
	if len(got) != 1 || got[0].name != "web" || got[0].taskID != "t-1" {
		t.Fatalf("entries = %v", got)
	}
	// deployed 缺失/形态不对：空表
	if got := stackDeployedEntries(map[string]interface{}{}); len(got) != 0 {
		t.Fatalf("empty = %v", got)
	}
}

// TestStackWebhookAgentFailure RPC 失败只落日志：不炸响应、不留半截历史
func TestStackWebhookAgentFailure(t *testing.T) {
	s := newStackWebhookTestServer(t, "sekrit")
	withFakeBackupAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "remote required"
	})
	rec := httptest.NewRecorder()
	s.handleStackWebhook(rec, webhookReq(`{"agentId":"a1"}`, "sekrit"))
	if rec.Code != http.StatusOK {
		t.Fatalf("accept still 200: code = %d", rec.Code)
	}
	// 同步直调一次：异步 goroutine 的 CallAgent 错误分支与覆盖采集存在
	// 竞态（CI 慢机漏盖），同步调用确定性落覆盖，行为断言不变
	s.runStackWebhookDeploy("a1", map[string]interface{}{"branch": "main"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, n, _ := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "stack_webhook_deploy"})
		if n > 0 {
			t.Fatal("failed deploy should not write audit success row")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
