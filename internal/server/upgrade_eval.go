package server

import (
	"fmt"
	"regexp"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 升级/回滚判定纯函数（P0 Agent 第四片，见 docs/design/agent-upgrade-rollback.md
// §3.3 健康窗口/回退成功条件）。全部可单测，不触 DB。

// DefaultUpgradeHealthWindow 健康窗口默认值（设计 §3.3：120s）。
const DefaultUpgradeHealthWindow = 120 * time.Second

// upgradeHealthWindow 读取配置窗口（≤0 回默认）。
func (s *Server) upgradeHealthWindow() time.Duration {
	if s.cfg == nil || s.cfg.Upgrade == nil || s.cfg.Upgrade.HealthWindowS <= 0 {
		return DefaultUpgradeHealthWindow
	}
	return time.Duration(s.cfg.Upgrade.HealthWindowS) * time.Second
}

// upgradeMaxArtifactBytes 产物大小上界（配置 maxArtifactMB，默认 256MB）。
func (s *Server) upgradeMaxArtifactBytes() int64 {
	const def = 256 << 20
	if s.cfg == nil || s.cfg.Upgrade == nil || s.cfg.Upgrade.MaxArtifactMB <= 0 {
		return def
	}
	return int64(s.cfg.Upgrade.MaxArtifactMB) << 20
}

// versionFormatRe semver-lite 格式（v? 数字段点分），供产物版本校验。
var versionFormatRe = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)*$`)

// ValidUpgradeVersion 版本可解析（semver-lite 格式）。
func ValidUpgradeVersion(v string) bool {
	return versionFormatRe.MatchString(v)
}

// validateArtifactMeta 产物元数据校验（设计 §3.1）：版本可解析、min_version
// 满足、size 正且有上界、sha256 为 64 位 hex。返回人类可读错误。
func validateArtifactMeta(version, minVersion, sha256 string, size, maxBytes int64) error {
	if !ValidUpgradeVersion(version) {
		return fmt.Errorf("invalid version %q (want semver-lite, e.g. 1.2.0)", version)
	}
	if minVersion != "" && !ValidUpgradeVersion(minVersion) {
		return fmt.Errorf("invalid min_version %q", minVersion)
	}
	if !protocol.VersionSatisfies(version, minVersion) {
		return fmt.Errorf("version %q does not satisfy min_version %q", version, minVersion)
	}
	if size <= 0 {
		return fmt.Errorf("artifact is empty")
	}
	if maxBytes > 0 && size > maxBytes {
		return fmt.Errorf("artifact %d bytes exceeds limit %d", size, maxBytes)
	}
	if len(sha256) != 64 {
		return fmt.Errorf("invalid sha256 length %d (want 64)", len(sha256))
	}
	for i := 0; i < len(sha256); i++ {
		c := sha256[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return fmt.Errorf("invalid sha256 (non-hex char at %d)", i)
		}
	}
	return nil
}

// upgradeNextStatus 纯判定：返回 (下一状态, 原因)；("", "") = 保持现状。
//
//   - planned → deploying：产物已在创建时校验，巡检武装意图。
//   - deploying：agent 上报目标版本 → verifying；窗口内未变保持；
//     窗口过期且仍在原版本 → rolled_back（回退条件「版本仍为原版本」命中，
//     agent 已在原版本即回退成功）；窗口过期且 agent 离线/异常版本 → failed。
//   - verifying：能力契约失败 → rolled_back；稳定在目标版本满窗口 →
//     succeeded；agent 回到原版本 → rolled_back；agent 离线/异常 → failed。
func upgradeNextStatus(u *storage.AgentUpgrade, agentVersion string, window time.Duration, now time.Time, capOK bool) (string, string) {
	switch u.Status {
	case storage.UpgradeStatusPlanned:
		return storage.UpgradeStatusDeploying, "artifact verified, intent armed"
	case storage.UpgradeStatusDeploying:
		switch {
		case agentVersion == u.ToVersion:
			return storage.UpgradeStatusVerifying, "agent reports target version"
		case agentVersion == "":
			if u.DeployedAt != nil && now.Sub(*u.DeployedAt) > window {
				return storage.UpgradeStatusFailed, "health window expired without agent re-registration"
			}
		case agentVersion == u.FromVersion:
			if u.DeployedAt != nil && now.Sub(*u.DeployedAt) > window {
				return storage.UpgradeStatusRolledBack, "window expired, agent still on original version"
			}
		default:
			if u.DeployedAt != nil && now.Sub(*u.DeployedAt) > window {
				return storage.UpgradeStatusFailed, "unexpected version reported during deploy"
			}
		}
	case storage.UpgradeStatusVerifying:
		switch {
		case !capOK:
			return storage.UpgradeStatusRolledBack, "capability contract check failed after upgrade"
		case agentVersion == u.ToVersion:
			if u.VerifiedAt != nil && now.Sub(*u.VerifiedAt) >= window {
				return storage.UpgradeStatusSucceeded, "health window passed at target version"
			}
		case agentVersion == u.FromVersion:
			return storage.UpgradeStatusRolledBack, "agent reverted to original version"
		case agentVersion == "":
			if u.VerifiedAt != nil && now.Sub(*u.VerifiedAt) > window {
				return storage.UpgradeStatusFailed, "agent offline during health window"
			}
		default:
			return storage.UpgradeStatusFailed, "unexpected version during verification"
		}
	}
	return "", ""
}

// upgradeCapabilityOK agent 上报能力是否满足意图要求的契约（空要求 = true）。
func upgradeCapabilityOK(reported []storage.Capability, required []string) bool {
	if len(required) == 0 {
		return true
	}
	have := map[string]bool{}
	for _, c := range reported {
		have[c.Type] = true
	}
	for _, r := range required {
		if !have[r] {
			return false
		}
	}
	return true
}
