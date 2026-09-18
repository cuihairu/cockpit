package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// ============ DSM provider 测试（nas-design.md D8）============
//
// httptest 起假 DSM：handler 按 api/method 应答 entry.cgi JSON 样例；
// t.Setenv 注入 COCKPIT_NAS_TARGETS（凭据纪律：快照与错误消息不得含密码）。

const fakeDsmPassword = `p@ss&word=1` // 含 URL 特殊字符，验证编码正确性

// sampleLoadInfo load_info 样例：四态池 + 一个卷（btrfs 2TB 用了一半）
const sampleLoadInfo = `{"pools":[
  {"id":"pool1","status":"Normal","devices":["sda","sdb"],"total_size":2000398934016,"used_size":1000204886016},
  {"id":"pool2","status":"Degrade","devices":["sdc"],"total_size":1000204886016,"used_size":0},
  {"id":"pool3","status":"Crashing","devices":["sdd"],"total_size":1000204886016,"used_size":0},
  {"id":"pool4","status":"Resyncing","devices":["sde"],"total_size":1000204886016,"used_size":0}
],"volumes":[
  {"volume":"/volume1","status":"Normal","fs_type":"btrfs","total_size":2000398934016,"used_size":1000204886016,"pool_id":"pool1"}
]}`

const sampleListShare = `{"shares":[
  {"name":"media","path":"/volume1/media","isdir":true},
  {"name":"backup","path":"/volume1/backup","isdir":true}
]}`

// fakeDsm 假 DSM：按密码校验登录，统计登出次数
type fakeDsm struct {
	mu       sync.Mutex
	password string // 期望密码；不匹配 → 登录失败
	logouts  int
}

func (f *fakeDsm) handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	api, method := q.Get("api"), q.Get("method")
	ok := func(data string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":` + data + `}`))
	}
	switch {
	case api == "SYNO.API.Auth" && method == "login":
		f.mu.Lock()
		defer f.mu.Unlock()
		if q.Get("passwd") != f.password || q.Get("account") != "cockpit" || q.Get("format") != "sid" {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":404}}`))
			return
		}
		ok(`{"sid":"test-sid","did":"d"}`)
	case api == "SYNO.API.Auth" && method == "logout":
		f.mu.Lock()
		f.logouts++
		f.mu.Unlock()
		ok(`{}`)
	case api == "SYNO.Storage.CGI.Storage" && method == "load_info":
		if q.Get("_sid") != "test-sid" {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":403}}`))
			return
		}
		ok(sampleLoadInfo)
	case api == "SYNO.FileStation.List" && method == "list_share":
		ok(sampleListShare)
	default:
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":402}}`))
	}
}

// newDsmTestProvider 干净本地源（桩全空）+ 指定 targets 的 provider
func newDsmTestProvider(t *testing.T, targets string) *NasProvider {
	t.Helper()
	t.Setenv(nasTargetsEnv, targets)
	// 本地源清零：LookPath 全失败 + mdstat 不存在 + 命令空产出，
	// 快照数据只来自 target（不受测试机真实环境影响）
	origLook, origMdstat := nasLookPath, nasMdstatPath
	nasLookPath = func(string) (string, error) { return "", execLookPathNotFound }
	nasMdstatPath = "/nonexistent-mdstat"
	t.Cleanup(func() { nasLookPath, nasMdstatPath = origLook, origMdstat })

	p := NewNasProvider(nil)
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		return nil, []byte("not found"), execLookPathNotFound
	}
	return p
}

// execLookPathNotFound 与 exec.ErrNotFound 语义一致的哨兵错误
var execLookPathNotFound = &lookPathError{}

type lookPathError struct{}

func (*lookPathError) Error() string { return "executable file not found in $PATH" }

func TestDsmSnapshotMergeAndHost(t *testing.T) {
	fake := &fakeDsm{password: fakeDsmPassword}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()

	targets := `[{"name":"home-dsm","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	m := raw.(map[string]interface{})
	if m["source"] != "linux,dsm" {
		t.Errorf("source = %v, want linux,dsm", m["source"])
	}
	if !m["available"].(bool) {
		t.Error("available = false, want true")
	}

	// 池：4 态映射 + host 打标 + 字节→GB
	pools := m["pools"].([]map[string]interface{})
	if len(pools) != 4 {
		t.Fatalf("pools = %d, want 4", len(pools))
	}
	wantStates := map[string]string{"pool1": "healthy", "pool2": "degraded", "pool3": "failed", "pool4": "resync"}
	for _, p := range pools {
		name := p["name"].(string)
		if p["state"] != wantStates[name] {
			t.Errorf("pool %s state = %v, want %s", name, p["state"], wantStates[name])
		}
		if p["kind"] != "dsm" || p["host"] != "home-dsm" {
			t.Errorf("pool %s kind/host = %v/%v", name, p["kind"], p["host"])
		}
	}
	if got := pools[0]["totalGB"].(float64); got < 1999 || got > 2001 {
		t.Errorf("pool1 totalGB = %v, want ~2000", got)
	}

	// 卷 → 挂载（挂载路径/fsType/GB/pool_id）
	mounts := m["mounts"].([]map[string]interface{})
	if len(mounts) != 1 {
		t.Fatalf("mounts = %d, want 1", len(mounts))
	}
	mv := mounts[0]
	if mv["mountPath"] != "/volume1" || mv["fsType"] != "btrfs" || mv["device"] != "pool1" || mv["host"] != "home-dsm" {
		t.Errorf("mount = %+v", mv)
	}
	if got := mv["usedGB"].(float64); got < 999 || got > 1001 {
		t.Errorf("volume usedGB = %v, want ~1000", got)
	}

	// 共享 → smb
	shares := m["shares"].([]map[string]interface{})
	if len(shares) != 2 {
		t.Fatalf("shares = %d, want 2", len(shares))
	}
	if shares[0]["protocol"] != "smb" || shares[0]["name"] != "media" || shares[0]["path"] != "/volume1/media" {
		t.Errorf("share = %+v", shares[0])
	}

	// 凭据纪律：快照不含密码
	blob, _ := json.Marshal(m)
	if strings.Contains(string(blob), fakeDsmPassword) {
		t.Error("snapshot leaks password")
	}

	// 登出 best-effort 被调用
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.logouts != 1 {
		t.Errorf("logouts = %d, want 1", fake.logouts)
	}
}

func TestDsmBadTargetDegrades(t *testing.T) {
	fake := &fakeDsm{password: fakeDsmPassword}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()

	// 一台密码错（登录失败降级）+ 一台 type 未实现（忽略）+ 一台正常
	targets := `[{"name":"bad","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"wrong","insecureTls":false},
	             {"name":"future","type":"truenas","addr":"` + srv.URL + `","username":"u","password":"p","insecureTls":false},
	             {"name":"good","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	m := raw.(map[string]interface{})
	pools := m["pools"].([]map[string]interface{})
	if len(pools) != 4 {
		t.Fatalf("pools = %d, want 4 (only good target)", len(pools))
	}
	for _, pool := range pools {
		if pool["host"] != "good" {
			t.Errorf("pool host = %v, want good", pool["host"])
		}
	}
	// 登录失败错误消息不含密码
	for _, pool := range pools {
		if s, _ := pool["detail"].(string); strings.Contains(s, "wrong") {
			t.Error("error detail leaks password")
		}
	}
}

func TestDsmInsecureTLSSelfSigned(t *testing.T) {
	fake := &fakeDsm{password: fakeDsmPassword}
	srv := httptest.NewTLSServer(http.HandlerFunc(fake.handler)) // 自签证书
	defer srv.Close()

	// 开 insecureTls 才能过自签校验
	targets := `[{"name":"tls-dsm","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":true}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if pools := raw.(map[string]interface{})["pools"].([]map[string]interface{}); len(pools) != 4 {
		t.Fatalf("pools = %d, want 4", len(pools))
	}
}

func TestDsmPoolStateMapping(t *testing.T) {
	cases := map[string]string{
		"Normal":    "healthy",
		"Degrade":   "degraded",
		"Degraded":  "degraded",
		"Crash":     "failed",
		"Crashing":  "failed",
		"Resyncing": "resync",
		"Migrating": "resync",
		"Whatever":  "unknown",
		"":          "unknown",
	}
	for in, want := range cases {
		if got := dsmPoolState(in); got != want {
			t.Errorf("dsmPoolState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseNasTargets(t *testing.T) {
	// 空/空白
	if got := parseNasTargets(""); got != nil {
		t.Errorf("empty → %v, want nil", got)
	}
	// 非法 JSON
	if got := parseNasTargets("{not-json"); got != nil {
		t.Errorf("invalid JSON → %v, want nil", got)
	}
	good := NasTarget{Name: "a", Type: "dsm", Addr: "https://1.2.3.4:5001", Username: "u", Password: "p", InsecureTLS: true}
	// 缺字段（缺密码/缺协议）丢弃，未实现 type 保留（由快照阶段忽略）
	raw := `[
		{"name":"a","type":"dsm","addr":"https://1.2.3.4:5001","username":"u","password":"p","insecureTls":true},
		{"name":"b","type":"dsm","addr":"https://1.2.3.4:5001","username":"u"},
		{"name":"c","type":"dsm","addr":"1.2.3.4:5001","username":"u","password":"p"},
		{"name":"d","type":"truenas","addr":"https://5.6.7.8","username":"u","password":"p"}
	]`
	got := parseNasTargets(raw)
	if len(got) != 2 {
		t.Fatalf("targets = %d (%+v), want 2", len(got), got)
	}
	if got[0] != good {
		t.Errorf("target[0] = %+v, want %+v", got[0], good)
	}
	if got[1].Name != "d" {
		t.Errorf("target[1] = %+v, want truenas entry kept", got[1])
	}
	// addr 尾斜杠由 dsmEntry TrimSuffix 处理
	u, _ := url.Parse(good.Addr)
	if u.Scheme != "https" {
		t.Errorf("addr scheme = %s", u.Scheme)
	}
}
