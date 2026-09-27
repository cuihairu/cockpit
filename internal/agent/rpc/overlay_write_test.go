package rpc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// overlay_write_test.go M3 写路径验收（overlay-design.md D23-D27）：
// join/leave argv 直传与应答码、tailscale BackendState 门与 tailnet 一致性、
// daemon 安装态/systemd 单元态矩阵、service 双白名单。全部走注入 Commander，
// 不碰真机 CLI；systemd 探测与 CLI 二进制名经包级 var 打桩（CI 容器无
// systemd、无 zerotier/tailscale，断言必须脱离真机环境）。

// wrCall 一次命令调用的 argv 记录
type wrCall struct{ argv []string }

func (c wrCall) String() string { return strings.Join(c.argv, " ") }

// wrRecorder 可编程假 Commander：按完整 argv 键路由输出，并记录全部调用。
// 未命中路由返回 exec.ErrNotFound 形态（Status 快照在全缺失下正常降级，
// 不影响 join/leave 主流程断言）。
type wrRecorder struct {
	calls  []wrCall
	routes map[string]wrRoute
}

type wrRoute struct {
	out    []byte
	stderr []byte
	err    error
}

func (w *wrRecorder) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	w.calls = append(w.calls, wrCall{argv: append([]string{name}, args...)})
	key := strings.Join(w.calls[len(w.calls)-1].argv, " ")
	if r, ok := w.routes[key]; ok {
		return r.out, r.stderr, r.err
	}
	return nil, nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func (w *wrRecorder) argvOf(i int) string {
	if i >= len(w.calls) {
		return "<no call " + itoa(i) + ">"
	}
	return w.calls[i].String()
}

// hasCall 是否执行过给定 argv（完整串匹配）
func (w *wrRecorder) hasCall(joined string) bool {
	for _, c := range w.calls {
		if c.String() == joined {
			return true
		}
	}
	return false
}

// mustFakeCLI 造一个真实存在且可执行的假二进制（LookPath 探测安装态用，
// 路径含分隔符时 LookPath 直接 stat，不查 PATH）
func mustFakeCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fake-overlay-cli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

const wrZTNet = "8056c2e21c000001"

// ============ join/leave：ZeroTier ============

func TestOverlayJoinLeaveZeroTier(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"zerotier-cli join " + wrZTNet:  {out: []byte("200 join OK")},
		"zerotier-cli leave " + wrZTNet: {out: []byte("200 leave OK")},
	}}
	p := NewOverlayProvider(rec.run)

	data, err := p.Join("zerotier", wrZTNet)
	if err != nil {
		t.Fatalf("Join error = %v", err)
	}
	if got := rec.argvOf(0); got != "zerotier-cli join "+wrZTNet {
		t.Errorf("join argv = %q", got)
	}
	dataMap := data.(map[string]interface{})
	if _, ok := dataMap["status"]; !ok {
		t.Error("join response should carry refreshed status snapshot (D26)")
	}
	if _, ok := dataMap["identity"]; ok {
		t.Error("identity must be absent when identityFn is not injected")
	}

	if _, err = p.Leave("zerotier", wrZTNet); err != nil {
		t.Fatalf("Leave error = %v", err)
	}
	if !rec.hasCall("zerotier-cli leave " + wrZTNet) {
		t.Errorf("leave argv 未执行, calls = %v", rec.calls)
	}
}

func TestOverlayJoinZeroTierAlreadyMemberCode101(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"zerotier-cli leave " + wrZTNet: {out: []byte("101 leave OK\n")},
	}}
	p := NewOverlayProvider(rec.run)
	if _, err := p.Leave("zerotier", wrZTNet); err != nil {
		t.Errorf("101 应答码应视为幂等成功, got %v", err)
	}
}

func TestOverlayJoinZeroTierUnexpectedResponse(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"zerotier-cli join " + wrZTNet: {out: []byte("500 get controller error")},
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Join("zerotier", wrZTNet)
	if err == nil || !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("err = %v, want unexpected response", err)
	}
}

func TestOverlayJoinZeroTierCommandFails(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"zerotier-cli join " + wrZTNet: {stderr: []byte("cannot connect to daemon"), err: exec.ErrNotFound},
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Join("zerotier", wrZTNet)
	if err == nil || !strings.Contains(err.Error(), "cannot connect to daemon") {
		t.Errorf("err = %v, want stderr summary", err)
	}
}

func TestOverlayJoinZeroTierInvalidNetworkID(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)
	for _, bad := range []string{"", "abc", "8056c2e21c00000", "8056c2e21c0000011", "8056C2E21C000001", "8056c2e21c00000g", "1; rm -rf /"} {
		if _, err := p.Join("zerotier", bad); err == nil || !strings.Contains(err.Error(), "16 hex") {
			t.Errorf("Join(zerotier, %q) err = %v, want 16 hex rejection", bad, err)
		}
	}
	if len(rec.calls) != 0 {
		t.Errorf("invalid id must not execute any command, got %v", rec.calls)
	}
}

// ============ join/leave：Tailscale ============

func tsStatusRoute(state, dnsName string) wrRoute {
	return wrRoute{out: []byte(`{"BackendState":"` + state + `","Self":{"DNSName":"` + dnsName + `"}}`)}
}

func TestOverlayJoinTailscaleRequiresLogin(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("NeedsLogin", ""),
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Join("tailscale", "tailnet-name.ts.net")
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("err = %v, want not-logged-in rejection", err)
	}
	if got := rec.argvOf(0); got != "tailscale status --json" {
		t.Errorf("first argv = %q", got)
	}
	if len(rec.calls) != 1 {
		t.Errorf("未登录必须在 up 之前拒绝, calls = %v", rec.calls)
	}
}

func TestOverlayJoinTailscaleOK(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", "nas.tailnet-name.ts.net."),
		"tailscale up":            {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)
	p.SetOverlayIdentityFn(func() map[string]interface{} {
		return map[string]interface{}{"tailscale": map[string]interface{}{"tailnet": "tailnet-name.ts.net"}}
	})
	data, err := p.Join("tailscale", "tailnet-name.ts.net")
	if err != nil {
		t.Fatalf("Join error = %v", err)
	}
	if got := rec.argvOf(1); got != "tailscale up" {
		t.Errorf("second argv = %q, want tailscale up", got)
	}
	dataMap := data.(map[string]interface{})
	if _, ok := dataMap["identity"]; !ok {
		t.Error("identityFn 注入后响应应携带 identity（D26）")
	}
	if _, ok := dataMap["status"]; !ok {
		t.Error("join response should carry refreshed status snapshot")
	}
}

func TestOverlayJoinTailscaleIdentityNilOmitted(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", ""),
		"tailscale up":            {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)
	p.SetOverlayIdentityFn(func() map[string]interface{} { return nil })
	data, err := p.Join("tailscale", "-")
	if err != nil {
		t.Fatalf("Join(-) error = %v", err)
	}
	if _, ok := data.(map[string]interface{})["identity"]; ok {
		t.Error("identityFn 返回 nil 时不应携带 identity 键")
	}
}

func TestOverlayJoinTailscaleNetIDDashSkipsCompare(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", "nas.tailnet-name.ts.net."),
		"tailscale up":            {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)
	// `-` = 不关心归属 tailnet，跳过一致性比较（前端「任意」入口）
	if _, err := p.Join("tailscale", "-"); err != nil {
		t.Fatalf("Join(-) error = %v", err)
	}
	if got := rec.argvOf(1); got != "tailscale up" {
		t.Errorf("second argv = %q, want tailscale up", got)
	}
}

func TestOverlayJoinTailscaleTailnetMismatch(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", "nas.other-tailnet.ts.net."),
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Join("tailscale", "tailnet-name.ts.net")
	if err == nil || !strings.Contains(err.Error(), "tailnet mismatch") {
		t.Errorf("err = %v, want tailnet mismatch", err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("mismatch 必须在 up 之前拒绝, calls = %v", rec.calls)
	}
}

func TestOverlayJoinTailscaleShortDNSNameSkipsCompare(t *testing.T) {
	// 自建 coordination server 的 DNSName 可能只有 2 段——形态不符不比较，
	// 不错杀（宁可不比不可错杀，见 tailnetFromDNSName 注释）
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", "nas.local"),
		"tailscale up":            {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)
	if _, err := p.Join("tailscale", "tailnet-name.ts.net"); err != nil {
		t.Fatalf("Join error = %v", err)
	}
	if got := rec.argvOf(1); got != "tailscale up" {
		t.Errorf("second argv = %q, want tailscale up", got)
	}
}

func TestOverlayJoinTailscaleUpFails(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", "nas.tailnet-name.ts.net."),
		"tailscale up":            {stderr: []byte("changing settings via 'tailscale up' is disabled"), err: exec.ErrNotFound},
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Join("tailscale", "tailnet-name.ts.net")
	if err == nil || !strings.Contains(err.Error(), "tailscale up") {
		t.Errorf("err = %v, want up failure passthrough", err)
	}
}

func TestOverlayJoinTailscaleStatusCommandFails(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": {err: exec.ErrNotFound},
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Join("tailscale", "tailnet-name.ts.net")
	if err == nil || !strings.Contains(err.Error(), "tailscale status") {
		t.Errorf("err = %v, want status failure", err)
	}
}

func TestOverlayLeaveTailscaleRunsDown(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale down": {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)
	if _, err := p.Leave("tailscale", "-"); err != nil {
		t.Fatalf("Leave error = %v", err)
	}
	// down 是第一条命令（其后 Status 快照还会发观测命令，不在此断言内）；
	// 全程不得出现 logout（清凭据不可逆，D24）
	if rec.argvOf(0) != "tailscale down" {
		t.Errorf("first argv = %q, want tailscale down", rec.argvOf(0))
	}
	if rec.hasCall("tailscale logout") {
		t.Error("leave 绝不允许 tailscale logout（D24）")
	}
}

func TestOverlayJoinTailscaleInvalidName(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)
	for _, bad := range []string{"", "my_tailnet", "My.Tailnet", "-x", "tailnet..name", ".ts.net", "x .com", "tailnet name.ts.net", "单标签"} {
		if _, err := p.Join("tailscale", bad); err == nil {
			t.Errorf("Join(tailscale, %q) 应拒绝", bad)
		}
	}
	if len(rec.calls) != 0 {
		t.Errorf("invalid name must not execute any command, got %v", rec.calls)
	}
}

func TestOverlayJoinTailscaleValidNameForms(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"tailscale status --json": tsStatusRoute("Running", ""),
		"tailscale up":            {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)
	for _, good := range []string{"-", "tailnet-name.ts.net", "corp.example.com", "ts.local", "a-b.c-d.e-f"} {
		if _, err := p.Join("tailscale", good); err != nil {
			t.Errorf("Join(tailscale, %q) err = %v, want ok", good, err)
		}
	}
}

// ============ 未知工具与 dispatch ============

func TestOverlayChangeNetworkUnknownTool(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)
	for _, tool := range []string{"wireguard", "frp", "wg", ""} {
		if _, err := p.Join(tool, wrZTNet); err == nil || !strings.Contains(err.Error(), "does not support join/leave") {
			t.Errorf("Join(%q) err = %v, want unsupported rejection", tool, err)
		}
		if _, err := p.Leave(tool, wrZTNet); err == nil {
			t.Errorf("Leave(%q) 应拒绝", tool)
		}
	}
	if len(rec.calls) != 0 {
		t.Errorf("unsupported tool must not execute any command, got %v", rec.calls)
	}
}

func TestOverlayWriteRPCCallDispatch(t *testing.T) {
	rec := &wrRecorder{routes: map[string]wrRoute{
		"zerotier-cli join " + wrZTNet:        {out: []byte("200 join OK")},
		"zerotier-cli leave " + wrZTNet:       {out: []byte("200 leave OK")},
		"zerotier-cli --version":              {out: []byte("1.14.2")},
		"tailscale version":                   {out: []byte("1.88.4")},
		"systemctl cat zerotier-one.service":  {out: []byte("# unit file")},
		"systemctl stop zerotier-one.service": {out: []byte("")},
	}}
	p := NewOverlayProvider(rec.run)

	if _, err := p.Call("join", map[string]interface{}{"tool": "zerotier", "networkId": wrZTNet}); err != nil {
		t.Errorf("Call(join) error = %v", err)
	}
	if _, err := p.Call("leave", map[string]interface{}{"tool": "zerotier", "networkId": wrZTNet}); err != nil {
		t.Errorf("Call(leave) error = %v", err)
	}
	if _, err := p.Call("daemon", nil); err != nil {
		t.Errorf("Call(daemon) error = %v", err)
	}
	if _, err := p.Call("service", map[string]interface{}{"tool": "zerotier", "action": "stop"}); err != nil {
		t.Errorf("Call(service) error = %v", err)
	}
	// 参数缺失 = 空串 = 白名单拒绝，不 panic
	if _, err := p.Call("join", nil); err == nil {
		t.Error("Call(join) 缺参数应被校验拒绝")
	}
}

// ============ daemon 检测（D27） ============

// stubDetectSystemd 临时替换 systemd 探测，测试结束恢复
func stubDetectSystemd(t *testing.T, v bool) {
	t.Helper()
	old := overlayDetectSystemd
	overlayDetectSystemd = func() bool { return v }
	t.Cleanup(func() { overlayDetectSystemd = old })
}

// stubOverlayBins 临时替换两个 CLI 二进制名 var（LookPath 安装态打桩）
func stubOverlayBins(t *testing.T, zt, ts string) {
	t.Helper()
	oldZT, oldTS := overlayZTCliBin, overlayTailscaleBin
	overlayZTCliBin, overlayTailscaleBin = zt, ts
	t.Cleanup(func() { overlayZTCliBin, overlayTailscaleBin = oldZT, oldTS })
}

func TestOverlayDaemonMissingInstalls(t *testing.T) {
	stubDetectSystemd(t, true)
	stubOverlayBins(t, "cockpit-missing-zt-cli", "cockpit-missing-tailscale")
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)

	data, err := p.Daemon()
	if err != nil {
		t.Fatalf("Daemon error = %v", err)
	}
	tools := data.(map[string]interface{})["tools"].([]map[string]interface{})
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	zt := tools[0]
	if zt["tool"] != "zerotier" || zt["installed"] != false {
		t.Errorf("zerotier = %v", zt)
	}
	if guide, _ := zt["missingGuide"].(string); !strings.Contains(guide, "install.zerotier.com") {
		t.Errorf("zerotier missingGuide = %q", guide)
	}
	ts := tools[1]
	if ts["tool"] != "tailscale" || ts["installed"] != false {
		t.Errorf("tailscale = %v", ts)
	}
	if guide, _ := ts["missingGuide"].(string); !strings.Contains(guide, "tailscale.com/install.sh") {
		t.Errorf("tailscale missingGuide = %q", guide)
	}
	if len(rec.calls) != 0 {
		t.Errorf("未安装不应执行任何命令, calls = %v", rec.calls)
	}
}

func TestOverlayDaemonSystemdMatrix(t *testing.T) {
	stubDetectSystemd(t, true)
	fakeCLI := mustFakeCLI(t)
	stubOverlayBins(t, fakeCLI, fakeCLI)
	rec := &wrRecorder{routes: map[string]wrRoute{
		fakeCLI + " --version":                      {out: []byte("1.14.2\n")},
		fakeCLI + " version":                        {out: []byte("1.88.4\n")},
		"systemctl cat zerotier-one.service":        {out: []byte("# unit")},
		"systemctl is-active zerotier-one.service":  {out: []byte("active")},
		"systemctl is-enabled zerotier-one.service": {err: exec.ErrNotFound}, // 非 0 退出 = disabled
		// tailscaled 单元缺失：cat 失败 → unitExists false
	}}
	p := NewOverlayProvider(rec.run)

	data, err := p.Daemon()
	if err != nil {
		t.Fatalf("Daemon error = %v", err)
	}
	tools := data.(map[string]interface{})["tools"].([]map[string]interface{})
	zt := tools[0]
	if zt["installed"] != true || zt["unitExists"] != true || zt["active"] != true || zt["enabled"] != false {
		t.Errorf("zerotier = %v", zt)
	}
	if zt["version"] != "1.14.2" {
		t.Errorf("zerotier version = %v", zt["version"])
	}
	ts := tools[1]
	if ts["installed"] != true || ts["unitExists"] != false {
		t.Errorf("tailscale = %v", ts)
	}
	if _, ok := ts["active"]; ok {
		t.Error("unit 缺席时不应有 active 字段")
	}
	if ts["version"] != "1.88.4" {
		t.Errorf("tailscale version = %v", ts["version"])
	}
}

func TestOverlayDaemonNonSystemdPlatform(t *testing.T) {
	stubDetectSystemd(t, false)
	fakeCLI := mustFakeCLI(t)
	stubOverlayBins(t, fakeCLI, fakeCLI)
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)

	data, _ := p.Daemon()
	tools := data.(map[string]interface{})["tools"].([]map[string]interface{})
	for _, tool := range tools {
		if tool["installed"] != true {
			t.Errorf("%v installed should be true (CLI present)", tool)
		}
		if _, ok := tool["unitExists"]; ok {
			t.Errorf("非 systemd 平台不应有 unitExists 字段: %v", tool)
		}
	}
}

// ============ service 管理（D27） ============

func TestOverlayServiceValidation(t *testing.T) {
	stubDetectSystemd(t, true)
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)

	// 工具白名单：wireguard/frp/未知/空 一律拒绝
	for _, tool := range []string{"wireguard", "frp", "openvpn", ""} {
		if _, err := p.Service(tool, "start"); err == nil || !strings.Contains(err.Error(), "no manageable overlay daemon unit") {
			t.Errorf("Service(%q) err = %v, want unit whitelist rejection", tool, err)
		}
	}
	// 动词白名单：restart/reload/status 等一律拒绝
	for _, action := range []string{"restart", "reload", "mask", "status", ""} {
		if _, err := p.Service("zerotier", action); err == nil || !strings.Contains(err.Error(), "unsupported service action") {
			t.Errorf("Service(zerotier, %q) err = %v, want action whitelist rejection", action, err)
		}
	}
	if len(rec.calls) != 0 {
		t.Errorf("白名单拒绝不应执行任何命令, calls = %v", rec.calls)
	}
}

func TestOverlayServiceSystemdUnavailable(t *testing.T) {
	stubDetectSystemd(t, false)
	rec := &wrRecorder{routes: map[string]wrRoute{}}
	p := NewOverlayProvider(rec.run)
	if _, err := p.Service("zerotier", "start"); err == nil || !strings.Contains(err.Error(), "systemd not available") {
		t.Errorf("err = %v, want systemd unavailable", err)
	}
}

func TestOverlayServiceUnitNotInstalled(t *testing.T) {
	stubDetectSystemd(t, true)
	rec := &wrRecorder{routes: map[string]wrRoute{
		// cat 失败 = 单元未安装
	}}
	p := NewOverlayProvider(rec.run)
	_, err := p.Service("tailscale", "start")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("err = %v, want unit-not-installed", err)
	}
}

func TestOverlayServiceStartAndFailure(t *testing.T) {
	stubDetectSystemd(t, true)
	rec := &wrRecorder{routes: map[string]wrRoute{
		"systemctl cat zerotier-one.service":   {out: []byte("# unit")},
		"systemctl start zerotier-one.service": {out: []byte("")},
		"systemctl stop zerotier-one.service":  {stderr: []byte("Job failed"), err: exec.ErrNotFound},
	}}
	p := NewOverlayProvider(rec.run)

	data, err := p.Service("zerotier", "start")
	if err != nil {
		t.Fatalf("Service(start) error = %v", err)
	}
	m := data.(map[string]interface{})
	if m["tool"] != "zerotier" || m["unit"] != "zerotier-one.service" || m["action"] != "start" {
		t.Errorf("Service(start) = %v", m)
	}

	if _, err := p.Service("zerotier", "stop"); err == nil || !strings.Contains(err.Error(), "systemctl stop") {
		t.Errorf("stop failure err = %v, want systemctl stop summary", err)
	}
}

// ============ 小工具 ============

func TestTailnetFromDNSName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"nas.tailnet-name.ts.net.", "tailnet-name.ts.net"},
		{"nas.tailnet-name.ts.net", "tailnet-name.ts.net"},
		{"host.corp.example.com", "corp.example.com"},
		{"nas.ts.local", "ts.local"},
		{"nas.local", ""}, // 2 段，形态不足
		{"nas", ""},       // 裸主机名
		{"", ""},          // 空
	}
	for _, c := range cases {
		if got := tailnetFromDNSName(c.in); got != c.want {
			t.Errorf("tailnetFromDNSName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOverlayFirstLineAndClipLine(t *testing.T) {
	if got := firstLine([]byte("  1.14.2\nother\n")); got != "1.14.2" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine([]byte("   \n")); got != "" {
		t.Errorf("firstLine(blank) = %q", got)
	}
	if got := firstLine(nil); got != "" {
		t.Errorf("firstLine(nil) = %q", got)
	}
	if got := clipLine("short", 10); got != "short" {
		t.Errorf("clipLine(short) = %q", got)
	}
	if got := clipLine(strings.Repeat("x", 30), 10); got != strings.Repeat("x", 10)+"…" {
		t.Errorf("clipLine(long) = %q", got)
	}
}
