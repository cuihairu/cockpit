package inventory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCovValidateNilEntries(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"nil-domain": nil,
			"d1":         {Domain: "example.com"},
		},
		Regions: map[string]*Region{
			"nil-region": nil,
			"r1": {
				Zones: map[string]*Zone{
					"nil-zone": nil,
					"z1": {
						Agents: map[string]*Agent{
							"nil-agent": nil,
							"a1":        {Hostname: "host1"},
						},
					},
				},
			},
		},
	}

	if err := inv.Validate(); err != nil {
		t.Fatalf("Validate() with nil entries error = %v", err)
	}
}

func TestCovResolveRefRegionShortPath(t *testing.T) {
	inv := testInventory()
	// parts = ["us-east"]，长度不足：invalid region ref
	if _, err := inv.ResolveRef("regions.us-east"); err == nil {
		t.Fatal("ResolveRef() should fail for region-only path")
	}
}

func TestCovResolveRefRegionNonZonesPath(t *testing.T) {
	inv := testInventory()
	// parts[1] 不是 "zones"：直接返回 region
	got, err := inv.ResolveRef("regions.us-east.labels")
	if err != nil {
		t.Fatalf("ResolveRef() error = %v", err)
	}
	if _, ok := got.(*Region); !ok {
		t.Errorf("ResolveRef() = %T, want *Region", got)
	}
}

func TestCovResolveRefZoneNotFound(t *testing.T) {
	inv := testInventory()
	if _, err := inv.ResolveRef("regions.us-east.zones.cov-missing"); err == nil {
		t.Fatal("ResolveRef() should fail for unknown zone")
	}
}

func TestCovResolveRefAgentNotFound(t *testing.T) {
	inv := testInventory()
	if _, err := inv.ResolveRef("regions.us-east.zones.us-east-1a.agents.cov-missing"); err == nil {
		t.Fatal("ResolveRef() should fail for unknown agent")
	}
}

func TestCovWriteMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	inv := &Inventory{Version: "v1"}
	err := inv.Write(filepath.Join(blocker, "sub", "inv.yaml"))
	if err == nil {
		t.Fatal("Write() should fail when directory cannot be created")
	}
}

// covBadMarshaler 实现 yaml.Marshaler 并始终返回错误，
// 用于触发 Write 中 yaml.Marshal 的错误分支。
// （注意：不能用 func 值——yaml.v3 v3.0.1 对 any 中的 func 是 panic 而非返回错误。）
type covBadMarshaler struct{}

func (covBadMarshaler) MarshalYAML() (interface{}, error) {
	return nil, errors.New("cov marshal failure")
}

func TestCovWriteMarshalError(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"r1": {
				Zones: map[string]*Zone{
					"z1": {
						Agents: map[string]*Agent{
							"a1": {
								Hostname: "host1",
								Config:   map[string]any{"bad": covBadMarshaler{}},
							},
						},
					},
				},
			},
		},
	}

	path := filepath.Join(t.TempDir(), "inv.yaml")
	if err := inv.Write(path); err == nil {
		t.Fatal("Write() should fail when inventory contains unmarshalable config")
	}
}
