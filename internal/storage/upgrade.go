package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Agent 升级与回滚（P0 Agent 第四片，见 docs/design/agent-upgrade-rollback.md）。
// 控制面只做产物登记/验证、单 agent 显式升级意图状态机、健康检查与回退成功
// 条件判定；agent 二进制 OS 级换装需真机验收（设计 §2 诚实延后，本片不推包）。

// AgentUpgradeStatus 升级意图状态（设计 §3.2 状态机）。
const (
	UpgradeStatusPlanned    = "planned"     // 意图已建，产物校验通过，待部署
	UpgradeStatusDeploying  = "deploying"   // 已武装，等待 agent 重部署（换装外部/真机）
	UpgradeStatusVerifying  = "verifying"   // 观察到 agent 上报目标版本，健康窗口计时
	UpgradeStatusSucceeded  = "succeeded"   // 健康窗口内稳定在目标版本
	UpgradeStatusRolledBack = "rolled_back" // 回退成功：agent 回到原版本
	UpgradeStatusFailed     = "failed"      // 窗口过期/异常，人工介入
)

// TerminalUpgradeStatus 终态集合（不再推进）。
func TerminalUpgradeStatus(s string) bool {
	return s == UpgradeStatusSucceeded || s == UpgradeStatusRolledBack || s == UpgradeStatusFailed
}

// AgentArtifact 升级产物登记（设计 §3.1 升级产物验证）。
// 同 version+arch+os 覆盖更新（幂等），sha256 与内容一致性在 API 层校验。
type AgentArtifact struct {
	ID      string `gorm:"primaryKey" json:"id"`
	Version string `gorm:"uniqueIndex:idx_artifact_vao" json:"version"`
	Arch    string `gorm:"uniqueIndex:idx_artifact_vao" json:"arch"`
	OS      string `gorm:"uniqueIndex:idx_artifact_vao" json:"os"`
	Size    int64  `json:"size"`
	SHA256  string `gorm:"index" json:"sha256"`
	// MinVersion 最低可升级起点（semver-lite）；空 = 不限（任何版本可升到此产物）
	MinVersion string    `json:"minVersion"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// AgentUpgrade 单 agent 显式升级意图（设计 §3.2；非 bulk，仅单 agent）。
// RequiredCaps 升级后 agent 仍需声明的能力契约（空 = 不校验）；校验失败
// 属回退条件之一（设计 §3.3）。
type AgentUpgrade struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	AgentID      string     `gorm:"index" json:"agentId"`
	FromVersion  string     `json:"fromVersion"`
	ToVersion    string     `json:"toVersion"`
	ArtifactID   string     `json:"artifactId"`
	Status       string     `gorm:"index;default:planned" json:"status"`
	Reason       string     `json:"reason,omitempty"`
	Operator     string     `json:"operator"`
	RequiredCaps []string   `gorm:"serializer:json" json:"requiredCapabilities,omitempty"`
	DeployedAt   *time.Time `json:"deployedAt,omitempty"`
	VerifiedAt   *time.Time `json:"verifiedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// SaveArtifact 登记产物（同 version+arch+os 覆盖更新，幂等）。
// 已存在同 platform 的登记时沿用其主键（覆盖内容而非另建一行），
// 调用方给的随机 id 在此被忽略——否则同平台重复上传会撞唯一约束。
func (d *DB) SaveArtifact(a *AgentArtifact) error {
	if existing, err := d.GetArtifact(a.Version, a.Arch, a.OS); err == nil && existing != nil {
		a.ID = existing.ID
		if a.CreatedAt.IsZero() {
			a.CreatedAt = existing.CreatedAt
		}
	}
	return d.db.Save(a).Error
}

// GetArtifact 按 version+arch+os 查产物；不存在返回 (nil, nil)。
func (d *DB) GetArtifact(version, arch, osName string) (*AgentArtifact, error) {
	var a AgentArtifact
	err := d.db.Where("version = ? AND arch = ? AND os = ?", version, arch, osName).
		First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// ListArtifacts 全部产物（倒序）。
func (d *DB) ListArtifacts() ([]AgentArtifact, error) {
	var out []AgentArtifact
	err := d.db.Order("created_at desc").Find(&out).Error
	return out, err
}

// CreateUpgrade 新建升级意图。
func (d *DB) CreateUpgrade(u *AgentUpgrade) error {
	return d.db.Create(u).Error
}

// GetUpgrade 按 id 查意图；不存在返回 (nil, nil)。
func (d *DB) GetUpgrade(id string) (*AgentUpgrade, error) {
	var u AgentUpgrade
	err := d.db.Where("id = ?", id).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// ListUpgrades 意图列表（agentID 空 = 全部；倒序，limit<=0 用 50）。
func (d *DB) ListUpgrades(agentID string, limit int) ([]AgentUpgrade, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []AgentUpgrade
	q := d.db
	if agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	err := q.Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// ListActiveUpgrades 非终态意图（巡检推进用）。
func (d *DB) ListActiveUpgrades() ([]AgentUpgrade, error) {
	var out []AgentUpgrade
	err := d.db.Where("status NOT IN ?",
		[]string{UpgradeStatusSucceeded, UpgradeStatusRolledBack, UpgradeStatusFailed}).
		Order("created_at asc").Find(&out).Error
	return out, err
}

// AdvanceUpgrade 条件推进：仅当仍处于 fromStatus 时翻到 toStatus。
// 返回 RowsAffected（0 = 未推进，说明已被并发/其他循环推进）。
func (d *DB) AdvanceUpgrade(id, fromStatus, toStatus, reason string, now time.Time) (int64, error) {
	updates := map[string]interface{}{
		"status":     toStatus,
		"reason":     reason,
		"updated_at": now,
	}
	switch toStatus {
	case UpgradeStatusDeploying:
		updates["deployed_at"] = now
	case UpgradeStatusVerifying:
		updates["verified_at"] = now
	case UpgradeStatusSucceeded, UpgradeStatusRolledBack, UpgradeStatusFailed:
		updates["finished_at"] = now
	}
	res := d.db.Model(&AgentUpgrade{}).Where("id = ? AND status = ?", id, fromStatus).
		Updates(updates)
	return res.RowsAffected, res.Error
}
