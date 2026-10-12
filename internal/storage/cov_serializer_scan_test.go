package storage

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// 序列化器 Scan/Value 的驱动类型分支直测：sqlite 驱动只产 string，nil /
// []byte / 异型分支经 schema.Field 直接调用覆盖（capability_crypto.go /
// acme_crypto.go 同批）。

func mustField(t *testing.T, model interface{}, fieldName string) (*schema.Field, reflect.Value) {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema parse: %v", err)
	}
	f := s.LookUpField(fieldName)
	if f == nil {
		t.Fatalf("field %s not found", fieldName)
	}
	return f, reflect.New(s.ModelType).Elem()
}

func TestAcmePrivKeySerializerScanBranches(t *testing.T) {
	f, dst := mustField(t, &AcmeAccount{}, "private_key_pem")
	ser := acmePrivKeySerializer{}
	ctx := context.Background()

	// nil（NULL 列）→ 无动作
	if err := ser.Scan(ctx, f, dst, nil); err != nil {
		t.Fatalf("nil scan: %v", err)
	}
	if dst.FieldByName("PrivateKeyPEM").String() != "" {
		t.Fatal("nil column must leave field empty")
	}

	// []byte 载荷（BLOB 读取形态）→ 解密
	ct, err := vaultEncrypt(acmeTestAccountPEM)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := ser.Scan(ctx, f, dst, []byte(ct)); err != nil {
		t.Fatalf("[]byte scan: %v", err)
	}
	if got := dst.FieldByName("PrivateKeyPEM").String(); got != acmeTestAccountPEM {
		t.Fatalf("[]byte scan decrypted = %.10q", got)
	}

	// 异型值 → 明确报错
	if err := ser.Scan(ctx, f, dst, 42); err == nil || !strings.Contains(err.Error(), "unexpected private key column type") {
		t.Fatalf("unexpected type err = %v", err)
	}
}

func TestCapabilitiesSerializerScanBranches(t *testing.T) {
	f, dst := mustField(t, &Agent{}, "capabilities")
	ser := capabilitiesSerializer{}
	ctx := context.Background()

	// 空 raw → 无动作
	if err := ser.Scan(ctx, f, dst, ""); err != nil {
		t.Fatalf("empty raw scan: %v", err)
	}

	// []byte：密文 config_enc 条目解回明文 Config
	ct, err := vaultEncrypt(`{"k":"cap-scan-secret-2w"}`)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	payload, err := json.Marshal([]Capability{{Type: "pve", ConfigEnc: ct}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := ser.Scan(ctx, f, dst, []byte(payload)); err != nil {
		t.Fatalf("[]byte scan: %v", err)
	}
	caps := dst.FieldByName("Capabilities").Interface().([]Capability)
	if len(caps) != 1 || caps[0].Config["k"] != "cap-scan-secret-2w" || caps[0].ConfigEnc != "" {
		t.Fatalf("[]byte scan caps = %+v", caps)
	}

	// 非法 JSON → 报错
	if err := ser.Scan(ctx, f, dst, []byte("not-json")); err == nil {
		t.Fatal("invalid json must error")
	}

	// 异型值 → 报错
	if err := ser.Scan(ctx, f, dst, 3.14); err == nil || !strings.Contains(err.Error(), "unexpected column type") {
		t.Fatalf("unexpected type err = %v", err)
	}
}

// 加密辅助的 Config 序列化失败与解密回的 JSON 形状失败。
func TestCapabilityConfigCodecFailureBranches(t *testing.T) {
	// Config 含不可序列化值 → 加密路径上抛
	_, err := encryptCapabilityConfigs([]Capability{{
		Type: "pve", Config: map[string]interface{}{"bad": func() {}},
	}})
	if err == nil || !strings.Contains(err.Error(), "capability pve config") {
		t.Fatalf("marshal failure err = %v", err)
	}

	// 解密出的明文不是 JSON → 上抛
	ct, err := vaultEncrypt("not-json-at-all")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := decryptCapabilityConfigs([]Capability{{Type: "pve", ConfigEnc: ct}}); err == nil ||
		!strings.Contains(err.Error(), "capability pve config") {
		t.Fatalf("unmarshal failure err = %v", err)
	}
}

// Value 的加密失败透传（序列化器写路径）。
func TestCapabilitiesSerializerValueEncryptFailure(t *testing.T) {
	f, dst := mustField(t, &Agent{}, "capabilities")
	orig := vaultEncrypt
	vaultEncrypt = func(string) (string, error) { return "", errors.New("boom ser encrypt") }
	t.Cleanup(func() { vaultEncrypt = orig })

	if _, err := (capabilitiesSerializer{}).Value(context.Background(), f, dst, []Capability{
		{Type: "pve", Config: map[string]interface{}{"k": "v"}},
	}); err == nil {
		t.Fatal("Value must propagate encrypt failure")
	}
}
