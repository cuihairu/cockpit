package server

import (
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/vault"
)

func writeSecretEntry(t *testing.T, p vault.Provider, path, value string) {
	t.Helper()
	if err := p.Write(path, &vault.Entry{Path: path, Kind: "pair", Value: value}); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// resolveVaultRef：非引用透传、空 path、缺条目、无 value、正常解析。
func TestResolveVaultRef(t *testing.T) {
	s, p := newVaultNasServer(t)
	_ = s
	writeSecretEntry(t, p, "dns/cloudflare", "cf-token-XYZ")

	// 非引用原样透传
	got, err := resolveVaultRef(p, "plain-token")
	if err != nil || got != "plain-token" {
		t.Fatalf("passthrough = %q, %v", got, err)
	}

	// 空 path
	if _, err := resolveVaultRef(p, "vault:"); err == nil || !strings.Contains(err.Error(), "empty path") {
		t.Fatalf("empty path err = %v", err)
	}

	// 缺条目：错误带 path、不带任何凭据内容
	if _, err := resolveVaultRef(p, "vault:dns/missing"); err == nil || !strings.Contains(err.Error(), "dns/missing") {
		t.Fatalf("missing entry err = %v", err)
	}

	// 条目无 value
	writeSecretEntry(t, p, "dns/empty", "")
	if _, err := resolveVaultRef(p, "vault:dns/empty"); err == nil || !strings.Contains(err.Error(), "no value") {
		t.Fatalf("empty value err = %v", err)
	}

	// 正常解析
	got, err = resolveVaultRef(p, "vault:dns/cloudflare")
	if err != nil || got != "cf-token-XYZ" {
		t.Fatalf("resolve = %q, %v", got, err)
	}
}

// applyVaultSecretRefs：全字段引用一次解析；非引用与 nil 节安全。
func TestApplyVaultSecretRefs(t *testing.T) {
	s, p := newVaultNasServer(t)
	_ = s
	for _, e := range [][2]string{
		{"dns/cloudflare", "cf-1"}, {"dns/dnspod", "dp-1"},
		{"dns/ali-id", "ali-id-1"}, {"dns/ali-secret", "ali-secret-1"},
		{"overlay/zerotier", "zt-1"}, {"overlay/tailscale", "ts-1"},
		{"notify/ntfy", "ntfy-1"}, {"notify/hook", "hook-1"}, {"notify/tg", "tg-1"},
		{"notify/smtp", "smtp-1"},
	} {
		writeSecretEntry(t, p, e[0], e[1])
	}

	cfg := &config.Config{
		DNS: &config.DNSConfig{
			Cloudflare: &config.CloudflareDNSConfig{APIToken: "vault:dns/cloudflare"},
			DNSPod:     &config.DNSPodConfig{LoginToken: "vault:dns/dnspod"},
			AliDNS:     &config.AliDNSConfig{AccessKey: "vault:dns/ali-id", SecretKey: "vault:dns/ali-secret"},
		},
		Overlay: &config.OverlayConfig{
			ZeroTier:  &config.ZeroTierCloudConfig{APIToken: "vault:overlay/zerotier"},
			Tailscale: &config.TailscaleCloudConfig{APIToken: "vault:overlay/tailscale"},
		},
		Notification: &config.NotificationConfig{
			Ntfy:     []*config.NtfyConfig{{Server: "https://n", Token: "vault:notify/ntfy"}},
			Webhook:  []*config.WebhookConfig{{URL: "https://w", Secret: "vault:notify/hook"}},
			Telegram: []*config.TelegramConfig{{BotToken: "vault:notify/tg", ChatID: "1"}},
		},
		Email: &config.EmailConfig{Enabled: true, SMTP: &config.SMTPConfig{
			Host: "smtp.x", Password: "vault:notify/smtp",
		}},
	}
	if err := applyVaultSecretRefs(cfg, p); err != nil {
		t.Fatalf("applyVaultSecretRefs: %v", err)
	}
	checks := map[string]string{
		cfg.DNS.Cloudflare.APIToken:           "cf-1",
		cfg.DNS.DNSPod.LoginToken:             "dp-1",
		cfg.DNS.AliDNS.AccessKey:              "ali-id-1",
		cfg.DNS.AliDNS.SecretKey:              "ali-secret-1",
		cfg.Overlay.ZeroTier.APIToken:         "zt-1",
		cfg.Overlay.Tailscale.APIToken:        "ts-1",
		cfg.Notification.Ntfy[0].Token:        "ntfy-1",
		cfg.Notification.Webhook[0].Secret:    "hook-1",
		cfg.Notification.Telegram[0].BotToken: "tg-1",
		cfg.Email.SMTP.Password:               "smtp-1",
	}
	for got, want := range checks {
		if got != want {
			t.Fatalf("resolved = %q, want %q", got, want)
		}
	}

	// 非引用字段与空节：不动、不报错
	cfg2 := &config.Config{
		DNS:          &config.DNSConfig{Cloudflare: &config.CloudflareDNSConfig{APIToken: "literal"}},
		Notification: &config.NotificationConfig{Ntfy: []*config.NtfyConfig{nil}},
	}
	if err := applyVaultSecretRefs(cfg2, p); err != nil {
		t.Fatalf("nil-safe apply: %v", err)
	}
	if cfg2.DNS.Cloudflare.APIToken != "literal" {
		t.Fatal("non-ref value must stay untouched")
	}
}

// 任一引用解析失败：错误带字段名与 path、不带明文。
func TestApplyVaultSecretRefsFailure(t *testing.T) {
	s, p := newVaultNasServer(t)
	_ = s
	writeSecretEntry(t, p, "dns/cloudflare", "cf-good-secret-9q")

	cfg := &config.Config{
		DNS: &config.DNSConfig{
			Cloudflare: &config.CloudflareDNSConfig{APIToken: "vault:dns/cloudflare"},
			DNSPod:     &config.DNSPodConfig{LoginToken: "vault:dns/nope"},
		},
	}
	err := applyVaultSecretRefs(cfg, p)
	if err == nil || !strings.Contains(err.Error(), "dns.dnspod.login_token") || !strings.Contains(err.Error(), "dns/nope") {
		t.Fatalf("failure err = %v", err)
	}
	if strings.Contains(err.Error(), "cf-good-secret-9q") {
		t.Fatal("error must not contain plaintext")
	}
	// 已解析字段保持明文（顺序解析，失败即中止）
	if cfg.DNS.Cloudflare.APIToken != "cf-good-secret-9q" {
		t.Fatalf("resolved before failure = %q", cfg.DNS.Cloudflare.APIToken)
	}
}

// 启动路径：引用解析失败 → NewServer fatal（V7 不降级）。
func TestNewServerVaultRefResolveFatals(t *testing.T) {
	t.Chdir(t.TempDir())
	covFinalInterceptFatal(t)
	covFinalWantFatal(t, "Failed to resolve vault secret refs", func() {
		NewServer(&config.Config{
			Database: &config.DatabaseConfig{Path: "./vault-ref-fatal-test.db"},
			DNS:      &config.DNSConfig{Cloudflare: &config.CloudflareDNSConfig{APIToken: "vault:dns/missing"}},
		})
	})
}

// 表驱动：每个字段独立坏引用，逐一命中各自的错误分支（错误含字段名+path）。
func TestApplyVaultSecretRefsFieldFailures(t *testing.T) {
	s, p := newVaultNasServer(t)
	_ = s
	writeSecretEntry(t, p, "dns/ali-id", "ali-id-ok")
	cases := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"cloudflare", &config.Config{DNS: &config.DNSConfig{
			Cloudflare: &config.CloudflareDNSConfig{APIToken: "vault:dns/miss-cf"}}}, "dns.cloudflare.api_token"},
		{"dnspod", &config.Config{DNS: &config.DNSConfig{
			DNSPod: &config.DNSPodConfig{LoginToken: "vault:dns/miss-dp"}}}, "dns.dnspod.login_token"},
		{"ali-access", &config.Config{DNS: &config.DNSConfig{
			AliDNS: &config.AliDNSConfig{AccessKey: "vault:dns/miss-ak"}}}, "dns.alidns.access_key"},
		{"ali-secret", &config.Config{DNS: &config.DNSConfig{
			AliDNS: &config.AliDNSConfig{AccessKey: "vault:dns/ali-id", SecretKey: "vault:dns/miss-sk"}}}, "dns.alidns.secret_key"},
		{"zerotier", &config.Config{Overlay: &config.OverlayConfig{
			ZeroTier: &config.ZeroTierCloudConfig{APIToken: "vault:overlay/miss-zt"}}}, "overlay.zerotier.api_token"},
		{"tailscale", &config.Config{Overlay: &config.OverlayConfig{
			Tailscale: &config.TailscaleCloudConfig{APIToken: "vault:overlay/miss-ts"}}}, "overlay.tailscale.api_token"},
		{"ntfy", &config.Config{Notification: &config.NotificationConfig{
			Ntfy: []*config.NtfyConfig{{Token: "vault:notify/miss-ntfy"}}}}, "notification.ntfy[0].token"},
		{"webhook", &config.Config{Notification: &config.NotificationConfig{
			Webhook: []*config.WebhookConfig{{Secret: "vault:notify/miss-hook"}}}}, "notification.webhook[0].secret"},
		{"telegram", &config.Config{Notification: &config.NotificationConfig{
			Telegram: []*config.TelegramConfig{{BotToken: "vault:notify/miss-tg"}}}}, "notification.telegram[0].bot_token"},
		{"smtp", &config.Config{Email: &config.EmailConfig{SMTP: &config.SMTPConfig{
			Password: "vault:notify/miss-smtp"}}}, "email.smtp.password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := applyVaultSecretRefs(tc.cfg, p)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "vault:") && !strings.Contains(err.Error(), "vault ref") {
				t.Fatalf("error leaked raw ref: %v", err)
			}
		})
	}
}

// nil 元素与 nil cfg/provider 安全跳过。
func TestApplyVaultSecretRefsNilElements(t *testing.T) {
	s, p := newVaultNasServer(t)
	_ = s
	cfg := &config.Config{Notification: &config.NotificationConfig{
		Ntfy:     []*config.NtfyConfig{nil},
		Webhook:  []*config.WebhookConfig{nil},
		Telegram: []*config.TelegramConfig{nil},
	}}
	if err := applyVaultSecretRefs(cfg, p); err != nil {
		t.Fatalf("nil elements must be skipped: %v", err)
	}
	if err := applyVaultSecretRefs(nil, nil); err != nil {
		t.Fatalf("nil cfg/provider must no-op: %v", err)
	}
}
