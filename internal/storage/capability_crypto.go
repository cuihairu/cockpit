package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"gorm.io/gorm/schema"
)

// capabilitiesSerializer agents.capabilities 列的自定义序列化器
// （vault-design.md V6 第二波③，决策 D-2026-10-12-1）：Capability.Config
// 是 inventory 声明的 capability 配置，历史上明文随 JSON 落库（盘点表已知
// 欠账）。写路径把 Config 序列化后以 v1 密文放 config_enc（Config 置空不再
// 出现明文键）；读路径解密回内存 Config，存量明文 config 键原样兼容。
// Config 当前零消费方，JSON 响应形状不变（config 键仍在，明文只见于内存）。
type capabilitiesSerializer struct{}

// encryptCapabilityConfigs 返回加密后的副本（不改入参——内存态 agent 的
// Config 保持明文）。Config 为空的条目原样复制。
func encryptCapabilityConfigs(caps []Capability) ([]Capability, error) {
	if len(caps) == 0 {
		return caps, nil
	}
	out := make([]Capability, len(caps))
	copy(out, caps)
	for i := range out {
		if len(out[i].Config) == 0 {
			continue
		}
		b, err := json.Marshal(out[i].Config)
		if err != nil {
			return nil, fmt.Errorf("capability %s config: %w", out[i].Type, err)
		}
		ct, err := vaultEncrypt(string(b))
		if err != nil {
			return nil, fmt.Errorf("capability %s config: %w", out[i].Type, err)
		}
		out[i].Config = nil
		out[i].ConfigEnc = ct
	}
	return out, nil
}

// decryptCapabilityConfigs 原地把 config_enc 解回 Config（存量明文 config
// 键不动）。解密失败上抛（拒绝服务，不降级为无配置）。
func decryptCapabilityConfigs(caps []Capability) error {
	for i := range caps {
		if caps[i].ConfigEnc == "" {
			continue
		}
		plain, err := vaultDecrypt(caps[i].ConfigEnc)
		if err != nil {
			return fmt.Errorf("capability %s config: %w", caps[i].Type, err)
		}
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
			return fmt.Errorf("capability %s config: %w", caps[i].Type, err)
		}
		caps[i].Config = cfg
		caps[i].ConfigEnc = ""
	}
	return nil
}

func (capabilitiesSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var raw string
	switch v := dbValue.(type) {
	case nil:
		return nil
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("capabilities: unexpected column type %T", dbValue)
	}
	if raw == "" {
		return nil
	}
	var caps []Capability
	if err := json.Unmarshal([]byte(raw), &caps); err != nil {
		return err
	}
	if err := decryptCapabilityConfigs(caps); err != nil {
		return err
	}
	return field.Set(ctx, dst, caps)
}

func (capabilitiesSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	caps, _ := fieldValue.([]Capability)
	enc, err := encryptCapabilityConfigs(caps)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(enc)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func init() {
	schema.RegisterSerializer("capconf", capabilitiesSerializer{})
}
