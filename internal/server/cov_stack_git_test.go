package server

// P2 ① Git-to-deploy：/api/stacks/agents/{id}/git[/pull] 路由、错误映射、审计。

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
)

func TestCovStackGitRoutes(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "stack.git.status":
			return covOKPayload(map[string]interface{}{
				"enabled": false, "reason": "not-a-repo",
			})
		case "stack.git.pull":
			return covOKPayload(map[string]interface{}{
				"enabled": true, "remote": "https://git.example/ops/stacks.git",
				"branch": "main", "head": "abc1234", "remoteHead": "abc1234",
				"ahead": float64(0), "behind": float64(0),
			})
		}
		return covErrPayload("unexpected method " + method)
	})

	// GET git：状态透传（无审计）
	rec := covRec()
	s.handleStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/git", nil))
	covWantCode(t, "git status", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"reason":"not-a-repo"`) {
		t.Errorf("git status body = %s", rec.Body.String())
	}

	// POST git/pull：透传 + 审计 stack_git_pull（资源=agentID，无 stack 名）
	req := covAuthReq(http.MethodPost, "/api/stacks/agents/a1/git/pull",
		strings.NewReader(`{"remote":"https://git.example/ops/stacks.git","branch":"main"}`), "1", "admin", "admin")
	rec = covCallAuth(s, s.handleStacks, req)
	covWantCode(t, "git pull", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"head":"abc1234"`) {
		t.Errorf("git pull body = %s", rec.Body.String())
	}

	// 空 body pull 也可（缺省 remote/branch 由 agent 处理）
	rec = covRec()
	s.handleStacks(rec, covReq(http.MethodPost, "/api/stacks/agents/a1/git/pull", nil))
	covWantCode(t, "git pull empty body", rec, http.StatusOK)

	// 审计落库断言：resource=stack 类型，resourceID=agentID
	deadline := time.Now().Add(3 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		logs, _, err := s.db.GetAuditLogs(0, 50, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range logs {
			if l.Action == audit.ActionStackGitPull && l.Resource == audit.ResourceStack && l.ResourceID == "a1" {
				found = true
				if !strings.Contains(l.Details, "git.example") {
					t.Errorf("git pull audit details missing result: %s", l.Details)
				}
			}
		}
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		t.Error("stack_git_pull audit not recorded")
	}

	// 405：GET pull / POST status / PATCH git
	for _, c := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/stacks/agents/a1/git/pull"},
		{http.MethodPost, "/api/stacks/agents/a1/git"},
		{http.MethodPatch, "/api/stacks/agents/a1/git"},
		{http.MethodDelete, "/api/stacks/agents/a1/git/pull"},
	} {
		rec := covRec()
		s.handleStacks(rec, covReq(c.method, c.path, nil))
		covWantCode(t, c.method+" "+c.path, rec, http.StatusMethodNotAllowed)
	}

	// pull body 非法 JSON → 400
	rec = covRec()
	s.handleStacks(rec, covReq(http.MethodPost, "/api/stacks/agents/a1/git/pull", strings.NewReader("not-json")))
	covWantCode(t, "git pull bad json", rec, http.StatusBadRequest)
}

func TestCovStackGitErrorMapping(t *testing.T) {
	// rpc error 含 conflict → 409；含 not found → 404；其他 → 502；无 docker 能力 → 404
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "stack.git.pull":
			return covErrPayload("merge conflict, resolve local changes first: Your local changes ... would be overwritten")
		case "stack.git.status":
			return covErrPayload("boom")
		}
		return covErrPayload("unexpected method " + method)
	})

	rec := covRec()
	s.handleStacks(rec, covReq(http.MethodPost, "/api/stacks/agents/a1/git/pull", nil))
	covWantCode(t, "git pull conflict", rec, http.StatusConflict)

	rec = covRec()
	s.handleStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/git", nil))
	covWantCode(t, "git status rpc err", rec, http.StatusBadGateway)

	// git pull 报 remote branch not found → 404
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("remote branch main not found (empty repository? push a commit first)")
	})
	rec = covRec()
	s2.handleStacks(rec, covReq(http.MethodPost, "/api/stacks/agents/a1/git/pull", nil))
	covWantCode(t, "git pull branch missing", rec, http.StatusNotFound)

	// 无 docker 能力的 agent → 404（requireStackAgent）
	covFakeAgent(t, s, "plain", nil, nil)
	rec = covRec()
	s.handleStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/plain/git", nil))
	covWantCode(t, "git no capability", rec, http.StatusNotFound)
}
