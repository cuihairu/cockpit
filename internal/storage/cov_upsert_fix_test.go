package storage

// cov_upsert_fix_test.go UpsertAgent/UpsertDomain 显式更新语义验证：
// GORM Assign(struct) 对已存在记录不落库的存量坑修复后，二次 upsert
// 非零字段必须真的写、零值字段必须保留库内原值。

import (
	"testing"
)

func TestUpsertAgentUpdatesExisting(t *testing.T) {
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.UpsertAgent(&Agent{
		ID: "a1", Hostname: "h1", IP: "203.0.113.10", Status: "online",
		SecretHash: "hash-v1",
		Labels: map[string]interface{}{"env": "prod"},
		Capabilities: []Capability{{Type: "docker", Version: "1.0"}},
	}); err != nil {
		t.Fatal(err)
	}
	first, _ := db.GetAgent("a1")

	// 二次 upsert：换 IP/hostname + 换 capabilities，不带 SecretHash/Labels
	if err := db.UpsertAgent(&Agent{
		ID: "a1", Hostname: "h1-new", IP: "203.0.113.99", Status: "online",
		Capabilities: []Capability{{Type: "nas", Version: "2.0"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetAgent("a1")
	if got.IP != "203.0.113.99" || got.Hostname != "h1-new" {
		t.Errorf("updated fields lost: ip=%q hostname=%q", got.IP, got.Hostname)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Type != "nas" {
		t.Errorf("capabilities should be updated (serializer), got %+v", got.Capabilities)
	}
	if got.SecretHash != "hash-v1" {
		t.Errorf("SecretHash should be preserved, got %q", got.SecretHash)
	}
	if !got.FirstSeen.Equal(first.FirstSeen) {
		t.Errorf("FirstSeen should be preserved, got %v want %v", got.FirstSeen, first.FirstSeen)
	}
	if got.Labels["env"] != "prod" {
		t.Errorf("Labels should be preserved (serializer), got %v", got.Labels)
	}

	// 全零字段二次 upsert：空更新短路，不报错不动数据
	if err := db.UpsertAgent(&Agent{ID: "a1"}); err != nil {
		t.Fatalf("zero-field upsert should be no-op, got %v", err)
	}
	got, _ = db.GetAgent("a1")
	if got.IP != "203.0.113.99" {
		t.Errorf("zero-field upsert should not touch data, ip=%q", got.IP)
	}

	// SecretHash 显式提供时才写（重生成密钥路径）
	if err := db.UpsertAgent(&Agent{ID: "a1", SecretHash: "hash-v2"}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetAgent("a1")
	if got.SecretHash != "hash-v2" {
		t.Errorf("explicit SecretHash should be written, got %q", got.SecretHash)
	}
}

func TestUpsertDomainUpdatesExisting(t *testing.T) {
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	aid := "a1"
	if err := db.UpsertDomain(&Domain{
		ID: "blog.example.com", Domain: "blog.example.com",
		Provider: "cloudflare", Status: "pending", AgentID: &aid,
	}); err != nil {
		t.Fatal(err)
	}

	// 二次 upsert：改 provider；AgentID 为 nil 不应清掉归属
	provider2 := "dnsdpod"
	if err := db.UpsertDomain(&Domain{
		ID: "blog.example.com", Provider: provider2, Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetDomainByName("blog.example.com")
	if got.Provider != provider2 || got.Status != "active" {
		t.Errorf("updated fields lost: %+v", got)
	}
	if got.AgentID == nil || *got.AgentID != "a1" {
		t.Errorf("nil AgentID should preserve binding, got %v", got.AgentID)
	}

	// 非零 AgentID 更新归属（inventory sync 场景）
	aid2 := "a2"
	if err := db.UpsertDomain(&Domain{ID: "blog.example.com", AgentID: &aid2}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetDomainByName("blog.example.com")
	if got.AgentID == nil || *got.AgentID != "a2" {
		t.Errorf("agent rebind via upsert failed: %v", got.AgentID)
	}

	// 全零字段：短路 no-op
	if err := db.UpsertDomain(&Domain{ID: "blog.example.com"}); err != nil {
		t.Fatalf("zero-field upsert should be no-op, got %v", err)
	}
	got, _ = db.GetDomainByName("blog.example.com")
	if got.Provider != provider2 {
		t.Errorf("zero-field upsert should not touch data, provider=%q", got.Provider)
	}
}
