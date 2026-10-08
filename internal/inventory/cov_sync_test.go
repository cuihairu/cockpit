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

// TestCovSyncLocationUnknownDefault 拍板 2026-10-08（todo Phase 2.2 region/zone
// 校验项）：为空允许——四类资源省略 region/zone 不拒绝同步，落库统一标记
// "unknown"；显式字段（含空白）归一；用户 labels 已给的键不被空字段覆盖。
func TestCovSyncLocationUnknownDefault(t *testing.T) {
	db := testDB(t)
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{},
		ComputeInstances: map[string]*ComputeInstance{
			"ci-blank": {Name: "vm-blank", Type: "vm", Agent: "a1"},
			"ci-ws":    {Name: "vm-ws", Type: "vm", Agent: "a1", Region: "   ", Zone: "\t"},
			"ci-full":  {Name: "vm-full", Type: "vm", Agent: "a1", Region: "r1", Zone: "z1"},
		},
		Services: map[string]*Service{
			"svc-blank":   {Name: "web", Type: "http"},
			"svc-labeled": {Name: "web2", Type: "http", Labels: map[string]string{"region": "r9"}},
			"svc-field":   {Name: "web3", Type: "http", Region: "r1"},
		},
		Gateways: map[string]*Gateway{
			"gw-partial": {Name: "edge", Type: "openwrt", Region: "r1"},
		},
		Storages: map[string]*Storage{
			"st-blank": {Name: "data", Type: "nfs", Path: "/exports"},
		},
	}

	result := NewSyncer(db).Sync(context.Background(), inv)
	if result.ComputeInstances.Created != 3 || result.Services.Created != 3 ||
		result.Gateways.Created != 1 || result.Storages.Created != 1 {
		t.Fatalf("sync created = %+v, want all created (为空不拒绝)", result)
	}

	ci, err := db.GetComputeInstance("ci-blank")
	if err != nil || ci.Region != "unknown" || ci.Zone != "unknown" {
		t.Errorf("ci-blank region/zone = %q/%q (err=%v), want unknown/unknown", ci.Region, ci.Zone, err)
	}
	ci, err = db.GetComputeInstance("ci-ws")
	if err != nil || ci.Region != "unknown" || ci.Zone != "unknown" {
		t.Errorf("ci-ws 空白 region/zone = %q/%q, want 归一 unknown", ci.Region, ci.Zone)
	}
	ci, err = db.GetComputeInstance("ci-full")
	if err != nil || ci.Region != "r1" || ci.Zone != "z1" {
		t.Errorf("ci-full region/zone = %q/%q, want 显式值保留", ci.Region, ci.Zone)
	}

	svc, err := db.GetService("svc-blank")
	if err != nil || svc.Labels["region"] != "unknown" || svc.Labels["zone"] != "unknown" {
		t.Errorf("svc-blank labels = %v, want region/zone unknown", svc.Labels)
	}
	svc, err = db.GetService("svc-labeled")
	if err != nil || svc.Labels["region"] != "r9" || svc.Labels["zone"] != "unknown" {
		t.Errorf("svc-labeled labels = %v, want 用户 region=r9 保留 + zone=unknown", svc.Labels)
	}
	svc, err = db.GetService("svc-field")
	if err != nil || svc.Labels["region"] != "r1" {
		t.Errorf("svc-field labels = %v, want 显式 region=r1", svc.Labels)
	}

	gw, err := db.GetGateway("gw-partial")
	if err != nil || gw.Labels["region"] != "r1" || gw.Labels["zone"] != "unknown" {
		t.Errorf("gw-partial labels = %v, want region=r1 + zone=unknown", gw.Labels)
	}
	st, err := db.GetStorage("st-blank")
	if err != nil || st.Labels["region"] != "unknown" || st.Labels["zone"] != "unknown" {
		t.Errorf("st-blank labels = %v, want region/zone unknown", st.Labels)
	}
}
