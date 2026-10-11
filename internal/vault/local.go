package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// LocalProvider 默认本地加密实现（V4）：sqlite vault_entries 表 + 收敛
// crypto core，自托管零外部依赖。密钥来自 env 来源链（V5，进程级）；
// Lock/Unlock 管会话门（密钥擦除随主口令模式立项，见 V3/V4）。
type LocalProvider struct {
	db    *gorm.DB
	sess  *Session
	audit AuditFunc
}

// VaultEntry vault_entries 行：username/value 只存密文，meta 为非敏感 JSON。
type VaultEntry struct {
	Path        string    `gorm:"primaryKey;size:255" json:"path"`
	Kind        string    `gorm:"size:32" json:"kind"`
	UsernameEnc string    `gorm:"size:4096" json:"-"`
	ValueEnc    string    `gorm:"size:12288" json:"-"`
	Meta        string    `gorm:"size:2048" json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 表名与既有 GORM 命名一致（复数）。
func (VaultEntry) TableName() string { return "vault_entries" }

// NewLocal 构建本地 provider 并迁移 vault_entries 表（V4）。
func NewLocal(db *gorm.DB, ttl time.Duration, audit AuditFunc) (*LocalProvider, error) {
	if db == nil {
		return nil, errors.New("vault: nil db")
	}
	if err := db.AutoMigrate(&VaultEntry{}); err != nil {
		return nil, fmt.Errorf("vault: migrate: %w", err)
	}
	p := &LocalProvider{db: db, sess: NewSession(ttl), audit: audit}
	// 进程级密钥已由 init 依 env 来源链装载（V5）；默认随进程解锁，
	// 与存量 storage/crypto.go 隐式语义一致。
	p.sess.Unlock()
	return p, nil
}

// Name provider 名（V7 配置选择用）。
func (p *LocalProvider) Name() string { return "local" }

// Unlock 解锁/重验口令（V2）：已解锁时口令必须与当前密钥同源（防误切
// 密钥）；未解锁时依口令重派生并开门。
func (p *LocalProvider) Unlock(passphrase string) error {
	if passphrase == "" {
		return errors.New("vault: empty passphrase")
	}
	newKey := DeriveKey(passphrase)
	if p.sess.Check() == nil {
		if len(newKey) != len(encryptionKey) || string(newKey) != string(encryptionKey) {
			return errors.New("vault: passphrase mismatch")
		}
		p.sess.Unlock()
		return nil
	}
	ApplyKey(passphrase)
	p.sess.Unlock()
	if p.audit != nil {
		p.audit("vault.unlock", "", true)
	}
	return nil
}

// Lock 关会话门（V3：密钥保留在进程内，见 LocalProvider 文注）。
func (p *LocalProvider) Lock() {
	p.sess.Lock()
	if p.audit != nil {
		p.audit("vault.lock", "", true)
	}
}

// Write 写入/更新条目（同 path 覆盖）。空 Username/Value 语义 = 不保管该字段。
func (p *LocalProvider) Write(path string, e *Entry) error {
	if path == "" {
		return errors.New("vault: path required")
	}
	if e == nil {
		return errors.New("vault: nil entry")
	}
	if err := p.sess.Check(); err != nil {
		return err
	}
	row, err := encodeEntry(e)
	if err != nil {
		return err
	}
	row.Path = path
	row.UpdatedAt = time.Now()
	var count int64
	if err := p.db.Model(&VaultEntry{}).Where("path = ?", path).Count(&count).Error; err != nil {
		p.auditFail("vault.write", path)
		return fmt.Errorf("vault: write %s: %w", path, err)
	}
	if count > 0 {
		err = p.db.Model(&VaultEntry{}).Where("path = ?", path).Updates(map[string]any{
			"kind": row.Kind, "username_enc": row.UsernameEnc,
			"value_enc": row.ValueEnc, "meta": row.Meta, "updated_at": row.UpdatedAt,
		}).Error
	} else {
		row.CreatedAt = row.UpdatedAt
		err = p.db.Create(row).Error
	}
	if err != nil {
		p.auditFail("vault.write", path)
		return fmt.Errorf("vault: write %s: %w", path, err)
	}
	p.auditOK("vault.write", path)
	return nil
}

// Read 按 path 取条目（唯一明文出口，供调用方立即使用）。
func (p *LocalProvider) Read(path string) (*Entry, error) {
	if err := p.sess.Check(); err != nil {
		return nil, err
	}
	var row VaultEntry
	if err := p.db.First(&row, "path = ?", path).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p.auditFail("vault.read", path)
			return nil, ErrNotFound
		}
		p.auditFail("vault.read", path)
		return nil, fmt.Errorf("vault: read %s: %w", path, err)
	}
	e, err := decodeEntry(&row)
	if err != nil {
		p.auditFail("vault.read", path)
		return nil, err
	}
	p.auditOK("vault.read", path)
	return e, nil
}

// List 按 path 前缀列元数据——明文永不出库（V2）。
func (p *LocalProvider) List(prefix string) ([]EntryMeta, error) {
	if err := p.sess.Check(); err != nil {
		return nil, err
	}
	var rows []VaultEntry
	q := p.db.Order("path")
	if prefix != "" {
		q = q.Where("path LIKE ?", prefix+"%")
	}
	if err := q.Find(&rows).Error; err != nil {
		p.auditFail("vault.list", prefix)
		return nil, fmt.Errorf("vault: list %s: %w", prefix, err)
	}
	metas := make([]EntryMeta, 0, len(rows))
	for i := range rows {
		metas = append(metas, EntryMeta{
			Path:        rows[i].Path,
			Kind:        rows[i].Kind,
			HasUsername: rows[i].UsernameEnc != "",
			HasValue:    rows[i].ValueEnc != "",
			UpdatedAt:   rows[i].UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	p.auditOK("vault.list", prefix)
	return metas, nil
}

// Delete 删除条目；不存在返回 ErrNotFound。
func (p *LocalProvider) Delete(path string) error {
	if err := p.sess.Check(); err != nil {
		return err
	}
	res := p.db.Delete(&VaultEntry{}, "path = ?", path)
	if res.Error != nil {
		p.auditFail("vault.delete", path)
		return fmt.Errorf("vault: delete %s: %w", path, res.Error)
	}
	if res.RowsAffected == 0 {
		p.auditFail("vault.delete", path)
		return ErrNotFound
	}
	p.auditOK("vault.delete", path)
	return nil
}

func (p *LocalProvider) auditOK(action, path string) {
	if p.audit != nil {
		p.audit(action, path, true)
	}
}

func (p *LocalProvider) auditFail(action, path string) {
	if p.audit != nil {
		p.audit(action, path, false)
	}
}

// encodeEntry 加密落库形态（空串直存 = 不保管，同 remote_credential 口径）。
func encodeEntry(e *Entry) (*VaultEntry, error) {
	row := &VaultEntry{Kind: e.Kind}
	if e.Username != "" {
		enc, err := Encrypt(e.Username)
		if err != nil {
			return nil, fmt.Errorf("vault: encrypt username: %w", err)
		}
		row.UsernameEnc = enc
	}
	if e.Value != "" {
		enc, err := Encrypt(e.Value)
		if err != nil {
			return nil, fmt.Errorf("vault: encrypt value: %w", err)
		}
		row.ValueEnc = enc
	}
	if len(e.Meta) > 0 {
		b, err := json.Marshal(e.Meta)
		if err != nil {
			return nil, fmt.Errorf("vault: encode meta: %w", err)
		}
		row.Meta = string(b)
	}
	return row, nil
}

// decodeEntry 解密出内存态条目；密文损坏只报类别不报明文（红线）。
func decodeEntry(row *VaultEntry) (*Entry, error) {
	e := &Entry{Path: row.Path, Kind: row.Kind}
	if row.UsernameEnc != "" {
		plain, err := Decrypt(row.UsernameEnc)
		if err != nil {
			return nil, fmt.Errorf("vault: decrypt username: %w", err)
		}
		e.Username = plain
	}
	if row.ValueEnc != "" {
		plain, err := Decrypt(row.ValueEnc)
		if err != nil {
			return nil, fmt.Errorf("vault: decrypt value: %w", err)
		}
		e.Value = plain
	}
	if row.Meta != "" {
		if err := json.Unmarshal([]byte(row.Meta), &e.Meta); err != nil {
			return nil, fmt.Errorf("vault: decode meta: %w", err)
		}
	}
	return e, nil
}
