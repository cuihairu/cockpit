package rpc

// cov_round8_test.go 第八轮覆盖率：NAS 网络客户端（DSM/OMV httptest 全链）、
// nasHTTPGet 失败族、dsmEntry/dsmLogin/omvEntry 快照降级、snapshotFromTargets
// 未知后端跳过、overlay 的 degraded+listpeers 与 frp admin 双向、admin API
// 响应截断、service Save 的查询/写盘失败、ddns fetchOne 校验族、cron 解析
// 的 range/step 非法分支、mdstat 解析与 NAS 工具探测桩、df/testparm/exportfs
// 失败族、file Write 的 mode 白名单与 Search 大文件/目录缺失、backup 分块
// 读取 eof。
// Read 对 regular 文件的 f.Read 失败（180-182）与 crypto 的 NewCipher/
// NewGCM 错误（密钥恒为 sha256 派生）不在目标内。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCovNasHTTPGetFailures nasHTTPGet：URL 非法与连接失败
func TestCovNasHTTPGetFailures(t *testing.T) {
	if _, err := nasHTTPGet(context.Background(), &http.Client{}, "http://[::bad", nil); err == nil {
		t.Fatal("expect request build error")
	}
	if _, err := nasHTTPGet(context.Background(), &http.Client{}, "http://127.0.0.1:1", nil); err == nil {
		t.Fatal("expect dial error")
	}
}

// dsmTestServer 按 mode 定制行为的 DSM webapi 桩（每 mode 独立 server，
// dsmEntry 请求不带 account，无法按账户分流）
func dsmTestServer(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("method") {
		case "login":
			if mode == "nosid" {
				w.Write([]byte(`{"success":true,"data":{"sid":""}}`))
				return
			}
			w.Write([]byte(`{"success":true,"data":{"sid":"s1"}}`))
		case "load_info":
			switch mode {
			case "loadfail":
				http.Error(w, "boom", http.StatusInternalServerError)
			case "loadbad":
				w.Write([]byte(`{"success":true,"data":"no"}`))
			case "nosuccess":
				w.Write([]byte(`{"success":false,"error":{"code":400}}`))
			default:
				w.Write([]byte(`{"success":true,"data":{"pools":[{"id":"p1","status":"Degrade","devices":["sda"],"total_size":2000000000,"used_size":1000000000}],"volumes":[{"volume":"volume_1","status":"Normal","fs_type":"btrfs","total_size":1000000000,"used_size":500000000,"pool_id":"p1"}]}}`))
			}
		case "list_share":
			switch mode {
			case "sharefail":
				http.Error(w, "boom", http.StatusInternalServerError)
			case "sharebad":
				w.Write([]byte(`{"success":true,"data":123}`))
			default:
				w.Write([]byte(`{"success":true,"data":{"shares":[{"name":"media","path":"/volume1/media"}]}}`))
			}
		default: // logout
			w.Write([]byte(`{"success":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dsmTarget 指向指定桩的 DSM target
func dsmTarget(srv *httptest.Server) NasTarget {
	return NasTarget{Name: "dsm", Type: "dsm", Addr: srv.URL, Username: "u", Password: "pw"}
}

// TestCovDsmSessionBranches dsmEntry 的非 200 / success:false / dsmLogin
// 缺 sid 分支
func TestCovDsmSessionBranches(t *testing.T) {
	client := &http.Client{}
	ctx := context.Background()

	// dsmLogin：响应缺 sid
	if _, err := dsmLogin(ctx, dsmTarget(dsmTestServer(t, "nosid")), client); err == nil || !strings.Contains(err.Error(), "missing sid") {
		t.Fatalf("login no sid: %v", err)
	}

	// dsmEntry：非 200
	s := &nasDsmSession{target: dsmTarget(dsmTestServer(t, "loadfail")), client: client, sid: "s1"}
	if _, err := s.dsmEntry(ctx, url.Values{"api": {"SYNO.Storage.CGI.Storage"}, "method": {"load_info"}}); err == nil || !strings.Contains(err.Error(), "http 500") {
		t.Fatalf("entry 500: %v", err)
	}

	// dsmEntry：success:false → dsm error code
	s = &nasDsmSession{target: dsmTarget(dsmTestServer(t, "nosuccess")), client: client, sid: "s1"}
	if _, err := s.dsmEntry(ctx, url.Values{"api": {"SYNO.Storage.CGI.Storage"}, "method": {"load_info"}}); err == nil || !strings.Contains(err.Error(), "dsm error code 400") {
		t.Fatalf("entry not success: %v", err)
	}
}

// TestCovDsmSnapshotBranches dsmSnapshot 全链：load_info 失败/坏数据、
// list_share 失败/坏数据（不致命）、成功路径的池/卷/共享解析
func TestCovDsmSnapshotBranches(t *testing.T) {
	ctx := context.Background()

	if _, _, _, err := dsmSnapshot(ctx, dsmTarget(dsmTestServer(t, "loadfail"))); err == nil || !strings.Contains(err.Error(), "load_info failed") {
		t.Fatalf("load_info fail: %v", err)
	}
	if _, _, _, err := dsmSnapshot(ctx, dsmTarget(dsmTestServer(t, "loadbad"))); err == nil || !strings.Contains(err.Error(), "load_info decode") {
		t.Fatalf("load_info decode: %v", err)
	}

	// list_share 失败不致命：池/卷照常返回、共享为空
	pools, mounts, shares, err := dsmSnapshot(ctx, dsmTarget(dsmTestServer(t, "sharefail")))
	if err != nil || len(pools) != 1 || len(shares) != 0 {
		t.Fatalf("share fail: pools=%d mounts=%d shares=%d err=%v", len(pools), len(mounts), len(shares), err)
	}
	if _, _, _, err = dsmSnapshot(ctx, dsmTarget(dsmTestServer(t, "sharebad"))); err != nil {
		t.Fatalf("share bad: %v", err)
	}

	// 成功路径：Degrade → degraded 映射
	pools, mounts, shares, err = dsmSnapshot(ctx, dsmTarget(dsmTestServer(t, "good")))
	if err != nil || len(pools) != 1 || len(shares) != 1 {
		t.Fatalf("good: pools=%d mounts=%d shares=%d err=%v", len(pools), len(mounts), len(shares), err)
	}
	if pools[0].State != "degraded" {
		t.Fatalf("pool state: %+v", pools[0])
	}
}

// TestCovNasSnapshotUnknownBackend snapshotFromTargets 对未实现后端的
// default continue（无任何 target 成功 → false）
func TestCovNasSnapshotUnknownBackend(t *testing.T) {
	t.Setenv(nasTargetsEnv, "")
	p := NewNasProvider(nil)
	p.targets = []NasTarget{{Name: "x", Type: "bogus"}}
	snap := &NasSnapshot{}
	sources := []string{}
	if p.snapshotFromTargets(context.Background(), snap, &sources) {
		t.Fatal("unknown backend should not count as ok")
	}
}

// omvTestServer 按路径前缀分流的 OMV rpc.php 桩（每个 mode 独立 server）
func omvTestServer(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode {
		case "login-fail":
			w.Write([]byte(`{"response":{},"error":null}`))
		case "fs-500":
			w.Write([]byte(`{"response":{"sessionid":"s"},"error":null}`))
		case "readall":
			// 声明超长 Content-Length 但只写半截 → 客户端读 body 报错
			w.Header().Set("Content-Length", "1000")
			w.Write([]byte(`{"resp`))
		default:
			w.Write([]byte(`{"response":{"sessionid":"s"},"error":null}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCovOMVSnapshotBranches omvSnapshot：登录失败、enumerateFilesystems
// 失败/坏数据、smb 失败与 nfs 坏数据不致命、共享成功解析
func TestCovOMVSnapshotBranches(t *testing.T) {
	mkTarget := func(srv *httptest.Server, name string) NasTarget {
		return NasTarget{Name: name, Type: "omv", Addr: srv.URL, Username: "u", Password: "p"}
	}
	ctx := context.Background()

	// 登录响应缺 sessionid
	if _, _, err := omvSnapshot(ctx, mkTarget(omvTestServer(t, "login-fail"), "omv-lf")); err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Fatalf("login fail: %v", err)
	}
	// 响应体截断 → 登录读失败
	if _, _, err := omvSnapshot(ctx, mkTarget(omvTestServer(t, "readall"), "omv-ra")); err == nil {
		t.Fatal("expect truncated body error")
	}

	// 全功能桩：filesystems/smb/nfs 按 service 分流
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["service"] {
		case "session":
			w.Write([]byte(`{"response":{"sessionid":"s"},"error":null}`))
		case "filesystemmgmt":
			w.Write([]byte(`{"response":[{"devicefile":"/dev/sda1","type":"ext4","mounted":true,"mountpoint":"/srv/media","size":"1.50 GiB","used":"0.50 GiB"},{"devicefile":"/dev/sda2","type":"swap","mounted":false}],"error":null}`))
		case "smb":
			w.Write([]byte(`{"response":{"data":[{"sharedfoldername":"media","comment":"c","enable":true,"hostsallow":"10.0.0.0/24"}]},"error":null}`))
		default: // nfs
			w.Write([]byte(`{"response":{"data":[{"sharedfoldername":"nfs1","client":"*"}]},"error":null}`))
		}
	}))
	t.Cleanup(srv.Close)
	mounts, shares, err := omvSnapshot(ctx, mkTarget(srv, "omv-good"))
	if err != nil {
		t.Fatalf("omv good: %v", err)
	}
	if len(mounts) != 1 || mounts[0].MountPath != "/srv/media" {
		t.Fatalf("mounts: %+v", mounts)
	}
	if len(shares) != 2 || shares[0].Protocol != "smb" || shares[1].Protocol != "nfs" {
		t.Fatalf("shares: %+v", shares)
	}

	// enumerateFilesystems 失败 / 坏数据
	srvFS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["service"] {
		case "session":
			w.Write([]byte(`{"response":{"sessionid":"s"},"error":null}`))
		default:
			if body["service"] == "filesystemmgmt" && body["method"] == "bad" {
				w.Write([]byte(`{"response":"no","error":null}`))
				return
			}
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srvFS.Close)
	if _, _, err := omvSnapshot(ctx, mkTarget(srvFS, "omv-fs")); err == nil || !strings.Contains(err.Error(), "enumerateFilesystems failed") {
		t.Fatalf("fs fail: %v", err)
	}

	// enumerateFilesystems 坏数据 + smb 失败 + nfs 坏数据（共享不致命）
	srvMix := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["service"] {
		case "session":
			w.Write([]byte(`{"response":{"sessionid":"s"},"error":null}`))
		case "filesystemmgmt":
			w.Write([]byte(`{"response":"no","error":null}`))
		case "smb":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			w.Write([]byte(`{"response":123,"error":null}`))
		}
	}))
	t.Cleanup(srvMix.Close)
	if _, _, err := omvSnapshot(ctx, mkTarget(srvMix, "omv-bad")); err == nil || !strings.Contains(err.Error(), "enumerateFilesystems decode") {
		t.Fatalf("fs decode: %v", err)
	}
}

// TestCovOverlayDegradedAndAdminBothSides zerotier：非 online 时 listpeers
// 失败保留 degraded；frp：frpc admin 成功 tunnels + frps admin 失败
// adminError；admin API 响应截断
func TestCovOverlayDegradedAndAdminBothSides(t *testing.T) {
	// info online=false → degraded；listpeers 也失败 → 保留 degraded 返回
	p := NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		if bin == "zerotier-cli" && len(args) > 1 && args[1] == "info" {
			return []byte(`{"version":"1.14.0","online":false}`), nil, nil
		}
		if bin == "zerotier-cli" {
			return nil, nil, errors.New("subcmd boom")
		}
		return nil, nil, nil
	}).cmd())
	zt := p.readZeroTier()
	if zt.Status != "degraded" || len(zt.Peers) != 0 {
		t.Fatalf("degraded keep: %+v", zt)
	}

	// frpc admin 成功（tunnels）+ frps admin 失败（adminError）
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tcp":[{"a":1},{"a":2}]}`))
	}))
	t.Cleanup(okSrv.Close)
	t.Setenv("COCKPIT_FRPC_ADMIN", okSrv.Listener.Addr().String())
	t.Setenv("COCKPIT_FRPS_ADMIN", "127.0.0.1:1")
	p = NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		if (bin == "frpc" || bin == "frps") && len(args) > 0 && args[0] == "-v" {
			return []byte("0.51.3\n"), nil, nil
		}
		return nil, nil, errors.New("pgrep missing")
	}).cmd())
	frp := p.readFRP()
	if frp.Status != "degraded" {
		t.Fatalf("frp degraded: %+v", frp)
	}
	if got := frp.Extra["frpc"].(map[string]interface{})["tunnels"]; got != 2 {
		t.Fatalf("frpc tunnels: %v", got)
	}
	if _, ok := frp.Extra["frps"].(map[string]interface{})["adminError"]; !ok {
		t.Fatalf("frps adminError missing: %+v", frp.Extra["frps"])
	}

	// admin API 响应体截断 → 读失败
	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.Write([]byte(`{"tcp`))
	}))
	t.Cleanup(cut.Close)
	if _, err := fetchFRPAdminTunnelCount(cut.Listener.Addr().String()); err == nil {
		t.Fatal("expect truncated admin body error")
	}
}

// TestCovServiceSaveQueryAndWriteFailures SaveUnitFile：FragmentPath 查询
// 失败、/etc 侧目标父目录缺失（原子写建临时文件失败）
func TestCovServiceSaveQueryAndWriteFailures(t *testing.T) {
	dir := t.TempDir()
	saved := etcSystemdDir
	etcSystemdDir = dir
	t.Cleanup(func() { etcSystemdDir = saved })

	// show 失败 → SaveUnitFile 前置查询失败
	p := NewServiceProvider(svcRunner{
		"show": func() ([]byte, []byte, error) { return nil, nil, errors.New("show boom") },
	}.cmd())
	if _, err := p.SaveUnitFile("a.service", "x"); err == nil {
		t.Fatal("expect show error")
	}

	// fragment 在 etc 下但父目录不存在 → 原子写失败
	p = NewServiceProvider(svcRunner{
		"show":          func() ([]byte, []byte, error) { return []byte(dir + "/missing-sub/x.service\n"), nil, nil },
		"daemon-reload": func() ([]byte, []byte, error) { return nil, nil, nil },
	}.cmd())
	if _, err := p.SaveUnitFile("x.service", "x"); err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("write fail: %v", err)
	}
}

// TestCovDDNSFetchOneFailures fetchOne：请求构造失败 / 非 200 / 非法 IP /
// 地址族不匹配，及正常取回
func TestCovDDNSFetchOneFailures(t *testing.T) {
	p := NewDDNSProvider(&http.Client{})
	ctx := context.Background()

	if ip := p.fetchOne(ctx, "http://[::bad", false); ip != "" {
		t.Fatalf("bad url: %q", ip)
	}
	var mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode {
		case "500":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "notip":
			w.Write([]byte("not-an-ip\n"))
		default:
			w.Write([]byte("203.0.113.5\n"))
		}
	}))
	t.Cleanup(srv.Close)

	mode = "500"
	if ip := p.fetchOne(ctx, srv.URL, false); ip != "" {
		t.Fatalf("500: %q", ip)
	}
	mode = "notip"
	if ip := p.fetchOne(ctx, srv.URL, false); ip != "" {
		t.Fatalf("notip: %q", ip)
	}
	mode = ""
	if ip := p.fetchOne(ctx, srv.URL, false); ip != "203.0.113.5" {
		t.Fatalf("ok: %q", ip)
	}
	if ip := p.fetchOne(ctx, srv.URL, true); ip != "" { // v4 响应 vs 要 v6
		t.Fatalf("family mismatch should be empty: %q", ip)
	}
}

// TestCovParseCronScheduleRanges cron 域解析：range 越界、倒序 range、
// step 非法与合法 step
func TestCovParseCronScheduleRanges(t *testing.T) {
	for _, expr := range []string{"5-99 * * * *", "10-5 * * * *", "*/0 * * * *", "61 * * * *"} {
		if _, err := parseCronSchedule(expr); err == nil {
			t.Fatalf("expect error for %q", expr)
		}
	}
	if _, err := parseCronSchedule("*/15 9-17 * * 1-5"); err != nil {
		t.Fatalf("valid step: %v", err)
	}
}

// stubNasLookPath 桩替换工具探测（全过 / 全不过），返回恢复函数
func stubNasLookPath(t *testing.T, pass bool) {
	t.Helper()
	saved := nasLookPath
	if pass {
		nasLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	} else {
		nasLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	}
	t.Cleanup(func() { nasLookPath = saved })
}

// TestCovNasDetectFalseAndMdstatPools 探测：env 空且无工具无 mdstat →
// false；mdstat 解析：前置杂行、active+(F) 降级、inactive → failed
func TestCovNasDetectFalseAndMdstatPools(t *testing.T) {
	t.Setenv(nasTargetsEnv, "")
	md := filepath.Join(t.TempDir(), "mdstat")
	savedPath := nasMdstatPath
	nasMdstatPath = md
	t.Cleanup(func() { nasMdstatPath = savedPath })

	stubNasLookPath(t, false)
	if DetectNas() {
		t.Fatal("expect DetectNas false with no tools/targets/mdstat")
	}

	// 前置杂行（无冒号 → cur==nil continue）、active+(F) → degraded、
	// inactive → failed
	content := "random text line\n" +
		"md0 : active raid1 sda1[0] sdb1[1](F)\n" +
		"          1024 blocks super 1.2 [2/2] [UU]\n" +
		"md1 : inactive sdc1[0]\n" +
		"          512 blocks\n"
	if err := os.WriteFile(md, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewNasProvider(nil)
	pools := p.poolsFromMdstat()
	if len(pools) != 2 {
		t.Fatalf("pools: %+v", pools)
	}
	if pools[0].Name != "md0" || pools[0].State != "degraded" || pools[0].TotalGB == 0 {
		t.Fatalf("md0: %+v", pools[0])
	}
	if pools[1].State != "failed" {
		t.Fatalf("md1: %+v", pools[1])
	}
}

// TestCovNasDfSmbNfsFailures df 短行跳过、testparm/exportfs 失败返回空
func TestCovNasDfSmbNfsFailures(t *testing.T) {
	stubNasLookPath(t, true)
	run := Commander(func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
		switch name {
		case "df":
			return []byte("shortline\n/dev/sda1 10485760 5242880 5242880 50% /mnt/data\n"), nil, nil
		case "testparm", "exportfs":
			return nil, nil, errors.New("exit 1")
		}
		return nil, nil, fmt.Errorf("unexpected %s", name)
	})
	p := NewNasProvider(run)

	mounts := p.mountsFromDf(context.Background())
	if len(mounts) != 1 || mounts[0].MountPath != "/mnt/data" {
		t.Fatalf("mounts: %+v", mounts)
	}
	if shares := p.sharesFromSmb(context.Background()); shares != nil {
		t.Fatalf("smb fail should be nil: %+v", shares)
	}
	if shares := p.sharesFromNfs(context.Background()); shares != nil {
		t.Fatalf("nfs fail should be nil: %+v", shares)
	}
}

// TestCovFileWriteModeAndSearchEdges Write 的 mode 白名单（非法拒绝 /
// 0600 显式落权）、Search 的大文件跳过与目录缺失
func TestCovFileWriteModeAndSearchEdges(t *testing.T) {
	p := NewFileProvider()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.conf")
	data := "aGVsbG8=" // base64("hello")

	// mode 非法（仅 0600/0644 白名单）
	if _, err := p.Write(map[string]interface{}{"path": path, "data": data, "mode": float64(0o755), "truncate": true}); err == nil || !strings.Contains(err.Error(), "mode must be 0600 or 0644") {
		t.Fatalf("bad mode: %v", err)
	}
	// 0600 显式落权
	if _, err := p.Write(map[string]interface{}{"path": path, "data": data, "mode": float64(0o600), "truncate": true}); err != nil {
		t.Fatalf("write 0600: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("perm: %v %v", info, err)
	}

	// Search：超过单文件上限的内容被跳过
	big := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("findme", 180*1024)), 0o644); err != nil { // >1MB
		t.Fatal(err)
	}
	res, err := p.Search(map[string]interface{}{"dir": dir, "query": "findme"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	for _, match := range m["matches"].([]map[string]interface{}) {
		if match["path"] == big {
			t.Fatalf("oversized file should be skipped: %v", match)
		}
	}

	// Search：目录不存在 → 回调吞掉根级 err（瞬时错误不中断搜索），优雅空结果
	res, err = p.Search(map[string]interface{}{"dir": filepath.Join(dir, "nope"), "query": "x"})
	if err != nil {
		t.Fatalf("missing dir should degrade: %v", err)
	}
	if got := len(res.(map[string]interface{})["matches"].([]map[string]interface{})); got != 0 {
		t.Fatalf("missing dir matches: %d", got)
	}
}

// TestCovBackupReadChunkEof ReadChunk 的 offset 越过文件尾 → eof 标记
func TestCovBackupReadChunkEof(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chunk.tar.gz"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := p.ReadChunk(map[string]interface{}{"dir": dir, "name": "chunk.tar.gz", "offset": float64(100)})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	if m["eof"] != true || m["data"] != "" {
		t.Fatalf("eof chunk: %v", m)
	}
}
