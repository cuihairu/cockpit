package server

// 各 agent 转发 API 与本地 API 的覆盖率补充测试：
// api_files / api_cron / api_logs / api_drift / api_proxy_sites /
// api_recordings / api_server_backup / api_dns / api_metrics。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ api_files.go ============

func TestCovFilesAPIDispatch(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"ok": true})
	})

	cases := []struct {
		name   string
		method string
		rest   string
		want   int
	}{
		{"no files suffix", http.MethodGet, "a1/whatever", http.StatusNotFound},
		{"empty agent", http.MethodPost, "/files/list", http.StatusNotFound},
		{"empty action", http.MethodPost, "a1/files/", http.StatusNotFound},
		{"agent offline", http.MethodPost, "ghost/files/list", http.StatusServiceUnavailable},
		{"list wrong method", http.MethodGet, "a1/files/list", http.StatusMethodNotAllowed},
		{"download wrong method", http.MethodPost, "a1/files/download", http.StatusMethodNotAllowed},
		{"unknown action", http.MethodPost, "a1/files/bogus", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleAgentFilesAPI(rec, covReq(c.method, "/api/agents/"+strings.TrimPrefix(c.rest, "/"), strings.NewReader(`{"dir":"/tmp"}`)), c.rest)
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

func TestCovForwardFileRPCValidation(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"ok": true})
	})

	// bad json → 400
	rec := covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader("not-json")), "a1", "file.list", "list")
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// list：dir 非绝对 → 400
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"dir":"rel"}`)), "a1", "file.list", "list")
	covWantCode(t, "list relative dir", rec, http.StatusBadRequest)

	// read：path 非绝对 → 400
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"path":"rel"}`)), "a1", "file.read", "read")
	covWantCode(t, "read relative path", rec, http.StatusBadRequest)

	// rename：path 非绝对 → 400
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"path":"rel","name":"b"}`)), "a1", "file.rename", "rename")
	covWantCode(t, "rename relative path", rec, http.StatusBadRequest)

	// rename：非法 name（含分隔符）→ 400
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"path":"/a/b","name":"x/y"}`)), "a1", "file.rename", "rename")
	covWantCode(t, "rename bad name", rec, http.StatusBadRequest)

	// write：base64 非法 → 400
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"path":"/a/b","data":"!!!"}`)), "a1", "file.write", "write")
	covWantCode(t, "write bad base64", rec, http.StatusBadRequest)

	// write：解码后超过 1MB → 400
	big := base64.StdEncoding.EncodeToString(make([]byte, 1024*1025))
	body, _ := json.Marshal(map[string]string{"path": "/a/b", "data": big})
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(string(body))), "a1", "file.write", "write")
	covWantCode(t, "write chunk too large", rec, http.StatusBadRequest)
}

func TestCovForwardFileRPCResults(t *testing.T) {
	// read：显式 offset/length 透传
	s := covNewServer(t)
	var gotParams map[string]interface{}
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		gotParams = params
		return covOKPayload(map[string]interface{}{"data": "eA=="})
	})
	rec := covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"path":"/a/b","offset":12,"length":64}`)), "a1", "file.read", "read")
	covWantCode(t, "read ok", rec, http.StatusOK)
	if gotParams["offset"] != float64(12) || gotParams["length"] != float64(64) {
		t.Errorf("read params = %v", gotParams)
	}

	// write 成功（带审计用户名）+ truncate 显式 false
	req := covAuthReq(http.MethodPost, "/x", strings.NewReader(`{"path":"/a/b","data":"eA==","truncate":false}`), "1", "admin", "admin")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.forwardFileRPC(w, r, "a1", "file.write", "write")
	}, req)
	covWantCode(t, "write ok", rec, http.StatusOK)
	if gotParams["truncate"] != false {
		t.Errorf("write truncate = %v", gotParams["truncate"])
	}

	// agent 不在线 → CallAgent err → 502
	rec = covRec()
	s.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"dir":"/tmp"}`)), "ghost", "file.list", "list")
	covWantCode(t, "agent offline", rec, http.StatusBadGateway)

	// decode err → 502
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"dir":"/tmp"}`)), "a1", "file.list", "list")
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	// rpc error → 502
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("denied")
	})
	rec = covRec()
	s3.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"dir":"/tmp"}`)), "a1", "file.list", "list")
	covWantCode(t, "rpc error", rec, http.StatusBadGateway)

	// data 非 map → 空 map 兜底
	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload("plain string")
	})
	rec = covRec()
	s4.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"dir":"/tmp"}`)), "a1", "file.list", "list")
	covWantCode(t, "non-map data", rec, http.StatusOK)

	// mkdir/delete/rename 成功审计（read/list 不审计）
	for _, action := range []string{"mkdir", "delete", "rename"} {
		sx := covNewServer(t)
		covFakeAgent(t, sx, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
			return covOKPayload(nil)
		})
		body := `{"path":"/a/b"}`
		if action == "rename" {
			body = `{"path":"/a/b","name":"c"}`
		}
		rec := covRec()
		sx.forwardFileRPC(rec, covReq(http.MethodPost, "/x", strings.NewReader(body)), "a1", "file."+action, action)
		covWantCode(t, action+" ok", rec, http.StatusOK)
	}
}

func TestCovFileDownloadBranches(t *testing.T) {
	// path 非法 → 400
	s := covNewServer(t)
	rec := covRec()
	s.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=rel", nil), "a1")
	covWantCode(t, "bad path", rec, http.StatusBadRequest)

	// agent 离线 → 首块 err → 502
	rec = covRec()
	s.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "ghost")
	covWantCode(t, "agent offline", rec, http.StatusBadGateway)

	// decode err → 502
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	// rpc error → 502
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("boom")
	})
	rec = covRec()
	s3.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")
	covWantCode(t, "rpc error", rec, http.StatusBadGateway)

	// base64 非法 → 502
	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"data": "!!!", "size": 10, "eof": true})
	})
	rec = covRec()
	s4.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")
	covWantCode(t, "bad base64", rec, http.StatusBadGateway)

	// 成功：多块 + Content-Length + eof break
	s5 := covNewServer(t)
	covFakeAgent(t, s5, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if off, _ := params["offset"].(float64); off == 0 {
			return covOKPayload(map[string]interface{}{"data": "eA==", "size": 2, "eof": false})
		}
		return covOKPayload(map[string]interface{}{"data": "eQ==", "size": 2, "eof": true})
	})
	rec = covRec()
	s5.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")
	if rec.Code != http.StatusOK || rec.Body.String() != "xy" {
		t.Errorf("download: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if cl := rec.Header().Get("Content-Length"); cl != "2" {
		t.Errorf("Content-Length = %q", cl)
	}

	// 首块空数据 + eof → 直接 break（空文件，total=0 不设 Content-Length）
	s6 := covNewServer(t)
	covFakeAgent(t, s6, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"data": "", "size": 0, "eof": true})
	})
	rec = covRec()
	s6.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("empty download: code=%d body=%q", rec.Code, rec.Body.String())
	}

	// 中途第二块失败 → 已写 200 后静默截断
	s7 := covNewServer(t)
	covFakeAgent(t, s7, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if off, _ := params["offset"].(float64); off == 0 {
			return covOKPayload(map[string]interface{}{"data": "eA==", "size": 100, "eof": false})
		}
		return covErrPayload("read failed")
	})
	rec = covRec()
	s7.handleFileDownload(rec, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")
	if rec.Code != http.StatusOK || rec.Body.String() != "x" {
		t.Errorf("mid-stream fail: code=%d body=%q", rec.Code, rec.Body.String())
	}

	// 首块写失败 → 直接返回
	s8 := covNewServer(t)
	covFakeAgent(t, s8, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"data": "eA==", "size": 100, "eof": false})
	})
	fw := &covFailWriter{}
	s8.handleFileDownload(fw, covReq(http.MethodGet, "/d?path=/tmp/a.txt", nil), "a1")

	// 客户端断开 → ctx done 分支
	s9 := covNewServer(t)
	covFakeAgent(t, s9, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if off, _ := params["offset"].(float64); off == 0 {
			return covOKPayload(map[string]interface{}{"data": "eA==", "size": 4096, "eof": false})
		}
		time.Sleep(300 * time.Millisecond)
		return covOKPayload(map[string]interface{}{"data": "eA==", "size": 4096, "eof": true})
	})
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/d?path=/tmp/a.txt", nil).WithContext(ctx)
	rec = covRec()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s9.handleFileDownload(rec, req, "a1")
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not return after client abort")
	}
}

// ============ api_cron.go ============

func TestCovValidateCronExpr(t *testing.T) {
	valid := []string{
		"*/5 * * * *", "0 3 * * *", "1-5/2 8-18 * * 1-5", "0 0 1,15 * *", "@reboot", "@hourly",
		"@daily", "@weekly", "@monthly", "@yearly", "@annually", "  0 3 * * *  ", "0 0 * * 0-7",
	}
	for _, expr := range valid {
		if err := validateCronExpr(expr); err != nil {
			t.Errorf("validateCronExpr(%q) = %v, want nil", expr, err)
		}
	}
	invalid := map[string]string{
		"":             "empty",
		"   ":          "blank",
		"@bogus":       "unsupported @",
		"@daily extra": "@ with extra",
		"* * * *":      "4 fields",
		"* * * * * *":  "6 fields",
		"a * * * *":    "bad field",
		"60 * * * *":   "minute out of range",
		"* 24 * * *":   "hour out of range",
		"0 0 0 * *":    "dom out of range",
		"0 0 * 0 *":    "month out of range",
		"8 * * *":      "reversed-ish short",
		"*/0 * * * *":  "step < 1",
		"*/61 * * * *": "step > hi",
		"*/x * * * *":  "step not numeric",
		"1-99 * * * *": "range out of range",
	}
	for expr := range invalid {
		if err := validateCronExpr(expr); err == nil {
			t.Errorf("validateCronExpr(%q) = nil, want error", expr)
		}
	}
}

func TestCovValidateCronJob(t *testing.T) {
	good := &cronPayload{Name: "backup", Schedule: "0 3 * * *", Command: "echo hi"}
	if err := validateCronJob(good); err != nil {
		t.Errorf("valid job: %v", err)
	}
	bad := []*cronPayload{
		{Name: "Bad Name", Schedule: "0 3 * * *", Command: "x"}, // name 大写与空格
		{Name: "-lead", Schedule: "0 3 * * *", Command: "x"},    // name 前导 -
		{Schedule: "0 3 * * *", Command: "x"},                   // name 空
		{Name: "j", Schedule: "bogus", Command: "x"},            // schedule 非法
		{Name: "j", Schedule: "0 3 * * *"},                      // command 空
		{Name: "j", Schedule: "0 3 * * *", Command: "a\nb"},     // command 换行
		{Name: "j", Schedule: "0 3 * * *", Command: "x\r"},      // command \r
	}
	for i, j := range bad {
		if err := validateCronJob(j); err == nil {
			t.Errorf("case %d: want error for %+v", i, j)
		}
	}
	// 超长 command
	long := &cronPayload{Name: "j", Schedule: "0 3 * * *", Command: strings.Repeat("x", 4*1024+1)}
	if err := validateCronJob(long); err == nil {
		t.Error("oversized command should fail")
	}
}

func TestCovCronAPIDispatch(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"jobs": []interface{}{}})
	})

	cases := []struct {
		name   string
		method string
		rest   string
		want   int
	}{
		{"no cron suffix", http.MethodGet, "a1/whatever", http.StatusNotFound},
		{"empty agent", http.MethodGet, "/cron/status", http.StatusNotFound},
		{"empty sub", http.MethodGet, "a1/cron/", http.StatusNotFound},
		{"agent offline", http.MethodGet, "ghost/cron/status", http.StatusServiceUnavailable},
		{"unknown sub", http.MethodGet, "a1/cron/bogus", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleAgentCronAPI(rec, covReq(c.method, "/api/agents/"+c.rest, nil), c.rest)
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// status / jobs 成功
	for _, sub := range []string{"status", "jobs"} {
		rec := covRec()
		s.handleAgentCronAPI(rec, covReq(http.MethodGet, "/api/agents/a1/cron/"+sub, nil), "a1/cron/"+sub)
		covWantCode(t, sub, rec, http.StatusOK)
	}
}

func TestCovCronJobBranches(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"applied": true})
	})

	// 非法 job name → 400
	rec := covRec()
	s.handleCronJob(rec, covReq(http.MethodPut, "/x", nil), "a1", "Bad Name", "")
	covWantCode(t, "bad name", rec, http.StatusBadRequest)

	// 其他方法 → 405
	rec = covRec()
	s.handleCronJob(rec, covReq(http.MethodGet, "/x", nil), "a1", "job1", "")
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// PUT bad json → 400
	rec = covRec()
	s.handleCronJob(rec, covReq(http.MethodPut, "/x", strings.NewReader("not-json")), "a1", "job1", "")
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// URL 与 body name 不一致 → 400
	rec = covRec()
	s.handleCronJob(rec, covReq(http.MethodPut, "/x",
		strings.NewReader(`{"name":"other","schedule":"0 3 * * *","command":"x"}`)), "a1", "job1", "")
	covWantCode(t, "name mismatch", rec, http.StatusBadRequest)

	// 校验失败（schedule 非法）→ 400
	rec = covRec()
	s.handleCronJob(rec, covReq(http.MethodPut, "/x",
		strings.NewReader(`{"name":"job1","schedule":"bogus","command":"x"}`)), "a1", "job1", "")
	covWantCode(t, "invalid schedule", rec, http.StatusBadRequest)

	// PUT 成功（带用户上下文 → 审计 username）
	req := covAuthReq(http.MethodPut, "/x",
		strings.NewReader(`{"name":"job1","schedule":"0 3 * * *","command":"echo hi","enabled":true}`), "1", "admin", "admin")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleCronJob(w, r, "a1", "job1", "")
	}, req)
	covWantCode(t, "apply ok", rec, http.StatusOK)

	// DELETE 成功（审计 cron_delete）
	rec = covRec()
	s.handleCronJob(rec, covReq(http.MethodDelete, "/x", nil), "a1", "job1", "")
	covWantCode(t, "delete ok", rec, http.StatusOK)
}

func TestCovForwardCronRPCBranches(t *testing.T) {
	// agent 离线 → 502
	s := covNewServer(t)
	rec := covRec()
	s.forwardCronRPC(rec, covReq(http.MethodGet, "/x", nil), "ghost", "cron.status", nil, "", nil)
	covWantCode(t, "offline", rec, http.StatusBadGateway)

	// decode err → 502
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.forwardCronRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "cron.status", nil, "", nil)
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	// rpc error 空 message → 默认文案 502
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error", "error": ""}
	})
	rec = covRec()
	s3.forwardCronRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "cron.status", nil, "", nil)
	covWantCode(t, "rpc empty error", rec, http.StatusBadGateway)

	// data 非 map → 空 map 兜底
	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload([]interface{}{1, 2})
	})
	rec = covRec()
	s4.forwardCronRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "cron.status", nil, "", nil)
	covWantCode(t, "non-map data", rec, http.StatusOK)
}

// ============ api_logs.go ============

func TestCovValidateLogsQuery(t *testing.T) {
	good := &logsQueryPayload{Type: "systemd", Source: "nginx.service", Tail: 100, SinceMinutes: 30, Grep: "error"}
	if err := validateLogsQuery(good); err != nil {
		t.Errorf("valid query: %v", err)
	}
	// Tail=0 → 默认 200
	def := &logsQueryPayload{Type: "docker", Source: "web"}
	if err := validateLogsQuery(def); err != nil || def.Tail != 200 {
		t.Errorf("default tail: %v %d", err, def.Tail)
	}
	bad := []*logsQueryPayload{
		{Type: "journal", Source: "x"},             // type 非法
		{Type: "systemd", Source: "-lead"},         // source 非法
		{Type: "systemd", Source: "x", Tail: -1},   // tail 下界
		{Type: "systemd", Source: "x", Tail: 2001}, // tail 上界
		{Type: "systemd", Source: "x", SinceMinutes: -1},
		{Type: "systemd", Source: "x", SinceMinutes: 1441},
		{Type: "systemd", Source: "x", Grep: strings.Repeat("a", 257)},
		{Type: "systemd", Source: "x", Grep: "a\nb"},
	}
	for i, q := range bad {
		if err := validateLogsQuery(q); err == nil {
			t.Errorf("case %d: want error for %+v", i, q)
		}
	}
}

func TestCovLogsAPIDispatch(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"lines": []interface{}{}})
	})

	cases := []struct {
		name   string
		method string
		rest   string
		body   string
		want   int
	}{
		{"no logs suffix", http.MethodGet, "a1/whatever", "", http.StatusNotFound},
		{"empty agent", http.MethodGet, "/logs/status", "", http.StatusNotFound},
		{"empty sub", http.MethodGet, "a1/logs/", "", http.StatusNotFound},
		{"agent offline", http.MethodGet, "ghost/logs/status", "", http.StatusServiceUnavailable},
		{"status wrong method", http.MethodPost, "a1/logs/status", "", http.StatusNotFound},
		{"query bad json", http.MethodPost, "a1/logs/query", "not-json", http.StatusBadRequest},
		{"query invalid", http.MethodPost, "a1/logs/query", `{"type":"bogus","source":"x"}`, http.StatusBadRequest},
		{"unknown sub", http.MethodGet, "a1/logs/bogus", "", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleAgentLogsAPI(rec, covReq(c.method, "/api/agents/"+c.rest, strings.NewReader(c.body)), c.rest)
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// status/sources/query 成功
	for _, c := range []struct{ method, sub, body string }{
		{http.MethodGet, "status", ""},
		{http.MethodGet, "sources", ""},
		{http.MethodPost, "query", `{"type":"systemd","source":"nginx.service","tail":50}`},
	} {
		rec := covRec()
		s.handleAgentLogsAPI(rec, covReq(c.method, "/api/agents/a1/logs/"+c.sub, strings.NewReader(c.body)), "a1/logs/"+c.sub)
		covWantCode(t, c.sub, rec, http.StatusOK)
	}
}

func TestCovForwardLogsRPCBranches(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.forwardLogsRPC(rec, covReq(http.MethodGet, "/x", nil), "ghost", "logs.status", nil)
	covWantCode(t, "offline", rec, http.StatusBadGateway)

	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.forwardLogsRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "logs.status", nil)
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error", "error": ""}
	})
	rec = covRec()
	s3.forwardLogsRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "logs.status", nil)
	covWantCode(t, "rpc empty error", rec, http.StatusBadGateway)

	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload([]interface{}{})
	})
	rec = covRec()
	s4.forwardLogsRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "logs.status", nil)
	covWantCode(t, "non-map data", rec, http.StatusOK)
}

// ============ api_drift.go ============

func TestCovDriftConfigBranches(t *testing.T) {
	s := covNewServer(t)

	// GET → 200
	rec := covRec()
	s.handleDriftConfig(rec, covReq(http.MethodGet, "/api/drift/config", nil))
	covWantCode(t, "get", rec, http.StatusOK)

	// 非 GET/PUT → 405
	rec = covRec()
	s.handleDriftConfig(rec, covReq(http.MethodPost, "/api/drift/config", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// bad json → 400
	rec = covRec()
	s.handleDriftConfig(rec, covReq(http.MethodPut, "/api/drift/config", strings.NewReader("not-json")))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 越界（max=86400）→ 400
	rec = covRec()
	s.handleDriftConfig(rec, covReq(http.MethodPut, "/api/drift/config", strings.NewReader(`{"scan_interval_seconds":999999}`)))
	covWantCode(t, "interval out of range", rec, http.StatusBadRequest)

	// 合法 PUT → 200
	rec = covRec()
	s.handleDriftConfig(rec, covReq(http.MethodPut, "/api/drift/config", strings.NewReader(`{"scan_interval_seconds":7200}`)))
	covWantCode(t, "put ok", rec, http.StatusOK)
}

func TestCovDriftAPIDispatch(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"drifts": []interface{}{}})
	})

	cases := []struct {
		name   string
		method string
		rest   string
		want   int
	}{
		{"no drift suffix", http.MethodGet, "a1/whatever", http.StatusNotFound},
		{"empty agent", http.MethodGet, "/drift/check", http.StatusNotFound},
		{"empty sub", http.MethodGet, "a1/drift/", http.StatusNotFound},
		{"agent offline", http.MethodPost, "ghost/drift/check", http.StatusServiceUnavailable},
		{"check wrong method", http.MethodGet, "a1/drift/check", http.StatusMethodNotAllowed},
		{"unknown sub", http.MethodPost, "a1/drift/bogus", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleAgentDriftAPI(rec, covReq(c.method, "/api/agents/"+c.rest, nil), c.rest)
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// check 成功
	rec := covRec()
	s.handleAgentDriftAPI(rec, covReq(http.MethodPost, "/api/agents/a1/drift/check", nil), "a1/drift/check")
	covWantCode(t, "check ok", rec, http.StatusOK)

	// forwardDriftRPC：离线 / decode err / rpc 空 msg / 非 map data
	rec = covRec()
	s.forwardDriftRPC(rec, covReq(http.MethodPost, "/x", nil), "ghost", "drift.check", nil)
	covWantCode(t, "offline", rec, http.StatusBadGateway)

	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.forwardDriftRPC(rec, covReq(http.MethodPost, "/x", nil), "a1", "drift.check", nil)
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error", "error": ""}
	})
	rec = covRec()
	s3.forwardDriftRPC(rec, covReq(http.MethodPost, "/x", nil), "a1", "drift.check", nil)
	covWantCode(t, "rpc empty error", rec, http.StatusBadGateway)

	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload("string data")
	})
	rec = covRec()
	s4.forwardDriftRPC(rec, covReq(http.MethodPost, "/x", nil), "a1", "drift.check", nil)
	covWantCode(t, "non-map data", rec, http.StatusOK)
}

// ============ api_proxy_sites.go ============

func TestCovValidateProxySite(t *testing.T) {
	good := &proxySitePayload{
		Name:        "web",
		ServerNames: []string{"example.com", "*.example.com"},
		Upstream:    "127.0.0.1:8080",
		Scheme:      "http",
	}
	if err := validateProxySite(good); err != nil {
		t.Errorf("valid site: %v", err)
	}
	https := &proxySitePayload{
		Name: "sec", ServerNames: []string{"s.example.com"}, Upstream: "10.0.0.1:443",
		Scheme: "https", TLSCert: "/etc/cert.pem", TLSKey: "/etc/key.pem",
	}
	if err := validateProxySite(https); err != nil {
		t.Errorf("valid https site: %v", err)
	}
	bad := []*proxySitePayload{
		{Name: "Bad", ServerNames: []string{"a.com"}, Upstream: "u:80"},                // name 非法
		{ServerNames: []string{"a.com"}, Upstream: "u:80"},                             // name 空
		{Name: "s", Upstream: "u:80"},                                                  // serverNames 空
		{Name: "s", ServerNames: covTooManyDomains(), Upstream: "u:80"},                // serverNames 过多
		{Name: "s", ServerNames: []string{"bad name"}, Upstream: "u:80"},               // 域名非法
		{Name: "s", ServerNames: []string{"a.com"}, Upstream: "bad upstream!"},         // upstream 非法
		{Name: "s", ServerNames: []string{"a.com"}, Upstream: "u:80", Scheme: "ftp"},   // scheme 非法
		{Name: "s", ServerNames: []string{"a.com"}, Upstream: "u:80", Scheme: "https"}, // https 缺证书路径
		{Name: "s", ServerNames: []string{"a.com"}, Upstream: "u:80", Scheme: "https", TLSCert: "rel.pem", TLSKey: "/k"},
		{Name: "s", ServerNames: []string{"a.com"}, Upstream: "u:80", Scheme: "http", Extra: strings.Repeat("x", 4*1024+1)},
	}
	for i, p := range bad {
		if err := validateProxySite(p); err == nil {
			t.Errorf("case %d: want error for %+v", i, p.Name)
		}
	}
}

func covTooManyDomains() []string {
	domains := make([]string, 17)
	for i := range domains {
		domains[i] = "d.example.com"
	}
	return domains
}

func TestCovProxyAPIDispatch(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"sites": []interface{}{}})
	})

	cases := []struct {
		name   string
		method string
		rest   string
		want   int
	}{
		{"no proxy suffix", http.MethodGet, "a1/whatever", http.StatusNotFound},
		{"empty agent", http.MethodGet, "/proxy/status", http.StatusNotFound},
		{"empty sub", http.MethodGet, "a1/proxy/", http.StatusNotFound},
		{"agent offline", http.MethodGet, "ghost/proxy/status", http.StatusServiceUnavailable},
		{"unknown sub", http.MethodGet, "a1/proxy/bogus", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleAgentProxyAPI(rec, covReq(c.method, "/api/agents/"+c.rest, nil), c.rest)
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// status / sites 成功
	for _, sub := range []string{"status", "sites"} {
		rec := covRec()
		s.handleAgentProxyAPI(rec, covReq(http.MethodGet, "/api/agents/a1/proxy/"+sub, nil), "a1/proxy/"+sub)
		covWantCode(t, sub, rec, http.StatusOK)
	}
}

func TestCovProxySiteBranches(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"rendered": "server {...}"})
	})

	// 非法站点名 → 400
	rec := covRec()
	s.handleProxySite(rec, covReq(http.MethodGet, "/x", nil), "a1", "Bad Name")
	covWantCode(t, "bad name", rec, http.StatusBadRequest)

	// 其他方法 → 405
	rec = covRec()
	s.handleProxySite(rec, covReq(http.MethodPost, "/x", nil), "a1", "site1")
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// GET 成功（details=nil 无审计）
	rec = covRec()
	s.handleProxySite(rec, covReq(http.MethodGet, "/x", nil), "a1", "site1")
	covWantCode(t, "get ok", rec, http.StatusOK)

	// PUT bad json → 400
	rec = covRec()
	s.handleProxySite(rec, covReq(http.MethodPut, "/x", strings.NewReader("not-json")), "a1", "site1")
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// name 不一致 → 400
	body := `{"name":"other","serverNames":["a.com"],"upstream":"u:80","scheme":"http"}`
	rec = covRec()
	s.handleProxySite(rec, covReq(http.MethodPut, "/x", strings.NewReader(body)), "a1", "site1")
	covWantCode(t, "name mismatch", rec, http.StatusBadRequest)

	// 校验失败 → 400
	body = `{"name":"site1","serverNames":[],"upstream":"u:80","scheme":"http"}`
	rec = covRec()
	s.handleProxySite(rec, covReq(http.MethodPut, "/x", strings.NewReader(body)), "a1", "site1")
	covWantCode(t, "invalid site", rec, http.StatusBadRequest)

	// PUT 成功（审计 username）
	body = `{"name":"site1","serverNames":["a.com"],"upstream":"127.0.0.1:8080","scheme":"http"}`
	req := covAuthReq(http.MethodPut, "/x", strings.NewReader(body), "1", "admin", "admin")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleProxySite(w, r, "a1", "site1")
	}, req)
	covWantCode(t, "apply ok", rec, http.StatusOK)

	// DELETE 成功（审计 proxy_delete）
	rec = covRec()
	s.handleProxySite(rec, covReq(http.MethodDelete, "/x", nil), "a1", "site1")
	covWantCode(t, "delete ok", rec, http.StatusOK)
}

func TestCovForwardProxyRPCBranches(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.forwardProxyRPC(rec, covReq(http.MethodGet, "/x", nil), "ghost", "nginx.status", nil, "", nil)
	covWantCode(t, "offline", rec, http.StatusBadGateway)

	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.forwardProxyRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "nginx.status", nil, "", nil)
	covWantCode(t, "decode err", rec, http.StatusBadGateway)

	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error", "error": ""}
	})
	rec = covRec()
	s3.forwardProxyRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "nginx.status", nil, "", nil)
	covWantCode(t, "rpc empty error", rec, http.StatusBadGateway)

	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload([]interface{}{})
	})
	rec = covRec()
	s4.forwardProxyRPC(rec, covReq(http.MethodGet, "/x", nil), "a1", "nginx.status", nil, "", nil)
	covWantCode(t, "non-map data", rec, http.StatusOK)
}

// ============ api_recordings.go ============

func covSeedRecording(t *testing.T, s *Server, sessionID string) {
	t.Helper()
	if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{
		SessionID: sessionID, AgentID: "a1", Username: "admin",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCovRecordingsBranches(t *testing.T) {
	s := newRecordingTestServer(t)

	// 列表 405 / 空 [] / 成功列表
	rec := covRec()
	s.handleRecordings(rec, covReq(http.MethodPost, "/recordings", nil))
	covWantCode(t, "list wrong method", rec, http.StatusMethodNotAllowed)

	rec = covRec()
	s.handleRecordings(rec, covReq(http.MethodGet, "/recordings", nil))
	covWantCode(t, "empty list", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("empty list should marshal as []: %s", rec.Body.String())
	}

	// cast 非 GET → 405
	rec = covRec()
	s.handleRecordings(rec, covReq(http.MethodPost, "/recordings/s1/cast", nil))
	covWantCode(t, "cast wrong method", rec, http.StatusMethodNotAllowed)

	// delete 非 DELETE → 405
	rec = covRec()
	s.handleRecordings(rec, covReq(http.MethodGet, "/recordings/s1", nil))
	covWantCode(t, "delete wrong method", rec, http.StatusMethodNotAllowed)

	// 未知路径 → 404
	rec = covRec()
	s.handleRecordings(rec, covReq(http.MethodGet, "/recordings/s1/whatever", nil))
	covWantCode(t, "unknown path", rec, http.StatusNotFound)

	// closed db：list → 500
	s2 := newRecordingTestServer(t)
	covCloseDB(t, s2)
	rec = covRec()
	s2.handleRecordings(rec, covReq(http.MethodGet, "/recordings", nil))
	covWantCode(t, "list db error", rec, http.StatusInternalServerError)

	// cast：无记录 → 404
	s3 := newRecordingTestServer(t)
	rec = covRec()
	s3.handleRecordings(rec, covReq(http.MethodGet, "/recordings/none/cast", nil))
	covWantCode(t, "cast missing record", rec, http.StatusNotFound)

	// cast：有记录无文件 → 404（先建 recordings 目录，模拟真实写入路径）
	if err := os.MkdirAll(s3.recordingsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	covSeedRecording(t, s3, "s1")
	rec = covRec()
	s3.handleRecordings(rec, covReq(http.MethodGet, "/recordings/s1/cast", nil))
	covWantCode(t, "cast missing file", rec, http.StatusNotFound)

	// cast 成功（带用户上下文 → 审计 username）
	if err := os.WriteFile(castPath(s3, "s2"), []byte("cast data"), 0o644); err != nil {
		t.Fatal(err)
	}
	covSeedRecording(t, s3, "s2")
	req := covAuthReq(http.MethodGet, "/recordings/s2/cast", nil, "1", "admin", "admin")
	rec = covCallAuth(s3, s3.handleRecordings, req)
	covWantCode(t, "cast ok", rec, http.StatusOK)
	if rec.Body.String() != "cast data" {
		t.Errorf("cast body = %q", rec.Body.String())
	}

	// delete：无记录 → 404
	rec = covRec()
	s3.handleRecordings(rec, covReq(http.MethodDelete, "/recordings/none", nil))
	covWantCode(t, "delete missing record", rec, http.StatusNotFound)

	// delete：cast 路径是非空目录 → os.Remove 失败 → 500
	if err := os.MkdirAll(castPath(s3, "s3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(castPath(s3, "s3"), "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	covSeedRecording(t, s3, "s3")
	rec = covRec()
	s3.handleRecordings(rec, covReq(http.MethodDelete, "/recordings/s3", nil))
	covWantCode(t, "delete non-empty dir fails", rec, http.StatusInternalServerError)

	// delete 成功 → 204
	if err := os.WriteFile(castPath(s3, "s4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	covSeedRecording(t, s3, "s4")
	rec = covRec()
	s3.handleRecordings(rec, covReq(http.MethodDelete, "/recordings/s4", nil))
	covWantCode(t, "delete ok", rec, http.StatusNoContent)
}

// ============ api_server_backup.go ============

// covMakeBackupDirFile 把 serverBackupDir 占位为普通文件（触发 MkdirAll/ReadDir 失败）
func covMakeBackupDirFile(t *testing.T, s *Server) {
	t.Helper()
	dir := s.serverBackupDir()
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCovServerBackupsBranches(t *testing.T) {
	s := newServerBackupTestServer(t)
	// 合法备份文件名（serverBackupNameRe = ^cockpit-\d{8}-\d{6}\.db$）
	const validName = "cockpit-20260101-010203.db"

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"root wrong method", http.MethodPost, "/server-backups", http.StatusMethodNotAllowed},
		{"run wrong method", http.MethodGet, "/server-backups/run", http.StatusMethodNotAllowed},
		{"download wrong method", http.MethodDelete, "/server-backups/" + validName + "/download", http.StatusMethodNotAllowed},
		{"delete wrong method", http.MethodGet, "/server-backups/" + validName, http.StatusMethodNotAllowed},
		{"download invalid name", http.MethodGet, "/server-backups/../evil/download", http.StatusBadRequest},
		{"unknown invalid name", http.MethodGet, "/server-backups/cockpit-bad.db", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleServerBackups(rec, covReq(c.method, c.path, nil))
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// config：GET / PUT bad json / 越界 / 405 / 成功
	rec := covRec()
	s.handleServerBackups(rec, covReq(http.MethodGet, "/server-backups/config", nil))
	covWantCode(t, "config get", rec, http.StatusOK)

	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodPut, "/server-backups/config", strings.NewReader("not-json")))
	covWantCode(t, "config bad json", rec, http.StatusBadRequest)

	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodPut, "/server-backups/config", strings.NewReader(`{"interval_hours":99999}`)))
	covWantCode(t, "config interval over", rec, http.StatusBadRequest)

	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodPut, "/server-backups/config", strings.NewReader(`{"interval_hours":1,"retention_days":-1}`)))
	covWantCode(t, "config retention under", rec, http.StatusBadRequest)

	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodPatch, "/server-backups/config", nil))
	covWantCode(t, "config wrong method", rec, http.StatusMethodNotAllowed)

	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodPut, "/server-backups/config", strings.NewReader(`{"interval_hours":2,"retention_days":3}`)))
	covWantCode(t, "config put ok", rec, http.StatusOK)

	// run：目录被文件占位 → MkdirAll 失败 → 500
	s2 := newServerBackupTestServer(t)
	covMakeBackupDirFile(t, s2)
	rec = covRec()
	s2.handleServerBackups(rec, covReq(http.MethodPost, "/server-backups/run", nil))
	covWantCode(t, "run mkdir fails", rec, http.StatusInternalServerError)

	// list：目录被文件占位 → ReadDir 失败 → 500
	rec = covRec()
	s2.handleServerBackups(rec, covReq(http.MethodGet, "/server-backups", nil))
	covWantCode(t, "list read dir fails", rec, http.StatusInternalServerError)

	// run 成功（真跑 VACUUM INTO）
	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodPost, "/server-backups/run", nil))
	covWantCode(t, "run ok", rec, http.StatusOK)

	// download：文件不存在 → 404
	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodGet, "/server-backups/cockpit-20990101-000000.db/download", nil))
	covWantCode(t, "download missing", rec, http.StatusNotFound)

	// download：成功（带用户上下文 → 审计）
	name := covFirstServerBackupName(t, s)
	req := covAuthReq(http.MethodGet, "/server-backups/"+name+"/download", nil, "1", "admin", "admin")
	rec = covCallAuth(s, s.handleServerBackups, req)
	covWantCode(t, "download ok", rec, http.StatusOK)

	// delete：不存在 → 404
	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodDelete, "/server-backups/cockpit-20990101-000000.db", nil))
	covWantCode(t, "delete missing", rec, http.StatusNotFound)

	// delete：成功 → 204
	rec = covRec()
	s.handleServerBackups(rec, covReq(http.MethodDelete, "/server-backups/"+name, nil))
	covWantCode(t, "delete ok", rec, http.StatusNoContent)
}

func covFirstServerBackupName(t *testing.T, s *Server) string {
	t.Helper()
	list, err := s.listServerBackups()
	if err != nil || len(list) == 0 {
		t.Fatalf("no server backup produced: %v", err)
	}
	return list[0].Name
}

// ============ api_dns.go ============

func TestCovDNSBranches(t *testing.T) {
	// 405 系列 + 404 子路径（dns 未配置时也先判方法/路径）
	s := newBackupTestServer(t) // s.dns == nil
	w := doDNS(s, http.MethodPost, "/dns/status", "")
	covWantCode(t, "status wrong method", w, http.StatusMethodNotAllowed)

	w = doDNS(s, http.MethodPost, "/dns/zones", "")
	covWantCode(t, "zones wrong method", w, http.StatusMethodNotAllowed)

	w = doDNS(s, http.MethodGet, "/dns/zones/z1/bogus", "")
	covWantCode(t, "bad sub path", w, http.StatusNotFound)

	w = doDNS(s, http.MethodGet, "/dns/zones/z1/records/r1/x", "")
	covWantCode(t, "too deep", w, http.StatusNotFound)

	w = doDNS(s, http.MethodPost, "/dns/zones/z1/records/r1", "")
	covWantCode(t, "record wrong method", w, http.StatusMethodNotAllowed)

	// 未配置：create/update/delete → 503
	w = doDNS(s, http.MethodPost, "/dns/zones/z1/records", `{"type":"A","name":"a.b","content":"1.2.3.4"}`)
	covWantCode(t, "create not configured", w, http.StatusServiceUnavailable)

	w = doDNS(s, http.MethodPut, "/dns/zones/z1/records/r1", `{"type":"A","name":"a.b","content":"1.2.3.4"}`)
	covWantCode(t, "update not configured", w, http.StatusServiceUnavailable)

	w = doDNS(s, http.MethodDelete, "/dns/zones/z1/records/r1", "")
	covWantCode(t, "delete not configured", w, http.StatusServiceUnavailable)

	// 配置了 provider：bad json / 上游错误
	s2 := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 500, `{"success":false,"errors":[{"code":1100,"message":"cloudflare down"}],"result":null}`
	})
	w = doDNS(s2, http.MethodPost, "/dns/zones/z1/records", "not-json")
	covWantCode(t, "create bad json", w, http.StatusBadRequest)

	w = doDNS(s2, http.MethodPut, "/dns/zones/z1/records/r1", "not-json")
	covWantCode(t, "update bad json", w, http.StatusBadRequest)

	w = doDNS(s2, http.MethodGet, "/dns/zones", "")
	covWantCode(t, "zones upstream error", w, http.StatusBadGateway)

	w = doDNS(s2, http.MethodPost, "/dns/zones/z1/records", `{"type":"A","name":"a.b","content":"1.2.3.4","ttl":300}`)
	covWantCode(t, "create upstream error", w, http.StatusBadGateway)

	w = doDNS(s2, http.MethodPut, "/dns/zones/z1/records/r1", `{"type":"A","name":"a.b","content":"1.2.3.4","ttl":300}`)
	covWantCode(t, "update upstream error", w, http.StatusBadGateway)

	w = doDNS(s2, http.MethodDelete, "/dns/zones/z1/records/r1", "")
	covWantCode(t, "delete upstream error", w, http.StatusBadGateway)

	// 入参校验类上游错误（消息含 "ttl must be"）→ 400
	s3 := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 400, `{"success":false,"errors":[{"code":1004,"message":"ttl must be 1-300"}],"result":null}`
	})
	w = doDNS(s3, http.MethodPut, "/dns/zones/z1/records/r1", `{"type":"A","name":"a.b","content":"1.2.3.4","ttl":99999}`)
	covWantCode(t, "update validation error", w, http.StatusBadRequest)
}

func TestCovDNSAuditUsername(t *testing.T) {
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodDelete {
			return 200, `{"success":true,"errors":[],"result":{"id":"r1"}}`
		}
		return 200, `{"success":true,"errors":[],"result":{"id":"r9","type":"A","name":"a.b","content":"1.2.3.4","ttl":300,"proxied":false}}`
	})

	// create 经认证上下文 → auditDNS username 分支
	req := covAuthReq(http.MethodPost, "/dns/zones/z1/records",
		strings.NewReader(`{"type":"A","name":"a.b","content":"1.2.3.4","ttl":300}`), "1", "admin", "admin")
	w := covCallAuth(s, s.handleDNS, req)
	covWantCode(t, "create ok", w, http.StatusOK)

	// delete 成功（audit username 兜底空串）
	w = doDNS(s, http.MethodDelete, "/dns/zones/z1/records/r1", "")
	covWantCode(t, "delete ok", w, http.StatusOK)
}

// ============ api_metrics.go ============

func TestCovRegisterMetricsAPI(t *testing.T) {
	s := covNewServer(t)
	mux := http.NewServeMux()
	s.registerMetricsAPI(mux)

	// 无认证 → 401
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/metrics/snapshots", nil))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	// 带认证（token 由该 server 的 authService 签发）
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		want int
	}{
		{"/api/metrics/snapshots", http.StatusOK},
		{"/api/metrics/snapshot?agent_id=a1", http.StatusNotFound}, // 空库无快照
		{"/api/metrics/history?agent_id=a1", http.StatusOK},
	} {
		req := covReq(http.MethodGet, c.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec = covRec()
		mux.ServeHTTP(rec, req)
		covWantCode(t, c.path, rec, c.want)
	}
}

func TestCovMetricsHandlers(t *testing.T) {
	s := covNewServer(t)

	// 405 系列
	for _, h := range []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"snapshots", s.handleSnapshots},
		{"snapshot", s.handleSnapshot},
		{"history", s.handleMetricsHistory},
	} {
		rec := covRec()
		h.fn(rec, covReq(http.MethodPost, "/x", nil))
		covWantCode(t, h.name+" wrong method", rec, http.StatusMethodNotAllowed)
	}

	// snapshot：缺 agent_id → 400
	rec := covRec()
	s.handleSnapshot(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "snapshot missing agent", rec, http.StatusBadRequest)

	// snapshot：无记录 → 404
	rec = covRec()
	s.handleSnapshot(rec, covReq(http.MethodGet, "/x?agent_id=none", nil))
	covWantCode(t, "snapshot missing", rec, http.StatusNotFound)

	// snapshot：成功
	if err := s.db.UpdateSystemInfoSnapshot(&storage.SystemInfoSnapshot{AgentID: "a1", CPUUsage: 12.5, CPUCores: 4}); err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s.handleSnapshot(rec, covReq(http.MethodGet, "/x?agent_id=a1", nil))
	covWantCode(t, "snapshot ok", rec, http.StatusOK)

	// snapshots：成功
	rec = covRec()
	s.handleSnapshots(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "snapshots ok", rec, http.StatusOK)

	// history：缺 agent_id → 400；成功 + limit 截断 + start/end 解析
	rec = covRec()
	s.handleMetricsHistory(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "history missing agent", rec, http.StatusBadRequest)

	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := s.db.SaveSystemMetric(&storage.SystemMetric{
			AgentID: "a1", Timestamp: now.Add(-time.Duration(i) * time.Minute), CPUUsage: float64(i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	rec = covRec()
	s.handleMetricsHistory(rec, covReq(http.MethodGet, "/x?agent_id=a1&limit=2&start=1&end=2000000000", nil))
	covWantCode(t, "history ok", rec, http.StatusOK)
	var resp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2 (limit)", resp.Count)
	}

	// closed db → 500
	s2 := covNewServer(t)
	covCloseDB(t, s2)
	rec = covRec()
	s2.handleSnapshots(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "snapshots db error", rec, http.StatusInternalServerError)

	rec = covRec()
	s2.handleMetricsHistory(rec, covReq(http.MethodGet, "/x?agent_id=a1", nil))
	covWantCode(t, "history db error", rec, http.StatusInternalServerError)
}
