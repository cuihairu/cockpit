package rpc

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseZeroTierInfo(t *testing.T) {
	version, online, err := parseZeroTierInfo([]byte(`{"address":"aaaa","version":"1.14.2","online":true}`))
	if err != nil {
		t.Fatalf("parseZeroTierInfo() error = %v", err)
	}
	if version != "1.14.2" {
		t.Errorf("version = %q, want 1.14.2", version)
	}
	if !online {
		t.Error("online = false, want true")
	}
}

func TestParseZeroTierInfoInvalid(t *testing.T) {
	if _, _, err := parseZeroTierInfo([]byte("not json")); err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestParseZeroTierNetworks(t *testing.T) {
	out := []byte(`[{"nwid":"8056c2e21c","name":"home","status":"OK","type":"Private","dev":"ztabcd","ips":["10.147.20.5/24"]},
		{"nwid":"deadbeef01","name":"","status":"ACCESS_DENIED","type":"Public","dev":"","ips":[]}]`)
	nets := parseZeroTierNetworks(out)
	if len(nets) != 2 {
		t.Fatalf("len(networks) = %d, want 2", len(nets))
	}
	if nets[0]["id"] != "8056c2e21c" || nets[0]["status"] != "OK" {
		t.Errorf("network[0] = %v", nets[0])
	}
	if nets[1]["status"] != "ACCESS_DENIED" {
		t.Errorf("network[1] status = %v, want ACCESS_DENIED preserved", nets[1]["status"])
	}
}

func TestParseZeroTierNetworksInvalid(t *testing.T) {
	if nets := parseZeroTierNetworks([]byte("error output")); nets != nil {
		t.Errorf("expected nil for invalid json, got %v", nets)
	}
}

func TestParseZeroTierPeers(t *testing.T) {
	out := []byte(`[
		{"address":"7f3d0a9b12","version":"1.14.2","latency":23,"role":"LEAF",
		 "paths":[{"priority":0,"address":"1.2.3.4/9993","active":false,"preferred":false},
		          {"priority":1,"address":"5.6.7.8/9993","active":true,"preferred":true}]},
		{"address":"ffffb17a53","version":"","latency":-1,"role":"PLANET",
		 "paths":[{"priority":0,"address":"9.9.9.9/443","active":true,"preferred":false}]}]`)
	peers := parseZeroTierPeers(out)
	if len(peers) != 2 {
		t.Fatalf("len(peers) = %d, want 2", len(peers))
	}
	p0 := peers[0]
	if !p0.Online || p0.Endpoint != "5.6.7.8/9993" || p0.LatencyMs != 23 || p0.Role != "LEAF" {
		t.Errorf("peer[0] = %+v", p0)
	}
	p1 := peers[1]
	if !p1.Online {
		t.Error("peer[1] should be online via active path")
	}
	if p1.LatencyMs != 0 {
		t.Errorf("peer[1] latencyMs = %d, want 0 (negative clamped)", p1.LatencyMs)
	}
	if p1.Endpoint != "9.9.9.9/443" {
		t.Errorf("peer[1] endpoint = %q, want fallback to first active", p1.Endpoint)
	}
}

func TestParseZeroTierPeersInvalid(t *testing.T) {
	if peers := parseZeroTierPeers([]byte("nope")); peers != nil {
		t.Errorf("expected nil for invalid json, got %v", peers)
	}
}

func TestParseTailscaleStatus(t *testing.T) {
	out := []byte(`{
		"Version":"1.80.2","BackendState":"Running",
		"Self":{"HostName":"myhost","TailscaleIPs":["100.64.0.1"],"Online":true},
		"Peer":{
			"key1":{"HostName":"nas","DNSName":"nas.tail-net.ts.net.","TailscaleIPs":["100.64.0.2"],
			        "Online":true,"LastHandshake":"2026-09-17T07:00:12.5Z","CurAddr":"1.2.3.4:41641","Relay":""},
			"key2":{"HostName":"vps","DNSName":"","TailscaleIPs":["100.64.0.3"],
			        "Online":false,"LastHandshake":"0001-01-01T00:00:00Z","CurAddr":"","Relay":"tok"}
		}}`)
	version, networks, peers, err := parseTailscaleStatus(out)
	if err != nil {
		t.Fatalf("parseTailscaleStatus() error = %v", err)
	}
	if version != "1.80.2" {
		t.Errorf("version = %q", version)
	}
	if len(networks) != 1 || networks[0]["name"] != "myhost" || networks[0]["status"] != "Running" {
		t.Errorf("networks = %v", networks)
	}
	if len(peers) != 2 {
		t.Fatalf("len(peers) = %d, want 2", len(peers))
	}
	byName := map[string]overlayPeer{}
	for _, p := range peers {
		byName[p.ID] = p
	}
	nas := byName["nas.tail-net.ts.net"]
	if !nas.Online || nas.Endpoint != "1.2.3.4:41641" || nas.LastHandshake == "" {
		t.Errorf("nas peer = %+v", nas)
	}
	if nas.LastHandshake != "2026-09-17T07:00:12Z" {
		t.Errorf("nas lastHandshake = %q, want RFC3339 UTC", nas.LastHandshake)
	}
	vps := byName["vps"]
	if vps.Online || vps.Relay != "tok" || vps.LastHandshake != "" {
		t.Errorf("vps peer = %+v (zero handshake must be omitted)", vps)
	}
}

func TestParseTailscaleStatusInvalid(t *testing.T) {
	if _, _, _, err := parseTailscaleStatus([]byte("bad")); err == nil {
		t.Error("expected error for invalid json")
	}
}

// wg dump fixture：interface 行含私钥、peer 行含预共享密钥——
// 两者都必须从输出中消失（D6 密钥剥离）
const wgDumpFixture = "wg0\tSECRET-PRIVATE-KEY\t51820\toff\n" +
	"wg0\tpubkeyAAA=\tSECRET-PSK-1\t1.2.3.4:51820\t10.0.0.0/24,192.168.1.0/24\t1789000000\t1024\t2048\t25\n" +
	"wg0\tpubkeyBBB=\t(none)\t(none)\t10.0.1.0/24\t0\t0\t0\t0\n" +
	"wg1\tSECRET-KEY-2\t\toff\n"

func TestParseWGDumpStripsSecrets(t *testing.T) {
	result := parseWGDump([]byte(wgDumpFixture), time.Unix(1789000100, 0))
	blob := marshalJSONForTest(result)
	if strings.Contains(blob, "SECRET") {
		t.Fatal("dump output leaked secret fields (D6 violation)")
	}
	if len(result) != 2 {
		t.Fatalf("len(interfaces) = %d, want 2", len(result))
	}
	wg0 := result[0]
	if wg0["name"] != "wg0" || wg0["listenPort"] != "51820" {
		t.Errorf("wg0 = %v", wg0)
	}
	peers := wg0["peers"].([]overlayPeer)
	if len(peers) != 2 {
		t.Fatalf("wg0 peers = %d, want 2", len(peers))
	}
	p0 := peers[0]
	if p0.ID != "pubkeyAAA=" || p0.Endpoint != "1.2.3.4:51820" || !p0.Online {
		t.Errorf("peer0 = %+v", p0)
	}
	if len(p0.VirtualIPs) != 2 {
		t.Errorf("peer0 virtualIps = %v", p0.VirtualIPs)
	}
	if p0.LastHandshake == "" {
		t.Error("peer0 lastHandshake should be set")
	}
	p1 := peers[1]
	if p1.Online || p1.LastHandshake != "" {
		t.Errorf("peer1 (handshake=0) = %+v, want offline & no timestamp", p1)
	}
	// wg1 无 listen port、无 peer
	wg1 := result[1]
	if wg1["name"] != "wg1" || wg1["peerCount"] != 0 {
		t.Errorf("wg1 = %v", wg1)
	}
}

func TestParseWGDumpOnlineWindow(t *testing.T) {
	now := time.Unix(1789000100, 0)
	// 9 字段 peer 行：iface\tpub\tpsk\tendpoint\tallowed\thandshake\trx\ttx\tkeepalive
	fresh := "wg0\tpk\tpsk\t(NONE)\t10.0.0.0/24\t" + strconv.FormatInt(now.Add(-2*time.Minute).Unix(), 10) + "\t0\t0\t0"
	stale := "wg0\tpk\tpsk\t(NONE)\t10.0.0.0/24\t" + strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10) + "\t0\t0\t0"
	result := parseWGDump([]byte(fresh+"\n"+stale), now)
	peers := result[0]["peers"].([]overlayPeer)
	if !peers[0].Online {
		t.Error("peer with 2min-old handshake should be online")
	}
	if peers[1].Online {
		t.Error("peer with 10min-old handshake should be offline")
	}
}

func TestParseWGDumpTruncatesPeers(t *testing.T) {
	var b strings.Builder
	b.WriteString("wg0\tSECRET\t51820\toff\n")
	for i := 0; i < overlayMaxPeers+10; i++ {
		b.WriteString("wg0\tpk")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("=\t(none)\t(none)\t10.0.0.0/24\t0\t0\t0\t0\n")
	}
	result := parseWGDump([]byte(b.String()), time.Now())
	if got := result[0]["peerCount"]; got != overlayMaxPeers+10 {
		t.Errorf("peerCount = %v, want %d (uncounted truncation)", got, overlayMaxPeers+10)
	}
	if peers := result[0]["peers"].([]overlayPeer); len(peers) != overlayMaxPeers {
		t.Errorf("peers len = %d, want %d", len(peers), overlayMaxPeers)
	}
}

// marshalJSONForTest 把解析结果序列化后做密钥泄漏断言
func marshalJSONForTest(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ============ 纯函数补测（覆盖率缺口）============

// degrade 工具标记 error 并携带阶段与错误摘要
func TestOverlayDegrade(t *testing.T) {
	tool := &overlayTool{Tool: "wg", Status: "ok"}
	got := degrade(tool, "execute", errors.New("exit status 1"))
	if got != tool || got.Status != "error" || got.Error != "execute: exit status 1" {
		t.Fatalf("degrade = %+v", got)
	}
}

// isNotFoundErr 只认 exec.Error 包裹的 ErrNotFound（LookPath 失败形态）
func TestOverlayIsNotFoundErr(t *testing.T) {
	if !isNotFoundErr(&exec.Error{Name: "wg", Err: exec.ErrNotFound}) {
		t.Fatal("exec.ErrNotFound not recognized")
	}
	if isNotFoundErr(&exec.Error{Name: "wg", Err: errors.New("permission denied")}) {
		t.Fatal("non-notfound exec error should not match")
	}
	if isNotFoundErr(errors.New("plain error")) {
		t.Fatal("plain error should not match")
	}
	if isNotFoundErr(nil) {
		t.Fatal("nil should not match")
	}
}

func TestOverlayItoa(t *testing.T) {
	for _, c := range []struct {
		in   int
		want string
	}{
		{0, "0"}, {42, "42"}, {-7, "-7"}, {overlayMaxPeers, "200"},
	} {
		if got := itoa(c.in); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFetchFRPAdminTunnelCount frp admin API /api/status 计数（httptest 桩）：
// 按类型分组的行数求和、非 200 与坏 JSON 报错、连接失败报错
func TestFetchFRPAdminTunnelCount(t *testing.T) {
	doc := `{
		"tcp": [{"name":"ssh"},{"name":"web"},{"name":"db"}],
		"udp": [{"name":"dns"}],
		"version": "0.51.0"
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(doc))
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	n, err := fetchFRPAdminTunnelCount(addr)
	if err != nil || n != 4 {
		t.Fatalf("count = %d err = %v, want 4", n, err)
	}

	// 非 200 显式覆盖：自定义 handler 返回 500
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv2.Close()
	if _, err := fetchFRPAdminTunnelCount(strings.TrimPrefix(srv2.URL, "http://")); err == nil {
		t.Error("500 should error")
	}

	// 坏 JSON
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv3.Close()
	if _, err := fetchFRPAdminTunnelCount(strings.TrimPrefix(srv3.URL, "http://")); err == nil {
		t.Error("bad json should error")
	}

	// 连接失败
	if _, err := fetchFRPAdminTunnelCount("127.0.0.1:1"); err == nil {
		t.Error("connection refused should error")
	}
}
