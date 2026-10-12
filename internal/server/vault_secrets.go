package server

import (
	"fmt"
	"strings"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/vault"
)

// vaultRefPrefix yaml secret 字段的 vault 引用前缀（vault-design.md V6 第二波，
// 决策 D-2026-10-12-1）：值形如 "vault:dns/cloudflare" 的字段在启动时经
// provider 解析为条目 value。优先级 env > yaml > vault——env 覆盖发生在
// config.Load（applyEnv），vault 引用只在 env 未覆盖该字段时存活；不做约定
// 路径隐式回填（隐式会让「vault 有没有条目」悄悄改变配置语义）。解析失败
// 启动即终止（V7 不降级），错误消息只含字段名与 path、不含任何凭据内容。
const vaultRefPrefix = "vault:"

// resolveVaultRef 解析单字段引用；非引用原样返回。错误只带 path。
func resolveVaultRef(p vault.Provider, raw string) (string, error) {
	if !strings.HasPrefix(raw, vaultRefPrefix) {
		return raw, nil
	}
	path := strings.TrimPrefix(raw, vaultRefPrefix)
	if path == "" {
		return "", fmt.Errorf("vault ref: empty path")
	}
	e, err := p.Read(path)
	if err != nil {
		return "", fmt.Errorf("vault ref %s: %w", path, err)
	}
	if e.Value == "" {
		return "", fmt.Errorf("vault ref %s: entry has no value", path)
	}
	return e.Value, nil
}

// applyVaultSecretRefs 把 cfg 中全部 vault: 引用一次性解析为明文（只在
// 内存 config 流转，V6 第二波：DNS/overlay/notification/SMTP）。任一引用
// 解析失败即返回错误，由启动路径终止进程。
func applyVaultSecretRefs(cfg *config.Config, p vault.Provider) error {
	if cfg == nil || p == nil {
		return nil
	}
	resolve := func(field *string, name string) error {
		v, err := resolveVaultRef(p, *field)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", name, err)
		}
		*field = v
		return nil
	}
	if cfg.DNS != nil {
		if cfg.DNS.Cloudflare != nil {
			if err := resolve(&cfg.DNS.Cloudflare.APIToken, "dns.cloudflare.api_token"); err != nil {
				return err
			}
		}
		if cfg.DNS.DNSPod != nil {
			if err := resolve(&cfg.DNS.DNSPod.LoginToken, "dns.dnspod.login_token"); err != nil {
				return err
			}
		}
		if cfg.DNS.AliDNS != nil {
			if err := resolve(&cfg.DNS.AliDNS.AccessKey, "dns.alidns.access_key"); err != nil {
				return err
			}
			if err := resolve(&cfg.DNS.AliDNS.SecretKey, "dns.alidns.secret_key"); err != nil {
				return err
			}
		}
	}
	if cfg.Overlay != nil {
		if cfg.Overlay.ZeroTier != nil {
			if err := resolve(&cfg.Overlay.ZeroTier.APIToken, "overlay.zerotier.api_token"); err != nil {
				return err
			}
		}
		if cfg.Overlay.Tailscale != nil {
			if err := resolve(&cfg.Overlay.Tailscale.APIToken, "overlay.tailscale.api_token"); err != nil {
				return err
			}
		}
	}
	if cfg.Notification != nil {
		for i, n := range cfg.Notification.Ntfy {
			if n == nil {
				continue
			}
			if err := resolve(&n.Token, fmt.Sprintf("notification.ntfy[%d].token", i)); err != nil {
				return err
			}
		}
		for i, wb := range cfg.Notification.Webhook {
			if wb == nil {
				continue
			}
			if err := resolve(&wb.Secret, fmt.Sprintf("notification.webhook[%d].secret", i)); err != nil {
				return err
			}
		}
		for i, tg := range cfg.Notification.Telegram {
			if tg == nil {
				continue
			}
			if err := resolve(&tg.BotToken, fmt.Sprintf("notification.telegram[%d].bot_token", i)); err != nil {
				return err
			}
		}
	}
	if cfg.Email != nil && cfg.Email.SMTP != nil {
		if err := resolve(&cfg.Email.SMTP.Password, "email.smtp.password"); err != nil {
			return err
		}
	}
	return nil
}
