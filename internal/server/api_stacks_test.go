package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestParseStacksRoutes(t *testing.T) {
	// 通过 handleStacks 的路由分支间接验证：用不存在的 agent 打各条路由，
	// 断言「路由命中并转发」时返回 404（agent 缺失），而非法路由返回 400/405
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"aggregate", http.MethodGet, "/api/stacks", http.StatusOK},
		{"agent list", http.MethodGet, "/api/stacks/agents/a1", http.StatusNotFound},
		{"status", http.MethodGet, "/api/stacks/agents/a1/web", http.StatusNotFound},
		{"compose get", http.MethodGet, "/api/stacks/agents/a1/web/compose", http.StatusNotFound},
		{"compose put", http.MethodPut, "/api/stacks/agents/a1/web/compose", http.StatusNotFound},
		{"up", http.MethodPost, "/api/stacks/agents/a1/web/up", http.StatusNotFound},
		{"down", http.MethodPost, "/api/stacks/agents/a1/web/down", http.StatusNotFound},
		{"logs", http.MethodGet, "/api/stacks/agents/a1/web/logs?tail=50", http.StatusNotFound},
		{"task get", http.MethodGet, "/api/stacks/agents/a1/tasks/t1", http.StatusNotFound},
		{"remove", http.MethodDelete, "/api/stacks/agents/a1/web", http.StatusNotFound},
		{"bad prefix", http.MethodGet, "/api/stacks/bogus", http.StatusBadRequest},
		{"too deep", http.MethodGet, "/api/stacks/agents/a1/web/bogus/x", http.StatusBadRequest},
		{"unknown action", http.MethodPost, "/api/stacks/agents/a1/web/bogus", http.StatusBadRequest},
		{"task wrong method", http.MethodPost, "/api/stacks/agents/a1/tasks/t1", http.StatusMethodNotAllowed},
		{"aggregate wrong method", http.MethodPost, "/api/stacks", http.StatusMethodNotAllowed},
	}

	db := testServerDB(t)
	s := &Server{registry: NewRegistry(), db: db}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			s.handleStacks(rec, req)
			if rec.Code != tt.want {
				t.Errorf("%s %s code = %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

// TestParseStacksBadAgentID httptest.NewRequest 拒绝非法 URL 转义，
// 用手工构造的 Request 验证 agent id 解码失败的 400 分支
func TestParseStacksBadAgentID(t *testing.T) {
	s := &Server{registry: NewRegistry(), db: testServerDB(t)}
	req := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/stacks/agents/%zz/web"}}
	rec := httptest.NewRecorder()
	s.handleStacks(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad agent id code = %d, want 400", rec.Code)
	}
}

// testServerDB 为 server 测试准备临时数据库
func testServerDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(storage.Config{Path: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSplitStacksPath(t *testing.T) {
	if got := splitStacksPath("/api/stacks"); got != nil {
		t.Errorf("splitStacksPath(no slash) = %v, want nil", got)
	}
	if got := splitStacksPath("/api/stacks/"); got != nil {
		t.Errorf("splitStacksPath(root) = %v, want nil", got)
	}
	got := splitStacksPath("/api/stacks/agents/a1/web/compose")
	if len(got) != 4 || got[0] != "agents" || got[1] != "a1" || got[2] != "web" || got[3] != "compose" {
		t.Errorf("splitStacksPath = %v", got)
	}
}

func TestMapAccessors(t *testing.T) {
	m := map[string]interface{}{"s": "x", "i": float64(3), "l": float64(1700000000)}
	if mapString(m, "s") != "x" || mapString(m, "missing") != "" {
		t.Error("mapString failed")
	}
	if mapInt(m, "i") != 3 || mapInt(m, "missing") != 0 {
		t.Error("mapInt failed")
	}
	if mapInt64(m, "l") != 1700000000 || mapInt64(m, "missing") != 0 {
		t.Error("mapInt64 failed")
	}
}

func TestStackViewFromItem(t *testing.T) {
	s := &Server{registry: NewRegistry()}
	agent := &Agent{ID: "a1", Hostname: "host1"}
	item := map[string]interface{}{
		"name":           "web",
		"running":        float64(2),
		"total":          float64(3),
		"lastAction":     "up",
		"lastStatus":     "success",
		"lastDeployedAt": float64(1700000000),
		"services": []interface{}{
			map[string]interface{}{"name": "nginx", "state": "running"},
		},
	}

	view := s.stackViewFromItem(agent, item, true)
	if view.AgentID != "a1" || view.AgentName != "host1" || view.Name != "web" {
		t.Errorf("view header = %+v", view)
	}
	if view.Running != 2 || view.Total != 3 || view.LastDeployedAt != 1700000000 || !view.Online {
		t.Errorf("view body = %+v", view)
	}
	if len(view.Services) != 1 || view.Services[0]["name"] != "nginx" {
		t.Errorf("view services = %+v", view.Services)
	}
}
