package storage

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const capSecretToken = "pve-token-secret-9q"

// Capability.Config 落库加密（vault-design.md V6 第二波③）：Update map 路径
// 与 Create model 路径（capconf 序列化器）都落 config_enc 密文，读回内存
// 明文 Config、config_enc 清空；存量明文 config 键行原样兼容。
func TestAgentCapabilityConfigEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	agent := &Agent{
		ID:       "a-cap-1",
		Hostname: "h1",
		Capabilities: []Capability{{
			Type: "pve", Version: "1",
			Config: map[string]interface{}{"url": "https://pve:8006", "token_secret": capSecretToken},
		}},
	}
	// Update map 路径（先建再更，命中 agentUpdateFields 分支）
	if err := db.UpsertAgent(&Agent{ID: "a-cap-1", Hostname: "h1"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.UpsertAgent(agent); err != nil {
		t.Fatalf("update agent: %v", err)
	}
	assertCapsEncryptedAtRest(t, db, "a-cap-1")

	got, err := db.GetAgent("a-cap-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].ConfigEnc != "" {
		t.Fatalf("in-memory caps = %+v", got.Capabilities)
	}
	if got.Capabilities[0].Config["token_secret"] != capSecretToken {
		t.Fatalf("config not decrypted back: %+v", got.Capabilities[0].Config)
	}
	// 入参不被改写：内存态 agent 的 Config 保持明文、无 ConfigEnc
	if agent.Capabilities[0].Config["token_secret"] != capSecretToken || agent.Capabilities[0].ConfigEnc != "" {
		t.Fatal("UpsertAgent must not mutate caller's capabilities")
	}

	// Create model 路径（capconf 序列化器）
	agent2 := &Agent{
		ID: "a-cap-2", Hostname: "h2",
		Capabilities: []Capability{{Type: "docker", Config: map[string]interface{}{"sock": "/var/run/docker.sock"}}},
	}
	if err := db.UpsertAgent(agent2); err != nil {
		t.Fatalf("create via model path: %v", err)
	}
	var raw2 string
	if err := db.Gorm().Table("agents").Where("id = ?", "a-cap-2").Select("capabilities").Scan(&raw2).Error; err != nil {
		t.Fatalf("raw fetch: %v", err)
	}
	if strings.Contains(raw2, "/var/run/docker.sock") || strings.Contains(raw2, "\"config\"") {
		t.Fatalf("model-path create must not carry plaintext config: %s", raw2)
	}
	got2, err := db.GetAgent("a-cap-2")
	if err != nil || got2.Capabilities[0].Config["sock"] != "/var/run/docker.sock" {
		t.Fatalf("model-path roundtrip: %+v, %v", got2, err)
	}

	// API JSON 形状：config 键在、config_enc 不在（读回后清空）
	b, _ := json.Marshal(got2.Capabilities)
	if strings.Contains(string(b), "config_enc") || !strings.Contains(string(b), "config") {
		t.Fatalf("API shape changed: %s", b)
	}
}

// assertCapsEncryptedAtRest 原始列断言：config_enc 带 v1 前缀、无 config 键与明文。
func assertCapsEncryptedAtRest(t *testing.T, db *DB, agentID string) {
	t.Helper()
	var raw string
	if err := db.Gorm().Table("agents").Where("id = ?", agentID).Select("capabilities").Scan(&raw).Error; err != nil {
		t.Fatalf("raw capabilities fetch: %v", err)
	}
	if strings.Contains(raw, capSecretToken) {
		t.Fatalf("plaintext config at rest: %s", raw)
	}
	if !strings.Contains(raw, `"config_enc":"v1:`) {
		t.Fatalf("config_enc with v1 prefix missing: %s", raw)
	}
	if strings.Contains(raw, `"config":`) {
		t.Fatalf("plaintext config key must be absent: %s", raw)
	}
}

// 存量明文行兼容：config 键原样读回（v0 现实）。
func TestAgentCapabilityConfigLegacyPlaintextCompat(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.Gorm().Exec(
		`INSERT INTO agents (id, hostname, capabilities) VALUES ('a-legacy','hl','[{"type":"pve","config":{"token_secret":"legacy-secret-1"}}]')`).Error; err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	got, err := db.GetAgent("a-legacy")
	if err != nil || got.Capabilities[0].Config["token_secret"] != "legacy-secret-1" {
		t.Fatalf("legacy plaintext read = %+v, %v", got, err)
	}
	// 保存 → 转为密文
	got.Hostname = "hl2"
	if err := db.UpsertAgent(got); err != nil {
		t.Fatalf("resave: %v", err)
	}
	assertCapsEncryptedAtRest(t, db, "a-legacy")
}

// config_enc 损坏 → 读取报错（拒绝服务，不降级为无配置）。
func TestAgentCapabilityConfigCorruptColumn(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.Gorm().Exec(
		`INSERT INTO agents (id, hostname, capabilities) VALUES ('a-corrupt','hc','[{"type":"pve","config_enc":"v1:not-base64!!"}]')`).Error; err != nil {
		t.Fatalf("corrupt insert: %v", err)
	}
	if _, err := db.GetAgent("a-corrupt"); err == nil || !strings.Contains(err.Error(), "capability pve config") {
		t.Fatalf("corrupt config_enc err = %v", err)
	}
}

// 加密失败注入：两条写路径都上抛、不落明文。
func TestAgentCapabilityConfigEncryptFailureInjection(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	orig := vaultEncrypt
	vaultEncrypt = func(string) (string, error) { return "", errors.New("boom cap encrypt") }
	t.Cleanup(func() { vaultEncrypt = orig })

	caps := []Capability{{Type: "pve", Config: map[string]interface{}{"k": capSecretToken}}}
	if _, err := encryptCapabilityConfigs(caps); err == nil {
		t.Fatal("encrypt failure must propagate from helper")
	}
	// 注入期间入参不被改写
	if caps[0].Config["k"] != capSecretToken || caps[0].ConfigEnc != "" {
		t.Fatal("failed encrypt must not mutate input")
	}

	a := &Agent{ID: "a-encfail", Hostname: "h", Capabilities: caps}
	if err := db.UpsertAgent(&Agent{ID: "a-encfail", Hostname: "h"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if err := db.UpsertAgent(a); err == nil {
		t.Fatal("map-path encrypt failure must propagate")
	}
	var raw string
	db.Gorm().Table("agents").Where("id = ?", "a-encfail").Select("capabilities").Scan(&raw)
	if strings.Contains(raw, capSecretToken) {
		t.Fatal("failed write must not leave plaintext at rest")
	}
}
