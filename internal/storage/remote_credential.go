package storage

import (
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// newRemoteCredentialID 生成凭据记录 ID（16 字节 hex）。复用 password.go 的
// randRead 注入点。
func newRemoteCredentialID() (string, error) {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RemoteCredentialInput 凭据明文入参。仅在存储层内转密文落库，不持久化明文。
type RemoteCredentialInput struct {
	UserID     string
	AgentID    string
	Host       string
	Port       int
	Protocol   string // ssh / rdp / vnc
	Username   string
	Domain     string
	Password   string
	PrivateKey string
}

// RemoteCredentialPlain 解密后的连接凭据。仅供 handleTicketCreate 的
// use_saved 注入路径使用，不进任何日志/审计/HTTP 响应。
type RemoteCredentialPlain struct {
	Username   string
	Domain     string
	Password   string
	PrivateKey string
}

// RemoteCredentialMeta 凭据元数据（列表 API 的唯一出参形态——明文永不出库）。
type RemoteCredentialMeta struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agentId"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Protocol     string    `json:"protocol"`
	Username     string    `json:"username"`
	Domain       string    `json:"domain,omitempty"`
	HasPassword  bool      `json:"hasPassword"`
	HasPrivKey   bool      `json:"hasPrivateKey"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// UpsertRemoteCredential 保存/更新凭据（同用户同目标视为更新）。
// 空口令/空私钥存空串，表示「该字段不保管」。
func (d *DB) UpsertRemoteCredential(in *RemoteCredentialInput) error {
	pwEnc, err := encryptOptional(in.Password)
	if err != nil {
		return err
	}
	keyEnc, err := encryptOptional(in.PrivateKey)
	if err != nil {
		return err
	}

	var existing RemoteCredential
	err = d.db.Where(
		"user_id = ? AND agent_id = ? AND host = ? AND port = ? AND protocol = ?",
		in.UserID, in.AgentID, in.Host, in.Port, in.Protocol,
	).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		id, err := newRemoteCredentialID()
		if err != nil {
			return err
		}
		return d.db.Create(&RemoteCredential{
			ID:            id,
			UserID:        in.UserID,
			AgentID:       in.AgentID,
			Host:          in.Host,
			Port:          in.Port,
			Protocol:      in.Protocol,
			Username:      in.Username,
			Domain:        in.Domain,
			PasswordEnc:   pwEnc,
			PrivateKeyEnc: keyEnc,
		}).Error
	}
	if err != nil {
		return err
	}

	return d.db.Model(&existing).Updates(map[string]interface{}{
		"username":        in.Username,
		"domain":          in.Domain,
		"password_enc":    pwEnc,
		"private_key_enc": keyEnc,
	}).Error
}

// GetRemoteCredential 按 (用户, 目标) 取解密后的凭据。没有记录返回 ErrNotFound。
func (d *DB) GetRemoteCredential(userID, agentID, host, protocol string, port int) (*RemoteCredentialPlain, error) {
	var row RemoteCredential
	err := d.db.Where(
		"user_id = ? AND agent_id = ? AND host = ? AND port = ? AND protocol = ?",
		userID, agentID, host, port, protocol,
	).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decryptRemoteCredential(&row)
}

// ListRemoteCredentials 当前用户的凭据元数据列表（更新时间倒序）。
func (d *DB) ListRemoteCredentials(userID string) ([]RemoteCredentialMeta, error) {
	var rows []RemoteCredential
	if err := d.db.Where("user_id = ?", userID).
		Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	metas := make([]RemoteCredentialMeta, 0, len(rows))
	for i := range rows {
		metas = append(metas, remoteCredentialMeta(&rows[i]))
	}
	return metas, nil
}

// DeleteRemoteCredential 删除当前用户的一条凭据。返回是否确实删除了记录
// （false = 记录不存在或不属于该用户，不区分二者以免探测）。
func (d *DB) DeleteRemoteCredential(userID, id string) (bool, error) {
	res := d.db.Where("user_id = ? AND id = ?", userID, id).Delete(&RemoteCredential{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// encryptOptional 空串不加密直存（语义：不保管该字段），非空走 AES-GCM。
func encryptOptional(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return Encrypt(plain)
}

func decryptRemoteCredential(row *RemoteCredential) (*RemoteCredentialPlain, error) {
	plain := &RemoteCredentialPlain{
		Username: row.Username,
		Domain:   row.Domain,
	}
	var err error
	if row.PasswordEnc != "" {
		if plain.Password, err = Decrypt(row.PasswordEnc); err != nil {
			return nil, err
		}
	}
	if row.PrivateKeyEnc != "" {
		if plain.PrivateKey, err = Decrypt(row.PrivateKeyEnc); err != nil {
			return nil, err
		}
	}
	return plain, nil
}

func remoteCredentialMeta(row *RemoteCredential) RemoteCredentialMeta {
	return RemoteCredentialMeta{
		ID:          row.ID,
		AgentID:     row.AgentID,
		Host:        row.Host,
		Port:        row.Port,
		Protocol:    row.Protocol,
		Username:    row.Username,
		Domain:      row.Domain,
		HasPassword: row.PasswordEnc != "",
		HasPrivKey:  row.PrivateKeyEnc != "",
		UpdatedAt:   row.UpdatedAt,
	}
}
