package github

// cov_client_test.go 覆盖 client.go 各 API 的错误分支与分页查询拼接：
// doRequest 的建请求/网络/读响应错误，逐接口的 HTTP 错误与 JSON 解析错误，
// TriggerWorkflow 的 marshal 错误，以及 GetDuration 的零值时间分支。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// covGitHubClient 起 mock server 并返回指向它的 client
func covGitHubClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(Config{Token: "cov-token", BaseURL: srv.URL})
}

// covAlways 返回固定状态码 + body 的 handler
func covAlways(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

// covTruncatedBody 声明 1000 字节但只写 2 字节：客户端 io.ReadAll 报 unexpected EOF
func covTruncatedBody() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{}`))
	}
}

func TestCovDoRequestCreateError(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://example.com"})
	// 非法 method 触发 http.NewRequestWithContext 失败
	if _, err := c.doRequest(context.Background(), "GE T", "/", nil); err == nil {
		t.Fatal("invalid method should error")
	}
}

func TestCovDoRequestNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	c := NewClient(Config{BaseURL: srv.URL})
	if _, err := c.doRequest(context.Background(), "GET", "/", nil); err == nil {
		t.Fatal("dead server should error")
	}
}

func TestCovDoRequestReadError(t *testing.T) {
	c := covGitHubClient(t, covTruncatedBody())
	if _, err := c.doRequest(context.Background(), "GET", "/", nil); err == nil {
		t.Fatal("truncated body should fail io.ReadAll")
	}
}

func TestCovListWorkflowRunsPageOnly(t *testing.T) {
	c := covGitHubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "page=3" {
			t.Errorf("query = %q, want page=3", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	})
	if runs, err := c.ListWorkflowRuns(context.Background(), "o", "r", ListOptions{Page: 3}); err != nil || len(runs) != 0 {
		t.Fatalf("runs = %v, err = %v", runs, err)
	}
}

func TestCovGetWorkflowRunErrors(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if _, err := c.GetWorkflowRun(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("500 should error")
	}
	c2 := covGitHubClient(t, covAlways(200, `not json`))
	if _, err := c2.GetWorkflowRun(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovRerunWorkflowRunError(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if err := c.RerunWorkflowRun(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("500 should error")
	}
}

func TestCovCancelWorkflowRunError(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if err := c.CancelWorkflowRun(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("500 should error")
	}
}

func TestCovListJobsForWorkflowRunErrors(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if _, err := c.ListJobsForWorkflowRun(context.Background(), "o", "r", 1, ListOptions{}); err == nil {
		t.Fatal("500 should error")
	}
	c2 := covGitHubClient(t, covAlways(200, `not json`))
	if _, err := c2.ListJobsForWorkflowRun(context.Background(), "o", "r", 1, ListOptions{}); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovGetRepositoryErrors(t *testing.T) {
	c := covGitHubClient(t, covAlways(404, `missing`))
	if _, err := c.GetRepository(context.Background(), "o", "r"); err == nil {
		t.Fatal("404 should error")
	}
	c2 := covGitHubClient(t, covAlways(200, `not json`))
	if _, err := c2.GetRepository(context.Background(), "o", "r"); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovGetWorkflowRunStatusError(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if _, err := c.GetWorkflowRunStatus(context.Background(), "o", "r"); err == nil {
		t.Fatal("500 should error")
	}
}

func TestCovTriggerWorkflowMarshalError(t *testing.T) {
	c := covGitHubClient(t, covAlways(200, `{}`))
	err := c.TriggerWorkflow(context.Background(), "o", "r", "ci.yml", TriggerWorkflowOptions{
		Ref:    "main",
		Inputs: map[string]interface{}{"bad": make(chan int)},
	})
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("err = %v, want marshal request", err)
	}
}

func TestCovTriggerWorkflowRequestError(t *testing.T) {
	c := covGitHubClient(t, covAlways(422, `unprocessable`))
	if err := c.TriggerWorkflow(context.Background(), "o", "r", "ci.yml", TriggerWorkflowOptions{Ref: "main"}); err == nil {
		t.Fatal("422 should error")
	}
}

func TestCovListArtifactsPageOnlyAndErrors(t *testing.T) {
	c := covGitHubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "page=4" {
			t.Errorf("query = %q, want page=4", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"total_count":0,"artifacts":[]}`))
	})
	if arts, err := c.ListArtifacts(context.Background(), "o", "r", ListOptions{Page: 4}); err != nil || len(arts) != 0 {
		t.Fatalf("artifacts = %v, err = %v", arts, err)
	}

	c2 := covGitHubClient(t, covAlways(500, `boom`))
	if _, err := c2.ListArtifacts(context.Background(), "o", "r", ListOptions{}); err == nil {
		t.Fatal("500 should error")
	}
	c3 := covGitHubClient(t, covAlways(200, `not json`))
	if _, err := c3.ListArtifacts(context.Background(), "o", "r", ListOptions{}); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovListWorkflowArtifactsErrors(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if _, err := c.ListWorkflowArtifacts(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("500 should error")
	}
	c2 := covGitHubClient(t, covAlways(200, `not json`))
	if _, err := c2.ListWorkflowArtifacts(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovDeleteArtifactError(t *testing.T) {
	c := covGitHubClient(t, covAlways(500, `boom`))
	if err := c.DeleteArtifact(context.Background(), "o", "r", 1); err == nil {
		t.Fatal("500 should error")
	}
}

func TestCovGetRateLimitErrors(t *testing.T) {
	c := covGitHubClient(t, covAlways(403, `forbidden`))
	if _, err := c.GetRateLimit(context.Background()); err == nil {
		t.Fatal("403 should error")
	}
	c2 := covGitHubClient(t, covAlways(200, `not json`))
	if _, err := c2.GetRateLimit(context.Background()); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovGetDurationZeroUpdatedAt(t *testing.T) {
	// 已完成但 UpdatedAt 零值：走 time.Since 兜底
	completed := &WorkflowRun{Status: "completed", StartedAt: time.Now().Add(-time.Minute)}
	if d := completed.GetDuration(); d < 30*time.Second || d > 5*time.Minute {
		t.Errorf("completed+zero UpdatedAt duration = %v", d)
	}
	// 未完成且 UpdatedAt 零值：同样走兜底
	pending := &WorkflowRun{Status: "queued", StartedAt: time.Now().Add(-time.Minute)}
	if d := pending.GetDuration(); d < 30*time.Second || d > 5*time.Minute {
		t.Errorf("pending+zero UpdatedAt duration = %v", d)
	}
}
