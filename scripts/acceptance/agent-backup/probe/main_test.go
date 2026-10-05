// Agent 文件备份探针 helpers 单测（覆盖率收口：helpers 全覆盖）。
//
// 场景编排 main()（K0-K11 真机 server/agent/调度 tick 停机重启/webhook 实收）
// 按验收惯例走真机探针 + .acceptance 证据，不在此单测面内（同 jobs/probe、
// server-backup/probe 先例）。本文件覆盖全部可确定性验证的逻辑：证据层 /
// REST 层（httptest 假 cockpit）/ 配置运行任务解析 / 下载与 sha256 / 源树与
// 归档比对 / 恶意归档构造 / 收包 JSONL / 进程观测与调度串生成。
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// ---------- 测试基建 ----------

// resetGlobals 每用例隔离包级状态（flag 变量与注入点还原为默认语义）
func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		evMu.Lock()
		token, fails, passes, evF = "", 0, 0, nil
		evMu.Unlock()
		*apiBase = "http://127.0.0.1:19998"
		*evDir = ""
		*hookLog = ".acceptance/agent-backup/evidence/webhooks.jsonl"
		*pidFile = ".acceptance/agent-backup/instance/server.pid"
		*pollStep = 2 * time.Second
		osExit = os.Exit
	})
	osExit = os.Exit
}

// stubExit 记录 fatal 的退出码而不真退出
func stubExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	osExit = func(c int) { *code = c }
	return code
}

// fakeAPI 假 cockpit：登录出 token，其余路径可注册
func fakeAPI(t *testing.T, h func(mux *http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"tk-abk-1"}`))
	})
	if h != nil {
		h(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	return srv
}

// setToken 登录态回填（newConfig/查询封装读包级 token）
func setToken(t *testing.T) {
	t.Helper()
	tk, err := loginAs("admin", "pw")
	if err != nil {
		t.Fatal(err)
	}
	evMu.Lock()
	token = tk
	evMu.Unlock()
}

// ---------- 证据层 ----------

func TestEvCheckFatalTruncateSave(t *testing.T) {
	resetGlobals(t)
	f, err := os.CreateTemp(t.TempDir(), "ev")
	if err != nil {
		t.Fatal(err)
	}
	evMu.Lock()
	evF = f
	evMu.Unlock()

	ev("line %d", 1)
	check("pass-case", true, "ok")
	check("fail-case", false, "bad")
	if passes != 1 || fails != 1 {
		t.Fatalf("counters: passes=%d fails=%d, want 1/1", passes, fails)
	}
	body, _ := os.ReadFile(f.Name())
	for _, want := range []string{"line 1", "[PASS] pass-case", "[FAIL] fail-case"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("evidence log missing %q:\n%s", want, body)
		}
	}

	code := stubExit(t)
	fatal(2, "boom %d", 42)
	if *code != 2 {
		t.Fatalf("fatal exit code = %d, want 2", *code)
	}

	if got := truncate("abcdef", 10); got != "abcdef" {
		t.Fatalf("truncate short = %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Fatalf("truncate long = %q", got)
	}

	*evDir = t.TempDir()
	saveEV("sample.json", []byte(`{"k":1}`))
	if b, err := os.ReadFile(filepath.Join(*evDir, "sample.json")); err != nil || string(b) != `{"k":1}` {
		t.Fatalf("saveEV: %v %q", err, b)
	}
	*evDir = ""
	saveEV("skip.json", []byte("x")) // 不 panic 即可
}

// ---------- REST 层 ----------

func TestReqAndLogin(t *testing.T) {
	resetGlobals(t)

	var gotAuth, gotMethod, gotCT string
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/backups/configs", func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotMethod = r.Method
			gotCT = r.Header.Get("Content-Type")
			_, _ = w.Write([]byte(`{"configs":[]}`))
		})
	})

	// 无 token：不带 Authorization
	code, raw := req(http.MethodGet, "/api/backups/configs", nil, "")
	if code != 200 || string(raw) != `{"configs":[]}` {
		t.Fatalf("req no-token: %d %s", code, raw)
	}
	if gotAuth != "" {
		t.Fatalf("no-token req sent Authorization: %q", gotAuth)
	}

	// 有 token + body：带 Bearer 与 JSON 头
	code, _ = req(http.MethodPost, "/api/backups/configs",
		map[string]string{"name": "x"}, "tk-x")
	if code != 200 || gotAuth != "Bearer tk-x" || gotMethod != http.MethodPost ||
		gotCT != "application/json" {
		t.Fatalf("req with token: %d auth=%q method=%s ct=%q", code, gotAuth, gotMethod, gotCT)
	}

	// 连接错误 → -1
	code, _ = req(http.MethodGet, "http://127.0.0.1:1/nope", nil, "")
	if code != -1 {
		t.Fatalf("conn error code = %d, want -1", code)
	}

	// body 不可序列化 → -1（json.Marshal 错误分支）
	code, _ = req(http.MethodPost, "/x", make(chan int), "")
	if code != -1 {
		t.Fatalf("marshal error code = %d, want -1", code)
	}

	// URL 合法但对端拒连 → -1（httpCli.Do 错误分支）
	dead := *apiBase
	*apiBase = "http://127.0.0.1:1"
	code, _ = req(http.MethodGet, "/nope", nil, "")
	if code != -1 {
		t.Fatalf("do error code = %d, want -1", code)
	}
	*apiBase = dead

	// loginAs：正常 / 非 200 / 坏 JSON
	tk, err := loginAs("admin", "pw")
	if err != nil || tk != "tk-abk-1" {
		t.Fatalf("loginAs: %q %v", tk, err)
	}
	savedBase := *apiBase
	*apiBase = savedBase + "/404"
	if _, err := loginAs("admin", "pw"); err == nil ||
		!strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("non-200 err = %v", err)
	}
	*apiBase = savedBase
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer bad.Close()
	*apiBase = bad.URL
	if _, err := loginAs("admin", "pw"); err == nil ||
		!strings.Contains(err.Error(), "token") {
		t.Fatalf("bad-json err = %v", err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":""}`))
	}))
	defer srv2.Close()
	*apiBase = srv2.URL
	if _, err := loginAs("admin", "pw"); err == nil ||
		!strings.Contains(err.Error(), "token") {
		t.Fatalf("empty-token err = %v", err)
	}
	_ = srv
}

// ---------- 解析层 ----------

func TestParseLayers(t *testing.T) {
	resetGlobals(t)

	cfg, err := parseConfigView([]byte(
		`{"id":7,"agent_id":"a1","name":"okset","dest_dir":"/d","schedule":"manual","retention":2,"enabled":true,"next_run_at":0}`))
	if err != nil || cfg.ID != 7 || cfg.Retention != 2 || !cfg.Enabled {
		t.Fatalf("parseConfigView: %+v %v", cfg, err)
	}
	if _, err := parseConfigView([]byte("not-json")); err == nil {
		t.Fatal("parseConfigView bad json: want error")
	}

	list, err := parseConfigList([]byte(`{"configs":[{"id":1},{"id":2}]}`))
	if err != nil || len(list) != 2 {
		t.Fatalf("parseConfigList: %v len=%d", err, len(list))
	}
	if _, err := parseConfigList([]byte("not-json")); err == nil {
		t.Fatal("parseConfigList bad json: want error")
	}

	runs, err := parseRuns([]byte(`{"runs":[{"id":3,"status":"success","file":"f.tar.gz","size":9,"error":"","startedAt":10,"finishedAt":12}]}`))
	if err != nil || len(runs) != 1 || runs[0].File != "f.tar.gz" || runs[0].StartedAt != 10 {
		t.Fatalf("parseRuns: %+v %v", runs, err)
	}
	if _, err := parseRuns([]byte("not-json")); err == nil {
		t.Fatal("parseRuns bad json: want error")
	}

	files, err := parseFiles([]byte(`{"files":[{"name":"a.tar.gz","size":5,"mtime":1}]}`))
	if err != nil || len(files) != 1 || files[0].Name != "a.tar.gz" {
		t.Fatalf("parseFiles: %+v %v", files, err)
	}
	if _, err := parseFiles([]byte("not-json")); err == nil {
		t.Fatal("parseFiles bad json: want error")
	}

	task, err := parseTask([]byte(`{"taskId":"b1","status":"success","error":"","log":"[restore] done: 2 entries"}`))
	if err != nil || task.Status != "success" || !strings.Contains(task.Log, "done") {
		t.Fatalf("parseTask: %+v %v", task, err)
	}
	if _, err := parseTask([]byte("not-json")); err == nil {
		t.Fatal("parseTask bad json: want error")
	}
}

// ---------- 配置/运行/恢复封装 ----------

func TestNewConfigAndQueries(t *testing.T) {
	resetGlobals(t)
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/backups/configs", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["retention"] == float64(99) {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"retention must be 0-365"}`))
					return
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":11,"name":"okset","retention":2,"enabled":true,"next_run_at":0}`))
			case http.MethodGet:
				if strings.HasSuffix(r.URL.Path, "/configs") {
					_, _ = w.Write([]byte(`{"configs":[{"id":11,"name":"okset"},{"id":12,"name":"other"}]}`))
				}
			}
		})
		mux.HandleFunc("/api/backups/configs/11/runs", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"runs":[{"id":1,"status":"success","file":"f.tar.gz"}]}`))
		})
		mux.HandleFunc("/api/backups/configs/11/run", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"started"}`))
		})
		mux.HandleFunc("/api/backups/configs/11/files", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"files":[{"name":"f.tar.gz","size":5}]}`))
		})
		mux.HandleFunc("/api/backups/configs/11/restore", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"taskId":"task-1","status":"started"}`))
		})
		mux.HandleFunc("/api/backups/configs/11/tasks/task-1", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"taskId":"task-1","status":"success","log":"[restore] done: 7 entries"}`))
		})
	})
	setToken(t)

	cfg, err := newConfig("okset", []string{"/x"}, "/d", "manual", 2)
	if err != nil || cfg.ID != 11 || cfg.Name != "okset" {
		t.Fatalf("newConfig: %+v %v", cfg, err)
	}
	if _, err := newConfig("bad", []string{"/x"}, "/d", "manual", 99); err == nil ||
		!strings.Contains(err.Error(), "400") {
		t.Fatalf("newConfig 400 err = %v", err)
	}

	cfgByID, err := configByID(11)
	if err != nil || cfgByID.Name != "okset" {
		t.Fatalf("configByID: %+v %v", cfgByID, err)
	}
	if _, err := configByID(999); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("configByID miss err = %v", err)
	}

	runs, err := runsOf(11)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runsOf: %v len=%d", err, len(runs))
	}
	if err := runConfig(11); err != nil {
		t.Fatalf("runConfig: %v", err)
	}
	files, err := filesOf(11)
	if err != nil || len(files) != 1 {
		t.Fatalf("filesOf: %v len=%d", err, len(files))
	}

	taskID, err := startRestore(11, "f.tar.gz", "/r", "f.tar.gz")
	if err != nil || taskID != "task-1" {
		t.Fatalf("startRestore: %q %v", taskID, err)
	}

	run, err := waitTerminalRun(11, time.Now().Add(time.Second))
	if err != nil || run.Status != "success" {
		t.Fatalf("waitTerminalRun: %+v %v", run, err)
	}
	if _, err := waitTerminalRun(11, time.Now().Add(-time.Second)); err == nil {
		t.Fatal("waitTerminalRun deadline: want error")
	}
	tv, err := waitTerminalTask(11, "task-1", time.Now().Add(time.Second))
	if err != nil || tv.Status != "success" {
		t.Fatalf("waitTerminalTask: %+v %v", tv, err)
	}
	if _, err := waitTerminalTask(11, "task-1", time.Now().Add(-time.Second)); err == nil {
		t.Fatal("waitTerminalTask deadline: want error")
	}
	_ = srv
}

// waitTerminal* 的 running→终态序列（先 running 后 success）
func TestWaitTerminalSequences(t *testing.T) {
	resetGlobals(t)
	var calls int32
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/backups/configs/5/runs", func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) == 1 {
				_, _ = w.Write([]byte(`{"runs":[{"id":1,"status":"running"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"runs":[{"id":1,"status":"success","file":"f.tar.gz"}]}`))
		})
		mux.HandleFunc("/api/backups/configs/5/tasks/b1", func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) == 1 {
				_, _ = w.Write([]byte(`{"taskId":"b1","status":"running"}`))
				return
			}
			_, _ = w.Write([]byte(`{"taskId":"b1","status":"success","log":"[restore] done"}`))
		})
	})
	setToken(t)
	*pollStep = time.Millisecond

	run, err := waitTerminalRun(5, time.Now().Add(3*time.Second))
	if err != nil || run.Status != "success" {
		t.Fatalf("waitTerminalRun seq: %+v %v", run, err)
	}
	tv, err := waitTerminalTask(5, "b1", time.Now().Add(3*time.Second))
	if err != nil || tv.Status != "success" {
		t.Fatalf("waitTerminalTask seq: %+v %v", tv, err)
	}
	_ = srv
}

// TestQueryErrorPaths 查询/触发/恢复封装的错误分支（HTTP 非 200、坏 JSON、
// 轮询超时）。失败响应按相位恒定返回——轮询多少次都失败，超时是唯一出口，
// 与 pollStep 无关，避免时序脆弱。
func TestQueryErrorPaths(t *testing.T) {
	resetGlobals(t)
	var cfgCalls, restoreCalls, taskCalls int32
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/backups/configs", func(w http.ResponseWriter, r *http.Request) {
			switch atomic.AddInt32(&cfgCalls, 1) {
			case 1:
				w.WriteHeader(http.StatusInternalServerError)
			case 2:
				_, _ = w.Write([]byte("not-json"))
			default:
				_, _ = w.Write([]byte(`{"configs":[{"id":7,"name":"n"}]}`))
			}
		})
		// runs 恒 502：waitTerminalRun 走 err 分支后只能超时
		mux.HandleFunc("/api/backups/configs/2/runs", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		// runs 恒空列表：waitTerminalRun 走 no-runs 分支后只能超时
		mux.HandleFunc("/api/backups/configs/4/runs", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"runs":[]}`))
		})
		mux.HandleFunc("/api/backups/configs/3/run", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		})
		mux.HandleFunc("/api/backups/configs/3/files", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		mux.HandleFunc("/api/backups/configs/3/restore", func(w http.ResponseWriter, r *http.Request) {
			switch atomic.AddInt32(&restoreCalls, 1) {
			case 1:
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte("busy"))
			case 2:
				_, _ = w.Write([]byte("garbage"))
			case 3:
				_, _ = w.Write([]byte(`{"taskId":""}`))
			default:
				_, _ = w.Write([]byte(`{"taskId":"t1"}`))
			}
		})
		// 任务恒失败序列后转 running：轮询永不到终态，超时是唯一出口
		mux.HandleFunc("/api/backups/configs/3/tasks/t1", func(w http.ResponseWriter, r *http.Request) {
			switch atomic.AddInt32(&taskCalls, 1) {
			case 1:
				w.WriteHeader(http.StatusInternalServerError)
			case 2:
				_, _ = w.Write([]byte("bad"))
			default:
				_, _ = w.Write([]byte(`{"taskId":"t1","status":"running"}`))
			}
		})
	})
	setToken(t)
	*pollStep = time.Millisecond

	if _, err := configByID(1); err == nil || !strings.Contains(err.Error(), "configs HTTP 500") {
		t.Fatalf("configByID 500: %v", err)
	}
	if _, err := configByID(1); err == nil {
		t.Fatalf("configByID bad-json: want error")
	}
	if _, err := configByID(99); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("configByID missing: %v", err)
	}

	// waitTerminalRun：错误分支、空列表分支（各自只能以超时收场）
	if _, err := waitTerminalRun(2, time.Now().Add(30*time.Millisecond)); err == nil ||
		!strings.Contains(err.Error(), "not terminal") {
		t.Fatalf("waitTerminalRun err-branch: %v", err)
	}
	if _, err := waitTerminalRun(4, time.Now().Add(30*time.Millisecond)); err == nil ||
		!strings.Contains(err.Error(), "no runs yet") {
		t.Fatalf("waitTerminalRun empty: %v", err)
	}

	if err := runConfig(3); err == nil || !strings.Contains(err.Error(), "run HTTP 409") {
		t.Fatalf("runConfig: %v", err)
	}
	if _, err := filesOf(3); err == nil || !strings.Contains(err.Error(), "files HTTP 503") {
		t.Fatalf("filesOf: %v", err)
	}

	// startRestore：非 200 / 坏 JSON / 空 taskId / 正常
	if _, err := startRestore(3, "f.tar.gz", "/dst", "f.tar.gz"); err == nil ||
		!strings.Contains(err.Error(), "restore HTTP 409") {
		t.Fatalf("startRestore 409: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := startRestore(3, "f.tar.gz", "/dst", "f.tar.gz"); err == nil ||
			!strings.Contains(err.Error(), "task id parse") {
			t.Fatalf("startRestore parse: %v", err)
		}
	}
	if id, err := startRestore(3, "f.tar.gz", "/dst", "f.tar.gz"); err != nil || id != "t1" {
		t.Fatalf("startRestore ok: %q %v", id, err)
	}

	// waitTerminalTask：HTTP 错误、解析错误、running 三态后超时
	_, err := waitTerminalTask(3, "t1", time.Now().Add(30*time.Millisecond))
	if err == nil || !strings.Contains(err.Error(), "not terminal") {
		t.Fatalf("waitTerminalTask seq: %v", err)
	}
	_ = srv
}

// ---------- 下载 ----------

func TestDownloadBackup(t *testing.T) {
	resetGlobals(t)
	payload := "archive-bytes-0123456789"
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/backups/configs/3/files/download", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("name") == "missing.tar.gz" {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("agent read error"))
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = w.Write([]byte(payload))
		})
	})
	setToken(t)

	var buf bytes.Buffer
	status, clen, sha, err := downloadBackup(3, "f.tar.gz", &buf)
	if err != nil || status != http.StatusOK || clen != int64(len(payload)) || buf.String() != payload {
		t.Fatalf("downloadBackup: %d %d %v buf=%q", status, clen, err, buf.String())
	}
	sum := sha256.Sum256([]byte(payload))
	if sha != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha = %s", sha)
	}

	// 无 writer 分支（只算哈希）
	status, clen, sha2, err := downloadBackup(3, "f.tar.gz", nil)
	if err != nil || status != http.StatusOK || sha2 != sha {
		t.Fatalf("downloadBackup no-writer: %d %v sha=%s", status, err, sha2)
	}

	status, _, _, err = downloadBackup(3, "missing.tar.gz", nil)
	if err == nil || status != http.StatusBadGateway || !strings.Contains(err.Error(), "502") {
		t.Fatalf("downloadBackup 502: %d %v", status, err)
	}
	_ = srv
}

// TestDownloadBackupErrorPaths 下载封装的错误分支：URL 构造失败、对端拒连、
// 响应中途断流（Content-Length 谎报 → io.Copy 错误）
func TestDownloadBackupErrorPaths(t *testing.T) {
	resetGlobals(t)

	*apiBase = "://bad"
	if _, _, _, err := downloadBackup(1, "f.tar.gz", nil); err == nil {
		t.Fatal("bad base: want error")
	}

	*apiBase = "http://127.0.0.1:1"
	status, _, _, err := downloadBackup(1, "f.tar.gz", nil)
	if err == nil || status != -1 {
		t.Fatalf("dead port: %d %v", status, err)
	}

	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999")
		_, _ = w.Write(make([]byte, 10))
	}))
	defer truncated.Close()
	*apiBase = truncated.URL
	if _, _, _, err = downloadBackup(1, "f.tar.gz", nil); err == nil ||
		!strings.Contains(err.Error(), "download copy") {
		t.Fatalf("truncated body: %v", err)
	}
}

// ---------- sha256 / 源树 / 归档 ----------

func TestSha256File(t *testing.T) {
	resetGlobals(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("cockpit"), 0o600); err != nil {
		t.Fatal(err)
	}
	// sha256("cockpit") = 1f4f4954cb64b1723252fd0090cc461eaf0162bbea0bb541402961e68104c5b7
	want := "1f4f4954cb64b1723252fd0090cc461eaf0162bbea0bb541402961e68104c5b7"
	sum, err := sha256File(p)
	if err != nil || sum != want {
		t.Fatalf("sha256File = %q %v, want %s", sum, err, want)
	}
	if _, err := sha256File(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("sha256File missing: want error")
	}
	if _, err := sha256File(dir); err == nil {
		t.Fatal("sha256File dir: want error")
	}
}

func TestBuildTreeAndDiff(t *testing.T) {
	resetGlobals(t)
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("BB"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}

	withPrefix, err := buildTree(src, "src")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"src", "src/a.txt", "src/sub", "src/sub/b.txt", "src/link"} {
		if _, ok := withPrefix[k]; !ok {
			t.Fatalf("buildTree prefix missing %q: %v", k, keys(withPrefix))
		}
	}
	if withPrefix["src"].Kind != "dir" || withPrefix["src/a.txt"].Kind != "reg" ||
		withPrefix["src/link"].Kind != "sym" || withPrefix["src/link"].Link != "a.txt" {
		t.Fatalf("buildTree kinds wrong: %+v", withPrefix)
	}
	if withPrefix["src/sub/b.txt"].Size != 2 {
		t.Fatalf("size: %+v", withPrefix["src/sub/b.txt"])
	}

	plain, err := buildTree(src, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["a.txt"]; !ok {
		t.Fatalf("buildTree plain missing a.txt: %v", keys(plain))
	}
	if _, ok := plain["src"]; ok {
		t.Fatalf("buildTree plain should skip root: %v", keys(plain))
	}

	if _, err := buildTree(filepath.Join(root, "missing"), "x"); err == nil {
		t.Fatal("buildTree missing root: want error")
	}

	// diffEntries：一致 / 缺失 / 多余 / 类型 / 内容 / 链接
	if probs := diffEntries(withPrefix, withPrefix); len(probs) != 0 {
		t.Fatalf("diffEntries same: %v", probs)
	}
	got := map[string]treeEntry{}
	for k, v := range withPrefix {
		if k == "src/a.txt" {
			continue // 缺失
		}
		got[k] = v
	}
	got["src/extra.txt"] = treeEntry{Kind: "reg", Size: 1, SHA: "x"}
	got["src/sub"] = treeEntry{Kind: "reg"}                    // 类型不符
	got["src/link"] = treeEntry{Kind: "sym", Link: "other"}    // 链接不符
	got["src/sub/b.txt"] = treeEntry{Kind: "reg", Size: 9, SHA: "y"} // 内容不符
	probs := diffEntries(withPrefix, got)
	for _, want := range []string{"missing: src/a.txt", "extra: src/extra.txt",
		"kind src/sub", "link src/link", "content src/sub/b.txt"} {
		found := false
		for _, p := range probs {
			if strings.HasPrefix(p, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("diffEntries missing %q in %v", want, probs)
		}
	}
}

func keys(m map[string]treeEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestBuildTreeSpecialFile FIFO 等特殊文件走 entryFor 的 default 分支
// （kind=other，不跟随、不算哈希）
func TestBuildTreeSpecialFile(t *testing.T) {
	resetGlobals(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	tree, err := buildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if tree["pipe"].Kind != "other" {
		t.Fatalf("fifo kind: %+v", tree["pipe"])
	}
	if tree["a.txt"].Kind != "reg" {
		t.Fatalf("reg kind: %+v", tree["a.txt"])
	}
}

// buildArchiveBytes 内存构造 tar.gz（reg/dir(带尾斜杠)/sym）
func buildArchiveBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tw.WriteHeader(&tar.Header{Name: "src/", Typeflag: tar.TypeDir, Mode: 0o755}))
	must(tw.WriteHeader(&tar.Header{Name: "src/a.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}))
	_, err := tw.Write([]byte("A"))
	must(err)
	must(tw.WriteHeader(&tar.Header{Name: "src/link", Typeflag: tar.TypeSymlink, Linkname: "a.txt"}))
	must(tw.Close())
	must(gz.Close())
	return buf.Bytes()
}

func TestExtractArchive(t *testing.T) {
	resetGlobals(t)
	entries, err := extractArchive(buildArchiveBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	if entries["src"].Kind != "dir" {
		t.Fatalf("dir entry (尾斜杠应去): %+v", entries["src"])
	}
	if entries["src/a.txt"].Kind != "reg" || entries["src/a.txt"].Size != 1 {
		t.Fatalf("reg entry: %+v", entries["src/a.txt"])
	}
	sum := sha256.Sum256([]byte("A"))
	if entries["src/a.txt"].SHA != hex.EncodeToString(sum[:]) {
		t.Fatalf("reg sha: %+v", entries["src/a.txt"])
	}
	if entries["src/link"].Kind != "sym" || entries["src/link"].Link != "a.txt" {
		t.Fatalf("sym entry: %+v", entries["src/link"])
	}
	if _, err := extractArchive([]byte("not-gzip")); err == nil {
		t.Fatal("extractArchive bad gzip: want error")
	}
	if _, err := extractArchive([]byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff}); err == nil {
		t.Fatal("extractArchive corrupt tar: want error")
	}
}

// TestExtractArchiveBadEntries 未知类型条目 → other；条目数据截断 →
// io.Copy 阶段报错（头部声明 100 字节只写 5 字节即关外层 gzip，不关 tar）
func TestExtractArchiveBadEntries(t *testing.T) {
	resetGlobals(t)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "dev0", Typeflag: tar.TypeChar, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := extractArchive(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if entries["dev0"].Kind != "other" {
		t.Fatalf("char device entry: %+v", entries["dev0"])
	}

	var short bytes.Buffer
	gz2 := gzip.NewWriter(&short)
	tw2 := tar.NewWriter(gz2)
	if err := tw2.WriteHeader(&tar.Header{Name: "bad.bin", Typeflag: tar.TypeReg, Mode: 0o644, Size: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw2.Write([]byte("short")); err != nil {
		t.Fatal(err)
	}
	// tw2 故意不 Close：条目数据截断，gzip 正常收尾
	if err := gz2.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractArchive(short.Bytes()); err == nil ||
		!strings.Contains(err.Error(), "entry bad.bin") {
		t.Fatalf("truncated entry: %v", err)
	}
}

func TestWriteEvilTarAndEscape(t *testing.T) {
	resetGlobals(t)
	p := filepath.Join(t.TempDir(), "evil.tar.gz")
	if err := writeEvilTar(p); err != nil {
		t.Fatal(err)
	}
	if err := writeEvilTar(filepath.Join(t.TempDir(), "no-such-dir", "e.tar.gz")); err == nil {
		t.Fatal("writeEvilTar bad path: want error")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := extractArchive(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := entries[evilOKEntry]; !ok {
		t.Fatalf("evil ok entry missing: %v", keys(entries))
	}
	for _, e := range evilEscapeEntries {
		ent, ok := entries[e]
		if !ok || ent.Kind != "reg" {
			t.Fatalf("evil entry %q: %+v (all=%v)", e, ent, keys(entries))
		}
	}
	// 越界落点与 unpack 的 Join+Clean 同式：恢复目录外
	got := escapeTarget("/restore/dir", "../zipslip-escape-1")
	if got != "/restore/zipslip-escape-1" {
		t.Fatalf("escapeTarget = %q", got)
	}
	if got := escapeTarget("/r", "sub/../../zipslip-escape-2"); got != "/zipslip-escape-2" {
		t.Fatalf("escapeTarget nested = %q", got)
	}
}

// ---------- 收包 JSONL ----------

func TestReadJSONL(t *testing.T) {
	resetGlobals(t)
	hooks, err := readJSONL(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil || len(hooks) != 0 {
		t.Fatalf("readJSONL missing: %v len=%d", err, len(hooks))
	}

	p := filepath.Join(t.TempDir(), "hooks.jsonl")
	line1 := `{"ts":"1","path":"/hook","secret_ok":true,"body":"{\"event_type\":\"backup.failed\"}"}`
	// 中段留一个空行 → 覆盖 continue 分支（首尾空白会被 TrimSpace 吃掉）
	if err := os.WriteFile(p, []byte(line1+"\n\n"+line1+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hooks, err = readJSONL(p)
	if err != nil || len(hooks) != 2 || !hooks[0].SecretOK || hooks[0].Path != "/hook" {
		t.Fatalf("readJSONL: %v %+v", err, hooks)
	}
	if !strings.Contains(hooks[0].Body, "backup.failed") {
		t.Fatalf("body: %q", hooks[0].Body)
	}

	if err := os.WriteFile(p, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONL(p); err == nil {
		t.Fatal("readJSONL bad line: want error")
	}

	if _, err := readJSONL(t.TempDir()); err == nil {
		t.Fatal("readJSONL dir path: want error")
	}
}

// ---------- 进程观测 / 调度串 ----------

func TestHwmKB(t *testing.T) {
	resetGlobals(t)
	sample := "Name:\tcockpit\nVmPeak:\t 999999 kB\nVmHWM:\t 123456 kB\nVmRSS:\t 100000 kB\n"
	kb, err := hwmKB(sample)
	if err != nil || kb != 123456 {
		t.Fatalf("hwmKB = %d %v", kb, err)
	}
	if _, err := hwmKB("Name:\tx\n"); err == nil {
		t.Fatal("hwmKB missing key: want error")
	}
	if _, err := hwmKB("VmHWM:\tabc kB\n"); err == nil {
		t.Fatal("hwmKB bad value: want error")
	}
}

func TestReadHWM(t *testing.T) {
	resetGlobals(t)
	pidFile := filepath.Join(t.TempDir(), "server.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	kb, err := readHWM(pidFile)
	if err != nil || kb <= 0 {
		t.Fatalf("readHWM self = %d %v", kb, err)
	}
	if _, err := readHWM(filepath.Join(t.TempDir(), "nope.pid")); err == nil {
		t.Fatal("readHWM missing pid file: want error")
	}
	empty := filepath.Join(t.TempDir(), "empty.pid")
	if err := os.WriteFile(empty, []byte("  "), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHWM(empty); err == nil {
		t.Fatal("readHWM empty pid: want error")
	}
	dead := filepath.Join(t.TempDir(), "dead.pid")
	if err := os.WriteFile(dead, []byte("9999999"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHWM(dead); err == nil {
		t.Fatal("readHWM dead pid: want error")
	}
}

func TestDailyAt(t *testing.T) {
	resetGlobals(t)
	base := time.Date(2026, 10, 5, 10, 30, 45, 0, time.Local)
	if got := dailyAt(base, 2); got != "daily@10:32" {
		t.Fatalf("dailyAt = %q", got)
	}
	// 跨小时与跨日翻转
	late := time.Date(2026, 10, 5, 23, 59, 10, 0, time.Local)
	if got := dailyAt(late, 2); got != "daily@00:01" {
		t.Fatalf("dailyAt rollover day = %q", got)
	}
	edge := time.Date(2026, 10, 5, 23, 58, 30, 0, time.Local)
	if got := dailyAt(edge, 2); got != "daily@00:00" {
		t.Fatalf("dailyAt rollover midnight = %q", got)
	}
	// dueOf 与 dailyAt 同一落点
	if !dueOf(base, 2).Equal(base.Truncate(time.Minute).Add(2 * time.Minute)) {
		t.Fatal("dueOf mismatch")
	}
	if dueOf(base, 2).Sub(dueOf(base, 2).Truncate(time.Minute)) != 0 {
		t.Fatal("dueOf not minute-aligned")
	}
}
