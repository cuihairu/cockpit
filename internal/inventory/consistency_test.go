package inventory

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

// CMDB 一致性比对纯函数测试（M6 D29）：表驱动覆盖四态与归一化规则。

func invWithAgents(agents map[string]*Agent) *Inventory {
	return &Inventory{
		Regions: map[string]*Region{
			"home": {
				ID: "home",
				Zones: map[string]*Zone{
					"rack": {
						ID:     "rack",
						Agents: agents,
					},
				},
			},
		},
	}
}

func dbAgent(id, hostname, ip string) *storage.Agent {
	return &storage.Agent{ID: id, Hostname: hostname, IP: ip}
}

func TestCompareAgentsStatuses(t *testing.T) {
	inv := invWithAgents(map[string]*Agent{
		"ok-host":          {ID: "ok-host", Hostname: "NAS-Home", IP: "10.0.0.2"},
		"bad-host":         {ID: "bad-host", Hostname: "old-name", IP: "10.0.0.3"},
		"bad-ip":           {ID: "bad-ip", IP: "10.0.0.4"},
		"both-bad":         {ID: "both-bad", Hostname: "x", IP: "10.0.0.5"},
		"ghost":            {ID: "ghost", Hostname: "never-online"},
		"undeclared-agent": {ID: "undeclared-agent"}, // 声明为空 → 跳过比对算 ok
		"":                 {ID: "", Hostname: ""},
	})
	delete(inv.Regions["home"].Zones["rack"].Agents, "")

	agents := []*storage.Agent{
		dbAgent("ok-host", "nas-home", "10.0.0.2"),  // 大小写不敏感 → ok
		dbAgent("bad-host", "new-name", "10.0.0.3"), // hostname 漂移
		dbAgent("bad-ip", "bad-ip", "10.0.0.99"),    // ip 漂移
		dbAgent("both-bad", "y", "10.0.0.98"),       // 双字段漂移
		dbAgent("undeclared-agent", "anything", ""), // 空声明 → ok
		dbAgent("stray", "stray", "10.0.0.100"),     // 库里有、YAML 没有 → undeclared
	}

	report := CompareAgents(inv, agents)
	byID := map[string]AgentConsistency{}
	for _, a := range report.Agents {
		byID[a.ID] = a
	}

	if a := byID["ok-host"]; a.Status != ConsistencyOK {
		t.Errorf("ok-host status = %q (mismatch %v), want ok", a.Status, a.Mismatch)
	}
	if a := byID["bad-host"]; a.Status != ConsistencyMismatch || len(a.Mismatch) != 1 ||
		a.Mismatch[0].Field != "hostname" || a.Mismatch[0].Declared != "old-name" || a.Mismatch[0].Actual != "new-name" {
		t.Errorf("bad-host = %+v", a)
	}
	if a := byID["bad-ip"]; a.Status != ConsistencyMismatch || len(a.Mismatch) != 1 ||
		a.Mismatch[0].Field != "ip" || a.Mismatch[0].Actual != "10.0.0.99" {
		t.Errorf("bad-ip = %+v", a)
	}
	if a := byID["both-bad"]; a.Status != ConsistencyMismatch || len(a.Mismatch) != 2 {
		t.Errorf("both-bad = %+v, want 2 field mismatches", a)
	}
	if a := byID["ghost"]; a.Status != ConsistencyUnregistered || a.Actual != nil || a.Declared == nil {
		t.Errorf("ghost = %+v", a)
	}
	if a := byID["undeclared-agent"]; a.Status != ConsistencyOK {
		t.Errorf("undeclared-agent status = %q, want ok (空声明跳过)", a.Status)
	}
	if a := byID["stray"]; a.Status != ConsistencyUndeclared || a.Declared != nil || a.Actual == nil {
		t.Errorf("stray = %+v", a)
	}

	s := report.Summary
	want := ConsistencySummary{Total: 7, OK: 2, Mismatch: 3, Unregistered: 1, Undeclared: 1}
	if s != want {
		t.Errorf("summary = %+v, want %+v", s, want)
	}

	// 稳定排序（id 字典序）
	for i := 1; i < len(report.Agents); i++ {
		if report.Agents[i-1].ID > report.Agents[i].ID {
			t.Fatalf("agents not sorted: %v", report.Agents)
		}
	}
}

func TestCompareAgentsEmptyDeclarations(t *testing.T) {
	// 双侧全空：inventory 无 agent、库也空 → 报告为空且不 panic
	report := CompareAgents(invWithAgents(map[string]*Agent{}), nil)
	if report.Summary.Total != 0 || len(report.Agents) != 0 {
		t.Errorf("empty report = %+v", report)
	}
}

func TestCompareAgentsWhitespaceTrim(t *testing.T) {
	inv := invWithAgents(map[string]*Agent{
		"a1": {ID: "a1", Hostname: " nas ", IP: " 10.0.0.2 "},
	})
	report := CompareAgents(inv, []*storage.Agent{dbAgent("a1", "NAS", "10.0.0.2")})
	if report.Summary.OK != 1 {
		t.Errorf("trim should make it ok: %+v", report.Summary)
	}
}
