package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

// cov_api_tags_test.go 服务器标签 API（api_tags.go，bc1098c）：
// 分发（405/404 边界）、四分类错误映射（400/404/409/500）、
// agent 标签覆盖设置（未知 tag 404、agent 缺失 500）与计数聚合组装。

// covTagsCall 直调 agent-tags 分发（无中间件，handler 自身不做鉴权）
func covTagsCall(s *Server, method, path, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	rec := covRec()
	s.handleAgentTagsAPI(rec, covReq(method, path, r))
	return rec
}

// covAssignCall 直调 agent 标签分发（api.go 传入 "id/tags" 形态）
func covAssignCall(s *Server, method, agentPath, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	rec := covRec()
	s.handleAgentTagAssignAPI(rec, covReq(method, "/api/agents/"+agentPath, r), agentPath)
	return rec
}

func covTagsCreate(t *testing.T, s *Server, name string) string {
	t.Helper()
	rec := covTagsCall(s, "POST", "/api/agent-tags", `{"name":"`+name+`","color":"blue"}`)
	covWantCode(t, "create "+name, rec, 201)
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("create body = %s, err %v", rec.Body.String(), err)
	}
	return out.ID
}

func TestCovAgentTagsDispatchAndList(t *testing.T) {
	s := covNewServer(t)

	// 空列表 → 200 []
	rec := covTagsCall(s, "GET", "/api/agent-tags", "")
	covWantCode(t, "list empty", rec, 200)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list body = %s", rec.Body.String())
	}

	// 尾斜杠归一后仍走根分发（TrimSuffix 分支）
	rec = covTagsCall(s, "GET", "/api/agent-tags/", "")
	covWantCode(t, "list trailing slash", rec, 200)

	// 根路径非法方法 → 405
	rec = covTagsCall(s, "PATCH", "/api/agent-tags", "")
	covWantCode(t, "root patch", rec, 405)

	// 含多段路径 → 404（id 再带 / 分发拦截）
	rec = covTagsCall(s, "GET", "/api/agent-tags/a/b", "")
	covWantCode(t, "nested id", rec, 404)

	// 单 id 路径非法方法 → 405
	rec = covTagsCall(s, "GET", "/api/agent-tags/t1", "")
	covWantCode(t, "id get not allowed", rec, 405)

	// 组装：agentCount 与 createdAt 进响应
	db := s.db
	if err := db.UpsertAgent(&storage.Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	id := covTagsCreate(t, s, "prod")
	if err := db.SetAgentTags("a1", []string{id}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	rec = covTagsCall(s, "GET", "/api/agent-tags", "")
	covWantCode(t, "list", rec, 200)
	var list []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Color      string `json:"color"`
		AgentCount int    `json:"agentCount"`
		CreatedAt  int64  `json:"createdAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 1 || list[0].Name != "prod" || list[0].Color != "blue" ||
		list[0].AgentCount != 1 || list[0].CreatedAt == 0 {
		t.Errorf("list = %+v", list)
	}
}

func TestCovAgentTagCreateErrors(t *testing.T) {
	s := covNewServer(t)

	// 坏 JSON → 400
	rec := covTagsCall(s, "POST", "/api/agent-tags", "{")
	covWantCode(t, "bad json", rec, 400)

	// 空名 → 400（handler 侧校验，不达 storage）
	rec = covTagsCall(s, "POST", "/api/agent-tags", `{"name":"  "}`)
	covWantCode(t, "blank name", rec, 400)

	// 重复 → 409
	covTagsCreate(t, s, "prod")
	rec = covTagsCall(s, "POST", "/api/agent-tags", `{"name":"prod"}`)
	covWantCode(t, "duplicate", rec, 409)

	// 其他存储错误 → 400（错误透传 err.Error()）
	covCloseDB(t, s)
	rec = covTagsCall(s, "POST", "/api/agent-tags", `{"name":"later"}`)
	covWantCode(t, "db error", rec, 400)
}

func TestCovAgentTagUpdateDelete(t *testing.T) {
	s := covNewServer(t)
	id := covTagsCreate(t, s, "prod")
	other := covTagsCreate(t, s, "db")

	// 重命名 + 改色 → 200
	rec := covTagsCall(s, "PUT", "/api/agent-tags/"+id, `{"name":"production","color":"red"}`)
	covWantCode(t, "update", rec, 200)

	// 坏 JSON → 400
	rec = covTagsCall(s, "PUT", "/api/agent-tags/"+id, "{")
	covWantCode(t, "update bad json", rec, 400)

	// 改名撞别的标签 → 409
	rec = covTagsCall(s, "PUT", "/api/agent-tags/"+id, `{"name":"db"}`)
	covWantCode(t, "update clash", rec, 409)

	// 不存在 → 404
	rec = covTagsCall(s, "PUT", "/api/agent-tags/nope", `{"name":"x"}`)
	covWantCode(t, "update missing", rec, 404)

	// 删除 → 200；重复删 → 404
	rec = covTagsCall(s, "DELETE", "/api/agent-tags/"+other, "")
	covWantCode(t, "delete", rec, 200)
	rec = covTagsCall(s, "DELETE", "/api/agent-tags/"+other, "")
	covWantCode(t, "delete missing", rec, 404)

	// DB 关闭 → 更新/删除通用错误 500
	covCloseDB(t, s)
	rec = covTagsCall(s, "PUT", "/api/agent-tags/"+id, `{"name":"prod2"}`)
	covWantCode(t, "update closed", rec, 500)
	rec = covTagsCall(s, "DELETE", "/api/agent-tags/"+id, "")
	covWantCode(t, "delete closed", rec, 500)
}

// TestCovListTagsCountError ListTags 成功但 TagCounts 失败（关联表被删）
// → 「Failed to count tags」500 分支
func TestCovListTagsCountError(t *testing.T) {
	s := covNewServer(t)
	covTagsCreate(t, s, "prod")
	if err := s.db.Session().Exec("DROP TABLE agent_tag_assignments").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	rec := covTagsCall(s, "GET", "/api/agent-tags", "")
	covWantCode(t, "count error", rec, 500)
	if !strings.Contains(rec.Body.String(), "Failed to count tags") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// TestCovListTagsError ListTags 本身失败 → 500
func TestCovListTagsError(t *testing.T) {
	s := covNewServer(t)
	covCloseDB(t, s)
	rec := covTagsCall(s, "GET", "/api/agent-tags", "")
	covWantCode(t, "list error", rec, 500)
}

func TestCovAgentTagAssign(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	id := covTagsCreate(t, s, "prod")

	// GET 单机标签 → 空数组
	rec := covAssignCall(s, "GET", "a1/tags", "")
	covWantCode(t, "get empty", rec, 200)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty body = %s", rec.Body.String())
	}

	// PUT 覆盖设置 → 200；回读带上
	rec = covAssignCall(s, "PUT", "a1/tags", `{"tagIds":["`+id+`"]}`)
	covWantCode(t, "set", rec, 200)
	rec = covAssignCall(s, "GET", "a1/tags", "")
	covWantCode(t, "get after set", rec, 200)
	var tags []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil || len(tags) != 1 ||
		tags[0].Name != "prod" {
		t.Fatalf("tags = %s, err %v", rec.Body.String(), err)
	}

	// 未知 tag（agent 存在）→ 404
	rec = covAssignCall(s, "PUT", "a1/tags", `{"tagIds":["ghost"]}`)
	covWantCode(t, "unknown tag", rec, 404)

	// agent 缺失 → storage.ErrNotFound（与 ErrTagNotFound 不同）→ 500 兜底
	rec = covAssignCall(s, "PUT", "ghost/tags", `{"tagIds":["`+id+`"]}`)
	covWantCode(t, "missing agent", rec, 500)

	// 坏 JSON → 400
	rec = covAssignCall(s, "PUT", "a1/tags", "{")
	covWantCode(t, "bad json", rec, 400)

	// 非法方法 → 405（分发 default）
	rec = covAssignCall(s, "PATCH", "a1/tags", "")
	covWantCode(t, "patch not allowed", rec, 405)

	// DB 关闭 → 读/写通用错误 500
	covCloseDB(t, s)
	rec = covAssignCall(s, "GET", "a1/tags", "")
	covWantCode(t, "get closed", rec, 500)
	rec = covAssignCall(s, "PUT", "a1/tags", `{"tagIds":[]}`)
	covWantCode(t, "set closed", rec, 500)
}
