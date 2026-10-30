package storage

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"gorm.io/gorm"
)

// 云账号与云实例的持久层（片2：docs/design/cloud-provider.md §5/§6）。
// 凭据红线：表内只存 `${VAR}` 形态的 vault 引用，明文永不落库/落日志；
// 解析在运行时做（ResolveCloudCredentialRef），未设值 fail-closed。

// cloudCredentialRefRe 是凭据引用的唯一合法形态：整串 `${VAR_NAME}`。
// 中间拼接、前后缀夹带一律拒绝——引用外任何字符都可能藏明文。
var cloudCredentialRefRe = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// ValidateCloudCredentialRef 校验引用形态。空串与非法形态都拒绝：
// 无凭据的账号是配置错误，不得静默通过。
func ValidateCloudCredentialRef(ref string) error {
	if !cloudCredentialRefRe.MatchString(ref) {
		return fmt.Errorf("credential ref must be exactly ${VAR_NAME}, got %d chars", len(ref))
	}
	return nil
}

// ResolveCloudCredentialRef 把 `${VAR}` 引用解析为凭据值（经环境变量，
// vault 注入面与 PERSONA_CONNECT_TOKEN 同款纪律）。变量未设置时报错，
// 不返回空串静默放行。
func ResolveCloudCredentialRef(ref string) (string, error) {
	m := cloudCredentialRefRe.FindStringSubmatch(ref)
	if m == nil {
		return "", fmt.Errorf("invalid credential ref form")
	}
	v := os.Getenv(m[1])
	if v == "" {
		return "", fmt.Errorf("credential env %s not set", m[1])
	}
	return v, nil
}

// CloudAccount 云账号（表 cloud_accounts）。
type CloudAccount struct {
	ID            string    `gorm:"primaryKey;size:32" json:"id"`
	Vendor        string    `gorm:"size:32;uniqueIndex:idx_cloud_acct_vendor_alias" json:"vendor"`
	Alias         string    `gorm:"size:64;uniqueIndex:idx_cloud_acct_vendor_alias" json:"alias"`
	CredentialRef string    `gorm:"size:128" json:"credentialRef"` // 仅 ${VAR} 引用，ValidateCloudCredentialRef 把关
	Scopes        []string  `gorm:"serializer:json" json:"scopes"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// CloudAccountInput 账号写入入参。
type CloudAccountInput struct {
	Vendor        string
	Alias         string
	CredentialRef string
	Scopes        []string
	Enabled       bool
}

// CloudInstance 云实例（表 cloud_instances；领域模型 internal/cloud.Instance
// 的持久化形态，不动既有 ComputeInstance）。
type CloudInstance struct {
	ID               string    `gorm:"primaryKey;size:32" json:"id"`
	AccountID        string    `gorm:"size:32;uniqueIndex:idx_cloud_inst_acct_native" json:"accountId"`
	NativeID         string    `gorm:"size:128;uniqueIndex:idx_cloud_inst_acct_native" json:"nativeId"`
	Name             string    `gorm:"size:128" json:"name"`
	Region           string    `gorm:"size:64" json:"region"`
	Zone             string    `gorm:"size:64" json:"zone,omitempty"`
	Spec             string    `gorm:"size:64" json:"spec,omitempty"`
	Status           string    `gorm:"size:32" json:"status"`
	BillingMode      string    `gorm:"size:32" json:"billingMode"`
	BillingContinues bool      `json:"billingContinues"` // 资源状态≠计费状态，显式字段不做推断
	RawVendorStatus  string    `gorm:"size:128" json:"rawVendorStatus,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// CloudInstanceInput 实例写入入参。
type CloudInstanceInput struct {
	AccountID        string
	NativeID         string
	Name             string
	Region           string
	Zone             string
	Spec             string
	Status           string
	BillingMode      string
	BillingContinues bool
	RawVendorStatus  string
}

// UpsertCloudAccount 保存/更新云账号（同 vendor+alias 视为更新）。
func (d *DB) UpsertCloudAccount(in *CloudAccountInput) error {
	if in.Vendor == "" || in.Alias == "" {
		return errors.New("vendor and alias required")
	}
	if err := ValidateCloudCredentialRef(in.CredentialRef); err != nil {
		return err
	}

	var existing CloudAccount
	err := d.db.Where("vendor = ? AND alias = ?", in.Vendor, in.Alias).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		id, err := newRemoteCredentialID()
		if err != nil {
			return err
		}
		rec := CloudAccount{
			ID: id, Vendor: in.Vendor, Alias: in.Alias,
			CredentialRef: in.CredentialRef, Scopes: in.Scopes,
			Enabled: true, // 新建默认启用；更新走 in.Enabled
		}
		return d.db.Create(&rec).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"credential_ref": in.CredentialRef,
		"scopes":         in.Scopes,
		"enabled":        in.Enabled,
	}
	return d.db.Model(&existing).Updates(updates).Error
}

// ListCloudAccounts 返回全部云账号。
func (d *DB) ListCloudAccounts() ([]CloudAccount, error) {
	var out []CloudAccount
	err := d.db.Order("created_at").Find(&out).Error
	return out, err
}

// DeleteCloudAccount 删除云账号；不存在时返回 ErrNotFound。
func (d *DB) DeleteCloudAccount(id string) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", id).Delete(&CloudInstance{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&CloudAccount{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// UpsertCloudInstance 保存/更新云实例（同账号+原生 ID 视为更新）。
func (d *DB) UpsertCloudInstance(in *CloudInstanceInput) error {
	if in.AccountID == "" || in.NativeID == "" {
		return errors.New("account_id and native_id required")
	}

	var existing CloudInstance
	err := d.db.Where("account_id = ? AND native_id = ?", in.AccountID, in.NativeID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		id, err := newRemoteCredentialID()
		if err != nil {
			return err
		}
		rec := CloudInstance{
			ID: id, AccountID: in.AccountID, NativeID: in.NativeID,
			Name: in.Name, Region: in.Region, Zone: in.Zone, Spec: in.Spec,
			Status: in.Status, BillingMode: in.BillingMode,
			BillingContinues: in.BillingContinues, RawVendorStatus: in.RawVendorStatus,
		}
		return d.db.Create(&rec).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"name":              in.Name,
		"region":            in.Region,
		"zone":              in.Zone,
		"spec":              in.Spec,
		"status":            in.Status,
		"billing_mode":      in.BillingMode,
		"billing_continues": in.BillingContinues,
		"raw_vendor_status": in.RawVendorStatus,
	}
	return d.db.Model(&existing).Updates(updates).Error
}

// ListCloudInstances 返回某账号下的实例（按账号硬过滤，不跨账号）。
func (d *DB) ListCloudInstances(accountID string) ([]CloudInstance, error) {
	var out []CloudInstance
	err := d.db.Where("account_id = ?", accountID).Order("created_at").Find(&out).Error
	return out, err
}

// DeleteCloudInstance 删除云实例；不存在时返回 ErrNotFound。
func (d *DB) DeleteCloudInstance(id string) error {
	res := d.db.Delete(&CloudInstance{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
