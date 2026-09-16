package server

// api_docker.go 的覆盖率补充测试：registerDockerAPI、handleDocker 各错误分支、
// parseDockerRequest/Container/Image/Volume 路由解析、requestParams 与 query 辅助函数。

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// ============ registerDockerAPI ============

func TestCovRegisterDockerAPI(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload([]interface{}{})
	})
	mux := http.NewServeMux()
	s.registerDockerAPI(mux)

	// 无认证 → 401
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/docker/agents/a1/containers", nil))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	// 带认证 → 200
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodGet, "/api/docker/agents/a1/containers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "with auth", rec, http.StatusOK)
}

// ============ handleDocker 错误分支 ============

func TestCovDockerHandleErrors(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"ok": true})
	})
	covFakeAgent(t, s, "plain", nil, nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"short path", http.MethodGet, "/api/docker/x", "", http.StatusBadRequest},
		{"not agents prefix", http.MethodGet, "/api/docker/foo/bar/baz", "", http.StatusBadRequest},
		{"agent missing", http.MethodGet, "/api/docker/agents/ghost/containers", "", http.StatusNotFound},
		{"no docker capability", http.MethodGet, "/api/docker/agents/plain/containers", "", http.StatusNotFound},
		{"invalid body", http.MethodPost, "/api/docker/agents/a1/containers", "not-json", http.StatusBadRequest},
		{"empty body containers", http.MethodPost, "/api/docker/agents/a1/containers", "", http.StatusBadRequest}, // container id required
		{"container id required len3", http.MethodPost, "/api/docker/agents/a1/containers/stop", "", http.StatusBadRequest},
		{"unsupported container route", http.MethodPut, "/api/docker/agents/a1/containers/c1", "", http.StatusBadRequest},
		{"unsupported container action", http.MethodGet, "/api/docker/agents/a1/containers/c1/bogus", "", http.StatusBadRequest},
		{"pull no ref", http.MethodPost, "/api/docker/agents/a1/images/pull", "", http.StatusBadRequest},
		{"unsupported image route", http.MethodPost, "/api/docker/agents/a1/images/x", "", http.StatusBadRequest},
		{"unsupported volume route", http.MethodPost, "/api/docker/agents/a1/volumes", "", http.StatusBadRequest},
		{"networks wrong method", http.MethodPost, "/api/docker/agents/a1/networks", "", http.StatusBadRequest},
		{"system short path", http.MethodGet, "/api/docker/agents/a1/system", "", http.StatusBadRequest},
		{"system wrong tail", http.MethodGet, "/api/docker/agents/a1/system/bogus", "", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body *strings.Reader
			if c.body != "" {
				body = strings.NewReader(c.body)
			} else {
				body = strings.NewReader("")
			}
			rec := covRec()
			s.handleDocker(rec, covReq(c.method, c.path, body))
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// 非法 URL 转义：agent id / container id / image id / volume name → 400
	for _, path := range []string{
		"/api/docker/agents/%zz/containers",
		"/api/docker/agents/a1/containers/%zz",
		"/api/docker/agents/a1/images/%zz",
		"/api/docker/agents/a1/volumes/%zz",
	} {
		method := http.MethodGet
		if strings.Contains(path, "images") || strings.Contains(path, "volumes") {
			method = http.MethodDelete
		}
		req := &http.Request{Method: method, URL: &url.URL{Path: path}}
		rec := covRec()
		s.handleDocker(rec, req)
		covWantCode(t, path, rec, http.StatusBadRequest)
	}

	// 应答非法（status 非字符串）→ decode err 502
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec := covRec()
	s2.handleDocker(rec, covReq(http.MethodGet, "/api/docker/agents/a1/containers", nil))
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	// rpc error → 502
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("docker boom")
	})
	rec = covRec()
	s3.handleDocker(rec, covReq(http.MethodGet, "/api/docker/agents/a1/containers", nil))
	covWantCode(t, "rpc error", rec, http.StatusBadGateway)
}

// ============ 各资源动作成功路径（参数构造）============

func TestCovDockerActions(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"method": method, "params": params})
	})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"container stop", http.MethodPost, "/api/docker/agents/a1/containers/c1/stop?timeout=10", ""},
		{"container restart", http.MethodPost, "/api/docker/agents/a1/containers/c1/restart?timeout=5", ""},
		{"container pause", http.MethodPost, "/api/docker/agents/a1/containers/c1/pause", ""},
		{"container unpause", http.MethodPost, "/api/docker/agents/a1/containers/c1/unpause", ""},
		{"container logs", http.MethodGet, "/api/docker/agents/a1/containers/c1/logs?tail=100&since=1h&follow=true&timestamps=true&stdout=false&stderr=false", ""},
		{"container stats", http.MethodGet, "/api/docker/agents/a1/containers/c1/stats", ""},
		{"image pull", http.MethodPost, "/api/docker/agents/a1/images/pull?ref=nginx:latest", ""},
		{"image pull body", http.MethodPost, "/api/docker/agents/a1/images/pull", `{"ref":"nginx:1.27"}`},
		{"image remove", http.MethodDelete, "/api/docker/agents/a1/images/img1?force=true&prune_children=true", ""},
		{"volume remove", http.MethodDelete, "/api/docker/agents/a1/volumes/vol1?force=true", ""},
		{"networks list", http.MethodGet, "/api/docker/agents/a1/networks", ""},
		{"system info", http.MethodGet, "/api/docker/agents/a1/system/info", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body *strings.Reader
			if c.body != "" {
				body = strings.NewReader(c.body)
			} else {
				body = strings.NewReader("")
			}
			rec := covRec()
			s.handleDocker(rec, covReq(c.method, c.path, body))
			covWantCode(t, c.name, rec, http.StatusOK)
		})
	}
}

// logs 参数默认值：body 已含 stdout 键时 setQueryBoolDefault 直接返回（exists 分支）
func TestCovDockerLogsParamDefaults(t *testing.T) {
	s := covNewServer(t)
	var got map[string]interface{}
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "docker.containers.logs" {
			got = params
		}
		return covOKPayload(nil)
	})

	// body 显式提供 stdout → exists 分支跳过默认值
	rec := covRec()
	s.handleDocker(rec, covReq(http.MethodGet, "/api/docker/agents/a1/containers/c1/logs?tail=50", strings.NewReader(`{"stdout":false}`)))
	covWantCode(t, "logs with body param", rec, http.StatusOK)
	if v, ok := got["stdout"].(bool); !ok || v {
		t.Errorf("stdout = %v, want false from body", got["stdout"])
	}
	// stderr 未显式提供 → 默认 true
	if v, ok := got["stderr"].(bool); !ok || !v {
		t.Errorf("stderr = %v, want default true", got["stderr"])
	}
	if got["tail"] != "50" {
		t.Errorf("tail = %v", got["tail"])
	}

	// 无 body → stdout/stderr 均默认 true；query 覆盖
	got = nil
	rec = covRec()
	s.handleDocker(rec, covReq(http.MethodGet, "/api/docker/agents/a1/containers/c1/logs?stdout=false", nil))
	covWantCode(t, "logs defaults", rec, http.StatusOK)
	if v, ok := got["stdout"].(bool); !ok || v {
		t.Errorf("stdout = %v, want false from query", got["stdout"])
	}
	if v, ok := got["stderr"].(bool); !ok || !v {
		t.Errorf("stderr = %v, want default true", got["stderr"])
	}
}

// ============ send timeout → 504（5s，并行摊销）============

func TestCovDockerTimeout(t *testing.T) {
	t.Parallel()
	s := covNewServer(t)
	covStuckDockerAgent(t, s, "stuck")

	rec := covRec()
	s.handleDocker(rec, covReq(http.MethodGet, "/api/docker/agents/stuck/containers", nil))
	covWantCode(t, "docker timeout", rec, http.StatusGatewayTimeout)
}

// ============ splitDockerPath / setQuery 系列 ============

func TestCovSplitDockerPath(t *testing.T) {
	if got := splitDockerPath("/api/docker/"); got != nil {
		t.Errorf("splitDockerPath(root) = %v, want nil", got)
	}
	got := splitDockerPath("/api/docker/agents/a1/containers")
	if len(got) != 3 || got[0] != "agents" || got[1] != "a1" || got[2] != "containers" {
		t.Errorf("splitDockerPath = %v", got)
	}
}

func TestCovSetQueryHelpers(t *testing.T) {
	// setQueryBoolDefault：已存在 → 直接返回；不存在 → 默认值，query 可覆盖
	params := map[string]interface{}{"stdout": false}
	setQueryBoolDefault(params, covReq(http.MethodGet, "/x?stdout=true", nil), "stdout", true)
	if v, _ := params["stdout"].(bool); v {
		t.Errorf("existing key should not be overwritten: %v", params["stdout"])
	}

	params = map[string]interface{}{}
	setQueryBoolDefault(params, covReq(http.MethodGet, "/x", nil), "stderr", true)
	if v, _ := params["stderr"].(bool); !v {
		t.Errorf("default missing: %v", params["stderr"])
	}

	params = map[string]interface{}{}
	setQueryBoolDefault(params, covReq(http.MethodGet, "/x?stderr=false", nil), "stderr", true)
	if v, _ := params["stderr"].(bool); v {
		t.Errorf("query should override default: %v", params["stderr"])
	}

	// setQueryBool：非法布尔值忽略
	params = map[string]interface{}{}
	setQueryBool(params, covReq(http.MethodGet, "/x?all=notabool", nil), "all")
	if _, ok := params["all"]; ok {
		t.Errorf("invalid bool should be ignored: %v", params["all"])
	}
	setQueryBool(params, covReq(http.MethodGet, "/x?all=1", nil), "all")
	if v, _ := params["all"].(bool); !v {
		t.Errorf("all = %v, want true", params["all"])
	}

	// setQueryInt：非法整数忽略
	params = map[string]interface{}{}
	setQueryInt(params, covReq(http.MethodGet, "/x?timeout=abc", nil), "timeout")
	if _, ok := params["timeout"]; ok {
		t.Errorf("invalid int should be ignored: %v", params["timeout"])
	}
	setQueryInt(params, covReq(http.MethodGet, "/x?timeout=9", nil), "timeout")
	if v, _ := params["timeout"].(int); v != 9 {
		t.Errorf("timeout = %v, want 9", params["timeout"])
	}

	// setQueryString：空值忽略
	params = map[string]interface{}{}
	setQueryString(params, covReq(http.MethodGet, "/x?ref=", nil), "ref")
	if _, ok := params["ref"]; ok {
		t.Errorf("empty string should be ignored: %v", params["ref"])
	}
	setQueryString(params, covReq(http.MethodGet, "/x?ref=nginx", nil), "ref")
	if params["ref"] != "nginx" {
		t.Errorf("ref = %v", params["ref"])
	}
}

// requestParams：EOF（空 body）与正常 body
func TestCovRequestParams(t *testing.T) {
	params, err := requestParams(covReq(http.MethodPost, "/x", strings.NewReader("")))
	if err != nil || len(params) != 0 {
		t.Errorf("empty body: %v %v", params, err)
	}
	params, err = requestParams(covReq(http.MethodPost, "/x", strings.NewReader(`{"a":1}`)))
	if err != nil || params["a"] == nil {
		t.Errorf("json body: %v %v", params, err)
	}
}
