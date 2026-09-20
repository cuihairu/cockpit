package server

// cov_agent_domains_test.go D7 agent 域名引用端点覆盖：
// /agents/{id}/domains 清单与 /snippet 配置片段。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

// agentDomainsGet 调 /api/agents/{id}/domains 系端点
func agentDomainsGet(s *Server, agentID, suffix string) *httptest.ResponseRecorder {
	_, req := doAuthenticatedRequest(s, http.MethodGet, "/api/agents/"+agentID+"/domains"+suffix, nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	return rec
}

func TestAgentDomainsList(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	for _, b := range []struct{ domain, target string }{
		{"blog.example.com", "127.0.0.1:8080"},
		{"nas.example.com", "docker://nas"},
	} {
		if rec := postBinding(t, s, bindingBody(b.domain, "a1", b.target)); rec.Code != http.StatusOK {
			t.Fatalf("save %s: %s", b.domain, rec.Body)
		}
	}

	rec := agentDomainsGet(s, "a1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d, body = %s", rec.Code, rec.Body)
	}
	var list struct {
		Data []*storage.DomainBinding `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list.Data) != 2 {
		t.Fatalf("items = %d, want 2", len(list.Data))
	}
	if list.Data[0].Domain != "blog.example.com" || list.Data[0].Target != "127.0.0.1:8080" {
		t.Errorf("first item = %+v", list.Data[0])
	}

	// trailing slash 同义
	if rec := agentDomainsGet(s, "a1", "/"); rec.Code != http.StatusOK {
		t.Errorf("trailing slash code = %d", rec.Code)
	}

	// agent 不存在 → 404（先于方法判定）
	if rec := agentDomainsGet(s, "ghost", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown agent code = %d, want 404", rec.Code)
	}

	// 非 GET → 405
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/agents/a1/domains", nil)
	rec2 := httptest.NewRecorder()
	s.serveAPI(rec2, req)
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST code = %d, want 405", rec2.Code)
	}

	// 未知子路径 → 404
	if rec := agentDomainsGet(s, "a1", "/xyz"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub code = %d, want 404", rec.Code)
	}

	// 库故障 → 500（清单查询先行）
	s.db.Close()
	if rec := agentDomainsGet(s, "a1", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("closed db code = %d, want 500", rec.Code)
	}
}

func TestAgentDomainsSnippet(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	for _, body := range []string{
		`{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:8080"}`,
		`{"domain":"nas.example.com","agentId":"a1","target":"docker://nas"}`,
		`{"domain":"off.example.com","agentId":"a1","target":"127.0.0.1:9","enabled":false}`,
	} {
		if rec := postBinding(t, s, body); rec.Code != http.StatusOK {
			t.Fatalf("save: %s", rec.Body)
		}
	}

	rec := agentDomainsGet(s, "a1", "/snippet")
	if rec.Code != http.StatusOK {
		t.Fatalf("snippet code = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content-type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `COCKPIT_AGENT_DOMAINS="blog.example.com nas.example.com"`) {
		t.Errorf("env line missing:\n%s", body)
	}
	if !strings.Contains(body, "server_name blog.example.com;") ||
		!strings.Contains(body, "server_name nas.example.com;") {
		t.Errorf("nginx lines missing:\n%s", body)
	}
	if strings.Contains(body, "off.example.com") {
		t.Errorf("disabled binding should be excluded:\n%s", body)
	}

	// 空清单
	seedBindingAgent(t, s, "a2", "203.0.113.11")
	rec = agentDomainsGet(s, "a2", "/snippet")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no enabled domain bindings") {
		t.Errorf("empty snippet = %d %q", rec.Code, rec.Body.String())
	}
}
