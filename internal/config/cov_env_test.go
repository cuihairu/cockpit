package config

import "testing"

// Overlay 组网云 token 的 env 覆盖（与 DNS 凭据同一惯例：secret 不落文件）
func TestOverlayEnvOverride(t *testing.T) {
	t.Setenv("ZEROTIER_API_TOKEN", "env-zt")
	t.Setenv("TAILSCALE_API_TOKEN", "env-ts")
	cfg := Normalize(&Config{Overlay: &OverlayConfig{
		ZeroTier:  &ZeroTierCloudConfig{APIToken: "yaml-zt"},
		Tailscale: &TailscaleCloudConfig{APIToken: "yaml-ts"},
	}})
	if cfg.Overlay.ZeroTier.APIToken != "env-zt" {
		t.Errorf("ZeroTier.APIToken = %q, want env-zt", cfg.Overlay.ZeroTier.APIToken)
	}
	if cfg.Overlay.Tailscale.APIToken != "env-ts" {
		t.Errorf("Tailscale.APIToken = %q, want env-ts", cfg.Overlay.Tailscale.APIToken)
	}

	// env 未设置时保留 yaml 值；完全未配置时子结构补齐为空
	t.Setenv("ZEROTIER_API_TOKEN", "")
	t.Setenv("TAILSCALE_API_TOKEN", "")
	cfg = Normalize(&Config{})
	if cfg.Overlay == nil || cfg.Overlay.ZeroTier == nil || cfg.Overlay.Tailscale == nil {
		t.Fatalf("default overlay = %+v", cfg.Overlay)
	}
	if cfg.Overlay.ZeroTier.APIToken != "" || cfg.Overlay.Tailscale.APIToken != "" {
		t.Fatalf("default overlay tokens = %q / %q",
			cfg.Overlay.ZeroTier.APIToken, cfg.Overlay.Tailscale.APIToken)
	}
}
