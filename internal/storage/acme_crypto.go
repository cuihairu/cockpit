package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm/schema"
)

// acmePrivKeySerializer ACME 私钥列的落库加密序列化器（vault-design.md V6
// 第二波②，决策 D-2026-10-12-1）：写即 vault.Encrypt（v1 密文），读兼容
// 存量明文 PEM 行（历史列从未加密，v0 现实）。错误消息不带任何明文。
// 证书/签发者 PEM 是公开物料，不加密。
type acmePrivKeySerializer struct{}

// vaultCiphertextV1 vault core 的版本前缀（internal/vault crypto.go）；
// storage 侧只做前缀识别，编解码全走 vault.Encrypt/Decrypt。
const vaultCiphertextV1 = "v1:"

// legacyPEMPrefix 明文 PEM 行识别前缀（v0 兼容：历史列就是明文 PEM）
const legacyPEMPrefix = "-----BEGIN"

func (acmePrivKeySerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var raw string
	switch v := dbValue.(type) {
	case nil:
		return nil
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("acme: unexpected private key column type %T", dbValue)
	}
	if raw == "" {
		return nil
	}
	plain := raw
	switch {
	case strings.HasPrefix(raw, vaultCiphertextV1):
		dec, err := vaultDecrypt(raw)
		if err != nil {
			return err
		}
		plain = dec
	case strings.HasPrefix(raw, legacyPEMPrefix):
		// 存量明文行原样读出（v0 兼容，下次保存起转为密文）
	default:
		return errors.New("acme: private key column neither PEM nor encrypted")
	}
	return field.Set(ctx, dst, plain)
}

func (acmePrivKeySerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	plain, ok := fieldValue.(string)
	if !ok || plain == "" {
		return "", nil
	}
	return vaultEncrypt(plain)
}

func init() {
	schema.RegisterSerializer("acmeprivkey", acmePrivKeySerializer{})
}
