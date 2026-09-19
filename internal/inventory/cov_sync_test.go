package inventory

import (
	"context"
	"testing"
)

// TestCovSyncAllUpsertsFail 用已关闭的数据库触发全部七类资源的 upsert 错误分支。
func TestCovSyncAllUpsertsFail(t *testing.T) {
	db := testDB(t)
	db.Close() // 此后所有 Get/Upsert 均失败

	inv := testInventory()
	inv.ComputeInstances = map[string]*ComputeInstance{
		"ci1": {Name: "vm1", Type: "vm", Agent: "agent-1"},
	}
	inv.Services = map[string]*Service{
		"svc1": {Name: "web", Type: "http", Region: "r1", Zone: "z1"},
	}
	inv.Gateways = map[string]*Gateway{
		"gw1": {Name: "edge", Type: "openwrt", Agent: "agent-1", Region: "r1"},
	}
	inv.Storages = map[string]*Storage{
		"st1": {Name: "data", Type: "nfs", Path: "/exports", Zone: "z1"},
	}

	s := NewSyncer(db)
	result := s.Sync(context.Background(), inv)
	if result == nil {
		t.Fatal("Sync() should return a result even when all upserts fail")
	}
	for name, r := range map[string]*ResourceResult{
		"agents":   result.Agents,
		"domains":  result.Domains,
		"compute":  result.ComputeInstances,
		"services": result.Services,
		"gateways": result.Gateways,
		"storages": result.Storages,
	} {
		if r == nil || r.Errors == 0 {
			t.Errorf("%s errors = %+v, want Errors > 0 with closed DB", name, r)
		}
	}
}

// TestCovSyncCertificatesDomainMatch 覆证书引用与 domains 匹配的分支。
func TestCovSyncCertificatesDomainMatch(t *testing.T) {
	db := testDB(t)
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"d1": {Domain: "matched.example.com"},
		},
		Regions: map[string]*Region{},
	}
	inv.Domains["d1"].Certificates = []*Certificate{
		{ID: "cert-1", Domain: "matched.example.com", Provider: "letsencrypt"},
	}

	result := NewSyncer(db).Sync(context.Background(), inv)
	if result.Certificates.Created != 1 {
		t.Fatalf("Certificates.Created = %d, want 1", result.Certificates.Created)
	}
}

// TestCovSyncAgentWithCapabilities 覆盖 agent 显式 capabilities 的直填路径
// （不触发 detectCapabilities 自动推断）。
func TestCovSyncAgentWithCapabilities(t *testing.T) {
	db := testDB(t)
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"r1": {
				Zones: map[string]*Zone{
					"z1": {
						Agents: map[string]*Agent{
							"a1": {
								ID:           "a1",
								Hostname:     "caps-host",
								Capabilities: []string{"docker", "pve"},
								Config:       map[string]any{"extra": true},
							},
						},
					},
				},
			},
		},
	}

	result := NewSyncer(db).Sync(context.Background(), inv)
	if result.Agents.Created != 1 {
		t.Fatalf("Agents.Created = %d, want 1", result.Agents.Created)
	}
	if result.Agents.Errors != 0 {
		t.Errorf("Agents.Errors = %d, want 0", result.Agents.Errors)
	}
}

// TestCovSyncDetectCapabilitiesEmptyConfig 覆盖 detectCapabilities 空 config 分支。
func TestCovSyncDetectCapabilitiesEmptyConfig(t *testing.T) {
	if got := detectCapabilities(nil); got != nil {
		t.Errorf("detectCapabilities(nil) = %v, want nil", got)
	}
	if got := detectCapabilities(map[string]any{}); got != nil {
		t.Errorf("detectCapabilities(empty) = %v, want nil", got)
	}
	got := detectCapabilities(map[string]any{"openwrt": true, "unknown": 1})
	if len(got) != 1 || got[0] != "openwrt" {
		t.Errorf("detectCapabilities(openwrt) = %v, want [openwrt]", got)
	}
}

// TestCovSyncComputeInstancesWithLabels 覆盖计算实例全字段的同步路径。
func TestCovSyncComputeInstancesWithLabels(t *testing.T) {
	db := testDB(t)
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{},
		ComputeInstances: map[string]*ComputeInstance{
			"ci1": {
				Name:     "vm1",
				Type:     "vm",
				Agent:    "a1",
				Region:   "r1",
				Zone:     "z1",
				CPU:      4,
				Memory:   8192,
				Disk:     100,
				IPv4:     "10.0.0.5",
				Labels:   map[string]string{"env": "cov"},
				Tags:     []string{"web"},
				Template: "tpl1",
			},
		},
	}

	result := NewSyncer(db).Sync(context.Background(), inv)
	if result.ComputeInstances.Created != 1 {
		t.Fatalf("ComputeInstances.Created = %d, want 1", result.ComputeInstances.Created)
	}
}

// TestCovSyncNilCertificate 覆盖 syncCertificates 对 nil 证书条目的跳过分支。
func TestCovSyncNilCertificate(t *testing.T) {
	db := testDB(t)
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{},
		Domains: map[string]*Domain{
			"d1": {
				Domain: "nil-cert.example.com",
				Certificates: []*Certificate{
					nil,
					{ID: "cert-ok", Domain: "nil-cert.example.com"},
				},
			},
		},
	}

	result := NewSyncer(db).Sync(context.Background(), inv)
	if result.Certificates.Created != 1 {
		t.Errorf("Certificates.Created = %d, want 1 (nil cert skipped)", result.Certificates.Created)
	}
}
