package rpc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ============ OMV provider 测试（nas-design.md D3d/D8）============
//
// httptest 起假 OMV：POST rpc.php 按 body 的 service/method 分发，
// X-Openmediavault-Sessionid 头认证（login 无 sid，其余必须携带）。

// 样例：已挂载两条（ext4 系统盘 + data 盘）、未挂载一条、swap 一条；
// 容量为 binary_format 字符串形态
const sampleOmvFilesystems = `[
  {"devicefile":"/dev/sda1","type":"ext4","mounted":true,"mountpoint":"/",
   "size":"78.13 GiB","used":"8.50 GiB"},
  {"devicefile":"/dev/sdb1","type":"ext4","mounted":true,"mountpoint":"/srv/dev-disk-by-uuid-data",
   "size":"1.50 GiB","used":"500.00 MiB"},
  {"devicefile":"/dev/sdc1","type":"ext4","mounted":false,"mountpoint":"",
   "size":"-1","used":"-1"},
  {"devicefile":"/dev/sdd2","type":"swap","mounted":false,"mountpoint":"",
   "size":"4.00 GiB","used":"0 B"}
]`

const sampleOmvSmb = `{"total":2,"data":[
  {"sharedfoldername":"media","comment":"家庭影音","enable":true,"hostsallow":"192.168.1."},
  {"sharedfoldername":"hidden","comment":"","enable":false,"hostsallow":""}
]}`

const sampleOmvNfs = `{"total":1,"data":[
  {"sharedfoldername":"backup","comment":"NFS 导出","client":"192.168.1.0/24","options":"rw"}
]}`

// newOmvTestServer 假 OMV：校验 sessionid 头，密码错返回 HTTP 400 错误包装
func newOmvTestServer(t *testing.T, username, password string, badAuth bool) *httptest.Server {
	t.Helper()
	var logouts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc.php" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			Service string          `json:"service"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeRPC := func(resp string) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":` + resp + `,"error":null}`))
		}
		switch {
		case req.Service == "session" && req.Method == "login":
			var p struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if badAuth || p.Username != username || p.Password != password {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"response":null,"error":{"code":400,"message":"Incorrect username or password."}}`))
				return
			}
			writeRPC(`{"sessionid":"omv-test-sid"}`)
		case req.Service == "session" && req.Method == "logout":
			if r.Header.Get("X-Openmediavault-Sessionid") == "" {
				t.Error("logout missing session header")
			}
			atomic.AddInt64(&logouts, 1)
			writeRPC(`true`)
		default:
			// 登录后的调用必须带认证头
			if r.Header.Get("X-Openmediavault-Sessionid") != "omv-test-sid" {
				t.Errorf("%s.%s: bad session header %q", req.Service, req.Method, r.Header.Get("X-Openmediavault-Sessionid"))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			switch {
			case req.Service == "filesystemmgmt" && req.Method == "enumerateFilesystems":
				writeRPC(sampleOmvFilesystems)
			case req.Service == "smb" && req.Method == "getShareList":
				writeRPC(sampleOmvSmb)
			case req.Service == "nfs" && req.Method == "getShareList":
				writeRPC(sampleOmvNfs)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	t.Cleanup(func() {
		if n := atomic.LoadInt64(&logouts); n != 1 {
			t.Errorf("logouts = %d, want 1", n)
		}
		srv.Close()
	})
	return srv
}

func TestOmvSnapshotMergeAndHost(t *testing.T) {
	srv := newOmvTestServer(t, "cockpit", fakeDsmPassword, false)

	targets := `[{"name":"omv-home","type":"omv","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	m := raw.(map[string]interface{})
	if m["source"] != "linux,omv" {
		t.Errorf("source = %v, want linux,omv", m["source"])
	}

	// Pools 置空（D3d：OMV 无统一存储池概念），available 由 mounts/shares 判定
	if len(m["pools"].([]map[string]interface{})) != 0 {
		t.Errorf("pools = %v, want empty", m["pools"])
	}

	// 挂载：只取 mounted && mountpoint 非空（swap 与未挂载条目过滤）
	mounts := m["mounts"].([]map[string]interface{})
	if len(mounts) != 2 {
		t.Fatalf("mounts = %d (%+v), want 2", len(mounts), mounts)
	}
	byPath := map[string]map[string]interface{}{}
	for _, mo := range mounts {
		byPath[mo["mountPath"].(string)] = mo
	}
	data := byPath["/srv/dev-disk-by-uuid-data"]
	if data == nil || data["fsType"] != "ext4" || data["device"] != "/dev/sdb1" || data["host"] != "omv-home" {
		t.Fatalf("data mount = %+v", data)
	}
	// "1.50 GiB" = 1610612736 B ≈ 1.61 GB（binary 单位换算，十进制口径）
	if got := data["totalGB"].(float64); got < 1.60 || got > 1.62 {
		t.Errorf("data totalGB = %v, want ~1.61", got)
	}
	// "500.00 MiB" ≈ 0.524 GB
	if got := data["usedGB"].(float64); got < 0.52 || got > 0.53 {
		t.Errorf("data usedGB = %v, want ~0.524", got)
	}
	if root := byPath["/"]; root == nil {
		t.Error("root mount missing")
	}

	// 共享：SMB 仅 enable=true + NFS 一条；SMB Hosts=hostsallow、NFS=client
	shares := m["shares"].([]map[string]interface{})
	if len(shares) != 2 {
		t.Fatalf("shares = %d (%+v), want 2", len(shares), shares)
	}
	byName := map[string]map[string]interface{}{}
	for _, sh := range shares {
		byName[sh["name"].(string)] = sh
	}
	if media := byName["media"]; media == nil || media["protocol"] != "smb" ||
		media["hosts"] != "192.168.1." || media["host"] != "omv-home" {
		t.Errorf("smb media share = %+v", media)
	}
	if backup := byName["backup"]; backup == nil || backup["protocol"] != "nfs" ||
		backup["hosts"] != "192.168.1.0/24" {
		t.Errorf("nfs backup share = %+v", backup)
	}

	// 凭据纪律：快照不含密码
	blob, _ := json.Marshal(m)
	if strings.Contains(string(blob), fakeDsmPassword) {
		t.Error("snapshot leaks password")
	}
}

func TestOmvBadTargetDegrades(t *testing.T) {
	srv := newOmvTestServer(t, "cockpit", fakeDsmPassword, false)

	// 密码错的 target 400 降级，好的正常
	targets := `[{"name":"bad-omv","type":"omv","addr":"` + srv.URL + `","username":"cockpit","password":"wrong","insecureTls":false},
	             {"name":"good-omv","type":"omv","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
	p := newDsmTestProvider(t, targets)

	raw, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	m := raw.(map[string]interface{})
	if m["source"] != "linux,omv" {
		t.Errorf("source = %v, want linux,omv", m["source"])
	}
	for _, mo := range m["mounts"].([]map[string]interface{}) {
		if mo["host"] != "good-omv" {
			t.Fatalf("mount host = %v, want good-omv", mo["host"])
		}
	}
}

func TestParseBinarySize(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"1.50 GiB", 1.5 * 1073741824 / 1e9},
		{"500.00 MiB", 500 * 1048576 / 1e9},
		{"123 B", 123 / 1e9},
		{"1.00 TiB", 1099511627776 / 1e9},
		{"78.13 GiB", 78.13 * 1073741824 / 1e9},
		{"0 B", 0},
		{"-1", 0},        // 未挂载条目
		{"-1.00 GiB", 0}, // 负值拒绝
		{"", 0},
		{"abc", 0},
		{"1.50", 0},     // 缺单位
		{"1.50 XYZ", 0}, // 未知单位
	}
	for _, c := range cases {
		got := parseBinarySize(c.in)
		if (c.want == 0 && got != 0) || (c.want > 0 && (got < c.want*0.999 || got > c.want*1.001)) {
			t.Errorf("parseBinarySize(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
