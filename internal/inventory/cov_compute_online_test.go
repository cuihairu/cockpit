package inventory

// cov_compute_online_test.go 覆盖 syncComputeInstances 的状态联动分支：
// 实例关联的 agent 在 DB 中在线 → 计算实例状态同步为 running（而非
// 默认 stopped）。

import (
	"context"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestCovSyncComputeInstanceOnlineAgentRunning(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 两个 agent：一个在线、一个离线
	if err := db.UpsertAgent(&storage.Agent{ID: "agent-on", Hostname: "h1", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAgent(&storage.Agent{ID: "agent-off", Hostname: "h2", Status: "offline"}); err != nil {
		t.Fatal(err)
	}

	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{},
		ComputeInstances: map[string]*ComputeInstance{
			"ci-on":  {Name: "vm-on", Type: "vm", Agent: "agent-on"},
			"ci-off": {Name: "vm-off", Type: "vm", Agent: "agent-off"},
		},
	}

	NewSyncer(db).Sync(context.Background(), inv)

	on, err := db.GetComputeInstance("ci-on")
	if err != nil {
		t.Fatalf("GetComputeInstance(ci-on): %v", err)
	}
	if on.Status != "running" {
		t.Errorf("online agent instance status = %q, want running", on.Status)
	}
	off, err := db.GetComputeInstance("ci-off")
	if err != nil {
		t.Fatalf("GetComputeInstance(ci-off): %v", err)
	}
	if off.Status != "stopped" {
		t.Errorf("offline agent instance status = %q, want stopped", off.Status)
	}
}
