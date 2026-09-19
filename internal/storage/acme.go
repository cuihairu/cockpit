package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// AcmeAccount ACME 账户（单行表，ID=1；设计见 docs/guide/acme-design.md D5）。
// PrivateKeyPEM 只存库，API 层负责剥离后再响应（D9）。
type AcmeAccount struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	Email           string    `gorm:"size:255" json:"email"`
	PrivateKeyPEM   string    `gorm:"size:4096" json:"-"`
	RegistrationURI string    `gorm:"size:512" json:"registrationURI"`
	CADirectory     string    `gorm:"size:64" json:"caDirectory"` // 账户注册时所用目录（staging/production 账户不通用）
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// AcmeCert 一条证书签发配置与其最新产物。
// 三个 PEM 字段持久化在库，API 层响应时剥离（D9）——PEM 只经下载端点出去。
type AcmeCert struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	Domains         []string  `gorm:"serializer:json" json:"domains"` // 首个为 primary
	PrimaryDomain   string    `gorm:"index;size:255" json:"primaryDomain"`
	CADirectory     string    `gorm:"size:32" json:"caDirectory"` // staging / production
	Status          string    `gorm:"size:16" json:"status"`      // pending / issued / failed
	CertificatePEM  string    `gorm:"size:16384" json:"-"`
	IssuerPEM       string    `gorm:"size:8192" json:"-"`
	PrivateKeyPEM   string    `gorm:"size:8192" json:"-"`
	ExpiresAt       time.Time `gorm:"index" json:"expiresAt"`
	RenewBeforeDays int       `gorm:"default:30" json:"renewBeforeDays"` // 7~90
	AutoRenew       bool      `json:"autoRenew"`
	LastRenewAt     int64     `json:"lastRenewAt"`               // 最近一次签发尝试（含失败）Unix 秒，0=从未——巡检失败节流依据（D8）
	LastStatus      string    `gorm:"size:16" json:"lastStatus"` // never / ok / failed
	LastError       string    `gorm:"size:512" json:"lastError"`
	CheckedAt       int64     `json:"checkedAt"` // 最近一次巡检看到它的时间，0=从未

	// 部署目标（D14，一对一；空 AgentID = 未绑定）：签发成功后把 PEM
	// 经 file.write 推到 agent，nginx 站点以绝对路径引用
	DeployAgentID   string `gorm:"size:100;index" json:"deployAgentId"`
	DeployCertPath  string `gorm:"size:255" json:"deployCertPath"`
	DeployKeyPath   string `gorm:"size:255" json:"deployKeyPath"`
	LastDeployAt    int64  `json:"lastDeployAt"`                    // 最近一次部署尝试（含失败）Unix 秒，0=从未
	LastDeployError string `gorm:"size:512" json:"lastDeployError"` // 空 = 最近一次部署成功

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

var ErrAcmeAccountNotFound = errors.New("acme account not found")

// GetAcmeAccount 读账户（单行）；没有时返回 ErrAcmeAccountNotFound
func (d *DB) GetAcmeAccount() (*AcmeAccount, error) {
	var acc AcmeAccount
	err := d.db.First(&acc, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAcmeAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// SaveAcmeAccount 幂等写入账户（固定 ID=1；已存在则覆盖——email 变更/续注册场景）
func (d *DB) SaveAcmeAccount(acc *AcmeAccount) error {
	acc.ID = 1
	return d.db.Save(acc).Error
}

// UpdateAcmeAccountEmail 只改 email（账户已注册时需要向 CA 同步，见 api 层）
func (d *DB) UpdateAcmeAccountEmail(email string) error {
	acc, err := d.GetAcmeAccount()
	if err != nil {
		return err
	}
	acc.Email = email
	return d.db.Save(acc).Error
}

// DeleteAcmeAccount 清掉账户（下次签发重新注册）
func (d *DB) DeleteAcmeAccount() error {
	return d.db.Delete(&AcmeAccount{}, 1).Error
}

// CreateAcmeCert
func (d *DB) CreateAcmeCert(cert *AcmeCert) error {
	if cert.PrimaryDomain == "" && len(cert.Domains) > 0 {
		cert.PrimaryDomain = cert.Domains[0]
	}
	return d.db.Create(cert).Error
}

// UpdateAcmeCert
func (d *DB) UpdateAcmeCert(cert *AcmeCert) error {
	if cert.PrimaryDomain == "" && len(cert.Domains) > 0 {
		cert.PrimaryDomain = cert.Domains[0]
	}
	return d.db.Save(cert).Error
}

// GetAcmeCert
func (d *DB) GetAcmeCert(id uint) (*AcmeCert, error) {
	var cert AcmeCert
	if err := d.db.First(&cert, id).Error; err != nil {
		return nil, err
	}
	return &cert, nil
}

// ListAcmeCerts 按 ID 升序
func (d *DB) ListAcmeCerts() ([]*AcmeCert, error) {
	var list []*AcmeCert
	if err := d.db.Order("id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteAcmeCert 仅删本地记录，不向 CA 吊销（D9）
func (d *DB) DeleteAcmeCert(id uint) error {
	return d.db.Delete(&AcmeCert{}, id).Error
}
