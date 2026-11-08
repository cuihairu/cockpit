package storage

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/cuihairu/cockpit/internal/cloud"

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
	CredentialRef string    `gorm:"size:128" json:"credentialRef"`   // 仅 ${VAR} 引用，ValidateCloudCredentialRef 把关
	Region        string    `gorm:"size:64" json:"region,omitempty"` // 适配器查询/创建缺省地域（云 API 按地域级查询）
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
	Region        string
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

// CloudOperationRecord 持久化的云操作（表 cloud_operations；领域模型
// internal/cloud.Operation 的持久化形态）。云操作只保存在进程内存会导致
// 服务重启后已受理操作丢失（审核发现 :273），改为落库以便重启后继续跟踪
// 云端仍在执行的创建/释放。
type CloudOperationRecord struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	AccountID       string    `gorm:"size:32;index:idx_cloud_op_acct_id" json:"accountId"`
	Kind            string    `gorm:"size:16" json:"kind"`
	NativeID        string    `gorm:"size:128" json:"nativeId,omitempty"`
	VendorRequestID string    `gorm:"size:128" json:"vendorRequestId"`
	Status          string    `gorm:"size:16" json:"status"`
	Error           string    `gorm:"size:512" json:"error,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// 复合唯一索引：同一账号下操作 ID 唯一，不同账号允许相同操作 ID。
func (CloudOperationRecord) TableName() string { return "cloud_operations" }

// CloudOperationInput 操作写入入参。
type CloudOperationInput struct {
	AccountID       string
	Kind            string
	NativeID        string
	VendorRequestID string
	Status          string
	Error           string
	UpdatedAt       time.Time
}

// UpsertCloudOperation 保存/更新云操作（同账号+操作 ID 视为更新）。
func (d *DB) UpsertCloudOperation(in *CloudOperationInput) error {
	if in.AccountID == "" || in.Kind == "" || in.VendorRequestID == "" {
		return errors.New("account_id, kind and vendor_request_id required")
	}
	rec := CloudOperationRecord{
		ID:        cloud.OperationID(in.Kind, in.VendorRequestID),
		AccountID: in.AccountID, Kind: in.Kind, NativeID: in.NativeID,
		VendorRequestID: in.VendorRequestID, Status: in.Status, Error: in.Error,
		UpdatedAt: in.UpdatedAt,
	}
	return d.db.Save(&rec).Error
}

// ListCloudOperations 返回某账号下的全部操作（按账号硬过滤，不跨账号）。
func (d *DB) ListCloudOperations(accountID string) ([]CloudOperationRecord, error) {
	var out []CloudOperationRecord
	err := d.db.Where("account_id = ?", accountID).Order("updated_at").Find(&out).Error
	return out, err
}

// GetCloudOperation 按账号+操作 ID 取单操作；不存在返回 ErrNotFound。
func (d *DB) GetCloudOperation(accountID, operationID string) (CloudOperationRecord, error) {
	var out CloudOperationRecord
	err := d.db.Where("account_id = ? AND id = ?", accountID, operationID).First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, ErrNotFound
	}
	return out, err
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
			Region: in.Region, Enabled: true, // 新建默认启用；更新走 in.Enabled
		}
		return d.db.Create(&rec).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"credential_ref": in.CredentialRef,
		"region":         in.Region,
		"scopes":         in.Scopes,
		"enabled":        in.Enabled,
	}
	return d.db.Model(&existing).Updates(updates).Error
}

// GetCloudAccount 按 ID 取单账号；不存在返回 ErrNotFound。
func (d *DB) GetCloudAccount(id string) (CloudAccount, error) {
	var out CloudAccount
	err := d.db.First(&out, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, ErrNotFound
	}
	return out, err
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

// GetCloudInstance 按 ID 取单实例；不存在返回 ErrNotFound。
func (d *DB) GetCloudInstance(id string) (CloudInstance, error) {
	var out CloudInstance
	err := d.db.First(&out, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, ErrNotFound
	}
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
