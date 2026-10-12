package storage

import (
	"time"

	"gorm.io/gorm"
)

// AcmeDeployTarget ACME 证书部署目标（D14 多目标扩展，D-2026-10-12-3）：
// 一张证书可推到多台网关 agent（主网关 + 备网关场景），每行一个
// (证书, agent) 绑定，部署状态随行记录。单目标时代的 AcmeCert 行内
// 五列由 backfillAcmeDeployTargets 一次性迁入本表后弃用（列保留只为
// 回滚安全，读写均不再经过）。
type AcmeDeployTarget struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	CertID          uint   `gorm:"index;uniqueIndex:idx_acme_deploy_target" json:"certId"`
	AgentID         string `gorm:"size:100;uniqueIndex:idx_acme_deploy_target" json:"agentId"`
	CertPath        string `gorm:"size:255" json:"certPath"`
	KeyPath         string `gorm:"size:255" json:"keyPath"`
	LastDeployAt    int64  `json:"lastDeployAt"`                    // 最近一次部署尝试（含失败）Unix 秒，0=从未
	LastDeployError string `gorm:"size:512" json:"lastDeployError"` // 空 = 最近一次部署成功

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListAcmeDeployTargets 按 ID 升序列出证书的部署目标
func (d *DB) ListAcmeDeployTargets(certID uint) ([]AcmeDeployTarget, error) {
	var list []AcmeDeployTarget
	if err := d.db.Where("cert_id = ?", certID).Order("id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ReplaceAcmeDeployTargets 整体替换证书的部署目标（事务内先删后插；
// 绑定编辑语义：请求里给的数组就是全量）。部署状态随行清零——重绑定
// 视为新目标而非延续旧状态。
func (d *DB) ReplaceAcmeDeployTargets(certID uint, targets []AcmeDeployTarget) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("cert_id = ?", certID).Delete(&AcmeDeployTarget{}).Error; err != nil {
			return err
		}
		for i := range targets {
			t := targets[i]
			t.ID = 0
			t.CertID = certID
			t.LastDeployAt = 0
			t.LastDeployError = ""
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateAcmeDeployTargetStatus 回写单目标部署状态（errMsg 空 = 成功）
func (d *DB) UpdateAcmeDeployTargetStatus(id uint, at int64, errMsg string) error {
	return d.db.Model(&AcmeDeployTarget{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_deploy_at":    at,
		"last_deploy_error": errMsg,
	}).Error
}

// DeleteAcmeDeployTargets 删证书时清理其部署目标
func (d *DB) DeleteAcmeDeployTargets(certID uint) error {
	return d.db.Where("cert_id = ?", certID).Delete(&AcmeDeployTarget{}).Error
}

// backfillAcmeDeployTargets 单目标 → 多目标一次性迁移：DeployAgentID
// 非空且名下无目标行的证书迁一行入表并清掉行内五列。幂等：迁移后列已清，
// 二次启动零命中。
func backfillAcmeDeployTargets(d *DB) error {
	var certs []*AcmeCert
	if err := d.db.Where("deploy_agent_id <> ''").Order("id").Find(&certs).Error; err != nil {
		return err
	}
	for _, c := range certs {
		var n int64
		if err := d.db.Model(&AcmeDeployTarget{}).Where("cert_id = ?", c.ID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			t := AcmeDeployTarget{
				CertID:          c.ID,
				AgentID:         c.DeployAgentID,
				CertPath:        c.DeployCertPath,
				KeyPath:         c.DeployKeyPath,
				LastDeployAt:    c.LastDeployAt,
				LastDeployError: c.LastDeployError,
			}
			if err := d.db.Create(&t).Error; err != nil {
				return err
			}
		}
		// 行内五列清零（列保留，读写不再经过；n>0 的重复启动也归零兜底）
		if err := d.db.Model(c).Select("deploy_agent_id", "deploy_cert_path", "deploy_key_path", "last_deploy_at", "last_deploy_error").Updates(map[string]interface{}{
			"deploy_agent_id":   "",
			"deploy_cert_path":  "",
			"deploy_key_path":   "",
			"last_deploy_at":    0,
			"last_deploy_error": "",
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
