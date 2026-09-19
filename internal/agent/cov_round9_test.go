package agent

// cov_round9_test.go 覆盖率：setupProviders 按 capability 分发注册的
// 其余分支——traefik-proxy（动态目录取 metadata）、pve-api、openwrt、
// cron+drift（cron 段检查启用）、overlay、hardware-monitor、ddns、nas、
// service 三种 backend（默认 systemd / windows-scm / launchd）。
// logs 分支需 proxyHandler 与 follow 回推，不在目标内。

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestCovSetupProvidersCapabilityMatrix 一次注册全部剩余 capability 类型，
// 断言对应 provider 均已注册（service 三 backend 同 Type，只验不 panic）
func TestCovSetupProvidersCapabilityMatrix(t *testing.T) {
	// pve/openwrt 注册从 env 取凭据，缺了会 skip
	t.Setenv("PVE_URL", "https://127.0.0.1:8006")
	t.Setenv("PVE_TOKEN_ID", "root@pam")
	t.Setenv("PVE_TOKEN_SECRET", "secret")
	t.Setenv("OPENWRT_HOST", "192.168.1.1")
	t.Setenv("OPENWRT_USER", "root")
	t.Setenv("OPENWRT_PASS", "pass")

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{
		{Type: "traefik-proxy", Metadata: map[string]interface{}{"dynamicDir": t.TempDir()}},
		{Type: "pve-api"},
		{Type: "openwrt"},
		{Type: "cron"},
		{Type: "drift"},
		{Type: "overlay"},
		{Type: "hardware-monitor"},
		{Type: "ddns"},
		{Type: "nas"},
		{Type: "service"}, // backend 缺省 → systemd
		{Type: "service", Metadata: map[string]interface{}{"backend": "windows-scm"}},
		{Type: "service", Metadata: map[string]interface{}{"backend": "launchd"}},
	}
	a.setupProviders()

	for _, typ := range []string{"traefik", "pve", "openwrt", "cron", "drift",
		"overlay", "hardware-monitor", "ddns", "nas", "service"} {
		if !covRegistered(a, typ) {
			t.Errorf("provider %q should be registered, got %v", typ, a.rpc.RegisteredTypes())
		}
	}
}
