package rpc

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// overlayFakeRun 按命令名+首参数路由到预设输出的可编程 Commander。
// 未命中的命令返回 exec.ErrNotFound 形态错误（模拟未安装）。
func overlayFakeRun(routes map[string]overlayRoute) Commander {
	return func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		key := name
		if len(args) > 0 {
			key = name + " " + args[0]
		}
		if r, ok := routes[key]; ok {
			return r.out, nil, r.err
		}
		return nil, nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
}

type overlayRoute struct {
	out []byte
	err error
}

// notFoundErr 模拟注入 Commander 下的命令缺失（exec.Error 形态）
func notFoundErr(name string) error {
	return &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func TestOverlayProviderUnknownAction(t *testing.T) {
	p := NewOverlayProvider(nil)
	if _, err := p.Call("nope", nil); err == nil || !strings.Contains(err.Error(), "unknown overlay action") {
		t.Errorf("error = %v, want unknown overlay action", err)
	}
	if p.Type() != "overlay" {
		t.Errorf("Type() = %q, want overlay", p.Type())
	}
}

func TestOverlayStatusAllUnavailable(t *testing.T) {
	run := overlayFakeRun(map[string]overlayRoute{})
	p := NewOverlayProvider(run)
	data, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call(status) error = %v", err)
	}
	tools := data.(map[string]interface{})["tools"].([]overlayTool)
	if len(tools) != 4 {
		t.Fatalf("len(tools) = %d, want 4", len(tools))
	}
	for _, tool := range tools {
		if tool.Status != "unavailable" {
			t.Errorf("tool %s status = %q, want unavailable", tool.Tool, tool.Status)
		}
	}
}

func TestOverlayStatusZeroTierOK(t *testing.T) {
	run := overlayFakeRun(map[string]overlayRoute{
		"zerotier-cli -j": {out: []byte(`{"version":"1.14.2","online":true}`)}, // info
	})
	// -j 路由无法区分 info/listnetworks/listpeers——用多参数完整键替换
	run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		full := name + " " + strings.Join(args, " ")
		switch full {
		case "zerotier-cli -j info":
			return []byte(`{"version":"1.14.2","online":true}`), nil, nil
		case "zerotier-cli -j listnetworks":
			return []byte(`[{"nwid":"n1","name":"home","status":"OK","type":"Private","dev":"zt0","ips":["10.0.0.5/24"]}]`), nil, nil
		case "zerotier-cli -j listpeers":
			return []byte(`[{"address":"aaa","version":"1.14.2","latency":12,"role":"LEAF","paths":[{"address":"1.1.1.1/9993","active":true,"preferred":true}]}]`), nil, nil
		}
		return nil, nil, notFoundErr(name)
	}
	p := NewOverlayProvider(run)
	data, _ := p.Call("status", nil)
	tools := data.(map[string]interface{})["tools"].([]overlayTool)
	zt := tools[0]
	if zt.Tool != "zerotier" || zt.Status != "ok" || zt.Version != "1.14.2" {
		t.Fatalf("zerotier tool = %+v", zt)
	}
	if len(zt.Networks) != 1 || len(zt.Peers) != 1 {
		t.Errorf("networks = %d, peers = %d, want 1/1", len(zt.Networks), len(zt.Peers))
	}
}

func TestOverlayStatusZeroTierNotOnline(t *testing.T) {
	run := overlayFakeRun(map[string]overlayRoute{
		"zerotier-cli -j": {out: []byte(`{"version":"1.14.2","online":false}`)},
	})
	p := NewOverlayProvider(run)
	data, _ := p.Call("status", nil)
	zt := data.(map[string]interface{})["tools"].([]overlayTool)[0]
	if zt.Status != "degraded" {
		t.Errorf("status = %q, want degraded when core not online", zt.Status)
	}
}

func TestOverlayStatusTailscaleFallbackWithoutPeersFlag(t *testing.T) {
	calls := 0
	run := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		full := name + " " + strings.Join(args, " ")
		if full == "tailscale status --json --peers" {
			calls++
			return nil, nil, errors.New("unknown flag: --peers")
		}
		if full == "tailscale status --json" {
			calls++
			return []byte(`{"Version":"1.60.0","BackendState":"Running","Self":{"HostName":"h","TailscaleIPs":["100.64.0.1"],"Online":true},"Peer":{}}`), nil, nil
		}
		return nil, nil, notFoundErr(name)
	}
	p := NewOverlayProvider(run)
	data, _ := p.Call("status", nil)
	ts := data.(map[string]interface{})["tools"].([]overlayTool)[1]
	if ts.Tool != "tailscale" || ts.Status != "ok" || calls != 2 {
		t.Errorf("tailscale = %+v, calls = %d (want fallback executed)", ts, calls)
	}
}

func TestOverlayStatusWireGuardStripsSecretsEndToEnd(t *testing.T) {
	run := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		full := name + " " + strings.Join(args, " ")
		switch full {
		case "wg --version":
			return []byte("wireguard-tools v1.0.2"), nil, nil
		case "wg show all dump":
			return []byte(wgDumpFixture), nil, nil
		}
		return nil, nil, notFoundErr(name)
	}
	p := NewOverlayProvider(run)
	data, _ := p.Call("status", nil)
	tools := data.(map[string]interface{})["tools"].([]overlayTool)
	wg := tools[2]
	if wg.Tool != "wireguard" || wg.Status != "ok" || wg.Version != "v1.0.2" {
		t.Fatalf("wireguard tool = %+v", wg)
	}
	blob := marshalJSONForTest(tools)
	if strings.Contains(blob, "SECRET") {
		t.Fatal("overlay.status leaked wireguard secrets (D6 violation)")
	}
	if len(wg.Interfaces) != 2 {
		t.Errorf("interfaces = %d, want 2", len(wg.Interfaces))
	}
}

func TestOverlayStatusFRPBasicWithAdminConfig(t *testing.T) {
	t.Setenv("COCKPIT_FRPC_ADMIN", "127.0.0.1:1") // 不可达端口 → degraded
	t.Setenv("COCKPIT_FRPS_ADMIN", "")
	run := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		full := name + " " + strings.Join(args, " ")
		switch {
		case full == "frpc -v":
			return []byte("0.52.3\n"), nil, nil
		case name == "pgrep":
			return []byte("123\n"), nil, nil // frpc/frps 均在跑
		}
		return nil, nil, notFoundErr(name)
	}
	p := NewOverlayProvider(run)
	data, _ := p.Call("status", nil)
	frp := data.(map[string]interface{})["tools"].([]overlayTool)[3]
	if frp.Tool != "frp" {
		t.Fatalf("tool = %+v", frp)
	}
	if frp.Version != "0.52.3" {
		t.Errorf("version = %q", frp.Version)
	}
	if frp.Status != "degraded" {
		t.Errorf("status = %q, want degraded (admin api unreachable)", frp.Status)
	}
	frpcExtra := frp.Extra["frpc"].(map[string]interface{})
	if frpcExtra["running"] != true {
		t.Errorf("frpc running = %v, want true", frpcExtra["running"])
	}
	if frpcExtra["adminError"] == "" {
		t.Error("frpc adminError should be set on unreachable admin api")
	}
	if frpsExtra, ok := frp.Extra["frps"].(map[string]interface{}); !ok || frpsExtra["running"] != true {
		t.Errorf("frps extra = %v", frp.Extra["frps"])
	}
}

func TestOverlayStatusFRPOnlyServerBinary(t *testing.T) {
	run := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name == "frpc" {
			return nil, nil, notFoundErr(name)
		}
		if name == "frps" && len(args) == 1 && args[0] == "-v" {
			return []byte("0.51.0\n"), nil, nil
		}
		return nil, nil, notFoundErr(name)
	}
	p := NewOverlayProvider(run)
	data, _ := p.Call("status", nil)
	frp := data.(map[string]interface{})["tools"].([]overlayTool)[3]
	if frp.Status == "unavailable" {
		t.Error("frps-only host should not be unavailable")
	}
	if frp.Version != "0.51.0" {
		t.Errorf("version = %q, want from frps", frp.Version)
	}
}

func TestClipOutput(t *testing.T) {
	if got := clipOutput([]byte("short")); string(got) != "short" {
		t.Errorf("clipOutput short passthrough = %q", got)
	}
	big := strings.Repeat("line\n", overlayMaxOutput/5+10)
	clipped := clipOutput([]byte(big))
	if len(clipped) > overlayMaxOutput {
		t.Errorf("clipped len = %d, want <= %d", len(clipped), overlayMaxOutput)
	}
	if !strings.HasSuffix(string(clipped), "\n") {
		t.Error("clipped output should end at line boundary")
	}
}
