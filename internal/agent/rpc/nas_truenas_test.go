package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============ TrueNAS provider 测试（nas-design.md D8/D3c）============
//
// httptest 起假 TrueNAS REST v2.0：Basic Auth 校验 + 端点分发；
// 容量字段特意混用 number 与字符串数字（版本兼容，D3c）。

// sampleTnPools 五态池：tank ONLINE 但 resilver SCANNING→resync，
// mirror2 DEGRADED、dead FAULTED、gone REMOVED、okpool ONLINE
const sampleTnPools = `[
  {"id":1,"name":"tank","status":"ONLINE","size":12000239693824,"allocated":6000119846912,
   "topology":{"data":[{"type":"MIRROR","disk":"sda","status":"ONLINE"},{"type":"MIRROR","disk":"sdb","status":"ONLINE"}]},
   "scan":{"function":"RESILVER","state":"SCANNING"}},
  {"id":2,"name":"mirror2","status":"DEGRADED","size":1000204886016,"allocated":1000204886,
   "topology":{"data":[{"type":"DISK","disk":"sdc","status":"DEGRADED"}]},"scan":null},
  {"id":3,"name":"dead","status":"FAULTED","size":1000204886016,"allocated":0,
   "topology":{"data":[]},"scan":null},
  {"id":4,"name":"gone","status":"REMOVED","size":1000204886016,"allocated":0,
   "topology":{"data":[]},"scan":null},
  {"id":5,"name":"okpool","status":"ONLINE","size":2000398934016,"allocated":100,
   "topology":{"data":[{"type":"RAIDZ2","disk":"sdd","status":"ONLINE"}]},"scan":null}
]`

// sampleTnDatasets used/available 用字符串数字（TrueNAS 版本差异，D3c）
const sampleTnDatasets = `[
  {"name":"tank","type":"FILESYSTEM","mounted":true,"mountpoint":"/mnt/tank","used":"6000119846912","available":"6000119846912"},
  {"name":"tank/media","type":"FILESYSTEM","mounted":true,"mountpoint":"/mnt/tank/media","used":"1000","available":"2000"},
  {"name":"tank/iso","type":"FILESYSTEM","mounted":false,"mountpoint":"-","used":"0","available":"0"},
  {"name":"jails","type":"VOLUME","mounted":true,"mountpoint":"/mnt/jails","used":"500","available":"500"},
  {"name":"apps","type":"FILESYSTEM","mounted":true,"mountpoint":"/mnt/apps","used":700,"available":300}
]`

const sampleTnSmb = `[{"name":"media","path":"/mnt/tank/media","comment":"家庭影音"},
                     {"name":"backup","path":"/mnt/tank/backup","comment":""}]`

const sampleTnNfs = `[{"paths":["/mnt/tank/media"],"networks":["192.168.1.0/24"],"hosts":["trusted.local"],"comment":"NFS 导出"}]`

// newTruenasTestServer 假 TrueNAS：Basic Auth 通过才按路径应答，
// 否则 401（凭据错误的 target 走降级路径）
func newTruenasTestServer(t *testing.T, username, password string, badAuth bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password || badAuth {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeArr := func(data string) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(data))
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/v2.0/") {
		case "pool":
			writeArr(sampleTnPools)
		case "dataset":
			writeArr(sampleTnDatasets)
		case "sharing/smb":
			writeArr(sampleTnSmb)
		case "sharing/nfs":
			writeArr(sampleTnNfs)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestTruenasSnapshotMergeAndHost(t *testing.T) {
	srv := newTruenasTestServer(t, "cockpit", fakeDsmPassword, false)
	defer srv.Close()

	targets := `[{"name":"tn-home","type":"truenas","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	m := raw.(map[string]interface{})
	if m["source"] != "linux,truenas" {
		t.Errorf("source = %v, want linux,truenas", m["source"])
	}

	// 池：5 个，状态映射（tank ONLINE 但 resilver 中 → resync）
	pools := m["pools"].([]map[string]interface{})
	if len(pools) != 5 {
		t.Fatalf("pools = %d, want 5", len(pools))
	}
	wantStates := map[string]string{
		"tank": "resync", "mirror2": "degraded", "dead": "failed", "gone": "failed", "okpool": "healthy",
	}
	for _, pool := range pools {
		name := pool["name"].(string)
		if pool["state"] != wantStates[name] {
			t.Errorf("pool %s state = %v, want %s", name, pool["state"], wantStates[name])
		}
		if pool["kind"] != "truenas" || pool["host"] != "tn-home" {
			t.Errorf("pool %s kind/host = %v/%v", name, pool["kind"], pool["host"])
		}
	}
	// tank 的成员盘与 resilver detail
	if got := pools[0]["devices"].([]string); len(got) != 2 || got[0] != "sda" {
		t.Errorf("tank devices = %v", got)
	}
	if d, _ := pools[0]["detail"].(string); !strings.Contains(d, "resilver") {
		t.Errorf("tank detail = %q, want resilver", d)
	}

	// 顶层 dataset → 挂载：tank + apps（子数据集 tank/media、未挂载 iso、
	// VOLUME jails 全部过滤）；字符串与数字容量都解析
	mounts := m["mounts"].([]map[string]interface{})
	if len(mounts) != 2 {
		t.Fatalf("mounts = %d (%+v), want 2", len(mounts), mounts)
	}
	byPath := map[string]map[string]interface{}{}
	for _, mo := range mounts {
		byPath[mo["mountPath"].(string)] = mo
	}
	tank := byPath["/mnt/tank"]
	if tank == nil || tank["fsType"] != "zfs" || tank["host"] != "tn-home" {
		t.Fatalf("tank mount = %+v", tank)
	}
	// used+available = 12000239693824 字节 ≈ 12000 GB（字符串数字解析成功）
	if got := tank["totalGB"].(float64); got < 11999 || got > 12001 {
		t.Errorf("tank totalGB = %v, want ~12000", got)
	}
	apps := byPath["/mnt/apps"]
	if apps == nil {
		t.Fatal("apps mount missing")
	}
	// used=700 available=300 为 number 字面量 → total=1000 字节（数字类型解析走通）
	if got := apps["totalGB"].(float64); got <= 0 || got > 0.001 {
		t.Errorf("apps totalGB = %v, want ~1e-6", got)
	}

	// 共享：SMB 2 条 + NFS 1 条（paths 展开）
	shares := m["shares"].([]map[string]interface{})
	if len(shares) != 3 {
		t.Fatalf("shares = %d, want 3", len(shares))
	}
	var nfs map[string]interface{}
	for _, sh := range shares {
		if sh["protocol"] == "nfs" {
			nfs = sh
		}
	}
	if nfs == nil {
		t.Fatal("nfs share missing")
	}
	if nfs["path"] != "/mnt/tank/media" || nfs["name"] != "tank/media" {
		t.Errorf("nfs share = %+v", nfs)
	}
	if h, _ := nfs["hosts"].(string); !strings.Contains(h, "192.168.1.0/24") || !strings.Contains(h, "trusted.local") {
		t.Errorf("nfs hosts = %q", h)
	}

	// 凭据纪律：快照不含密码
	blob, _ := json.Marshal(m)
	if strings.Contains(string(blob), fakeDsmPassword) {
		t.Error("snapshot leaks password")
	}
}

func TestTruenasBadTargetDegrades(t *testing.T) {
	srv := newTruenasTestServer(t, "cockpit", fakeDsmPassword, false)
	defer srv.Close()

	// 密码错的 target 401 降级，好的正常
	targets := `[{"name":"bad-tn","type":"truenas","addr":"` + srv.URL + `","username":"cockpit","password":"wrong","insecureTls":false},
	             {"name":"good-tn","type":"truenas","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	pools := raw.(map[string]interface{})["pools"].([]map[string]interface{})
	if len(pools) != 5 {
		t.Fatalf("pools = %d, want 5 (only good target)", len(pools))
	}
	for _, pool := range pools {
		if pool["host"] != "good-tn" {
			t.Fatalf("pool host = %v, want good-tn", pool["host"])
		}
	}
}

func TestTruenasPoolStateMapping(t *testing.T) {
	cases := []struct {
		status   string
		scanning bool
		want     string
	}{
		{"ONLINE", false, "healthy"},
		{"ONLINE", true, "resync"},
		{"DEGRADED", false, "degraded"},
		{"DEGRADED", true, "resync"},
		{"FAULTED", false, "failed"},
		{"OFFLINE", false, "failed"},
		{"UNAVAIL", false, "failed"},
		{"REMOVED", false, "failed"},
		{"LOCKED", false, "unknown"},
		{"", false, "unknown"},
	}
	for _, c := range cases {
		if got := truenasPoolState(c.status, c.scanning); got != c.want {
			t.Errorf("truenasPoolState(%q,%v) = %q, want %q", c.status, c.scanning, got, c.want)
		}
	}
}

func TestFlexInt(t *testing.T) {
	cases := []struct {
		json string
		want int64
	}{
		{`123`, 123},
		{`"456"`, 456},
		{`0`, 0},
		{`""`, 0},
		{`null`, 0},
	}
	for _, c := range cases {
		var got flexInt
		if err := json.Unmarshal([]byte(c.json), &got); err != nil {
			t.Errorf("unmarshal %s: %v", c.json, err)
			continue
		}
		if int64(got) != c.want {
			t.Errorf("flexInt(%s) = %d, want %d", c.json, got, c.want)
		}
	}
	var got flexInt
	if err := json.Unmarshal([]byte(`"abc"`), &got); err == nil {
		t.Error(`flexInt("abc") should error`)
	}
}
