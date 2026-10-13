package server

import (
	"fmt"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

// systemVaultAuditUser provider 级审计的用户名占位（进程自身操作，
// 非 HTTP 请求发起；用户级动作由既有 vault-token 门单独审计）。
const systemVaultAuditUser = "cockpit"

// initVault 构建密码箱 provider（vault-design.md V4/V7）：默认 local，
// 未知 provider 返回错误由启动路径终止进程（不降级）。本地实现挂
// vault_entries 表（经同一 sqlite）与审计钩子。
func initVault(cfg *config.VaultConfig, db *storage.DB, logger *audit.Logger) (vault.Provider, error) {
	name := "local"
	var ttl time.Duration
	if cfg != nil {
		if cfg.Provider != "" {
			name = cfg.Provider
		}
		ttl = cfg.SessionTTL
	}
	var hook vault.AuditFunc
	if logger != nil {
		hook = func(action, path string, ok bool) {
			details := map[string]string{"path": path}
			if ok {
				logger.LogSuccess(systemVaultAuditUser, action, "vault", path, details, "", "")
			} else {
				logger.LogFailure(systemVaultAuditUser, action, "vault", path, details, "", "")
			}
		}
	}
	vault.Register("local", func(map[string]string) (vault.Provider, error) {
		return vault.NewLocal(db.Gorm(), ttl, hook)
	})
	// persona（只读 Connect API 实现，接口映射见 docs/guide/persona-vault-mapping.md）：
	// endpoint/token/timeout 进 cfg map，session_ttl 未设回退全局；校验在
	// NewPersona 内做（provider=persona 选中时启动即报错，fail-fast 不降级）。
	personaCfg := map[string]string{}
	personaTTL := ttl
	if cfg != nil && cfg.Providers != nil && cfg.Providers.Persona != nil {
		pc := cfg.Providers.Persona
		if pc.Endpoint != "" {
			personaCfg["endpoint"] = pc.Endpoint
		}
		if pc.Token != "" {
			personaCfg["token"] = pc.Token
		}
		if pc.Timeout > 0 {
			personaCfg["timeout"] = pc.Timeout.String()
		}
		if pc.SessionTTL > 0 {
			personaTTL = pc.SessionTTL
		}
	}
	vault.Register("persona", func(map[string]string) (vault.Provider, error) {
		return vault.NewPersona(personaCfg, personaTTL, hook)
	})
	p, err := vault.New(name, nil)
	if err != nil {
		return nil, fmt.Errorf("vault provider %q: %w", name, err)
	}
	return p, nil
}
