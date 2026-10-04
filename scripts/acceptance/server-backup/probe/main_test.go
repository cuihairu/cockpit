// 面板数据库备份探针 helpers 单测（覆盖率收口：helpers 全覆盖）。
//
// 场景编排 main()（B0-B9 真机双实例/定时 tick 等待/sqlite3 抽查）按验收
// 惯例走真机探针 + .acceptance 证据，不在此单测面内（同 guac/probe、
// jobs/probe 先例）。本文件覆盖全部可确定性验证的逻辑：证据层 / REST 层
// （httptest 假 cockpit）/ config 与备份列表解析 / 老文件植入与 sha256 /
// sqlite3 查询（注入桩）/ 审计解析。
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- 测试基建 ----------

// resetGlobals 每用例隔离包级状态（flag 变量与注入点还原为默认语义）
func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		evMu.Lock()
		fails, passes, evFile = 0, 0, nil
		evMu.Unlock()
		*apiA = "http://127.0.0.1:19995"
		*apiB = "http://127.0.0.1:19996"
		*dirA = ".acceptance/server-backup/instance-a/data/server-backups"
		*dirB = ".acceptance/server-backup/instance-b/data/server-backups"
		*evDir = ""
		osExit = os.Exit
		sqlite3Cmd = func(db, sql string) *exec.Cmd {
			return exec.Command("false")
		}
	})
	osExit = os.Exit
	sqlite3Cmd = func(db, sql string) *exec.Cmd {
		return exec.Command("false")
	}
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
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-sb-1"})
	})
	if h != nil {
		h(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---------- 证据层 ----------

func TestEvCheckFatalTruncateSave(t *testing.T) {
	resetGlobals(t)
	f, err := os.CreateTemp(t.TempDir(), "ev")
	if err != nil {
		t.Fatal(err)
	}
	evMu.Lock()
	evFile = f
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
	fatal("boom %d", 42)
	if *code != 2 {
		t.Fatalf("fatal exit code = %d, want 2", *code)
	}

	if got := truncate("abcdef", 10); got != "abcdef" {
		t.Fatalf("truncate short = %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc..." {
		t.Fatalf("truncate long = %q", got)
	}

	// saveEV：有目录落盘、无目录静默跳过
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

	var gotAuth, gotMethod string
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/server-backups", func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotMethod = r.Method
			_, _ = w.Write([]byte(`{"data":[]}`))
		})
	})

	// 无 token：不带 Authorization
	code, raw := req(http.MethodGet, srv.URL+"/api/server-backups", nil, "")
	if code != 200 || string(raw) != `{"data":[]}` {
		t.Fatalf("req no-token: %d %s", code, raw)
	}
	if gotAuth != "" {
		t.Fatalf("no-token req sent Authorization: %q", gotAuth)
	}

	// 有 token + body：带 Bearer 与 JSON 体
	code, _ = req(http.MethodPut, srv.URL+"/api/server-backups",
		map[string]int{"interval_hours": 1}, "tk-x")
	if code != 200 || gotAuth != "Bearer tk-x" || gotMethod != http.MethodPut {
		t.Fatalf("req with token: %d auth=%q method=%s", code, gotAuth, gotMethod)
	}

	// 连接错误 → -1
	code, _ = req(http.MethodGet, "http://127.0.0.1:1/nope", nil, "")
	if code != -1 {
		t.Fatalf("conn error code = %d, want -1", code)
	}

	// loginAs：正常 / 非 200 / 坏 JSON
	tk, err := loginAs(srv.URL, "admin", "pw")
	if err != nil || tk != "tk-sb-1" {
		t.Fatalf("loginAs: %q %v", tk, err)
	}
	if _, err := loginAs(srv.URL+"/404", "admin", "pw"); err == nil ||
		!strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("loginAs non-200 err = %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer bad.Close()
	if _, err := loginAs(bad.URL, "admin", "pw"); err == nil ||
		!strings.Contains(err.Error(), "token") {
		t.Fatalf("loginAs bad-json err = %v", err)
	}
}

// ---------- config / 列表解析 ----------

func TestParseConfigResp(t *testing.T) {
	resetGlobals(t)
	iv, rt, err := parseConfigResp([]byte(
		`{"interval_hours":24,"retention_days":7,"remote_dest":"","rclone_available":false}`))
	if err != nil || iv != 24 || rt != 7 {
		t.Fatalf("parseConfigResp: %d %d %v", iv, rt, err)
	}
	if _, _, err := parseConfigResp([]byte("not-json")); err == nil {
		t.Fatal("parseConfigResp bad json: want error")
	}
}

func TestParseBackupsAndFind(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"data":[
		{"name":"cockpit-20261005-120000.db","size":100,"modTime":"2026-10-05T12:00:00Z"},
		{"name":"cockpit-20260701-000000.db","size":5,"modTime":"2026-07-01T00:00:00Z"}]}`)
	list, err := parseBackups(raw)
	if err != nil || len(list) != 2 {
		t.Fatalf("parseBackups: %v len=%d", err, len(list))
	}
	if !listHas(list, "cockpit-20260701-000000.db") || listHas(list, "nope.db") {
		t.Fatal("listHas misjudge")
	}
	b := findBackup(list, "cockpit-20260701-000000.db")
	if b == nil || b.Size != 5 {
		t.Fatalf("findBackup: %+v", b)
	}
	if names := backupNames(list); len(names) != 2 || names[0] != "cockpit-20261005-120000.db" {
		t.Fatalf("backupNames: %v", names)
	}
	// firstOtherName：识别非植入产物；空列表返回空串
	if got := firstOtherName(list, "cockpit-20260701-000000.db"); got != "cockpit-20261005-120000.db" {
		t.Fatalf("firstOtherName = %q", got)
	}
	if got := firstOtherName(list, "cockpit-20261005-120000.db"); got != "cockpit-20260701-000000.db" {
		t.Fatalf("firstOtherName exclude-first = %q", got)
	}
	if got := firstOtherName(nil, "x"); got != "" {
		t.Fatalf("firstOtherName empty = %q", got)
	}
	if _, err := parseBackups([]byte("not-json")); err == nil {
		t.Fatal("parseBackups bad json: want error")
	}
}

// ---------- 老文件植入 / sha256 ----------

func TestPlantFakeOldAndSHA256(t *testing.T) {
	resetGlobals(t)
	dir := t.TempDir()
	mod := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if err := plantFakeOld(dir, fakeOldName, mod); err != nil {
		t.Fatalf("plantFakeOld: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, fakeOldName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.ModTime().Equal(mod) {
		t.Fatalf("mtime = %v, want %v", info.ModTime(), mod)
	}
	// 嵌套目录自动创建
	nested := filepath.Join(dir, "sub", "server-backups")
	if err := plantFakeOld(nested, fakeOldName, mod); err != nil {
		t.Fatalf("plantFakeOld nested: %v", err)
	}

	// 目录成分里有普通文件 → MkdirAll 失败
	notDir := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(notDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := plantFakeOld(filepath.Join(notDir, "sub"), fakeOldName, mod); err == nil {
		t.Fatal("plantFakeOld through file: want error")
	}

	// 目录存在但只读 → WriteFile 失败（MkdirAll 对已存在目录恒成功）
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	err = plantFakeOld(ro, fakeOldName, mod)
	if chmodErr := os.Chmod(ro, 0o700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil {
		t.Fatal("plantFakeOld read-only dir: want error")
	}

	// fileSHA256：已知内容 + 缺文件报错
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("cockpit"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(p)
	// sha256("cockpit") = 1f4f4954cb64b1723252fd0090cc461eaf0162bbea0bb541402961e68104c5b7
	want := "1f4f4954cb64b1723252fd0090cc461eaf0162bbea0bb541402961e68104c5b7"
	if err != nil || sum != want {
		t.Fatalf("fileSHA256 = %q %v, want %s", sum, err, want)
	}
	if _, err := fileSHA256(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("fileSHA256 missing: want error")
	}
	// 目录可 Open 但读失败（EISDIR）→ io.Copy 错误透传
	if _, err := fileSHA256(dir); err == nil {
		t.Fatal("fileSHA256 on dir: want error")
	}
}

// ---------- sqlite3 查询（注入桩） ----------

func TestSqliteQuery(t *testing.T) {
	resetGlobals(t)
	sqlite3Cmd = func(db, sql string) *exec.Cmd {
		if db != "/tmp/x.db" || sql != "PRAGMA integrity_check;" {
			t.Errorf("sqlite3Cmd args = %q %q", db, sql)
		}
		return exec.Command("echo", "-n", " ok ")
	}
	out, err := sqliteQuery("/tmp/x.db", "PRAGMA integrity_check;")
	if err != nil || out != "ok" {
		t.Fatalf("sqliteQuery = %q %v, want ok", out, err)
	}

	sqlite3Cmd = func(db, sql string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo boom >&2; exit 3")
	}
	if _, err := sqliteQuery("/tmp/x.db", "SELECT 1;"); err == nil ||
		!strings.Contains(err.Error(), "boom") {
		t.Fatalf("sqliteQuery err = %v, want stderr summary", err)
	}
}

// ---------- 审计解析 ----------

func TestParseAndCountAudit(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"data":[
		{"username":"admin","action":"create","resource":"server_backup","resource_id":"cockpit-20261005-120001.db","details":"{}"},
		{"username":"admin","action":"export","resource":"server_backup","resource_id":"cockpit-20261005-120001.db","details":"{}"},
		{"username":"admin","action":"update","resource":"server_backup","resource_id":"config","details":"{}"}],
		"pagination":{"page":1}}`)
	entries, err := parseAuditEntries(raw)
	if err != nil || len(entries) != 3 {
		t.Fatalf("parseAuditEntries: %v len=%d", err, len(entries))
	}
	if got := countAudit(entries, "create", "cockpit-20261005-120001.db"); len(got) != 1 || got[0].Username != "admin" {
		t.Fatalf("countAudit create: %+v", got)
	}
	if got := countAudit(entries, "export", "cockpit-20261005-120001.db"); len(got) != 1 {
		t.Fatalf("countAudit export: %+v", got)
	}
	if got := countAudit(entries, "update", "config"); len(got) != 1 {
		t.Fatalf("countAudit update: %v", got)
	}
	if got := countAudit(entries, "delete", "config"); len(got) != 0 {
		t.Fatalf("countAudit miss: %+v", got)
	}
	if _, err := parseAuditEntries([]byte("not-json")); err == nil {
		t.Fatal("parseAuditEntries bad json: want error")
	}
}
