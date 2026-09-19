package rpc

// cov_round7_test.go 第七轮覆盖率：可注入 Commander 的 provider 错误分支
// 与外部 HTTP 客户端分支——overlay 的 zerotier/tailscale/wireguard/frp 降级
// 族、service 的 List/Status/unit 文件读写失败族（etcSystemdDir 注入）、
// traefik 的版本行回退与写盘冲突、drift 的目录 env 归一化与 cron 命令失败、
// omv 的 HTTP 错误族（httptest 控制）。
// json.Marshal 不可失败与 TOCTOU 类（ReadFile 竞态）不在目标内。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ovF 按「bin + 参数」分流的 overlay 假命令，适配为 Commander
type ovF func(bin string, args []string) ([]byte, []byte, error)

func (f ovF) cmd() Commander {
	return func(_ context.Context, bin string, args ...string) ([]byte, []byte, error) {
		return f(bin, args)
	}
}

// TestCovOverlayZerotierBranches zerotier：info 解析失败、listpeers 失败
// 的 degrade、peers 超限截断
func TestCovOverlayZerotierBranches(t *testing.T) {
	// info 坏 JSON → parse info 降级
	p := NewOverlayProvider(ovF(func(bin string, _ []string) ([]byte, []byte, error) {
		if bin == "zerotier-cli" {
			return []byte("not-json"), nil, nil
		}
		return nil, nil, nil
	}).cmd())
	if zt := p.readZeroTier(); zt.Status != "error" || !strings.Contains(zt.Error, "parse info") {
		t.Fatalf("bad info: %+v", zt)
	}

	// info 正常但 listpeers 失败 → ok 态下 degrade（listnetworks 失败被忽略）
	p = NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		if bin == "zerotier-cli" && len(args) > 1 && args[1] != "info" {
			return nil, nil, errors.New("subcmd boom")
		}
		if bin == "zerotier-cli" {
			return []byte(`{"version":"1.14.0","online":true}`), nil, nil
		}
		return nil, nil, nil
	}).cmd())
	zt := p.readZeroTier()
	if zt.Status != "error" || !strings.Contains(zt.Error, "listpeers") {
		t.Fatalf("listpeers boom: %+v", zt)
	}

	// peers 超上限 → 截断并提示（info 成功 + listpeers 返回 205 条）
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < overlayMaxPeers+5; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"address":"%011x","version":"1.4.0","latency":%d}`, i, i)
	}
	sb.WriteString("]")
	p = NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		if bin != "zerotier-cli" {
			return nil, nil, nil
		}
		if len(args) > 1 && args[1] == "listpeers" {
			return []byte(sb.String()), nil, nil
		}
		return []byte(`{"version":"1.14.0","online":true}`), nil, nil
	}).cmd())
	zt = p.readZeroTier()
	if len(zt.Peers) != overlayMaxPeers || !strings.Contains(zt.Error, "truncated") {
		t.Fatalf("peers truncate: n=%d err=%q", len(zt.Peers), zt.Error)
	}
}

// tsJSON 构造 tailscale status --json 输出（peer 数可调）
func tsJSON(peers int) []byte {
	var sb strings.Builder
	sb.WriteString(`{"Version":"1.80.0","BackendState":"Running","Self":{"HostName":"ts-host","TailscaleIPs":["100.64.0.1"],"Online":true},"Peer":{`)
	for i := 0; i < peers; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `"p%03d":{"HostName":"p%03d","TailscaleIPs":["100.64.0.%d"],"Online":true}`, i, i, i%250+1)
	}
	sb.WriteString("}}")
	return []byte(sb.String())
}

// TestCovOverlayTailscaleBranches tailscale：--peers 失败回退、双重失败
// 不可用判定、解析失败降级、peers 超限截断
func TestCovOverlayTailscaleBranches(t *testing.T) {
	// 首次（--peers）失败非 NotFound → 回退基础形式成功
	p := NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		if bin != "tailscale" {
			return nil, nil, nil
		}
		if len(args) > 2 && args[2] == "--peers" {
			return nil, nil, errors.New("unknown flag: --peers")
		}
		return tsJSON(0), nil, nil
	}).cmd())
	if ts := p.readTailscale(); ts.Status != "ok" || ts.Version != "1.80.0" {
		t.Fatalf("fallback: %+v", ts)
	}
	// 回退仍失败 → 不可用判定
	p = NewOverlayProvider(ovF(func(bin string, _ []string) ([]byte, []byte, error) {
		if bin == "tailscale" {
			return nil, nil, errors.New("daemon down")
		}
		return nil, nil, nil
	}).cmd())
	if ts := p.readTailscale(); ts.Status == "ok" {
		t.Fatalf("expect unavailable: %+v", ts)
	}
	// 解析失败 → degrade
	p = NewOverlayProvider(ovF(func(bin string, _ []string) ([]byte, []byte, error) {
		if bin == "tailscale" {
			return []byte("garbage"), nil, nil
		}
		return nil, nil, nil
	}).cmd())
	if ts := p.readTailscale(); ts.Status != "error" || !strings.Contains(ts.Error, "parse status") {
		t.Fatalf("parse fail: %+v", ts)
	}
	// peers 超上限 → 截断
	p = NewOverlayProvider(ovF(func(bin string, _ []string) ([]byte, []byte, error) {
		if bin == "tailscale" {
			return tsJSON(overlayMaxPeers + 3), nil, nil
		}
		return nil, nil, nil
	}).cmd())
	ts := p.readTailscale()
	if len(ts.Peers) != overlayMaxPeers || !strings.Contains(ts.Error, "truncated") {
		t.Fatalf("peers truncate: n=%d err=%q", len(ts.Peers), ts.Error)
	}
}

// TestCovOverlayWgAndFRP wireguard：dump 失败降级；frp：双二进制失败判定
// 与 admin API env 注入（frpc 端失败降级、frps 端正常计数）
func TestCovOverlayWgAndFRP(t *testing.T) {
	// wg 版本成功但 show all dump 失败 → degrade
	p := NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		if bin == "wg" && len(args) > 0 && args[0] == "--version" {
			return []byte("wireguard-tools v1.0.2"), nil, nil
		}
		if bin == "wg" {
			return nil, nil, errors.New("wg dump boom")
		}
		return nil, nil, nil
	}).cmd())
	if wg := p.readWireGuard(); wg.Status != "error" || !strings.Contains(wg.Error, "wg show") {
		t.Fatalf("wg: %+v", wg)
	}

	// frpc 与 frps 都失败 → 非 NotFound 错误按执行失败降级
	p = NewOverlayProvider(ovF(func(string, []string) ([]byte, []byte, error) {
		return nil, nil, errors.New("exec boom")
	}).cmd())
	if frp := p.readFRP(); frp.Status != "error" {
		t.Fatalf("frp both failing: %+v", frp)
	}

	// frpc admin 连接失败 + frps admin 正常：整体 degraded、两端各归各
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"proxies":[{"a":1},{"a":2}],"tcp":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("COCKPIT_FRPC_ADMIN", "127.0.0.1:1") // 死地址 → 连接失败
	t.Setenv("COCKPIT_FRPS_ADMIN", srv.Listener.Addr().String())
	p = NewOverlayProvider(ovF(func(bin string, args []string) ([]byte, []byte, error) {
		switch {
		case bin == "frpc":
			return nil, nil, errors.New("no frpc")
		case bin == "frps" && len(args) > 0 && args[0] == "-v":
			return []byte("0.51.3\n"), nil, nil
		}
		return nil, nil, errors.New("pgrep missing") // processRunning → false
	}).cmd())
	frp := p.readFRP()
	if frp.Status != "degraded" {
		t.Fatalf("frp degraded: %+v", frp)
	}
	if got := frp.Extra["frps"].(map[string]interface{})["proxies"]; got != 2 {
		t.Fatalf("frps proxies: %v", got)
	}
	if _, ok := frp.Extra["frpc"].(map[string]interface{})["adminError"]; !ok {
		t.Fatalf("frpc adminError missing: %+v", frp.Extra["frpc"])
	}
}

// TestCovFetchFRPAdminTunnelCount admin API 客户端：连接失败 / 非 200 /
// 坏 JSON / 成功计数（顶层非数组键跳过）
func TestCovFetchFRPAdminTunnelCount(t *testing.T) {
	if _, err := fetchFRPAdminTunnelCount("127.0.0.1:1"); err == nil {
		t.Fatal("expect dial error")
	}
	var mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode {
		case "status500":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "badjson":
			w.Write([]byte("not-json"))
		default:
			w.Write([]byte(`{"tcp":[{"x":1},{"x":2}],"udp":[{"x":3}],"note":"str"}`))
		}
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()

	mode = "status500"
	if _, err := fetchFRPAdminTunnelCount(addr); err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("500: %v", err)
	}
	mode = "badjson"
	if _, err := fetchFRPAdminTunnelCount(addr); err == nil || !strings.Contains(err.Error(), "invalid admin api json") {
		t.Fatalf("badjson: %v", err)
	}
	mode = "ok"
	n, err := fetchFRPAdminTunnelCount(addr)
	if err != nil || n != 3 {
		t.Fatalf("count: n=%d err=%v", n, err)
	}
}

// TestCovToIfacePeersFields peers 全字段映射的可选键分支（空值省键）
func TestCovToIfacePeersFields(t *testing.T) {
	out := toIfacePeers([]overlayPeer{
		{ID: "a", Online: true, Name: "alpha", VirtualIPs: []string{"10.0.0.1"}, Version: "1.4",
			LatencyMs: 12, Endpoint: "1.2.3.4:51820", Relay: "relay-1", Role: "PLANET", LastHandshake: "ago"},
		{ID: "b"},
	})
	if len(out) != 2 {
		t.Fatalf("n=%d", len(out))
	}
	full := out[0]
	if full["id"] != "a" || full["online"] != true {
		t.Fatalf("base keys: %v", full)
	}
	for _, k := range []string{"name", "virtualIps", "version", "latencyMs", "endpoint", "relay", "role", "lastHandshake"} {
		if _, ok := full[k]; !ok {
			t.Fatalf("missing key %s: %v", k, full)
		}
	}
	if _, ok := out[1]["name"]; ok {
		t.Fatalf("empty peer should omit name: %v", out[1])
	}
}

// svcRunner 按 systemctl 子命令分流的假 runner
type svcRunner map[string]func() ([]byte, []byte, error)

func (f svcRunner) cmd() Commander {
	return func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name != "systemctl" {
			return nil, nil, fmt.Errorf("unexpected command: %s", name)
		}
		fn, ok := f[args[0]]
		if !ok {
			return nil, nil, fmt.Errorf("unexpected subcommand: %s", args[0])
		}
		return fn()
	}
}

// TestCovServiceListStatusErrors List/Status 的 systemctl 失败分支
func TestCovServiceListStatusErrors(t *testing.T) {
	// list-units 失败（Status 透传）
	p := NewServiceProvider(svcRunner{
		"list-units": func() ([]byte, []byte, error) { return nil, nil, errors.New("units boom") },
	}.cmd())
	if _, err := p.List(); err == nil || !strings.Contains(err.Error(), "list-units") {
		t.Fatalf("list-units: %v", err)
	}
	if _, err := p.Status(); err == nil {
		t.Fatal("expect status propagate list error")
	}
	// list-unit-files 失败
	p = NewServiceProvider(svcRunner{
		"list-units":      func() ([]byte, []byte, error) { return []byte(""), nil, nil },
		"list-unit-files": func() ([]byte, []byte, error) { return nil, nil, errors.New("files boom") },
	}.cmd())
	if _, err := p.List(); err == nil || !strings.Contains(err.Error(), "list-unit-files") {
		t.Fatalf("list-unit-files: %v", err)
	}
	// is-system-running 失败且 stdout 为空 → 探测失败
	p = NewServiceProvider(svcRunner{
		"list-units":        func() ([]byte, []byte, error) { return []byte(""), nil, nil },
		"list-unit-files":   func() ([]byte, []byte, error) { return []byte(""), nil, nil },
		"is-system-running": func() ([]byte, []byte, error) { return nil, []byte("degraded"), errors.New("exit 1") },
	}.cmd())
	if _, err := p.Status(); err == nil || !strings.Contains(err.Error(), "is-system-running") {
		t.Fatalf("is-system-running: %v", err)
	}
}

// TestCovServiceUnitFileOps unit 文件读写的失败与成功族（etcSystemdDir
// 指向临时目录，注释明言该变量为测试可注入）
func TestCovServiceUnitFileOps(t *testing.T) {
	dir := t.TempDir()
	saved := etcSystemdDir
	etcSystemdDir = dir
	t.Cleanup(func() { etcSystemdDir = saved })

	// fragmentPath 查询失败
	p := NewServiceProvider(svcRunner{
		"show": func() ([]byte, []byte, error) { return nil, nil, errors.New("show boom") },
	}.cmd())
	if _, err := p.UnitFile("a.service"); err == nil {
		t.Fatal("expect show error")
	}

	// 未安装（FragmentPath 为空）：UnitFile 与 SaveUnitFile 同源
	emptyShow := svcRunner{"show": func() ([]byte, []byte, error) { return []byte("\n"), nil, nil }}
	p = NewServiceProvider(emptyShow.cmd())
	if _, err := p.UnitFile("a.service"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("unit not installed: %v", err)
	}
	if _, err := p.SaveUnitFile("a.service", "x"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("save not installed: %v", err)
	}

	// cat 失败 / 正常读取
	p = NewServiceProvider(svcRunner{
		"show": func() ([]byte, []byte, error) { return []byte(dir + "/a.service\n"), nil, nil },
		"cat":  func() ([]byte, []byte, error) { return nil, nil, errors.New("cat boom") },
	}.cmd())
	if _, err := p.UnitFile("a.service"); err == nil {
		t.Fatal("expect cat error")
	}
	p = NewServiceProvider(svcRunner{
		"show": func() ([]byte, []byte, error) { return []byte(dir + "/a.service\n"), nil, nil },
		"cat":  func() ([]byte, []byte, error) { return []byte("[Unit]\n"), nil, nil },
	}.cmd())
	if v, err := p.UnitFile("a.service"); err != nil || v.(map[string]interface{})["content"] != "[Unit]\n" {
		t.Fatalf("unit file: %v %v", v, err)
	}

	// Save：内容超限（runner 均合法，先撞上限校验）+ /etc 覆盖位直接写入
	p = NewServiceProvider(svcRunner{
		"show":          func() ([]byte, []byte, error) { return []byte(dir + "/a.service\n"), nil, nil },
		"daemon-reload": func() ([]byte, []byte, error) { return nil, nil, nil },
	}.cmd())
	if _, err := p.SaveUnitFile("a.service", strings.Repeat("x", unitFileContentLimit+1)); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("too large: %v", err)
	}
	if v, err := p.SaveUnitFile("a.service", "[Unit]\n"); err != nil || v.(map[string]interface{})["path"] != filepath.Join(dir, "a.service") {
		t.Fatalf("save: %v %v", v, err)
	}

	// Save：包管路径（/etc 之外）→ 复制到 /etc 覆盖位再写入
	pkgDir := t.TempDir()
	pkgFile := filepath.Join(pkgDir, "b.service")
	if err := os.WriteFile(pkgFile, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	p = NewServiceProvider(svcRunner{
		"show":          func() ([]byte, []byte, error) { return []byte(pkgFile + "\n"), nil, nil },
		"daemon-reload": func() ([]byte, []byte, error) { return nil, nil, nil },
	}.cmd())
	if v, err := p.SaveUnitFile("b.service", "new"); err != nil || v.(map[string]interface{})["path"] != filepath.Join(dir, "b.service") {
		t.Fatalf("save pkg: %v %v", v, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "b.service")); err != nil || string(data) != "new" {
		t.Fatalf("override content: %q %v", data, err)
	}

	// Save：覆盖位已被目录占用 → 复制失败
	if err := os.Mkdir(filepath.Join(dir, "c.service"), 0o755); err != nil {
		t.Fatal(err)
	}
	cFile := filepath.Join(pkgDir, "c.service")
	if err := os.WriteFile(cFile, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	p = NewServiceProvider(svcRunner{
		"show":          func() ([]byte, []byte, error) { return []byte(cFile + "\n"), nil, nil },
		"daemon-reload": func() ([]byte, []byte, error) { return nil, nil, nil },
	}.cmd())
	if _, err := p.SaveUnitFile("c.service", "new"); err == nil || !strings.Contains(err.Error(), "copy") {
		t.Fatalf("copy blocked: %v", err)
	}

	// Save：daemon-reload 失败
	p = NewServiceProvider(svcRunner{
		"show":          func() ([]byte, []byte, error) { return []byte(dir + "/d.service\n"), nil, nil },
		"daemon-reload": func() ([]byte, []byte, error) { return nil, nil, errors.New("reload boom") },
	}.cmd())
	if _, err := p.SaveUnitFile("d.service", "x"); err == nil {
		t.Fatal("expect reload error")
	}
}

// TestCovTraefikVersionAndWriteFailures 版本行无 Version: 前缀的回退、
// Sites 分发扫描错误、ApplySite 写盘冲突（只读目录 / 目标位被目录占用）
// 与 DeleteSite 删除失败、loadSites 的正则跳过
func TestCovTraefikVersionAndWriteFailures(t *testing.T) {
	dir := t.TempDir()
	// "traefik version 2.10.7" 无 "Version:" 键 → 整行作为版本号
	p := NewTraefikProvider(dir, func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
		return []byte("traefik version 2.10.7\n"), nil, nil
	})
	res, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if res.(map[string]interface{})["version"] != "traefik version 2.10.7" {
		t.Fatalf("version fallback: %v", res.(map[string]interface{})["version"])
	}

	// Sites 分发在扫描失败（坏 glob 模式）时透传错误
	broken := NewTraefikProvider(filepath.Join(dir, "bad["), okTraefikRunner)
	if _, err := broken.Call("sites", nil); err == nil {
		t.Fatal("expect scan error")
	}

	// ApplySite：目录只读 → 临时文件写失败
	roDir := t.TempDir()
	ro := newTraefikTestProvider(t, roDir, okTraefikRunner)
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })
	site := &ProxySite{Name: "wtest", ServerNames: []string{"w.example.com"},
		Upstream: "127.0.0.1:8080", Scheme: "http"}
	if _, err := ro.ApplySite(site); err == nil {
		t.Fatal("expect write failure on readonly dir")
	}

	// ApplySite：目标位被目录占用 → rename 失败（临时文件被清理）
	dir2 := t.TempDir()
	block := newTraefikTestProvider(t, dir2, okTraefikRunner)
	if err := os.Mkdir(filepath.Join(dir2, "cockpit-site-block.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	blocked := &ProxySite{Name: "block", ServerNames: []string{"b.example.com"},
		Upstream: "127.0.0.1:8080", Scheme: "http"}
	if _, err := block.ApplySite(blocked); err == nil {
		t.Fatal("expect rename failure")
	}

	// DeleteSite：先成功落一个片段，再把目录改只读 → remove 失败
	del := &ProxySite{Name: "del1", ServerNames: []string{"d.example.com"},
		Upstream: "127.0.0.1:8080", Scheme: "http"}
	if _, err := block.ApplySite(del); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	if err := os.Chmod(dir2, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir2, 0o700) })
	if _, err := block.DeleteSite("del1"); err == nil || !strings.Contains(err.Error(), "remove config") {
		t.Fatalf("remove failure: %v", err)
	}

	// loadSites：文件名匹配 glob 但不匹配站点名正则 → 跳过不报错
	dir3 := t.TempDir()
	ok := newTraefikTestProvider(t, dir3, okTraefikRunner)
	for _, name := range []string{"cockpit-site-.yml", "cockpit-site-UPPER.yml"} {
		if err := os.WriteFile(filepath.Join(dir3, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ok.Call("sites", nil); err != nil {
		t.Fatalf("sites skip: %v", err)
	}
}

// TestCovDriftConfigEnvAndCronErr 目录 env 归一化与 cron 命令失败、
// traefik 扫描的正则跳过
func TestCovDriftConfigEnvAndCronErr(t *testing.T) {
	t.Setenv("COCKPIT_NGINX_CONF_DIR", "/tmp/ng7")
	t.Setenv("COCKPIT_TRAEFIK_DIR", "/tmp/tr7")
	p := NewDriftProvider(nil, DriftConfig{})
	if p.confDir != "/tmp/ng7" || p.dynamicDir != "/tmp/tr7" {
		t.Fatalf("env dirs: %q %q", p.confDir, p.dynamicDir)
	}

	// cron 命令失败（错误不含 no crontab for）→ currentContent 报错
	p = NewDriftProvider(nil, DriftConfig{
		CronRun: Commander(func(context.Context, string, ...string) ([]byte, []byte, error) {
			return nil, []byte("permission denied"), errors.New("exit 1")
		}),
	})
	if _, err := p.currentContent("cron", "cockpit"); err == nil {
		t.Fatal("expect crontab error")
	}

	// traefik 扫描：glob 命中但正则不命中的文件被跳过（不产生条目）
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cockpit-site-.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p = NewDriftProvider(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")), DriftConfig{DynamicDir: dir})
	res, err := p.Check()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.(map[string]interface{})["items"].([]driftItem) {
		if it.Kind == "traefik" {
			t.Fatalf("bad-name site should be skipped: %+v", it)
		}
	}
}

// TestCovOMVEntryBranches omvEntry/omvLogin 的 HTTP 错误族：连接失败 /
// 非 200 / 坏 JSON / error 字段（长 message 截断）/ session 缺失与成功
func TestCovOMVEntryBranches(t *testing.T) {
	// 连接失败
	if _, err := omvEntry(context.Background(), &http.Client{}, "http://127.0.0.1:1", "", "svc", "m", nil); err == nil {
		t.Fatal("expect dial error")
	}

	var mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode {
		case "status500":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "badjson":
			w.Write([]byte("not-json"))
		case "omvErr":
			w.Write([]byte(`{"response":null,"error":{"code":99,"message":"` + strings.Repeat("x", 150) + `"}}`))
		case "noSession":
			w.Write([]byte(`{"response":{"challengeRequired":true}}`))
		default:
			w.Write([]byte(`{"response":{"sessionid":"sid-1"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	client := &http.Client{}

	mode = "status500"
	if _, err := omvEntry(context.Background(), client, srv.URL, "", "svc", "m", nil); err == nil || !strings.Contains(err.Error(), "http 500") {
		t.Fatalf("500: %v", err)
	}
	mode = "badjson"
	if _, err := omvEntry(context.Background(), client, srv.URL, "", "svc", "m", nil); err == nil {
		t.Fatal("expect json error")
	}
	mode = "omvErr"
	_, err := omvEntry(context.Background(), client, srv.URL, "", "svc", "m", nil)
	if err == nil || !strings.Contains(err.Error(), "omv error 99") || len(err.Error()) > 130 {
		t.Fatalf("omv error truncated: %v", err)
	}

	// omvLogin：response 缺 sessionid（2FA challenge 同型）/ 成功
	mode = "noSession"
	if _, err := omvLogin(context.Background(), NasTarget{Addr: srv.URL}, client); err == nil || !strings.Contains(err.Error(), "sessionid") {
		t.Fatalf("no session: %v", err)
	}
	mode = ""
	sid, err := omvLogin(context.Background(), NasTarget{Addr: srv.URL}, client)
	if err != nil || sid != "sid-1" {
		t.Fatalf("login: %q %v", sid, err)
	}
}
